package textdiff

import (
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

// These are sanity checks that run without Node. The real verification is the
// differential test in internal/docupdater, which puts a million random
// documents through this and through diff-match-patch itself and compares every
// answer; what is here is what should fail first and most legibly when
// something is wrong.

func text(s string) []uint16 { return utf16.Encode([]rune(s)) }

func render(diffs []Diff) string {
	var out strings.Builder
	for _, diff := range diffs {
		switch diff.Kind {
		case Delete:
			out.WriteString("-")
		case Insert:
			out.WriteString("+")
		case Equal:
			out.WriteString("=")
		}
		out.WriteString(string(utf16.Decode(diff.Text)))
		out.WriteString("|")
	}
	return out.String()
}

func diff(before, after string) []Diff {
	return CleanupSemantic(Main(text(before), text(after), true, 100*time.Millisecond))
}

func TestDiff(t *testing.T) {
	cases := []struct {
		name          string
		before, after string
		want          string
	}{
		{"nothing changed", "abc", "abc", "=abc|"},
		{"everything added", "", "abc", "+abc|"},
		{"everything removed", "abc", "", "-abc|"},
		{"a word replaced", "the quick fox", "the slow fox", "=the |-quick|+slow|= fox|"},
		{"inserted in the middle", "ab", "axb", "=a|+x|=b|"},
		// The edit slides to the word boundary rather than sitting inside a
		// word: "The c<ins>at c</ins>ame" reads worse than "The <ins>cat </ins>came".
		{"aligned to a word", "The came.", "The cat came.", "=The |+cat |=came.|"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := render(diff(c.before, c.after)); got != c.want {
				t.Errorf("\n  want: %s\n  got:  %s", c.want, got)
			}
		})
	}
}

// A paragraph taken out of the middle of a document is reported with the blank
// line before it rather than the one after it. Which of the two is chosen is
// the difference the ported boundary scoring exists for.
func TestDiffPrefersTheBlankLineBefore(t *testing.T) {
	before := "first\n\nsecond\nthird"
	after := "first\nthird"
	if got, want := render(diff(before, after)), "=first|-\n\nsecond|=\nthird|"; got != want {
		t.Errorf("\n  want: %s\n  got:  %s", want, got)
	}
}

// Nothing empty survives into the result: an operation with a zero-length
// component in it applies the same and reads as though something were missing.
func TestDiffHasNoEmptyComponents(t *testing.T) {
	before := "x   indented x \n  indented  indented quick \n   indented "
	after := "  indented  indented quick  fox\n   indented  fox"
	for i, d := range diff(before, after) {
		if len(d.Text) == 0 {
			t.Errorf("component %d is empty: %s", i, render(diff(before, after)))
		}
	}
}

// The line-first path is taken above a hundred characters, and has to agree
// with the character-by-character one about what changed.
func TestDiffOverTheLineModeThreshold(t *testing.T) {
	var before, after strings.Builder
	for i := 0; i < 40; i++ {
		before.WriteString("the quick brown fox\n")
		after.WriteString("the quick brown fox\n")
	}
	after.WriteString("one more line\n")

	diffs := diff(before.String(), after.String())
	if len(diffs) != 2 || diffs[0].Kind != Equal || diffs[1].Kind != Insert {
		t.Fatalf("expected an equality and an insert, got %s", render(diffs))
	}
	if got := string(utf16.Decode(diffs[1].Text)); got != "one more line\n" {
		t.Errorf("inserted %q", got)
	}
}

// Positions are counted in UTF-16 code units, so an astral character counts as
// two. Getting this wrong puts every later position out by one per emoji.
func TestDiffCountsCodeUnits(t *testing.T) {
	diffs := diff("a🎉b", "a🎉xb")
	var before int
	for _, d := range diffs {
		if d.Kind == Insert {
			break
		}
		before += len(d.Text)
	}
	if before != 3 {
		t.Errorf("the insert is at %d code units in, want 3: %s", before, render(diffs))
	}
}

// Applying what came back has to rebuild the document, whatever route through
// the algorithm was taken.
func TestDiffRebuildsTheDocument(t *testing.T) {
	cases := [][2]string{
		{"", ""},
		{"a", "b"},
		{"café 中文 🎉", "🎉 café 中文"},
		{strings.Repeat("line\n", 50), strings.Repeat("line\n", 25) + "x\n"},
		{"one\ntwo\nthree", "three\ntwo\none"},
	}
	for _, c := range cases {
		var rebuilt []uint16
		for _, d := range diff(c[0], c[1]) {
			if d.Kind != Delete {
				rebuilt = append(rebuilt, d.Text...)
			}
		}
		if got := string(utf16.Decode(rebuilt)); got != c[1] {
			t.Errorf("%q -> %q rebuilt as %q", c[0], c[1], got)
		}
	}
}
