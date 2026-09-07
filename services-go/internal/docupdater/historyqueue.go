package docupdater

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/textot"
)

// The entries below are what project-history reads to rebuild a project. They
// are written as maps rather than structs because the shape is the wire format
// of another service, and a field spelled differently here is a field that
// service silently ignores.

// QueueRenameEntity records that a document or a file moved.
func (q *HistoryQueue) QueueRenameEntity(ctx context.Context, projectID, projectHistoryID,
	entityType, entityID, userID string, update ProjectStructureUpdate,
	originOrSource json.RawMessage) (int64, error) {

	entry := map[string]any{
		"meta":             historyMeta(userID, originOrSource),
		"version":          update.Version,
		"projectHistoryId": projectHistoryID,
		entityType:         entityID,
	}
	copyPresent(entry, update.Raw, map[string]string{
		"pathname": "pathname", "newPathname": "new_pathname",
	})
	encoded, err := json.Marshal(entry)
	if err != nil {
		return 0, err
	}
	return q.QueueOps(ctx, projectID, encoded)
}

// QueueAddEntity records that a document or a file appeared.
func (q *HistoryQueue) QueueAddEntity(ctx context.Context, projectID, projectHistoryID,
	entityType, entityID, userID string, update ProjectStructureUpdate,
	originOrSource json.RawMessage) (int64, error) {

	if update.HistoryRangesSupport && len(update.Ranges) > 0 {
		// That mode stores the content with tracked deletions put back in and
		// the markers converted to the history form, neither of which this port
		// produces. Queueing the plain content instead would leave the history
		// quietly missing them.
		return 0, ErrHistoryRangesNotSupported
	}

	entry := map[string]any{
		"meta":             historyMeta(userID, originOrSource),
		"version":          update.Version,
		"projectHistoryId": projectHistoryID,
		// The one field with a default rather than an absence.
		"createdBlob": update.CreatedBlob,
		entityType:    entityID,
	}
	copyPresent(entry, update.Raw, map[string]string{
		"pathname": "pathname", "docLines": "docLines", "url": "url",
		"hash": "hash", "metadata": "metadata",
	})
	encoded, err := json.Marshal(entry)
	if err != nil {
		return 0, err
	}
	return q.QueueOps(ctx, projectID, encoded)
}

// QueueResyncProjectStructure records what a project contains now.
func (q *HistoryQueue) QueueResyncProjectStructure(ctx context.Context, projectID,
	projectHistoryID string, docs, files []json.RawMessage, opts ResyncOptions) (int64, error) {

	if docs == nil {
		docs = []json.RawMessage{}
	}
	if files == nil {
		files = []json.RawMessage{}
	}
	entry := map[string]any{
		"resyncProjectStructure": map[string]any{"docs": docs, "files": files},
		"projectHistoryId":       projectHistoryID,
		"meta":                   map[string]any{"ts": nowISO()},
	}
	if opts.StructureOnly {
		entry["resyncProjectStructureOnly"] = true
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return 0, err
	}
	return q.QueueOps(ctx, projectID, encoded)
}

// QueueResyncDocContent records what one document contains now.
func (q *HistoryQueue) QueueResyncDocContent(ctx context.Context, projectID,
	projectHistoryID, docID string, lines []string, version int64,
	pathname string, maxDocLength int) (int64, error) {

	entry := map[string]any{
		"resyncDocContent": map[string]any{
			"version": version,
			"content": textot.JoinLines(lines).String(),
		},
		"projectHistoryId": projectHistoryID,
		"path":             pathname,
		"doc":              docID,
		"meta":             map[string]any{"ts": nowISO()},
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return 0, err
	}

	// The encoded entry is an upper bound on the size of the document in it,
	// which is a cheap first check before counting the lines properly.
	if docIsTooLarge(len(encoded), lines, maxDocLength) {
		return 0, fmt.Errorf(
			"%w: refusing to queue a resync of doc %s, it is too large", ErrFileTooLarge, docID)
	}
	return q.QueueOps(ctx, projectID, encoded)
}

// copyPresent copies the fields the caller sent, under the names the history
// knows them by, and leaves out the ones it did not send.
func copyPresent(entry map[string]any, raw map[string]json.RawMessage, names map[string]string) {
	for from, to := range names {
		if value, ok := raw[from]; ok {
			entry[to] = value
		}
	}
}

// historyMeta is the who and when that every entry carries.
//
// A change from anywhere but the editor is marked external, which is how the
// history tells an edit somebody made from one a sync or an import made.
func historyMeta(userID string, originOrSource json.RawMessage) map[string]any {
	meta := map[string]any{"ts": nowISO()}
	if userID != "" {
		meta["user_id"] = userID
	} else {
		meta["user_id"] = nil
	}

	origin, source := splitOriginOrSource(originOrSource)
	switch {
	case origin != nil:
		meta["origin"] = origin
		var kind struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(origin, &kind); err == nil && kind.Kind != "editor" {
			meta["type"] = "external"
		}
	case source != nil:
		meta["source"] = source
		var name string
		if err := json.Unmarshal(source, &name); err == nil && name != "editor" {
			meta["type"] = "external"
		}
	}
	return meta
}
