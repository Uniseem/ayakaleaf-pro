package docupdater

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

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/textot"
)

// An edit on a document with history ranges support has to be described twice:
// once in the positions the editor works in, and once in the positions of the
// text that still holds the tracked deletions. Getting the second one wrong is
// invisible until somebody looks at the history, and by then every edit since
// has been recorded against the wrong place.
//
// So the whole conversion is compared against the real RangesManager, over
// documents with tracked changes and comments arranged in the ways that make
// the arithmetic hard: an insert inside a comment, an insert that rejects a
// tracked delete, a delete that runs over one, a comment that spans one.
//
//	DOCUPDATER_NODE_DIR=/build/services/document-updater \
//	  go test ./internal/docupdater/ -run HistoryOpsDifferential

var historyOpsCases = envInt("HISTORY_OPS_CASES", 20000)

type historyOpsScenario struct {
	Changes              []json.RawMessage `json:"changes"`
	Comments             []json.RawMessage `json:"comments"`
	Updates              []historyOpsUpdate
	NewDocLines          []string `json:"newDocLines"`
	HistoryRangesSupport bool     `json:"historyRangesSupport"`
}

type historyOpsUpdate struct {
	Op   textot.Op         `json:"op"`
	Meta map[string]string `json:"meta,omitempty"`
}

// MarshalJSON keeps the field names the Node service reads.
func (s historyOpsScenario) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"changes": s.Changes, "comments": s.Comments, "updates": s.Updates,
		"newDocLines": s.NewDocLines, "historyRangesSupport": s.HistoryRangesSupport,
	})
}

type historyOpsResult struct {
	HistoryUpdates []struct {
		Op json.RawMessage `json:"op"`
	} `json:"historyUpdates"`
	NewRanges           json.RawMessage `json:"newRanges"`
	RemovedChangeIDs    []string        `json:"removedChangeIds"`
	RangesWereCollapsed bool            `json:"rangesWereCollapsed"`
	Error               string          `json:"error"`
}

func TestHistoryOpsDifferentialAgainstNode(t *testing.T) {
	nodeDir := os.Getenv("DOCUPDATER_NODE_DIR")
	if nodeDir == "" {
		t.Skip("set DOCUPDATER_NODE_DIR to the document-updater service to run this")
	}
	nodeDir, err := filepath.Abs(nodeDir)
	if err != nil {
		t.Fatalf("resolving DOCUPDATER_NODE_DIR: %v", err)
	}

	scenarios := generateHistoryOpsScenarios(historyOpsCases)
	results := runHistoryOpsHarness(t, nodeDir, scenarios)
	if len(results) != len(scenarios) {
		t.Fatalf("the harness returned %d results for %d scenarios",
			len(results), len(scenarios))
	}

	var compared int
	for i, scenario := range scenarios {
		encodedScenario, _ := json.Marshal(scenario)

		// The tracker refuses an op that does not match the text, and so does
		// the port. A scenario both refuse is not a difference.
		got, goErr := applyHistoryOpsScenario(scenario)
		if results[i].Error != "" {
			if goErr == nil {
				t.Fatalf("case %d: Node refused this and Go accepted it: %s\n  %s",
					i, results[i].Error, encodedScenario)
			}
			continue
		}
		if goErr != nil {
			t.Fatalf("case %d: Go refused this and Node accepted it: %v\n  %s",
				i, goErr, encodedScenario)
		}
		compared++

		want := make([]json.RawMessage, 0, len(results[i].HistoryUpdates))
		for _, update := range results[i].HistoryUpdates {
			want = append(want, update.Op)
		}
		encodedWant, _ := json.Marshal(want)
		encodedGot, _ := json.Marshal(got.ops)
		if !sameJSON(string(encodedWant), string(encodedGot)) {
			t.Fatalf("case %d: history ops\n  scenario: %s\n  node: %s\n  go:   %s",
				i, encodedScenario, encodedWant, encodedGot)
		}

		// An edit that splits a tracked change gives the second half a fresh
		// id, and a fresh id carries a per-process random part. Those are
		// renamed in order on both sides so the rest can be compared.
		known := knownMarkerIDs(scenario)
		if !sameJSONIgnoring(
			normalizeGeneratedIDs(nonEmptyJSON(string(results[i].NewRanges)), known),
			normalizeGeneratedIDs(nonEmptyJSON(string(got.ranges)), known),
			"changes.metadata.ts", "comments.metadata.ts") {
			t.Fatalf("case %d: ranges\n  scenario: %s\n  node: %s\n  go:   %s",
				i, encodedScenario, results[i].NewRanges, got.ranges)
		}
		if results[i].RangesWereCollapsed != got.collapsed {
			t.Fatalf("case %d: collapsed: node %v, go %v\n  %s",
				i, results[i].RangesWereCollapsed, got.collapsed, encodedScenario)
		}
	}
	if compared == 0 {
		t.Fatal("every scenario was refused by both; the generator is producing nothing usable")
	}
	t.Logf("%d of %d scenarios were applied by both", compared, len(scenarios))
}

type historyOpsOutcome struct {
	ops       []json.RawMessage
	ranges    json.RawMessage
	collapsed bool
}

// applyHistoryOpsScenario puts the scenario through the port, one update at a
// time, the way the pipeline does.
func applyHistoryOpsScenario(scenario historyOpsScenario) (historyOpsOutcome, error) {
	var out historyOpsOutcome

	ranges, err := json.Marshal(map[string]any{
		"changes": scenario.Changes, "comments": scenario.Comments,
	})
	if err != nil {
		return out, err
	}

	for _, update := range scenario.Updates {
		result, err := applyUpdateToRanges(ranges, update.Op, scenario.NewDocLines,
			update.Meta["user_id"], update.Meta["tc"], scenario.HistoryRangesSupport)
		if err != nil {
			return out, err
		}
		if len(result.HistoryOps) > 0 {
			encoded, err := json.Marshal(result.HistoryOps)
			if err != nil {
				return out, err
			}
			out.ops = append(out.ops, encoded)
		}
		ranges = result.NewRanges
		out.collapsed = out.collapsed || result.Collapsed
	}
	out.ranges = ranges
	return out, nil
}

// generateHistoryOpsScenarios builds documents with markers and an edit over
// them.
//
// The document is fixed and the markers are placed into it, so that the op and
// the markers agree often enough for the tracker to accept them: a scenario
// both implementations refuse says nothing.
func generateHistoryOpsScenarios(count int) []historyOpsScenario {
	random := rand.New(rand.NewPCG(31, 7))
	const line = "the quick brown fox jumps over the lazy dog"

	scenarios := make([]historyOpsScenario, 0, count)
	for len(scenarios) < count {
		length := len(line)

		// The changes are in order and do not overlap, which is what the
		// tracker guarantees for a real document.
		var changes []json.RawMessage
		position := 0
		for c := random.IntN(4); c > 0 && position < length-6; c-- {
			position += random.IntN(8)
			if position >= length-2 {
				break
			}
			size := 1 + random.IntN(5)
			if position+size > length {
				size = length - position
			}
			field := "i"
			if random.IntN(2) == 0 {
				field = "d"
			}
			change, _ := json.Marshal(map[string]any{
				"id": "change-" + string(rune('a'+len(changes))),
				"op": map[string]any{
					"p": position, field: line[position : position+size],
				},
				"metadata": map[string]any{
					"user_id": "u1", "ts": "2026-01-01T00:00:00.000Z",
				},
			})
			changes = append(changes, change)
			position += size
		}

		var comments []json.RawMessage
		for c := random.IntN(3); c > 0; c-- {
			at := random.IntN(length - 1)
			size := 1 + random.IntN(10)
			if at+size > length {
				size = length - at
			}
			comment, _ := json.Marshal(map[string]any{
				"id": "thread-" + string(rune('a'+len(comments))),
				"op": map[string]any{
					"p": at, "c": line[at : at+size],
					"t": "thread-" + string(rune('a'+len(comments))),
				},
				"metadata": map[string]any{
					"user_id": "u2", "ts": "2026-01-01T00:00:00.000Z",
				},
			})
			comments = append(comments, comment)
		}

		// One op, of a kind that has something to interact with.
		at := random.IntN(length - 1)
		var op textot.Component
		switch random.IntN(3) {
		case 0:
			op = textot.Component{Kind: textot.Insert, Pos: at, Text: textot.T("XY"),
				Undo: random.IntN(2) == 0}
		case 1:
			size := 1 + random.IntN(6)
			if at+size > length {
				size = length - at
			}
			op = textot.Component{Kind: textot.Delete, Pos: at,
				Text: textot.T(line[at : at+size])}
		default:
			size := 1 + random.IntN(8)
			if at+size > length {
				size = length - at
			}
			op = textot.Component{Kind: textot.Comment, Pos: at,
				Text: textot.T(line[at : at+size]), Thread: "thread-new"}
		}

		meta := map[string]string{"user_id": "u3"}
		if random.IntN(2) == 0 {
			meta["tc"] = "0123456789abcdef01"
		}

		// The resulting document is only used for the validation the tracker
		// does, which both sides do the same way.
		after := line
		switch op.Kind {
		case textot.Insert:
			after = line[:op.Pos] + op.Text.String() + line[op.Pos:]
		case textot.Delete:
			after = line[:op.Pos] + line[op.Pos+op.Len():]
		}

		scenarios = append(scenarios, historyOpsScenario{
			Changes:              changes,
			Comments:             comments,
			Updates:              []historyOpsUpdate{{Op: textot.Op{op}, Meta: meta}},
			NewDocLines:          []string{after},
			HistoryRangesSupport: true,
		})
	}
	return scenarios
}

func runHistoryOpsHarness(t *testing.T, nodeDir string,
	scenarios []historyOpsScenario) []historyOpsResult {
	t.Helper()

	harness, err := filepath.Abs(filepath.Join("testdata", "historyops.js"))
	if err != nil {
		t.Fatalf("resolving the harness: %v", err)
	}
	cmd := exec.Command("node", harness, nodeDir)
	cmd.Stderr = os.Stderr
	// RangesManager logs a record per update, on stdout, and those parse as
	// results because every field of one is optional.
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
		for _, scenario := range scenarios {
			encoded, err := json.Marshal(scenario)
			if err != nil {
				break
			}
			_, _ = writer.Write(encoded)
			_ = writer.WriteByte('\n')
		}
		_ = writer.Flush()
		_ = stdin.Close()
	}()

	var results []historyOpsResult
	reader := bufio.NewReaderSize(stdout, 1<<20)
	for {
		line, err := reader.ReadBytes('\n')
		// A result is recognised by the field only a result has: the service
		// prints a settings banner before anything else, and anything it logs
		// would otherwise be taken for an answer.
		if bytes.Contains(line, []byte("historyUpdates")) || bytes.Contains(line, []byte(`"error"`)) {
			var result historyOpsResult
			if err := json.Unmarshal(line, &result); err == nil {
				results = append(results, result)
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

// knownMarkerIDs are the ids the scenario started with. Anything else in the
// result was generated while the update was applied.
func knownMarkerIDs(scenario historyOpsScenario) map[string]bool {
	known := map[string]bool{}
	for _, list := range [][]json.RawMessage{scenario.Changes, scenario.Comments} {
		for _, raw := range list {
			var marker struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(raw, &marker); err == nil {
				known[marker.ID] = true
			}
		}
	}
	return known
}

// normalizeGeneratedIDs renames the ids that were minted during the update,
// in the order they appear, so two runs can be compared.
func normalizeGeneratedIDs(ranges string, known map[string]bool) string {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(ranges), &decoded); err != nil {
		return ranges
	}

	generated := 0
	for _, key := range []string{"changes", "comments"} {
		list, ok := decoded[key].([]any)
		if !ok {
			continue
		}
		for _, element := range list {
			marker, ok := element.(map[string]any)
			if !ok {
				continue
			}
			id, ok := marker["id"].(string)
			if !ok || known[id] {
				continue
			}
			generated++
			marker["id"] = fmt.Sprintf("generated-%d", generated)
		}
	}

	encoded, err := json.Marshal(decoded)
	if err != nil {
		return ranges
	}
	return string(encoded)
}
