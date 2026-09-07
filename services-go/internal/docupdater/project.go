package docupdater

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"
)

// flushProjectEveryNOps is how often the history queue is drained.
//
// project-history drains it on its own too; this only says "now" as the queue
// crosses another multiple, so it does not grow without bound between edits.
const flushProjectEveryNOps = 500

// ProjectStructureUpdate is one change to what a project contains: a file or a
// document added, or one renamed or deleted.
//
// It is carried as raw JSON because most of it is passed through to
// project-history untouched, and the fields this service does not read still
// have to arrive there.
type ProjectStructureUpdate struct {
	Type        string          `json:"type"`
	ID          string          `json:"id"`
	Pathname    string          `json:"pathname"`
	NewPathname string          `json:"newPathname"`
	DocLines    json.RawMessage `json:"docLines,omitempty"`
	URL         string          `json:"url,omitempty"`
	Hash        string          `json:"hash,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	CreatedBlob bool            `json:"createdBlob,omitempty"`
	Ranges      json.RawMessage `json:"ranges,omitempty"`

	// HistoryRangesSupport asks for the document to be recorded with its
	// tracked deletions put back in, which this port does not do.
	HistoryRangesSupport bool `json:"historyRangesSupport,omitempty"`

	// Version is filled in per update as the project version plus a counter,
	// so several changes to the same project version stay ordered.
	Version string `json:"version,omitempty"`

	// Raw is the update as it arrived.
	//
	// The fields this service passes through are copied from here rather than
	// from the struct above, so that one the caller left out is left out again.
	// The Node service builds these entries in JavaScript, where a field set to
	// nothing disappears when the entry is encoded, and project-history can
	// tell an absent field from an empty one.
	Raw map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON decodes the update and keeps a copy of what it contained.
func (u *ProjectStructureUpdate) UnmarshalJSON(data []byte) error {
	// A distinct type, or decoding would call this method again.
	type fields ProjectStructureUpdate
	var decoded fields
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*u = ProjectStructureUpdate(decoded)
	return json.Unmarshal(data, &u.Raw)
}

// UpdateProjectWithLocks records changes to what a project contains.
//
// Nothing here changes a document, only where it lives. The rename is applied
// to the copy in Redis so the next flush writes it back under its new name, and
// every change is queued for the history.
func (p *ProjectManager) UpdateProjectWithLocks(ctx context.Context, projectID,
	projectHistoryID, userID string, updates []ProjectStructureUpdate,
	projectVersion int64, originOrSource json.RawMessage) error {

	var queueLength int64
	for i, update := range updates {
		update.Version = fmt.Sprintf("%d.%d", projectVersion, i)

		var err error
		switch update.Type {
		case "add-doc":
			queueLength, err = p.history.QueueAddEntity(ctx, projectID, projectHistoryID,
				"doc", update.ID, userID, update, originOrSource)

		case "rename-doc":
			// An empty new pathname means the document was deleted, and there
			// is nothing in Redis left to rename.
			if update.NewPathname != "" {
				if err = p.docs.RenameDocWithLock(ctx, projectID, update.ID,
					update.NewPathname); err != nil {
					return err
				}
			}
			queueLength, err = p.history.QueueRenameEntity(ctx, projectID, projectHistoryID,
				"doc", update.ID, userID, update, originOrSource)

		case "add-file":
			queueLength, err = p.history.QueueAddEntity(ctx, projectID, projectHistoryID,
				"file", update.ID, userID, update, originOrSource)

		case "rename-file":
			queueLength, err = p.history.QueueRenameEntity(ctx, projectID, projectHistoryID,
				"file", update.ID, userID, update, originOrSource)

		default:
			return fmt.Errorf("unknown update type: %s", update.Type)
		}
		if err != nil {
			return err
		}
	}

	if shouldFlushHistoryOps(queueLength, len(updates), flushProjectEveryNOps) &&
		p.historyAPI != nil {
		p.historyAPI.FlushProjectChangesAsync(ctx, projectID)
	}
	return nil
}

// shouldFlushHistoryOps reports whether the queue has just crossed a multiple
// of the threshold, which is when it is drained.
//
// It is expressed as a crossing rather than a size so that one flush per
// threshold happens however many operations arrive at a time.
func shouldFlushHistoryOps(length int64, added, threshold int) bool {
	if length == 0 {
		// The length is unknown, so there is nothing to decide on.
		return false
	}
	before := (length - int64(added)) / int64(threshold)
	return length/int64(threshold) != before
}

// RenameDocWithLock records a document under its new name.
func (m *DocumentManager) RenameDocWithLock(ctx context.Context, projectID, docID,
	newPathname string) error {
	return m.locker.WithLock(ctx, docID, func(ctx context.Context) error {
		return m.redis.RenameDoc(ctx, projectID, docID, newPathname)
	})
}

// ResyncOptions are the parts of a resync request that change what is sent.
type ResyncOptions struct {
	// StructureOnly asks for the list of files and documents alone, without
	// the contents of each.
	StructureOnly bool
	// HistoryRangesMigration switches a project between recording its tracked
	// changes in the history and not. This port does not produce the form that
	// mode needs, so a request for it is refused.
	HistoryRangesMigration string
}

// ResyncProjectHistory tells project-history what a project contains now, so it
// can rebuild from a known state.
//
// It is what runs when the history and the project have drifted apart, which is
// why the contents are sent as well as the structure: whatever went wrong, this
// is the answer that replaces it.
func (p *ProjectManager) ResyncProjectHistory(ctx context.Context, projectID,
	projectHistoryID string, docs, files []json.RawMessage, opts ResyncOptions) error {
	if opts.HistoryRangesMigration != "" {
		return ErrHistoryRangesNotSupported
	}

	if _, err := p.history.QueueResyncProjectStructure(ctx, projectID, projectHistoryID,
		docs, files, opts); err != nil {
		return err
	}
	if opts.StructureOnly {
		return nil
	}

	for _, doc := range docs {
		var entry struct {
			Doc  string `json:"doc"`
			Path string `json:"path"`
		}
		if err := json.Unmarshal(doc, &entry); err != nil {
			return fmt.Errorf("malformed doc in resync request: %w", err)
		}
		if err := p.docs.ResyncDocContentsWithLock(ctx, projectID, projectHistoryID,
			entry.Doc, entry.Path); err != nil {
			return fmt.Errorf("resyncing doc %s: %w", entry.Doc, err)
		}
	}
	return nil
}

// ResyncDocContents queues one document as it stands now.
//
// The path comes from the resync request rather than from the document, because
// the same document id can appear under two paths while the two are out of step
// and project-history refuses a contents update for a path it was not told
// about.
func (m *DocumentManager) ResyncDocContents(ctx context.Context, projectID,
	projectHistoryID, docID, path string) error {
	doc, err := m.redis.GetDoc(ctx, projectID, docID)
	if err != nil {
		return err
	}

	lines, version := doc.Lines, doc.Version
	historyID := doc.ProjectHistoryID
	if !doc.Loaded() {
		// Not in Redis, so the database has the current copy. Peeking asks web
		// not to count the read, since nobody opened the document.
		persisted, err := m.persistence.GetDoc(ctx, projectID, docID, true)
		if err != nil {
			return err
		}
		if persisted.OTMigrationStage > 0 {
			return &OTTypeMismatchError{Got: TypeHistoryOT, Want: TypeShareJSTextOT}
		}
		lines, version = persisted.Lines, persisted.Version
		historyID = persisted.ProjectHistoryID
	} else if doc.Type() != TypeShareJSTextOT {
		return &OTTypeMismatchError{Got: doc.Type(), Want: TypeShareJSTextOT}
	}
	if historyID == "" {
		historyID = projectHistoryID
	}

	// The markers are not sent: without history ranges support the history
	// records the text alone.
	_, err = m.history.QueueResyncDocContent(ctx, projectID, historyID, docID,
		lines, version, path, m.maxDocLength)
	return err
}

// ResyncDocContentsWithLock takes the document lock first.
func (m *DocumentManager) ResyncDocContentsWithLock(ctx context.Context, projectID,
	projectHistoryID, docID, path string) error {
	return m.locker.WithLock(ctx, docID, func(ctx context.Context) error {
		return m.ResyncDocContents(ctx, projectID, projectHistoryID, docID, path)
	})
}

// DeleteQueueOptions bound one pass over the queue of projects waiting to be
// dropped.
type DeleteQueueOptions struct {
	// Limit is how many projects to flush before stopping.
	Limit int
	// Timeout is how long to keep going for.
	Timeout time.Duration
	// MinDeleteAge is how long a project has to have been waiting.
	MinDeleteAge time.Duration
}

// FlushAndDeleteOldProjects drops the projects that have been waiting long
// enough with nobody editing them.
//
// real-time asks for a project to be dropped when the last editor leaves, but
// somebody usually comes back, so the request waits a few minutes first. A
// project that was edited after the request was made is left alone: the edit
// means somebody is still there, and another request will follow when they go.
func (p *ProjectManager) FlushAndDeleteOldProjects(ctx context.Context, opts DeleteQueueOptions) (int, error) {
	start := time.Now()
	// The cutoff is jittered so that several instances doing this at once do
	// not all reach for the same project.
	jitter := time.Duration((rand.Float64() - 0.5) * float64(100*time.Millisecond))
	cutoff := start.Add(-opts.MinDeleteAge).Add(jitter).UnixMilli()

	var flushed int
	for {
		if time.Since(start) > opts.Timeout {
			p.log.Debug("hit the time limit flushing old projects")
			return flushed, nil
		}
		if flushed > opts.Limit {
			p.log.Debug("hit the count limit flushing old projects")
			return flushed, nil
		}

		candidate, err := p.redis.GetNextProjectToFlushAndDelete(ctx, cutoff)
		if err != nil {
			return flushed, err
		}
		if candidate == nil {
			return flushed, nil
		}

		done, err := p.flushIfNotModified(ctx, candidate)
		if err != nil {
			// One project failing does not stop the queue: leaving entries in
			// it is what fills Redis up.
			p.log.Error("error flushing a queued project",
				slog.String("project", candidate.ProjectID), slog.String("err", err.Error()))
		}
		if done {
			flushed++
		}
	}
}

// flushIfNotModified drops a project unless it was edited since it was queued.
func (p *ProjectManager) flushIfNotModified(ctx context.Context, candidate *FlushCandidate) (bool, error) {
	timestamps, err := p.GetProjectDocsTimestamps(ctx, candidate.ProjectID)
	if err != nil {
		return false, err
	}
	if len(timestamps) == 0 {
		// Nothing loaded, so there is nothing to flush.
		return false, nil
	}
	for _, ts := range timestamps {
		if ts == "" {
			continue
		}
		if parseMillis(ts) > candidate.FlushTimestamp {
			p.log.Debug("found a newer timestamp, skipping the delete",
				slog.String("project", candidate.ProjectID))
			return false, nil
		}
	}

	err = p.FlushAndDeleteProjectWithLocks(ctx, candidate.ProjectID, FlushOptions{})
	// Counted as done either way: the entry has been taken off the queue, and
	// a failure is reported by the caller rather than retried here.
	return true, err
}
