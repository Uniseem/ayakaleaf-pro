package projecthistory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// This is what the service actually does. Everything else is a step in it.
//
// Updates arrive on a Redis queue, written there by document-updater and by
// web. A flush takes the project's lock, reads the queue in batches, and for
// each batch: drops what a running resync says to drop, turns any resync
// updates into the changes they imply, discards what the history has already
// been told, merges what can be merged, stores the contents of any new files,
// turns the rest into history changes, and sends them.
//
// The lock is what makes any of that safe. Two flushes of the same project at
// once would each read the queue, each decide the history is at version N, and
// each send changes based on that; the second would be refused, and the queue
// entries it had already taken off would be gone.

// redisReadBatchSize is how many updates are read at once.
const redisReadBatchSize = 500

// Processor flushes projects.
type Processor struct {
	redis     *RedisStore
	store     *Store
	web       *WebAPI
	history   *HistoryStore
	sync      *SyncManager
	locks     *LockManager
	snapshots *Snapshots
	metrics   ProcessorMetrics
}

// ProcessorMetrics is what a flush reports about itself.
type ProcessorMetrics interface {
	// FlushFinished is called once per flush that had anything to do.
	FlushFinished(duration time.Duration, queueSize int)
	// UpdatesDiscarded counts updates the history had already been told about.
	UpdatesDiscarded(reason string)
}

// NewProcessor builds the processor.
func NewProcessor(redis *RedisStore, store *Store, web *WebAPI,
	history *HistoryStore, sync *SyncManager, locks *LockManager,
	snapshots *Snapshots, metrics ProcessorMetrics) *Processor {

	if metrics == nil {
		metrics = noProcessorMetrics{}
	}
	return &Processor{
		redis: redis, store: store, web: web, history: history,
		sync: sync, locks: locks, snapshots: snapshots, metrics: metrics,
	}
}

// noProcessorMetrics reports nothing, for a caller that does not care.
type noProcessorMetrics struct{}

func (noProcessorMetrics) FlushFinished(time.Duration, int) {}
func (noProcessorMetrics) UpdatesDiscarded(string)          {}

// flushResult is what a flush found and did.
type flushResult struct {
	// QueueSize is how many updates were waiting, which is recorded against a
	// failure so that a project falling behind can be told from one that
	// merely failed once.
	QueueSize int
	// ResyncNeeded is set when the history store asked for the project to be
	// sent again.
	ResyncNeeded bool
}

// ProcessUpdatesForProject flushes a project's queue.
func (p *Processor) ProcessUpdatesForProject(ctx context.Context, projectID string) error {
	started := time.Now()

	result, flushErr := p.underLock(ctx, projectID, redisReadBatchSize)
	p.afterFlush(ctx, projectID, started, result)

	if flushErr != nil {
		failure, recordErr := p.recordFailure(ctx, projectID, result, flushErr)
		if recordErr != nil {
			return recordErr
		}
		// The first failure after a run of successes, of a kind that will not
		// clear itself, is worth a resync straight away: the alternative is
		// waiting for the retry that would only fail the same way.
		if failure != nil && isFirstFailure(failure) && isHardFailure(failure) {
			return p.ResyncProject(ctx, projectID)
		}
		return flushErr
	}

	if err := p.store.ClearFailure(ctx, projectID); err != nil {
		return err
	}
	if result.ResyncNeeded {
		return p.ResyncProject(ctx, projectID)
	}
	return nil
}

// StartResyncAndProcessUpdatesUnderLock asks for a resync and flushes what it
// produces without letting go of the lock in between.
//
// Letting go would let an ordinary flush run first, and it would refuse the
// resync updates because the state it read said no resync had started.
func (p *Processor) StartResyncAndProcessUpdatesUnderLock(ctx context.Context,
	projectID string, options StartResyncOptions) error {

	started := time.Now()
	result := &flushResult{}

	flushErr := p.locks.RunWithLock(ctx, Keys{}.ProjectHistoryLock(projectID),
		func(ctx context.Context, lock *Lock) error {
			if err := p.sync.StartResyncWithoutLock(ctx, projectID, options); err != nil {
				return err
			}
			if err := lock.Extend(ctx); err != nil {
				return err
			}
			var err error
			result, err = p.countAndProcessUpdates(ctx, projectID, lock,
				redisReadBatchSize)
			return err
		})

	p.afterFlush(ctx, projectID, started, result)
	if flushErr != nil {
		if _, err := p.recordFailure(ctx, projectID, result, flushErr); err != nil {
			return err
		}
		return flushErr
	}
	return p.store.ClearFailure(ctx, projectID)
}

// ResyncProject rebuilds a project and flushes what that produces.
func (p *Processor) ResyncProject(ctx context.Context, projectID string) error {
	if err := p.sync.StartHardResync(ctx, projectID, StartResyncOptions{}); err != nil {
		return err
	}

	// This cannot loop: whatever the flush does now, it will not be the first
	// failure any more, so it will not come back here.
	result, flushErr := p.underLock(ctx, projectID, redisReadBatchSize)
	if flushErr != nil {
		if _, err := p.recordFailure(ctx, projectID, result, flushErr); err != nil {
			return err
		}
		return flushErr
	}
	return p.store.ClearFailure(ctx, projectID)
}

// ProcessUpdatesForProjectUsingBisect flushes a project a few updates at a
// time, halving the batch each time one fails.
//
// It is how a queue with one bad update in it is got through by hand: the
// batches either side of the bad one go, and what is left is small enough to
// look at.
func (p *Processor) ProcessUpdatesForProjectUsingBisect(ctx context.Context,
	projectID string, amountToProcess int) error {

	for {
		result, flushErr := p.underLock(ctx, projectID, amountToProcess)

		if amountToProcess == 0 || result.QueueSize == 0 {
			// Nothing further can be done: either the batch cannot be made
			// smaller or there is nothing left.
			if flushErr != nil {
				if _, err := p.recordFailure(ctx, projectID, result, flushErr); err != nil {
					return err
				}
				return flushErr
			}
			return nil
		}
		if flushErr != nil {
			amountToProcess /= 2
		}
	}
}

// ProcessSingleUpdateForProject flushes exactly one update, for stepping
// through a queue.
func (p *Processor) ProcessSingleUpdateForProject(ctx context.Context,
	projectID string) error {

	result, flushErr := p.underLock(ctx, projectID, 1)
	if flushErr != nil {
		if _, err := p.recordFailure(ctx, projectID, result, flushErr); err != nil {
			return err
		}
		return flushErr
	}
	// The record of when the queue started waiting is left alone: stepping
	// does not mean the queue is finished, and the next background flush will
	// clear it if it is.
	return p.store.ClearFailure(ctx, projectID)
}

// underLock runs a flush holding the project's lock.
func (p *Processor) underLock(ctx context.Context, projectID string,
	batchSize int) (*flushResult, error) {

	result := &flushResult{}
	err := p.locks.RunWithLock(ctx, Keys{}.ProjectHistoryLock(projectID),
		func(ctx context.Context, lock *Lock) error {
			var err error
			result, err = p.countAndProcessUpdates(ctx, projectID, lock, batchSize)
			return err
		})
	return result, err
}

// afterFlush records how long a flush took and tidies up after it.
func (p *Processor) afterFlush(ctx context.Context, projectID string,
	started time.Time, result *flushResult) {

	if result != nil && result.QueueSize > 0 {
		p.metrics.FlushFinished(time.Since(started), result.QueueSize)
	}
	// The record of when the queue started waiting is cleared if the queue is
	// now empty. It is best effort: a stale record only means the project is
	// flushed sooner than it needs to be.
	_, _ = p.redis.ClearDanglingFirstOpTimestamp(context.WithoutCancel(ctx), projectID)
}

// recordFailure stores what a flush failed with, and returns the record.
func (p *Processor) recordFailure(ctx context.Context, projectID string,
	result *flushResult, cause error) (*Failure, error) {

	queueSize := 0
	if result != nil {
		queueSize = result.QueueSize
	}
	if err := p.store.RecordFailure(ctx, projectID, queueSize, cause); err != nil {
		return nil, err
	}
	return p.store.GetFailure(ctx, projectID)
}

// countAndProcessUpdates flushes a project's queue in batches.
//
// The queue size is returned even when a batch fails, because it is recorded
// against the failure: how far behind a project is says more about it than the
// failure alone.
func (p *Processor) countAndProcessUpdates(ctx context.Context, projectID string,
	lock *Lock, batchSize int) (*flushResult, error) {

	queueSize, err := p.redis.CountUnprocessedUpdates(ctx, projectID)
	if err != nil {
		return &flushResult{}, err
	}
	result := &flushResult{QueueSize: int(queueSize)}
	if queueSize == 0 {
		return result, nil
	}

	err = p.updatesInBatches(ctx, projectID, batchSize,
		func(updates []*Update) error {
			response, err := p.processUpdatesBatch(ctx, projectID, updates, lock)
			if err != nil {
				return err
			}
			if response.ResyncNeeded {
				result.ResyncNeeded = true
			}
			return nil
		})
	return result, err
}

// updatesInBatches reads the queue a batch at a time and hands each batch to
// the runner, taking each batch off the queue once it has been dealt with.
//
// A batch is capped three ways: by how many updates it holds, by how many
// operations they carry between them, and by how many whole documents. The
// last two matter because one update can be a keystroke or an entire file.
func (p *Processor) updatesInBatches(ctx context.Context, projectID string,
	batchSize int, runner func([]*Update) error) error {

	for moreBatches := true; moreBatches; {
		batch, err := p.redis.GetRawUpdatesBatch(ctx, projectID, batchSize)
		if err != nil {
			return err
		}
		if len(batch.Raw) == 0 {
			return nil
		}
		moreBatches = batch.HasMore

		var raws []string
		var updates []*Update
		var totalOpLength, totalDocContentCount int

		for _, raw := range batch.Raw {
			var update Update
			if err := json.Unmarshal([]byte(raw), &update); err != nil {
				return fmt.Errorf("failed to parse update: %w", err)
			}

			ops := len(update.Op)
			if ops == 0 {
				// An update that is not an edit still counts for something,
				// or a queue of them would never be capped.
				ops = 1
			}
			totalOpLength += ops
			if hasField(&update, "resyncDocContent") {
				totalDocContentCount++
			}

			if len(updates) > 0 && (totalOpLength > maxUpdateOpLength ||
				totalDocContentCount > maxNewDocContentCount) {
				moreBatches = true
				break
			}
			if isTrue(update.Rest["resyncProjectStructureOnly"]) {
				// This is the one update that may have to be put back, and
				// taking it off again is done by value.
				update.Raw = raw
			}

			raws = append(raws, raw)
			updates = append(updates, &update)
		}

		if err := runner(updates); err != nil {
			return err
		}
		if err := p.redis.DeleteAppliedUpdates(ctx, projectID, raws); err != nil {
			return err
		}

		if batchSize == 1 {
			// Stepping through one update at a time: stop rather than going
			// on to the next.
			return nil
		}
	}
	return nil
}

// processUpdatesBatch deals with one batch of updates.
func (p *Processor) processUpdatesBatch(ctx context.Context, projectID string,
	updates []*Update, lock *Lock) (*flushResult, error) {

	historyID, err := p.historyIDFor(ctx, projectID, updates)
	if err != nil {
		return nil, err
	}
	if historyID == "" {
		// The project has no history, so there is nothing to write these to.
		// They are still taken off the queue: keeping them would mean the
		// queue grew for ever.
		return &flushResult{}, nil
	}
	return p.processUpdates(ctx, projectID, historyID, updates, lock)
}

// historyIDFor works out which history a batch of updates belongs to.
//
// The updates may say, and web may say, and they have to agree: a batch
// written against one history and applied to another would put changes into
// somebody else's project.
func (p *Processor) historyIDFor(ctx context.Context, projectID string,
	updates []*Update) (string, error) {

	idFromUpdates := ""
	for _, update := range updates {
		id := rawString(update.Rest["projectHistoryId"])
		if id == "" {
			// A number rather than a string, for a project whose history
			// predates the current store.
			id = jsonScalarString(update.Rest["projectHistoryId"])
		}
		if id == "" {
			continue
		}
		if idFromUpdates == "" {
			idFromUpdates = id
			continue
		}
		if idFromUpdates != id {
			return "", fmt.Errorf(
				"inconsistent project history id between updates: %s and %s",
				idFromUpdates, id)
		}
	}

	idFromWeb, err := p.web.GetHistoryID(ctx, projectID)
	if err != nil {
		if idFromUpdates != "" {
			// Web could not be asked, but the updates say which history this
			// is, and they were written by something that knew.
			return idFromUpdates, nil
		}
		return "", err
	}

	switch {
	case idFromWeb == "" && idFromUpdates == "":
		return "", nil
	case idFromUpdates == "":
		return idFromWeb, nil
	case idFromWeb == "":
		return idFromUpdates, nil
	case idFromWeb != idFromUpdates:
		return "", errors.New("inconsistent project history id between updates and web")
	}
	return idFromWeb, nil
}

// processUpdates turns a batch of updates into changes and sends them.
func (p *Processor) processUpdates(ctx context.Context, projectID, historyID string,
	updates []*Update, lock *Lock) (*flushResult, error) {

	extendLock := func(ctx context.Context) error { return lock.Extend(ctx) }

	// A resync that is running says which of these to drop, and dropping them
	// is done first: a batch that is entirely dropped needs nothing else.
	filtered, newSyncState, err := p.sync.SkipUpdatesDuringSync(ctx, projectID, updates)
	if err != nil {
		return nil, err
	}
	if len(filtered) == 0 {
		if err := p.sync.SetResyncState(ctx, projectID, newSyncState); err != nil {
			return nil, err
		}
		return &flushResult{}, nil
	}

	// Only now is the history store asked anything, because until there is
	// something to write there is nothing to ask.
	recent, err := p.mostRecentVersion(ctx, projectID, historyID)
	if err != nil {
		return nil, err
	}

	expanded, err := p.sync.ExpandSyncUpdates(ctx, projectID, historyID,
		recent.Chunk, filtered, extendLock)
	if err != nil {
		return nil, err
	}

	unapplied, err := skipAlreadyAppliedUpdates(projectID, expanded,
		recent.Versions, p.metrics)
	if err != nil {
		return nil, err
	}

	compressed, err := CompressRawUpdates(unapplied)
	if err != nil {
		return nil, err
	}

	withBlobs, err := CreateBlobsForUpdates(ctx, p.history, projectID, historyID,
		compressed, extendLock)
	if err != nil {
		return nil, err
	}

	changes, err := ConvertToChanges(withBlobs)
	if err != nil {
		return nil, err
	}

	// Sending is usually the longest part, so the lock is extended in front of
	// it rather than after.
	if err := lock.Extend(ctx); err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		if err := p.sync.SetResyncState(ctx, projectID, newSyncState); err != nil {
			return nil, err
		}
		return &flushResult{}, nil
	}

	response, err := p.history.SendChanges(ctx, historyID, changes, recent.Version)
	if err != nil {
		return nil, err
	}
	if err := p.sync.SetResyncState(ctx, projectID, newSyncState); err != nil {
		return nil, err
	}
	return &flushResult{ResyncNeeded: response.ResyncNeeded}, nil
}

// mostRecentVersion reads where the history has got to.
//
// A history whose versions are out of order is an error, except for a project
// that has been marked for debugging: there, the versions are taken as they
// are so that somebody can see what the project does next.
func (p *Processor) mostRecentVersion(ctx context.Context, projectID,
	historyID string) (*MostRecentVersion, error) {

	recent, err := p.history.GetMostRecentVersion(ctx, projectID, historyID)
	if err == nil {
		return recent, nil
	}
	if !errors.Is(err, ErrOpsOutOfOrder) || recent == nil {
		return nil, err
	}

	forced, debugErr := p.store.ForceDebug(ctx, projectID)
	if debugErr != nil {
		return nil, debugErr
	}
	if !forced {
		return nil, err
	}
	return recent, nil
}

// skipAlreadyAppliedUpdates drops the updates the history has already been
// told about, and refuses a batch whose own versions run backwards.
//
// The two are different failures. A version the history already has is an
// update that was sent twice, which is ordinary and is dropped. A batch whose
// versions run backwards means the queue itself is wrong, and sending it would
// write a history that says the project went backwards.
func skipAlreadyAppliedUpdates(projectID string, updates []*Update,
	applied ProjectStructureAndDocVersions,
	metrics ProcessorMetrics) ([]*Update, error) {

	incomingProjectVersion := ""
	incomingDocVersions := map[string]int64{}
	for _, update := range updates {
		if seenProjectVersion(incomingProjectVersion, update) {
			return nil, fmt.Errorf(
				"%w: project structure version out of order on incoming updates",
				ErrOpsOutOfOrder)
		}
		if seenDocVersion(incomingDocVersions, update) {
			return nil, fmt.Errorf("%w: doc version out of order on incoming updates",
				ErrOpsOutOfOrder)
		}
		if isProjectStructureUpdate(update) {
			incomingProjectVersion = projectVersionOf(update)
		} else if update.IsTextUpdate() && update.V != nil {
			incomingDocVersions[update.Doc] = *update.V
		}
	}

	previousDocVersions := map[string]int64{}
	for docID, version := range applied.Docs {
		previousDocVersions[docID] = version.V
	}

	toApply := make([]*Update, 0, len(updates))
	for _, update := range updates {
		if seenProjectVersion(applied.Project, update) {
			metrics.UpdatesDiscarded("project_structure_version")
			continue
		}
		if seenDocVersion(previousDocVersions, update) {
			metrics.UpdatesDiscarded("doc_version")
			continue
		}
		// Characters outside the basic plane are taken out of resync updates,
		// which have not been through document-updater and so have not had it
		// done already. The history cannot hold them.
		sanitizeUpdate(update)
		toApply = append(toApply, update)
	}
	return toApply, nil
}

// seenProjectVersion reports whether the history is already at or past the
// version an update would put the project at.
func seenProjectVersion(applied string, update *Update) bool {
	if !isProjectStructureUpdate(update) || applied == "" {
		return false
	}
	version := projectVersionOf(update)
	if version == "" {
		return false
	}
	return compareVersions(applied, version) >= 0
}

// seenDocVersion reports the same for one document.
func seenDocVersion(applied map[string]int64, update *Update) bool {
	if !update.IsTextUpdate() || update.V == nil {
		return false
	}
	version, ok := applied[update.Doc]
	return ok && version >= *update.V
}

// isProjectStructureUpdate reports whether an update changes the file tree.
func isProjectStructureUpdate(update *Update) bool {
	return isAdd(update) || isRename(update)
}

// sanitizeUpdate takes the characters the history cannot hold out of an
// update.
//
// Anything outside the basic multilingual plane is replaced, and it is
// replaced by two characters rather than one because that is how many code
// units it took: the positions in the operations around it are counted in
// those, and shortening the text would move them.
func sanitizeUpdate(update *Update) {
	for i := range update.Op {
		if update.Op[i].Insert != nil {
			cleaned := removeNonBMP(*update.Op[i].Insert)
			update.Op[i].Insert = &cleaned
		}
	}
	if raw, ok := update.Rest["docLines"]; ok {
		var docLines string
		if err := json.Unmarshal(raw, &docLines); err == nil {
			update.setRest("docLines", removeNonBMP(docLines))
		}
	}
}

// removeNonBMP replaces every character outside the basic multilingual plane
// with the replacement character, twice over: one such character is two UTF-16
// code units, and every position in an update is counted in those.
func removeNonBMP(text string) string {
	needed := false
	for _, r := range text {
		if r > 0xFFFF {
			needed = true
			break
		}
	}
	if !needed {
		return text
	}

	var out strings.Builder
	out.Grow(len(text))
	for _, r := range text {
		if r > 0xFFFF {
			out.WriteRune('�')
			out.WriteRune('�')
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}
