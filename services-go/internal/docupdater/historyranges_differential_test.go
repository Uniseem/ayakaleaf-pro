package docupdater

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/rangestracker"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/textot"
)

// History stores a document with its tracked deletions still in the text, and
// every marker sent to it carries a second position measured in that longer
// text. Getting the arithmetic wrong is invisible until somebody restores a
// version and finds their comments attached to the wrong words.
//
//	DOCUPDATER_NODE_DIR=/build/services/document-updater \
//	  go test ./internal/docupdater/ -run HistoryRangesDifferential

var historyRangesCases = envInt("HISTORY_RANGES_CASES", 20000)

type historyRangesScenario struct {
	Content  string                   `json:"content"`
	Changes  []*rangestracker.Change  `json:"changes"`
	Comments []*rangestracker.Comment `json:"comments"`
}

type historyRangesResult struct {
	Ranges  json.RawMessage `json:"ranges"`
	Content string          `json:"content"`
	Error   string          `json:"error"`
}

func TestHistoryRangesDifferentialAgainstNode(t *testing.T) {
	nodeDir := os.Getenv("DOCUPDATER_NODE_DIR")
	if nodeDir == "" {
		t.Skip("set DOCUPDATER_NODE_DIR to the document-updater service to run this")
	}
	nodeDir, err := filepath.Abs(nodeDir)
	if err != nil {
		t.Fatalf("resolving DOCUPDATER_NODE_DIR: %v", err)
	}

	scenarios := generateHistoryRangesScenarios(historyRangesCases)
	results := runHistoryRangesHarness(t, nodeDir, scenarios)
	if len(results) != len(scenarios) {
		t.Fatalf("the harness returned %d results for %d scenarios",
			len(results), len(scenarios))
	}

	for i, scenario := range scenarios {
		if results[i].Error != "" {
			t.Fatalf("case %d: the Node conversion failed: %s", i, results[i].Error)
		}

		gotContent := addTrackedDeletesToContent(
			textot.T(scenario.Content), scenario.Changes).String()
		if gotContent != results[i].Content {
			t.Fatalf("case %d: content\n  node: %q\n  go:   %q",
				i, results[i].Content, gotContent)
		}

		got, err := json.Marshal(toHistoryRanges(scenario.Changes, scenario.Comments))
		if err != nil {
			t.Fatalf("case %d: encoding the ranges: %v", i, err)
		}
		if !sameJSON(string(results[i].Ranges), string(got)) {
			t.Fatalf("case %d\n  changes:  %s\n  comments: %s\n  node: %s\n  go:   %s",
				i, describeMarkers(scenario.Changes), describeMarkers(scenario.Comments),
				results[i].Ranges, got)
		}
	}
}

func describeMarkers(markers any) string {
	encoded, _ := json.Marshal(markers)
	return string(encoded)
}

// generateHistoryRangesScenarios builds documents with tracked changes and
// comments overlapping in the ways that make the arithmetic interesting: a
// comment before a tracked deletion, after one, spanning one, and spanning
// several.
func generateHistoryRangesScenarios(count int) []historyRangesScenario {
	random := rand.New(rand.NewPCG(17, 5))

	scenarios := make([]historyRangesScenario, 0, count)
	for len(scenarios) < count {
		content := strings.Repeat("abcdefghij", 1+random.IntN(4))
		length := len([]rune(content))

		// The changes have to be in order and not overlap, which is what the
		// tracker guarantees for a real document.
		var changes []*rangestracker.Change
		position := 0
		for c := random.IntN(4); c > 0 && position < length-2; c-- {
			position += random.IntN(6)
			if position >= length-1 {
				break
			}
			size := 1 + random.IntN(4)
			if position+size > length {
				size = length - position
			}
			kind := textot.Insert
			if random.IntN(2) == 0 {
				kind = textot.Delete
			}
			changes = append(changes, &rangestracker.Change{
				ID: fmt.Sprintf("change-%d", len(changes)),
				Op: textot.Component{
					Kind: kind, Pos: position,
					Text: textot.T(content[position : position+size]),
				},
				Metadata: rangestracker.Metadata{
					"user_id": json.RawMessage(`"u1"`),
					"ts":      json.RawMessage(`"2026-01-01T00:00:00.000Z"`),
				},
			})
			position += size
		}

		var comments []*rangestracker.Comment
		for c := random.IntN(4); c > 0; c-- {
			at := random.IntN(length)
			size := 1 + random.IntN(12)
			if at+size > length {
				size = length - at
			}
			if size <= 0 {
				continue
			}
			comments = append(comments, &rangestracker.Comment{
				ID: fmt.Sprintf("comment-%d", len(comments)),
				Op: textot.Component{
					Kind: textot.Comment, Pos: at,
					Text:   textot.T(content[at : at+size]),
					Thread: fmt.Sprintf("thread-%d", len(comments)),
				},
				Metadata: rangestracker.Metadata{
					"user_id": json.RawMessage(`"u2"`),
					"ts":      json.RawMessage(`"2026-01-01T00:00:00.000Z"`),
				},
			})
		}

		scenarios = append(scenarios, historyRangesScenario{
			Content: content, Changes: changes, Comments: comments,
		})
	}
	return scenarios
}

func runHistoryRangesHarness(t *testing.T, nodeDir string,
	scenarios []historyRangesScenario) []historyRangesResult {
	t.Helper()

	harness, err := filepath.Abs(filepath.Join("testdata", "historyranges.js"))
	if err != nil {
		t.Fatalf("resolving the harness: %v", err)
	}
	cmd := exec.Command("node", harness, nodeDir)
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

	var results []historyRangesResult
	reader := bufio.NewReaderSize(stdout, 1<<20)
	for {
		line, err := reader.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			var result historyRangesResult
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
