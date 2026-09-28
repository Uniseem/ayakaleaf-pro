package projecthistory

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Three collections, and each of them is a record of something that has to
// survive a restart: which projects failed and why, the labels people have put
// on versions, and where a project has got to in being resynced.

// Store is the Mongo side of project-history.
type Store struct {
	failures  *mongo.Collection
	labels    *mongo.Collection
	syncState *mongo.Collection
	projects  *mongo.Collection
}

// NewStore builds the store.
func NewStore(database *mongo.Database) *Store {
	return &Store{
		failures:  database.Collection("projectHistoryFailures"),
		labels:    database.Collection("projectHistoryLabels"),
		syncState: database.Collection("projectHistorySyncState"),
		projects:  database.Collection("projects"),
	}
}

// objectIDFor turns a project id into the id these collections are keyed by.
//
// Most are twenty-four hex characters. A few are numbers -- projects whose
// history predates the current store -- and the reference implementation turns
// one of those into an object id built from the number as a timestamp, so this
// does too: a project's labels have to be looked for where they were put.
func objectIDFor(projectID string) (bson.ObjectID, error) {
	if id, err := bson.ObjectIDFromHex(projectID); err == nil {
		return id, nil
	}
	number, err := strconv.ParseUint(projectID, 10, 32)
	if err != nil {
		return bson.ObjectID{}, fmt.Errorf("%w: not a project id: %s",
			ErrBadRequest, projectID)
	}
	var id bson.ObjectID
	binary.BigEndian.PutUint32(id[0:4], uint32(number))
	return id, nil
}

// StoredID is a project id as it is stored.
//
// This service writes them as strings, because a history id need not be an
// object id at all, but records written elsewhere hold them as object ids. A
// record that could not be read would take the whole listing with it.
type StoredID string

// UnmarshalBSONValue reads an id stored either way.
func (s *StoredID) UnmarshalBSONValue(bsonType byte, data []byte) error {
	value := bson.RawValue{Type: bson.Type(bsonType), Value: data}
	switch bson.Type(bsonType) {
	case bson.TypeString:
		text, ok := value.StringValueOK()
		if !ok {
			return errors.New("unreadable project id")
		}
		*s = StoredID(text)
	case bson.TypeObjectID:
		id, ok := value.ObjectIDOK()
		if !ok {
			return errors.New("unreadable project id")
		}
		*s = StoredID(id.Hex())
	case bson.TypeNull, bson.TypeUndefined:
		*s = ""
	default:
		*s = StoredID(value.String())
	}
	return nil
}

// MarshalBSONValue writes an id as a string, which is how this service stores
// them.
func (s StoredID) MarshalBSONValue() (byte, []byte, error) {
	bsonType, data, err := bson.MarshalValue(string(s))
	return byte(bsonType), data, err
}

// String is the id as text.
func (s StoredID) String() string { return string(s) }

// Failure is what is recorded when a project cannot be processed.
type Failure struct {
	ProjectID StoredID  `bson:"project_id" json:"project_id"`
	Attempts  int       `bson:"attempts" json:"attempts"`
	QueueSize int       `bson:"queueSize" json:"queueSize"`
	Error     string    `bson:"error" json:"error"`
	Stack     string    `bson:"stack" json:"stack"`
	TS        time.Time `bson:"ts" json:"ts"`
	// ResyncStartedAt is set while a resync is running, so a project stuck
	// mid-resync can be told from one that simply failed.
	ResyncStartedAt *time.Time `bson:"resyncStartedAt,omitempty" json:"resyncStartedAt,omitempty"`
	// RequestCount is how many times the last failure has been asked about,
	// which tells a project somebody is watching from one nobody is.
	RequestCount int `bson:"requestCount,omitempty" json:"requestCount,omitempty"`
	// ResyncAttempts is how many times the project has been sent again, which
	// is what decides whether it gets another one.
	ResyncAttempts int `bson:"resyncAttempts,omitempty" json:"resyncAttempts,omitempty"`
	// ForceDebug lets a project be processed past a history whose versions are
	// out of order, so that somebody can see what it does next.
	ForceDebug bool `bson:"forceDebug,omitempty" json:"forceDebug,omitempty"`
	// History is the last few failures, newest first.
	History []bson.M `bson:"history,omitempty" json:"history,omitempty"`
}

// RecordFailure notes that a project could not be processed.
//
// The last ten failures are kept alongside the current one: one failure says
// little, and the same failure ten times in a row says the project needs
// looking at rather than retrying.
func (s *Store) RecordFailure(ctx context.Context, projectID string,
	queueSize int, failure error) error {

	record := bson.M{
		"queueSize": queueSize,
		"error":     failure.Error(),
		"stack":     "",
		"ts":        time.Now(),
	}
	_, err := s.failures.UpdateOne(ctx,
		bson.M{"project_id": projectID},
		bson.M{
			"$set": record,
			"$inc": bson.M{"attempts": 1},
			"$push": bson.M{
				"history": bson.M{
					"$each": []any{record}, "$position": 0, "$slice": 10,
				},
			},
		},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

// ClearFailure forgets a project that has since been processed.
func (s *Store) ClearFailure(ctx context.Context, projectID string) error {
	_, err := s.failures.DeleteOne(ctx, bson.M{"project_id": projectID})
	return err
}

// GetFailure returns the record for one project, or nil.
func (s *Store) GetFailure(ctx context.Context, projectID string) (*Failure, error) {
	var failure Failure
	err := s.failures.FindOne(ctx, bson.M{"project_id": projectID}).Decode(&failure)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	failure.Error = normalizeFailureMessage(failure.Error)
	return &failure, nil
}

// GetFailures returns every failure record.
func (s *Store) GetFailures(ctx context.Context) ([]Failure, error) {
	cursor, err := s.failures.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var failures []Failure
	if err := cursor.All(ctx, &failures); err != nil {
		return nil, err
	}
	for i := range failures {
		failures[i].Error = normalizeFailureMessage(failures[i].Error)
	}
	return failures, nil
}

// normalizeFailureMessage rewrites the one prefix that two versions of the
// error library disagree about, so that counting failures by message does not
// split the same failure in two.
//
// It is done on the way out rather than on the way in, so that a record
// written before this existed is counted the same as one written now.
func normalizeFailureMessage(message string) string {
	if strings.Contains(message, "OError:") {
		return strings.ReplaceAll(message, "OError:", "Error:")
	}
	return message
}

// Label is a name somebody has put on a version.
type Label struct {
	ID        bson.ObjectID  `bson:"_id,omitempty"`
	ProjectID bson.ObjectID  `bson:"project_id"`
	UserID    *bson.ObjectID `bson:"user_id,omitempty"`
	Comment   string         `bson:"comment"`
	Version   int            `bson:"version"`
	CreatedAt time.Time      `bson:"created_at"`
}

// MarshalJSON writes a label the way the editor reads it: the stored field
// names, with the mongo id under "id".
func (l Label) MarshalJSON() ([]byte, error) {
	fields := map[string]any{
		"id": l.ID.Hex(), "comment": l.Comment, "version": l.Version,
		"created_at": l.CreatedAt,
	}
	if l.UserID != nil {
		fields["user_id"] = l.UserID.Hex()
	} else {
		// Present and null rather than absent, because a label with no user is
		// one made by the system and the editor tells the two apart.
		fields["user_id"] = nil
	}
	return json.Marshal(fields)
}

// CloneLabels copies one project's labels onto another, which is what copying
// a project needs so that the copy keeps the versions somebody marked.
func (s *Store) CloneLabels(ctx context.Context, fromProjectID, toProjectID string) error {
	from, err := objectIDFor(fromProjectID)
	if err != nil {
		return err
	}
	to, err := objectIDFor(toProjectID)
	if err != nil {
		return err
	}

	cursor, err := s.labels.Find(ctx, bson.M{"project_id": from},
		options.Find().SetProjection(bson.M{"_id": 0, "project_id": 0}))
	if err != nil {
		return err
	}
	var labels []bson.M
	if err := cursor.All(ctx, &labels); err != nil {
		return err
	}
	if len(labels) == 0 {
		return nil
	}

	documents := make([]any, 0, len(labels))
	for _, label := range labels {
		label["project_id"] = to
		documents = append(documents, label)
	}
	_, err = s.labels.InsertMany(ctx, documents)
	return err
}

// GetLabels returns the labels on a project.
func (s *Store) GetLabels(ctx context.Context, projectID string) ([]Label, error) {
	id, err := objectIDFor(projectID)
	if err != nil {
		return nil, err
	}
	cursor, err := s.labels.Find(ctx, bson.M{"project_id": id})
	if err != nil {
		return nil, err
	}
	var labels []Label
	if err := cursor.All(ctx, &labels); err != nil {
		return nil, err
	}
	return labels, nil
}

// CreateLabel puts a label on a version.
func (s *Store) CreateLabel(ctx context.Context, projectID, userID, comment string,
	version int, createdAt time.Time) (*Label, error) {

	project, err := objectIDFor(projectID)
	if err != nil {
		return nil, err
	}
	label := Label{
		ProjectID: project, Comment: comment, Version: version, CreatedAt: createdAt,
	}
	if userID != "" {
		user, err := bson.ObjectIDFromHex(userID)
		if err != nil {
			return nil, err
		}
		label.UserID = &user
	}

	result, err := s.labels.InsertOne(ctx, label)
	if err != nil {
		return nil, err
	}
	if id, ok := result.InsertedID.(bson.ObjectID); ok {
		label.ID = id
	}
	return &label, nil
}

// DeleteLabel takes a label off, whoever put it there.
func (s *Store) DeleteLabel(ctx context.Context, projectID, labelID string) error {
	project, err := objectIDFor(projectID)
	if err != nil {
		return err
	}
	label, err := bson.ObjectIDFromHex(labelID)
	if err != nil {
		return err
	}
	_, err = s.labels.DeleteOne(ctx, bson.M{"_id": label, "project_id": project})
	return err
}

// DeleteLabelForUser takes off a label the given user put there.
//
// The user id is part of the query rather than checked beforehand: it is what
// stops one person removing another person's label.
func (s *Store) DeleteLabelForUser(ctx context.Context, projectID, userID, labelID string) error {
	project, err := objectIDFor(projectID)
	if err != nil {
		return err
	}
	user, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return err
	}
	label, err := bson.ObjectIDFromHex(labelID)
	if err != nil {
		return err
	}
	_, err = s.labels.DeleteOne(ctx,
		bson.M{"_id": label, "project_id": project, "user_id": user})
	return err
}

// TransferLabels moves every label from one user to another, which is what
// happens when an account is merged.
func (s *Store) TransferLabels(ctx context.Context, fromUserID, toUserID string) error {
	from, err := bson.ObjectIDFromHex(fromUserID)
	if err != nil {
		return err
	}
	to, err := bson.ObjectIDFromHex(toUserID)
	if err != nil {
		return err
	}
	_, err = s.labels.UpdateMany(ctx,
		bson.M{"user_id": from}, bson.M{"$set": bson.M{"user_id": to}})
	return err
}

// RawSyncState is the record of where a project has got to in being resynced.
//
// It is read and written whole rather than field by field: the fields are the
// business of SyncManager, and passing it through keeps the two apart.
type RawSyncState bson.M

// GetSyncState reads it, or nil when the project has never been resynced.
func (s *Store) GetSyncState(ctx context.Context, projectID string) (RawSyncState, error) {
	id, err := objectIDFor(projectID)
	if err != nil {
		return nil, err
	}
	var state bson.M
	err = s.syncState.FindOne(ctx, bson.M{"project_id": id}).Decode(&state)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return RawSyncState(state), nil
}

// SetSyncState writes it, keeping the last few states alongside so that a
// project that keeps getting stuck can be looked at.
func (s *Store) SetSyncState(ctx context.Context, projectID string, state bson.M) error {
	id, err := objectIDFor(projectID)
	if err != nil {
		return err
	}
	_, err = s.syncState.UpdateOne(ctx,
		bson.M{"project_id": id},
		bson.M{
			"$set": state,
			"$push": bson.M{
				"history": bson.M{
					"$each": []any{bson.M{
						"syncState": state, "timestamp": time.Now(),
					}},
					"$position": 0, "$slice": 10,
				},
			},
		},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

// ClearSyncState forgets it.
func (s *Store) ClearSyncState(ctx context.Context, projectID string) error {
	id, err := objectIDFor(projectID)
	if err != nil {
		return err
	}
	_, err = s.syncState.DeleteOne(ctx, bson.M{"project_id": id})
	return err
}

// CloneSyncState copies the state of one project onto another, which is what a
// project copied from a template needs.
func (s *Store) CloneSyncState(ctx context.Context, fromProjectID, toProjectID string) error {
	from, err := objectIDFor(fromProjectID)
	if err != nil {
		return err
	}
	to, err := objectIDFor(toProjectID)
	if err != nil {
		return err
	}

	var state bson.M
	err = s.syncState.FindOne(ctx, bson.M{"project_id": from},
		options.FindOne().SetProjection(bson.M{"_id": 0, "project_id": 0})).Decode(&state)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil
	}
	if err != nil {
		return err
	}

	state["project_id"] = to
	_, err = s.syncState.InsertOne(ctx, state)
	return err
}
