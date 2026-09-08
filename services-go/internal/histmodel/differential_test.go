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
	"strconv"
	"strings"
	"testing"
)

// The history is the record of what a project was, and an operation written
// into it wrongly is not something anybody notices until they go back to look.
// Two implementations can each be self-consistent and disagree about how an
// operation normalises, where a compose splits a run, or which of two tracked
// marks survives -- and the disagreement is invisible until a version is
// restored.
//
// So the model is compared against the real overleaf-editor-core over random
// operations: the round trip, the lengths, the length after applying, and the
// composition of two.
//
//	OVERLEAF_EDITOR_CORE_DIR=../../../libraries/overleaf-editor-core \
//	  go test ./internal/histmodel/ -run Differential

var differentialCases = envInt("HIST_MODEL_CASES", 50000)

func envInt(name string, fallback int) int {
	if raw := os.Getenv(name); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			return value
		}
	}
	return fallback
}

type scenario struct {
	First []json.RawMessage `json:"first"`
	// Not omitempty: an operation with no steps is a valid operation, and
	// leaving it out would ask the two sides different questions.
	Second []json.RawMessage `json:"second"`
	Length *int              `json:"length,omitempty"`
	// TrackedChanges are the marks on the document the operation applies to.
	TrackedChanges []json.RawMessage `json:"trackedChanges"`
	// Comments are the threads on it.
	Comments []json.RawMessage `json:"comments"`
}

type result struct {
	RoundTrip     json.RawMessage `json:"roundTrip"`
	BaseLength    *int            `json:"baseLength"`
	TargetLength  *int            `json:"targetLength"`
	AppliedLength *int            `json:"appliedLength"`
	ApplyError    string          `json:"applyError"`
	Composed      json.RawMessage `json:"composed"`
	ComposeError  string          `json:"composeError"`

	TrackedChanges      json.RawMessage `json:"trackedChanges"`
	TrackedChangesError string          `json:"trackedChangesError"`

	Comments      json.RawMessage `json:"comments"`
	CommentsError string          `json:"commentsError"`
	Error         string          `json:"error"`
}

func TestDifferentialAgainstCore(t *testing.T) {
	libraryDir := os.Getenv("OVERLEAF_EDITOR_CORE_DIR")
	if libraryDir == "" {
		t.Skip("set OVERLEAF_EDITOR_CORE_DIR to the overleaf-editor-core library to run this")
	}
	libraryDir, err := filepath.Abs(libraryDir)
	if err != nil {
		t.Fatalf("resolving OVERLEAF_EDITOR_CORE_DIR: %v", err)
	}

	scenarios := generateScenarios(differentialCases)
	results := runHarness(t, libraryDir, scenarios)
	if len(results) != len(scenarios) {
		t.Fatalf("the harness returned %d results for %d scenarios",
			len(results), len(scenarios))
	}

	var compared int
	for i, s := range scenarios {
		want := results[i]
		encoded, _ := json.Marshal(s)

		first := NewTextOperation()
		err := json.Unmarshal(wrap(s.First), first)

		if want.Error != "" {
			if err == nil {
				t.Fatalf("case %d: the library refused this and the port accepted it: %s\n  %s",
					i, want.Error, encoded)
			}
			continue
		}
		if err != nil {
			t.Fatalf("case %d: the port refused this and the library accepted it: %v\n  %s",
				i, err, encoded)
		}
		compared++

		got, err := json.Marshal(first)
		if err != nil {
			t.Fatalf("case %d: encoding: %v", i, err)
		}
		if !sameJSON(string(want.RoundTrip), string(got)) {
			t.Fatalf("case %d: round trip\n  scenario: %s\n  core: %s\n  go:   %s",
				i, encoded, want.RoundTrip, got)
		}
		if want.BaseLength != nil && *want.BaseLength != first.BaseLength {
			t.Fatalf("case %d: base length: core %d, go %d\n  %s",
				i, *want.BaseLength, first.BaseLength, encoded)
		}
		if want.TargetLength != nil && *want.TargetLength != first.TargetLength {
			t.Fatalf("case %d: target length: core %d, go %d\n  %s",
				i, *want.TargetLength, first.TargetLength, encoded)
		}

		if s.Length != nil {
			length, err := first.ApplyToLength(*s.Length)
			if want.ApplyError != "" {
				if err == nil {
					t.Fatalf("case %d: applying to %d: core refused it (%s), the port gave %d\n  %s",
						i, *s.Length, want.ApplyError, length, encoded)
				}
			} else if err != nil {
				t.Fatalf("case %d: applying to %d: the port refused it: %v\n  %s",
					i, *s.Length, err, encoded)
			} else if want.AppliedLength != nil && *want.AppliedLength != length {
				t.Fatalf("case %d: length after applying: core %d, go %d\n  %s",
					i, *want.AppliedLength, length, encoded)
			}
		}

		if s.TrackedChanges != nil {
			var changes []TrackedChange
			if err := json.Unmarshal(mustMarshal(s.TrackedChanges), &changes); err == nil {
				list := NewTrackedChangeList(changes)
				err := list.ApplyTextOperation(first)
				switch {
				case want.TrackedChangesError != "":
					if err == nil {
						t.Fatalf("case %d: tracked changes: core refused it (%s), the port did not\n  %s",
							i, want.TrackedChangesError, encoded)
					}
				case err != nil:
					t.Fatalf("case %d: tracked changes: the port refused it: %v\n  %s",
						i, err, encoded)
				default:
					got, _ := json.Marshal(list)
					if !sameJSON(string(want.TrackedChanges), string(got)) {
						t.Fatalf("case %d: tracked changes\n  scenario: %s\n  core: %s\n  go:   %s",
							i, encoded, want.TrackedChanges, got)
					}
				}
			}
		}

		if s.Comments != nil {
			var comments []*Comment
			if err := json.Unmarshal(mustMarshal(s.Comments), &comments); err == nil {
				list := NewCommentList(comments)
				err := applyOperationToComments(list, first)
				switch {
				case want.CommentsError != "":
					if err == nil {
						t.Fatalf("case %d: comments: core refused it (%s), the port did not: %s",
							i, want.CommentsError, encoded)
					}
				case err != nil:
					t.Fatalf("case %d: comments: the port refused it: %v: %s", i, err, encoded)
				default:
					got, _ := json.Marshal(list)
					if !sameJSON(string(want.Comments), string(got)) {
						t.Fatalf("case %d: comments: %s: core %s, go %s",
							i, encoded, want.Comments, got)
					}
				}
			}
		}

		if s.Second != nil {
			second := NewTextOperation()
			if err := json.Unmarshal(wrap(s.Second), second); err != nil {
				// The second operation could not be read by the port; the
				// library would have failed the compose for its own reason.
				continue
			}
			composed, err := first.Compose(second)
			if want.ComposeError != "" {
				if err == nil {
					t.Fatalf("case %d: compose: core refused it (%s), the port did not\n  %s",
						i, want.ComposeError, encoded)
				}
				continue
			}
			if err != nil {
				t.Fatalf("case %d: compose: the port refused it: %v\n  %s", i, err, encoded)
			}
			gotComposed, _ := json.Marshal(composed)
			if !sameJSON(string(want.Composed), string(gotComposed)) {
				t.Fatalf("case %d: composed\n  scenario: %s\n  core: %s\n  go:   %s",
					i, encoded, want.Composed, gotComposed)
			}
		}
	}

	if compared == 0 {
		t.Fatal("every scenario was refused by both; the generator produces nothing usable")
	}
	t.Logf("%d of %d scenarios were read by both", compared, len(scenarios))
}

// wrap puts the steps in the envelope the operation is stored in.
func wrap(steps []json.RawMessage) []byte {
	encoded, _ := json.Marshal(map[string]any{"textOperation": steps})
	return encoded
}

// generateScenarios builds random operations, and a second one that fits the
// first so that composing them is a question worth asking.
func generateScenarios(count int) []scenario {
	random := rand.New(rand.NewPCG(11, 29))

	scenarios := make([]scenario, 0, count)
	for len(scenarios) < count {
		// The base length is chosen first and the steps are made to fit it, so
		// that most scenarios are operations both implementations accept.
		base := random.IntN(40)
		first, target := randomOperation(random, base)

		s := scenario{First: first}
		if random.IntN(2) == 0 {
			length := base
			if random.IntN(8) == 0 {
				// Sometimes the wrong length, to compare the refusals.
				length = random.IntN(40)
			}
			s.Length = &length
		}
		if random.IntN(2) == 0 {
			second, _ := randomOperation(random, target)
			s.Second = second
		}
		if random.IntN(2) == 0 {
			s.TrackedChanges = randomTrackedChanges(random, base)
		}
		if random.IntN(2) == 0 {
			s.Comments = randomComments(random, base)
		}
		scenarios = append(scenarios, s)
	}
	return scenarios
}

// randomOperation builds an operation over a document of the given length, and
// reports how long the document is afterwards.
func randomOperation(random *rand.Rand, base int) ([]json.RawMessage, int) {
	// Not nil: an operation with no steps is a valid one, and a nil slice would
	// be written as null, which is a different question.
	steps := []json.RawMessage{}
	var consumed, produced int

	// The text is drawn from a small alphabet so that a compose has runs to
	// split rather than a string of distinct characters.
	const alphabet = "abcxyz \n"

	for consumed < base || random.IntN(4) == 0 {
		switch random.IntN(3) {
		case 0: // retain
			if consumed >= base {
				continue
			}
			n := 1 + random.IntN(base-consumed)
			consumed += n
			produced += n
			steps = append(steps, retainStep(random, n))

		case 1: // insert
			n := 1 + random.IntN(4)
			var text strings.Builder
			for ; n > 0; n-- {
				text.WriteByte(alphabet[random.IntN(len(alphabet))])
			}
			produced += text.Len()
			steps = append(steps, insertStep(random, text.String()))

		default: // remove
			if consumed >= base {
				continue
			}
			n := 1 + random.IntN(base-consumed)
			consumed += n
			encoded, _ := json.Marshal(-n)
			steps = append(steps, encoded)
		}

		if len(steps) > 12 {
			break
		}
	}

	// Whatever is left has to be retained: an operation must cover the whole
	// document.
	if consumed < base {
		steps = append(steps, retainStep(random, base-consumed))
		produced += base - consumed
	}
	return steps, produced
}

func retainStep(random *rand.Rand, n int) json.RawMessage {
	if random.IntN(3) != 0 {
		encoded, _ := json.Marshal(n)
		return encoded
	}
	encoded, _ := json.Marshal(map[string]any{"r": n, "tracking": randomTracking(random)})
	return encoded
}

func insertStep(random *rand.Rand, text string) json.RawMessage {
	if random.IntN(3) != 0 {
		encoded, _ := json.Marshal(text)
		return encoded
	}
	fields := map[string]any{"i": text}
	if random.IntN(2) == 0 {
		fields["tracking"] = randomTracking(random)
	}
	if random.IntN(2) == 0 {
		fields["commentIds"] = []string{fmt.Sprintf("thread-%d", random.IntN(3))}
	}
	return mustMarshal(fields)
}

// randomTracking draws a mark, including the one that clears a mark.
func randomTracking(random *rand.Rand) map[string]any {
	switch random.IntN(3) {
	case 0:
		return map[string]any{"type": "none"}
	case 1:
		return map[string]any{
			"type": "insert", "userId": fmt.Sprintf("u%d", random.IntN(2)),
			"ts": timestamps[random.IntN(len(timestamps))],
		}
	default:
		return map[string]any{
			"type": "delete", "userId": fmt.Sprintf("u%d", random.IntN(2)),
			"ts": timestamps[random.IntN(len(timestamps))],
		}
	}
}

// A short list, so that two marks often agree and the merging is exercised.
var timestamps = []string{
	"2026-01-01T00:00:00.000Z",
	"2026-01-01T00:00:01.000Z",
	"2026-02-03T04:05:06.007Z",
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

func runHarness(t *testing.T, libraryDir string, scenarios []scenario) []result {
	t.Helper()

	harness, err := filepath.Abs(filepath.Join("testdata", "harness.js"))
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

	var results []result
	reader := bufio.NewReaderSize(stdout, 1<<20)
	for {
		line, err := reader.ReadBytes('\n')
		// A result is recognised by its shape: the library prints a banner
		// before anything else, and anything it logs would be taken for an
		// answer otherwise.
		if bytes.HasPrefix(bytes.TrimSpace(line), []byte("{")) {
			var r result
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

// randomTrackedChanges places marks over a document of the given length.
//
// They are laid down left to right and never overlap, which is what the list
// guarantees for a real document and what it refuses to be given.
func randomTrackedChanges(random *rand.Rand, base int) []json.RawMessage {
	changes := []json.RawMessage{}
	position := 0
	for c := random.IntN(4); c > 0 && position < base; c-- {
		position += random.IntN(4)
		if position >= base {
			break
		}
		length := 1 + random.IntN(minInt(6, base-position))
		changes = append(changes, mustMarshal(map[string]any{
			"range":    map[string]any{"pos": position, "length": length},
			"tracking": randomTrackingProps(random),
		}))
		position += length
	}
	return changes
}

// randomTrackingProps draws a mark. Unlike the one used for operations it never
// draws the clearing directive, which is not something a stored mark can be.
func randomTrackingProps(random *rand.Rand) map[string]any {
	kind := "insert"
	if random.IntN(2) == 0 {
		kind = "delete"
	}
	return map[string]any{
		"type": kind, "userId": fmt.Sprintf("u%d", random.IntN(2)),
		"ts": timestamps[random.IntN(len(timestamps))],
	}
}

// applyOperationToComments walks an operation over a comment list the way the
// file does when the operation is applied to it.
func applyOperationToComments(list *CommentList, operation *TextOperation) error {
	var cursor int
	for _, op := range operation.Ops {
		switch typed := op.(type) {
		case *RetainOp:
			cursor += typed.Length
		case *InsertOp:
			err := list.ApplyInsert(
				Range{Pos: cursor, Length: typed.Len()}, typed.CommentIDs)
			if err != nil {
				return err
			}
			cursor += typed.Len()
		case *RemoveOp:
			if err := list.ApplyDelete(Range{Pos: cursor, Length: typed.Length}); err != nil {
				return err
			}
		}
	}
	return nil
}

// randomComments places threads over a document of the given length.
//
// A thread can cover more than one stretch, which is what happens once an edit
// has split it, so some of them are generated that way to begin with.
func randomComments(random *rand.Rand, base int) []json.RawMessage {
	comments := []json.RawMessage{}
	for c := random.IntN(3); c > 0; c-- {
		ranges := []map[string]any{}
		position := random.IntN(maxInt(1, base))
		for r := 1 + random.IntN(2); r > 0 && position < base; r-- {
			length := 1 + random.IntN(minInt(5, maxInt(1, base-position)))
			ranges = append(ranges, map[string]any{"pos": position, "length": length})
			// A gap, so the two stretches do not merge back into one.
			position += length + 1 + random.IntN(3)
		}
		if len(ranges) == 0 {
			continue
		}
		comment := map[string]any{
			"id": fmt.Sprintf("thread-%d", len(comments)), "ranges": ranges,
		}
		if random.IntN(3) == 0 {
			comment["resolved"] = true
		}
		comments = append(comments, mustMarshal(comment))
	}
	return comments
}
