package textot

import (
	"bufio"
	"encoding/json"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The golden tests pin the cases upstream thought to write down, and the
// convergence test proves this implementation is self-consistent. Neither shows
// that it agrees with the implementation it replaces: two transforms can each
// converge and still converge on different text.
//
// This runs both against the same random scenarios and compares every answer.
//
// Set TEXTOT_SHAREJS_TEXT to the vendored text.js to enable it:
//
//	TEXTOT_SHAREJS_TEXT=../../../services/document-updater/app/js/sharejs/types/text.js \
//	  go test ./internal/textot/ -run Differential
var differentialCases = 20000

type scenario struct {
	Snapshot string `json:"snapshot"`
	A        Op     `json:"a"`
	B        Op     `json:"b"`
}

type nodeResult struct {
	ALeft       Op     `json:"aLeft"`
	BRight      Op     `json:"bRight"`
	DocA        string `json:"docA"`
	DocB        string `json:"docB"`
	AfterAThenB string `json:"afterAThenB"`
	AfterBThenA string `json:"afterBThenA"`
	Error       string `json:"error"`
}

func TestDifferentialAgainstNode(t *testing.T) {
	textJS := os.Getenv("TEXTOT_SHAREJS_TEXT")
	if textJS == "" {
		t.Skip("set TEXTOT_SHAREJS_TEXT to the vendored sharejs text.js to run this")
	}
	textJS, err := filepath.Abs(textJS)
	if err != nil {
		t.Fatalf("resolving TEXTOT_SHAREJS_TEXT: %v", err)
	}
	if _, err := os.Stat(textJS); err != nil {
		t.Fatalf("TEXTOT_SHAREJS_TEXT: %v", err)
	}

	scenarios := generateScenarios(t, differentialCases)
	results := runNodeHarness(t, textJS, scenarios)
	if len(results) != len(scenarios) {
		t.Fatalf("harness returned %d results for %d scenarios", len(results), len(scenarios))
	}

	for i, s := range scenarios {
		want := results[i]
		if want.Error != "" {
			// Node refused the scenario; this port must refuse it too, though
			// the wording of the two errors is not expected to match.
			if _, err := transformBoth(s); err == nil {
				t.Fatalf("case %d: Node rejected this and Go accepted it: %s\n  S=%q\n  A=%s\n  B=%s",
					i, want.Error, s.Snapshot, mustJSON(t, s.A), mustJSON(t, s.B))
			}
			continue
		}

		got, err := transformBoth(s)
		if err != nil {
			t.Fatalf("case %d: Go rejected a scenario Node accepted: %v\n  S=%q\n  A=%s\n  B=%s",
				i, err, s.Snapshot, mustJSON(t, s.A), mustJSON(t, s.B))
		}

		if a, b := mustJSON(t, got.aLeft), mustJSON(t, want.ALeft); a != b {
			t.Fatalf("case %d: transform(A, B, left) differs\n  S=%q\n  A=%s\n  B=%s\n  go   %s\n  node %s",
				i, s.Snapshot, mustJSON(t, s.A), mustJSON(t, s.B), a, b)
		}
		if a, b := mustJSON(t, got.bRight), mustJSON(t, want.BRight); a != b {
			t.Fatalf("case %d: transform(B, A, right) differs\n  S=%q\n  A=%s\n  B=%s\n  go   %s\n  node %s",
				i, s.Snapshot, mustJSON(t, s.A), mustJSON(t, s.B), a, b)
		}
		if got.afterBThenA.String() != want.AfterBThenA {
			t.Fatalf("case %d: the resulting document differs\n  S=%q\n  A=%s\n  B=%s\n  go   %q\n  node %q",
				i, s.Snapshot, mustJSON(t, s.A), mustJSON(t, s.B),
				got.afterBThenA.String(), want.AfterBThenA)
		}
	}

	t.Logf("%d random scenarios produced identical results in both implementations", len(scenarios))
}

type transformed struct {
	aLeft, bRight            Op
	afterAThenB, afterBThenA Text
}

func transformBoth(s scenario) (transformed, error) {
	var out transformed
	var err error

	if out.aLeft, err = Transform(s.A.Clone(), s.B, Left); err != nil {
		return out, err
	}
	if out.bRight, err = Transform(s.B.Clone(), s.A, Right); err != nil {
		return out, err
	}
	docA, err := Apply(T(s.Snapshot), s.A)
	if err != nil {
		return out, err
	}
	docB, err := Apply(T(s.Snapshot), s.B)
	if err != nil {
		return out, err
	}
	if out.afterAThenB, err = Apply(docA, out.bRight); err != nil {
		return out, err
	}
	if out.afterBThenA, err = Apply(docB, out.aLeft); err != nil {
		return out, err
	}
	return out, nil
}

// generateScenarios builds random cases. The alphabet is restricted to the
// Basic Multilingual Plane here, unlike the convergence test: a scenario has to
// survive a JSON round trip through Node, and an operation that splits a
// surrogate pair cannot.
func generateScenarios(t *testing.T, n int) []scenario {
	t.Helper()
	rng := rand.New(rand.NewPCG(7, 8))
	saved := alphabet
	alphabet = []rune{'a', 'b', 'c', ' ', '\n', 'é', 'ß', '中', '日'}
	defer func() { alphabet = saved }()

	scenarios := make([]scenario, 0, n)
	for i := 0; i < n; i++ {
		snapshot := randomText(rng, 12)
		opA, _ := randomOp(rng, snapshot)
		opB, _ := randomOp(rng, snapshot)
		scenarios = append(scenarios, scenario{
			Snapshot: snapshot.String(), A: opA, B: opB,
		})
	}
	return scenarios
}

func runNodeHarness(t *testing.T, textJS string, scenarios []scenario) []nodeResult {
	t.Helper()

	harness, err := filepath.Abs(filepath.Join("testdata", "harness.js"))
	if err != nil {
		t.Fatalf("locating the harness: %v", err)
	}
	cmd := exec.Command("node", harness, textJS)
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

	var results []nodeResult
	reader := bufio.NewReaderSize(stdout, 1<<20)
	decoder := json.NewDecoder(reader)
	for {
		var r nodeResult
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
