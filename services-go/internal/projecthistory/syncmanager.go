package projecthistory

import (
	"context"
	"encoding/json"
	"fmt"
)

// SyncManager starts and runs resyncs.
type SyncManager struct {
	store     *Store
	redis     *RedisStore
	web       *WebAPI
	history   *HistoryStore
	snapshots *Snapshots
	locks     *LockManager
}

// NewSyncManager builds it.
func NewSyncManager(store *Store, redis *RedisStore, web *WebAPI,
	history *HistoryStore, snapshots *Snapshots, locks *LockManager) *SyncManager {

	return &SyncManager{
		store: store, redis: redis, web: web, history: history,
		snapshots: snapshots, locks: locks,
	}
}

// StartResyncOptions are the things a resync can be asked to do.
type StartResyncOptions struct {
	// Origin is what the changes the resync makes are recorded as. It defaults
	// to a resync of the history.
	Origin json.RawMessage
	// Hard says to rebuild the project rather than correct it.
	Hard bool
	// RecoverCorruptedFiles allows a file the history cannot read to be
	// replaced with the editor's copy.
	RecoverCorruptedFiles bool

	HistoryRangesMigration     string
	ResyncProjectStructureOnly bool
}

// defaultResyncOrigin is what a resync's changes are recorded as when nothing
// else says.
var defaultResyncOrigin = json.RawMessage(`{"kind":"history-resync"}`)

// StartResync asks the editor to send the project again.
//
// It runs under the project's lock, because the state has to be written and
// the request made without another flush getting in between: a flush that saw
// the resync updates before the state said a resync had started would refuse
// them.
func (m *SyncManager) StartResync(ctx context.Context, projectID string,
	options StartResyncOptions) error {

	err := m.locks.RunWithLock(ctx, Keys{}.ProjectHistoryLock(projectID),
		func(ctx context.Context, lock *Lock) error {
			return m.StartResyncWithoutLock(ctx, projectID, options)
		})
	if err != nil {
		// Recorded so that a project whose resync will not start shows up
		// among the failures rather than only in the logs.
		_ = m.store.RecordFailure(ctx, projectID, -1, err)
		return err
	}
	return nil
}

// StartHardResync throws away what is queued and rebuilds the project.
func (m *SyncManager) StartHardResync(ctx context.Context, projectID string,
	options StartResyncOptions) error {

	options.Hard = true
	err := m.locks.RunWithLock(ctx, Keys{}.ProjectHistoryLock(projectID),
		func(ctx context.Context, lock *Lock) error {
			if err := m.store.ClearSyncState(ctx, projectID); err != nil {
				return err
			}
			if err := m.redis.ClearFirstOpTimestamp(ctx, projectID); err != nil {
				return err
			}
			if err := m.redis.DestroyQueue(ctx, projectID); err != nil {
				return err
			}
			return m.StartResyncWithoutLock(ctx, projectID, options)
		})
	if err != nil {
		_ = m.store.RecordFailure(ctx, projectID, -1, err)
		return err
	}
	return nil
}

// StartResyncWithoutLock is the body of a resync, for a caller that already
// holds the lock.
func (m *SyncManager) StartResyncWithoutLock(ctx context.Context, projectID string,
	options StartResyncOptions) error {

	if err := m.store.RecordSyncStart(ctx, projectID); err != nil {
		return err
	}

	state, err := m.store.GetSyncStateFor(ctx, projectID)
	if err != nil {
		return err
	}

	if state.IsSyncOngoing() {
		if !state.IsSyncStuck() {
			return ErrSyncOngoing
		}
		// A resync that has been pending for hours is not going to finish, so
		// the state is cleared and it is started again -- but only so many
		// times, because a project that gets stuck every time needs looking at
		// rather than restarting.
		if state.StuckClearCount >= maxStuckClearAttempts {
			if err := m.store.RecordStuckClear(ctx, projectID,
				state.ResyncDocContents); err != nil {
				return err
			}
			return ErrSyncPermanentlyStuck
		}
		if err := m.store.RecordStuckClear(ctx, projectID,
			state.ResyncDocContents); err != nil {
			return err
		}
	}

	origin := options.Origin
	if len(origin) == 0 || string(origin) == "null" {
		origin = defaultResyncOrigin
	}
	state.Origin = origin
	state.StartProjectStructureSync()
	state.HardResync = options.Hard
	state.RecoverCorruptedFiles = options.RecoverCorruptedFiles

	err = m.web.RequestResync(ctx, projectID, ResyncOptions{
		HistoryRangesMigration:     options.HistoryRangesMigration,
		ResyncProjectStructureOnly: options.ResyncProjectStructureOnly,
	})
	if err != nil {
		return err
	}
	return m.store.WriteSyncState(ctx, state)
}

// SetResyncState writes a sync state, doing nothing when there is none to
// write: an unchanged state is passed along as nothing at all.
func (m *SyncManager) SetResyncState(ctx context.Context, projectID string,
	state *SyncState) error {

	if state == nil {
		return nil
	}
	return m.store.WriteSyncState(ctx, state)
}

// SkipUpdatesDuringSync drops the updates that would edit a part of the
// project that is being rebuilt, and returns the state as the sync updates
// among them leave it.
//
// The state comes back as nil when no resync is running, which is how the
// caller is told there is nothing to write.
func (m *SyncManager) SkipUpdatesDuringSync(ctx context.Context, projectID string,
	updates []*Update) ([]*Update, *SyncState, error) {

	state, err := m.store.GetSyncStateFor(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}
	if !state.IsSyncOngoing() {
		return updates, nil, nil
	}

	filtered := make([]*Update, 0, len(updates))
	for _, update := range updates {
		if err := state.UpdateState(update); err != nil {
			return nil, nil, err
		}
		if !state.ShouldSkipUpdate(update) {
			filtered = append(filtered, update)
		}
	}
	return filtered, state, nil
}

// ExpandSyncUpdates turns the project the editor sent into the changes the
// history would need to say the same thing.
//
// Updates that are not part of a resync pass through untouched, and a batch
// with no resync updates in it is not looked at at all: working out what the
// project currently is would mean fetching every file.
func (m *SyncManager) ExpandSyncUpdates(ctx context.Context, projectID,
	historyID string, chunk *ChunkResponse, updates []*Update,
	extendLock func(context.Context) error) ([]*Update, error) {

	queued := false
	for _, update := range updates {
		if hasField(update, "resyncProjectStructure") ||
			hasField(update, "resyncDocContent") {
			queued = true
			break
		}
	}
	if !queued {
		return updates, nil
	}

	state, err := m.store.GetSyncStateFor(ctx, projectID)
	if err != nil {
		return nil, err
	}

	files, err := m.snapshots.LatestSnapshotFilesForChunk(ctx, historyID, chunk)
	if err != nil {
		return nil, err
	}
	for _, path := range files.Paths() {
		if files.GetFile(path) == nil {
			return nil, fmt.Errorf("%w: file is missing: %s", ErrSync, path)
		}
	}

	expander := NewSyncUpdateExpander(projectID, files, state.Origin,
		state.HardResync, state.RecoverCorruptedFiles,
		m.history, m.web, m.redis, m.store)

	for _, update := range updates {
		if err := expander.ExpandUpdate(ctx, update); err != nil {
			return nil, err
		}
		// Between files, because working out what one large document has to
		// become can take longer than the lock is held for.
		if err := extendLock(ctx); err != nil {
			return nil, err
		}
	}
	return expander.ExpandedUpdates(), nil
}

// CloneResyncState copies a project's sync state onto another project.
func (m *SyncManager) CloneResyncState(ctx context.Context, from, to string) error {
	return m.store.CloneSyncState(ctx, from, to)
}

// GetResyncState reads a project's sync state.
func (m *SyncManager) GetResyncState(ctx context.Context, projectID string) (*SyncState, error) {
	return m.store.GetSyncStateFor(ctx, projectID)
}

// ClearResyncState forgets it.
func (m *SyncManager) ClearResyncState(ctx context.Context, projectID string) error {
	return m.store.ClearSyncState(ctx, projectID)
}
