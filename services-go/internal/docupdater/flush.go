package docupdater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
)

// maxUnflushedAge is how long a document may sit in Redis with unsaved changes
// before a read writes it back. It is a backstop for a document nobody is
// editing any more, not the usual path: an open document is flushed when the
// last editor leaves.
const maxUnflushedAge = 5 * 60 * 1000 // five minutes, in milliseconds

// FlushDocIfLoaded writes a document back to the database if it has changes
// that are not there yet.
//
// A document nobody has edited since it was loaded is left alone: the copy in
// the database is already the current one, and writing it again would cost a
// round trip and a new revision for nothing.
func (m *DocumentManager) FlushDocIfLoaded(ctx context.Context, projectID, docID string) (json.RawMessage, error) {
	doc, err := m.redis.GetDoc(ctx, projectID, docID)
	if err != nil {
		return nil, err
	}
	if !doc.Loaded() {
		m.log.Debug("doc is not loaded so not flushing",
			slog.String("project", projectID), slog.String("doc", docID))
		return nil, nil
	}
	if doc.UnflushedTime == "" {
		m.log.Debug("doc is not modified so not flushing",
			slog.String("project", projectID), slog.String("doc", docID))
		return nil, nil
	}
	if doc.Type() != TypeShareJSTextOT {
		return nil, &OTTypeMismatchError{Got: doc.Type(), Want: TypeShareJSTextOT}
	}

	m.log.Debug("flushing doc", slog.String("project", projectID),
		slog.String("doc", docID), slog.Int64("version", doc.Version))

	result, err := m.persistence.SetDoc(ctx, projectID, docID, doc.Lines, doc.Version,
		doc.Ranges, doc.LastUpdatedAt, doc.LastUpdatedBy)
	if err != nil {
		return nil, err
	}
	if err := m.redis.ClearUnflushedTime(ctx, docID); err != nil {
		return nil, err
	}
	return result, nil
}

// FlushDocIfLoadedWithLock takes the document lock first.
func (m *DocumentManager) FlushDocIfLoadedWithLock(ctx context.Context, projectID, docID string) error {
	return m.locker.WithLock(ctx, docID, func(ctx context.Context) error {
		_, err := m.FlushDocIfLoaded(ctx, projectID, docID)
		return err
	})
}

// FlushAndDeleteDoc writes a document back and then drops it from Redis.
//
// ignoreFlushErrors is for a caller that is deleting the project anyway: a
// document whose write fails would otherwise stay in Redis for ever, holding
// memory for a project that no longer exists.
func (m *DocumentManager) FlushAndDeleteDoc(ctx context.Context, projectID, docID string, ignoreFlushErrors bool) (json.RawMessage, error) {
	result, err := m.FlushDocIfLoaded(ctx, projectID, docID)
	if err != nil {
		if !ignoreFlushErrors {
			return nil, err
		}
		m.log.Warn("ignoring flush error while deleting document",
			slog.String("project", projectID), slog.String("doc", docID),
			slog.String("err", err.Error()))
	}
	if err := m.redis.RemoveDocFromMemory(ctx, projectID, docID); err != nil {
		return nil, err
	}
	return result, nil
}

// FlushAndDeleteDocWithLock takes the document lock first.
func (m *DocumentManager) FlushAndDeleteDocWithLock(ctx context.Context, projectID, docID string, ignoreFlushErrors bool) error {
	return m.locker.WithLock(ctx, docID, func(ctx context.Context) error {
		_, err := m.FlushAndDeleteDoc(ctx, projectID, docID, ignoreFlushErrors)
		return err
	})
}

// GetDocAndFlushIfOld returns a document, writing it back first if it has been
// sitting in Redis with unsaved changes for too long.
//
// This is what the compile path calls: it is about to read every document in
// the project, which is a good moment to notice one that was left unflushed.
func (m *DocumentManager) GetDocAndFlushIfOld(ctx context.Context, projectID, docID string) (*Doc, error) {
	loaded, err := m.GetDoc(ctx, projectID, docID)
	if err != nil {
		return nil, err
	}
	if loaded.Type() != TypeShareJSTextOT {
		return nil, &OTTypeMismatchError{Got: loaded.Type(), Want: TypeShareJSTextOT}
	}

	// A document this call loaded cannot be stale, whatever its timestamp says.
	if loaded.AlreadyLoaded && loaded.UnflushedTime != "" {
		if nowMillis()-parseMillis(loaded.UnflushedTime) > maxUnflushedAge {
			if _, err := m.FlushDocIfLoaded(ctx, projectID, docID); err != nil {
				return nil, err
			}
		}
	}
	return loaded.Doc, nil
}

// GetDocAndFlushIfOldWithLock takes the document lock first.
func (m *DocumentManager) GetDocAndFlushIfOldWithLock(ctx context.Context, projectID, docID string) (*Doc, error) {
	var doc *Doc
	err := m.locker.WithLock(ctx, docID, func(ctx context.Context) error {
		var err error
		doc, err = m.GetDocAndFlushIfOld(ctx, projectID, docID)
		return err
	})
	return doc, err
}

// FlushProjectWithLocks writes every loaded document of a project back.
//
// One document failing does not stop the others: the point of the call is to
// get as much as possible into the database, and stopping at the first failure
// would leave the rest unflushed for no gain.
func (p *ProjectManager) FlushProjectWithLocks(ctx context.Context, projectID string) error {
	docIDs, err := p.redis.GetDocIDsInProject(ctx, projectID)
	if err != nil {
		return err
	}
	p.log.Debug("flushing docs", slog.String("project", projectID),
		slog.Int("docs", len(docIDs)))

	var failures int
	for _, docID := range docIDs {
		err := p.docs.FlushDocIfLoadedWithLock(ctx, projectID, docID)
		switch {
		case err == nil:
		case errors.Is(err, ErrNotFound):
			// The document was deleted while it sat in Redis. There is nothing
			// left to write it back to, and nothing to be done about it here.
			p.log.Warn("found deleted doc when flushing",
				slog.String("project", projectID), slog.String("doc", docID))
		default:
			p.log.Error("error flushing doc", slog.String("project", projectID),
				slog.String("doc", docID), slog.String("err", err.Error()))
			failures++
		}
	}
	if failures > 0 {
		return fmt.Errorf("errors flushing docs: %d of %d failed", failures, len(docIDs))
	}
	return nil
}

// FlushAndDeleteProjectWithLocks writes every document of a project back and
// drops the project from Redis.
func (p *ProjectManager) FlushAndDeleteProjectWithLocks(ctx context.Context, projectID string, opts FlushOptions) error {
	docIDs, err := p.redis.GetDocIDsInProject(ctx, projectID)
	if err != nil {
		return err
	}
	p.log.Debug("deleting docs", slog.String("project", projectID),
		slog.Int("docs", len(docIDs)))

	var failures int
	for _, docID := range docIDs {
		if err := p.docs.FlushAndDeleteDocWithLock(ctx, projectID, docID, false); err != nil {
			p.log.Error("error deleting doc", slog.String("project", projectID),
				slog.String("doc", docID), slog.String("err", err.Error()))
			failures++
		}
	}

	// The history queue is drained here rather than in the background: web may
	// delete the project as soon as this call returns, and a flush after that
	// has nothing left to flush into.
	if p.historyAPI != nil {
		if err := p.historyAPI.FlushProjectChanges(ctx, projectID, opts); err != nil {
			return err
		}
	}
	if failures > 0 {
		return fmt.Errorf("errors deleting docs: %d of %d failed", failures, len(docIDs))
	}
	return nil
}

// QueueFlushAndDeleteProject leaves the project for the background worker to
// flush and drop, for a caller that does not need it gone now.
func (p *ProjectManager) QueueFlushAndDeleteProject(ctx context.Context, projectID string) error {
	return p.redis.QueueFlushAndDeleteProject(ctx, projectID)
}

// ProjectDoc is one document as the compile path reads it.
type ProjectDoc struct {
	ID      string   `json:"_id"`
	Lines   []string `json:"lines"`
	Version int64    `json:"v"`
}

// GetProjectDocsAndFlushIfOld returns every document of a project, provided the
// structure the caller expects still matches.
//
// The hash is how the caller says which set of documents it thinks the project
// has. If one has been added or removed since, what is in Redis no longer
// answers the question that was asked, so the caller is told to go to the
// database rather than given a stale answer.
func (p *ProjectManager) GetProjectDocsAndFlushIfOld(ctx context.Context, projectID, projectStateHash string) ([]ProjectDoc, error) {
	changed, err := p.redis.CheckOrSetProjectState(ctx, projectID, projectStateHash)
	if err != nil {
		return nil, err
	}
	if changed {
		return nil, ErrProjectStateChanged
	}

	docIDs, err := p.redis.GetDocIDsInProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	docs := make([]ProjectDoc, 0, len(docIDs))
	for _, docID := range docIDs {
		doc, err := p.docs.GetDocAndFlushIfOldWithLock(ctx, projectID, docID)
		if err != nil {
			return nil, err
		}
		docs = append(docs, ProjectDoc{ID: docID, Lines: doc.Lines, Version: doc.Version})
	}
	return docs, nil
}

// parseMillis reads a millisecond timestamp as it is stored in Redis, which is
// as a decimal string.
func parseMillis(stored string) int64 {
	parsed, err := strconv.ParseInt(stored, 10, 64)
	if err != nil {
		// An unreadable timestamp is treated as the epoch, which makes the
		// document look old and gets it flushed. That is the safe way round:
		// the alternative leaves changes unsaved.
		return 0
	}
	return parsed
}
