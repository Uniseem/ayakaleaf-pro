package histmodel

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

// A chunk is how the history is stored: the project at some version, and the
// changes after it. Recovering the project at a later version means replaying
// those changes, and that replay is what every read of the history goes
// through -- a diff, a restore, a snapshot.
//
// So the chunk is compared against the real library twice over: the round trip
// through JSON, and the project as it stood at every version the chunk covers.
//
//	OVERLEAF_EDITOR_CORE_DIR=../../../libraries/overleaf-editor-core \
//	  go test ./internal/histmodel/ -run ChunkDifferential

var chunkCases = envInt("HIST_CHUNK_CASES", 20000)

type chunkScenario struct {
	Chunk json.RawMessage `json:"chunk"`
}

type chunkResult struct {
	RoundTrip  json.RawMessage   `json:"roundTrip"`
	EndVersion *int              `json:"endVersion"`
	Snapshots  []json.RawMessage `json:"snapshots"`
	Error      string            `json:"error"`
}

func TestChunkDifferentialAgainstCore(t *testing.T) {
	libraryDir := os.Getenv("OVERLEAF_EDITOR_CORE_DIR")
	if libraryDir == "" {
		t.Skip("set OVERLEAF_EDITOR_CORE_DIR to the overleaf-editor-core library to run this")
	}
	libraryDir, err := filepath.Abs(libraryDir)
	if err != nil {
		t.Fatalf("resolving OVERLEAF_EDITOR_CORE_DIR: %v", err)
	}

	scenarios := generateChunkScenarios(chunkCases)
	results := runChunkHarness(t, libraryDir, scenarios)
	if len(results) != len(scenarios) {
		t.Fatalf("the harness returned %d results for %d scenarios",
			len(results), len(scenarios))
	}

	var compared, replayed int
	for i, s := range scenarios {
		want := results[i]

		var chunk Chunk
		err := json.Unmarshal(s.Chunk, &chunk)

		if want.Error != "" {
			if err == nil {
				t.Fatalf("case %d: the library refused this and the port accepted it: %s\n  %s",
					i, want.Error, s.Chunk)
			}
			continue
		}
		if err != nil {
			t.Fatalf("case %d: the port refused this and the library accepted it: %v\n  %s",
				i, err, s.Chunk)
		}
		compared++

		got, err := json.Marshal(&chunk)
		if err != nil {
			t.Fatalf("case %d: encoding: %v", i, err)
		}
		if !sameJSON(string(want.RoundTrip), string(got)) {
			t.Fatalf("case %d: round trip\n  chunk: %s\n  core: %s\n  go:   %s",
				i, s.Chunk, want.RoundTrip, got)
		}
		if want.EndVersion != nil && *want.EndVersion != chunk.EndVersion() {
			t.Fatalf("case %d: end version: core %d, go %d\n  %s",
				i, *want.EndVersion, chunk.EndVersion(), s.Chunk)
		}

		for v, wantSnapshot := range want.Snapshots {
			version := chunk.StartVersion + v
			snapshot, err := chunk.GetSnapshotAt(version)

			// A snapshot the library could not build comes back as an error
			// object rather than a snapshot.
			var failed struct {
				Error string `json:"error"`
			}
			_ = json.Unmarshal(wantSnapshot, &failed)
			if failed.Error != "" {
				if err == nil {
					t.Fatalf("case %d: version %d: core refused it (%s), the port did not\n  %s",
						i, version, failed.Error, s.Chunk)
				}
				continue
			}
			if err != nil {
				t.Fatalf("case %d: version %d: the port refused it: %v\n  %s",
					i, version, err, s.Chunk)
			}
			replayed++

			gotSnapshot, _ := json.Marshal(snapshot)
			if !sameJSON(string(wantSnapshot), string(gotSnapshot)) {
				t.Fatalf("case %d: version %d\n  chunk: %s\n  core: %s\n  go:   %s",
					i, version, s.Chunk, wantSnapshot, gotSnapshot)
			}
		}
	}

	if compared == 0 || replayed == 0 {
		t.Fatalf("the generator produces nothing usable: %d read, %d replayed",
			compared, replayed)
	}
	t.Logf("%d of %d chunks were read by both, %d versions replayed",
		compared, len(scenarios), replayed)
}

// generateChunkScenarios builds chunks: a snapshot of a few files, and changes
// that edit, add, move and delete them.
func generateChunkScenarios(count int) []chunkScenario {
	random := rand.New(rand.NewPCG(41, 13))

	scenarios := make([]chunkScenario, 0, count)
	for len(scenarios) < count {
		// The files the snapshot starts with, and how long each one is, so the
		// edits that follow can be made to fit.
		paths := []string{"main.tex"}
		if random.IntN(2) == 0 {
			paths = append(paths, "chapter.tex")
		}
		if random.IntN(3) == 0 {
			paths = append(paths, "figure.png")
		}

		files := map[string]any{}
		lengths := map[string]int{}
		for _, path := range paths {
			switch {
			case path == "figure.png":
				// A binary file, stored by hash and byte length.
				files[path] = map[string]any{
					"hash": "aed2973e4b8a7ff1b30ff5c4751e5a2b38989e74", "byteLength": 3,
				}
			default:
				content := randomContent(random)
				lengths[path] = len(content)
				file := map[string]any{"content": content}
				if random.IntN(3) == 0 {
					file["trackedChanges"] = decodeAll(
						randomTrackedChanges(random, len(content)))
				}
				if random.IntN(3) == 0 {
					file["comments"] = decodeAll(randomComments(random, len(content)))
				}
				files[path] = file
			}
		}

		changes := []any{}
		for c := random.IntN(4); c > 0; c-- {
			change := randomChange(random, paths, lengths)
			if change == nil {
				continue
			}
			changes = append(changes, change)
		}

		chunk := map[string]any{
			"history": map[string]any{
				"snapshot": map[string]any{"files": files},
				"changes":  changes,
			},
			"startVersion": random.IntN(5),
		}
		scenarios = append(scenarios, chunkScenario{Chunk: mustMarshal(chunk)})
	}
	return scenarios
}

// randomChange builds one entry of a history over the given files.
func randomChange(random *rand.Rand, paths []string, lengths map[string]int) any {
	path := paths[random.IntN(len(paths))]
	operations := []any{}

	switch random.IntN(4) {
	case 0: // an edit
		length, editable := lengths[path]
		if !editable {
			return nil
		}
		steps, produced := randomOperation(random, length)
		lengths[path] = produced
		operations = append(operations, map[string]any{
			"pathname": path, "textOperation": decodeAll(steps),
		})

	case 1: // a file added
		content := randomContent(random)
		name := fmt.Sprintf("added-%d.tex", random.IntN(3))
		lengths[name] = len(content)
		operations = append(operations, map[string]any{
			"pathname": name, "file": map[string]any{"content": content},
		})

	case 2: // a rename, or a delete when the new name is empty
		to := ""
		if random.IntN(2) == 0 {
			to = fmt.Sprintf("moved-%d.tex", random.IntN(3))
			lengths[to] = lengths[path]
		}
		operations = append(operations, map[string]any{
			"pathname": path, "newPathname": to,
		})
		delete(lengths, path)

	default: // metadata
		operations = append(operations, map[string]any{
			"pathname": path,
			"metadata": map[string]any{"importedAt": "2026-01-01T00:00:00.000Z"},
		})
	}

	change := map[string]any{
		"operations": operations,
		"timestamp":  timestamps[random.IntN(len(timestamps))],
		"authors":    []any{},
	}
	if random.IntN(3) == 0 {
		// The library checks these are object ids, so they have to look like
		// object ids.
		change["v2Authors"] = []any{
			fmt.Sprintf("6a9eb3bc8a19695a4a6c97%02d", random.IntN(2)),
		}
	}
	if random.IntN(4) == 0 {
		change["origin"] = map[string]any{"kind": "dropbox"}
	}
	if random.IntN(4) == 0 {
		change["projectVersion"] = fmt.Sprintf("%d.0", random.IntN(5))
	}
	return change
}

// randomContent builds a short document out of a small alphabet, so that the
// edits over it have runs to split.
func randomContent(random *rand.Rand) string {
	const alphabet = "abcxyz \n"
	length := 1 + random.IntN(30)
	out := make([]byte, length)
	for i := range out {
		out[i] = alphabet[random.IntN(len(alphabet))]
	}
	return string(out)
}

// decodeAll turns encoded fragments back into values, so they can be nested in
// a structure that is encoded as a whole.
func decodeAll(raw []json.RawMessage) []any {
	out := make([]any, 0, len(raw))
	for _, encoded := range raw {
		var value any
		if err := json.Unmarshal(encoded, &value); err == nil {
			out = append(out, value)
		}
	}
	return out
}

func runChunkHarness(t *testing.T, libraryDir string, scenarios []chunkScenario) []chunkResult {
	t.Helper()

	harness, err := filepath.Abs(filepath.Join("testdata", "chunkharness.js"))
	if err != nil {
		t.Fatalf("resolving the harness: %v", err)
	}
	cmd := exec.Command("node", harness, libraryDir)
	cmd.Stderr = os.Stderr

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

	var results []chunkResult
	reader := bufio.NewReaderSize(stdout, 4<<20)
	for {
		line, err := reader.ReadBytes('\n')
		if bytes.HasPrefix(bytes.TrimSpace(line), []byte("{")) {
			var r chunkResult
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
