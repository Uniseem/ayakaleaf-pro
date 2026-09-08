package projecthistory

import (
	"context"
	"math/rand/v2"
	"time"
)

// Nothing flushes a project the moment it is edited. Updates sit on the queue
// until something asks for them, and what usually asks is this: a sweep over
// every project with a queue, flushing the ones whose oldest update has been
// waiting long enough.
//
// Waiting is the point. Somebody typing a paragraph produces a few hundred
// updates, and flushing each one would write a few hundred entries into the
// history where one belongs. So the queue is left to gather, and the record of
// when it started gathering is what says it is time.

// FlushManager flushes the queues that have been waiting.
type FlushManager struct {
	redis     *RedisStore
	store     *Store
	processor *Processor
	// shortQueueProjects are flushed whenever they are looked at, whatever
	// their queue says. It is how a project being watched is kept current.
	shortQueueProjects map[string]bool
}

// NewFlushManager builds it.
func NewFlushManager(redis *RedisStore, store *Store, processor *Processor,
	shortQueueProjects []string) *FlushManager {

	short := map[string]bool{}
	for _, projectID := range shortQueueProjects {
		if projectID != "" {
			short[projectID] = true
		}
	}
	return &FlushManager{
		redis: redis, store: store, processor: processor,
		shortQueueProjects: short,
	}
}

// FlushIfOld flushes a project whose queue has been waiting since before the
// cutoff.
//
// A queue with no record of when it started waiting is flushed too. The record
// is written by document-updater when it puts the first update on, and one
// that is missing means something went wrong rather than that the queue is
// new: leaving it would leave the updates there for ever.
func (m *FlushManager) FlushIfOld(ctx context.Context, projectID string,
	cutoff time.Time) (bool, error) {

	firstOp, err := m.redis.GetFirstOpTimestamp(ctx, projectID)
	if err != nil {
		return false, err
	}

	if firstOp.IsZero() || firstOp.Before(cutoff) {
		return true, m.processor.ProcessUpdatesForProject(ctx, projectID)
	}
	if m.shortQueueProjects[projectID] {
		return true, m.processor.ProcessUpdatesForProject(ctx, projectID)
	}
	return false, nil
}

// FlushOldOptions bound a sweep.
type FlushOldOptions struct {
	// MaxAge is how long a queue may wait before it is flushed.
	MaxAge time.Duration
	// Timeout stops the sweep once it has been going this long.
	Timeout time.Duration
	// Limit stops it after this many projects.
	Limit int
	// QueueDelay is the pause between projects, which keeps a sweep from
	// taking the whole service for itself.
	QueueDelay time.Duration
}

// FlushOldResult is which projects were swept and how it went.
type FlushOldResult struct {
	Success []string `json:"success"`
	Failure []string `json:"failure"`
	// FailedProjects are the ones that were skipped because they are already
	// recorded as failing; retrying those is the retry manager's job.
	FailedProjects []string `json:"failedProjects"`
}

// FlushOldOps flushes every project whose queue has been waiting.
func (m *FlushManager) FlushOldOps(ctx context.Context,
	options FlushOldOptions) (*FlushOldResult, error) {

	if options.MaxAge == 0 {
		options.MaxAge = 6 * time.Hour
	}
	if options.QueueDelay == 0 {
		options.QueueDelay = 100 * time.Millisecond
	}

	projectIDs, err := m.redis.GetProjectIDsWithHistoryOps(ctx, 0)
	if err != nil {
		return nil, err
	}

	failures, err := m.store.GetFailures(ctx)
	if err != nil {
		return nil, err
	}
	failing := map[string]bool{}
	failedProjects := make([]string, 0, len(failures))
	for _, failure := range failures {
		failing[failure.ProjectID] = true
		failedProjects = append(failedProjects, failure.ProjectID)
	}

	// Shuffled, so that a limit does not mean the same projects are swept
	// every time and the ones after them never are.
	rand.Shuffle(len(projectIDs), func(i, j int) {
		projectIDs[i], projectIDs[j] = projectIDs[j], projectIDs[i]
	})

	cutoff := time.Now().Add(-options.MaxAge)
	started := time.Now()
	result := &FlushOldResult{
		Success: []string{}, Failure: []string{}, FailedProjects: failedProjects,
	}

	for count, projectID := range projectIDs {
		if options.Timeout > 0 && time.Since(started) > options.Timeout {
			break
		}
		if options.Limit > 0 && count >= options.Limit {
			break
		}

		if failing[projectID] {
			// Already recorded as failing, so flushing it again here would
			// only record the same failure.
			result.Success = append(result.Success, projectID)
		} else if _, err := m.FlushIfOld(ctx, projectID, cutoff); err != nil {
			result.Failure = append(result.Failure, projectID)
		} else {
			result.Success = append(result.Success, projectID)
		}

		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(options.QueueDelay):
		}
	}
	return result, nil
}
