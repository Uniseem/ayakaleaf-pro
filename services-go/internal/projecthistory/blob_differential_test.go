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
	"testing"
)

// A document sent to the history whole carries its marks with it, and they are
// rewritten on the way: the editor's flat list of tracked changes and comment
// operations becomes the history's merged ranges over the text it stores.
//
// Getting that wrong loses somebody's tracked changes or moves a comment onto
// different words, and nothing complains at the time -- the blob is stored,
// the change is written, and the mistake is only visible much later when
// somebody opens the history. So it is compared against the real translator
// over random documents.
//
//	PROJECT_HISTORY_DIR=/build/services/project-history \
//	  go test ./internal/projecthistory/ -run BlobDifferential
var blobCases = envInt("BLOB_CASES", 20000)

type blobScenario struct {
	Update   json.RawMessage `json:"update"`
	Versions [][2]string     `json:"versions,omitempty"`
}

type blobResult struct {
	Ranges  json.RawMessage `json:"ranges"`
	Hash    string          `json:"hash"`
	Compare []int           `json:"compare"`
	Error   string          `json:"error"`
}

func TestBlobDifferentialAgainstNode(t *testing.T) {
	serviceDir := os.Getenv("PROJECT_HISTORY_DIR")
	if serviceDir == "" {
		t.Skip("set PROJECT_HISTORY_DIR to the project-history service to run this")
	}
	serviceDir, err := filepath.Abs(serviceDir)
	if err != nil {
		t.Fatalf("resolving PROJECT_HISTORY_DIR: %v", err)
	}

	scenarios := generateBlobScenarios(blobCases)
	results := runBlobHarness(t, serviceDir, scenarios)
	if len(results) != len(scenarios) {
		t.Fatalf("the harness returned %d results for %d scenarios",
			len(results), len(scenarios))
	}

	var compared int
	for i, s := range scenarios {
		want := results[i]
		encoded, _ := json.Marshal(s)

		var update Update
		if err := json.Unmarshal(s.Update, &update); err != nil {
			t.Fatalf("case %d: the port could not read the update: %v\n  %s",
				i, err, encoded)
		}

		ranges, err := CreateRangeBlobDataFromUpdate(&update)
		if want.Error != "" {
			if err == nil {
				t.Fatalf("case %d: the real translator refused this (%s), the port did not\n  %s",
					i, want.Error, encoded)
			}
			continue
		}
		if err != nil {
			t.Fatalf("case %d: the port refused this: %v\n  %s", i, err, encoded)
		}
		compared++

		got, err := json.Marshal(ranges)
		if err != nil {
			t.Fatalf("case %d: encoding: %v", i, err)
		}
		if !sameJSON(string(want.Ranges), string(got)) {
			t.Fatalf("case %d\n  update: %s\n  node: %s\n  go:   %s",
				i, encoded, want.Ranges, got)
		}

		// The hash names the blob, so the two sides have to agree on it or the
		// same file is stored twice under different names.
		docLines := rawString(update.Rest["docLines"])
		if hash := BlobHash([]byte(docLines)); hash != want.Hash {
			t.Fatalf("case %d: blob hash of %q\n  node: %s\n  go:   %s",
				i, docLines, want.Hash, hash)
		}

		for j, pair := range s.Versions {
			if got := compareVersions(pair[0], pair[1]); got != want.Compare[j] {
				t.Fatalf("case %d: comparing %q with %q\n  node: %d\n  go:   %d",
					i, pair[0], pair[1], want.Compare[j], got)
			}
		}
	}

	if compared == 0 {
		t.Fatal("every scenario was refused by both; the generator produces nothing usable")
	}
	t.Logf("%d of %d updates were translated by both", compared, len(scenarios))
}

// generateBlobScenarios builds documents with marks on them.
//
// The marks are generated the way the editor holds them: disjoint tracked
// changes over the text as it is shown, each with a history position that
// leaves room for the deletions the editor hides. Comments are generated the
// same way, several to a thread.
func generateBlobScenarios(count int) []blobScenario {
	random := rand.New(rand.NewPCG(101, 103))

	scenarios := make([]blobScenario, 0, count)
	for len(scenarios) < count {
		document := randomText(random, 20+random.IntN(60))
		length := len(document)

		changes := []any{}
		comments := []any{}

		// Tracked changes, laid down left to right so they do not overlap.
		cursor := 0
		offset := 0
		for cursor < length && random.IntN(3) != 0 {
			cursor += random.IntN(6)
			if cursor >= length {
				break
			}
			size := 1 + random.IntN(minInt(6, length-cursor))
			op := map[string]any{"p": cursor}
			if random.IntN(2) == 0 {
				op["d"] = document[cursor : cursor+size]
			} else {
				op["i"] = document[cursor : cursor+size]
			}
			if random.IntN(3) == 0 {
				// The history keeps the deletions the editor hides, so a mark
				// can sit further along there than it does here.
				op["hpos"] = cursor + offset
				offset += random.IntN(4)
			}
			changes = append(changes, map[string]any{
				"op": op,
				"metadata": map[string]any{
					"user_id": fmt.Sprintf("user-%d", random.IntN(2)),
					"ts":      randomTimestamp(random),
				},
			})
			cursor += size
		}

		// Comments, which may overlap each other and may share a thread.
		for c := random.IntN(4); c > 0; c-- {
			pos := random.IntN(length)
			size := random.IntN(minInt(8, length-pos) + 1)
			op := map[string]any{
				"p": pos, "c": document[pos : pos+size],
				"t": fmt.Sprintf("thread-%d", random.IntN(3)),
			}
			if random.IntN(4) == 0 {
				op["hpos"] = pos + random.IntN(3)
			}
			if random.IntN(3) == 0 {
				op["resolved"] = random.IntN(2) == 0
			}
			comments = append(comments, map[string]any{"op": op})
		}

		update := map[string]any{
			"doc": "6a9eb3bd8a19695a4a6c97ac", "docLines": document,
			"pathname": "/main.tex", "meta": map[string]any{"ts": 1700000000000},
		}
		switch random.IntN(8) {
		case 0:
			// No ranges at all, which is a document with nothing on it.
		case 1:
			update["ranges"] = map[string]any{}
		default:
			ranges := map[string]any{}
			if len(changes) > 0 || random.IntN(2) == 0 {
				ranges["changes"] = changes
			}
			if len(comments) > 0 || random.IntN(2) == 0 {
				ranges["comments"] = comments
			}
			update["ranges"] = ranges
		}

		scenarios = append(scenarios, blobScenario{
			Update:   mustMarshal(update),
			Versions: randomVersionPairs(random),
		})
	}
	return scenarios
}

// randomTimestamp is a time in one of the two forms a mark carries.
func randomTimestamp(random *rand.Rand) any {
	millis := int64(1700000000000) + int64(random.IntN(1000000))
	if random.IntN(2) == 0 {
		return millis
	}
	return fmt.Sprintf("2023-11-%02dT%02d:%02d:%02d.%03dZ",
		1+random.IntN(28), random.IntN(24), random.IntN(60), random.IntN(60),
		random.IntN(1000))
}

// randomVersionPairs builds project versions to compare.
func randomVersionPairs(random *rand.Rand) [][2]string {
	pairs := make([][2]string, 0, 3)
	for n := 1 + random.IntN(3); n > 0; n-- {
		pairs = append(pairs, [2]string{randomVersion(random), randomVersion(random)})
	}
	return pairs
}

func randomVersion(random *rand.Rand) string {
	switch random.IntN(4) {
	case 0:
		return fmt.Sprintf("%d", random.IntN(20))
	case 1:
		return fmt.Sprintf("%d.%d", random.IntN(20), random.IntN(20))
	case 2:
		return fmt.Sprintf("%d.%d.%d", random.IntN(5), random.IntN(5), random.IntN(5))
	}
	return fmt.Sprintf("%d.%d", random.IntN(3), random.IntN(3))
}

func runBlobHarness(t *testing.T, serviceDir string,
	scenarios []blobScenario) []blobResult {
	t.Helper()

	harness, err := filepath.Abs(filepath.Join("testdata", "blob.js"))
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

	var results []blobResult
	reader := bufio.NewReaderSize(stdout, 4<<20)
	for {
		line, err := reader.ReadBytes('\n')
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte(`{"ranges"`)) ||
			bytes.HasPrefix(trimmed, []byte(`{"error"`)) {
			var r blobResult
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
