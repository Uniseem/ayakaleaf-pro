package docupdater

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/rediskeys"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/textot"
	"github.com/redis/go-redis/v9"
)

// UpdateManager applies the updates queued by real-time.
type UpdateManager struct {
	redis     *RedisStore
	docs      *DocumentManager
	locker    *Locker
	realtime  *RealTimeBridge
	history   *HistoryQueue
	web       *WebClient
	snapshots *SnapshotStore
	log       *slog.Logger

	maxDocLength int
}

// NewUpdateManager builds the update pipeline.
func NewUpdateManager(redis *RedisStore, docs *DocumentManager, locker *Locker,
	realtime *RealTimeBridge, history *HistoryQueue, web *WebClient,
	snapshots *SnapshotStore, maxDocLength int, log *slog.Logger) *UpdateManager {
	return &UpdateManager{
		redis: redis, docs: docs, locker: locker, realtime: realtime,
		history: history, web: web, snapshots: snapshots,
		maxDocLength: maxDocLength, log: log,
	}
}

// ProcessOutstandingUpdatesWithLock applies whatever is queued for a document.
//
// It takes the lock without waiting: if another worker holds it, that worker is
// already doing this work and will keep going until the queue is empty.
func (m *UpdateManager) ProcessOutstandingUpdatesWithLock(ctx context.Context, projectID, docID string) error {
	value, err := m.locker.TryLock(ctx, docID)
	if err != nil {
		return err
	}
	if value == "" {
		return nil
	}

	processErr := m.processOutstandingUpdates(ctx, projectID, docID)
	if err := m.locker.Unlock(ctx, docID, value); err != nil && processErr == nil {
		return err
	}
	if processErr != nil {
		return processErr
	}

	// Updates may have queued up while the lock was held, and nothing else will
	// come along to notice: the queue entry that triggered this was already
	// consumed.
	return m.continueProcessing(ctx, projectID, docID)
}

func (m *UpdateManager) continueProcessing(ctx context.Context, projectID, docID string) error {
	length, err := m.realtime.GetUpdatesLength(ctx, docID)
	if err != nil {
		return err
	}
	if length > 0 {
		return m.ProcessOutstandingUpdatesWithLock(ctx, projectID, docID)
	}
	return nil
}

func (m *UpdateManager) processOutstandingUpdates(ctx context.Context, projectID, docID string) error {
	updates, err := m.realtime.GetPendingUpdatesForDoc(ctx, docID)
	if err != nil {
		return err
	}
	if len(updates) == 0 {
		return nil
	}
	m.log.Debug("processing updates", slog.String("project", projectID),
		slog.String("doc", docID), slog.Int("count", len(updates)))

	for _, update := range updates {
		if err := m.ApplyUpdate(ctx, projectID, docID, update); err != nil {
			return err
		}
	}
	return nil
}

// ApplyUpdate applies one update to a document.
//
// The order of what follows matters and is the Node service's: the operation is
// published to the other editors as soon as it has been applied in memory, and
// only then written back to Redis. A failure anywhere after the read tells the
// editors, which is what makes them reload rather than carry on from a state
// the server rejected.
func (m *UpdateManager) ApplyUpdate(ctx context.Context, projectID, docID string, update *Update) error {
	if err := sanitizeUpdate(update); err != nil {
		return err
	}

	err := m.applyUpdateInner(ctx, projectID, docID, update)
	if err != nil {
		m.realtime.SendError(ctx, projectID, docID, err)
	}
	return err
}

func (m *UpdateManager) applyUpdateInner(ctx context.Context, projectID, docID string, update *Update) error {
	loaded, err := m.docs.GetDoc(ctx, projectID, docID)
	if err != nil {
		return err
	}
	if !loaded.Loaded() {
		return fmt.Errorf("%w: document not found: %s", ErrNotFound, docID)
	}
	if loaded.Type() != TypeShareJSTextOT {
		return &OTTypeMismatchError{Got: loaded.Type(), Want: TypeShareJSTextOT}
	}
	applied, err := m.applyUpdate(ctx, projectID, docID, update, loaded.Lines, loaded.Version)
	if err != nil {
		return err
	}
	if applied.Duplicate {
		// The client resent an operation already applied. It is acknowledged so
		// the client stops resending, and not broadcast to anybody else.
		update.Dup = true
		m.realtime.SendAppliedOp(ctx, projectID, docID, update)
		m.realtime.SendCanaryAppliedOp(ctx, projectID, docID, update)
		return nil
	}

	m.realtime.SendAppliedOp(ctx, projectID, docID, applied.Applied)
	m.realtime.SendCanaryAppliedOp(ctx, projectID, docID, applied.Applied)

	// The tracked changes and comments move with the edit. This happens before
	// the document is written back, because it can refuse the edit: markers
	// that no longer match the text mean the arithmetic went wrong, and storing
	// the result would make that permanent.
	var appliedOp textot.Op
	if err := json.Unmarshal(applied.Applied.Op, &appliedOp); err != nil {
		return err
	}
	ranges, err := applyUpdateToRanges(loaded.Ranges, appliedOp, applied.Lines,
		metaString(update.Meta, "user_id"), metaString(update.Meta, "tc"))
	if err != nil {
		return err
	}

	appliedOps := []json.RawMessage{}
	encoded, err := json.Marshal(applied.Applied)
	if err != nil {
		return err
	}
	appliedOps = append(appliedOps, encoded)

	if err := m.redis.UpdateDocument(ctx, projectID, docID, applied.Lines,
		applied.Version, appliedOps, ranges.NewRanges,
		metaString(update.Meta, "user_id")); err != nil {
		return err
	}

	// An edit that removed a tracked change rejected it, and whoever made that
	// change is told. It is not waited on: the document lock is held here, and
	// the answer does not affect the edit.
	if len(ranges.RemovedChangeIDs) > 0 {
		authors := changeAuthors(loaded.Ranges, ranges.RemovedChangeIDs)
		go m.notifyRejected(context.WithoutCancel(ctx), projectID, docID, authors,
			metaString(update.Meta, "user_id"))
	}

	if ranges.Collapsed && m.snapshots != nil {
		// A marker was emptied or lost, and there is nothing left in the
		// document to recover it from, so the document as it was is kept.
		m.log.Debug("update collapsed some ranges, snapshotting previous content",
			slog.String("project", projectID), slog.String("doc", docID),
			slog.Int64("previousVersion", loaded.Version))

		// Last, because it is the one call here that leaves Redis. Overrunning
		// the lock does not matter by this point: everything else is done.
		if err := m.snapshots.RecordSnapshot(ctx, projectID, docID, loaded.Version,
			loaded.Pathname, loaded.Lines, loaded.Ranges); err != nil {
			return err
		}
	}

	// project-history gets the same operation with the metadata it needs to
	// place it: which file it was, and how long the document was before it.
	// Only the operations that changed text reach it, so a comment-only update
	// sends nothing.
	if len(ranges.HistoryOps) == 0 {
		return m.recordNotificationTimestamp(ctx, projectID, update)
	}
	historyUpdate, err := m.historyUpdate(applied.Applied, ranges.HistoryOps, loaded, projectID)
	if err != nil {
		return err
	}
	queueLength, err := m.history.QueueOps(ctx, projectID, historyUpdate)
	if err != nil {
		// The history queue failing does not fail the edit: project-history can
		// resync a project that has fallen behind, and refusing the write would
		// lose the user's typing instead.
		m.log.Error("failed to queue history op", slog.String("project", projectID),
			slog.String("doc", docID), slog.String("err", err.Error()))
	} else {
		m.log.Debug("queued history op", slog.String("project", projectID),
			slog.Int64("queueLength", queueLength))
	}

	return m.recordNotificationTimestamp(ctx, projectID, update)
}

// recordNotificationTimestamp notes when a project first changed, for the email
// that tells collaborators about it.
func (m *UpdateManager) recordNotificationTimestamp(ctx context.Context, projectID string, update *Update) error {
	timestamp := nowMillis()
	if ts := metaInt(update.Meta, "ts"); ts != 0 {
		timestamp = ts
	}
	return m.redis.RecordProjectNotificationTimestamp(ctx, projectID, timestamp)
}

// notifyRejected tells web which authors had a tracked change rejected.
func (m *UpdateManager) notifyRejected(ctx context.Context, projectID, docID string, authors []string, userID string) {
	if m.web == nil {
		return
	}
	if err := m.web.NotifyTrackChangesRejected(ctx, projectID, docID, authors, userID); err != nil {
		m.log.Warn("failed to notify web of rejected track changes",
			slog.String("project", projectID), slog.String("doc", docID),
			slog.String("err", err.Error()))
	}
}

// changeAuthors reads the authors of the given tracked changes out of the
// markers as they were before the update, since they are gone from the ones
// after it.
func changeAuthors(ranges json.RawMessage, removedIDs []string) []string {
	changes, _, err := decodeRanges(ranges)
	if err != nil {
		return nil
	}
	wanted := make(map[string]bool, len(removedIDs))
	for _, id := range removedIDs {
		wanted[id] = true
	}
	var authors []string
	for _, change := range changes {
		if wanted[change.ID] {
			authors = append(authors, change.Metadata.UserID())
		}
	}
	return authors
}

// historyUpdate builds the copy of an update that goes to project-history.
func (m *UpdateManager) historyUpdate(applied *Update, historyOps textot.Op, doc *LoadedDoc, projectID string) (json.RawMessage, error) {
	// Rendered through a map so the fields this service does not model are
	// carried across untouched.
	encoded, err := json.Marshal(applied)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}

	fields["projectHistoryId"], _ = json.Marshal(doc.ProjectHistoryID)
	fields["op"], err = json.Marshal(historyOps)
	if err != nil {
		return nil, err
	}

	meta := map[string]json.RawMessage{}
	if len(applied.Meta) > 0 {
		_ = json.Unmarshal(applied.Meta, &meta)
	}
	meta["pathname"], _ = json.Marshal(doc.Pathname)
	meta["doc_length"], _ = json.Marshal(docLength(doc.Lines))
	if !doc.HistoryRangesSupport {
		// Without history ranges support, project-history must not try to
		// interpret tracked changes.
		delete(meta, "tc")
	}
	fields["meta"], _ = json.Marshal(meta)

	return json.Marshal(fields)
}

// docLength is the length of the document as history measures it: every line,
// plus the newlines that join them.
//
// The newline count is one fewer than the number of lines, because the last
// line does not end with one -- and zero for an empty document, which would
// otherwise come out as minus one. This is Utils.getDocLength, not the
// getTotalSizeOfLines used for the size limit, which counts a newline after
// every line including the last.
//
// Lengths are in UTF-16 code units, because that is what a JavaScript string
// length is and history places its operations by these numbers.
func docLength(lines []string) int {
	total := 0
	for _, line := range lines {
		total += textot.T(line).Len()
	}
	return total + max(len(lines)-1, 0)
}

// metaInt reads a numeric metadata field.
func metaInt(meta json.RawMessage, field string) int64 {
	if len(meta) == 0 {
		return 0
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(meta, &fields); err != nil {
		return 0
	}
	raw, ok := fields[field]
	if !ok {
		return 0
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0
	}
	return value
}

func nowMillis() int64 { return time.Now().UnixMilli() }

// nowISO renders the current time the way JSON.stringify renders a Date, which
// is what project-history reads out of the entries this service queues.
func nowISO() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000Z") }

// HistoryQueue hands operations to project-history.
type HistoryQueue struct {
	redis *redis.Client
	keys  rediskeys.Schema
}

// NewHistoryQueue builds the queue.
func NewHistoryQueue(client *redis.Client, keys rediskeys.Schema) *HistoryQueue {
	return &HistoryQueue{redis: client, keys: keys}
}

// QueueOps pushes operations onto a project's history queue and returns how
// long the queue is.
func (q *HistoryQueue) QueueOps(ctx context.Context, projectID string, ops ...json.RawMessage) (int64, error) {
	if len(ops) == 0 {
		return 0, nil
	}
	values := make([]any, len(ops))
	for i, op := range ops {
		values[i] = string(op)
	}

	pipe := q.redis.TxPipeline()
	length := pipe.RPush(ctx, q.keys.ProjectHistoryOps(projectID), values...)
	// The timestamp records when the oldest unprocessed operation was queued,
	// which is how a history backlog is measured. Only the first one sets it.
	pipe.SetNX(ctx, q.keys.ProjectHistoryFirstOpTimestamp(projectID), nowMillis(), 0)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return length.Val(), nil
}
