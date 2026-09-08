package docupdater

import (
	"encoding/json"
	"fmt"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/rangestracker"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/textot"
)

// With history ranges support on, the history is told about an edit in the
// positions of a text that still contains the tracked deletions. The editor
// works in a text with them taken out, so every op has to be restated: a
// second position, sometimes a second length, and a note of which tracked
// changes the edit ran over.
//
// This is the port of RangesManager's history op conversion. It runs against
// the markers as they were before the op was applied, which is why the caller
// converts each op before handing it to the tracker.

// historyOp is an operation as the history receives it: the editor op, plus
// the fields that place it in the longer text.
type historyOp struct {
	// Op is the operation as the editor made it.
	Op textot.Component
	// HPos is its position in the history text, when that differs.
	HPos *int
	// HLen is the length a comment covers there, when that differs.
	HLen *int
	// CommentIDs are the comment threads an insert landed inside.
	CommentIDs []string
	// TrackedDeleteRejection marks an insert that put back exactly what a
	// tracked deletion had taken out.
	TrackedDeleteRejection bool
	// TrackedChanges are the tracked changes a delete ran over, so the history
	// can delete around them rather than through them.
	TrackedChanges []historyTrackedChange
}

// historyTrackedChange is one tracked change caught inside a delete.
type historyTrackedChange struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
}

// MarshalJSON renders the op with the extra fields the history reads.
func (h historyOp) MarshalJSON() ([]byte, error) {
	encoded, err := json.Marshal(h.Op)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}

	if h.HPos != nil {
		fields["hpos"], _ = json.Marshal(*h.HPos)
	}
	if h.HLen != nil {
		fields["hlen"], _ = json.Marshal(*h.HLen)
	}
	if len(h.CommentIDs) > 0 {
		fields["commentIds"], _ = json.Marshal(h.CommentIDs)
	}
	if h.TrackedDeleteRejection {
		fields["trackedDeleteRejection"] = json.RawMessage("true")
	}
	if len(h.TrackedChanges) > 0 {
		fields["trackedChanges"], _ = json.Marshal(h.TrackedChanges)
	}
	return json.Marshal(fields)
}

// toHistoryOp restates one op in the positions the history stores.
func toHistoryOp(op textot.Component, comments []*rangestracker.Comment,
	changes []*rangestracker.Change) (historyOp, error) {
	switch op.Kind {
	case textot.Insert:
		return historyOpForInsert(op, comments, changes), nil
	case textot.Delete:
		return historyOpForDelete(op, changes), nil
	case textot.Comment:
		return historyOpForComment(op, changes), nil
	}
	return historyOp{}, fmt.Errorf("unrecognized op kind %d", op.Kind)
}

// historyOpForInsert places an insert in the history text.
//
// A tracked deletion strictly before it moves it along. One at the same
// position does not, because an insert there goes in front of it -- unless the
// insert is an undo that puts back exactly what that deletion removed, which
// makes it a rejection of that deletion and the history has to be told so.
func historyOpForInsert(op textot.Component, comments []*rangestracker.Comment,
	changes []*rangestracker.Change) historyOp {
	out := historyOp{Op: op}
	hpos := op.Pos

	// An insert strictly inside a comment belongs to it. One at either edge
	// does not: the editor does not extend a comment by typing against it.
	seen := map[string]bool{}
	for _, comment := range comments {
		start := comment.Op.Pos
		end := start + comment.Op.Len()
		if start < op.Pos && op.Pos < end && !seen[comment.Op.Thread] {
			seen[comment.Op.Thread] = true
			out.CommentIDs = append(out.CommentIDs, comment.Op.Thread)
		}
	}

	// Several tracked deletions can sit at the same position. The ones before
	// the one actually being rejected still move the insert along.
	var rejectionOffset int
loop:
	for _, change := range changes {
		if change.Op.Kind != textot.Delete {
			continue
		}
		switch {
		case change.Op.Pos < op.Pos:
			hpos += change.Op.Len()
		case change.Op.Pos == op.Pos:
			if op.Undo && startsWith(change.Op.Text, op.Text) {
				out.TrackedDeleteRejection = true
				hpos += rejectionOffset
				// Everything after this is past the insert.
				break loop
			}
			// This one does not match, but it still sits in front of whichever
			// one turns out to.
			rejectionOffset += change.Op.Len()
		default:
			// The changes are ordered, so the rest are after the insert.
			break loop
		}
	}

	if hpos != op.Pos {
		out.HPos = &hpos
	}
	return out
}

// historyOpForDelete places a delete in the history text.
//
// A tracked deletion before or at the position moves it along. A tracked
// change inside it is recorded rather than deleted: the history keeps tracked
// deletions, and a tracked insert inside the deleted text is a separate thing
// to account for.
func historyOpForDelete(op textot.Component, changes []*rangestracker.Change) historyOp {
	out := historyOp{Op: op}
	hpos := op.Pos
	opEnd := op.Pos + op.Len()

	for _, change := range changes {
		changePos := change.Op.Pos
		switch {
		case changePos <= op.Pos:
			switch change.Op.Kind {
			case textot.Delete:
				hpos += change.Op.Len()
			case textot.Insert:
				// The tail of a tracked insert can reach into the delete.
				changeEnd := changePos + change.Op.Len()
				end := minInt(changeEnd, opEnd)
				if end > op.Pos {
					out.TrackedChanges = append(out.TrackedChanges, historyTrackedChange{
						Type: "insert", Offset: 0, Length: end - op.Pos,
					})
				}
			}

		case changePos < opEnd:
			switch change.Op.Kind {
			case textot.Delete:
				out.TrackedChanges = append(out.TrackedChanges, historyTrackedChange{
					Type: "delete", Offset: changePos - op.Pos, Length: change.Op.Len(),
				})
			case textot.Insert:
				out.TrackedChanges = append(out.TrackedChanges, historyTrackedChange{
					Type:   "insert",
					Offset: changePos - op.Pos,
					Length: minInt(change.Op.Len(), opEnd-changePos),
				})
			}

		default:
			// The changes are ordered, so the rest are past the delete.
			if out.HPos == nil && hpos != op.Pos {
				out.HPos = &hpos
			}
			return out
		}
	}

	if hpos != op.Pos {
		out.HPos = &hpos
	}
	return out
}

// historyOpForComment places a comment in the history text.
//
// A tracked deletion inside the commented text makes the comment longer there
// than the editor shows it, because the history still holds that text.
func historyOpForComment(op textot.Component, changes []*rangestracker.Change) historyOp {
	out := historyOp{Op: op}
	hpos := op.Pos
	hlen := op.Len()

	for _, change := range changes {
		if change.Op.Kind != textot.Delete {
			continue
		}
		switch {
		case change.Op.Pos <= op.Pos:
			hpos += change.Op.Len()
		case change.Op.Pos < op.Pos+op.Len():
			hlen += change.Op.Len()
		default:
			// The changes are ordered, so the rest are past the comment.
			if hpos != op.Pos {
				out.HPos = &hpos
			}
			if hlen != op.Len() {
				out.HLen = &hlen
			}
			return out
		}
	}

	if hpos != op.Pos {
		out.HPos = &hpos
	}
	if hlen != op.Len() {
		out.HLen = &hlen
	}
	return out
}

// croppedCommentOps are the operations that trim a comment the history would
// otherwise keep at its old length.
//
// The editor treats a tracked deletion as a deletion and shortens a comment
// that overlapped it. The history keeps the deleted text, so it would keep the
// comment over it too. These ops are what bring the two back into line.
func croppedCommentOps(op textot.Component, comments []*rangestracker.Comment) []textot.Component {
	deleteStart := op.Pos
	deleteEnd := deleteStart + op.Len()

	var cropped []textot.Component
	for _, comment := range comments {
		commentStart := comment.Op.Pos
		commentEnd := commentStart + comment.Op.Len()

		switch {
		case deleteStart <= commentStart && deleteEnd > commentStart:
			// The delete covers the start of the comment, or all of it.
			overlap := minInt(deleteEnd, commentEnd) - commentStart
			cropped = append(cropped, textot.Component{
				Kind: textot.Comment, Pos: deleteStart,
				Text: comment.Op.Text.From(overlap), Thread: comment.Op.Thread,
			})

		case deleteStart > commentStart && deleteStart < commentEnd && deleteEnd >= commentEnd:
			// The delete covers the end of the comment.
			overlap := commentEnd - deleteStart
			cropped = append(cropped, textot.Component{
				Kind: textot.Comment, Pos: commentStart,
				Text:   comment.Op.Text.Slice(0, comment.Op.Len()-overlap),
				Thread: comment.Op.Thread,
			})
		}
	}
	return cropped
}

// startsWith reports whether text begins with prefix.
func startsWith(text, prefix textot.Text) bool {
	if len(prefix) > len(text) {
		return false
	}
	return text.Slice(0, len(prefix)).Equal(prefix)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
