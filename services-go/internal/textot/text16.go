// Package textot implements the ShareJS "text" operational transformation
// type, as vendored in services/document-updater/app/js/sharejs/types/text.js.
//
// It is the only OT type document-updater actually uses: ShareJsDB.getSnapshot
// hardcodes `type: 'text'`, so the json, text-tp2 and text-composable types
// alongside it are unreachable.
//
// Every position and length in this package is counted in UTF-16 code units,
// because that is what a JavaScript string index is. Getting that wrong does
// not fail loudly -- it silently corrupts any document containing a character
// outside ASCII, and only when two people edit it at once.
package textot

import (
	"strings"
	"unicode/utf16"
)

// Text is a string measured the way JavaScript measures one.
//
// It exists so that the arithmetic below cannot accidentally be done in bytes
// or runes. Every offset in an operation indexes into one of these.
type Text []uint16

// T converts a Go string to its UTF-16 form.
func T(s string) Text { return utf16.Encode([]rune(s)) }

// String converts back to UTF-8.
//
// A lone surrogate -- which only arises if an operation splits a character
// outside the Basic Multilingual Plane -- decodes to U+FFFD, where JavaScript
// would have kept it. See TestSurrogatePairSplit for what that looks like.
func (t Text) String() string { return string(utf16.Decode(t)) }

// Len is the length in UTF-16 code units.
func (t Text) Len() int { return len(t) }

// Slice returns t[start:end], clamped to the bounds of t.
//
// It clamps rather than panicking because String.prototype.slice does: a
// position past the end of the string yields an empty result, and the OT code
// below relies on that in several places.
func (t Text) Slice(start, end int) Text {
	if start < 0 {
		start = 0
	}
	if end > len(t) {
		end = len(t)
	}
	if start >= end {
		return Text{}
	}
	return t[start:end]
}

// From returns t[start:], clamped.
func (t Text) From(start int) Text { return t.Slice(start, len(t)) }

// Equal reports whether two texts are identical.
func (t Text) Equal(other Text) bool {
	if len(t) != len(other) {
		return false
	}
	for i := range t {
		if t[i] != other[i] {
			return false
		}
	}
	return true
}

// Inject returns t with other inserted at pos.
func (t Text) Inject(pos int, other Text) Text {
	if pos < 0 {
		pos = 0
	}
	if pos > len(t) {
		pos = len(t)
	}
	out := make(Text, 0, len(t)+len(other))
	out = append(out, t[:pos]...)
	out = append(out, other...)
	out = append(out, t[pos:]...)
	return out
}

// Concat joins texts.
func Concat(parts ...Text) Text {
	n := 0
	for _, p := range parts {
		n += len(p)
	}
	out := make(Text, 0, n)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// SplitLines splits on the line endings the Node service splits on:
//
//	data.snapshot.split(/\r\n|\n|\r/)
//
// Documents are stored as an array of lines and joined with "\n" to make the
// snapshot, so this is the inverse of that join.
func SplitLines(t Text) []string {
	s := t.String()
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\r':
			lines = append(lines, s[start:i])
			if i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
			start = i + 1
		case '\n':
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	return append(lines, s[start:])
}

// JoinLines builds a snapshot from stored lines.
func JoinLines(lines []string) Text { return T(strings.Join(lines, "\n")) }
