package textdiff

import "regexp"

// CleanupMerge joins adjacent edits of the same kind, pulls the text they share
// out into the equalities around them, and drops what is left empty.
func CleanupMerge(diffs []Diff) []Diff {
	// A trailing equality, so the last run of edits is dealt with by the same
	// branch as all the others.
	diffs = append(diffs, Diff{Kind: Equal})

	pointer := 0
	countDelete, countInsert := 0, 0
	var textDelete, textInsert []uint16

	for pointer < len(diffs) {
		switch diffs[pointer].Kind {
		case Insert:
			countInsert++
			textInsert = concat(textInsert, diffs[pointer].Text)
			pointer++

		case Delete:
			countDelete++
			textDelete = concat(textDelete, diffs[pointer].Text)
			pointer++

		case Equal:
			switch {
			case countDelete+countInsert > 1:
				if countDelete != 0 && countInsert != 0 {
					// Text at the front of both the deletion and the insertion
					// was never edited, so it belongs to the equality before
					// them rather than to either edit.
					if common := commonPrefix(textInsert, textDelete); common != 0 {
						at := pointer - countDelete - countInsert
						if at > 0 && diffs[at-1].Kind == Equal {
							diffs[at-1].Text = concat(diffs[at-1].Text, textInsert[:common])
						} else {
							diffs = splice(diffs, 0, 0,
								Diff{Kind: Equal, Text: textInsert[:common]})
							pointer++
						}
						textInsert = textInsert[common:]
						textDelete = textDelete[common:]
					}
					// The same at the back, which belongs to the equality that
					// follows.
					if common := commonSuffix(textInsert, textDelete); common != 0 {
						diffs[pointer].Text = concat(textInsert[len(textInsert)-common:],
							diffs[pointer].Text)
						textInsert = textInsert[:len(textInsert)-common]
						textDelete = textDelete[:len(textDelete)-common]
					}
				}

				pointer -= countDelete + countInsert
				diffs = splice(diffs, pointer, countDelete+countInsert)
				// Only what is left of them goes back: an edit emptied by the
				// factoring above is not an edit.
				if len(textDelete) > 0 {
					diffs = splice(diffs, pointer, 0, Diff{Kind: Delete, Text: textDelete})
					pointer++
				}
				if len(textInsert) > 0 {
					diffs = splice(diffs, pointer, 0, Diff{Kind: Insert, Text: textInsert})
					pointer++
				}
				pointer++

			case pointer != 0 && diffs[pointer-1].Kind == Equal:
				diffs[pointer-1].Text = concat(diffs[pointer-1].Text, diffs[pointer].Text)
				diffs = splice(diffs, pointer, 1)

			default:
				pointer++
			}

			countInsert, countDelete = 0, 0
			textDelete, textInsert = nil, nil
		}
	}

	if len(diffs) > 0 && len(diffs[len(diffs)-1].Text) == 0 {
		diffs = diffs[:len(diffs)-1]
	}

	// An edit that ends with the equality before it, or starts with the one
	// after it, describes the same change one position over, and moving it
	// there does away with an equality:
	//
	//	A<ins>BA</ins>C  ->  <ins>AB</ins>AC
	changes := false
	pointer = 1
	for pointer < len(diffs)-1 {
		if diffs[pointer-1].Kind == Equal && diffs[pointer+1].Kind == Equal {
			edit := diffs[pointer].Text
			previous := diffs[pointer-1].Text
			next := diffs[pointer+1].Text

			switch {
			case len(edit) >= len(previous) &&
				equalText(edit[len(edit)-len(previous):], previous):
				diffs[pointer].Text = concat(previous, edit[:len(edit)-len(previous)])
				diffs[pointer+1].Text = concat(previous, next)
				diffs = splice(diffs, pointer-1, 1)
				changes = true

			case len(edit) >= len(next) && equalText(edit[:len(next)], next):
				diffs[pointer-1].Text = concat(previous, next)
				diffs[pointer].Text = concat(edit[len(next):], next)
				diffs = splice(diffs, pointer+1, 1)
				changes = true
			}
		}
		pointer++
	}
	if changes {
		// The shifts can have left two edits of the same kind next to each
		// other, so the whole thing runs again.
		return CleanupMerge(diffs)
	}
	return diffs
}

// CleanupSemantic removes the equalities that are too small to be worth keeping
// and aligns what is left to sensible boundaries.
//
// Without it the diff is minimal but unreadable: it matches stray characters
// across unrelated words, which makes for tiny operations that look nothing
// like the edit somebody made.
func CleanupSemantic(diffs []Diff) []Diff {
	changes := false
	// equalities holds the indices of the equalities seen so far, so one can be
	// removed once the edits either side of it turn out to be larger.
	var equalities []int
	var lastEquality []uint16
	haveLastEquality := false

	// How much changed before and after the equality under consideration.
	var insertions1, deletions1, insertions2, deletions2 int

	pointer := 0
	for pointer < len(diffs) {
		if diffs[pointer].Kind == Equal {
			equalities = append(equalities, pointer)
			insertions1, deletions1 = insertions2, deletions2
			insertions2, deletions2 = 0, 0
			lastEquality = diffs[pointer].Text
			haveLastEquality = true
			pointer++
			continue
		}

		if diffs[pointer].Kind == Insert {
			insertions2 += len(diffs[pointer].Text)
		} else {
			deletions2 += len(diffs[pointer].Text)
		}

		// An equality smaller than the edits on both sides of it is not really
		// an equality: it is a coincidence in the middle of a rewrite, and
		// keeping it splits one edit into three.
		if haveLastEquality && len(lastEquality) > 0 &&
			len(lastEquality) <= maxInt(insertions1, deletions1) &&
			len(lastEquality) <= maxInt(insertions2, deletions2) {

			at := equalities[len(equalities)-1]
			diffs = splice(diffs, at, 0, Diff{Kind: Delete, Text: lastEquality})
			diffs[at+1].Kind = Insert

			// Drop the equality just replaced, and the one before it, which
			// has to be looked at again now that this one is gone.
			equalities = equalities[:len(equalities)-1]
			if len(equalities) > 0 {
				equalities = equalities[:len(equalities)-1]
			}
			pointer = -1
			if len(equalities) > 0 {
				pointer = equalities[len(equalities)-1]
			}

			insertions1, deletions1, insertions2, deletions2 = 0, 0, 0, 0
			lastEquality = nil
			haveLastEquality = false
			changes = true
		}
		pointer++
	}

	if changes {
		diffs = CleanupMerge(diffs)
	}
	diffs = cleanupSemanticLossless(diffs)

	// A deletion and an insertion that share a run of text describe an edit
	// around that run rather than a replacement of it:
	//
	//	<del>abcxxx</del><ins>xxxdef</ins>  ->  <del>abc</del>xxx<ins>def</ins>
	//
	// The shared run is only pulled out when it is at least half of one of the
	// two, or a genuine replacement would be turned into three edits.
	pointer = 1
	for pointer < len(diffs) {
		if diffs[pointer-1].Kind == Delete && diffs[pointer].Kind == Insert {
			deletion := diffs[pointer-1].Text
			insertion := diffs[pointer].Text
			overlap1 := commonOverlap(deletion, insertion)
			overlap2 := commonOverlap(insertion, deletion)

			if overlap1 >= overlap2 {
				if overlap1*2 >= len(deletion) || overlap1*2 >= len(insertion) {
					diffs = splice(diffs, pointer, 0,
						Diff{Kind: Equal, Text: insertion[:overlap1]})
					diffs[pointer-1].Text = deletion[:len(deletion)-overlap1]
					diffs[pointer+1].Text = insertion[overlap1:]
					pointer++
				}
			} else if overlap2*2 >= len(deletion) || overlap2*2 >= len(insertion) {
				// The same the other way round, which also swaps the two edits.
				diffs = splice(diffs, pointer, 0,
					Diff{Kind: Equal, Text: deletion[:overlap2]})
				diffs[pointer-1].Kind = Insert
				diffs[pointer-1].Text = insertion[:len(insertion)-overlap2]
				diffs[pointer+1].Kind = Delete
				diffs[pointer+1].Text = deletion[overlap2:]
				pointer++
			}
			pointer++
		}
		pointer++
	}
	return diffs
}

// cleanupSemanticLossless slides each edit that sits between two equalities to
// whichever nearby position reads best -- a word boundary, the end of a
// sentence, a line break -- without changing what the diff produces.
func cleanupSemanticLossless(diffs []Diff) []Diff {
	pointer := 1
	// The first and last elements have nothing on one side to slide into, so
	// they are left alone.
	for pointer < len(diffs)-1 {
		if diffs[pointer-1].Kind != Equal || diffs[pointer+1].Kind != Equal {
			pointer++
			continue
		}

		equality1 := diffs[pointer-1].Text
		edit := diffs[pointer].Text
		equality2 := diffs[pointer+1].Text
		buffer := concat(concat(equality1, edit), equality2)

		// How far the edit can slide: left by whatever it shares with the text
		// before it, right by whatever it shares with the text after.
		offsetLeft := commonSuffix(equality1, edit)
		offsetRight := commonPrefix(edit, equality2)

		originalEditStart := len(equality1)
		editStart := originalEditStart - offsetLeft
		maxEditStart := originalEditStart + offsetRight
		editEnd := editStart + len(edit)

		bestEditStart, bestEditEnd := editStart, editEnd
		bestScore := boundaryScore(buffer, editStart) + boundaryScore(buffer, editEnd)

		for editStart < maxEditStart {
			editStart++
			editEnd++
			score := boundaryScore(buffer, editStart) + boundaryScore(buffer, editEnd)
			// Not a strict improvement: an equal score later is preferred, so
			// an edit ends up with trailing rather than leading whitespace.
			if score >= bestScore {
				bestScore = score
				bestEditStart, bestEditEnd = editStart, editEnd
			}
		}

		if bestEditStart != originalEditStart {
			if bestEditStart > 0 {
				diffs[pointer-1].Text = buffer[:bestEditStart]
			} else {
				diffs = splice(diffs, pointer-1, 1)
				pointer--
			}
			diffs[pointer].Text = buffer[bestEditStart:bestEditEnd]
			if bestEditEnd < len(buffer) {
				diffs[pointer+1].Text = buffer[bestEditEnd:]
			} else {
				diffs = splice(diffs, pointer+1, 1)
				pointer--
			}
		}
		pointer++
	}
	return diffs
}

// Boundary patterns, as the reference implementation defines them.
var (
	nonAlphaNumericPattern = regexp.MustCompile(`[^a-zA-Z0-9]`)
	whitespacePattern      = regexp.MustCompile(`\s`)
	linebreakPattern       = regexp.MustCompile(`[\r\n]`)
	blanklineEndPattern    = regexp.MustCompile(`\n\r?\n$`)
	blanklineStartPattern  = regexp.MustCompile(`^\r?\n\r?\n`)
)

// boundaryScore rates a position in the text, from 6 for the very edge down to
// 0 for the middle of a word.
func boundaryScore(buffer []uint16, index int) int {
	if index == 0 || index == len(buffer) {
		return 6
	}

	char1 := unitAsString(buffer[index-1])
	char2 := unitAsString(buffer[index])

	nonAlphaNumeric1 := nonAlphaNumericPattern.MatchString(char1)
	nonAlphaNumeric2 := nonAlphaNumericPattern.MatchString(char2)
	whitespace1 := nonAlphaNumeric1 && whitespacePattern.MatchString(char1)
	whitespace2 := nonAlphaNumeric2 && whitespacePattern.MatchString(char2)
	lineBreak1 := whitespace1 && linebreakPattern.MatchString(char1)
	lineBreak2 := whitespace2 && linebreakPattern.MatchString(char2)

	// Both look at the surrounding text rather than only at their own side: a
	// blank line before the boundary can begin inside the previous element.
	blankLine1 := lineBreak1 &&
		blanklineEndPattern.MatchString(textAsString(buffer[maxInt(0, index-3):index]))
	blankLine2 := lineBreak2 &&
		blanklineStartPattern.MatchString(textAsString(buffer[index:minInt(len(buffer), index+4)]))

	switch {
	case blankLine1 || blankLine2:
		return 5
	case lineBreak1 || lineBreak2:
		return 4
	case nonAlphaNumeric1 && !whitespace1 && whitespace2:
		// The end of a sentence.
		return 3
	case whitespace1 || whitespace2:
		return 2
	case nonAlphaNumeric1 || nonAlphaNumeric2:
		return 1
	}
	return 0
}

// unitAsString renders one code unit for the patterns above.
//
// A lone half of an astral character has no character of its own, and is given
// a placeholder that scores as it does in JavaScript: not a letter, not a
// digit, not a space.
func unitAsString(unit uint16) string {
	if unit >= 0xD800 && unit <= 0xDFFF {
		return "�"
	}
	return string(rune(unit))
}

// textAsString renders a run of code units for the patterns above. Only the
// whitespace in it is ever matched, so the halves of an astral character are
// replaced rather than decoded.
func textAsString(text []uint16) string {
	var out []rune
	for _, unit := range text {
		if unit >= 0xD800 && unit <= 0xDFFF {
			out = append(out, '�')
			continue
		}
		out = append(out, rune(unit))
	}
	return string(out)
}
