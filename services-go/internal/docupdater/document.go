package docupdater

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// DocumentManager reads documents, loading them from the database into Redis
// the first time somebody asks.
type DocumentManager struct {
	redis       *RedisStore
	persistence *PersistenceClient
	locker      *Locker
	history     *HistoryClient
	log         *slog.Logger

	// maxDocLength is the size a document may not exceed, in characters. The
	// append path checks it before doing the work of a diff.
	maxDocLength int

	// updates is what a write through the API becomes once the difference has
	// been worked out. It is set after construction because the two managers
	// need each other: the update manager reads documents through this one.
	updates *UpdateManager
}

// NewDocumentManager builds a document manager.
func NewDocumentManager(redis *RedisStore, persistence *PersistenceClient,
	locker *Locker, history *HistoryClient, maxDocLength int,
	log *slog.Logger) *DocumentManager {
	return &DocumentManager{
		redis: redis, persistence: persistence, locker: locker, history: history,
		maxDocLength: maxDocLength, log: log,
	}
}

// UseUpdateManager completes the wiring for the write paths.
func (m *DocumentManager) UseUpdateManager(updates *UpdateManager) {
	m.updates = updates
}

// LoadedDoc is a document together with how it was obtained.
type LoadedDoc struct {
	*Doc
	// AlreadyLoaded is false when this call is what brought the document into
	// Redis. The flush-if-old path uses it: a document just loaded cannot be
	// stale.
	AlreadyLoaded bool
}

// GetDoc returns a document, loading it from the database if it is not already
// in Redis.
func (m *DocumentManager) GetDoc(ctx context.Context, projectID, docID string) (*LoadedDoc, error) {
	doc, err := m.redis.GetDoc(ctx, projectID, docID)
	if err != nil {
		return nil, err
	}
	if doc.Loaded() {
		return &LoadedDoc{Doc: doc, AlreadyLoaded: true}, nil
	}

	m.log.Debug("doc not in redis so getting from persistence API",
		slog.String("project", projectID), slog.String("doc", docID))

	persisted, err := m.persistence.GetDoc(ctx, projectID, docID, false)
	if err != nil {
		return nil, err
	}
	// A project migrated to history-ot stores its content differently and needs
	// an OT implementation this port does not have. Loading it as though it
	// were a ShareJS document would corrupt it on the first edit.
	if persisted.OTMigrationStage > 0 {
		return nil, &OTTypeMismatchError{Got: TypeHistoryOT, Want: TypeShareJSTextOT}
	}

	if err := m.redis.PutDocInMemory(ctx, projectID, docID, persisted.Lines,
		persisted.Version, persisted.Ranges, persisted.ResolvedCommentIDs,
		persisted.Pathname, persisted.ProjectHistoryID,
		persisted.HistoryRangesSupport); err != nil {
		return nil, err
	}

	ranges := persisted.Ranges
	if len(ranges) == 0 {
		ranges = json.RawMessage("{}")
	}
	raw, err := json.Marshal(persisted.Lines)
	if err != nil {
		return nil, err
	}
	return &LoadedDoc{
		Doc: &Doc{
			RawLines:             raw,
			Lines:                persisted.Lines,
			Version:              persisted.Version,
			Ranges:               ranges,
			Pathname:             persisted.Pathname,
			ProjectHistoryID:     persisted.ProjectHistoryID,
			HistoryRangesSupport: persisted.HistoryRangesSupport,
			ResolvedCommentIDs:   persisted.ResolvedCommentIDs,
			// A document just loaded has nothing unsaved in it.
			UnflushedTime: "",
		},
		AlreadyLoaded: false,
	}, nil
}

// DocWithOps is a document plus the operations a client needs to catch up.
type DocWithOps struct {
	*Doc
	Ops []json.RawMessage
}

// GetDocAndRecentOps returns a document and the operations applied since
// fromVersion, so a reconnecting client can catch up rather than reload.
//
// A fromVersion of -1 asks for the document alone.
func (m *DocumentManager) GetDocAndRecentOps(ctx context.Context, projectID, docID string, fromVersion int64) (*DocWithOps, error) {
	loaded, err := m.GetDoc(ctx, projectID, docID)
	if err != nil {
		return nil, err
	}
	if fromVersion == -1 {
		return &DocWithOps{Doc: loaded.Doc, Ops: []json.RawMessage{}}, nil
	}
	ops, err := m.redis.GetPreviousDocOps(ctx, docID, fromVersion, loaded.Version)
	if err != nil {
		return nil, err
	}
	return &DocWithOps{Doc: loaded.Doc, Ops: ops}, nil
}

// GetDocAndRecentOpsWithLock does the same while holding the document lock, so
// the version and the operations it returns cannot be from either side of an
// update being applied.
func (m *DocumentManager) GetDocAndRecentOpsWithLock(ctx context.Context, projectID, docID string, fromVersion int64) (*DocWithOps, error) {
	var result *DocWithOps
	err := m.locker.WithLock(ctx, docID, func(ctx context.Context) error {
		var err error
		result, err = m.GetDocAndRecentOps(ctx, projectID, docID, fromVersion)
		return err
	})
	return result, err
}

// PeekDoc returns a document only if it is already in Redis, without loading it
// from the database.
func (m *DocumentManager) PeekDoc(ctx context.Context, projectID, docID string) (*Doc, error) {
	doc, err := m.redis.GetDoc(ctx, projectID, docID)
	if err != nil {
		return nil, err
	}
	if !doc.Loaded() {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, docID)
	}
	return doc, nil
}

// ProjectManager answers questions about a whole project.
type ProjectManager struct {
	redis   *RedisStore
	docs    *DocumentManager
	history *HistoryClient
	log     *slog.Logger
}

// NewProjectManager builds a project manager.
func NewProjectManager(redis *RedisStore, docs *DocumentManager, history *HistoryClient,
	log *slog.Logger) *ProjectManager {
	return &ProjectManager{redis: redis, docs: docs, history: history, log: log}
}

// DocRanges is one document's tracked changes and comments.
type DocRanges struct {
	ID     string          `json:"id"`
	Ranges json.RawMessage `json:"ranges"`
}

// GetProjectRanges lists the tracked changes and comments of every loaded
// document in a project.
func (p *ProjectManager) GetProjectRanges(ctx context.Context, projectID string) ([]DocRanges, error) {
	docIDs, err := p.redis.GetDocIDsInProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	docs := make([]DocRanges, 0, len(docIDs))
	for _, docID := range docIDs {
		ranges, err := p.redis.GetDocRanges(ctx, docID)
		if err != nil {
			return nil, err
		}
		docs = append(docs, DocRanges{ID: docID, Ranges: ranges})
	}
	return docs, nil
}

// GetProjectDocsTimestamps lists when each loaded document was last changed.
func (p *ProjectManager) GetProjectDocsTimestamps(ctx context.Context, projectID string) ([]string, error) {
	docIDs, err := p.redis.GetDocIDsInProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if len(docIDs) == 0 {
		return []string{}, nil
	}
	return p.redis.GetDocTimestamps(ctx, docIDs)
}

// ClearProjectState forgets the recorded document list, so the next read
// rebuilds it.
func (p *ProjectManager) ClearProjectState(ctx context.Context, projectID string) error {
	return p.redis.ClearProjectState(ctx, projectID)
}
