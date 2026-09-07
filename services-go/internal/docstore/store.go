package docstore

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// Store is the data access layer for the docstore service, reading and writing
// the same `docs` documents as the Node implementation.
type Store struct {
	docs                 *mongo.Collection
	secondary            *mongo.Collection
	maxDeletedDocs       int64
	archivingLockTimeout time.Duration
}

// NewStore binds a Store to the docs collection of db.
func NewStore(db *mongo.Database, maxDeletedDocs int64, archivingLockTimeout time.Duration) *Store {
	return &Store{
		docs: db.Collection("docs"),
		secondary: db.Collection("docs",
			options.Collection().SetReadPreference(readpref.Secondary())),
		maxDeletedDocs:       maxDeletedDocs,
		archivingLockTimeout: archivingLockTimeout,
	}
}

func (s *Store) collection(useSecondary bool) *mongo.Collection {
	if useSecondary {
		return s.secondary
	}
	return s.docs
}

// FindDoc returns one doc, or nil when it does not exist.
//
// A doc written before versions were tracked has no version field; when the
// caller asked for one, it reads back as 0.
func (s *Store) FindDoc(ctx context.Context, projectID, docID bson.ObjectID, projection bson.D, useSecondary bool) (*Doc, error) {
	var doc Doc
	err := s.collection(useSecondary).FindOne(ctx,
		bson.D{{Key: "_id", Value: docID}, {Key: "project_id", Value: projectID}},
		options.FindOne().SetProjection(projection),
	).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if projectionWants(projection, "version") && doc.Version == nil {
		doc.Version = int64Ptr(0)
	}
	return &doc, nil
}

func projectionWants(projection bson.D, key string) bool {
	value, present := docGet(projection, key)
	if !present {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case int32:
		return v != 0
	case int64:
		return v != 0
	default:
		return true
	}
}

// GetProjectsDeletedDocs returns a project's soft-deleted docs, newest first.
func (s *Store) GetProjectsDeletedDocs(ctx context.Context, projectID bson.ObjectID, projection bson.D) ([]Doc, error) {
	cur, err := s.docs.Find(ctx,
		bson.D{{Key: "project_id", Value: projectID}, {Key: "deleted", Value: true}},
		options.Find().
			SetProjection(projection).
			SetSort(bson.D{{Key: "deletedAt", Value: -1}}).
			SetLimit(s.maxDeletedDocs),
	)
	if err != nil {
		return nil, err
	}
	return decodeDocs(ctx, cur)
}

// ProjectDocsOptions controls GetProjectsDocs.
type ProjectDocsOptions struct {
	IncludeDeleted bool
	UseSecondary   bool
	Limit          int64
}

// GetProjectsDocs returns a project's docs.
func (s *Store) GetProjectsDocs(ctx context.Context, projectID bson.ObjectID, opts ProjectDocsOptions, projection bson.D) ([]Doc, error) {
	query := bson.D{{Key: "project_id", Value: projectID}}
	if !opts.IncludeDeleted {
		query = append(query, bson.E{Key: "deleted", Value: bson.D{{Key: "$ne", Value: true}}})
	}
	findOpts := options.Find().SetProjection(projection)
	if opts.Limit > 0 {
		findOpts.SetLimit(opts.Limit)
	}
	cur, err := s.collection(opts.UseSecondary).Find(ctx, query, findOpts)
	if err != nil {
		return nil, err
	}
	return decodeDocs(ctx, cur)
}

// GetArchivedProjectDocs returns up to maxResults archived docs of a project.
func (s *Store) GetArchivedProjectDocs(ctx context.Context, projectID bson.ObjectID, maxResults int64) ([]Doc, error) {
	return s.findIDs(ctx, bson.D{
		{Key: "project_id", Value: projectID},
		{Key: "inS3", Value: true},
	}, maxResults)
}

// GetNonDeletedArchivedProjectDocs is GetArchivedProjectDocs restricted to
// docs that have not been soft-deleted.
func (s *Store) GetNonDeletedArchivedProjectDocs(ctx context.Context, projectID bson.ObjectID, maxResults int64) ([]Doc, error) {
	return s.findIDs(ctx, bson.D{
		{Key: "project_id", Value: projectID},
		{Key: "deleted", Value: bson.D{{Key: "$ne", Value: true}}},
		{Key: "inS3", Value: true},
	}, maxResults)
}

// GetNonArchivedProjectDocIDs lists the ids of a project's unarchived docs.
func (s *Store) GetNonArchivedProjectDocIDs(ctx context.Context, projectID bson.ObjectID) ([]bson.ObjectID, error) {
	docs, err := s.findIDs(ctx, bson.D{
		{Key: "project_id", Value: projectID},
		{Key: "inS3", Value: bson.D{{Key: "$ne", Value: true}}},
	}, 0)
	if err != nil {
		return nil, err
	}
	ids := make([]bson.ObjectID, 0, len(docs))
	for _, d := range docs {
		ids = append(ids, d.ID)
	}
	return ids, nil
}

func (s *Store) findIDs(ctx context.Context, query bson.D, maxResults int64) ([]Doc, error) {
	opts := options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}})
	if maxResults > 0 {
		opts.SetLimit(maxResults)
	}
	cur, err := s.docs.Find(ctx, query, opts)
	if err != nil {
		return nil, err
	}
	return decodeDocs(ctx, cur)
}

func decodeDocs(ctx context.Context, cur *mongo.Cursor) ([]Doc, error) {
	docs := []Doc{}
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}

// updateToPipeline mirrors MongoManager.convertUpdateToPipeline.
//
// The update has to run as an aggregation pipeline with every value wrapped in
// $literal. Document lines routinely begin with "$" -- the acceptance suite
// stores "$1.00" and "$foo" precisely to catch this -- and a plain $set would
// read those as field paths and substitute them.
func updateToPipeline(set bson.D, unset []string) mongo.Pipeline {
	pipeline := mongo.Pipeline{}
	for _, e := range set {
		pipeline = append(pipeline, bson.D{{Key: "$set", Value: bson.D{
			{Key: e.Key, Value: bson.D{{Key: "$literal", Value: e.Value}}},
		}}})
	}
	for _, field := range unset {
		pipeline = append(pipeline, bson.D{{Key: "$unset", Value: field}})
	}
	return pipeline
}

// UpsertIntoDocCollection writes a doc update, bumping rev when the contents
// changed. previousRev is nil for a doc that does not exist yet.
func (s *Store) UpsertIntoDocCollection(ctx context.Context, projectID, docID bson.ObjectID, previousRev *int64, update bson.D) error {
	if previousRev != nil && *previousRev != 0 {
		set := update
		if _, hasLines := docGet(update, "lines"); hasLines {
			set = docSet(set, "rev", *previousRev+1)
		} else if _, hasRanges := docGet(update, "ranges"); hasRanges {
			set = docSet(set, "rev", *previousRev+1)
		}
		result, err := s.docs.UpdateOne(ctx,
			bson.D{
				{Key: "_id", Value: docID},
				{Key: "project_id", Value: projectID},
				{Key: "rev", Value: *previousRev},
			},
			updateToPipeline(set, []string{"inS3"}),
		)
		if err != nil {
			return err
		}
		if result.MatchedCount != 1 {
			return ErrDocRevValue
		}
		return nil
	}

	doc := bson.D{
		{Key: "_id", Value: docID},
		{Key: "project_id", Value: projectID},
		{Key: "rev", Value: int64(1)},
	}
	doc = append(doc, update...)
	_, err := s.docs.InsertOne(ctx, doc)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDocRevValue
	}
	return err
}

// PatchDoc sets the deleted/deletedAt/name metadata of a doc.
func (s *Store) PatchDoc(ctx context.Context, projectID, docID bson.ObjectID, meta bson.D) error {
	_, err := s.docs.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: docID}, {Key: "project_id", Value: projectID}},
		bson.D{{Key: "$set", Value: meta}},
	)
	return err
}

// GetDocForArchiving fetches a doc and takes the archiving lock on it.
//
// It returns nil when the doc is missing, already archived, or locked by
// another archiving run — the caller cannot tell which, and does not need to.
func (s *Store) GetDocForArchiving(ctx context.Context, projectID, docID bson.ObjectID) (*Doc, error) {
	now := time.Now()
	var doc Doc
	err := s.docs.FindOneAndUpdate(ctx,
		bson.D{
			{Key: "_id", Value: docID},
			{Key: "project_id", Value: projectID},
			{Key: "inS3", Value: bson.D{{Key: "$ne", Value: true}}},
			{Key: "$or", Value: bson.A{
				bson.D{{Key: "archivingUntil", Value: nil}},
				bson.D{{Key: "archivingUntil", Value: bson.D{{Key: "$lt", Value: now}}}},
			}},
		},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "archivingUntil", Value: now.Add(s.archivingLockTimeout)},
		}}},
		options.FindOneAndUpdate().SetProjection(bson.D{
			{Key: "lines", Value: 1}, {Key: "ranges", Value: 1}, {Key: "rev", Value: 1},
		}),
	).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

// MarkDocAsArchived clears the contents from Mongo and releases the lock.
func (s *Store) MarkDocAsArchived(ctx context.Context, docID bson.ObjectID, rev int64) error {
	_, err := s.docs.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: docID}, {Key: "rev", Value: rev}},
		bson.D{
			{Key: "$set", Value: bson.D{{Key: "inS3", Value: true}}},
			{Key: "$unset", Value: bson.D{
				{Key: "lines", Value: 1}, {Key: "ranges", Value: 1}, {Key: "archivingUntil", Value: 1},
			}},
		},
	)
	return err
}

// RestoreArchivedDoc writes an archived doc's contents back into Mongo, but
// only if its rev still matches.
func (s *Store) RestoreArchivedDoc(ctx context.Context, projectID, docID bson.ObjectID, archived *ArchivedDoc) error {
	ranges := archived.Ranges
	if ranges == nil {
		ranges = bson.D{}
	}
	set := bson.D{
		{Key: "lines", Value: archived.Lines},
		{Key: "ranges", Value: ranges},
	}
	result, err := s.docs.UpdateOne(ctx,
		bson.D{
			{Key: "_id", Value: docID},
			{Key: "project_id", Value: projectID},
			{Key: "rev", Value: archived.Rev},
		},
		updateToPipeline(set, []string{"inS3"}),
	)
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return ErrDocRevValue
	}
	return nil
}

// CheckRevUnchanged verifies that a doc's rev has not moved since it was read.
func (s *Store) CheckRevUnchanged(ctx context.Context, doc *Doc) error {
	var current Doc
	err := s.docs.FindOne(ctx,
		bson.D{{Key: "_id", Value: doc.ID}},
		options.FindOne().SetProjection(bson.D{{Key: "rev", Value: 1}}),
	).Decode(&current)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrDocRevValue
	}
	if err != nil {
		return err
	}
	if doc.Rev == nil || current.Rev == nil {
		return ErrDocRevValue
	}
	if *doc.Rev != *current.Rev {
		return ErrDocModified
	}
	return nil
}

// DestroyProject removes every doc of a project.
func (s *Store) DestroyProject(ctx context.Context, projectID bson.ObjectID) error {
	_, err := s.docs.DeleteMany(ctx, bson.D{{Key: "project_id", Value: projectID}})
	return err
}
