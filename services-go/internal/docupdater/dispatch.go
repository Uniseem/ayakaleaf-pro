package docupdater

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/textot"
	"github.com/redis/go-redis/v9"
)

// Dispatcher watches the queues real-time pushes documents onto and applies
// what is waiting for them.
//
// There is one worker per shard, and real-time picks a shard at random for each
// edit, so the shard count here must match the one it uses. A worker missing
// for a shard means the edits sent to it are never applied: nothing fails, the
// document simply stops updating.
type Dispatcher struct {
	redis   *redis.Client
	updates *UpdateManager
	log     *slog.Logger
	count   int
}

// NewDispatcher builds the dispatcher.
func NewDispatcher(client *redis.Client, updates *UpdateManager, count int, log *slog.Logger) *Dispatcher {
	if count < 1 {
		count = 1
	}
	return &Dispatcher{redis: client, updates: updates, count: count, log: log}
}

// queueKey names one shard. Shard zero has no suffix, which is what real-time
// writes.
func queueKey(shard int) string {
	if shard == 0 {
		return "pending-updates-list"
	}
	return "pending-updates-list-" + strconv.Itoa(shard)
}

// Run starts a worker per shard and returns when the context is cancelled.
func (d *Dispatcher) Run(ctx context.Context) {
	for shard := 0; shard < d.count; shard++ {
		go d.worker(ctx, shard)
	}
}

func (d *Dispatcher) worker(ctx context.Context, shard int) {
	key := queueKey(shard)
	d.log.Debug("dispatcher started", slog.String("queue", key))

	for {
		if ctx.Err() != nil {
			return
		}
		// A blocking pop with a timeout rather than an indefinite one, so the
		// worker notices a cancelled context instead of sitting on a socket.
		result, err := d.redis.BLPop(ctx, 5*time.Second, key).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			d.log.Error("error waiting for updates", slog.String("queue", key),
				slog.String("err", err.Error()))
			// Backing off rather than spinning: whatever is wrong with Redis
			// will not be fixed by asking again immediately.
			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
				return
			}
			continue
		}
		if len(result) != 2 {
			continue
		}

		projectID, docID, ok := strings.Cut(result[1], ":")
		if !ok {
			d.log.Error("malformed doc key on the queue",
				slog.String("queue", key), slog.String("value", result[1]))
			continue
		}

		// The work runs in its own context: the pop is done, and abandoning the
		// update now would lose it.
		d.dispatch(context.WithoutCancel(ctx), projectID, docID)
	}
}

func (d *Dispatcher) dispatch(ctx context.Context, projectID, docID string) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	err := d.updates.ProcessOutstandingUpdatesWithLock(ctx, projectID, docID)
	if err == nil {
		return
	}

	// An update the client built against a version the server no longer holds,
	// or a delete that does not match, is the client being out of step rather
	// than this service being broken. The client has already been told, and it
	// recovers by reloading.
	var opRange *OpRangeNotAvailable
	if errors.As(err, &opRange) || errors.Is(err, textot.ErrDeleteMismatch) {
		d.log.Debug("client is out of step with the document",
			slog.String("project", projectID), slog.String("doc", docID),
			slog.String("err", err.Error()))
		return
	}
	d.log.Error("error applying updates", slog.String("project", projectID),
		slog.String("doc", docID), slog.String("err", err.Error()))
}
