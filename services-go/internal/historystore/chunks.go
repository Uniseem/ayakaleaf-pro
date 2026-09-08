package historystore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/histmodel"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/persistor"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Chunks.
//
// A chunk is a snapshot and the changes made after it. Reading a version means
// finding the chunk it falls in and moving that snapshot forward, so a history
// of ten thousand edits is a handful of chunks rather than ten thousand copies
// of the project.
//
// A chunk stops growing at some point and the next one starts from where it
// finished. The index in Mongo says which versions each chunk covers; the
// chunk itself is JSON in the object store.

const (
	// maxChanges is how long a chunk grows before the next one starts. Bigger
	// means fewer objects and slower reads of a recent version, because every
	// read replays the changes after the snapshot.
	maxChanges = 1000
	// maxChunkBytes is the other limit. A project of very large files reaches
	// it long before it reaches the change count.
	maxChunkBytes = 16 << 20
)

// chunk states. A chunk is written before it is pointed at, so that a reader
// never sees an index entry for an object that is not there yet.
const (
	statePending = "pending"
	stateActive  = "active"
	stateClosed  = "closed"
)

type chunkRecord struct {
	ID           bson.ObjectID `bson:"_id"`
	ProjectID    bson.ObjectID `bson:"projectId"`
	StartVersion int           `bson:"startVersion"`
	EndVersion   int           `bson:"endVersion"`
	EndTimestamp *time.Time    `bson:"endTimestamp,omitempty"`
	State        string        `bson:"state"`
	UpdatedAt    time.Time     `bson:"updatedAt"`
}

// Version is how far a project's history goes, without the changes.
type Version struct {
	StartVersion int        `json:"startVersion"`
	EndVersion   int        `json:"endVersion"`
	EndTimestamp *time.Time `json:"endTimestamp,omitempty"`
}

// LatestVersion is where a project's history has got to.
func (s *Store) LatestVersion(ctx context.Context, historyID string) (*Version, error) {
	record, err := s.latestRecord(ctx, historyID)
	if err != nil {
		return nil, err
	}
	return &Version{
		StartVersion: record.StartVersion,
		EndVersion:   record.EndVersion,
		EndTimestamp: record.EndTimestamp,
	}, nil
}

// LatestChunk is the end of a project's history.
func (s *Store) LatestChunk(ctx context.Context, historyID string) (*histmodel.Chunk, error) {
	record, err := s.latestRecord(ctx, historyID)
	if err != nil {
		return nil, err
	}
	return s.readChunk(ctx, historyID, record)
}

// ChunkAtVersion is the run of history a version falls in.
func (s *Store) ChunkAtVersion(ctx context.Context, historyID string, version int) (*histmodel.Chunk, error) {
	id, err := bson.ObjectIDFromHex(historyID)
	if err != nil {
		return nil, fmt.Errorf("%w: %q is not a project", ErrBadRequest, historyID)
	}
	var record chunkRecord
	err = s.chunks.FindOne(ctx, bson.M{
		"projectId":    id,
		"state":        bson.M{"$in": bson.A{stateActive, stateClosed}},
		"startVersion": bson.M{"$lte": version},
		"endVersion":   bson.M{"$gte": version},
	}, options.FindOne().SetSort(bson.D{{Key: "startVersion", Value: 1}})).Decode(&record)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.readChunk(ctx, historyID, &record)
}

// InitialiseProject gives a project an empty history.
//
// Every project needs one before anything can be recorded against it, and it
// is the empty snapshot at version zero: a project with no files that nobody
// has changed.
func (s *Store) InitialiseProject(ctx context.Context, historyID string) error {
	id, err := bson.ObjectIDFromHex(historyID)
	if err != nil {
		return fmt.Errorf("%w: %q is not a project", ErrBadRequest, historyID)
	}
	if err := s.InitialiseBlobs(ctx, historyID); err != nil {
		return err
	}

	existing, err := s.chunks.CountDocuments(ctx, bson.M{
		"projectId": id,
		"state":     bson.M{"$in": bson.A{stateActive, stateClosed}},
	})
	if err != nil {
		return err
	}
	if existing > 0 {
		// Already has a history. Making a second empty one would hide it.
		return nil
	}

	chunk := &histmodel.Chunk{
		StartVersion: 0,
		History: &histmodel.History{
			Snapshot: histmodel.NewSnapshot(),
			Changes:  []*histmodel.Change{},
		},
	}
	return s.writeChunk(ctx, historyID, chunk, "")
}

// AppendChanges adds changes to the end of a project's history.
//
// endVersion is what the caller believes the history ends at. If it does not,
// somebody else wrote first and this refuses rather than writing changes on top
// of a project that has moved -- which would record edits against text they
// were not made to.
func (s *Store) AppendChanges(
	ctx context.Context,
	historyID string,
	changes []*histmodel.Change,
	endVersion int,
) error {
	record, err := s.latestRecord(ctx, historyID)
	if err != nil {
		return err
	}
	if record.EndVersion != endVersion {
		return fmt.Errorf("%w: the history is at %d, not %d",
			ErrConflict, record.EndVersion, endVersion)
	}
	if len(changes) == 0 {
		return nil
	}

	chunk, err := s.readChunk(ctx, historyID, record)
	if err != nil {
		return err
	}

	// Everything that fits goes on the end of the chunk that is already there;
	// the rest starts a new one from where that finished. Two writes rather
	// than one, and the alternative is a chunk that grows until reading a
	// recent version means replaying a hundred thousand changes.
	room := maxChanges - len(chunk.History.Changes)
	if room > len(changes) {
		room = len(changes)
	}
	if room < 0 {
		room = 0
	}
	if size(chunk) > maxChunkBytes {
		room = 0
	}

	if room > 0 {
		chunk.History.Changes = append(chunk.History.Changes, changes[:room]...)
		if err := s.writeChunk(ctx, historyID, chunk, record.ID.Hex()); err != nil {
			return err
		}
		changes = changes[room:]
		if len(changes) == 0 {
			return nil
		}
		record, err = s.latestRecord(ctx, historyID)
		if err != nil {
			return err
		}
		chunk, err = s.readChunk(ctx, historyID, record)
		if err != nil {
			return err
		}
	}

	for len(changes) > 0 {
		// The new chunk starts from what the old one finished as, so nothing
		// before it has to be read to know what the project looked like.
		snapshot, err := chunk.GetSnapshotAt(chunk.EndVersion())
		if err != nil {
			return err
		}
		batch := changes
		if len(batch) > maxChanges {
			batch = batch[:maxChanges]
		}
		next := &histmodel.Chunk{
			StartVersion: chunk.EndVersion(),
			History: &histmodel.History{
				Snapshot: snapshot,
				Changes:  append([]*histmodel.Change{}, batch...),
			},
		}
		if err := s.writeChunk(ctx, historyID, next, record.ID.Hex()); err != nil {
			return err
		}
		changes = changes[len(batch):]
		chunk = next
		record, err = s.latestRecord(ctx, historyID)
		if err != nil {
			return err
		}
	}
	return nil
}

// CopyHistory gives one project another's history.
func (s *Store) CopyHistory(ctx context.Context, fromHistoryID, toHistoryID string) error {
	if err := s.CopyBlobs(ctx, fromHistoryID, toHistoryID); err != nil &&
		!errors.Is(err, ErrNotFound) {
		return err
	}
	chunk, err := s.LatestChunk(ctx, fromHistoryID)
	if err != nil {
		return err
	}
	// One chunk, not the whole history: a copy starts from what the project is
	// now, and everything before that belongs to the project it was copied
	// from.
	snapshot, err := chunk.GetSnapshotAt(chunk.EndVersion())
	if err != nil {
		return err
	}
	copied := &histmodel.Chunk{
		StartVersion: 0,
		History: &histmodel.History{
			Snapshot: snapshot,
			Changes:  []*histmodel.Change{},
		},
	}
	if err := s.InitialiseBlobs(ctx, toHistoryID); err != nil {
		return err
	}
	return s.writeChunk(ctx, toHistoryID, copied, "")
}

// --- the two halves of a write ---------------------------------------------

// writeChunk stores a chunk and then points the index at it.
//
// In that order, and with the index entry written as pending first, so that
// there is never an entry a reader could follow to an object that is not
// there. A pending entry nobody activates is a few kilobytes of rubbish; the
// other way round is a history that cannot be read.
func (s *Store) writeChunk(ctx context.Context, historyID string, chunk *histmodel.Chunk, replacing string) error {
	id, err := bson.ObjectIDFromHex(historyID)
	if err != nil {
		return fmt.Errorf("%w: %q is not a project", ErrBadRequest, historyID)
	}

	chunkID := bson.NewObjectID()
	_, err = s.chunks.InsertOne(ctx, chunkRecord{
		ID:           chunkID,
		ProjectID:    id,
		StartVersion: chunk.StartVersion,
		EndVersion:   chunk.EndVersion(),
		EndTimestamp: endTimestamp(chunk),
		State:        statePending,
		UpdatedAt:    time.Now().UTC(),
	})
	if err != nil {
		return err
	}

	encoded, err := json.Marshal(chunk.History)
	if err != nil {
		return err
	}
	key := objectKey(historyID) + "/" + pad(chunkID.Hex())
	err = s.objects.SendStream(ctx, s.buckets.Chunks, key,
		newReader(encoded), persistor.GetOptions{UseSubdirectories: true})
	if err != nil {
		return err
	}

	// The old chunk is closed and the new one activated together: a moment
	// with both active would answer two different things to the same question.
	if replacing != "" {
		oldID, err := bson.ObjectIDFromHex(replacing)
		if err == nil {
			_, err = s.chunks.UpdateOne(ctx,
				bson.M{"_id": oldID, "projectId": id, "state": stateActive},
				bson.M{"$set": bson.M{"state": stateClosed, "updatedAt": time.Now().UTC()}})
			if err != nil {
				return err
			}
		}
	}
	result, err := s.chunks.UpdateOne(ctx,
		bson.M{"_id": chunkID, "projectId": id, "state": statePending},
		bson.M{"$set": bson.M{"state": stateActive, "updatedAt": time.Now().UTC()}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("%w: the chunk was not there to activate", ErrConflict)
	}
	return nil
}

// readChunk fetches a chunk's contents.
func (s *Store) readChunk(ctx context.Context, historyID string, record *chunkRecord) (*histmodel.Chunk, error) {
	key := objectKey(historyID) + "/" + pad(record.ID.Hex())
	body, err := s.objects.GetObject(ctx, s.buckets.Chunks, key,
		persistor.GetOptions{UseSubdirectories: true})
	if notFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(body, 512<<20))
	if err != nil {
		return nil, err
	}
	history := &histmodel.History{}
	if err := json.Unmarshal(raw, history); err != nil {
		return nil, err
	}
	if history.Snapshot == nil {
		history.Snapshot = histmodel.NewSnapshot()
	}
	if history.Changes == nil {
		history.Changes = []*histmodel.Change{}
	}
	return &histmodel.Chunk{History: history, StartVersion: record.StartVersion}, nil
}

// latestRecord is the index entry for the end of a project's history.
func (s *Store) latestRecord(ctx context.Context, historyID string) (*chunkRecord, error) {
	id, err := bson.ObjectIDFromHex(historyID)
	if err != nil {
		return nil, fmt.Errorf("%w: %q is not a project", ErrBadRequest, historyID)
	}
	var record chunkRecord
	err = s.chunks.FindOne(ctx,
		bson.M{
			"projectId": id,
			"state":     bson.M{"$in": bson.A{stateActive, stateClosed}},
		},
		options.FindOne().SetSort(bson.D{{Key: "startVersion", Value: -1}}),
	).Decode(&record)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// EnsureIndexes creates what every read here relies on.
func (s *Store) EnsureIndexes(ctx context.Context) error {
	_, err := s.chunks.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "projectId", Value: 1}, {Key: "startVersion", Value: 1}},
			Options: options.Index().SetName("projectId_1_startVersion_1").
				// One chunk may start at a version, which is what stops two
				// writers from both extending the history from the same place.
				SetUnique(true).
				SetPartialFilterExpression(bson.M{
					"state": bson.M{"$in": bson.A{stateActive, stateClosed}},
				}),
		},
		{
			Keys:    bson.D{{Key: "projectId", Value: 1}, {Key: "endVersion", Value: 1}},
			Options: options.Index().SetName("projectId_1_endVersion_1"),
		},
		{
			Keys:    bson.D{{Key: "state", Value: 1}, {Key: "updatedAt", Value: 1}},
			Options: options.Index().SetName("state_1_updatedAt_1"),
		},
	})
	if err != nil {
		var cmdErr mongo.CommandError
		if errors.As(err, &cmdErr) && (cmdErr.Code == 85 || cmdErr.Code == 86) {
			return nil
		}
		return err
	}
	return nil
}

// endTimestamp is when the last change in a chunk was made, which is how a
// chunk is found by date rather than by version.
func endTimestamp(chunk *histmodel.Chunk) *time.Time {
	if chunk.History == nil || len(chunk.History.Changes) == 0 {
		return nil
	}
	last := chunk.History.Changes[len(chunk.History.Changes)-1]
	if last == nil || last.Timestamp.IsZero() {
		return nil
	}
	at := last.Timestamp
	return &at
}

// size is roughly how big a chunk is, for deciding when to start another.
func size(chunk *histmodel.Chunk) int {
	encoded, err := json.Marshal(chunk.History)
	if err != nil {
		return 0
	}
	return len(encoded)
}
