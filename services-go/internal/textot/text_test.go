package textot

import (
	"encoding/json"
	"testing"
)

// The cases below are the ones in
// services/document-updater/test/unit/js/ShareJS/TextTransformTests.js,
// transcribed one for one. They are the only executable specification of what
// the transform is supposed to do, so they are kept in the same order and with
// the same names as the originals.

const thread = "mock-thread-id"

func ins(text string, pos int) Component {
	return Component{Kind: Insert, Text: T(text), Pos: pos}
}

func del(text string, pos int) Component {
	return Component{Kind: Delete, Text: T(text), Pos: pos}
}

func com(text string, pos int, t string) Component {
	return Component{Kind: Comment, Text: T(text), Pos: pos, Thread: t}
}

// opJSON renders an op the way the failure messages read best: as the wire
// form the Node tests are written in.
func opJSON(t *testing.T, op Op) string {
	t.Helper()
	if op == nil {
		return "null"
	}
	encoded, err := json.Marshal(op)
	if err != nil {
		t.Fatalf("encoding op: %v", err)
	}
	return string(encoded)
}

func checkTransform(t *testing.T, name string, c, otherC Component, side Side, want Op) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		got, err := TransformComponent(Op{}, c, otherC, side)
		if err != nil {
			t.Fatalf("TransformComponent: %v", err)
		}
		if opJSON(t, got) != opJSON(t, want) {
			t.Errorf("got %s, want %s", opJSON(t, got), opJSON(t, want))
		}
	})
}

func TestTransformInsertInsert(t *testing.T) {
	checkTransform(t, "with an insert before",
		ins("foo", 9), ins("bar", 3), Left, Op{ins("foo", 12)})
	checkTransform(t, "with an insert after",
		ins("foo", 3), ins("bar", 9), Left, Op{ins("foo", 3)})
	checkTransform(t, "with an insert at the same place with side == right",
		ins("foo", 3), ins("bar", 3), Right, Op{ins("foo", 6)})
	checkTransform(t, "with an insert at the same place with side == left",
		ins("foo", 3), ins("bar", 3), Left, Op{ins("foo", 3)})
}

func TestTransformInsertDelete(t *testing.T) {
	checkTransform(t, "with a delete before",
		ins("foo", 9), del("bar", 3), Left, Op{ins("foo", 6)})
	checkTransform(t, "with a delete after",
		ins("foo", 3), del("bar", 9), Left, Op{ins("foo", 3)})
	checkTransform(t, "with a delete at the same place with side == right",
		ins("foo", 3), del("bar", 3), Right, Op{ins("foo", 3)})
	checkTransform(t, "with a delete at the same place with side == left",
		ins("foo", 3), del("bar", 3), Left, Op{ins("foo", 3)})
}

func TestTransformDeleteInsert(t *testing.T) {
	checkTransform(t, "with an insert before",
		del("foo", 9), ins("bar", 3), Left, Op{del("foo", 12)})
	checkTransform(t, "with an insert after",
		del("foo", 3), ins("bar", 9), Left, Op{del("foo", 3)})
	checkTransform(t, "with an insert at the same place with side == right",
		del("foo", 3), ins("bar", 3), Right, Op{del("foo", 6)})
	checkTransform(t, "with an insert at the same place with side == left",
		del("foo", 3), ins("bar", 3), Left, Op{del("foo", 6)})
	// The insert landed inside the deleted range, so the delete splits in two.
	checkTransform(t, "with a delete that overlaps the insert location",
		del("foo", 3), ins("bar", 4), Left, Op{del("f", 3), del("oo", 6)})
}

func TestTransformDeleteDelete(t *testing.T) {
	checkTransform(t, "with a delete before",
		del("foo", 9), del("bar", 3), Left, Op{del("foo", 6)})
	checkTransform(t, "with a delete after",
		del("foo", 3), del("bar", 9), Left, Op{del("foo", 3)})
	// Somebody else already deleted it, so there is nothing left to do.
	checkTransform(t, "with deleting the same content",
		del("foo", 3), del("foo", 3), Right, Op{})
	checkTransform(t, "with the delete overlapping before",
		del("foobar", 3), del("abcfoo", 0), Right, Op{del("bar", 0)})
	checkTransform(t, "with the delete overlapping after",
		del("abcfoo", 3), del("foobar", 6), Left, Op{del("abc", 3)})
	checkTransform(t, "with the delete overlapping the whole delete",
		del("abcfoo123", 3), del("foo", 6), Left, Op{del("abc123", 3)})
	checkTransform(t, "with the delete inside the whole delete",
		del("foo", 6), del("abcfoo123", 3), Left, Op{})
}

func TestTransformCommentInsert(t *testing.T) {
	checkTransform(t, "with an insert before",
		com("foo", 9, thread), ins("bar", 3), Left, Op{com("foo", 12, thread)})
	checkTransform(t, "with an insert after",
		com("foo", 3, thread), ins("bar", 9), Left, Op{com("foo", 3, thread)})
	// RangesTracker does not inject inserts into comments on their edges, so
	// neither does this.
	checkTransform(t, "with an insert at the left edge",
		com("foo", 3, thread), ins("bar", 3), Left, Op{com("foo", 6, thread)})
	checkTransform(t, "with an insert at the right edge",
		com("foo", 3, thread), ins("bar", 6), Left, Op{com("foo", 3, thread)})
	checkTransform(t, "with an insert in the middle",
		com("foo", 3, thread), ins("bar", 5), Left, Op{com("fobaro", 3, thread)})
}

func TestTransformCommentDelete(t *testing.T) {
	checkTransform(t, "with a delete before",
		com("foo", 9, thread), del("bar", 3), Left, Op{com("foo", 6, thread)})
	checkTransform(t, "with a delete after",
		com("foo", 3, thread), ins("bar", 9), Left, Op{com("foo", 3, thread)})
	checkTransform(t, "with a delete overlapping the comment content before",
		com("foobar", 6, thread), del("123foo", 3), Left, Op{com("bar", 3, thread)})
	checkTransform(t, "with a delete overlapping the comment content after",
		com("foobar", 6, thread), del("bar123", 9), Left, Op{com("foo", 6, thread)})
	checkTransform(t, "with a delete overlapping the comment content in the middle",
		com("foo123bar", 6, thread), del("123", 9), Left, Op{com("foobar", 6, thread)})
	// The comment survives as an empty one; only inserts and deletes are
	// dropped when they empty out.
	checkTransform(t, "with a delete overlapping the whole comment",
		com("foo", 6, thread), del("123foo456", 3), Left, Op{com("", 3, thread)})
}

// A comment changes no text, so nothing transforms against it.
func TestTransformAgainstComment(t *testing.T) {
	checkTransform(t, "insert against comment",
		ins("foo", 6), com("bar", 3, ""), Left, Op{ins("foo", 6)})
	checkTransform(t, "delete against comment",
		del("foo", 6), com("bar", 3, ""), Left, Op{del("foo", 6)})
	checkTransform(t, "comment against comment",
		com("foo", 6, ""), com("bar", 3, ""), Left, Op{com("foo", 6, "")})
}

func TestApply(t *testing.T) {
	cases := []struct {
		name     string
		snapshot string
		op       Op
		want     string
		wantErr  error
	}{
		{"an insert", "foo", Op{ins("bar", 2)}, "fobaro", nil},
		{"a delete", "foo123bar", Op{del("123", 3)}, "foobar", nil},
		{"a comment changes nothing", "foo123bar", Op{com("123", 3, thread)}, "foo123bar", nil},
		{"deleted content that does not match", "foo123bar", Op{del("456", 3)}, "", ErrDeleteMismatch},
		{"comment content that does not match", "foo123bar", Op{com("456", 3, thread)}, "", ErrCommentMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Apply(T(tc.snapshot), tc.op)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("Apply = %q, want an error", got.String())
				}
				return
			}
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if got.String() != tc.want {
				t.Errorf("Apply = %q, want %q", got.String(), tc.want)
			}
		})
	}
}

// Positions index UTF-16 code units, not bytes and not characters. A port that
// counted either of the others would corrupt every document containing an
// accent, and only when two people edited it at once.
func TestPositionsAreUTF16CodeUnits(t *testing.T) {
	// "é" is two bytes in UTF-8 and one UTF-16 code unit, so an insert after
	// it sits at position 1.
	got, err := Apply(T("é"), Op{ins("x", 1)})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got.String() != "éx" {
		t.Errorf("Apply = %q, want %q", got.String(), "éx")
	}

	// An emoji is four bytes in UTF-8, one rune, and two UTF-16 code units, so
	// the text after it starts at position 2.
	got, err = Apply(T("\U0001F600ab"), Op{del("ab", 2)})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got.String() != "\U0001F600" {
		t.Errorf("Apply = %q, want just the emoji", got.String())
	}

	// The same in a transform: an insert of one emoji shifts later positions
	// by two, not by one or four.
	out, err := TransformComponent(Op{}, ins("x", 5), ins("\U0001F600", 0), Left)
	if err != nil {
		t.Fatalf("TransformComponent: %v", err)
	}
	if out[0].Pos != 7 {
		t.Errorf("position = %d, want 7", out[0].Pos)
	}
}

// An operation can in principle land between the halves of a surrogate pair.
// JavaScript keeps the lone surrogate; Go cannot represent one in a string, so
// it becomes U+FFFD. Nothing produces such an operation in practice -- the
// editor counts in UTF-16 too -- but the behaviour is pinned here so a change
// to it is deliberate.
func TestSurrogatePairSplit(t *testing.T) {
	got, err := Apply(T("\U0001F600"), Op{ins("x", 1)})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got.Len() != 3 {
		t.Errorf("length = %d code units, want 3", got.Len())
	}
	if runes := []rune(got.String()); len(runes) != 3 || runes[1] != 'x' {
		t.Errorf("got %q, want a replacement char, an x, and a replacement char", got.String())
	}
}

func TestAppendMergesAdjacentComponents(t *testing.T) {
	// Two inserts that touch become one.
	op := Normalize(Op{ins("foo", 3), ins("bar", 6)})
	if opJSON(t, op) != opJSON(t, Op{ins("foobar", 3)}) {
		t.Errorf("got %s, want a single merged insert", opJSON(t, op))
	}

	// Two deletes at the same position become one, in the order the original
	// document had them: deleting "bar" then "foo" removes "barfoo".
	op = Normalize(Op{del("bar", 3), del("foo", 3)})
	if opJSON(t, op) != opJSON(t, Op{del("barfoo", 3)}) {
		t.Errorf("got %s, want a single merged delete", opJSON(t, op))
	}

	// Empty inserts and deletes are dropped entirely.
	op = Normalize(Op{ins("", 3), del("", 5)})
	if len(op) != 0 {
		t.Errorf("got %s, want nothing", opJSON(t, op))
	}

	// Components that do not touch stay separate.
	op = Normalize(Op{ins("foo", 3), ins("bar", 30)})
	if len(op) != 2 {
		t.Errorf("got %s, want two components", opJSON(t, op))
	}
}

func TestCompose(t *testing.T) {
	got, err := Compose(Op{ins("foo", 3)}, Op{ins("bar", 6)})
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if opJSON(t, got) != opJSON(t, Op{ins("foobar", 3)}) {
		t.Errorf("got %s, want the two inserts merged", opJSON(t, got))
	}

	// Composing must not alter its inputs.
	first := Op{ins("foo", 3)}
	if _, err := Compose(first, Op{ins("bar", 6)}); err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if opJSON(t, first) != opJSON(t, Op{ins("foo", 3)}) {
		t.Errorf("Compose modified its input: %s", opJSON(t, first))
	}
}

func TestInvert(t *testing.T) {
	op := Op{ins("foo", 3), del("bar", 10)}
	inverted := Invert(op)
	want := Op{ins("bar", 10), del("foo", 3)}
	if opJSON(t, inverted) != opJSON(t, want) {
		t.Errorf("got %s, want %s", opJSON(t, inverted), opJSON(t, want))
	}

	// Applying an operation and then its inverse returns the original text.
	snapshot := T("abcdefghijklmnop")
	after, err := Apply(snapshot, Op{del("def", 3)})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	back, err := Apply(after, Invert(Op{del("def", 3)}))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if back.String() != snapshot.String() {
		t.Errorf("round trip = %q, want %q", back.String(), snapshot.String())
	}
}

func TestTransformCursorFollowsEdits(t *testing.T) {
	op := Op{ins("hello", 0)}
	if got := TransformCursor(3, op, Left); got != 8 {
		t.Errorf("cursor = %d, want 8", got)
	}
	// A cursor exactly on an insert goes before it on the left, after it on
	// the right.
	if got := TransformCursor(0, op, Left); got != 0 {
		t.Errorf("cursor = %d, want 0", got)
	}
	if got := TransformCursor(0, op, Right); got != 5 {
		t.Errorf("cursor = %d, want 5", got)
	}
	// A cursor inside deleted text collapses to where the text was.
	if got := TransformCursor(5, Op{del("abc", 3)}, Left); got != 3 {
		t.Errorf("cursor = %d, want 3", got)
	}
}

func TestComponentJSON(t *testing.T) {
	cases := []struct {
		json string
		want Component
	}{
		{`{"i":"foo","p":3}`, ins("foo", 3)},
		{`{"d":"foo","p":3}`, del("foo", 3)},
		{`{"c":"foo","p":3,"t":"thread-1"}`, com("foo", 3, "thread-1")},
		{`{"i":"foo","p":3,"u":true}`, Component{Kind: Insert, Text: T("foo"), Pos: 3, Undo: true}},
	}
	for _, tc := range cases {
		var got Component
		if err := json.Unmarshal([]byte(tc.json), &got); err != nil {
			t.Errorf("decoding %s: %v", tc.json, err)
			continue
		}
		if opJSON(t, Op{got}) != opJSON(t, Op{tc.want}) {
			t.Errorf("decoding %s gave %s", tc.json, opJSON(t, Op{got}))
		}
		encoded, err := json.Marshal(tc.want)
		if err != nil {
			t.Fatalf("encoding: %v", err)
		}
		var round Component
		if err := json.Unmarshal(encoded, &round); err != nil {
			t.Fatalf("round trip: %v", err)
		}
		if opJSON(t, Op{round}) != opJSON(t, Op{tc.want}) {
			t.Errorf("round trip of %s gave %s", tc.json, string(encoded))
		}
	}

	// A component that names no operation, or more than one, is refused.
	for _, bad := range []string{`{"p":3}`, `{"i":"a","d":"b","p":3}`, `{"i":"a","p":-1}`} {
		var got Component
		if err := json.Unmarshal([]byte(bad), &got); err == nil {
			t.Errorf("decoding %s should have failed", bad)
		}
	}
}

// The editor still sends a single unwrapped component for one keystroke.
func TestOpAcceptsSingleComponent(t *testing.T) {
	var op Op
	if err := json.Unmarshal([]byte(`{"i":"a","p":0}`), &op); err != nil {
		t.Fatalf("decoding a single component: %v", err)
	}
	if len(op) != 1 || op[0].Kind != Insert {
		t.Errorf("got %s, want a one-component op", opJSON(t, op))
	}

	if err := json.Unmarshal([]byte(`[{"i":"a","p":0},{"d":"b","p":5}]`), &op); err != nil {
		t.Fatalf("decoding a list: %v", err)
	}
	if len(op) != 2 {
		t.Errorf("got %s, want two components", opJSON(t, op))
	}
}

// Two operations that claim to have deleted different text from the same place
// were built against documents that cannot both have existed. Node throws here
// and so does this, because silently picking one of the two would write the
// disagreement into the stored document.
func TestConcurrentDeletesMustAgree(t *testing.T) {
	if _, err := TransformComponent(Op{}, del("abc", 0), del("xbc", 0), Left); err == nil {
		t.Error("two deletes disagreeing about the same region should be refused")
	}
	if _, err := TransformComponent(Op{}, com("abc", 0, thread), del("xbc", 0), Left); err == nil {
		t.Error("a comment disagreeing with a delete should be refused")
	}

	// Agreeing on the overlap is fine, even when the ranges differ.
	got, err := TransformComponent(Op{}, del("abcd", 0), del("bc", 1), Left)
	if err != nil {
		t.Fatalf("deletes that agree should transform: %v", err)
	}
	if opJSON(t, got) != opJSON(t, Op{del("ad", 0)}) {
		t.Errorf("got %s, want the non-overlapping parts", opJSON(t, got))
	}
}

// Documents are stored as an array of lines and joined into a snapshot with a
// newline, but split back apart on any of the three line endings, because the
// Node service splits on /\r\n|\n|\r/. A document pasted from Windows or from
// an old Mac editor gains or loses lines if this does not match.
func TestSplitLines(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"a\nb", []string{"a", "b"}},
		{"a\r\nb", []string{"a", "b"}},
		{"a\rb", []string{"a", "b"}},
		{"", []string{""}},
		{"a\n", []string{"a", ""}},
		{"\n", []string{"", ""}},
		{"a\r\n\nb", []string{"a", "", "b"}},
		// A newline followed by a carriage return is two breaks, not one: only
		// the \r\n order is treated as a single ending.
		{"a\n\rb", []string{"a", "", "b"}},
	}
	for _, tc := range cases {
		got := SplitLines(T(tc.in))
		if len(got) != len(tc.want) {
			t.Errorf("SplitLines(%q) = %q, want %q", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("SplitLines(%q) = %q, want %q", tc.in, got, tc.want)
				break
			}
		}
	}
}

func TestJoinLinesRoundTrip(t *testing.T) {
	lines := []string{`\documentclass{article}`, "", "café 中文", `\end{document}`}
	got := SplitLines(JoinLines(lines))
	if len(got) != len(lines) {
		t.Fatalf("round trip gave %d lines, want %d", len(got), len(lines))
	}
	for i := range got {
		if got[i] != lines[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], lines[i])
		}
	}
}
