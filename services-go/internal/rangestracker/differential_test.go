package rangestracker

import (
	"bufio"
	"encoding/json"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/textot"
)

// Tracked changes and comments are the other half of this service where being
// close is worthless: the document comes out right either way, and only the
// markers are wrong, so a mistake shows up as somebody's comment attached to
// the wrong sentence weeks later.
//
// These scenarios are put through both this port and the real ranges-tracker,
// and every marker that results is compared.
//
//	RANGES_TRACKER_DIR=../../../libraries/ranges-tracker \
//	  go test ./internal/rangestracker/ -run Differential

// differentialCases is how many scenarios to compare. CI sets it lower so the
// step finishes in a couple of minutes; a run before a release should use the
// default, which is where the mistakes found so far turned up.
var differentialCases = envInt("RANGES_TRACKER_CASES", 300000)

func envInt(name string, fallback int) int {
	if raw := os.Getenv(name); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			return value
		}
	}
	return fallback
}

type step struct {
	Op           textot.Component `json:"op"`
	Metadata     Metadata         `json:"metadata"`
	TrackChanges bool             `json:"trackChanges"`
	IDSeed       string           `json:"idSeed,omitempty"`
}

type scenario struct {
	IDSeed   string     `json:"idSeed"`
	Changes  []*Change  `json:"changes"`
	Comments []*Comment `json:"comments"`
	Steps    []step     `json:"steps"`
	Text     *string    `json:"text,omitempty"`
}

type harnessResult struct {
	Changes         []*Change  `json:"changes"`
	Comments        []*Comment `json:"comments"`
	ValidationError *string    `json:"validationError"`
	Error           string     `json:"error"`
}

func TestDifferentialAgainstNode(t *testing.T) {
	libraryDir := os.Getenv("RANGES_TRACKER_DIR")
	if libraryDir == "" {
		t.Skip("set RANGES_TRACKER_DIR to the ranges-tracker library to run this")
	}
	libraryDir, err := filepath.Abs(libraryDir)
	if err != nil {
		t.Fatalf("resolving RANGES_TRACKER_DIR: %v", err)
	}

	scenarios := generateScenarios(differentialCases)
	results := runHarness(t, libraryDir, scenarios)
	if len(results) != len(scenarios) {
		t.Fatalf("harness returned %d results for %d scenarios", len(results), len(scenarios))
	}

	for i, s := range scenarios {
		want := results[i]
		got, gotErr := applyScenario(s)

		if want.Error != "" {
			if gotErr == nil {
				t.Fatalf("case %d: Node refused this and Go accepted it: %s\n%s",
					i, want.Error, describe(t, s))
			}
			continue
		}
		if gotErr != nil {
			t.Fatalf("case %d: Go refused a scenario Node accepted: %v\n%s",
				i, gotErr, describe(t, s))
		}

		if a, b := marshal(t, got.Changes), marshal(t, want.Changes); a != b {
			t.Fatalf("case %d: tracked changes differ\n  go   %s\n  node %s\n%s",
				i, a, b, describe(t, s))
		}
		if a, b := marshal(t, got.Comments), marshal(t, want.Comments); a != b {
			t.Fatalf("case %d: comments differ\n  go   %s\n  node %s\n%s",
				i, a, b, describe(t, s))
		}

		// Both must also agree on whether the markers still match the document.
		goValidation := ""
		if s.Text != nil {
			if err := got.Validate(textot.T(*s.Text)); err != nil {
				goValidation = err.Error()
			}
		}
		nodeValidation := want.ValidationError != nil
		if (goValidation != "") != nodeValidation {
			t.Fatalf("case %d: they disagree about whether the markers are valid\n"+
				"  go   %q\n  node %v\n%s", i, goValidation, want.ValidationError, describe(t, s))
		}
	}

	t.Logf("%d random scenarios produced identical markers in both implementations",
		len(scenarios))
}

// applyScenario runs a scenario through this implementation.
func applyScenario(s scenario) (*Tracker, error) {
	tracker := New(cloneChanges(s.Changes), cloneComments(s.Comments))
	tracker.SetIDSeed(s.IDSeed)
	for _, st := range s.Steps {
		tracker.TrackChanges = st.TrackChanges
		if st.IDSeed != "" {
			tracker.SetIDSeed(st.IDSeed)
		}
		if err := tracker.ApplyOp(st.Op, st.Metadata); err != nil {
			return nil, err
		}
	}
	return tracker, nil
}

func cloneChanges(changes []*Change) []*Change {
	out := make([]*Change, 0, len(changes))
	for _, c := range changes {
		copied := *c
		copied.Op.Text = cloneText(c.Op.Text)
		copied.Metadata = c.Metadata.Clone()
		out = append(out, &copied)
	}
	return out
}

func cloneComments(comments []*Comment) []*Comment {
	out := make([]*Comment, 0, len(comments))
	for _, c := range comments {
		copied := *c
		copied.Op.Text = cloneText(c.Op.Text)
		copied.Metadata = c.Metadata.Clone()
		out = append(out, &copied)
	}
	return out
}

// generateScenarios builds random edit sequences over a document, tracking the
// document as it goes so that every delete matches the text it removes.
//
// The alphabet stays inside the Basic Multilingual Plane because a scenario has
// to survive a JSON round trip through Node.
func generateScenarios(n int) []scenario {
	rng := rand.New(rand.NewPCG(11, 12))
	alphabet := []rune{'a', 'b', 'c', 'd', ' ', 'é', '中'}
	users := []string{"u1", "u2", "u3"}

	randomText := func(maxLen int) textot.Text {
		length := 1 + rng.IntN(maxLen)
		runes := make([]rune, length)
		for i := range runes {
			runes[i] = alphabet[rng.IntN(len(alphabet))]
		}
		return textot.T(string(runes))
	}

	scenarios := make([]scenario, 0, n)
	for i := 0; i < n; i++ {
		doc := randomText(14)
		s := scenario{IDSeed: "0123456789abcdef" + leftPad(strconv.FormatInt(int64(i%256), 16), 2)}

		threads := 0
		for steps := 1 + rng.IntN(5); steps > 0; steps-- {
			metadata := Metadata{}
			metadata["user_id"], _ = json.Marshal(users[rng.IntN(len(users))])
			// A fixed timestamp, so the two implementations are not compared on
			// which of them ran first.
			metadata["ts"], _ = json.Marshal("2026-01-01T00:00:00.000Z")

			st := step{Metadata: metadata, TrackChanges: rng.IntN(100) < 70}

			roll := rng.IntN(100)
			switch {
			case doc.Len() == 0 || roll < 45:
				pos := rng.IntN(doc.Len() + 1)
				text := randomText(4)
				st.Op = textot.Component{
					Kind: textot.Insert, Text: text, Pos: pos,
					// An undo cancels a tracked delete rather than inserting
					// again, which is a path nothing else reaches.
					Undo: rng.IntN(100) < 25,
				}
				doc = doc.Inject(pos, text)

			case roll < 80:
				pos := rng.IntN(doc.Len())
				length := 1 + rng.IntN(min(doc.Len()-pos, 5))
				st.Op = textot.Component{
					Kind: textot.Delete, Text: doc.Slice(pos, pos+length), Pos: pos,
				}
				doc = textot.Concat(doc.Slice(0, pos), doc.From(pos+length))

			default:
				pos := rng.IntN(doc.Len())
				length := 1 + rng.IntN(min(doc.Len()-pos, 5))
				threads++
				st.Op = textot.Component{
					Kind: textot.Comment, Text: doc.Slice(pos, pos+length), Pos: pos,
					// Reusing a thread id sometimes, so the move-an-existing
					// comment path is exercised as well as the add path.
					Thread: "thread-" + strconv.Itoa(rng.IntN(threads+1)),
				}
			}
			s.Steps = append(s.Steps, st)
		}

		text := doc.String()
		s.Text = &text
		scenarios = append(scenarios, s)
	}
	return scenarios
}

func runHarness(t *testing.T, libraryDir string, scenarios []scenario) []harnessResult {
	t.Helper()

	script, err := filepath.Abs(filepath.Join("testdata", "harness.js"))
	if err != nil {
		t.Fatalf("locating the harness: %v", err)
	}
	cmd := exec.Command("node", script, libraryDir)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout: %v", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Skipf("cannot run node: %v", err)
	}

	go func() {
		encoder := json.NewEncoder(stdin)
		for _, s := range scenarios {
			if err := encoder.Encode(s); err != nil {
				break
			}
		}
		_ = stdin.Close()
	}()

	var results []harnessResult
	decoder := json.NewDecoder(bufio.NewReaderSize(stdout, 1<<20))
	for {
		var r harnessResult
		if err := decoder.Decode(&r); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("decoding harness output: %v (stderr: %s)", err, stderr.String())
		}
		results = append(results, r)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("harness exited with %v (stderr: %s)", err, stderr.String())
	}
	return results
}

func marshal(t *testing.T, v any) string {
	t.Helper()
	encoded, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	return string(encoded)
}

func describe(t *testing.T, s scenario) string {
	t.Helper()
	return "  scenario: " + marshal(t, s)
}
