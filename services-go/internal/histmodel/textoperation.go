package histmodel

import (
	"encoding/json"
	"errors"
	"fmt"
)

// maxStringLength is the largest document the history will hold.
const maxStringLength = 2 * 1024 * 1024

// ErrTooLong is an operation whose result would exceed that.
var ErrTooLong = errors.New("document too long")

// TextOperation is an edit to one file, expressed as a scan over the whole of
// it.
//
// BaseLength is how long the document has to be for the operation to apply, and
// TargetLength how long it will be afterwards. They are maintained as the
// operation is built rather than computed, which is what lets a mistake be
// caught where it is made.
type TextOperation struct {
	Ops []ScanOp
	// ContentHash is the hash of the resulting document, when the editor sent
	// one. The history uses it to notice that the two have drifted apart.
	ContentHash string

	BaseLength   int
	TargetLength int
}

// NewTextOperation builds an empty operation.
func NewTextOperation() *TextOperation { return &TextOperation{} }

// Retain keeps the next n code units of the document, optionally changing what
// they are marked as.
func (t *TextOperation) Retain(n int, tracking TrackingDirective) *TextOperation {
	if n <= 0 {
		return t
	}
	op := &RetainOp{Length: n, Tracking: tracking}
	t.BaseLength += n
	t.TargetLength += n
	t.appendMerging(op)
	return t
}

// Insert puts text in at the current position.
func (t *TextOperation) Insert(text string, tracking TrackingDirective, commentIDs []string) *TextOperation {
	if text == "" {
		return t
	}
	op := &InsertOp{Text: text, Tracking: tracking, CommentIDs: commentIDs}
	t.TargetLength += op.Len()

	last := t.last()
	switch {
	case last != nil && last.CanMergeWith(op):
		t.Ops[len(t.Ops)-1] = last.MergeWith(op)

	case isRemove(last):
		// A remove followed by an insert and an insert followed by a remove do
		// the same thing. Putting the insert first always means two operations
		// that behave the same are also written the same, which is what lets
		// them be compared.
		if len(t.Ops) >= 2 && t.Ops[len(t.Ops)-2].CanMergeWith(op) {
			t.Ops[len(t.Ops)-2] = t.Ops[len(t.Ops)-2].MergeWith(op)
		} else {
			removed := t.Ops[len(t.Ops)-1]
			t.Ops[len(t.Ops)-1] = op
			t.Ops = append(t.Ops, removed)
		}

	default:
		t.Ops = append(t.Ops, op)
	}
	return t
}

// Remove takes the next n code units out.
func (t *TextOperation) Remove(n int) *TextOperation {
	if n == 0 {
		return t
	}
	if n < 0 {
		n = -n
	}
	t.BaseLength += n
	t.appendMerging(&RemoveOp{Length: n})
	return t
}

// appendMerging adds a step, joining it to the last one where it can.
func (t *TextOperation) appendMerging(op ScanOp) {
	if last := t.last(); last != nil && last.CanMergeWith(op) {
		t.Ops[len(t.Ops)-1] = last.MergeWith(op)
		return
	}
	t.Ops = append(t.Ops, op)
}

func (t *TextOperation) last() ScanOp {
	if len(t.Ops) == 0 {
		return nil
	}
	return t.Ops[len(t.Ops)-1]
}

func isRemove(op ScanOp) bool {
	_, ok := op.(*RemoveOp)
	return ok
}

// IsNoop reports whether the operation changes nothing.
func (t *TextOperation) IsNoop() bool {
	if len(t.Ops) == 0 {
		return true
	}
	if len(t.Ops) == 1 {
		_, ok := t.Ops[0].(*RetainOp)
		return ok
	}
	return false
}

// rawTextOperation is the wire form.
type rawTextOperation struct {
	TextOperation []json.RawMessage `json:"textOperation"`
	ContentHash   string            `json:"contentHash,omitempty"`
}

// MarshalJSON writes the operation.
func (t *TextOperation) MarshalJSON() ([]byte, error) {
	steps := make([]json.RawMessage, 0, len(t.Ops))
	for _, op := range t.Ops {
		encoded, err := op.MarshalJSON()
		if err != nil {
			return nil, err
		}
		steps = append(steps, encoded)
	}
	return json.Marshal(rawTextOperation{TextOperation: steps, ContentHash: t.ContentHash})
}

// UnmarshalJSON reads an operation.
//
// It is rebuilt through the same builder the writers use rather than assigned
// directly, so an operation that arrives written in a form nobody would write
// -- two adjacent retains, say -- comes out in the one form.
func (t *TextOperation) UnmarshalJSON(data []byte) error {
	var raw rawTextOperation
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("%w: %v", ErrUnprocessable, err)
	}

	rebuilt := NewTextOperation()
	for _, step := range raw.TextOperation {
		op, err := scanOpFromRaw(step)
		if err != nil {
			return err
		}
		switch typed := op.(type) {
		case *RetainOp:
			rebuilt.Retain(typed.Length, typed.Tracking)
		case *InsertOp:
			rebuilt.Insert(typed.Text, typed.Tracking, typed.CommentIDs)
		case *RemoveOp:
			rebuilt.Remove(typed.Length)
		}
	}
	rebuilt.ContentHash = raw.ContentHash
	*t = *rebuilt
	return nil
}

// ApplyToLength reports how long the document is after the operation, and
// checks that the operation fits it.
func (t *TextOperation) ApplyToLength(length int) (int, error) {
	if length != t.BaseLength {
		return 0, fmt.Errorf(
			"%w: the operation expects a document of %d, and this one is %d",
			ErrApply, t.BaseLength, length)
	}

	var newLength, cursor int
	for _, op := range t.Ops {
		switch typed := op.(type) {
		case *InsertOp:
			newLength += typed.Len()
		case *RetainOp:
			if cursor+typed.Length > length {
				return 0, fmt.Errorf("%w: cannot retain more than is left", ErrApply)
			}
			newLength += typed.Length
			cursor += typed.Length
		case *RemoveOp:
			cursor += typed.Length
		}
	}

	if cursor != length {
		return 0, fmt.Errorf("%w: the operation did not cover the whole document", ErrApply)
	}
	if newLength > maxStringLength {
		return 0, fmt.Errorf("%w: %d", ErrTooLong, newLength)
	}
	return newLength, nil
}

// Compose returns the operation that does what this one does and then what the
// next one does.
//
// It is what turns a run of keystrokes into one entry in the history. The two
// operations are walked together, splitting a step of either wherever the other
// one ends.
func (t *TextOperation) Compose(next *TextOperation) (*TextOperation, error) {
	if t.TargetLength != next.BaseLength {
		return nil, fmt.Errorf(
			"%w: the second operation expects a document of %d, and the first produces %d",
			ErrApply, next.BaseLength, t.TargetLength)
	}

	out := NewTextOperation()
	var i1, i2 int
	op1 := at(t.Ops, i1)
	i1++
	op2 := at(next.Ops, i2)
	i2++

	for {
		if op1 == nil && op2 == nil {
			return out, nil
		}

		// Text the first operation removed is not there for the second one to
		// have an opinion about, so it goes straight through.
		if remove, ok := op1.(*RemoveOp); ok {
			out.Remove(remove.Length)
			op1 = at(t.Ops, i1)
			i1++
			continue
		}
		// Text the second operation inserts was not there for the first one,
		// likewise.
		if insert, ok := op2.(*InsertOp); ok {
			out.Insert(insert.Text, insert.Tracking, insert.CommentIDs)
			op2 = at(next.Ops, i2)
			i2++
			continue
		}

		if op1 == nil {
			return nil, fmt.Errorf("%w: the first operation is too short", ErrApply)
		}
		if op2 == nil {
			return nil, fmt.Errorf("%w: the first operation is too long", ErrApply)
		}

		var err error
		op1, op2, err = composeStep(out, op1, op2, t.Ops, next.Ops, &i1, &i2)
		if err != nil {
			return nil, err
		}
	}
}

// composeStep handles one pairing of steps and returns the two that follow.
func composeStep(out *TextOperation, op1, op2 ScanOp, ops1, ops2 []ScanOp,
	i1, i2 *int) (ScanOp, ScanOp, error) {

	switch first := op1.(type) {
	case *RetainOp:
		switch second := op2.(type) {
		case *RetainOp:
			// Both keep the text. A mark set by the second wins, because it
			// came later; where it sets none, the first one's stands.
			tracking := second.Tracking
			if tracking == nil {
				tracking = first.Tracking
			}
			switch {
			case first.Length > second.Length:
				out.Retain(second.Length, tracking)
				return &RetainOp{Length: first.Length - second.Length, Tracking: first.Tracking},
					advance(ops2, i2), nil
			case first.Length == second.Length:
				out.Retain(first.Length, tracking)
				return advance(ops1, i1), advance(ops2, i2), nil
			default:
				out.Retain(first.Length, tracking)
				return advance(ops1, i1),
					&RetainOp{Length: second.Length - first.Length, Tracking: second.Tracking}, nil
			}

		case *RemoveOp:
			switch {
			case first.Length > second.Length:
				out.Remove(second.Length)
				return &RetainOp{Length: first.Length - second.Length, Tracking: first.Tracking},
					advance(ops2, i2), nil
			case first.Length == second.Length:
				out.Remove(second.Length)
				return advance(ops1, i1), advance(ops2, i2), nil
			default:
				out.Remove(first.Length)
				return advance(ops1, i1), &RemoveOp{Length: second.Length - first.Length}, nil
			}
		}

	case *InsertOp:
		switch second := op2.(type) {
		case *RemoveOp:
			// The second operation removes what the first one inserted, so
			// neither reaches the result.
			inserted := first.Len()
			switch {
			case inserted > second.Length:
				return &InsertOp{
					Text:     sliceUTF16(first.Text, second.Length, inserted),
					Tracking: first.Tracking, CommentIDs: first.CommentIDs,
				}, advance(ops2, i2), nil
			case inserted == second.Length:
				return advance(ops1, i1), advance(ops2, i2), nil
			default:
				return advance(ops1, i1), &RemoveOp{Length: second.Length - inserted}, nil
			}

		case *RetainOp:
			// The second operation keeps what the first one inserted, and may
			// change what it is marked as.
			commentIDs := first.CommentIDs
			var tracking TrackingDirective
			switch second.Tracking.(type) {
			case *TrackingProps:
				// A mark set by the second operation wins.
				tracking = second.Tracking
			case ClearTracking:
				// The second operation took the mark off.
			default:
				tracking = first.Tracking
			}

			inserted := first.Len()
			switch {
			case inserted > second.Length:
				out.Insert(sliceUTF16(first.Text, 0, second.Length), tracking, commentIDs)
				return &InsertOp{
					Text:     sliceUTF16(first.Text, second.Length, inserted),
					Tracking: first.Tracking, CommentIDs: first.CommentIDs,
				}, advance(ops2, i2), nil
			case inserted == second.Length:
				out.Insert(first.Text, tracking, commentIDs)
				return advance(ops1, i1), advance(ops2, i2), nil
			default:
				out.Insert(first.Text, tracking, commentIDs)
				return advance(ops1, i1),
					&RetainOp{Length: second.Length - inserted, Tracking: second.Tracking}, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("%w: cannot compose these two steps", ErrApply)
}

func at(ops []ScanOp, i int) ScanOp {
	if i < 0 || i >= len(ops) {
		return nil
	}
	return ops[i]
}

func advance(ops []ScanOp, i *int) ScanOp {
	op := at(ops, *i)
	*i++
	return op
}

// sliceUTF16 takes a range of a string measured in UTF-16 code units.
//
// Everything in the history is counted that way, and an insertion is refused if
// it contains a character that takes two of them, so this only ever cuts on a
// character boundary.
func sliceUTF16(text string, from, to int) string {
	var units, start int
	for i, r := range text {
		if units == from {
			start = i
		}
		if units == to {
			return text[start:i]
		}
		units++
		if r > 0xFFFF {
			units++
		}
	}
	if units == from {
		start = len(text)
	}
	if to >= units {
		return text[start:]
	}
	return text[start:]
}
