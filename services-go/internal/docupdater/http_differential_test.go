package docupdater

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/rediskeys"
	"github.com/redis/go-redis/v9"
)

// The read API is compared by running both services side by side: the same
// mock web behind them, the same Redis under them, and the same requests put to
// each. Anything the Go service words differently -- a field name, a number
// rendered as a string, a status code -- shows up as a diff here rather than as
// a puzzled frontend later.
//
//	DOCUPDATER_TEST_REDIS=127.0.0.1:6399 \
//	DOCUPDATER_NODE_DIR=/build/services/document-updater \
//	  go test ./internal/docupdater/ -run SideBySide

const (
	testProjectID = "6a9eb3bc8a19695a4a6c97a7"
	testDocID     = "6a9eb3bd8a19695a4a6c97ac"
)

// mockWeb serves the one document both services will load.
type mockWeb struct {
	*httptest.Server
	lines            []string
	version          int64
	ranges           json.RawMessage
	pathname         string
	projectHistoryID string
	requests         int

	// writes are the documents written back, in order. A flush is judged by
	// what reached the database, which is the part that outlives Redis.
	mu     sync.Mutex
	writes []webWrite
}

// webWrite is one call to the write side of the web API.
type webWrite struct {
	Path string          `json:"path"`
	Body json.RawMessage `json:"body"`
}

// recordedWrites returns the writes so far.
func (m *mockWeb) recordedWrites() []webWrite {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]webWrite(nil), m.writes...)
}

// forgetWrites drops the record, so each side starts from nothing.
func (m *mockWeb) forgetWrites() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writes = nil
}

func newMockWeb(t *testing.T) *mockWeb {
	t.Helper()
	m := &mockWeb{
		lines:            []string{`\documentclass{article}`, "", "café 中文", `\end{document}`},
		version:          12,
		ranges:           json.RawMessage(`{}`),
		pathname:         "/main.tex",
		projectHistoryID: "history-1",
	}
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			m.mu.Lock()
			m.writes = append(m.writes, webWrite{Path: r.URL.Path, Body: body})
			m.mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		m.requests++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"lines": m.lines, "version": m.version, "ranges": m.ranges,
			"pathname": m.pathname, "projectHistoryId": m.projectHistoryID,
		})
	}))
	t.Cleanup(m.Close)
	return m
}

// startNodeService runs the real document-updater against the given web and
// Redis, and waits for it to answer.
func startNodeService(t *testing.T, nodeDir, webURL, redisAddr string) string {
	t.Helper()

	host, port, _ := strings.Cut(redisAddr, ":")
	web, err := url.Parse(webURL)
	if err != nil {
		t.Fatalf("parsing the mock web url: %v", err)
	}

	cmd := exec.Command("node", "app.js")
	cmd.Dir = nodeDir
	cmd.Env = append(os.Environ(),
		"WEB_API_HOST="+web.Hostname(),
		"WEB_API_PORT="+web.Port(),
		"WEB_HOST="+web.Hostname(),
		"WEB_PORT="+web.Port(),
		"DOC_UPDATER_REDIS_HOST="+host,
		"DOC_UPDATER_REDIS_PORT="+port,
		"REDIS_HOST="+host,
		"REDIS_PORT="+port,
		"LISTEN_ADDRESS=127.0.0.1",
		"LOG_LEVEL=fatal",
	)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr: %v", err)
	}
	cmd.Stdout = io.Discard
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot run node: %v", err)
	}
	go func() { _, _ = io.Copy(io.Discard, bufio.NewReader(stderr)) }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	base := "http://127.0.0.1:3003"
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		res, err := http.Get(base + "/status")
		if err == nil {
			_ = res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return base
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("the Node service never became ready on port 3003")
	return ""
}

// startGoService runs this implementation against the same web and Redis.
func startGoService(t *testing.T, webURL, redisAddr string) (string, *redis.Client) {
	t.Helper()

	client := redis.NewClient(&redis.Options{Addr: redisAddr})
	t.Cleanup(func() { _ = client.Close() })

	log := slog.New(slog.DiscardHandler)
	store := NewRedisStore(client, rediskeys.Upstream, testMaxDocLength, 0, log)
	persistence := NewPersistenceClient(webURL, "overleaf", "password")
	locker := NewLocker(client, rediskeys.Upstream, 0)
	history := NewHistoryQueue(client, rediskeys.Upstream)
	docs := NewDocumentManager(store, persistence, locker, history, nil, testMaxDocLength, log)
	project := NewProjectManager(store, docs, history, nil, log)

	server := httptest.NewServer(
		NewServer(docs, project, store, nil, testMaxDocLength, log).Handler(nil))
	t.Cleanup(server.Close)
	return server.URL, client
}

type sideBySide struct {
	t      *testing.T
	node   string
	goSvc  string
	client *redis.Client
}

func newSideBySide(t *testing.T) *sideBySide {
	t.Helper()
	addr := os.Getenv("DOCUPDATER_TEST_REDIS")
	nodeDir := os.Getenv("DOCUPDATER_NODE_DIR")
	if addr == "" || nodeDir == "" {
		t.Skip("set DOCUPDATER_TEST_REDIS and DOCUPDATER_NODE_DIR to run this")
	}

	web := newMockWeb(t)
	goURL, client := startGoService(t, web.URL, addr)
	nodeURL := startNodeService(t, nodeDir, web.URL, addr)
	return &sideBySide{t: t, node: nodeURL, goSvc: goURL, client: client}
}

// compare issues the same request to both services from a clean Redis and
// reports any difference in what they answer.
func (s *sideBySide) compare(name, method, path string) {
	s.t.Helper()
	s.t.Run(name, func(t *testing.T) {
		nodeStatus, nodeBody := s.request(t, s.node, method, path)
		goStatus, goBody := s.request(t, s.goSvc, method, path)

		if nodeStatus != goStatus {
			t.Fatalf("status: node %d, go %d\n  node body: %s\n  go body:   %s",
				nodeStatus, goStatus, nodeBody, goBody)
		}
		if nodeStatus >= 400 {
			// Error bodies are prose and are allowed to differ; the status is
			// the part clients act on.
			return
		}
		if !sameJSON(nodeBody, goBody) {
			t.Errorf("bodies differ\n  node: %s\n  go:   %s", nodeBody, goBody)
		}
	})
}

// request flushes Redis first, so each service is measured on the same starting
// state rather than on whatever the other one left behind.
func (s *sideBySide) request(t *testing.T, base, method, path string) (int, string) {
	t.Helper()
	if err := s.client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatalf("flushing redis: %v", err)
	}

	req, err := http.NewRequest(method, base+path, nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	return res.StatusCode, strings.TrimSpace(string(body))
}

func TestSideBySideReadAPI(t *testing.T) {
	s := newSideBySide(t)

	doc := fmt.Sprintf("/project/%s/doc/%s", testProjectID, testDocID)
	s.compare("get a doc", http.MethodGet, doc)
	s.compare("get a doc from a version", http.MethodGet, doc+"?fromVersion=12")
	s.compare("get a doc from an unavailable version", http.MethodGet, doc+"?fromVersion=1")
	s.compare("peek a doc that is not loaded", http.MethodGet, doc+"/peek")
	s.compare("ranges of a project with nothing loaded", http.MethodGet,
		"/project/"+testProjectID+"/ranges")
	s.compare("last updated at of a project with nothing loaded", http.MethodGet,
		"/project/"+testProjectID+"/last_updated_at")
	s.compare("clear project state", http.MethodPost,
		"/project/"+testProjectID+"/clearState")
	s.compare("status", http.MethodGet, "/status")
}

// Loading a document must leave Redis in the same state whichever service did
// it, because the other one may be the next to read it.
func TestSideBySideLoadLeavesTheSameState(t *testing.T) {
	s := newSideBySide(t)
	ctx := context.Background()
	doc := fmt.Sprintf("/project/%s/doc/%s", testProjectID, testDocID)

	snapshot := func(base string) map[string]string {
		if err := s.client.FlushDB(ctx).Err(); err != nil {
			t.Fatalf("flushing redis: %v", err)
		}
		status, body := s.request(t, base, http.MethodGet, doc)
		if status != http.StatusOK {
			t.Fatalf("loading the doc through %s: %d %s", base, status, body)
		}

		keys, err := s.client.Keys(ctx, "*").Result()
		if err != nil {
			t.Fatalf("listing keys: %v", err)
		}
		state := map[string]string{}
		for _, key := range keys {
			kind, err := s.client.Type(ctx, key).Result()
			if err != nil {
				t.Fatalf("typing %s: %v", key, err)
			}
			switch kind {
			case "string":
				state[key], _ = s.client.Get(ctx, key).Result()
			case "set":
				members, _ := s.client.SMembers(ctx, key).Result()
				state[key] = strings.Join(members, ",")
			case "list":
				items, _ := s.client.LRange(ctx, key, 0, -1).Result()
				state[key] = strings.Join(items, "\n")
			default:
				state[key] = kind
			}
		}
		return state
	}

	// The Go service is asked first, so a difference cannot be explained by
	// one of them having warmed something up for the other.
	fromGo := snapshot(s.goSvc)
	fromNode := snapshot(s.node)

	for key, want := range fromNode {
		got, ok := fromGo[key]
		if !ok {
			t.Errorf("the Go service did not write %s", key)
			continue
		}
		if got != want {
			t.Errorf("%s\n  node: %q\n  go:   %q", key, want, got)
		}
	}
	for key := range fromGo {
		if _, ok := fromNode[key]; !ok {
			t.Errorf("the Go service wrote %s, which the Node service does not", key)
		}
	}
}

// sameJSON compares two bodies as data rather than as text, so a difference in
// key order or in whitespace is not reported as a difference in behaviour.
func sameJSON(a, b string) bool {
	var av, bv any
	if err := json.Unmarshal([]byte(a), &av); err != nil {
		return a == b
	}
	if err := json.Unmarshal([]byte(b), &bv); err != nil {
		return false
	}
	ae, _ := json.Marshal(av)
	be, _ := json.Marshal(bv)
	return string(ae) == string(be)
}
