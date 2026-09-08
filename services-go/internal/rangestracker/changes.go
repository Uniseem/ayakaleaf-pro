package rangestracker

import (
	"fmt"
	"sort"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/textot"
)

// applyInsertToChanges moves the tracked changes to account for an insert, and
// records the insert itself when track changes is on.
//
// This is the most intricate part of the whole service. The cases it has to
// separate are: an insert that extends somebody's existing insert, an insert
// that splits another user's insert in two, an insert that undoes a tracked
// delete, and an insert that is merely before or after everything.
func (t *Tracker) applyInsertToChanges(op textot.Component, metadata Metadata) {
	opStart := op.Pos
	opLength := op.Len()
	opEnd := opStart + opLength
	undoing := op.Undo

	alreadyMerged := false
	var previousChange *Change
	var movedChanges, removeChanges, trackedDeletesAtOpPosition []*Change
	var newChanges []*Change

	for i := 0; i < len(t.Changes); i++ {
		change := t.Changes[i]
		changeStart := change.Op.Pos

		switch change.Op.Kind {
		case textot.Delete:
			if opStart < changeStart {
				// Shift deletes after this insert along by its length.
				change.Op.Pos += opLength
				movedChanges = append(movedChanges, change)
			} else if opStart == changeStart {
				if !alreadyMerged && undoing && change.Op.Len() >= op.Len() &&
					change.Op.Text.Slice(0, op.Len()).Equal(op.Text) {
					// Undoing at the start of a tracked delete: rather than
					// inserting the text again, take it back out of the delete
					// so the deletion is simply undone.
					change.Op.Text = change.Op.Text.From(op.Len())
					change.Op.Pos += op.Len()
					if change.Op.Len() == 0 {
						removeChanges = append(removeChanges, change)
					} else {
						movedChanges = append(movedChanges, change)
					}
					alreadyMerged = true

					// Deletes at this position were shifted past the insert a
					// moment ago on the assumption it was an insert. It turned
					// out to be a rejection, so they belong before it again.
					for _, trackedDelete := range trackedDeletesAtOpPosition {
						trackedDelete.Op.Pos -= opLength
					}
				} else {
					change.Op.Pos += opLength
					movedChanges = append(movedChanges, change)

					// Remembered in case a later delete at this position turns
					// out to be the one being rejected.
					if !alreadyMerged {
						trackedDeletesAtOpPosition = append(trackedDeletesAtOpPosition, change)
					}
				}
			}

		case textot.Insert:
			changeEnd := changeStart + change.Op.Len()
			isChangeOverlapping := opStart >= changeStart && opStart <= changeEnd
			// Only one user's inserts merge together; two users' stay apart so
			// the document shows who wrote what.
			isSameUser := metadata.UserID() == change.Metadata.UserID()

			// When undoing, this insert will cancel the delete just after it.
			// In that case it must not also extend the insert just before, or
			// the undo would leave text behind:
			//
			//	                 foo|  <- inserting 'b' here
			//	 inserted 'foo' --^ ^-- deleted 'bar'
			//
			// should end up as 'foo', not 'foob'.
			var nextChange *Change
			if i+1 < len(t.Changes) {
				nextChange = t.Changes[i+1]
			}
			isOpAdjacentToNextDelete := nextChange != nil &&
				nextChange.Op.Kind == textot.Delete &&
				op.Pos == changeEnd && nextChange.Op.Pos == op.Pos
			willOpCancelNextDelete := undoing && isOpAdjacentToNextDelete &&
				nextChange.Op.Text.Slice(0, op.Len()).Equal(op.Text)

			// A delete sitting at the end of this insert partitions it from
			// the existing one, which would otherwise merge:
			//
			//	Originally: |-- existing insert --|
			//	            | <- existing delete at the same offset
			//	After:                         |-- existing insert --|
			//	            |-- this insert --|| <- existing delete
			isInsertBlockedByDelete := previousChange != nil &&
				previousChange.Op.Kind == textot.Delete &&
				previousChange.Op.Pos == opEnd

			switch {
			case t.TrackChanges && isChangeOverlapping && !isInsertBlockedByDelete &&
				!alreadyMerged && !willOpCancelNextDelete && isSameUser:
				// Extending the same user's insert: the two become one.
				offset := opStart - changeStart
				change.Op.Text = textot.Concat(
					change.Op.Text.Slice(0, offset), op.Text, change.Op.Text.From(offset))
				change.Metadata["ts"] = metadata["ts"]
				alreadyMerged = true
				movedChanges = append(movedChanges, change)

			case opStart <= changeStart:
				// Entirely before the existing insert, so it shifts along.
				change.Op.Pos += opLength
				movedChanges = append(movedChanges, change)

			case (!isSameUser || !t.TrackChanges) && changeStart < opStart && opStart < changeEnd:
				// Inserting inside another user's insert splits it in two, so
				// each half stays attributed to whoever wrote it.
				offset := opStart - changeStart
				before := change.Op.Text.Slice(0, offset)
				after := change.Op.Text.From(offset)

				change.Op.Text = cloneText(before)
				movedChanges = append(movedChanges, change)

				newChanges = append(newChanges, &Change{
					Op: textot.Component{
						Kind: textot.Insert, Text: cloneText(after),
						Pos: changeStart + offset + opLength,
					},
					Metadata: change.Metadata.Clone(),
				})
			}
		}

		previousChange = change
	}

	if t.TrackChanges && !alreadyMerged {
		t.addOp(op, metadata)
	}
	for _, pending := range newChanges {
		t.addOp(pending.Op, pending.Metadata)
	}
	for _, change := range removeChanges {
		t.removeChange(change)
	}
	for _, change := range movedChanges {
		t.changeDirty.Moved[change.ID] = true
	}
}

// opModification is a change to the delete being applied, produced when it
// meets an existing marker.
type opModification struct {
	// insert and delete are mutually exclusive, matching the {i, p} and {d, p}
	// shapes the Node code builds.
	insert   textot.Text
	delete   textot.Text
	pos      int
	isInsert bool
}

// applyDeleteToChanges moves the tracked changes to account for a delete.
//
// A delete does not simply remove text: where it overlaps a tracked insert the
// two cancel out, and where it overlaps a tracked delete the two merge. The
// delete itself is rewritten as it goes, which is why the modifications are
// collected and applied afterwards rather than as they are found -- changing it
// mid-loop would move the offsets everything else is measured against.
func (t *Tracker) applyDeleteToChanges(op textot.Component, metadata Metadata) error {
	opStart := op.Pos
	opLength := op.Len()
	opEnd := opStart + opLength

	var removeChanges, movedChanges []*Change
	var modifications []opModification

	for _, change := range t.Changes {
		switch change.Op.Kind {
		case textot.Insert:
			changeStart := change.Op.Pos
			changeEnd := changeStart + change.Op.Len()

			switch {
			case opEnd <= changeStart:
				change.Op.Pos -= opLength
				movedChanges = append(movedChanges, change)
			case opStart >= changeEnd:
				// After the insert; nothing to do.
			default:
				// Where a delete overlaps an insert, the two cancel out: the
				// deleted part of the insert goes, and so does the part of the
				// delete that covered it.
				var deleteRemainingBefore, insertRemainingBefore textot.Text
				if opStart >= changeStart {
					insertRemainingBefore = change.Op.Text.Slice(0, opStart-changeStart)
				} else {
					deleteRemainingBefore = op.Text.Slice(0, changeStart-opStart)
				}

				var deleteRemainingAfter, insertRemainingAfter textot.Text
				if opEnd <= changeEnd {
					insertRemainingAfter = change.Op.Text.From(opEnd - changeStart)
				} else {
					deleteRemainingAfter = op.Text.From(changeEnd - opStart)
				}

				insertRemaining := textot.Concat(insertRemainingBefore, insertRemainingAfter)
				if insertRemaining.Len() > 0 {
					change.Op.Text = insertRemaining
					change.Op.Pos = min(changeStart, opStart)
					movedChanges = append(movedChanges, change)
				} else {
					removeChanges = append(removeChanges, change)
				}

				// What is left of the delete is what fell outside the insert,
				// so the part in between is cut out of it.
				removedLength := op.Len() - deleteRemainingBefore.Len() - deleteRemainingAfter.Len()
				removedStart := deleteRemainingBefore.Len()
				cut := op.Text.Slice(removedStart, removedStart+removedLength)
				if cut.Len() > 0 {
					modifications = append(modifications, opModification{
						delete: cut, pos: removedStart,
					})
				}
			}

		case textot.Delete:
			changeStart := change.Op.Pos
			switch {
			case opEnd < changeStart || (!t.TrackChanges && opEnd == changeStart):
				// Shift deletes after this one back. With track changes on,
				// touching deletes merge below instead, so this has to be
				// strictly before.
				change.Op.Pos -= opLength
				movedChanges = append(movedChanges, change)
			case opStart <= changeStart && changeStart <= opEnd:
				if t.TrackChanges {
					// Overlapping an existing delete: take its text into this
					// one and drop it, rather than trying to edit it in place
					// while possibly overlapping several more.
					modifications = append(modifications, opModification{
						insert: change.Op.Text, pos: changeStart - opStart, isInsert: true,
					})
					removeChanges = append(removeChanges, change)
				} else {
					change.Op.Pos = opStart
					movedChanges = append(movedChanges, change)
				}
			}
		}
	}

	// Copied rather than modified, because the caller still has to apply the
	// original delete to the comments.
	rewrittenText, err := applyOpModifications(op.Text, modifications)
	if err != nil {
		return err
	}
	rewritten := textot.Component{Kind: textot.Delete, Text: rewrittenText, Pos: op.Pos}

	for _, change := range removeChanges {
		// Replacing a delete in place rather than removing one and adding
		// another, which would make the marker flicker in the editor.
		if rewritten.Len() > 0 && change.Op.Kind == textot.Delete &&
			rewritten.Pos <= change.Op.Pos &&
			change.Op.Pos <= rewritten.Pos+rewritten.Len() {
			change.Op.Pos = rewritten.Pos
			change.Op.Text = rewritten.Text
			change.Metadata = metadata
			movedChanges = append(movedChanges, change)
			rewritten.Text = textot.Text{} // it has been absorbed, so do not add it
		} else {
			t.removeChange(change)
		}
	}

	if t.TrackChanges && rewritten.Len() > 0 {
		t.addOp(rewritten, metadata)
	} else {
		// Deleting an insert that sat between two of the same user's inserts
		// leaves those two adjacent, and they belong together:
		//
		//	|-- user 1 --||-- user 2 --||-- user 1 --|
		//
		// becomes one insert once user 2's is gone.
		merged := t.scanAndMergeAdjacentUpdates()
		movedChanges = append(movedChanges, merged.moved...)
		for _, change := range merged.removed {
			t.removeChange(change)
			movedChanges = withoutChange(movedChanges, change)
		}
	}

	for _, change := range movedChanges {
		t.changeDirty.Moved[change.ID] = true
	}
	return nil
}

func withoutChange(changes []*Change, unwanted *Change) []*Change {
	remaining := changes[:0]
	for _, change := range changes {
		if change != unwanted {
			remaining = append(remaining, change)
		}
	}
	return remaining
}

// applyOpModifications rewrites a delete's text.
//
// The modifications are applied from the back, so that each one's offset still
// refers to the text as it was when the offset was worked out. At the same
// offset a delete is applied before an insert, because inserting first would
// move the text the delete is meant to remove.
func applyOpModifications(content textot.Text, modifications []opModification) (textot.Text, error) {
	sorted := make([]opModification, len(modifications))
	copy(sorted, modifications)
	sort.SliceStable(sorted, func(a, b int) bool {
		if sorted[a].pos != sorted[b].pos {
			return sorted[a].pos > sorted[b].pos
		}
		// A delete sorts before an insert at the same offset.
		return !sorted[a].isInsert && sorted[b].isInsert
	})

	for _, modification := range sorted {
		if modification.isInsert {
			content = content.Inject(modification.pos, modification.insert)
			continue
		}
		existing := content.Slice(modification.pos, modification.pos+modification.delete.Len())
		if !existing.Equal(modification.delete) {
			return nil, fmt.Errorf("%w: %q is not %q", ErrDeletionMismatch,
				existing.String(), modification.delete.String())
		}
		content = textot.Concat(
			content.Slice(0, modification.pos),
			content.From(modification.pos+modification.delete.Len()))
	}
	return content, nil
}

// addOp records a new tracked change.
func (t *Tracker) addOp(op textot.Component, metadata Metadata) {
	change := &Change{
		ID: t.newID(),
		// Copied, because the op will be modified in place by later edits.
		Op: textot.Component{
			Kind: op.Kind, Text: cloneText(op.Text), Pos: op.Pos,
			Thread: op.Thread, Undo: op.Undo,
		},
		Metadata: metadata.Clone(),
	}
	t.Changes = append(t.Changes, change)

	// Kept in order of position, with deletes before inserts at the same one.
	// The sort has to be stable: markers at the same position and of the same
	// kind keep the order they were added in, and the editor renders them that
	// way.
	sort.SliceStable(t.Changes, func(a, b int) bool {
		first, second := t.Changes[a], t.Changes[b]
		if first.Op.Pos != second.Op.Pos {
			return first.Op.Pos < second.Op.Pos
		}
		if first.Op.Kind == textot.Delete && second.Op.Kind == textot.Insert {
			return true
		}
		return false
	})

	t.changeDirty.Added[change.ID] = true
}

func (t *Tracker) removeChange(unwanted *Change) {
	remaining := make([]*Change, 0, len(t.Changes))
	for _, change := range t.Changes {
		if change != unwanted {
			remaining = append(remaining, change)
		}
	}
	t.Changes = remaining
	t.changeDirty.Removed[unwanted.ID] = true
}

type mergeResult struct {
	moved   []*Change
	removed []*Change
}

// scanAndMergeAdjacentUpdates joins markers that have become neighbours.
//
// It is only needed after deleting a marker from between two others: inserts
// from the same user are merged as they are made, so no other route produces
// two adjacent ones.
func (t *Tracker) scanAndMergeAdjacentUpdates() mergeResult {
	var result mergeResult
	var previousChange *Change

	for _, change := range t.Changes {
		switch {
		case previousChange != nil && previousChange.Op.Kind == textot.Insert &&
			change.Op.Kind == textot.Insert:
			// Note what does not happen here: when the two do not merge,
			// previousChange is left pointing at the earlier one rather than
			// advancing. That is the Node control flow -- it only advances in
			// the final branch -- and it matters. Advancing here lets a pair
			// that the earlier change kept apart merge on the next step, which
			// silently joins two users' inserts into one.
			previousEnd := previousChange.Op.Pos + previousChange.Op.Len()
			if previousEnd == change.Op.Pos &&
				previousChange.Metadata.UserID() == change.Metadata.UserID() {
				result.removed = append(result.removed, change)
				previousChange.Op.Text = textot.Concat(previousChange.Op.Text, change.Op.Text)
				result.moved = append(result.moved, previousChange)
			}

		case previousChange != nil && previousChange.Op.Kind == textot.Delete &&
			change.Op.Kind == textot.Delete && previousChange.Op.Pos == change.Op.Pos:
			previousChange.Op.Text = textot.Concat(previousChange.Op.Text, change.Op.Text)
			result.removed = append(result.removed, change)
			result.moved = append(result.moved, previousChange)

		default:
			previousChange = change
		}
	}
	return result
}
