package docupdater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
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
	history *mockProjectHistory
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
	// The Node service has the project-history port built in, so the mock has
	// to listen on that one for both sides to reach the same place.
	projectHistory := newMockProjectHistory(t)
	historyClient := NewHistoryClient(projectHistory.URL, log)

	store := NewRedisStore(client, rediskeys.Upstream, testMaxDocLength, 0, log)
	persistence := NewPersistenceClient(web.URL, "overleaf", "password")
	locker := NewLocker(client, rediskeys.Upstream, 0)
	docs := NewDocumentManager(store, persistence, locker, historyClient, testMaxDocLength, log)
	bridge := NewRealTimeBridge(client, client, rediskeys.Upstream, false, log)
	history := NewHistoryQueue(client, rediskeys.Upstream)
	updates := NewUpdateManager(store, docs, locker, bridge, history,
		NewWebClient(web.URL, "overleaf", "password"), testMaxDocLength, log)
	docs.UseUpdateManager(updates)

	project := NewProjectManager(store, docs, historyClient, log)
	goServer := httptest.NewServer(
		NewServer(docs, project, store, historyClient, testMaxDocLength, log).Handler(nil))
	t.Cleanup(goServer.Close)

	nodeURL := startNodeService(t, nodeDir, web.URL, addr)
	return &writePathSetup{
		sideBySide: &sideBySide{t: t, node: nodeURL, goSvc: goServer.URL, client: client},
		updates:    updates, store: store, web: web, goBase: goServer.URL,
		history: projectHistory,
	}
}

// mockProjectHistory stands in for the project-history service and records the
// flushes asked of it.
type mockProjectHistory struct {
	*httptest.Server
	mu    sync.Mutex
	calls []string
}

// projectHistoryPort is where the Node service looks for project-history. Only
// the host is configurable there, so the mock takes the port.
const projectHistoryPort = "127.0.0.1:3054"

func newMockProjectHistory(t *testing.T) *mockProjectHistory {
	t.Helper()
	m := &mockProjectHistory{}
	listener, err := net.Listen("tcp", projectHistoryPort)
	if err != nil {
		t.Skipf("cannot listen on %s for the project-history mock: %v", projectHistoryPort, err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.calls = append(m.calls, r.Method+" "+r.URL.RequestURI())
		m.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	_ = server.Listener.Close()
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	m.Server = server
	return m
}

func (m *mockProjectHistory) recordedCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.calls...)
}

func (m *mockProjectHistory) forgetCalls() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = nil
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
	// The Node service has the project-history port built in, so the mock has
	// to listen on that one for both sides to reach the same place.
	projectHistory := newMockProjectHistory(t)
	historyClient := NewHistoryClient(projectHistory.URL, log)

	store := NewRedisStore(client, rediskeys.Upstream, testMaxDocLength, 0, log)
	persistence := NewPersistenceClient(web.URL, "overleaf", "password")
	locker := NewLocker(client, rediskeys.Upstream, 0)
	docs := NewDocumentManager(store, persistence, locker, historyClient, testMaxDocLength, log)
	bridge := NewRealTimeBridge(client, client, rediskeys.Upstream, false, log)
	history := NewHistoryQueue(client, rediskeys.Upstream)
	updates := NewUpdateManager(store, docs, locker, bridge, history,
		NewWebClient(web.URL, "overleaf", "password"), testMaxDocLength, log)
	docs.UseUpdateManager(updates)

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

// flushState is what a flush left behind: what was written back to the
// database, and what is left in Redis afterwards.
type flushState struct {
	Writes        []webWrite
	HistoryCalls  []string
	UnflushedTime string
	DocLines      string
	DocsInProject []string
	Status        int
}

func (s *writePathSetup) captureFlush(t *testing.T, status int) flushState {
	t.Helper()
	ctx := context.Background()
	keys := rediskeys.Upstream

	// A flush of the history queue may be made in the background on either
	// side, so this waits briefly for one to arrive rather than reading
	// whatever happens to have landed by now.
	deadline := time.Now().Add(time.Second)
	for len(s.history.recordedCalls()) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}

	state := flushState{
		Writes: s.web.recordedWrites(), HistoryCalls: s.history.recordedCalls(),
		Status: status,
	}
	state.UnflushedTime, _ = s.client.Get(ctx, keys.UnflushedTime(testDocID)).Result()
	state.DocLines, _ = s.client.Get(ctx, keys.DocLines(testDocID)).Result()
	state.DocsInProject, _ = s.client.SMembers(ctx, keys.DocsInProject(testProjectID)).Result()
	return state
}

// compareFlush reports any difference in what the two services wrote back and
// what they left behind.
func compareFlush(t *testing.T, node, goSvc flushState) {
	t.Helper()

	if node.Status != goSvc.Status {
		t.Errorf("status: node %d, go %d", node.Status, goSvc.Status)
	}
	if len(node.Writes) != len(goSvc.Writes) {
		t.Fatalf("writes to web\n  node: %d %v\n  go:   %d %v",
			len(node.Writes), node.Writes, len(goSvc.Writes), goSvc.Writes)
	}
	for i := range node.Writes {
		if node.Writes[i].Path != goSvc.Writes[i].Path {
			t.Errorf("write %d path: node %s, go %s", i,
				node.Writes[i].Path, goSvc.Writes[i].Path)
			continue
		}
		// lastUpdatedAt is when the edit was applied, which cannot match
		// between two runs.
		if !sameJSONIgnoring(string(node.Writes[i].Body), string(goSvc.Writes[i].Body),
			"lastUpdatedAt") {
			t.Errorf("write %d body\n  node: %s\n  go:   %s", i,
				node.Writes[i].Body, goSvc.Writes[i].Body)
		}
	}
	if strings.Join(node.HistoryCalls, ",") != strings.Join(goSvc.HistoryCalls, ",") {
		t.Errorf("calls to project-history\n  node: %v\n  go:   %v",
			node.HistoryCalls, goSvc.HistoryCalls)
	}
	// Only whether there is one: the value is when the document was modified,
	// which is a different moment for each of the two runs.
	if (node.UnflushedTime == "") != (goSvc.UnflushedTime == "") {
		t.Errorf("unflushed time: node %q, go %q", node.UnflushedTime, goSvc.UnflushedTime)
	}
	if node.DocLines != goSvc.DocLines {
		t.Errorf("doc lines left in redis\n  node: %s\n  go:   %s",
			node.DocLines, goSvc.DocLines)
	}
	if strings.Join(node.DocsInProject, ",") != strings.Join(goSvc.DocsInProject, ",") {
		t.Errorf("docs in project: node %v, go %v", node.DocsInProject, goSvc.DocsInProject)
	}
}

// edit is the update both sides are given before a flush, so there is something
// unsaved for the flush to write.
func flushTestUpdate() map[string]any {
	return map[string]any{
		"doc":  testDocID,
		"op":   []map[string]any{{"p": 23, "i": " % edited"}},
		"v":    12,
		"meta": map[string]any{"source": "P.testsource", "user_id": "u1", "ts": 1700000000000},
	}
}

// runFlushCase drives one HTTP call against each service, from a clean Redis
// and with an edit applied first, and compares what each left behind.
func (s *writePathSetup) runFlushCase(t *testing.T, name, method, path string) {
	t.Helper()
	s.t.Run(name, func(t *testing.T) {
		ctx := context.Background()

		fromNode := func() flushState {
			if err := s.client.FlushDB(ctx).Err(); err != nil {
				t.Fatalf("flushing redis: %v", err)
			}
			s.web.forgetWrites()
			s.history.forgetCalls()
			s.loadDocViaNode(t)
			s.pushUpdate(t, flushTestUpdate(), true)
			s.waitForVersion(t, "13")
			status, _ := s.request(t, s.node, method, path)
			return s.captureFlush(t, status)
		}()

		fromGo := func() flushState {
			if err := s.client.FlushDB(ctx).Err(); err != nil {
				t.Fatalf("flushing redis: %v", err)
			}
			s.web.forgetWrites()
			s.history.forgetCalls()
			s.loadDocViaGo(t)
			s.pushUpdate(t, flushTestUpdate(), false)
			if err := s.updates.ProcessOutstandingUpdatesWithLock(ctx, testProjectID, testDocID); err != nil {
				t.Fatalf("applying the update: %v", err)
			}
			status, _ := s.request(t, s.goSvc, method, path)
			return s.captureFlush(t, status)
		}()

		compareFlush(t, fromNode, fromGo)
	})
}

// A flush is where an edit stops being something only this service knows about.
// Whichever implementation performs it, the same document has to reach the
// database and the same state has to be left in Redis, because the other one
// may be what reads it next.
func TestSideBySideFlushAndDelete(t *testing.T) {
	s := newWritePathSetup(t)

	doc := fmt.Sprintf("/project/%s/doc/%s", testProjectID, testDocID)
	project := "/project/" + testProjectID

	s.runFlushCase(t, "flush a doc", http.MethodPost, doc+"/flush")
	s.runFlushCase(t, "delete a doc", http.MethodDelete, doc)
	s.runFlushCase(t, "delete a doc ignoring flush errors", http.MethodDelete,
		doc+"?ignore_flush_errors=true")
	s.runFlushCase(t, "flush a project", http.MethodPost, project+"/flush")
	s.runFlushCase(t, "delete a project in the background", http.MethodDelete,
		project+"?background=true")
	s.runFlushCase(t, "delete a project", http.MethodDelete, project)
	s.runFlushCase(t, "delete a project on shutdown", http.MethodDelete,
		project+"?shutdown=true")
	s.runFlushCase(t, "block a project", http.MethodPost, project+"/block")
	s.runFlushCase(t, "unblock a project", http.MethodPost, project+"/unblock")
}

// Flushing a document that has not been edited must not write it back: the copy
// in the database is already the current one, and writing it again would make a
// revision out of nothing.
func TestSideBySideFlushWithoutChangesWritesNothing(t *testing.T) {
	s := newWritePathSetup(t)
	ctx := context.Background()
	path := fmt.Sprintf("/project/%s/doc/%s/flush", testProjectID, testDocID)

	run := func(base string, load func(*testing.T)) flushState {
		if err := s.client.FlushDB(ctx).Err(); err != nil {
			t.Fatalf("flushing redis: %v", err)
		}
		s.web.forgetWrites()
		s.history.forgetCalls()
		load(t)
		status, _ := s.request(t, base, http.MethodPost, path)
		return s.captureFlush(t, status)
	}

	fromNode := run(s.node, s.loadDocViaNode)
	fromGo := run(s.goSvc, s.loadDocViaGo)

	compareFlush(t, fromNode, fromGo)
	if len(fromGo.Writes) != 0 {
		t.Errorf("an unmodified doc was written back: %v", fromGo.Writes)
	}
}

// postJSON sends a request with a JSON body, which the write routes need and
// the read-only helper does not do.
func (s *writePathSetup) postJSON(t *testing.T, base, method, path string, body any) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encoding the body: %v", err)
		}
		reader = strings.NewReader(string(encoded))
	}
	req, err := http.NewRequest(method, base+path, reader)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = res.Body.Close() }()
	answer, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	return res.StatusCode, strings.TrimSpace(string(answer))
}

// writeOutcome is everything a write through the HTTP API produced.
type writeOutcome struct {
	flushState
	Body       string
	AppliedOps []map[string]any
	History    []map[string]any
	Lines      []string
	Version    string
	Ranges     string
}

// runWriteCase puts the same write to both services from the same starting
// state and compares everything each of them produced.
//
// preload says whether the document is already in Redis when the write arrives,
// which is the difference between a document somebody has open and one nobody
// does. The two take different branches: the first is flushed and left, the
// second is flushed and dropped.
func (s *writePathSetup) runWriteCase(t *testing.T, name, method, path string, body any, preload bool) {
	t.Helper()
	s.t.Run(name, func(t *testing.T) {
		ctx := context.Background()

		run := func(base string, load func(*testing.T)) writeOutcome {
			if err := s.client.FlushDB(ctx).Err(); err != nil {
				t.Fatalf("flushing redis: %v", err)
			}
			s.web.forgetWrites()
			s.history.forgetCalls()
			if preload {
				load(t)
			}

			var status int
			var answer string
			ops := s.collectAppliedOps(t, func() {
				status, answer = s.postJSON(t, base, method, path, body)
			})

			out := writeOutcome{Body: answer, AppliedOps: ops}
			out.flushState = s.captureFlush(t, status)
			captured := s.capture(t)
			out.History = captured.History
			out.Lines = captured.Lines
			out.Version = captured.Version
			return out
		}

		fromNode := run(s.node, s.loadDocViaNode)
		fromGo := run(s.goSvc, s.loadDocViaGo)

		compareFlush(t, fromNode.flushState, fromGo.flushState)
		if !sameJSON(fromNode.Body, fromGo.Body) {
			t.Errorf("response\n  node: %s\n  go:   %s", fromNode.Body, fromGo.Body)
		}
		if strings.Join(fromNode.Lines, "\n") != strings.Join(fromGo.Lines, "\n") {
			t.Errorf("doc left in redis\n  node: %q\n  go:   %q",
				fromNode.Lines, fromGo.Lines)
		}
		if fromNode.Version != fromGo.Version {
			t.Errorf("version: node %s, go %s", fromNode.Version, fromGo.Version)
		}

		if len(fromNode.AppliedOps) != len(fromGo.AppliedOps) {
			t.Fatalf("published ops\n  node: %v\n  go:   %v",
				fromNode.AppliedOps, fromGo.AppliedOps)
		}
		for i := range fromNode.AppliedOps {
			a, _ := json.Marshal(fromNode.AppliedOps[i])
			b, _ := json.Marshal(fromGo.AppliedOps[i])
			if !sameJSONIgnoring(string(a), string(b), "op.meta.ts", "op.meta.tsRT") {
				t.Errorf("published op %d\n  node: %s\n  go:   %s", i, a, b)
			}
		}
		if len(fromNode.History) != len(fromGo.History) {
			t.Fatalf("history entries\n  node: %v\n  go:   %v",
				fromNode.History, fromGo.History)
		}
		for i := range fromNode.History {
			a, _ := json.Marshal(fromNode.History[i])
			b, _ := json.Marshal(fromGo.History[i])
			if !sameJSONIgnoring(string(a), string(b), "meta.ts", "meta.tsRT") {
				t.Errorf("history entry %d\n  node: %s\n  go:   %s", i, a, b)
			}
		}
	})
}

// A write through the API is turned into an edit by diffing against what is
// already there. The two services have to find the same difference: it is what
// the other editors are told, what the tracked changes move against, and what
// the history records.
func TestSideBySideSetDoc(t *testing.T) {
	s := newWritePathSetup(t)

	doc := fmt.Sprintf("/project/%s/doc/%s", testProjectID, testDocID)
	original := []string{`\documentclass{article}`, "", "café 中文", `\end{document}`}

	edited := append([]string(nil), original...)
	edited[2] = "café 中文 with more text"

	rewritten := []string{`\documentclass{report}`, "", "an entirely different line",
		"and another", `\end{document}`}

	shorter := []string{`\documentclass{article}`, `\end{document}`}

	for _, preloaded := range []bool{true, false} {
		suffix := " (loaded)"
		if !preloaded {
			suffix = " (not loaded)"
		}
		s.runWriteCase(t, "an edit in the middle"+suffix, http.MethodPost, doc,
			map[string]any{"lines": edited, "source": "dropbox", "user_id": "u1"}, preloaded)
		s.runWriteCase(t, "a rewrite"+suffix, http.MethodPost, doc,
			map[string]any{"lines": rewritten, "source": "dropbox", "user_id": "u1"}, preloaded)
		s.runWriteCase(t, "lines removed"+suffix, http.MethodPost, doc,
			map[string]any{"lines": shorter, "source": "dropbox", "user_id": "u1"}, preloaded)
		s.runWriteCase(t, "no change at all"+suffix, http.MethodPost, doc,
			map[string]any{"lines": original, "source": "dropbox", "user_id": "u1"}, preloaded)
		s.runWriteCase(t, "an undo"+suffix, http.MethodPost, doc,
			map[string]any{"lines": edited, "source": "dropbox", "user_id": "u1",
				"undoing": true}, preloaded)
		s.runWriteCase(t, "an origin rather than a source"+suffix, http.MethodPost, doc,
			map[string]any{"lines": edited, "user_id": "u1",
				"origin": map[string]any{"kind": "file-restore"}}, preloaded)
		s.runWriteCase(t, "no source at all"+suffix, http.MethodPost, doc,
			map[string]any{"lines": edited, "user_id": "u1"}, preloaded)
		s.runWriteCase(t, "no user"+suffix, http.MethodPost, doc,
			map[string]any{"lines": edited, "source": "dropbox"}, preloaded)

		s.runWriteCase(t, "append a line"+suffix, http.MethodPost, doc+"/append",
			map[string]any{"lines": []string{"appended"}, "source": "dropbox",
				"user_id": "u1"}, preloaded)
		s.runWriteCase(t, "append nothing"+suffix, http.MethodPost, doc+"/append",
			map[string]any{"lines": []string{}, "source": "dropbox", "user_id": "u1"}, preloaded)
	}
}

// The diff has to be found the same way on text that gives it something to do:
// repeated words, moved lines, and characters outside the basic plane, where
// counting in the wrong unit puts every later position out by one.
func TestSideBySideSetDocDiffs(t *testing.T) {
	s := newWritePathSetup(t)
	doc := fmt.Sprintf("/project/%s/doc/%s", testProjectID, testDocID)

	cases := []struct {
		name  string
		lines []string
	}{
		{"a word repeated", []string{`\documentclass{article}`, "", "café 中文 café café", `\end{document}`}},
		{"lines swapped", []string{"", `\documentclass{article}`, `\end{document}`, "café 中文"}},
		{"everything replaced", []string{"nothing", "in", "common"}},
		{"emptied", []string{""}},
		{"one long line", []string{strings.Repeat("the quick brown fox. ", 200)}},
		{"astral characters", []string{`\documentclass{article}`, "", "café 中文 🎉🎉 tail", `\end{document}`}},
		{"a line split in two", []string{`\documentclass{article}`, "", "café", "中文", `\end{document}`}},
		{"leading whitespace changed", []string{`  \documentclass{article}`, "", "  café 中文", `\end{document}`}},
	}
	for _, c := range cases {
		s.runWriteCase(t, c.name, http.MethodPost, doc,
			map[string]any{"lines": c.lines, "source": "dropbox", "user_id": "u1"}, true)
	}
}

// seedMarkers puts a tracked change and a comment into the document, by making
// the edits that create them rather than by writing the markers directly, so
// they are exactly what the running service would have produced.
//
// It returns the id of the tracked change, which the accept and reject routes
// address it by.
func (s *writePathSetup) seedMarkers(t *testing.T, base string, viaNode bool) string {
	t.Helper()
	ctx := context.Background()

	tracked := map[string]any{
		"doc": testDocID,
		"op":  []map[string]any{{"p": 4, "i": "INSERTED"}},
		"v":   12,
		"meta": map[string]any{"source": "P.tc", "user_id": "u1", "ts": 1700000000000,
			"tc": "0123456789abcdef01"},
	}
	comment := map[string]any{
		"doc":  testDocID,
		"op":   []map[string]any{{"p": 4, "c": "INSERTED", "t": "thread-1"}},
		"v":    13,
		"meta": map[string]any{"source": "P.tc", "user_id": "u2", "ts": 1700000000001},
	}

	if viaNode {
		s.loadDocViaNode(t)
		s.pushUpdate(t, tracked, true)
		s.waitForVersion(t, "13")
		s.pushUpdate(t, comment, true)
		s.waitForVersion(t, "14")
	} else {
		s.loadDocViaGo(t)
		for _, update := range []map[string]any{tracked, comment} {
			s.pushUpdate(t, update, false)
			if err := s.updates.ProcessOutstandingUpdatesWithLock(ctx, testProjectID, testDocID); err != nil {
				t.Fatalf("applying an update: %v", err)
			}
		}
	}

	// The id is generated from the seed, so both sides produce the same one;
	// reading it back rather than assuming it keeps the test honest.
	stored, err := s.client.Get(ctx, rediskeys.Upstream.Ranges(testDocID)).Result()
	if err != nil {
		t.Fatalf("reading the ranges back: %v", err)
	}
	var ranges struct {
		Changes []struct {
			ID string `json:"id"`
		} `json:"changes"`
	}
	if err := json.Unmarshal([]byte(stored), &ranges); err != nil {
		t.Fatalf("decoding the ranges: %v", err)
	}
	if len(ranges.Changes) != 1 {
		t.Fatalf("expected one tracked change, got %s", stored)
	}
	return ranges.Changes[0].ID
}

// runMarkerCase drives one call against each service on a document that has a
// tracked change and a comment in it, and compares everything left behind.
func (s *writePathSetup) runMarkerCase(t *testing.T, name, method string,
	path func(changeID string) string, body any) {
	t.Helper()
	s.t.Run(name, func(t *testing.T) {
		ctx := context.Background()

		run := func(base string, viaNode bool) writeOutcome {
			if err := s.client.FlushDB(ctx).Err(); err != nil {
				t.Fatalf("flushing redis: %v", err)
			}
			s.web.forgetWrites()
			s.history.forgetCalls()
			changeID := s.seedMarkers(t, base, viaNode)

			var status int
			var answer string
			ops := s.collectAppliedOps(t, func() {
				status, answer = s.postJSON(t, base, method, path(changeID), body)
			})

			out := writeOutcome{Body: answer, AppliedOps: ops}
			out.flushState = s.captureFlush(t, status)
			captured := s.capture(t)
			out.History = captured.History
			out.Lines = captured.Lines
			out.Version = captured.Version
			out.Ranges = captured.Ranges
			return out
		}

		fromNode := run(s.node, true)
		fromGo := run(s.goSvc, false)

		compareFlush(t, fromNode.flushState, fromGo.flushState)
		// Error bodies are prose and are allowed to differ; the status is the
		// part callers act on, and compareFlush has already checked it. The
		// timestamp on a marker is stamped as it is created, so it cannot
		// match between two runs either.
		if fromNode.Status < 400 &&
			!sameJSONIgnoring(nonEmptyJSON(fromNode.Body), nonEmptyJSON(fromGo.Body),
				"metadata.ts") {
			t.Errorf("response\n  node: %s\n  go:   %s", fromNode.Body, fromGo.Body)
		}
		if strings.Join(fromNode.Lines, "\n") != strings.Join(fromGo.Lines, "\n") {
			t.Errorf("doc\n  node: %q\n  go:   %q", fromNode.Lines, fromGo.Lines)
		}
		if fromNode.Version != fromGo.Version {
			t.Errorf("version: node %s, go %s", fromNode.Version, fromGo.Version)
		}
		if !sameJSONIgnoring(nonEmptyJSON(fromNode.Ranges), nonEmptyJSON(fromGo.Ranges),
			"changes.metadata.ts", "comments.metadata.ts") {
			t.Errorf("ranges\n  node: %s\n  go:   %s", fromNode.Ranges, fromGo.Ranges)
		}
		if len(fromNode.AppliedOps) != len(fromGo.AppliedOps) {
			t.Fatalf("published ops\n  node: %v\n  go:   %v",
				fromNode.AppliedOps, fromGo.AppliedOps)
		}
		for i := range fromNode.AppliedOps {
			a, _ := json.Marshal(fromNode.AppliedOps[i])
			b, _ := json.Marshal(fromGo.AppliedOps[i])
			if !sameJSONIgnoring(string(a), string(b), "op.meta.ts", "op.meta.tsRT") {
				t.Errorf("published op %d\n  node: %s\n  go:   %s", i, a, b)
			}
		}
		if len(fromNode.History) != len(fromGo.History) {
			t.Fatalf("history entries\n  node: %v\n  go:   %v",
				fromNode.History, fromGo.History)
		}
		for i := range fromNode.History {
			a, _ := json.Marshal(fromNode.History[i])
			b, _ := json.Marshal(fromGo.History[i])
			if !sameJSONIgnoring(string(a), string(b), "meta.ts", "meta.tsRT") {
				t.Errorf("history entry %d\n  node: %s\n  go:   %s", i, a, b)
			}
		}
	})
}

// Accepting a change only removes its marker, while rejecting one puts the text
// back the way it was and is seen by everyone with the document open. The two
// are easy to get the wrong way round, and the difference is somebody losing
// their edit.
func TestSideBySideChangesAndComments(t *testing.T) {
	s := newWritePathSetup(t)

	doc := fmt.Sprintf("/project/%s/doc/%s", testProjectID, testDocID)
	fixed := func(path string) func(string) string {
		return func(string) string { return path }
	}

	s.runMarkerCase(t, "accept a change by id", http.MethodPost,
		func(changeID string) string { return doc + "/change/" + changeID + "/accept" }, nil)
	s.runMarkerCase(t, "accept a list of changes", http.MethodPost,
		fixed(doc+"/change/accept"),
		map[string]any{"change_ids": []string{"0123456789abcdef01000001"}})
	s.runMarkerCase(t, "accept a change that is not there", http.MethodPost,
		fixed(doc+"/change/accept"), map[string]any{"change_ids": []string{"missing"}})
	s.runMarkerCase(t, "reject a change", http.MethodPost,
		fixed(doc+"/change/reject"),
		map[string]any{"change_ids": []string{"0123456789abcdef01000001"}, "user_id": "u3"})
	s.runMarkerCase(t, "reject a change that is not there", http.MethodPost,
		fixed(doc+"/change/reject"),
		map[string]any{"change_ids": []string{"missing"}, "user_id": "u3"})

	s.runMarkerCase(t, "get a comment", http.MethodGet,
		fixed(doc+"/comment/thread-1"), nil)
	s.runMarkerCase(t, "get a comment that is not there", http.MethodGet,
		fixed(doc+"/comment/thread-missing"), nil)
	s.runMarkerCase(t, "resolve a comment", http.MethodPost,
		fixed(doc+"/comment/thread-1/resolve"), map[string]any{"user_id": "u3"})
	s.runMarkerCase(t, "reopen a comment", http.MethodPost,
		fixed(doc+"/comment/thread-1/reopen"), map[string]any{"user_id": "u3"})
	s.runMarkerCase(t, "delete a comment", http.MethodDelete,
		fixed(doc+"/comment/thread-1"), map[string]any{"user_id": "u3"})
	s.runMarkerCase(t, "delete a comment that is not there", http.MethodDelete,
		fixed(doc+"/comment/thread-missing"), map[string]any{"user_id": "u3"})
}
