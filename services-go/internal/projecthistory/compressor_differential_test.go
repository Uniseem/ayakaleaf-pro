package projecthistory

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

// Somebody typing a sentence produces one update per keystroke, and the history
// records one entry for the sentence. Which keystrokes get merged into which
// entry is what this decides, and it is the shape of the history everybody
// reads afterwards: a paste that changed one word should not look like the
// passage being rewritten, and two people typing in the same place should not
// come out as one person.
//
// So the compressor is compared against the real one over random queues.
//
//	PROJECT_HISTORY_DIR=/build/services/project-history \
//	  go test ./internal/projecthistory/ -run CompressorDifferential

var compressorCases = envInt("COMPRESSOR_CASES", 20000)

func envInt(name string, fallback int) int {
	if raw := os.Getenv(name); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			return value
		}
	}
	return fallback
}

type compressorScenario struct {
	Updates []json.RawMessage `json:"updates"`
}

type compressorResult struct {
	Compressed json.RawMessage `json:"compressed"`
	Error      string          `json:"error"`
}

func TestCompressorDifferentialAgainstNode(t *testing.T) {
	serviceDir := os.Getenv("PROJECT_HISTORY_DIR")
	if serviceDir == "" {
		t.Skip("set PROJECT_HISTORY_DIR to the project-history service to run this")
	}
	serviceDir, err := filepath.Abs(serviceDir)
	if err != nil {
		t.Fatalf("resolving PROJECT_HISTORY_DIR: %v", err)
	}

	scenarios := generateCompressorScenarios(compressorCases)
	results := runCompressorHarness(t, serviceDir, scenarios)
	if len(results) != len(scenarios) {
		t.Fatalf("the harness returned %d results for %d scenarios",
			len(results), len(scenarios))
	}

	var compared int
	for i, s := range scenarios {
		want := results[i]
		encoded, _ := json.Marshal(s)

		updates := make([]*Update, 0, len(s.Updates))
		var parseErr error
		for _, raw := range s.Updates {
			var update Update
			if err := json.Unmarshal(raw, &update); err != nil {
				parseErr = err
				break
			}
			updates = append(updates, &update)
		}
		if parseErr != nil {
			t.Fatalf("case %d: the port could not read the queue: %v\n  %s",
				i, parseErr, encoded)
		}

		compressed, err := CompressRawUpdates(updates)
		if want.Error != "" {
			if err == nil {
				t.Fatalf("case %d: the real compressor refused this (%s), the port did not\n  %s",
					i, want.Error, encoded)
			}
			continue
		}
		if err != nil {
			t.Fatalf("case %d: the port refused this: %v\n  %s", i, err, encoded)
		}
		compared++

		got, err := json.Marshal(nonNilUpdates(compressed))
		if err != nil {
			t.Fatalf("case %d: encoding: %v", i, err)
		}
		if !sameJSON(string(want.Compressed), string(got)) {
			t.Fatalf("case %d\n  queue: %s\n  node: %s\n  go:   %s",
				i, encoded, want.Compressed, got)
		}
	}

	if compared == 0 {
		t.Fatal("every scenario was refused by both; the generator produces nothing usable")
	}
	t.Logf("%d of %d queues were compressed by both", compared, len(scenarios))
}

// nonNilUpdates renders an empty result as an empty list rather than null,
// which is what the other side produces.
func nonNilUpdates(updates []*Update) []*Update {
	if updates == nil {
		return []*Update{}
	}
	return updates
}

// generateCompressorScenarios builds queues of updates over one document.
//
// They are generated as a person editing would produce them -- successive
// operations against a running document length, mostly by the same user and
// close together in time -- so that the merging rules are actually reached.
// Every so often one of the things that stops a merge is thrown in.
func generateCompressorScenarios(count int) []compressorScenario {
	random := rand.New(rand.NewPCG(23, 47))

	scenarios := make([]compressorScenario, 0, count)
	for len(scenarios) < count {
		document := randomText(random, 10+random.IntN(30))
		docLength := len(document)
		version := int64(random.IntN(10))
		timestamp := int64(1700000000000)

		updates := []json.RawMessage{}
		for u := 1 + random.IntN(6); u > 0; u-- {
			update, newLength := randomUpdate(random, document, docLength, version, timestamp)
			if update == nil {
				continue
			}
			updates = append(updates, update)
			docLength = newLength
			version++
			// Usually the next keystroke, sometimes long enough after to stop
			// a merge.
			timestamp += int64(random.IntN(200))
			if random.IntN(10) == 0 {
				timestamp += maxTimeBetweenUpdates + 1
			}
		}
		scenarios = append(scenarios, compressorScenario{Updates: updates})
	}
	return scenarios
}

// randomUpdate builds one update, and reports how long the document is after
// it.
func randomUpdate(random *rand.Rand, document string, docLength int,
	version, timestamp int64) (json.RawMessage, int) {

	meta := map[string]any{
		"ts": timestamp, "user_id": fmt.Sprintf("user-%d", random.IntN(2)),
		"pathname": "/main.tex", "doc_length": docLength,
	}
	// The things that stop a merge, thrown in often enough to be reached.
	switch random.IntN(12) {
	case 0:
		meta["tc"] = "0123456789abcdef01"
	case 1:
		meta["type"] = "external"
		meta["source"] = fmt.Sprintf("dropbox-%d", random.IntN(2))
	case 2:
		meta["resync"] = true
	}
	if random.IntN(6) == 0 {
		meta["doc_hash"] = "0a207c060e61f3b88eaee0a8cd0696f46fb155eb"
	}

	// One or two operations, so the splitting is exercised as well.
	ops := []any{}
	length := docLength
	for o := 1 + random.IntN(2); o > 0; o-- {
		op, newLength := randomOp(random, document, length)
		if op == nil {
			continue
		}
		ops = append(ops, op)
		length = newLength
	}
	if len(ops) == 0 {
		return nil, docLength
	}

	update := map[string]any{
		"doc": "6a9eb3bd8a19695a4a6c97ac", "op": ops, "v": version, "meta": meta,
	}
	return mustMarshal(update), length
}

// randomOp builds one operation against a document of the given length.
func randomOp(random *rand.Rand, document string, docLength int) (any, int) {
	if docLength <= 0 {
		text := randomText(random, 1+random.IntN(4))
		return map[string]any{"p": 0, "i": text}, len(text)
	}

	position := random.IntN(docLength)
	switch random.IntN(3) {
	case 0: // an insert
		text := randomText(random, 1+random.IntN(4))
		op := map[string]any{"p": position, "i": text}
		if random.IntN(6) == 0 {
			op["u"] = true
		}
		if random.IntN(8) == 0 {
			op["commentIds"] = []string{fmt.Sprintf("thread-%d", random.IntN(2))}
		}
		return op, docLength + len(text)

	case 1: // a delete of text that is really there
		size := 1 + random.IntN(minInt(4, docLength-position))
		if position+size > len(document) {
			return nil, docLength
		}
		op := map[string]any{"p": position, "d": document[position : position+size]}
		if random.IntN(8) == 0 {
			op["u"] = true
		}
		return op, docLength - size

	default: // a comment, which changes no text
		size := 1 + random.IntN(minInt(4, docLength-position))
		if position+size > len(document) {
			return nil, docLength
		}
		return map[string]any{
			"p": position, "c": document[position : position+size],
			"t": fmt.Sprintf("thread-%d", random.IntN(2)),
		}, docLength
	}
}

// randomText builds a short piece of text out of a small alphabet, so that a
// paste has something to diff against.
func randomText(random *rand.Rand, length int) string {
	const alphabet = "abcxyz \n"
	out := make([]byte, length)
	for i := range out {
		out[i] = alphabet[random.IntN(len(alphabet))]
	}
	return string(out)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func mustMarshal(v any) json.RawMessage {
	encoded, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return encoded
}

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

func runCompressorHarness(t *testing.T, serviceDir string,
	scenarios []compressorScenario) []compressorResult {
	t.Helper()

	harness, err := filepath.Abs(filepath.Join("testdata", "compressor.js"))
	if err != nil {
		t.Fatalf("resolving the harness: %v", err)
	}
	cmd := exec.Command("node", harness, serviceDir)
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "LOG_LEVEL=fatal")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot run node: %v", err)
	}

	go func() {
		writer := bufio.NewWriter(stdin)
		for _, s := range scenarios {
			encoded, err := json.Marshal(s)
			if err != nil {
				break
			}
			_, _ = writer.Write(encoded)
			_ = writer.WriteByte('\n')
		}
		_ = writer.Flush()
		_ = stdin.Close()
	}()

	var results []compressorResult
	reader := bufio.NewReaderSize(stdout, 4<<20)
	for {
		line, err := reader.ReadBytes('\n')
		// A result is recognised by its shape: the service prints a settings
		// banner before anything else.
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte(`{"compressed"`)) ||
			bytes.HasPrefix(trimmed, []byte(`{"error"`)) {
			var r compressorResult
			if err := json.Unmarshal(line, &r); err == nil {
				results = append(results, r)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading from the harness: %v", err)
		}
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("the harness exited badly: %v", err)
	}
	return results
}
