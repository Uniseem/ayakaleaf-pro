// Package textdiff finds the difference between two versions of a document.
//
// It is a port of the diff half of diff-match-patch, the library the Node
// service uses, and it answers exactly as that library does for the same input.
// That matters because the answer is not unique: several correct diffs rebuild
// the same document, and which one is chosen is what the other editors are
// shown, what the tracked changes are moved against, and what goes into the
// history. Two implementations that each find a correct difference and find
// different ones would leave the two services visibly disagreeing.
//
// The Go port on offer was not usable for this. It works in characters and in
// bytes where the original works in UTF-16 code units, so it searches different
// places on any document with an emoji in it; it scores one side of a boundary
// with the wrong pattern; and it leaves emptied components in the result. Each
// difference is harmless on its own and none produces a wrong document, but
// together they mean a different operation for a large share of real edits.
//
// Everything here works in UTF-16 code units, which is what JavaScript counts
// and what every position in this codebase is expressed in.
package textdiff

import "time"

// Kind is what one element of a diff says about a piece of text, using the
// values the reference implementation uses.
type Kind int

// The three kinds.
const (
	Delete Kind = -1
	Equal  Kind = 0
	Insert Kind = 1
)

// Diff is one run of text and what became of it.
type Diff struct {
	Kind Kind
	Text []uint16
}

// lineModeThreshold is the size at which the diff first compares whole lines
// rather than characters. Below it the character diff is affordable.
const lineModeThreshold = 100

// Main returns the difference between two texts.
//
// checkLines asks for the line-first shortcut on large inputs, which is what
// the callers want; it is turned off for the sub-diffs that shortcut produces.
//
// timeout bounds the search. When it runs out the result is coarser but still
// correct, so the cost of a slow machine is a larger operation rather than a
// wrong one. A timeout of zero means no limit, and also turns off the
// half-match shortcut, which trades optimality for speed.
func Main(text1, text2 []uint16, checkLines bool, timeout time.Duration) []Diff {
	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	return diffMain(text1, text2, checkLines, deadline, timeout > 0)
}

func diffMain(text1, text2 []uint16, checkLines bool, deadline time.Time, halfMatch bool) []Diff {
	if equalText(text1, text2) {
		if len(text1) > 0 {
			return []Diff{{Kind: Equal, Text: text1}}
		}
		return []Diff{}
	}

	// The parts at either end that did not change are set aside, so the search
	// only has to look at the middle.
	common := commonPrefix(text1, text2)
	prefix := text1[:common]
	text1, text2 = text1[common:], text2[common:]

	common = commonSuffix(text1, text2)
	suffix := text1[len(text1)-common:]
	text1, text2 = text1[:len(text1)-common], text2[:len(text2)-common]

	diffs := diffCompute(text1, text2, checkLines, deadline, halfMatch)

	if len(prefix) > 0 {
		diffs = append([]Diff{{Kind: Equal, Text: prefix}}, diffs...)
	}
	if len(suffix) > 0 {
		diffs = append(diffs, Diff{Kind: Equal, Text: suffix})
	}
	return CleanupMerge(diffs)
}

// diffCompute works on two texts with nothing in common at either end.
func diffCompute(text1, text2 []uint16, checkLines bool, deadline time.Time, halfMatch bool) []Diff {
	if len(text1) == 0 {
		return []Diff{{Kind: Insert, Text: text2}}
	}
	if len(text2) == 0 {
		return []Diff{{Kind: Delete, Text: text1}}
	}

	longText, shortText := text1, text2
	if len(text1) <= len(text2) {
		longText, shortText = text2, text1
	}

	// The whole of one text inside the other is an insertion or a deletion
	// around it, and needs no search.
	if i := indexOf(longText, shortText, 0); i != -1 {
		kind := Insert
		if len(text1) > len(text2) {
			kind = Delete
		}
		return []Diff{
			{Kind: kind, Text: longText[:i]},
			{Kind: Equal, Text: shortText},
			{Kind: kind, Text: longText[i+len(shortText):]},
		}
	}

	if len(shortText) == 1 {
		// One code unit, and the check above says it is not in the other text.
		return []Diff{{Kind: Delete, Text: text1}, {Kind: Insert, Text: text2}}
	}

	// A long enough run of text common to both splits the problem in two, which
	// is much cheaper than searching the whole of it.
	if halfMatch {
		if hm := halfMatchOf(text1, text2); hm != nil {
			before := diffMain(hm.text1A, hm.text2A, checkLines, deadline, halfMatch)
			after := diffMain(hm.text1B, hm.text2B, checkLines, deadline, halfMatch)
			result := append(before, Diff{Kind: Equal, Text: hm.common})
			return append(result, after...)
		}
	}

	if checkLines && len(text1) > lineModeThreshold && len(text2) > lineModeThreshold {
		return diffLineMode(text1, text2, deadline, halfMatch)
	}
	return bisect(text1, text2, deadline, halfMatch)
}

// diffLineMode compares whole lines first and only then looks inside the parts
// that changed. On a document where a few lines were edited that is the
// difference between searching a paragraph and searching the file.
func diffLineMode(text1, text2 []uint16, deadline time.Time, halfMatch bool) []Diff {
	encoded1, encoded2, lines := linesToChars(text1, text2)

	diffs := diffMain(encoded1, encoded2, false, deadline, halfMatch)
	diffs = charsToLines(diffs, lines)
	// A blank line matching another blank line half a file away is a match by
	// coincidence, and this is what throws those out.
	diffs = CleanupSemantic(diffs)

	// Now look inside each block that was replaced, code unit by code unit.
	diffs = append(diffs, Diff{Kind: Equal})
	pointer := 0
	countDelete, countInsert := 0, 0
	var textDelete, textInsert []uint16

	for pointer < len(diffs) {
		switch diffs[pointer].Kind {
		case Insert:
			countInsert++
			textInsert = concat(textInsert, diffs[pointer].Text)
		case Delete:
			countDelete++
			textDelete = concat(textDelete, diffs[pointer].Text)
		case Equal:
			if countDelete >= 1 && countInsert >= 1 {
				at := pointer - countDelete - countInsert
				diffs = splice(diffs, at, countDelete+countInsert)
				pointer = at
				inner := diffMain(textDelete, textInsert, false, deadline, halfMatch)
				diffs = splice(diffs, pointer, 0, inner...)
				pointer += len(inner)
			}
			countInsert, countDelete = 0, 0
			textDelete, textInsert = nil, nil
		}
		pointer++
	}
	return diffs[:len(diffs)-1]
}

// bisect finds the middle of the shortest edit path between two texts and
// recurses on the two halves.
func bisect(text1, text2 []uint16, deadline time.Time, halfMatch bool) []Diff {
	length1, length2 := len(text1), len(text2)
	maxD := (length1 + length2 + 1) / 2
	offset := maxD
	vLength := 2 * maxD

	v1 := make([]int, vLength)
	v2 := make([]int, vLength)
	for i := range v1 {
		v1[i], v2[i] = -1, -1
	}
	if offset+1 < vLength {
		v1[offset+1], v2[offset+1] = 0, 0
	}

	delta := length1 - length2
	// With an odd total the forward path is the one that will meet the reverse
	// path, so only that direction has to check for the overlap.
	front := delta%2 != 0

	k1start, k1end, k2start, k2end := 0, 0, 0, 0
	for d := 0; d < maxD; d++ {
		if !deadline.IsZero() && time.Now().After(deadline) {
			break
		}

		for k1 := -d + k1start; k1 <= d-k1end; k1 += 2 {
			k1Offset := offset + k1
			var x1 int
			if k1 == -d || (k1 != d && v1[k1Offset-1] < v1[k1Offset+1]) {
				x1 = v1[k1Offset+1]
			} else {
				x1 = v1[k1Offset-1] + 1
			}
			y1 := x1 - k1
			for x1 < length1 && y1 < length2 && text1[x1] == text2[y1] {
				x1++
				y1++
			}
			v1[k1Offset] = x1

			switch {
			case x1 > length1:
				k1end += 2
			case y1 > length2:
				k1start += 2
			case front:
				k2Offset := offset + delta - k1
				if k2Offset >= 0 && k2Offset < vLength && v2[k2Offset] != -1 {
					// The reverse path measures from the far end.
					x2 := length1 - v2[k2Offset]
					if x1 >= x2 {
						return bisectSplit(text1, text2, x1, y1, deadline, halfMatch)
					}
				}
			}
		}

		for k2 := -d + k2start; k2 <= d-k2end; k2 += 2 {
			k2Offset := offset + k2
			var x2 int
			if k2 == -d || (k2 != d && v2[k2Offset-1] < v2[k2Offset+1]) {
				x2 = v2[k2Offset+1]
			} else {
				x2 = v2[k2Offset-1] + 1
			}
			y2 := x2 - k2
			for x2 < length1 && y2 < length2 &&
				text1[length1-x2-1] == text2[length2-y2-1] {
				x2++
				y2++
			}
			v2[k2Offset] = x2

			switch {
			case x2 > length1:
				k2end += 2
			case y2 > length2:
				k2start += 2
			case !front:
				k1Offset := offset + delta - k2
				if k1Offset >= 0 && k1Offset < vLength && v1[k1Offset] != -1 {
					x1 := v1[k1Offset]
					y1 := offset + x1 - k1Offset
					if x1 >= length1-x2 {
						return bisectSplit(text1, text2, x1, y1, deadline, halfMatch)
					}
				}
			}
		}
	}

	// Out of time, or the two texts have nothing in common at all.
	return []Diff{{Kind: Delete, Text: text1}, {Kind: Insert, Text: text2}}
}

func bisectSplit(text1, text2 []uint16, x, y int, deadline time.Time, halfMatch bool) []Diff {
	before := diffMain(text1[:x], text2[:y], false, deadline, halfMatch)
	after := diffMain(text1[x:], text2[y:], false, deadline, halfMatch)
	return append(before, after...)
}
