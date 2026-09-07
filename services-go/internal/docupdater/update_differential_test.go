package docupdater

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/rediskeys"
	"github.com/redis/go-redis/v9"
)

// The write path is compared by putting the same edit through both services and
// looking at everything each of them produced: the document, the version, the
// operation kept for transforming later updates, the message published to the
// editors, and the entry queued for history.
//
// The two are driven differently on purpose. The Node service is given the edit
// the way real-time gives it one -- pushed onto a shard queue for its dispatcher
// to pick up -- while the Go pipeline is called directly, because both
// dispatchers are running and would otherwise race for the same queue entry.
// The Go dispatcher is covered on its own below.

type writePathSetup struct {
	*sideBySide
	updates *UpdateManager
	store   *RedisStore
	web     *mockWeb
	goBase  string
}

func newWritePathSetup(t *testing.T) *writePathSetup {
	t.Helper()
	addr := os.Getenv("DOCUPDATER_TEST_REDIS")
	nodeDir := os.Getenv("DOCUPDATER_NODE_DIR")
	if addr == "" || nodeDir == "" {
		t.Skip("set DOCUPDATER_TEST_REDIS and DOCUPDATER_NODE_DIR to run this")
	}

	web := newMockWeb(t)
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })

	log := slog.New(slog.DiscardHandler)
	store := NewRedisStore(client, rediskeys.Upstream, testMaxDocLength, 0, log)
	persistence := NewPersistenceClient(web.URL, "overleaf", "password")
	locker := NewLocker(client, rediskeys.Upstream, 0)
	docs := NewDocumentManager(store, persistence, locker, log)
	bridge := NewRealTimeBridge(client, client, rediskeys.Upstream, false, log)
	history := NewHistoryQueue(client, rediskeys.Upstream)
	updates := NewUpdateManager(store, docs, locker, bridge, history,
		NewWebClient(web.URL, "overleaf", "password"), testMaxDocLength, log)

	nodeURL := startNodeService(t, nodeDir, web.URL, addr)
	return &writePathSetup{
		sideBySide: &sideBySide{t: t, node: nodeURL, client: client},
		updates:    updates, store: store, web: web,
	}
}

// pushUpdate queues an edit the way real-time does.
func (s *writePathSetup) pushUpdate(t *testing.T, update map[string]any, toShardQueue bool) {
	t.Helper()
	ctx := context.Background()
	encoded, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("encoding update: %v", err)
	}
	if err := s.client.RPush(ctx, rediskeys.Upstream.PendingUpdates(testDocID),
		string(encoded)).Err(); err != nil {
		t.Fatalf("queueing the update: %v", err)
	}
	if toShardQueue {
		if err := s.client.RPush(ctx, "pending-updates-list",
			testProjectID+":"+testDocID).Err(); err != nil {
			t.Fatalf("queueing the doc: %v", err)
		}
	}
}

// outcome is everything an applied update leaves behind.
type outcome struct {
	Lines      []string
	Version    string
	Ranges     string
	DocOps     []string
	AppliedOps []map[string]any
	History    []map[string]any
}

func (s *writePathSetup) capture(t *testing.T) outcome {
	t.Helper()
	ctx := context.Background()
	keys := rediskeys.Upstream

	var out outcome
	if raw, err := s.client.Get(ctx, keys.DocLines(testDocID)).Result(); err == nil {
		_ = json.Unmarshal([]byte(raw), &out.Lines)
	}
	out.Version, _ = s.client.Get(ctx, keys.DocVersion(testDocID)).Result()
	out.Ranges, _ = s.client.Get(ctx, keys.Ranges(testDocID)).Result()
	out.DocOps, _ = s.client.LRange(ctx, keys.DocOps(testDocID), 0, -1).Result()

	history, _ := s.client.LRange(ctx, keys.ProjectHistoryOps(testProjectID), 0, -1).Result()
	for _, entry := range history {
		var decoded map[string]any
		if err := json.Unmarshal([]byte(entry), &decoded); err == nil {
			out.History = append(out.History, decoded)
		}
	}
	return out
}

// collectAppliedOps subscribes before the edit is made and returns whatever was
// published while fn ran.
func (s *writePathSetup) collectAppliedOps(t *testing.T, fn func()) []map[string]any {
	t.Helper()
	ctx := context.Background()

	sub := s.client.Subscribe(ctx, "applied-ops")
	defer func() { _ = sub.Close() }()
	if _, err := sub.Receive(ctx); err != nil {
		t.Fatalf("subscribing: %v", err)
	}

	done := make(chan []map[string]any, 1)
	go func() {
		var messages []map[string]any
		channel := sub.Channel()
		idle := time.NewTimer(2 * time.Second)
		defer idle.Stop()
		for {
			select {
			case msg, ok := <-channel:
				if !ok {
					done <- messages
					return
				}
				var decoded map[string]any
				if err := json.Unmarshal([]byte(msg.Payload), &decoded); err == nil {
					// The message id carries the host and a per-process
					// counter, so it cannot match between two services.
					delete(decoded, "_id")
					messages = append(messages, decoded)
				}
				idle.Reset(500 * time.Millisecond)
			case <-idle.C:
				done <- messages
				return
			}
		}
	}()

	fn()
	return <-done
}

// loadDoc puts the document into Redis through the given service's read API,
// which is how it gets there before an edit arrives.
func (s *writePathSetup) loadDocViaNode(t *testing.T) {
	t.Helper()
	status, body := s.request(t, s.node, http.MethodGet,
		fmt.Sprintf("/project/%s/doc/%s", testProjectID, testDocID))
	if status != http.StatusOK {
		t.Fatalf("loading the doc through Node: %d %s", status, body)
	}
}

func (s *writePathSetup) loadDocViaGo(t *testing.T) {
	t.Helper()
	if _, err := s.updates.docs.GetDoc(context.Background(), testProjectID, testDocID); err != nil {
		t.Fatalf("loading the doc through Go: %v", err)
	}
}

func TestSideBySideApplyUpdate(t *testing.T) {
	s := newWritePathSetup(t)
	ctx := context.Background()

	// An insert at the end of the first line, at the version the mock web
	// serves.
	update := map[string]any{
		"doc":  testDocID,
		"op":   []map[string]any{{"p": 23, "i": " % edited"}},
		"v":    12,
		"meta": map[string]any{"source": "P.testsource", "user_id": "u1", "ts": 1700000000000},
	}

	// Node first, driven the way real-time drives it.
	if err := s.client.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flushing redis: %v", err)
	}
	s.loadDocViaNode(t)
	nodeOps := s.collectAppliedOps(t, func() {
		s.pushUpdate(t, update, true)
		s.waitForVersion(t, "13")
	})
	fromNode := s.capture(t)
	fromNode.AppliedOps = nodeOps

	// Then Go, from the same starting state.
	if err := s.client.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flushing redis: %v", err)
	}
	s.loadDocViaGo(t)
	goOps := s.collectAppliedOps(t, func() {
		s.pushUpdate(t, update, false)
		if err := s.updates.ProcessOutstandingUpdatesWithLock(ctx, testProjectID, testDocID); err != nil {
			t.Fatalf("applying the update: %v", err)
		}
	})
	fromGo := s.capture(t)
	fromGo.AppliedOps = goOps

	compareOutcomes(t, fromNode, fromGo)
}

// An update built against an older version must be transformed against what
// landed in between, and both services must transform it the same way.
func TestSideBySideApplyUpdateNeedingTransform(t *testing.T) {
	s := newWritePathSetup(t)
	ctx := context.Background()

	first := map[string]any{
		"doc":  testDocID,
		"op":   []map[string]any{{"p": 0, "i": "AAA"}},
		"v":    12,
		"meta": map[string]any{"source": "P.first", "user_id": "u1", "ts": 1700000000000},
	}
	// Built against version 12 as well, so it arrives one version behind and
	// has to be moved past the insert above.
	second := map[string]any{
		"doc":  testDocID,
		"op":   []map[string]any{{"p": 5, "i": "BBB"}},
		"v":    12,
		"meta": map[string]any{"source": "P.second", "user_id": "u2", "ts": 1700000000001},
	}

	run := func(driver func()) outcome {
		if err := s.client.FlushDB(ctx).Err(); err != nil {
			t.Fatalf("flushing redis: %v", err)
		}
		ops := s.collectAppliedOps(t, driver)
		out := s.capture(t)
		out.AppliedOps = ops
		return out
	}

	fromNode := run(func() {
		s.loadDocViaNode(t)
		s.pushUpdate(t, first, true)
		s.waitForVersion(t, "13")
		s.pushUpdate(t, second, true)
		s.waitForVersion(t, "14")
	})
	fromGo := run(func() {
		s.loadDocViaGo(t)
		s.pushUpdate(t, first, false)
		if err := s.updates.ProcessOutstandingUpdatesWithLock(ctx, testProjectID, testDocID); err != nil {
			t.Fatalf("applying the first update: %v", err)
		}
		s.pushUpdate(t, second, false)
		if err := s.updates.ProcessOutstandingUpdatesWithLock(ctx, testProjectID, testDocID); err != nil {
			t.Fatalf("applying the second update: %v", err)
		}
	})

	compareOutcomes(t, fromNode, fromGo)

	// The point of the test: the second insert must have moved past the first.
	if len(fromGo.Lines) == 0 || !strings.Contains(fromGo.Lines[0], "AAA") ||
		!strings.Contains(fromGo.Lines[0], "BBB") {
		t.Errorf("both inserts should be present: %q", fromGo.Lines)
	}
}

// waitForVersion waits for the Node dispatcher to have applied an update.
func (s *writePathSetup) waitForVersion(t *testing.T, want string) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		got, err := s.client.Get(ctx, rediskeys.Upstream.DocVersion(testDocID)).Result()
		if err == nil && got == want {
			// The version is written before the history queue entry, so give
			// the rest of the work a moment to land.
			time.Sleep(200 * time.Millisecond)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("the Node service never reached version %s", want)
}

func compareOutcomes(t *testing.T, node, goSvc outcome) {
	t.Helper()

	if len(node.Lines) != len(goSvc.Lines) {
		t.Fatalf("lines\n  node: %q\n  go:   %q", node.Lines, goSvc.Lines)
	}
	for i := range node.Lines {
		if node.Lines[i] != goSvc.Lines[i] {
			t.Errorf("line %d\n  node: %q\n  go:   %q", i, node.Lines[i], goSvc.Lines[i])
		}
	}
	if node.Version != goSvc.Version {
		t.Errorf("version: node %s, go %s", node.Version, goSvc.Version)
	}

	// The markers have to match exactly, ids included: the editor addresses a
	// comment thread by the id stored here.
	if node.Ranges != "" || goSvc.Ranges != "" {
		if !sameJSONIgnoring(nonEmptyJSON(node.Ranges), nonEmptyJSON(goSvc.Ranges),
			"changes.metadata.ts", "comments.metadata.ts") {
			t.Errorf("ranges\n  node: %s\n  go:   %s", node.Ranges, goSvc.Ranges)
		}
	}

	if len(node.DocOps) != len(goSvc.DocOps) {
		t.Fatalf("stored ops\n  node: %v\n  go:   %v", node.DocOps, goSvc.DocOps)
	}
	for i := range node.DocOps {
		if !sameJSONIgnoring(node.DocOps[i], goSvc.DocOps[i], "meta.ts", "meta.tsRT") {
			t.Errorf("stored op %d\n  node: %s\n  go:   %s", i, node.DocOps[i], goSvc.DocOps[i])
		}
	}

	if len(node.AppliedOps) != len(goSvc.AppliedOps) {
		t.Fatalf("published ops\n  node: %v\n  go:   %v", node.AppliedOps, goSvc.AppliedOps)
	}
	for i := range node.AppliedOps {
		a, _ := json.Marshal(node.AppliedOps[i])
		b, _ := json.Marshal(goSvc.AppliedOps[i])
		if !sameJSONIgnoring(string(a), string(b), "op.meta.ts", "op.meta.tsRT") {
			t.Errorf("published op %d\n  node: %s\n  go:   %s", i, a, b)
		}
	}

	if len(node.History) != len(goSvc.History) {
		t.Fatalf("history entries\n  node: %v\n  go:   %v", node.History, goSvc.History)
	}
	for i := range node.History {
		a, _ := json.Marshal(node.History[i])
		b, _ := json.Marshal(goSvc.History[i])
		if !sameJSONIgnoring(string(a), string(b), "meta.ts", "meta.tsRT") {
			t.Errorf("history entry %d\n  node: %s\n  go:   %s", i, a, b)
		}
	}
}

// sameJSONIgnoring compares two documents after removing fields that cannot
// match between two runs, such as the timestamps stamped as an update is
// applied.
func sameJSONIgnoring(a, b string, paths ...string) bool {
	var av, bv any
	if err := json.Unmarshal([]byte(a), &av); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(b), &bv); err != nil {
		return false
	}
	for _, path := range paths {
		removePath(av, strings.Split(path, "."))
		removePath(bv, strings.Split(path, "."))
	}
	ae, _ := json.Marshal(av)
	be, _ := json.Marshal(bv)
	return string(ae) == string(be)
}

// removePath deletes a field, descending through lists transparently so
// that "changes.metadata.ts" reaches every element of the changes array.
func removePath(value any, path []string) {
	if len(path) == 0 {
		return
	}
	if list, ok := value.([]any); ok {
		for _, element := range list {
			removePath(element, path)
		}
		return
	}
	object, ok := value.(map[string]any)
	if !ok {
		return
	}
	if len(path) == 1 {
		delete(object, path[0])
		return
	}
	removePath(object[path[0]], path[1:])
}

// The dispatcher is tested on its own, because the Node service's dispatcher is
// running throughout the tests above and would take the queue entry first.
func TestDispatcherConsumesTheQueue(t *testing.T) {
	addr := os.Getenv("DOCUPDATER_TEST_REDIS")
	if addr == "" {
		t.Skip("set DOCUPDATER_TEST_REDIS to run this")
	}

	web := newMockWeb(t)
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 7})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flushing redis: %v", err)
	}

	log := slog.New(slog.DiscardHandler)
	store := NewRedisStore(client, rediskeys.Upstream, testMaxDocLength, 0, log)
	persistence := NewPersistenceClient(web.URL, "overleaf", "password")
	locker := NewLocker(client, rediskeys.Upstream, 0)
	docs := NewDocumentManager(store, persistence, locker, log)
	bridge := NewRealTimeBridge(client, client, rediskeys.Upstream, false, log)
	history := NewHistoryQueue(client, rediskeys.Upstream)
	updates := NewUpdateManager(store, docs, locker, bridge, history,
		NewWebClient(web.URL, "overleaf", "password"), testMaxDocLength, log)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	NewDispatcher(client, updates, 10, log).Run(runCtx)

	update, err := json.Marshal(map[string]any{
		"doc":  testDocID,
		"op":   []map[string]any{{"p": 0, "i": "X"}},
		"v":    12,
		"meta": map[string]any{"source": "P.dispatch", "user_id": "u1"},
	})
	if err != nil {
		t.Fatalf("encoding update: %v", err)
	}
	if err := client.RPush(ctx, rediskeys.Upstream.PendingUpdates(testDocID), string(update)).Err(); err != nil {
		t.Fatalf("queueing the update: %v", err)
	}

	// Shard seven, to check that a worker exists for a shard other than the
	// first: real-time picks one at random, and a missing worker loses every
	// edit sent to it.
	if err := client.RPush(ctx, "pending-updates-list-7", testProjectID+":"+testDocID).Err(); err != nil {
		t.Fatalf("queueing the doc: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		version, err := client.Get(ctx, rediskeys.Upstream.DocVersion(testDocID)).Result()
		if err == nil && version == "13" {
			lines, _ := client.Get(ctx, rediskeys.Upstream.DocLines(testDocID)).Result()
			if !strings.HasPrefix(lines, `["X\\document`) {
				t.Errorf("the edit was not applied: %s", lines[:40])
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the dispatcher never applied the queued update")
}

// Inserted text is stripped of surrogate code units before it is applied, and
// that includes the two halves of a well-formed character outside the Basic
// Multilingual Plane: the regex the Node service uses matches code units, not
// characters, so typing an emoji stores two replacement characters instead.
//
// That is upstream's behaviour, not this port's, and it is checked here against
// the running Node service rather than asserted from reading the source,
// because it is surprising enough to be worth proving.
func TestSideBySideEmojiIsReplaced(t *testing.T) {
	s := newWritePathSetup(t)
	ctx := context.Background()

	update := map[string]any{
		"doc":  testDocID,
		"op":   []map[string]any{{"p": 0, "i": "a😀b"}},
		"v":    12,
		"meta": map[string]any{"source": "P.emoji", "user_id": "u1", "ts": 1700000000000},
	}

	if err := s.client.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flushing redis: %v", err)
	}
	s.loadDocViaNode(t)
	s.pushUpdate(t, update, true)
	s.waitForVersion(t, "13")
	fromNode := s.capture(t)

	if err := s.client.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flushing redis: %v", err)
	}
	s.loadDocViaGo(t)
	s.pushUpdate(t, update, false)
	if err := s.updates.ProcessOutstandingUpdatesWithLock(ctx, testProjectID, testDocID); err != nil {
		t.Fatalf("applying the update: %v", err)
	}
	fromGo := s.capture(t)

	compareOutcomes(t, fromNode, fromGo)

	if len(fromGo.Lines) == 0 || !strings.HasPrefix(fromGo.Lines[0], "a��b") {
		t.Errorf("first line = %q, want the emoji replaced by two replacement characters",
			fromGo.Lines[0])
	}
}

// nonEmptyJSON renders an empty stored value as an empty object, which is what
// both services read it back as.
func nonEmptyJSON(stored string) string {
	if stored == "" {
		return "{}"
	}
	return stored
}

// Tracked changes and comments have to survive the whole pipeline, not just the
// tracker: the ids the editor addresses a comment thread by are the ones stored
// here, so they are compared exactly.
func TestSideBySideTrackedChangesAndComments(t *testing.T) {
	s := newWritePathSetup(t)
	ctx := context.Background()

	// meta.tc turns track changes on and seeds the ids of the markers this edit
	// creates, so both services produce the same ones.
	tracked := map[string]any{
		"doc": testDocID,
		"op":  []map[string]any{{"p": 4, "i": "INSERTED"}},
		"v":   12,
		"meta": map[string]any{"source": "P.tc", "user_id": "u1", "ts": 1700000000000,
			"tc": "0123456789abcdef01"},
	}
	comment := map[string]any{
		"doc": testDocID,
		// The comment covers the text the previous update inserted, so it has to
		// match the document as it stands by then.
		"op":   []map[string]any{{"p": 4, "c": "INSERTED", "t": "thread-1"}},
		"v":    13,
		"meta": map[string]any{"source": "P.tc", "user_id": "u2", "ts": 1700000000001},
	}
	// An ordinary edit before both markers, which moves them along.
	shifting := map[string]any{
		"doc":  testDocID,
		"op":   []map[string]any{{"p": 0, "i": "XY"}},
		"v":    14,
		"meta": map[string]any{"source": "P.plain", "user_id": "u3", "ts": 1700000000002},
	}

	run := func(driver func()) outcome {
		if err := s.client.FlushDB(ctx).Err(); err != nil {
			t.Fatalf("flushing redis: %v", err)
		}
		ops := s.collectAppliedOps(t, driver)
		out := s.capture(t)
		out.AppliedOps = ops
		return out
	}

	fromNode := run(func() {
		s.loadDocViaNode(t)
		s.pushUpdate(t, tracked, true)
		s.waitForVersion(t, "13")
		s.pushUpdate(t, comment, true)
		s.waitForVersion(t, "14")
		s.pushUpdate(t, shifting, true)
		s.waitForVersion(t, "15")
	})
	fromGo := run(func() {
		s.loadDocViaGo(t)
		for _, update := range []map[string]any{tracked, comment, shifting} {
			s.pushUpdate(t, update, false)
			if err := s.updates.ProcessOutstandingUpdatesWithLock(ctx, testProjectID, testDocID); err != nil {
				t.Fatalf("applying an update: %v", err)
			}
		}
	})

	compareOutcomes(t, fromNode, fromGo)

	// The point of the test: both a tracked change and a comment should be
	// there, and they should have moved past the plain edit at the start.
	if !strings.Contains(fromGo.Ranges, "INSERTED") {
		t.Errorf("the tracked change was lost: %s", fromGo.Ranges)
	}
	if !strings.Contains(fromGo.Ranges, "thread-1") {
		t.Errorf("the comment was lost: %s", fromGo.Ranges)
	}
}
