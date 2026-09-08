package docupdater

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/rediskeys"
	"github.com/redis/go-redis/v9"
)

// The Go store and the Node RedisManager have to agree byte for byte: the two
// run against the same Redis during a cut-over, and either can be the one that
// wrote what the other reads. Reading the source twice does not establish that,
// so these tests write with one implementation and read with the other, both
// ways round.
//
// They need a throwaway Redis and the Node service:
//
//	DOCUPDATER_TEST_REDIS=127.0.0.1:6399 \
//	DOCUPDATER_NODE_DIR=../../../services/document-updater \
//	  go test ./internal/docupdater/ -run Differential

const testMaxDocLength = 2 * 1024 * 1024

type harness struct {
	t      *testing.T
	stdin  io.WriteCloser
	stdout *bufio.Reader
	cmd    *exec.Cmd
	stderr *strings.Builder
}

type harnessResult struct {
	Doc    *nodeDoc          `json:"doc"`
	Ops    []json.RawMessage `json:"ops"`
	DocIDs []string          `json:"docIds"`
	Error  string            `json:"error"`
}

// nodeDoc is what RedisManager.getDoc returns.
type nodeDoc struct {
	Lines                []string        `json:"lines"`
	Version              int64           `json:"version"`
	Ranges               json.RawMessage `json:"ranges"`
	Pathname             string          `json:"pathname"`
	ProjectHistoryID     string          `json:"projectHistoryId"`
	UnflushedTime        string          `json:"unflushedTime"`
	LastUpdatedAt        string          `json:"lastUpdatedAt"`
	LastUpdatedBy        string          `json:"lastUpdatedBy"`
	HistoryRangesSupport bool            `json:"historyRangesSupport"`
	ResolvedCommentIDs   []string        `json:"resolvedCommentIds"`
}

func startHarness(t *testing.T, nodeDir, redisAddr string) *harness {
	t.Helper()

	host, port, ok := strings.Cut(redisAddr, ":")
	if !ok {
		t.Fatalf("DOCUPDATER_TEST_REDIS should be host:port, got %q", redisAddr)
	}

	script, err := filepath.Abs(filepath.Join("testdata", "redisharness.js"))
	if err != nil {
		t.Fatalf("locating the harness: %v", err)
	}
	cmd := exec.Command("node", script, nodeDir)
	cmd.Dir = nodeDir
	cmd.Env = append(os.Environ(),
		"DOC_UPDATER_REDIS_HOST="+host,
		"DOC_UPDATER_REDIS_PORT="+port,
		"REDIS_HOST="+host,
		"REDIS_PORT="+port,
		"LOG_LEVEL=fatal",
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout: %v", err)
	}
	stderr := &strings.Builder{}
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		t.Skipf("cannot run node: %v", err)
	}
	h := &harness{t: t, stdin: stdin, stdout: bufio.NewReaderSize(stdout, 1<<20), cmd: cmd, stderr: stderr}
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	})
	return h
}

func (h *harness) call(command map[string]any) harnessResult {
	h.t.Helper()
	encoded, err := json.Marshal(command)
	if err != nil {
		h.t.Fatalf("encoding command: %v", err)
	}
	if _, err := h.stdin.Write(append(encoded, '\n')); err != nil {
		h.t.Fatalf("writing to the harness: %v (stderr: %s)", err, h.stderr.String())
	}
	// @overleaf/settings announces which settings file it loaded on stdout, so
	// anything that is not a JSON object is a banner rather than a reply.
	var line []byte
	for {
		line, err = h.stdout.ReadBytes('\n')
		if err != nil {
			h.t.Fatalf("reading from the harness: %v (stderr: %s)", err, h.stderr.String())
		}
		if trimmed := bytes.TrimSpace(line); len(trimmed) > 0 && trimmed[0] == '{' {
			break
		}
	}
	var result harnessResult
	if err := json.Unmarshal(line, &result); err != nil {
		h.t.Fatalf("decoding harness output %q: %v", line, err)
	}
	if result.Error != "" {
		h.t.Fatalf("harness reported: %s", result.Error)
	}
	return result
}

func differentialSetup(t *testing.T) (*RedisStore, *harness, *redis.Client) {
	t.Helper()
	addr := os.Getenv("DOCUPDATER_TEST_REDIS")
	nodeDir := os.Getenv("DOCUPDATER_NODE_DIR")
	if addr == "" || nodeDir == "" {
		t.Skip("set DOCUPDATER_TEST_REDIS and DOCUPDATER_NODE_DIR to run this")
	}
	nodeDir, err := filepath.Abs(nodeDir)
	if err != nil {
		t.Fatalf("resolving DOCUPDATER_NODE_DIR: %v", err)
	}

	// Database zero, because that is the one the Node client connects to and
	// its settings do not expose a way to change it. The Redis this points at
	// has to be a throwaway one.
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	if err := client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatalf("cannot reach the test redis: %v", err)
	}

	// The harness runs with the services' own settings, which use the upstream
	// key schema, so the Go store must use the same one here.
	store := NewRedisStore(client, rediskeys.Upstream, testMaxDocLength, 0,
		slog.New(slog.DiscardHandler))
	return store, startHarness(t, nodeDir, addr), client
}

// A document written by Go must read back through the Node manager unchanged.
func TestDifferentialGoWritesNodeReads(t *testing.T) {
	store, node, _ := differentialSetup(t)
	ctx := context.Background()

	const projectID = "6a9eb3bc8a19695a4a6c97a7"
	const docID = "6a9eb3bd8a19695a4a6c97ac"
	lines := []string{`\documentclass{article}`, "", "café 中文 \U0001F600", `\end{document}`}
	ranges := json.RawMessage(`{"comments":[{"id":"c1","op":{"c":"café","p":3,"t":"t1"}}]}`)

	if err := store.PutDocInMemory(ctx, projectID, docID, lines, 42, ranges,
		[]string{"c1"}, "/main.tex", "history-1", true); err != nil {
		t.Fatalf("PutDocInMemory: %v", err)
	}

	got := node.call(map[string]any{"op": "get", "projectId": projectID, "docId": docID}).Doc
	if got == nil {
		t.Fatal("the Node manager found no document")
	}
	if len(got.Lines) != len(lines) {
		t.Fatalf("lines = %q, want %q", got.Lines, lines)
	}
	for i := range lines {
		if got.Lines[i] != lines[i] {
			t.Errorf("line %d = %q, want %q", i, got.Lines[i], lines[i])
		}
	}
	if got.Version != 42 {
		t.Errorf("version = %d, want 42", got.Version)
	}
	if got.Pathname != "/main.tex" {
		t.Errorf("pathname = %q", got.Pathname)
	}
	if got.ProjectHistoryID != "history-1" {
		t.Errorf("projectHistoryId = %q", got.ProjectHistoryID)
	}
	if !got.HistoryRangesSupport {
		t.Error("historyRangesSupport was lost")
	}
	if len(got.ResolvedCommentIDs) != 1 || got.ResolvedCommentIDs[0] != "c1" {
		t.Errorf("resolvedCommentIds = %q", got.ResolvedCommentIDs)
	}
	if !jsonEqual(t, got.Ranges, ranges) {
		t.Errorf("ranges = %s, want %s", got.Ranges, ranges)
	}
}

// And the other way round: a document the Node manager wrote must read back
// through the Go store unchanged, including the checksum it stored.
func TestDifferentialNodeWritesGoReads(t *testing.T) {
	store, node, client := differentialSetup(t)
	ctx := context.Background()

	const projectID = "6a9eb3bc8a19695a4a6c97a7"
	const docID = "6a9eb3bd8a19695a4a6c97ac"
	lines := []string{`\documentclass{article}`, "", "café 中文 \U0001F600", `\end{document}`}

	node.call(map[string]any{
		"op": "put", "projectId": projectID, "docId": docID,
		"lines": lines, "version": 7, "ranges": map[string]any{},
		"pathname": "/main.tex", "projectHistoryId": "history-1",
	})

	doc, err := store.GetDoc(ctx, projectID, docID)
	if err != nil {
		t.Fatalf("GetDoc: %v", err)
	}
	if !doc.Loaded() {
		t.Fatal("the Go store found no document")
	}
	if doc.Type() != TypeShareJSTextOT {
		t.Errorf("type = %s, want %s", doc.Type(), TypeShareJSTextOT)
	}
	for i := range lines {
		if doc.Lines[i] != lines[i] {
			t.Errorf("line %d = %q, want %q", i, doc.Lines[i], lines[i])
		}
	}
	if doc.Version != 7 {
		t.Errorf("version = %d, want 7", doc.Version)
	}
	if string(doc.Ranges) != "{}" {
		t.Errorf("ranges = %s, want an empty object", doc.Ranges)
	}

	// The checksum Node stored must be the one Go computes, or every read
	// through the Go store would log a corruption warning.
	stored, err := client.Get(ctx, rediskeys.Upstream.DocHash(docID)).Result()
	if err != nil {
		t.Fatalf("reading the stored hash: %v", err)
	}
	if computed := computeHash(string(doc.RawLines)); computed != stored {
		t.Errorf("hash = %s, Node stored %s", computed, stored)
	}
}

// An update written by Go must leave Redis in the state the Node manager would
// have left it in: the version, the checksum, and the operations kept for
// transforming later updates.
func TestDifferentialUpdateRoundTrip(t *testing.T) {
	store, node, _ := differentialSetup(t)
	ctx := context.Background()

	const projectID = "6a9eb3bc8a19695a4a6c97a7"
	const docID = "6a9eb3bd8a19695a4a6c97ac"

	node.call(map[string]any{
		"op": "put", "projectId": projectID, "docId": docID,
		"lines": []string{"hello"}, "version": 3, "ranges": map[string]any{},
		"pathname": "/main.tex", "projectHistoryId": "history-1",
	})

	op := json.RawMessage(`{"doc":"` + docID + `","op":[{"p":5,"i":" world"}],"v":3,` +
		`"meta":{"source":"P.abc","user_id":"u1","ts":1700000000000}}`)
	if err := store.UpdateDocument(ctx, projectID, docID,
		[]string{"hello world"}, 4, []json.RawMessage{op},
		json.RawMessage(`{}`), "u1"); err != nil {
		t.Fatalf("UpdateDocument: %v", err)
	}

	got := node.call(map[string]any{"op": "get", "projectId": projectID, "docId": docID}).Doc
	if got.Version != 4 {
		t.Errorf("version = %d, want 4", got.Version)
	}
	if len(got.Lines) != 1 || got.Lines[0] != "hello world" {
		t.Errorf("lines = %q", got.Lines)
	}
	if got.LastUpdatedBy != "u1" {
		t.Errorf("lastUpdatedBy = %q, want u1", got.LastUpdatedBy)
	}
	if got.UnflushedTime == "" {
		t.Error("unflushedTime was not set, so the document would never be written out")
	}

	// The Node manager must find the operation Go pushed, and find it at the
	// version it was applied at.
	ops := node.call(map[string]any{
		"op": "getPreviousDocOps", "docId": docID, "start": 3, "end": 4,
	}).Ops
	if len(ops) != 1 {
		t.Fatalf("Node read %d ops, want 1", len(ops))
	}
	if !jsonEqual(t, ops[0], op) {
		t.Errorf("op = %s, want %s", ops[0], op)
	}

	// And the reverse: an update written by Node must be readable by Go, with
	// the versions lining up.
	nodeOp := json.RawMessage(`{"doc":"` + docID + `","op":[{"p":11,"i":"!"}],"v":4,` +
		`"meta":{"source":"P.def","user_id":"u2","ts":1700000000001}}`)
	node.call(map[string]any{
		"op": "update", "projectId": projectID, "docId": docID,
		"lines": []string{"hello world!"}, "version": 5,
		"appliedOps": []json.RawMessage{nodeOp},
		"ranges":     map[string]any{}, "updateMeta": map[string]any{"user_id": "u2"},
	})

	goOps, err := store.GetPreviousDocOps(ctx, docID, 3, -1)
	if err != nil {
		t.Fatalf("GetPreviousDocOps: %v", err)
	}
	if len(goOps) != 2 {
		t.Fatalf("Go read %d ops, want 2", len(goOps))
	}
	if !jsonEqual(t, goOps[1], nodeOp) {
		t.Errorf("op = %s, want %s", goOps[1], nodeOp)
	}

	version, err := store.GetDocVersion(ctx, docID)
	if err != nil || version != 5 {
		t.Errorf("version = %d (%v), want 5", version, err)
	}

	// Asking for operations older than Redis holds must be refused rather than
	// answered with a short list, which would transform an update against the
	// wrong history.
	if _, err := store.GetPreviousDocOps(ctx, docID, 0, -1); err == nil {
		t.Error("asking for trimmed ops should have been refused")
	}
}

// Unloading a document must leave nothing behind, whichever implementation
// loaded it.
func TestDifferentialRemoveLeavesNothing(t *testing.T) {
	store, node, client := differentialSetup(t)
	ctx := context.Background()

	const projectID = "6a9eb3bc8a19695a4a6c97a7"
	const docID = "6a9eb3bd8a19695a4a6c97ac"

	node.call(map[string]any{
		"op": "put", "projectId": projectID, "docId": docID,
		"lines": []string{"hello"}, "version": 1, "ranges": map[string]any{},
		"pathname": "/main.tex", "projectHistoryId": "history-1",
		"historyRangesSupport": true,
	})
	if err := store.RemoveDocFromMemory(ctx, projectID, docID); err != nil {
		t.Fatalf("RemoveDocFromMemory: %v", err)
	}

	keys, err := client.Keys(ctx, "*"+docID+"*").Result()
	if err != nil {
		t.Fatalf("listing keys: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("keys left behind: %q", keys)
	}
	if members := node.call(map[string]any{
		"op": "docIdsInProject", "projectId": projectID,
	}).DocIDs; len(members) != 0 {
		t.Errorf("the document is still listed in the project: %q", members)
	}
	if in, err := client.SIsMember(ctx, rediskeys.Upstream.HistoryRangesSupport(), docID).Result(); err != nil || in {
		t.Error("the history ranges flag was left behind")
	}
}

// Most documents have no tracked changes or comments, and those are stored as
// an empty value rather than as "{}" so Redis is not filled with them. Both
// implementations have to agree on that, and on reading it back as an empty
// object.
func TestDifferentialEmptyRangesAreStoredEmpty(t *testing.T) {
	store, node, client := differentialSetup(t)
	ctx := context.Background()

	const projectID = "6a9eb3bc8a19695a4a6c97a7"
	const docID = "6a9eb3bd8a19695a4a6c97ac"

	if err := store.PutDocInMemory(ctx, projectID, docID, []string{"hello"}, 1,
		json.RawMessage(`{}`), nil, "/main.tex", "history-1", false); err != nil {
		t.Fatalf("PutDocInMemory: %v", err)
	}
	stored, err := client.Get(ctx, rediskeys.Upstream.Ranges(docID)).Result()
	if err != nil {
		t.Fatalf("reading ranges: %v", err)
	}
	if stored != "" {
		t.Errorf("empty ranges were stored as %q, want an empty value", stored)
	}

	got := node.call(map[string]any{"op": "get", "projectId": projectID, "docId": docID}).Doc
	if !jsonEqual(t, got.Ranges, json.RawMessage(`{}`)) {
		t.Errorf("Node read ranges back as %s, want an empty object", got.Ranges)
	}

	doc, err := store.GetDoc(ctx, projectID, docID)
	if err != nil {
		t.Fatalf("GetDoc: %v", err)
	}
	if string(doc.Ranges) != "{}" {
		t.Errorf("Go read ranges back as %s, want an empty object", doc.Ranges)
	}
}

// A document in one project must not be readable through another.
func TestDifferentialWrongProjectIsRefused(t *testing.T) {
	store, node, _ := differentialSetup(t)
	ctx := context.Background()

	node.call(map[string]any{
		"op": "put", "projectId": "6a9eb3bc8a19695a4a6c97a7",
		"docId": "6a9eb3bd8a19695a4a6c97ac",
		"lines": []string{"hello"}, "version": 1, "ranges": map[string]any{},
		"pathname": "/main.tex", "projectHistoryId": "history-1",
	})

	if _, err := store.GetDoc(ctx, "000000000000000000000000", "6a9eb3bd8a19695a4a6c97ac"); err == nil {
		t.Error("reading a document through the wrong project should be refused")
	}
}

func jsonEqual(t *testing.T, a, b json.RawMessage) bool {
	t.Helper()
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	ae, _ := json.Marshal(av)
	be, _ := json.Marshal(bv)
	return string(ae) == string(be)
}
