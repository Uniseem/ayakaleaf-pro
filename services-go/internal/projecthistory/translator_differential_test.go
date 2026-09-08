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

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/histmodel"
)

// This is where the two ways of describing a document meet. The editor works in
// positions into the text it shows, with the tracked deletions taken out; the
// history works in a scan over the text it keeps, with them left in. A delete
// that runs over a tracked deletion is not one delete in the history but a
// delete, a retain over the marked text, and another delete -- and a delete
// made with track changes on is not a delete there at all, but a retain that
// marks the text.
//
// Getting that arithmetic wrong writes a plausible but wrong entry into the
// permanent record, so it is compared against the real translator.
//
//	PROJECT_HISTORY_DIR=/build/services/project-history \
//	  go test ./internal/projecthistory/ -run TranslatorDifferential

var translatorCases = envInt("TRANSLATOR_CASES", 20000)

type translatorScenario struct {
	Updates []json.RawMessage `json:"updates"`
}

type translatorResult struct {
	Changes json.RawMessage `json:"changes"`
	Error   string          `json:"error"`
}

func TestTranslatorDifferentialAgainstNode(t *testing.T) {
	serviceDir := os.Getenv("PROJECT_HISTORY_DIR")
	if serviceDir == "" {
		t.Skip("set PROJECT_HISTORY_DIR to the project-history service to run this")
	}
	serviceDir, err := filepath.Abs(serviceDir)
	if err != nil {
		t.Fatalf("resolving PROJECT_HISTORY_DIR: %v", err)
	}

	scenarios := generateTranslatorScenarios(translatorCases)
	results := runTranslatorHarness(t, serviceDir, scenarios)
	if len(results) != len(scenarios) {
		t.Fatalf("the harness returned %d results for %d scenarios",
			len(results), len(scenarios))
	}

	var compared int
	for i, s := range scenarios {
		want := results[i]
		encoded, _ := json.Marshal(s)

		withBlobs, err := readUpdatesWithBlobs(s.Updates)
		if err != nil {
			t.Fatalf("case %d: the port could not read the updates: %v\n  %s",
				i, err, encoded)
		}

		changes, err := ConvertToChanges(withBlobs)
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

		got, err := json.Marshal(nonNilChanges(changes))
		if err != nil {
			t.Fatalf("case %d: encoding: %v", i, err)
		}
		if !sameJSON(string(want.Changes), string(got)) {
			t.Fatalf("case %d\n  updates: %s\n  node: %s\n  go:   %s",
				i, encoded, want.Changes, got)
		}
	}

	if compared == 0 {
		t.Fatal("every scenario was refused by both; the generator produces nothing usable")
	}
	t.Logf("%d of %d update runs were translated by both", compared, len(scenarios))
}

func nonNilChanges(changes []*histmodel.Change) []*histmodel.Change {
	if changes == nil {
		return []*histmodel.Change{}
	}
	return changes
}

// readUpdatesWithBlobs reads the updates and picks the blob hashes out of them.
//
// The real translator is given the hashes alongside the update; here they are
// carried on the update itself so that one scenario is one JSON object.
func readUpdatesWithBlobs(raw []json.RawMessage) ([]UpdateWithBlob, error) {
	withBlobs := make([]UpdateWithBlob, 0, len(raw))
	for _, encoded := range raw {
		var update Update
		if err := json.Unmarshal(encoded, &update); err != nil {
			return nil, err
		}
		var blobs struct {
			BlobHashes struct {
				File   string `json:"file"`
				Ranges string `json:"ranges"`
			} `json:"blobHashes"`
		}
		_ = json.Unmarshal(encoded, &blobs)
		withBlobs = append(withBlobs, UpdateWithBlob{
			Update: &update, FileHash: blobs.BlobHashes.File,
			RangesHash: blobs.BlobHashes.Ranges,
		})
	}
	return withBlobs, nil
}

// generateTranslatorScenarios builds runs of updates of every shape the
// translator understands.
func generateTranslatorScenarios(count int) []translatorScenario {
	random := rand.New(rand.NewPCG(53, 17))

	scenarios := make([]translatorScenario, 0, count)
	for len(scenarios) < count {
		updates := []json.RawMessage{}
		for u := 1 + random.IntN(3); u > 0; u-- {
			updates = append(updates, randomTranslatorUpdate(random))
		}
		scenarios = append(scenarios, translatorScenario{Updates: updates})
	}
	return scenarios
}

// randomTranslatorUpdate builds one update.
func randomTranslatorUpdate(random *rand.Rand) json.RawMessage {
	timestamp := int64(1700000000000 + random.IntN(100000))
	meta := map[string]any{
		"ts": timestamp, "user_id": fmt.Sprintf("6a9eb3bc8a19695a4a6c97%02d", random.IntN(2)),
	}

	switch random.IntN(8) {
	case 0: // a rename, or a delete when the new name is empty
		newPath := ""
		if random.IntN(2) == 0 {
			newPath = "/renamed.tex"
		}
		return mustMarshal(map[string]any{
			"pathname": "/main.tex", "new_pathname": newPath,
			"version": fmt.Sprintf("%d.0", random.IntN(5)), "meta": meta,
		})

	case 1: // a document added
		return mustMarshal(map[string]any{
			"pathname": "/added.tex", "doc": "6a9eb3bd8a19695a4a6c97ac",
			"docLines": "a\nb",
			"blobHashes": map[string]any{
				"file":   "aed2973e4b8a7ff1b30ff5c4751e5a2b38989e74",
				"ranges": "0a207c060e61f3b88eaee0a8cd0696f46fb155eb",
			},
			"version": fmt.Sprintf("%d.0", random.IntN(5)), "meta": meta,
		})

	case 2: // a file added
		return mustMarshal(map[string]any{
			"pathname": "/figure.png", "file": "6a9eb3bd8a19695a4a6c97ad",
			"url": "http://127.0.0.1:3009/file",
			"blobHashes": map[string]any{
				"file": "aed2973e4b8a7ff1b30ff5c4751e5a2b38989e74",
			},
			"version": fmt.Sprintf("%d.0", random.IntN(5)), "meta": meta,
		})

	case 3: // a comment resolved or reopened
		return mustMarshal(map[string]any{
			"pathname": "/main.tex", "commentId": "thread-1",
			"resolved": random.IntN(2) == 0, "meta": meta,
		})

	case 4: // a comment deleted
		return mustMarshal(map[string]any{
			"pathname": "/main.tex", "deleteComment": "thread-1", "meta": meta,
		})

	case 5: // metadata set on a file
		return mustMarshal(map[string]any{
			"pathname": "/figure.png",
			"metadata": map[string]any{"importedAt": "2026-01-01T00:00:00.000Z"},
			"meta":     meta,
		})

	default: // an edit
		return randomTranslatorTextUpdate(random, meta, timestamp)
	}
}

// randomTranslatorTextUpdate builds an edit, sometimes with track changes on
// and sometimes running over marked text.
func randomTranslatorTextUpdate(random *rand.Rand, meta map[string]any, timestamp int64) json.RawMessage {
	const document = "the quick brown fox jumps over the lazy dog"
	docLength := len(document)

	meta["pathname"] = "/main.tex"
	meta["doc_length"] = docLength
	if random.IntN(3) == 0 {
		meta["tc"] = "0123456789abcdef01"
	}
	if random.IntN(6) == 0 {
		meta["doc_hash"] = "0a207c060e61f3b88eaee0a8cd0696f46fb155eb"
	}
	if random.IntN(8) == 0 {
		meta["type"] = "external"
		meta["source"] = "dropbox"
	}

	ops := []any{}
	for o := 1 + random.IntN(3); o > 0; o-- {
		position := random.IntN(docLength)
		switch random.IntN(4) {
		case 0: // an insert
			op := map[string]any{"p": position, "i": "XY"}
			if random.IntN(5) == 0 {
				op["commentIds"] = []string{"thread-1"}
			}
			if random.IntN(6) == 0 {
				op["trackedDeleteRejection"] = true
				op["u"] = true
			}
			if random.IntN(4) == 0 {
				op["hpos"] = position + 1 + random.IntN(3)
			}
			ops = append(ops, op)

		case 1: // a delete
			size := 1 + random.IntN(minInt(5, docLength-position))
			op := map[string]any{"p": position, "d": document[position : position+size]}
			if random.IntN(3) == 0 {
				// A delete that ran over marked text, which is where the
				// arithmetic is hardest.
				op["trackedChanges"] = []map[string]any{{
					"type": []string{"insert", "delete"}[random.IntN(2)],
					// Inside the deleted stretch, and short enough to fit.
					"offset": random.IntN(size),
					"length": 1,
				}}
			}
			if random.IntN(4) == 0 {
				op["hpos"] = position + 1 + random.IntN(3)
			}
			ops = append(ops, op)

		default: // a comment
			size := 1 + random.IntN(minInt(5, docLength-position))
			op := map[string]any{
				"p": position, "c": document[position : position+size], "t": "thread-1",
			}
			if random.IntN(4) == 0 {
				op["hpos"] = position + 1 + random.IntN(3)
				op["hlen"] = size + 1
			}
			if random.IntN(4) == 0 {
				// Both ways round: an unresolved comment is written
				// differently from one that says nothing about it.
				op["resolved"] = random.IntN(2) == 0
			}
			if random.IntN(6) == 0 {
				// A second comment on the same thread, which is the one case
				// where two comment operations meet inside one change.
				op2 := map[string]any{
					"p": position, "c": document[position : position+size],
					"t": "thread-1",
				}
				if random.IntN(3) == 0 {
					op2["resolved"] = random.IntN(2) == 0
				}
				ops = append(ops, op2)
			}
			ops = append(ops, op)
		}
	}

	return mustMarshal(map[string]any{
		"doc": "6a9eb3bd8a19695a4a6c97ac", "op": ops,
		"v": random.IntN(20), "meta": meta,
	})
}

func runTranslatorHarness(t *testing.T, serviceDir string,
	scenarios []translatorScenario) []translatorResult {
	t.Helper()

	harness, err := filepath.Abs(filepath.Join("testdata", "translator.js"))
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
			// The blob hashes ride on the update here; the real translator is
			// given them alongside it.
			updates := make([]json.RawMessage, 0, len(s.Updates))
			for _, update := range s.Updates {
				updates = append(updates, wrapWithBlob(update))
			}
			encoded, err := json.Marshal(map[string]any{"updates": updates})
			if err != nil {
				break
			}
			_, _ = writer.Write(encoded)
			_ = writer.WriteByte('\n')
		}
		_ = writer.Flush()
		_ = stdin.Close()
	}()

	var results []translatorResult
	reader := bufio.NewReaderSize(stdout, 4<<20)
	for {
		line, err := reader.ReadBytes('\n')
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte(`{"changes"`)) ||
			bytes.HasPrefix(trimmed, []byte(`{"error"`)) {
			var r translatorResult
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

// wrapWithBlob puts the update in the envelope the real translator expects: the
// update under "update", and the hashes beside it.
func wrapWithBlob(raw json.RawMessage) json.RawMessage {
	var blobs struct {
		BlobHashes json.RawMessage `json:"blobHashes"`
	}
	_ = json.Unmarshal(raw, &blobs)

	envelope := map[string]any{"update": raw}
	if len(blobs.BlobHashes) > 0 {
		envelope["blobHashes"] = blobs.BlobHashes
	}
	return mustMarshal(envelope)
}
