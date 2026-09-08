// Package projecthistory is the Go port of services/project-history.
//
// It reads the queue document-updater fills, turns what it finds into the form
// the history is written in, and hands it to history-v1. It also answers the
// questions web asks about a project's history: what changed, what a version
// looked like, what the labels are.
package projecthistory

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// How much is taken off the queue at once.
//
// The limits are all about memory rather than speed: a project that has been
// edited heavily can have a very long queue, and reading it whole would be one
// project deciding how much memory this service uses.
const (
	// rawUpdateSizeThreshold bounds the bytes read in one batch.
	rawUpdateSizeThreshold = 4 * 1024 * 1024
	// rawUpdatesBatchSize is how many entries are read from Redis at a time.
	rawUpdatesBatchSize = 50
	// maxUpdateOpLength bounds the operations processed in one iteration.
	maxUpdateOpLength = 1024
	// maxNewDocContentCount bounds the whole documents processed in one
	// iteration, which are much larger than an ordinary edit.
	maxNewDocContentCount = 32
	// warnRawUpdateSize is the point at which a batch is worth a log line.
	warnRawUpdateSize = 1024 * 1024
	// cachedHistoryIDTTL is how long the history id of a project is remembered
	// rather than asked of web again.
	cachedHistoryIDTTL = time.Hour
)

// Keys names the Redis keys this service shares with document-updater.
//
// server-ce replaces the key schema of every service with an unbraced one, but
// not these three: they keep the hash-tag braces even there. Writing them the
// other way is silent -- the queue is filled under one name and read under
// another, and nothing reaches the history.
type Keys struct{}

// ProjectHistoryOps is the queue of updates waiting for a project.
func (Keys) ProjectHistoryOps(projectID string) string {
	return "ProjectHistory:Ops:{" + projectID + "}"
}

// ProjectHistoryFirstOpTimestamp is when the oldest update in that queue
// arrived, which is how a project that has been waiting too long is found.
func (Keys) ProjectHistoryFirstOpTimestamp(projectID string) string {
	return "ProjectHistory:FirstOpTimestamp:{" + projectID + "}"
}

// ProjectHistoryCachedHistoryID is the history id of a project, remembered so
// that every flush does not ask web for it again.
func (Keys) ProjectHistoryCachedHistoryID(projectID string) string {
	return "ProjectHistory:CachedHistoryId:{" + projectID + "}"
}

// ProjectHistoryLock is the lock held while a project is being processed.
func (Keys) ProjectHistoryLock(projectID string) string {
	return "ProjectHistoryLock:{" + projectID + "}"
}

// RedisStore is the queue and the small pieces of state around it.
type RedisStore struct {
	queue *redis.Client
	keys  Keys
}

// NewRedisStore builds the store.
func NewRedisStore(queue *redis.Client) *RedisStore {
	return &RedisStore{queue: queue}
}

// CountUnprocessedUpdates is how many updates are waiting for a project.
func (s *RedisStore) CountUnprocessedUpdates(ctx context.Context, projectID string) (int64, error) {
	return s.queue.LLen(ctx, s.keys.ProjectHistoryOps(projectID)).Result()
}

// RawBatch is a run of updates taken off the queue, still as they were stored.
//
// The raw bytes are kept alongside the parsed updates because that is what
// removing them from the queue matches on: an update is deleted by value, so
// what is deleted has to be exactly what was read.
type RawBatch struct {
	Raw []string
	// HasMore says whether the queue held more than this batch.
	HasMore bool
}

// GetRawUpdatesBatch reads a batch off the front of the queue without removing
// it.
//
// Nothing is removed until the batch has been dealt with. A crash halfway
// through leaves the queue as it was, and the work is done again rather than
// lost.
func (s *RedisStore) GetRawUpdatesBatch(ctx context.Context, projectID string,
	batchSize int) (*RawBatch, error) {

	key := s.keys.ProjectHistoryOps(projectID)
	batch := &RawBatch{}
	var totalSize int
	var start int64

	for {
		stop := start + rawUpdatesBatchSize - 1
		updates, err := s.queue.LRange(ctx, key, start, stop).Result()
		if err != nil {
			return nil, err
		}

		for _, update := range updates {
			totalSize += len(update)
			// The first update is always taken, however large: refusing it
			// would leave the queue stuck on it for ever.
			if len(batch.Raw) > 0 && totalSize > rawUpdateSizeThreshold {
				batch.HasMore = true
				return batch, nil
			}
			batch.Raw = append(batch.Raw, update)
			if len(batch.Raw) >= batchSize {
				batch.HasMore = true
				return batch, nil
			}
		}

		if len(updates) < rawUpdatesBatchSize {
			return batch, nil
		}
		start += rawUpdatesBatchSize
	}
}

// DeleteAppliedUpdates takes the updates that have been dealt with off the
// queue.
//
// Each is removed by value, one occurrence at a time. Removing by value with a
// count of zero would search the whole queue, which on a busy project is long
// enough to hold Redis up.
func (s *RedisStore) DeleteAppliedUpdates(ctx context.Context, projectID string,
	raw []string) error {
	if len(raw) == 0 {
		return nil
	}

	key := s.keys.ProjectHistoryOps(projectID)
	pipe := s.queue.TxPipeline()
	for _, update := range raw {
		pipe.LRem(ctx, key, 1, update)
	}
	// The queue has been dealt with up to here, so the record of when it
	// started waiting goes too.
	pipe.Del(ctx, s.keys.ProjectHistoryFirstOpTimestamp(projectID))
	_, err := pipe.Exec(ctx)
	return err
}

// DestroyQueue empties a project's queue. It is what deleting a project does,
// and nothing else should.
func (s *RedisStore) DestroyQueue(ctx context.Context, projectID string) error {
	return s.queue.Del(ctx,
		s.keys.ProjectHistoryOps(projectID),
		s.keys.ProjectHistoryFirstOpTimestamp(projectID),
	).Err()
}

// SetFirstOpTimestamp records when a project started waiting, if it is not
// already recorded.
func (s *RedisStore) SetFirstOpTimestamp(ctx context.Context, projectID string) error {
	return s.queue.SetNX(ctx, s.keys.ProjectHistoryFirstOpTimestamp(projectID),
		time.Now().UnixMilli(), 0).Err()
}

// GetFirstOpTimestamp reads when a project started waiting, or the zero time.
func (s *RedisStore) GetFirstOpTimestamp(ctx context.Context, projectID string) (time.Time, error) {
	raw, err := s.queue.Get(ctx, s.keys.ProjectHistoryFirstOpTimestamp(projectID)).Result()
	if err == redis.Nil {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return parseMillis(raw), nil
}

// GetFirstOpTimestamps reads them for several projects at once.
func (s *RedisStore) GetFirstOpTimestamps(ctx context.Context,
	projectIDs []string) ([]time.Time, error) {
	if len(projectIDs) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(projectIDs))
	for _, projectID := range projectIDs {
		keys = append(keys, s.keys.ProjectHistoryFirstOpTimestamp(projectID))
	}

	values, err := s.queue.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	timestamps := make([]time.Time, 0, len(values))
	for _, value := range values {
		raw, _ := value.(string)
		timestamps = append(timestamps, parseMillis(raw))
	}
	return timestamps, nil
}

// ClearFirstOpTimestamp forgets when a project started waiting.
func (s *RedisStore) ClearFirstOpTimestamp(ctx context.Context, projectID string) error {
	return s.queue.Del(ctx, s.keys.ProjectHistoryFirstOpTimestamp(projectID)).Err()
}

// ClearDanglingFirstOpTimestamp forgets a timestamp left behind by a queue that
// is no longer there.
//
// The two keys should appear and disappear together. One without the other is
// a record of a wait that finished, and leaving it would make the project look
// permanently overdue.
func (s *RedisStore) ClearDanglingFirstOpTimestamp(ctx context.Context, projectID string) (bool, error) {
	present, err := s.queue.Exists(ctx,
		s.keys.ProjectHistoryFirstOpTimestamp(projectID),
		s.keys.ProjectHistoryOps(projectID),
	).Result()
	if err != nil {
		return false, err
	}
	if present == 2 || present == 0 {
		return false, nil
	}
	cleared, err := s.queue.Del(ctx, s.keys.ProjectHistoryFirstOpTimestamp(projectID)).Result()
	return cleared > 0, err
}

// GetCachedHistoryID reads the remembered history id of a project.
func (s *RedisStore) GetCachedHistoryID(ctx context.Context, projectID string) (string, error) {
	id, err := s.queue.Get(ctx, s.keys.ProjectHistoryCachedHistoryID(projectID)).Result()
	if err == redis.Nil {
		return "", nil
	}
	return id, err
}

// SetCachedHistoryID remembers the history id of a project for an hour.
func (s *RedisStore) SetCachedHistoryID(ctx context.Context, projectID, historyID string) error {
	return s.queue.Set(ctx, s.keys.ProjectHistoryCachedHistoryID(projectID),
		historyID, cachedHistoryIDTTL).Err()
}

// ClearCachedHistoryID forgets it.
func (s *RedisStore) ClearCachedHistoryID(ctx context.Context, projectID string) error {
	return s.queue.Del(ctx, s.keys.ProjectHistoryCachedHistoryID(projectID)).Err()
}

// GetProjectIDsWithHistoryOps lists the projects that have a queue.
func (s *RedisStore) GetProjectIDsWithHistoryOps(ctx context.Context, limit int) ([]string, error) {
	return s.scanForIDs(ctx, s.keys.ProjectHistoryOps("*"), limit)
}

// CountProjectsWithHistoryOps is how many projects have a queue.
func (s *RedisStore) CountProjectsWithHistoryOps(ctx context.Context) (int, error) {
	ids, err := s.scanForIDs(ctx, s.keys.ProjectHistoryOps("*"), -1)
	return len(ids), err
}

// GetProjectIDsWithFirstOpTimestamps lists the projects that are waiting.
func (s *RedisStore) GetProjectIDsWithFirstOpTimestamps(ctx context.Context, limit int) ([]string, error) {
	return s.scanForIDs(ctx, s.keys.ProjectHistoryFirstOpTimestamp("*"), limit)
}

// scanForIDs walks the keyspace for a pattern and pulls the project ids out.
//
// It scans rather than using KEYS: this runs against the same Redis the editing
// does, and blocking it to list a pattern would stop everybody typing.
func (s *RedisStore) scanForIDs(ctx context.Context, pattern string, limit int) ([]string, error) {
	var ids []string
	var cursor uint64
	for {
		keys, next, err := s.queue.Scan(ctx, cursor, pattern, 1000).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			if id := extractID(key); id != "" {
				ids = append(ids, id)
				if limit > 0 && len(ids) >= limit {
					return ids, nil
				}
			}
		}
		if next == 0 {
			return ids, nil
		}
		cursor = next
	}
}

// extractID takes the project id out of a key, which carries it in braces.
func extractID(key string) string {
	open := strings.Index(key, "{")
	closed := strings.LastIndex(key, "}")
	if open == -1 || closed == -1 || closed < open {
		return ""
	}
	return key[open+1 : closed]
}

// parseMillis reads a millisecond timestamp as it is stored, and returns the
// zero time for anything unreadable.
func parseMillis(raw string) time.Time {
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.UnixMilli(parsed).UTC()
}

// ParseUpdates reads the queued updates.
func ParseUpdates(raw []string) ([]json.RawMessage, error) {
	updates := make([]json.RawMessage, 0, len(raw))
	for i, update := range raw {
		if !json.Valid([]byte(update)) {
			return nil, fmt.Errorf("failed to parse update %d", i)
		}
		updates = append(updates, json.RawMessage(update))
	}
	return updates, nil
}
