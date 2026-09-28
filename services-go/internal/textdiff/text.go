package textdiff

// The helpers the diff is built out of. All of them count and compare UTF-16
// code units, which is the unit the algorithm measures everything in.

func equalText(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// commonPrefix is how much of the start the two texts share.
func commonPrefix(a, b []uint16) int {
	n := minInt(len(a), len(b))
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// commonSuffix is how much of the end the two texts share.
func commonSuffix(a, b []uint16) int {
	n := minInt(len(a), len(b))
	for i := 1; i <= n; i++ {
		if a[len(a)-i] != b[len(b)-i] {
			return i - 1
		}
	}
	return n
}

// commonOverlap is the longest run at the end of a that also starts b.
func commonOverlap(a, b []uint16) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	// Only the ends can overlap, so the rest is trimmed away first.
	if len(a) > len(b) {
		a = a[len(a)-len(b):]
	} else if len(a) < len(b) {
		b = b[:len(a)]
	}
	length := minInt(len(a), len(b))
	if equalText(a, b) {
		return length
	}

	// Start from a run of one and grow it for as long as it still matches.
	best, size := 0, 1
	for {
		pattern := a[length-size:]
		found := indexOf(b, pattern, 0)
		if found == -1 {
			return best
		}
		size += found
		if size > length {
			return best
		}
		if found == 0 || equalText(a[length-size:], b[:size]) {
			best = size
			size++
			if size > length {
				return best
			}
		}
	}
}

// indexOf is the first position of needle in haystack at or after start, or -1.
func indexOf(haystack, needle []uint16, start int) int {
	if len(needle) == 0 {
		if start > len(haystack) {
			return -1
		}
		return start
	}
	if start < 0 {
		start = 0
	}
	for i := start; i+len(needle) <= len(haystack); i++ {
		if equalText(haystack[i:i+len(needle)], needle) {
			return i
		}
	}
	return -1
}

// concat joins two texts into a text that shares neither.
func concat(a, b []uint16) []uint16 {
	joined := make([]uint16, 0, len(a)+len(b))
	joined = append(joined, a...)
	return append(joined, b...)
}

// splice replaces count elements at index with the given ones, the way the
// reference implementation uses Array.splice.
func splice(diffs []Diff, index, count int, insert ...Diff) []Diff {
	result := make([]Diff, 0, len(diffs)-count+len(insert))
	result = append(result, diffs[:index]...)
	result = append(result, insert...)
	return append(result, diffs[index+count:]...)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// halfMatch is a run of text long enough to split the problem around.
type halfMatch struct {
	text1A, text1B []uint16
	text2A, text2B []uint16
	common         []uint16
}

// halfMatchOf looks for a run of text, at least half the length of the longer
// of the two, that appears in both.
//
// Splitting there costs some optimality and saves a great deal of searching,
// which is why it only runs when there is a deadline to respect.
func halfMatchOf(text1, text2 []uint16) *halfMatch {
	longText, shortText := text1, text2
	if len(text1) <= len(text2) {
		longText, shortText = text2, text1
	}
	if len(longText) < 4 || len(shortText)*2 < len(longText) {
		// Too short to split, or too different for a half to be shared.
		return nil
	}

	// A quarter of the way in and half of the way in, which between them catch
	// a shared run wherever it sits.
	first := halfMatchAt(longText, shortText, (len(longText)+3)/4)
	second := halfMatchAt(longText, shortText, (len(longText)+1)/2)

	var found *halfMatch
	switch {
	case first == nil && second == nil:
		return nil
	case second == nil:
		found = first
	case first == nil:
		found = second
	case len(first.common) > len(second.common):
		found = first
	default:
		found = second
	}

	// halfMatchAt answered about the longer text; which of the two that is
	// decides how the answer is read back.
	if len(text1) > len(text2) {
		return found
	}
	return &halfMatch{
		text1A: found.text2A, text1B: found.text2B,
		text2A: found.text1A, text2B: found.text1B,
		common: found.common,
	}
}

// halfMatchAt looks for a shared run seeded by the quarter-length piece of the
// longer text starting at i.
func halfMatchAt(longText, shortText []uint16, i int) *halfMatch {
	seed := longText[i:minInt(len(longText), i+len(longText)/4)]

	var best halfMatch
	for j := indexOf(shortText, seed, 0); j != -1; j = indexOf(shortText, seed, j+1) {
		prefix := commonPrefix(longText[i:], shortText[j:])
		suffix := commonSuffix(longText[:i], shortText[:j])
		if len(best.common) < suffix+prefix {
			best.common = concat(shortText[j-suffix:j], shortText[j:j+prefix])
			best.text1A = longText[:i-suffix]
			best.text1B = longText[i+prefix:]
			best.text2A = shortText[:j-suffix]
			best.text2B = shortText[j+prefix:]
		}
	}
	if len(best.common)*2 >= len(longText) {
		return &best
	}
	return nil
}

// maxLinesFirstPass and maxLinesSecondPass bound how many distinct lines the
// line-first comparison can name.
//
// Each line becomes one code unit, so there are only so many to go round. Two
// thirds of them are set aside for the first text; past the limit the rest of
// that text is treated as one long line, which still gives a correct answer.
const (
	maxLinesFirstPass  = 40000
	maxLinesSecondPass = 65535
)

// linesToChars rewrites both texts with one code unit standing for each
// distinct line, so the diff can compare lines at the cost of comparing
// characters.
func linesToChars(text1, text2 []uint16) (encoded1, encoded2 []uint16, lines [][]uint16) {
	// Index zero is never used, so no line is encoded as a NUL.
	lines = [][]uint16{nil}
	seen := map[string]int{}

	encode := func(text []uint16, maxLines int) []uint16 {
		var encoded []uint16
		lineStart, lineEnd := 0, -1
		for lineEnd < len(text)-1 {
			lineEnd = indexOfUnit(text, '\n', lineStart)
			if lineEnd == -1 {
				lineEnd = len(text) - 1
			}
			line := text[lineStart : lineEnd+1]

			if at, ok := seen[lineKey(line)]; ok {
				encoded = append(encoded, uint16(at))
			} else {
				if len(lines) == maxLines {
					// No room for another line: whatever is left becomes one.
					line = text[lineStart:]
					lineEnd = len(text)
				}
				encoded = append(encoded, uint16(len(lines)))
				seen[lineKey(line)] = len(lines)
				lines = append(lines, line)
			}
			lineStart = lineEnd + 1
		}
		return encoded
	}

	encoded1 = encode(text1, maxLinesFirstPass)
	encoded2 = encode(text2, maxLinesSecondPass)
	return encoded1, encoded2, lines
}

// charsToLines puts the lines back in place of the code units standing for
// them.
func charsToLines(diffs []Diff, lines [][]uint16) []Diff {
	for i := range diffs {
		var text []uint16
		for _, unit := range diffs[i].Text {
			text = append(text, lines[unit]...)
		}
		diffs[i].Text = text
	}
	return diffs
}

// indexOfUnit is the first position of one code unit at or after start.
func indexOfUnit(text []uint16, unit uint16, start int) int {
	for i := start; i < len(text); i++ {
		if text[i] == unit {
			return i
		}
	}
	return -1
}

// lineKey renders a line as a map key.
func lineKey(line []uint16) string {
	raw := make([]byte, len(line)*2)
	for i, unit := range line {
		raw[2*i] = byte(unit)
		raw[2*i+1] = byte(unit >> 8)
	}
	return string(raw)
}
