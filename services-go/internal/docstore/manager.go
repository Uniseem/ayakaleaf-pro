package docstore

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Manager is the docstore's business logic, the counterpart of DocManager.js.
type Manager struct {
	store    *Store
	archiver *Archiver
	log      *slog.Logger
	cfg      ArchiveConfig
}

// NewManager builds a Manager.
func NewManager(store *Store, archiver *Archiver, log *slog.Logger, cfg ArchiveConfig) *Manager {
	return &Manager{store: store, archiver: archiver, log: log, cfg: cfg}
}

// getDoc fetches a doc, unarchiving it first if its contents are in the
// persistor. The projection must ask for inS3, or the archived state cannot be
// detected.
func (m *Manager) getDoc(ctx context.Context, projectID, docID bson.ObjectID, projection bson.D) (*Doc, error) {
	for attempt := 0; attempt < 2; attempt++ {
		doc, err := m.store.FindDoc(ctx, projectID, docID, projection, false)
		if err != nil {
			return nil, err
		}
		if doc == nil {
			return nil, ErrNotFound
		}
		if doc.IsArchived() {
			if err := m.archiver.UnarchiveDoc(ctx, projectID, docID); err != nil {
				return nil, err
			}
			continue
		}
		if projectionWants(projection, "ranges") {
			FixCommentIds(doc.Ranges)
		}
		return doc, nil
	}
	return nil, ErrNotFound
}

var fullDocProjection = bson.D{
	{Key: "lines", Value: true},
	{Key: "rev", Value: true},
	{Key: "deleted", Value: true},
	{Key: "version", Value: true},
	{Key: "ranges", Value: true},
	{Key: "inS3", Value: true},
}

// GetFullDoc returns a doc with all of its content fields.
func (m *Manager) GetFullDoc(ctx context.Context, projectID, docID bson.ObjectID) (*Doc, error) {
	return m.getDoc(ctx, projectID, docID, fullDocProjection)
}

// IsDocDeleted reports a doc's soft-deleted state.
func (m *Manager) IsDocDeleted(ctx context.Context, projectID, docID bson.ObjectID) (bool, error) {
	doc, err := m.store.FindDoc(ctx, projectID, docID, bson.D{{Key: "deleted", Value: true}}, false)
	if err != nil {
		return false, err
	}
	if doc == nil {
		return false, ErrNotFound
	}
	return doc.IsDeleted(), nil
}

// GetDocLines returns a doc's contents as a single string.
func (m *Manager) GetDocLines(ctx context.Context, projectID, docID bson.ObjectID) (string, error) {
	doc, err := m.getDoc(ctx, projectID, docID, bson.D{
		{Key: "lines", Value: true}, {Key: "inS3", Value: true},
	})
	if err != nil {
		return "", err
	}
	if doc.Lines == nil {
		return "", ErrDocWithoutLines
	}
	return strings.Join(doc.Lines, "\n"), nil
}

// peekRawDoc reads a doc without unarchiving it, pulling the contents straight
// from the persistor when they are not in Mongo.
func (m *Manager) peekRawDoc(ctx context.Context, projectID, docID bson.ObjectID, projection bson.D, useSecondary bool) (*Doc, error) {
	full := append(bson.D{}, projection...)
	full = docSet(full, "rev", true)
	full = docSet(full, "inS3", true)

	doc, err := m.store.FindDoc(ctx, projectID, docID, full, useSecondary)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, ErrNotFound
	}
	if doc.IsArchived() {
		archived, err := m.archiver.GetDoc(ctx, projectID, docID)
		if err != nil {
			return nil, err
		}
		doc.Lines = archived.Lines
		if archived.Ranges != nil {
			doc.Ranges = archived.Ranges
		}
		// The rev check always goes to the primary.
		if err := m.store.CheckRevUnchanged(ctx, doc); err != nil {
			return nil, err
		}
	}
	return doc, nil
}

// PeekDoc reads a doc without writing anything back to Mongo.
func (m *Manager) PeekDoc(ctx context.Context, projectID, docID bson.ObjectID, projection bson.D, useSecondary bool) (*Doc, error) {
	doc, err := m.peekRawDoc(ctx, projectID, docID, projection, useSecondary)
	if errors.Is(err, ErrDocModified) {
		// Retry once on a rev mismatch, from the primary this time.
		return m.peekRawDoc(ctx, projectID, docID, projection, false)
	}
	return doc, err
}

// GetAllDeletedDocs lists a project's soft-deleted docs.
func (m *Manager) GetAllDeletedDocs(ctx context.Context, projectID bson.ObjectID, projection bson.D) ([]Doc, error) {
	return m.store.GetProjectsDeletedDocs(ctx, projectID, projection)
}

// GetAllNonDeletedDocs lists a project's live docs, unarchiving first.
func (m *Manager) GetAllNonDeletedDocs(ctx context.Context, projectID bson.ObjectID, projection bson.D) ([]Doc, error) {
	if err := m.archiver.UnArchiveAllDocs(ctx, projectID); err != nil {
		return nil, err
	}
	docs, err := m.store.GetProjectsDocs(ctx, projectID, ProjectDocsOptions{}, projection)
	if err != nil {
		return nil, err
	}
	if projectionWants(projection, "ranges") {
		for i := range docs {
			FixCommentIds(docs[i].Ranges)
		}
	}
	return docs, nil
}

// GetAllDocVersions lists doc versions without unarchiving: the version of an
// archived doc stays in Mongo.
func (m *Manager) GetAllDocVersions(ctx context.Context, projectID bson.ObjectID) ([]Doc, error) {
	return m.store.GetProjectsDocs(ctx, projectID, ProjectDocsOptions{}, bson.D{
		{Key: "_id", Value: true}, {Key: "version", Value: true},
	})
}

// GetCommentThreadIds maps each doc to the comment threads it carries.
func (m *Manager) GetCommentThreadIds(ctx context.Context, projectID bson.ObjectID) (map[string][]string, error) {
	docs, err := m.GetAllNonDeletedDocs(ctx, projectID, bson.D{
		{Key: "_id", Value: true}, {Key: "ranges", Value: true},
	})
	if err != nil {
		return nil, err
	}
	result := map[string][]string{}
	for i := range docs {
		ids := []string{}
		seen := map[string]bool{}
		for _, comment := range rangeEntries(docs[i].Ranges, "comments") {
			op, ok := docGet(comment, "op")
			if !ok {
				continue
			}
			opDoc, ok := op.(bson.D)
			if !ok {
				continue
			}
			t, ok := docGet(opDoc, "t")
			if !ok {
				continue
			}
			id := stringifyID(t)
			if id != "" && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			result[docs[i].ID.Hex()] = ids
		}
	}
	return result, nil
}

// GetTrackedChangesUserIds lists the users with tracked changes in a project.
func (m *Manager) GetTrackedChangesUserIds(ctx context.Context, projectID bson.ObjectID) ([]string, error) {
	docs, err := m.GetAllNonDeletedDocs(ctx, projectID, bson.D{{Key: "ranges", Value: true}})
	if err != nil {
		return nil, err
	}
	userIDs := []string{}
	seen := map[string]bool{}
	for i := range docs {
		for _, change := range rangeEntries(docs[i].Ranges, "changes") {
			metadata, ok := docGet(change, "metadata")
			if !ok {
				continue
			}
			metaDoc, ok := metadata.(bson.D)
			if !ok {
				continue
			}
			raw, ok := docGet(metaDoc, "user_id")
			if !ok {
				continue
			}
			id := stringifyID(raw)
			if id == "anonymous-user" || id == "" || seen[id] {
				continue
			}
			seen[id] = true
			userIDs = append(userIDs, id)
		}
	}
	return userIDs, nil
}

// ProjectHasRanges reports whether any doc in a project carries comments or
// tracked changes.
func (m *Manager) ProjectHasRanges(ctx context.Context, projectID bson.ObjectID, useSecondary bool) (bool, error) {
	docs, err := m.store.GetProjectsDocs(ctx, projectID,
		ProjectDocsOptions{UseSecondary: useSecondary},
		bson.D{{Key: "_id", Value: 1}})
	if err != nil {
		return false, err
	}
	for i := range docs {
		doc, err := m.PeekDoc(ctx, projectID, docs[i].ID,
			bson.D{{Key: "ranges", Value: true}}, useSecondary)
		if err != nil {
			return false, err
		}
		if len(rangeEntries(doc.Ranges, "comments")) > 0 ||
			len(rangeEntries(doc.Ranges, "changes")) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// UpdateResult reports what an update did.
type UpdateResult struct {
	Modified bool  `json:"modified"`
	Rev      int64 `json:"rev"`
}

// UpdateDoc writes new contents for a doc, retrying once when another update
// races it.
func (m *Manager) UpdateDoc(ctx context.Context, projectID, docID bson.ObjectID, lines []string, version int64, ranges any) (UpdateResult, error) {
	const maxAttempts = 2
	var result UpdateResult
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result, err = m.tryUpdateDoc(ctx, projectID, docID, lines, version, ranges)
		if err == nil {
			return result, nil
		}
		if errors.Is(err, ErrDocRevValue) && attempt < maxAttempts {
			m.log.Warn("detected concurrent updateDoc call",
				slog.String("projectId", projectID.Hex()),
				slog.String("docId", docID.Hex()))
			select {
			case <-time.After(time.Duration(100+rand.Intn(100)) * time.Millisecond):
			case <-ctx.Done():
				return result, ctx.Err()
			}
			continue
		}
		return result, err
	}
	return result, err
}

func (m *Manager) tryUpdateDoc(ctx context.Context, projectID, docID bson.ObjectID, lines []string, version int64, incoming any) (UpdateResult, error) {
	doc, err := m.getDoc(ctx, projectID, docID, bson.D{
		{Key: "version", Value: true},
		{Key: "rev", Value: true},
		{Key: "lines", Value: true},
		{Key: "ranges", Value: true},
		{Key: "inS3", Value: true},
	})
	if err != nil && !errors.Is(err, ErrNotFound) {
		return UpdateResult{}, err
	}
	if errors.Is(err, ErrNotFound) {
		doc = nil
	}

	ranges := JSONRangesToMongo(incoming)

	var updateLines, updateRanges, updateVersion bool
	if doc == nil {
		updateLines, updateRanges, updateVersion = true, true, true
	} else {
		if doc.Version != nil && *doc.Version > version {
			// A racing flush or broken history; refuse to go backwards.
			return UpdateResult{}, ErrDocVersionDecremented
		}
		updateLines = !equalLines(doc.Lines, lines)
		updateVersion = doc.Version == nil || *doc.Version != version
		updateRanges = ShouldUpdateRanges(rangesOrNil(doc.Ranges), ranges)
	}

	// A doc that does not exist yet starts at rev 0.
	result := UpdateResult{Rev: doc.RevValue()}

	if !updateLines && !updateRanges && !updateVersion {
		return result, nil
	}

	update := bson.D{}
	if updateLines {
		update = append(update, bson.E{Key: "lines", Value: lines})
	}
	if updateRanges {
		update = append(update, bson.E{Key: "ranges", Value: ranges})
	}
	if updateVersion {
		update = append(update, bson.E{Key: "version", Value: version})
	}
	if updateLines || updateRanges {
		result.Rev++
	}
	result.Modified = true

	var previousRev *int64
	if doc != nil {
		previousRev = doc.Rev
	}
	if err := m.store.UpsertIntoDocCollection(ctx, projectID, docID, previousRev, update); err != nil {
		return UpdateResult{}, err
	}
	return result, nil
}

// PatchDoc updates a doc's deleted/name metadata.
func (m *Manager) PatchDoc(ctx context.Context, projectID, docID bson.ObjectID, meta bson.D) error {
	doc, err := m.store.FindDoc(ctx, projectID, docID,
		bson.D{{Key: "_id", Value: 1}, {Key: "deleted", Value: true}}, false)
	if err != nil {
		return err
	}
	if doc == nil {
		return ErrNotFound
	}

	if deleted, ok := docGet(meta, "deleted"); ok && deleted == true && m.cfg.ArchiveOnSoftDelete {
		// Nobody will read this doc soon; flush it out of Mongo in the
		// background, and do not fail the request if that does not work.
		go func() {
			bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
			defer cancel()
			if err := m.archiver.ArchiveDoc(bg, projectID, docID); err != nil {
				m.log.Warn("archiving a single doc in the background failed",
					slog.String("projectId", projectID.Hex()),
					slog.String("docId", docID.Hex()),
					slog.String("err", err.Error()))
			}
		}()
	}
	return m.store.PatchDoc(ctx, projectID, docID, meta)
}

// DestroyProject removes a project's docs and archived contents.
func (m *Manager) DestroyProject(ctx context.Context, projectID bson.ObjectID) error {
	return m.archiver.DestroyProject(ctx, projectID)
}

// ArchiveAllDocs, ArchiveDoc and UnArchiveAllDocs forward to the archiver.
func (m *Manager) ArchiveAllDocs(ctx context.Context, projectID bson.ObjectID) error {
	return m.archiver.ArchiveAllDocs(ctx, projectID)
}

func (m *Manager) ArchiveDoc(ctx context.Context, projectID, docID bson.ObjectID) error {
	return m.archiver.ArchiveDoc(ctx, projectID, docID)
}

func (m *Manager) UnArchiveAllDocs(ctx context.Context, projectID bson.ObjectID) error {
	return m.archiver.UnArchiveAllDocs(ctx, projectID)
}

// rangeEntries returns the comments or changes array of a range document.
func rangeEntries(ranges bson.D, key string) []bson.D {
	if ranges == nil {
		return nil
	}
	raw, ok := docGet(ranges, key)
	if !ok {
		return nil
	}
	arr, ok := raw.(bson.A)
	if !ok {
		return nil
	}
	entries := make([]bson.D, 0, len(arr))
	for _, item := range arr {
		if entry, ok := item.(bson.D); ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

// stringifyID renders an id that may be stored as an ObjectId or a raw string.
func stringifyID(value any) string {
	switch v := value.(type) {
	case bson.ObjectID:
		return v.Hex()
	case string:
		return v
	default:
		return ""
	}
}

func rangesOrNil(ranges bson.D) any {
	if ranges == nil {
		return nil
	}
	return ranges
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
