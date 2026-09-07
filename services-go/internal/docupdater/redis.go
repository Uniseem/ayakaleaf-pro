package docupdater

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/rediskeys"
	"github.com/redis/go-redis/v9"
)

const (
	// maxRedisRequestLength bounds a read. The document lock expires after
	// thirty seconds, so a call that takes longer than this has probably lost
	// it and must not go on to write.
	maxRedisRequestLength = 5 * time.Second

	// projectBlockTTL is how long a project can be held closed while it is
	// migrated between OT types.
	projectBlockTTL = 30 * time.Second

	// projectStateTTL is how long the record of a project's document list
	// survives.
	projectStateTTL = 30 * time.Minute

	// docOpsTTL is how long recently applied operations are kept, and
	// docOpsMaxLength how many. An update that arrives more versions behind
	// than this cannot be transformed and the client must reload.
	docOpsTTL       = 60 * time.Minute
	docOpsMaxLength = 100

	// maxRangesSize caps the tracked changes and comments of one document.
	maxRangesSize = 3 * 1024 * 1024
)

// RedisStore is the document state in Redis. It is the Go port of
// services/document-updater/app/js/RedisManager.js.
type RedisStore struct {
	redis *redis.Client
	keys  rediskeys.Schema
	log   *slog.Logger

	// maxDocLength is the size a document may not exceed, in characters.
	maxDocLength int
	// smoothingOffset spreads queued flushes out in time so a burst of
	// projects emptying at once does not arrive as a spike.
	smoothingOffset time.Duration
}

// NewRedisStore builds a store.
func NewRedisStore(client *redis.Client, keys rediskeys.Schema, maxDocLength int,
	smoothingOffset time.Duration, log *slog.Logger) *RedisStore {
	return &RedisStore{
		redis: client, keys: keys, log: log,
		maxDocLength: maxDocLength, smoothingOffset: smoothingOffset,
	}
}

// computeHash is the checksum stored alongside a document to detect corruption
// in Redis.
//
// It is a plain SHA-1 of the serialised lines -- not the git-style blob hash
// the client sends with an update, which covers the joined document text
// instead. The two are computed over different bytes and are not
// interchangeable.
func computeHash(docLines string) string {
	sum := sha1.Sum([]byte(docLines))
	return hex.EncodeToString(sum[:])
}

// serializeRanges renders the tracked changes and comments for storage.
//
// An empty object is stored as an empty string: most documents have no ranges,
// and filling Redis with "{}" values costs more than it says.
func serializeRanges(ranges json.RawMessage) (string, error) {
	if len(ranges) == 0 {
		return "", nil
	}
	// Compact first so the emptiness check below does not depend on
	// whitespace: the Node service compares the output of JSON.stringify,
	// which never has any.
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, ranges); err != nil {
		return "", err
	}
	encoded := compacted.String()
	if len(encoded) > maxRangesSize {
		return "", ErrRangesTooLarge
	}
	if encoded == "{}" || encoded == "null" {
		return "", nil
	}
	if strings.ContainsRune(encoded, 0) {
		return "", fmt.Errorf("%w in ranges", ErrNullBytes)
	}
	return encoded, nil
}

// deserializeRanges reads them back. A missing or empty value is an empty
// object, not a null one.
func deserializeRanges(stored string) json.RawMessage {
	if stored == "" {
		return json.RawMessage("{}")
	}
	return json.RawMessage(stored)
}

// docIsTooLarge reports whether the document exceeds the size limit.
//
// The serialised length is an upper bound on the true size, so a document under
// the limit by that measure needs no further checking. Only when it is over
// does the exact size get counted, one line at a time, stopping as soon as the
// limit is passed.
func docIsTooLarge(estimatedSize int, lines []string, maxDocLength int) bool {
	if estimatedSize <= maxDocLength {
		return false
	}
	size := 0
	for _, line := range lines {
		size += len(line) + 1 // the newline that joins it to the next
		if size > maxDocLength {
			return true
		}
	}
	return false
}

// PutDocInMemory loads a document into Redis.
func (s *RedisStore) PutDocInMemory(
	ctx context.Context, projectID, docID string, lines []string, version int64,
	ranges json.RawMessage, resolvedCommentIDs []string,
	pathname, projectHistoryID string, historyRangesSupport bool,
) error {
	encodedLines, err := json.Marshal(lines)
	if err != nil {
		return err
	}
	docLines := string(encodedLines)
	if strings.ContainsRune(docLines, 0) {
		return fmt.Errorf("%w in doc lines", ErrNullBytes)
	}
	if docIsTooLarge(len(docLines), lines, s.maxDocLength) {
		return fmt.Errorf("blocking doc insert into redis: %w", ErrDocTooLarge)
	}
	serializedRanges, err := serializeRanges(ranges)
	if err != nil {
		return err
	}

	// The project's document set is updated before the contents, so a project
	// being blocked mid-load is noticed here rather than after the write.
	blockPipe := s.redis.TxPipeline()
	blocked := blockPipe.Exists(ctx, s.keys.ProjectBlock(projectID))
	blockPipe.SAdd(ctx, s.keys.DocsInProject(projectID), docID)
	if _, err := blockPipe.Exec(ctx); err != nil {
		return err
	}
	if blocked.Val() == 1 {
		// The stray docId left in the set is deliberate: a concurrent load may
		// have added it legitimately, and an extra id only causes an extra
		// unload attempt, while a missing one strands the document in Redis.
		return ErrProjectBlocked
	}

	if err := s.SetHistoryRangesSupportFlag(ctx, docID, historyRangesSupport); err != nil {
		return err
	}

	pipe := s.redis.TxPipeline()
	pipe.MSet(ctx, map[string]any{
		s.keys.DocLines(docID):         docLines,
		s.keys.ProjectKey(docID):       projectID,
		s.keys.DocVersion(docID):       version,
		s.keys.DocHash(docID):          computeHash(docLines),
		s.keys.Ranges(docID):           serializedRanges,
		s.keys.Pathname(docID):         pathname,
		s.keys.ProjectHistoryID(docID): projectHistoryID,
	})
	if historyRangesSupport {
		pipe.Del(ctx, s.keys.ResolvedCommentIds(docID))
		if len(resolvedCommentIDs) > 0 {
			members := make([]any, len(resolvedCommentIDs))
			for i, id := range resolvedCommentIDs {
				members[i] = id
			}
			pipe.SAdd(ctx, s.keys.ResolvedCommentIds(docID), members...)
		}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to write doc to Redis: %w", err)
	}
	return nil
}

// RemoveDocFromMemory unloads a document.
func (s *RedisStore) RemoveDocFromMemory(ctx context.Context, projectID, docID string) error {
	pipe := s.redis.TxPipeline()
	pipe.Del(ctx,
		s.keys.DocLines(docID),
		s.keys.ProjectKey(docID),
		s.keys.DocVersion(docID),
		s.keys.DocHash(docID),
		s.keys.Ranges(docID),
		s.keys.Pathname(docID),
		s.keys.ProjectHistoryID(docID),
		s.keys.UnflushedTime(docID),
		s.keys.LastUpdatedAt(docID),
		s.keys.LastUpdatedBy(docID),
		s.keys.ResolvedCommentIds(docID),
	)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}

	pipe = s.redis.TxPipeline()
	pipe.SRem(ctx, s.keys.DocsInProject(projectID), docID)
	pipe.Del(ctx, s.keys.ProjectState(projectID))
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}

	return s.redis.SRem(ctx, s.keys.HistoryRangesSupport(), docID).Err()
}

// GetDoc reads a document.
//
// A document that is not loaded comes back with Loaded() false rather than an
// error: the caller then fetches it from the database and loads it.
func (s *RedisStore) GetDoc(ctx context.Context, projectID, docID string) (*Doc, error) {
	started := time.Now()

	values, err := s.redis.MGet(ctx,
		s.keys.DocLines(docID),
		s.keys.DocVersion(docID),
		s.keys.DocHash(docID),
		s.keys.ProjectKey(docID),
		s.keys.Ranges(docID),
		s.keys.Pathname(docID),
		s.keys.ProjectHistoryID(docID),
		s.keys.UnflushedTime(docID),
		s.keys.LastUpdatedAt(docID),
		s.keys.LastUpdatedBy(docID),
	).Result()
	if err != nil {
		return nil, err
	}

	historyRangesSupport, err := s.redis.SIsMember(ctx, s.keys.HistoryRangesSupport(), docID).Result()
	if err != nil {
		return nil, err
	}
	resolvedCommentIDs, err := s.redis.SMembers(ctx, s.keys.ResolvedCommentIds(docID)).Result()
	if err != nil {
		return nil, err
	}

	// Only this call is timed. It is the first of every update, so if it came
	// back in time the ones after it have a fair chance of doing the same.
	if elapsed := time.Since(started); elapsed > maxRedisRequestLength {
		return nil, fmt.Errorf("redis getDoc exceeded timeout after %s", elapsed)
	}

	str := func(i int) string {
		if v, ok := values[i].(string); ok {
			return v
		}
		return ""
	}

	doc := &Doc{
		Pathname:             str(5),
		ProjectHistoryID:     str(6),
		UnflushedTime:        str(7),
		LastUpdatedAt:        str(8),
		LastUpdatedBy:        str(9),
		HistoryRangesSupport: historyRangesSupport,
		ResolvedCommentIDs:   resolvedCommentIDs,
		Ranges:               deserializeRanges(str(4)),
	}

	docLines := str(0)
	if docLines != "" {
		if stored := str(2); stored != "" {
			if computed := computeHash(docLines); computed != stored {
				// The document is served anyway, as the Node service does:
				// refusing it would lock the user out of their project, and
				// the copy in Redis is still the most recent one there is.
				s.log.Error("hash mismatch on retrieved document",
					slog.String("project", projectID), slog.String("doc", docID),
					slog.String("computed", computed), slog.String("stored", stored))
			}
		}
		doc.RawLines = json.RawMessage(docLines)
		// An array is a ShareJS document; anything else is history-ot, which
		// this port does not handle but must still be able to recognise.
		var lines []string
		if err := json.Unmarshal(doc.RawLines, &lines); err == nil {
			doc.Lines = lines
		}
	}

	doc.Version = parseVersion(str(1))

	// A document id is unique, so finding it under a different project means
	// the caller asked about the wrong one.
	if docProjectID := str(3); docProjectID != "" && docProjectID != projectID {
		return nil, fmt.Errorf("%w: doc %s belongs to project %s, not %s",
			ErrNotFound, docID, docProjectID, projectID)
	}

	return doc, nil
}

// parseVersion reads a stored version, treating anything unparseable as zero,
// the way parseInt(version || 0, 10) does.
func parseVersion(stored string) int64 {
	if stored == "" {
		return 0
	}
	v, err := strconv.ParseInt(stored, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// GetDocVersion reads just the version.
func (s *RedisStore) GetDocVersion(ctx context.Context, docID string) (int64, error) {
	stored, err := s.redis.Get(ctx, s.keys.DocVersion(docID)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return parseVersion(stored), nil
}

// GetDocLines reads the stored lines without decoding them.
func (s *RedisStore) GetDocLines(ctx context.Context, docID string) (string, error) {
	lines, err := s.redis.Get(ctx, s.keys.DocLines(docID)).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	return lines, err
}

// GetDocRanges reads the tracked changes and comments.
func (s *RedisStore) GetDocRanges(ctx context.Context, docID string) (json.RawMessage, error) {
	stored, err := s.redis.Get(ctx, s.keys.Ranges(docID)).Result()
	if errors.Is(err, redis.Nil) {
		return deserializeRanges(""), nil
	}
	if err != nil {
		return nil, err
	}
	return deserializeRanges(stored), nil
}

// GetPreviousDocOps returns the operations applied between two versions, so an
// update built against an older version can be transformed up to the current
// one.
//
// The range is half open, matching the Node service: end is exclusive, and -1
// means "up to the latest".
func (s *RedisStore) GetPreviousDocOps(ctx context.Context, docID string, start, end int64) ([]json.RawMessage, error) {
	started := time.Now()

	length, err := s.redis.LLen(ctx, s.keys.DocOps(docID)).Result()
	if err != nil {
		return nil, err
	}
	version, err := s.GetDocVersion(ctx, docID)
	if err != nil {
		return nil, err
	}

	// Only the last docOpsMaxLength operations are kept, so the oldest version
	// that can still be caught up from is this one.
	firstVersionInRedis := version - length
	if start < firstVersionInRedis || end > version {
		return nil, &OpRangeNotAvailable{
			FirstVersionInRedis: firstVersionInRedis,
			Version:             version,
			TTLInS:              int(docOpsTTL / time.Second),
		}
	}

	start -= firstVersionInRedis
	if end > -1 {
		end -= firstVersionInRedis
	}

	encoded, err := s.redis.LRange(ctx, s.keys.DocOps(docID), start, end).Result()
	if err != nil {
		return nil, err
	}
	if elapsed := time.Since(started); elapsed > maxRedisRequestLength {
		return nil, fmt.Errorf("redis getPreviousDocOps exceeded timeout after %s", elapsed)
	}

	ops := make([]json.RawMessage, 0, len(encoded))
	for _, op := range encoded {
		ops = append(ops, json.RawMessage(op))
	}
	return ops, nil
}

// UpdateDocument writes a document back after operations have been applied to
// it, and records those operations so later updates can be transformed.
func (s *RedisStore) UpdateDocument(
	ctx context.Context, projectID, docID string, lines []string, newVersion int64,
	appliedOps []json.RawMessage, ranges json.RawMessage, userID string,
) error {
	currentVersion, err := s.GetDocVersion(ctx, docID)
	if err != nil {
		return err
	}
	// The update was built against the version in Redis and applied under the
	// document lock, so the arithmetic must come out exactly. If it does not,
	// something wrote without holding the lock.
	if currentVersion+int64(len(appliedOps)) != newVersion {
		return fmt.Errorf("%w: at %d, %d ops applied, told to write %d",
			ErrVersionMismatch, currentVersion, len(appliedOps), newVersion)
	}

	for _, op := range appliedOps {
		if strings.ContainsRune(string(op), 0) {
			return fmt.Errorf("%w in applied ops", ErrNullBytes)
		}
	}

	encodedLines, err := json.Marshal(lines)
	if err != nil {
		return err
	}
	newDocLines := string(encodedLines)
	if strings.ContainsRune(newDocLines, 0) {
		return fmt.Errorf("%w in doc lines", ErrNullBytes)
	}
	if docIsTooLarge(len(newDocLines), lines, s.maxDocLength) {
		return fmt.Errorf("blocking doc update: %w", ErrDocTooLarge)
	}
	serializedRanges, err := serializeRanges(ranges)
	if err != nil {
		return err
	}

	now := time.Now().UnixMilli()
	pipe := s.redis.TxPipeline()
	pipe.MSet(ctx, map[string]any{
		s.keys.DocLines(docID):      newDocLines,
		s.keys.DocVersion(docID):    newVersion,
		s.keys.DocHash(docID):       computeHash(newDocLines),
		s.keys.Ranges(docID):        serializedRanges,
		s.keys.LastUpdatedAt(docID): now,
		s.keys.LastUpdatedBy(docID): userID,
	})
	pipe.LTrim(ctx, s.keys.DocOps(docID), -docOpsMaxLength, -1)
	if len(appliedOps) > 0 {
		ops := make([]any, len(appliedOps))
		for i, op := range appliedOps {
			ops[i] = string(op)
		}
		pipe.RPush(ctx, s.keys.DocOps(docID), ops...)
		// The expiry has to follow the push: setting it on an empty list does
		// nothing.
		pipe.Expire(ctx, s.keys.DocOps(docID), docOpsTTL)
	}
	// Only the first unflushed change records its time, so the age of the
	// oldest unsaved edit is what decides when to write the document out.
	pipe.SetArgs(ctx, s.keys.UnflushedTime(docID), now, redis.SetArgs{Mode: "NX"})

	_, err = pipe.Exec(ctx)
	return err
}

// RenameDoc records a document's new path, but only if it is loaded.
func (s *RedisStore) RenameDoc(ctx context.Context, projectID, docID, newPathname string) error {
	doc, err := s.GetDoc(ctx, projectID, docID)
	if err != nil {
		return err
	}
	if !doc.Loaded() {
		return nil
	}
	if newPathname == "" {
		s.log.Warn("missing pathname in renameDoc",
			slog.String("project", projectID), slog.String("doc", docID))
	}
	return s.redis.Set(ctx, s.keys.Pathname(docID), newPathname, 0).Err()
}

// ClearUnflushedTime marks a document as written out.
func (s *RedisStore) ClearUnflushedTime(ctx context.Context, docID string) error {
	return s.redis.Del(ctx, s.keys.UnflushedTime(docID)).Err()
}

// UpdateCommentState records whether a comment thread is resolved.
func (s *RedisStore) UpdateCommentState(ctx context.Context, docID, commentID string, resolved bool) error {
	if resolved {
		return s.redis.SAdd(ctx, s.keys.ResolvedCommentIds(docID), commentID).Err()
	}
	return s.redis.SRem(ctx, s.keys.ResolvedCommentIds(docID), commentID).Err()
}

// GetDocIDsInProject lists the documents currently loaded for a project.
func (s *RedisStore) GetDocIDsInProject(ctx context.Context, projectID string) ([]string, error) {
	return s.redis.SMembers(ctx, s.keys.DocsInProject(projectID)).Result()
}

// GetDocTimestamps reads when each document was last changed. A document that
// has never been changed yields an empty string.
func (s *RedisStore) GetDocTimestamps(ctx context.Context, docIDs []string) ([]string, error) {
	timestamps := make([]string, 0, len(docIDs))
	for _, docID := range docIDs {
		ts, err := s.redis.Get(ctx, s.keys.LastUpdatedAt(docID)).Result()
		if errors.Is(err, redis.Nil) {
			timestamps = append(timestamps, "")
			continue
		}
		if err != nil {
			return nil, err
		}
		timestamps = append(timestamps, ts)
	}
	return timestamps, nil
}

// SetHistoryRangesSupportFlag records whether a document's ranges are tracked
// in history.
func (s *RedisStore) SetHistoryRangesSupportFlag(ctx context.Context, docID string, supported bool) error {
	if supported {
		return s.redis.SAdd(ctx, s.keys.HistoryRangesSupport(), docID).Err()
	}
	return s.redis.SRem(ctx, s.keys.HistoryRangesSupport(), docID).Err()
}

// RecordProjectNotificationTimestamp records when a project first changed, for
// the email that tells collaborators about it. Only the first change counts.
func (s *RedisStore) RecordProjectNotificationTimestamp(ctx context.Context, projectID string, timestamp int64) error {
	return s.redis.SetArgs(ctx, s.keys.ProjectNotificationTimestamp(projectID),
		timestamp, redis.SetArgs{Mode: "NX"}).Err()
}

// CheckOrSetProjectState records the state of a project's document list and
// reports whether it changed.
func (s *RedisStore) CheckOrSetProjectState(ctx context.Context, projectID, newState string) (changed bool, err error) {
	pipe := s.redis.TxPipeline()
	previous := pipe.GetSet(ctx, s.keys.ProjectState(projectID), newState)
	pipe.Expire(ctx, s.keys.ProjectState(projectID), projectStateTTL)
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return false, err
	}
	return previous.Val() != newState, nil
}

// ClearProjectState forgets the recorded document list.
func (s *RedisStore) ClearProjectState(ctx context.Context, projectID string) error {
	return s.redis.Del(ctx, s.keys.ProjectState(projectID)).Err()
}

// QueueFlushAndDeleteProject schedules a project to be written out and
// unloaded.
//
// The scheduled time is spread over a window so that a lecture theatre closing
// its laptops at once does not arrive as a single spike of database writes.
func (s *RedisStore) QueueFlushAndDeleteProject(ctx context.Context, projectID string) error {
	offset := int64(0)
	if s.smoothingOffset > 0 {
		offset = rand.Int64N(s.smoothingOffset.Milliseconds() + 1)
	}
	return s.redis.ZAdd(ctx, s.keys.FlushAndDeleteQueue(), redis.Z{
		Score:  float64(time.Now().UnixMilli() + offset),
		Member: projectID,
	}).Err()
}

// FlushCandidate is a project waiting to be written out.
type FlushCandidate struct {
	ProjectID      string
	FlushTimestamp int64
	QueueLength    int64
}

// GetNextProjectToFlushAndDelete takes the oldest project scheduled before the
// cutoff. It returns a nil candidate when nothing is due.
func (s *RedisStore) GetNextProjectToFlushAndDelete(ctx context.Context, cutoff int64) (*FlushCandidate, error) {
	ready, err := s.redis.ZRangeByScoreWithScores(ctx, s.keys.FlushAndDeleteQueue(), &redis.ZRangeBy{
		Min: "0", Max: strconv.FormatInt(cutoff, 10), Offset: 0, Count: 1,
	}).Result()
	if err != nil {
		return nil, err
	}
	if len(ready) == 0 {
		return nil, nil
	}

	// Take the oldest entry and remove it in one round trip. ZPOPMIN would do
	// this directly but needs Redis 5, which the Node service does not assume.
	pipe := s.redis.TxPipeline()
	oldest := pipe.ZRangeWithScores(ctx, s.keys.FlushAndDeleteQueue(), 0, 0)
	pipe.ZRemRangeByRank(ctx, s.keys.FlushAndDeleteQueue(), 0, 0)
	remaining := pipe.ZCard(ctx, s.keys.FlushAndDeleteQueue())
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}
	if len(oldest.Val()) == 0 {
		return nil, nil
	}

	entry := oldest.Val()[0]
	member, _ := entry.Member.(string)
	return &FlushCandidate{
		ProjectID:      member,
		FlushTimestamp: int64(entry.Score),
		QueueLength:    remaining.Val(),
	}, nil
}

// BlockProject stops new documents being loaded for a project, so it can be
// migrated between OT types.
//
// It fails if documents are already loaded: the migration needs the project
// quiet, and losing that race is normal rather than exceptional.
func (s *RedisStore) BlockProject(ctx context.Context, projectID string) (bool, error) {
	pipe := s.redis.TxPipeline()
	pipe.Set(ctx, s.keys.ProjectBlock(projectID), "1", projectBlockTTL)
	loaded := pipe.SCard(ctx, s.keys.DocsInProject(projectID))
	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}
	if loaded.Val() > 0 {
		if err := s.redis.Del(ctx, s.keys.ProjectBlock(projectID)).Err(); err != nil {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

// UnblockProject lets documents be loaded again, reporting whether the project
// was in fact blocked.
func (s *RedisStore) UnblockProject(ctx context.Context, projectID string) (bool, error) {
	removed, err := s.redis.Del(ctx, s.keys.ProjectBlock(projectID)).Result()
	if err != nil {
		return false, err
	}
	return removed == 1, nil
}
