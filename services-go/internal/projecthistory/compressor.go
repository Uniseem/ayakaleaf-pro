package projecthistory

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/textdiff"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/textot"
)

// Somebody typing a sentence produces one update per keystroke. Writing each of
// those into the history would make it unreadable and enormous, so a run of
// them that amounts to one edit is merged into one entry before it is sent.
//
// This is the port of UpdateCompressor. What can be merged is narrow on
// purpose: two updates that look mergeable but differ in who made them, whether
// track changes was on, or whether one was an undo, have to stay apart, because
// the history treats each of those differently.

const (
	// maxTimeBetweenUpdates is how far apart two edits can be and still be one
	// entry. Past a minute they are two separate pieces of work.
	maxTimeBetweenUpdates = int64(60 * 1000)
	// maxUpdateSize bounds the text a merged update may carry.
	maxUpdateSize = 2 * 1024 * 1024
	// compressorDiffTimeout matches the limit the Node service gives the diff.
	compressorDiffTimeout = 100 * time.Millisecond
)

// ErrUnexpectedOp is an operation that is none of the four kinds.
var ErrUnexpectedOp = errors.New("unexpected op type")

// CompressRawUpdates turns the queue into the entries that go to the history.
func CompressRawUpdates(raw []*Update) ([]*Update, error) {
	updates, err := ConvertToSingleOpUpdates(raw)
	if err != nil {
		return nil, err
	}
	updates, err = CompressUpdates(updates)
	if err != nil {
		return nil, err
	}
	updates = FilterBlankUpdates(updates)
	return ConcatUpdatesWithSameVersion(updates), nil
}

// ConvertToSingleOpUpdates splits an update carrying several operations into
// one update per operation.
//
// Everything after this works on one operation at a time, which is what makes
// the merging rules expressible. The document length is carried along and
// adjusted as it goes, because each operation sees the document as the ones
// before it left it.
func ConvertToSingleOpUpdates(updates []*Update) ([]*Update, error) {
	var split []*Update

	for _, update := range updates {
		if len(update.Op) == 0 {
			// Not an edit: a file added, renamed, or a resync of the tree.
			split = append(split, update)
			continue
		}

		docLength := update.Meta.DocLength
		if update.Meta.HistoryDocLength != nil {
			docLength = update.Meta.HistoryDocLength
		}
		// document-updater has been known to send -1 for an empty document.
		if docLength != nil && *docLength == -1 {
			zero := 0
			docLength = &zero
		}
		docHash := update.Meta.DocHash

		for _, op := range update.Op {
			piece := update.cloneWithOp(op)
			// Only the last of the split updates keeps the hash, since it is
			// the hash of the document after all of them.
			piece.Meta.DocHash = ""
			if docLength != nil {
				length := *docLength
				piece.Meta.DocLength = &length
				adjusted, err := adjustLengthByOp(length, &op, update.IsTracked())
				if err != nil {
					return nil, err
				}
				docLength = &adjusted
				piece.Meta.HistoryDocLength = nil
			}
			split = append(split, piece)
		}

		if docHash != "" && len(split) > 0 {
			split[len(split)-1].Meta.DocHash = docHash
		}
	}
	return split, nil
}

// adjustLengthByOp reports how long the document is once an operation has been
// applied to it.
func adjustLengthByOp(length int, op *Op, tracked bool) (int, error) {
	switch {
	case op.IsInsert():
		if op.TrackedDeleteRejection {
			// It becomes a retain in the history: the text is already there,
			// and the insert only takes the mark off it.
			return length, nil
		}
		return length + op.Size(), nil

	case op.IsDelete():
		if !tracked {
			return length - op.Size(), nil
		}
		// A tracked delete becomes a retain too, since the history keeps the
		// text. What it does remove is any tracked insert it ran over, because
		// deleting text somebody had just added takes it away outright.
		for _, change := range op.TrackedChanges {
			if change.Type == "insert" {
				length -= change.Length
			}
		}
		return length, nil

	case op.Retain != nil, op.IsComment():
		return length, nil
	}
	return 0, ErrUnexpectedOp
}

// FilterBlankUpdates drops the updates that do nothing.
//
// Diffing an insert against a delete can produce an empty one of each, and the
// history has no way to express an operation that inserts nothing.
func FilterBlankUpdates(updates []*Update) []*Update {
	var kept []*Update
	for _, update := range updates {
		if len(update.Op) == 1 {
			op := update.Op[0]
			if (op.IsInsert() && *op.Insert == "") || (op.IsDelete() && *op.Delete == "") {
				continue
			}
		}
		kept = append(kept, update)
	}
	return kept
}

// ConcatUpdatesWithSameVersion puts back together the operations that came from
// one update and were not merged into anything else.
func ConcatUpdatesWithSameVersion(updates []*Update) []*Update {
	var concatenated []*Update
	for _, update := range updates {
		// Anything without an operation is a change to the file tree, which
		// stands on its own.
		if len(update.Op) == 0 {
			concatenated = append(concatenated, update)
			continue
		}

		current := update.clone()
		if len(concatenated) > 0 {
			last := concatenated[len(concatenated)-1]
			if len(last.Op) > 0 && sameVersion(last, current) &&
				isHistoryOTOp(&last.Op[0]) == isHistoryOTOp(&current.Op[0]) {
				last.Op = append(last.Op, current.Op...)
				// The hash belongs to the last operation of the run.
				last.Meta.DocHash = current.Meta.DocHash
				continue
			}
		}
		concatenated = append(concatenated, current)
	}
	return concatenated
}

// sameVersion reports whether two updates came from the same edit.
func sameVersion(a, b *Update) bool {
	switch {
	case a.V == nil && b.V == nil:
		// Neither says which version it is, which is what a resync sends: two
		// of those are as much the same version as two numbered alike.
	case a.V == nil || b.V == nil:
		return false
	case *a.V != *b.V:
		return false
	}
	return a.Doc == b.Doc && a.Pathname == b.Pathname
}

// CompressUpdates merges each update into the one before it where it can.
func CompressUpdates(updates []*Update) ([]*Update, error) {
	if len(updates) == 0 {
		return nil, nil
	}

	compressed := []*Update{updates[0]}
	for _, update := range updates[1:] {
		// The list can be empty: two updates that cancelled each other out
		// leave nothing behind, and the next update then starts a fresh run.
		if len(compressed) == 0 {
			compressed = append(compressed, update)
			continue
		}

		last := compressed[len(compressed)-1]
		compressed = compressed[:len(compressed)-1]

		merged, err := concatTwoUpdates(last, update)
		if err != nil {
			return nil, err
		}
		compressed = append(compressed, merged...)
	}
	return compressed, nil
}

// concatTwoUpdates returns the two updates merged into one where that has the
// same effect, and the two of them otherwise.
//
// Most of this is the list of reasons not to merge. Each one is a case where
// two updates would apply the same but be recorded differently, and the history
// records what happened rather than only the result.
func concatTwoUpdates(first, second *Update) ([]*Update, error) {
	if len(first.Op) == 0 || len(second.Op) == 0 {
		// One of them is a change to the project rather than to a document.
		return []*Update{first, second}, nil
	}
	if isHistoryOTOp(&first.Op[0]) != isHistoryOTOp(&second.Op[0]) {
		// One speaks the history's operation type and the other the editor's.
		// Nothing should be producing both at once, and there is no sensible
		// way to merge them.
		return []*Update{first, second}, nil
	}
	if first.Doc != second.Doc || first.Pathname != second.Pathname {
		return []*Update{first, second}, nil
	}

	if first.Meta.Resync || second.Meta.Resync {
		// A resync sends the content and then the markers. Merging across that
		// boundary lets the two get out of step with each other.
		return []*Update{first, second}, nil
	}
	if first.Meta.UserID != second.Meta.UserID {
		return []*Update{first, second}, nil
	}

	firstExternal := first.Meta.Type == "external"
	secondExternal := second.Meta.Type == "external"
	if firstExternal != secondExternal ||
		(firstExternal && secondExternal && first.Meta.Source != second.Meta.Source) {
		return []*Update{first, second}, nil
	}

	if second.Meta.Timestamp()-first.Meta.Timestamp() > maxTimeBetweenUpdates {
		return []*Update{first, second}, nil
	}

	if (first.Meta.TC == "") != (second.Meta.TC == "") {
		// One was made with track changes on and the other was not, and the
		// history records those differently.
		return []*Update{first, second}, nil
	}

	firstOp, secondOp := &first.Op[0], &second.Op[0]

	if firstOp.Undo != secondOp.Undo {
		// One is an undo and the other is not. Merging them would have to pick
		// one flag for both, and a tracked delete that is an undo is read by
		// the history as the rejection of a tracked insert.
		return []*Update{first, second}, nil
	}
	if isHistoryOTOp(firstOp) && isHistoryOTOp(secondOp) {
		// Two operations already in the history's form: the history's own
		// composition rules say whether they merge.
		composed, ok := composeHistoryOTOps(firstOp, secondOp)
		if !ok {
			return []*Update{first, second}, nil
		}
		var merged Op
		if err := json.Unmarshal(composed, &merged); err != nil {
			return nil, err
		}
		return []*Update{mergeUpdatesWithOp(first, second, merged)}, nil
	}

	if firstOp.TrackedDeleteRejection || secondOp.TrackedDeleteRejection {
		// Each rejection is its own operation.
		return []*Update{first, second}, nil
	}
	if firstOp.TrackedChanges != nil || secondOp.TrackedChanges != nil {
		// Merging operations that ran over tracked changes would mean working
		// out what the marks did in between, which is not worth the risk.
		return []*Update{first, second}, nil
	}
	if firstOp.HistoryOffset() != secondOp.HistoryOffset() {
		// The two were measured against different states of the tracked
		// changes, so merging them would put one in the wrong place in the
		// history.
		return []*Update{first, second}, nil
	}

	firstSize, secondSize := firstOp.Size(), secondOp.Size()
	firstInsideSecond := secondOp.Pos <= firstOp.Pos && firstOp.Pos <= secondOp.Pos+secondSize
	secondInsideFirst := firstOp.Pos <= secondOp.Pos && secondOp.Pos <= firstOp.Pos+firstSize
	underLimit := firstSize+secondSize < maxUpdateSize
	neitherTracked := first.Meta.TC == "" && second.Meta.TC == ""

	switch {
	// Two inserts, the second inside the first: somebody typing.
	case firstOp.IsInsert() && secondOp.IsInsert() && secondInsideFirst &&
		underLimit && sameComments(firstOp, secondOp):
		merged := *firstOp
		text := injectText(*firstOp.Insert, secondOp.Pos-firstOp.Pos, *secondOp.Insert)
		merged.Insert = &text
		return []*Update{mergeUpdatesWithOp(first, second, merged)}, nil

	// Two deletes, the first inside the second: somebody holding backspace.
	case firstOp.IsDelete() && secondOp.IsDelete() && firstInsideSecond &&
		underLimit && neitherTracked:
		merged := *secondOp
		text := injectText(*secondOp.Delete, firstOp.Pos-secondOp.Pos, *firstOp.Delete)
		merged.Delete = &text
		return []*Update{mergeUpdatesWithOp(first, second, merged)}, nil

	// An insert and then a delete of part of it: somebody typing and correcting.
	case firstOp.IsInsert() && secondOp.IsDelete() && secondInsideFirst && neitherTracked:
		offset := secondOp.Pos - firstOp.Pos
		inserted := sliceUTF16(*firstOp.Insert, offset, offset+secondOp.Size())
		if inserted != *secondOp.Delete {
			// The delete reaches past the insert, so the two cannot be
			// expressed as one.
			return []*Update{first, second}, nil
		}
		remaining := removeText(*firstOp.Insert, offset, secondOp.Size())
		if remaining == "" {
			// The two cancelled out entirely.
			return nil, nil
		}
		merged := *firstOp
		merged.Insert = &remaining
		return []*Update{mergeUpdatesWithOp(first, second, merged)}, nil

	// A delete and then an insert at the same place: a paste over a selection.
	case firstOp.IsDelete() && secondOp.IsInsert() && firstOp.Pos == secondOp.Pos &&
		neitherTracked:
		return diffPastedText(first, second, firstOp, secondOp)
	}

	return []*Update{first, second}, nil
}

// diffPastedText turns a delete followed by an insert at the same place into
// the difference between the two texts.
//
// Recording it as a delete of everything and an insert of everything would make
// a paste that changed one word look like the whole passage being rewritten.
func diffPastedText(first, second *Update, firstOp, secondOp *Op) ([]*Update, error) {
	offset := firstOp.Pos
	historyOffset := firstOp.HPos

	ops := diffAsShareJSOps(*firstOp.Delete, *secondOp.Insert)

	var updates []*Update
	for _, op := range ops {
		// The diff gives positions inside the pasted text; they have to be
		// moved to where that text sits in the document.
		position := op.Pos
		op.Pos = position + offset
		if historyOffset != nil {
			hpos := position + *historyOffset
			op.HPos = &hpos
		}
		if firstOp.Undo && secondOp.Undo {
			op.Undo = true
		}
		if op.IsInsert() && secondOp.CommentIDs != nil {
			op.CommentIDs = secondOp.CommentIDs
		}

		update := mergeUpdatesWithOp(first, second, op)
		update.Meta.DocHash = ""
		updates = append(updates, update)
	}

	if second.Meta.DocHash != "" && len(updates) > 0 {
		updates[len(updates)-1].Meta.DocHash = second.Meta.DocHash
	}

	// Diffing loses the running document length, so it is worked out again.
	docLength := first.Meta.DocLength
	if first.Meta.HistoryDocLength != nil {
		docLength = first.Meta.HistoryDocLength
	}
	for _, update := range updates {
		if docLength == nil {
			break
		}
		length := *docLength
		update.Meta.DocLength = &length
		adjusted, err := adjustLengthByOp(length, &update.Op[0], update.IsTracked())
		if err != nil {
			return nil, err
		}
		docLength = &adjusted
		update.Meta.HistoryDocLength = nil
	}
	return updates, nil
}

// mergeUpdatesWithOp builds the update that stands for two merged ones.
//
// It keeps the length and the time of the first -- the merged edit started
// there -- and the version and hash of the second, which is where it ended up.
func mergeUpdatesWithOp(first, second *Update, op Op) *Update {
	merged := first.cloneWithOp(op)
	if second.V != nil {
		merged.V = second.V
	}
	merged.Meta.DocHash = second.Meta.DocHash
	return merged
}

// sameComments reports whether two inserts belong to the same comment threads.
func sameComments(a, b *Op) bool {
	if a.CommentIDs == nil && b.CommentIDs == nil {
		return true
	}
	if a.CommentIDs == nil || b.CommentIDs == nil {
		return false
	}
	return containsAll(a.CommentIDs, b.CommentIDs) && containsAll(b.CommentIDs, a.CommentIDs)
}

func containsAll(ids, want []string) bool {
	for _, id := range want {
		found := false
		for _, other := range ids {
			if id == other {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// diffAsShareJSOps is the difference between two pieces of text, as inserts and
// deletes.
//
// It is the same algorithm the rest of the stack uses, over the same limit, so
// that a paste is recorded the same way whichever service worked it out.
func diffAsShareJSOps(before, after string) []Op {
	diffs := textdiff.Main(textot.T(before), textot.T(after), true, compressorDiffTimeout)
	diffs = textdiff.CleanupSemantic(diffs)

	var ops []Op
	var position int
	for _, diff := range diffs {
		text := textot.Text(diff.Text).String()
		switch diff.Kind {
		case textdiff.Insert:
			inserted := text
			ops = append(ops, Op{Pos: position, Insert: &inserted})
			position += len(diff.Text)
		case textdiff.Delete:
			deleted := text
			ops = append(ops, Op{Pos: position, Delete: &deleted})
		case textdiff.Equal:
			position += len(diff.Text)
		}
	}
	return ops
}

// injectText puts one string inside another at a position counted in UTF-16
// code units.
func injectText(text string, at int, inserted string) string {
	return sliceUTF16(text, 0, at) + inserted + sliceUTF16(text, at, utf16Len(text))
}

// removeText takes a stretch out of a string.
func removeText(text string, at, length int) string {
	return sliceUTF16(text, 0, at) + sliceUTF16(text, at+length, utf16Len(text))
}

// sliceUTF16 takes a range of a string measured in UTF-16 code units.
func sliceUTF16(text string, from, to int) string {
	units := textot.T(text)
	if from < 0 {
		from = 0
	}
	if to > len(units) {
		to = len(units)
	}
	if from > to {
		return ""
	}
	return units.Slice(from, to).String()
}
