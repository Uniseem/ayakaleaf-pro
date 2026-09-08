package projecthistory

import (
	"context"
	"fmt"
	"math/rand/v2"
	"regexp"
	"strings"
	"time"
)

// A project that fails to flush is not retried in place. It is recorded, and
// something comes back for it later -- which is this.
//
// What that something does depends on what went wrong. A failure that will
// clear itself, like a lock that timed out, wants nothing more than another
// flush. A failure that will not, like a history that refused the changes,
// wants the project sent again from the editor; and if that has already been
// tried and failed, it wants it sent again from nothing.

// temporaryFailures are the failures that go away by themselves.
var temporaryFailures = map[string]bool{
	"Error: ENOSPC: no space left on device, write": true,
	"Error: ESOCKETTIMEDOUT":                        true,
	"Error: failed to extend lock":                  true,
	"Error: tried to release timed out lock":        true,
	"Error: Timeout":                                true,
}

// hardFailures are the failures that another flush will not fix, because they
// say the history and the project no longer agree.
var hardFailures = map[string]bool{
	"Error: history store a non-success status code: 422":                            true,
	"OpsOutOfOrderError: project structure version out of order":                     true,
	"OpsOutOfOrderError: project structure version out of order on incoming updates": true,
	"OpsOutOfOrderError: doc version out of order":                                   true,
	"OpsOutOfOrderError: doc version out of order on incoming updates":               true,
}

// maxResyncAttempts is how many resyncs a project gets before it is left
// alone.
const maxResyncAttempts = 2

// maxSoftResyncAttempts is how many of those are soft -- correcting the
// project -- before the next one rebuilds it.
const maxSoftResyncAttempts = 1

// isTemporaryFailure reports whether a failure will clear itself.
func isTemporaryFailure(failure *Failure) bool {
	return temporaryFailures[failure.Error]
}

// isHardFailure reports whether a failure needs the project sent again.
func isHardFailure(failure *Failure) bool {
	return hardFailures[failure.Error]
}

// isFirstFailure reports whether this is the first failure since the project
// was last processed.
func isFirstFailure(failure *Failure) bool {
	return failure.Attempts <= 1
}

// isRepeatedFailure reports whether a project has failed enough times that
// flushing it again is not going to help.
func isRepeatedFailure(failure *Failure) bool {
	return failure.Attempts > 3
}

// isOngoingSyncFailure reports whether a project failed because a resync was
// already running.
func isOngoingSyncFailure(failure *Failure) bool {
	return strings.Contains(failure.Error, ErrSyncOngoing.Error())
}

// isStuckFailure reports whether a project has had all the resyncs it gets.
func isStuckFailure(failure *Failure) bool {
	return failure.ResyncAttempts >= maxResyncAttempts
}

// failureRequiresHardResync reports whether correcting the project has been
// tried and did not work.
func failureRequiresHardResync(failure *Failure) bool {
	return failure.ResyncAttempts >= maxSoftResyncAttempts
}

// softErrorSelector picks the projects worth simply flushing again.
func softErrorSelector(failure *Failure) bool {
	return (isTemporaryFailure(failure) && !isRepeatedFailure(failure)) ||
		(isFirstFailure(failure) && !isHardFailure(failure))
}

// hardErrorSelector picks the projects worth sending again.
func hardErrorSelector(failure *Failure) bool {
	if isOngoingSyncFailure(failure) {
		// A resync that is already running is retried through the soft path,
		// which keeps the sync state: the state is what lets a stuck resync be
		// recognised, and clearing it would hide that.
		return true
	}
	return (isHardFailure(failure) || isRepeatedFailure(failure)) &&
		!isStuckFailure(failure)
}

// RetryManager comes back to the projects that failed.
type RetryManager struct {
	store     *Store
	redis     *RedisStore
	web       *WebAPI
	sync      *SyncManager
	processor *Processor
	// queueWait is how long to wait for a project's queue to empty after a
	// resync has been asked for.
	queueWait time.Duration
}

// NewRetryManager builds it.
func NewRetryManager(store *Store, redis *RedisStore, web *WebAPI,
	sync *SyncManager, processor *Processor) *RetryManager {

	return &RetryManager{
		store: store, redis: redis, web: web, sync: sync, processor: processor,
		queueWait: time.Second,
	}
}

// RetryOptions bound how much retrying is done in one go.
type RetryOptions struct {
	// FailureType is "soft" for another flush or "hard" for a resync.
	FailureType string
	// Timeout stops the run once it has been going this long.
	Timeout time.Duration
	// Limit stops it after this many projects.
	Limit int
}

// RetryResult is which projects were retried and how it went.
type RetryResult struct {
	Succeeded []string `json:"succeeded"`
	Failed    []string `json:"failed"`
}

// RetryFailures comes back to the projects that failed.
func (m *RetryManager) RetryFailures(ctx context.Context,
	options RetryOptions) (*RetryResult, error) {

	switch options.FailureType {
	case "soft":
		batch, err := m.failureBatch(ctx, softErrorSelector, options.Limit)
		if err != nil {
			return nil, err
		}
		return m.retryBatch(ctx, batch, options.Timeout,
			func(ctx context.Context, failure *Failure) error {
				return m.processor.ProcessUpdatesForProject(ctx, failure.ProjectID.String())
			})

	case "hard":
		batch, err := m.failureBatch(ctx, hardErrorSelector, options.Limit)
		if err != nil {
			return nil, err
		}
		return m.retryBatch(ctx, batch, options.Timeout,
			func(ctx context.Context, failure *Failure) error {
				// A resync that is already running is always retried softly:
				// rebuilding would throw away the state that says it is stuck.
				hard := failureRequiresHardResync(failure) &&
					!isOngoingSyncFailure(failure)
				return m.resyncProject(ctx, failure.ProjectID.String(), hard)
			})
	}
	return &RetryResult{Succeeded: []string{}, Failed: []string{}}, nil
}

// failureBatch picks the projects to retry.
//
// They are shuffled, so that a limit does not mean the same projects are
// retried every time and the ones after them never are.
func (m *RetryManager) failureBatch(ctx context.Context,
	selector func(*Failure) bool, limit int) ([]Failure, error) {

	failures, err := m.store.GetFailures(ctx)
	if err != nil {
		return nil, err
	}

	var selected []Failure
	for i := range failures {
		if selector(&failures[i]) {
			selected = append(selected, failures[i])
		}
	}
	rand.Shuffle(len(selected), func(i, j int) {
		selected[i], selected[j] = selected[j], selected[i]
	})
	if limit > 0 && len(selected) > limit {
		selected = selected[:limit]
	}
	return selected, nil
}

// retryBatch retries each project in turn, stopping when it has been going too
// long.
func (m *RetryManager) retryBatch(ctx context.Context, failures []Failure,
	timeout time.Duration,
	retry func(context.Context, *Failure) error) (*RetryResult, error) {

	started := time.Now()
	result := &RetryResult{Succeeded: []string{}, Failed: []string{}}

	for i := range failures {
		if timeout > 0 && time.Since(started) > timeout {
			break
		}
		if err := retry(ctx, &failures[i]); err != nil {
			result.Failed = append(result.Failed, failures[i].ProjectID.String())
			continue
		}
		result.Succeeded = append(result.Succeeded, failures[i].ProjectID.String())
	}
	return result, nil
}

// objectIDPattern is what a project id looks like.
var objectIDPattern = regexp.MustCompile(`^[0-9a-f]{24}$`)

// resyncProject sends a project again and waits for it to come back.
func (m *RetryManager) resyncProject(ctx context.Context, projectID string,
	hard bool) error {

	if !objectIDPattern.MatchString(projectID) {
		// A failure recorded against something that is not a project id
		// cannot be retried, and keeping it would mean retrying it for ever.
		return m.store.ClearFailure(ctx, projectID)
	}

	historyID, err := m.web.GetHistoryID(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to resync project %s: %w", projectID, err)
	}
	if historyID == "" {
		return fmt.Errorf("failed to resync project %s: no history id", projectID)
	}

	if hard {
		err = m.sync.StartHardResync(ctx, projectID, StartResyncOptions{})
	} else {
		err = m.sync.StartResync(ctx, projectID, StartResyncOptions{})
	}
	if err != nil {
		return fmt.Errorf("failed to resync project %s: %w", projectID, err)
	}

	if err := m.waitForEmptyQueue(ctx, projectID); err != nil {
		return fmt.Errorf("failed to resync project %s: %w", projectID, err)
	}

	// The record is only gone once the project has been flushed successfully,
	// so its absence is what says the resync worked.
	failure, err := m.store.GetFailure(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to resync project %s: %w", projectID, err)
	}
	if failure != nil {
		return fmt.Errorf("failed to resync project %s: failure record still exists",
			projectID)
	}
	return nil
}

// waitForEmptyQueue waits for a project's queue to be flushed.
func (m *RetryManager) waitForEmptyQueue(ctx context.Context, projectID string) error {
	for attempt := 0; attempt < 30; attempt++ {
		count, err := m.redis.CountUnprocessedUpdates(ctx, projectID)
		if err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(m.queueWait):
		}
	}
	return fmt.Errorf("queue not empty")
}
