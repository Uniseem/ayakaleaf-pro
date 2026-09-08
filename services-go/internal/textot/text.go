package textot

import (
	"errors"
	"fmt"
)

// Side says which of two concurrent operations is being transformed. When both
// insert at the same position, the left one goes first.
type Side uint8

// The two transform sides.
const (
	Left Side = iota
	Right
)

// ErrDeleteMismatch is raised when a delete names text that is not there. The
// Node service turns it into DeleteMismatchError and logs it at debug: it means
// a client and the server disagree about the document, which the client
// recovers from by reloading.
var ErrDeleteMismatch = errors.New("delete component does not match deleted text")

// ErrCommentMismatch is the same for a comment.
var ErrCommentMismatch = errors.New("comment component does not match commented text")

// ErrConcurrentDeleteMismatch is raised when two operations delete different
// text from the same region, which means one of them was built against a
// document state that never existed.
var ErrConcurrentDeleteMismatch = errors.New(
	"delete ops delete different text in the same region of the document")

// Apply returns the snapshot with op applied to it.
func Apply(snapshot Text, op Op) (Text, error) {
	if err := op.Validate(); err != nil {
		return nil, err
	}
	for _, c := range op {
		switch c.Kind {
		case Insert:
			snapshot = snapshot.Inject(c.Pos, c.Text)
		case Delete:
			deleted := snapshot.Slice(c.Pos, c.Pos+c.Len())
			if !deleted.Equal(c.Text) {
				return nil, fmt.Errorf("%w: %q is not %q",
					ErrDeleteMismatch, c.Text.String(), deleted.String())
			}
			snapshot = Concat(snapshot.Slice(0, c.Pos), snapshot.From(c.Pos+c.Len()))
		case Comment:
			// A comment does not change the text; it only has to still match
			// the text it was anchored to.
			commented := snapshot.Slice(c.Pos, c.Pos+c.Len())
			if !commented.Equal(c.Text) {
				return nil, fmt.Errorf("%w: %q is not %q",
					ErrCommentMismatch, c.Text.String(), commented.String())
			}
		}
	}
	return snapshot, nil
}

// appendComponent adds c to op, merging it into the previous component when
// the two are adjacent inserts or adjacent deletes.
//
// The merge deliberately does not compare the undo flag, matching the server
// copy of ShareJS. The browser's copy does compare it -- see
// services/web/frontend/js/vendor/libs/sharejs.js:384 -- and that divergence is
// upstream's, not this port's.
func appendComponent(op Op, c Component) Op {
	// An empty insert or delete does nothing. An empty comment is left alone:
	// RangesManager drops those itself, and dropping them here would change
	// which comments survive a transform.
	if (c.Kind == Insert || c.Kind == Delete) && c.Len() == 0 {
		return op
	}
	if len(op) == 0 {
		return append(op, c)
	}

	last := op[len(op)-1]
	switch {
	case last.Kind == Insert && c.Kind == Insert &&
		last.Pos <= c.Pos && c.Pos <= last.Pos+last.Len():
		op[len(op)-1] = Component{
			Kind: Insert,
			Text: last.Text.Inject(c.Pos-last.Pos, c.Text),
			Pos:  last.Pos,
		}
	case last.Kind == Delete && c.Kind == Delete &&
		c.Pos <= last.Pos && last.Pos <= c.Pos+c.Len():
		op[len(op)-1] = Component{
			Kind: Delete,
			Text: c.Text.Inject(last.Pos-c.Pos, last.Text),
			Pos:  c.Pos,
		}
	default:
		op = append(op, c)
	}
	return op
}

// Compose returns an operation equivalent to applying op1 then op2.
func Compose(op1, op2 Op) (Op, error) {
	if err := op1.Validate(); err != nil {
		return nil, err
	}
	if err := op2.Validate(); err != nil {
		return nil, err
	}
	out := op1.Clone()
	for _, c := range op2 {
		out = appendComponent(out, c)
	}
	return out, nil
}

// Normalize compacts an operation, defaulting missing positions to zero.
func Normalize(op Op) Op {
	out := Op{}
	for _, c := range op {
		out = appendComponent(out, c)
	}
	return out
}

// transformPosition moves a position across a component.
//
// insertAfter decides where a position sitting exactly on an insert ends up:
// after the inserted text, or before it.
func transformPosition(pos int, c Component, insertAfter bool) int {
	switch c.Kind {
	case Insert:
		if c.Pos < pos || (c.Pos == pos && insertAfter) {
			return pos + c.Len()
		}
		return pos
	case Delete:
		switch {
		case pos <= c.Pos:
			return pos
		case pos <= c.Pos+c.Len():
			return c.Pos
		default:
			return pos - c.Len()
		}
	default: // Comment: the text does not move.
		return pos
	}
}

// TransformCursor moves a cursor position across an operation, which is how a
// collaborator's caret follows somebody else's edit.
func TransformCursor(position int, op Op, side Side) int {
	insertAfter := side == Right
	for _, c := range op {
		position = transformPosition(position, c, insertAfter)
	}
	return position
}

// transformComponent transforms c against otherC, appending the result to dest.
// It is asymmetric: side says which operation c belongs to.
func transformComponent(dest Op, c, otherC Component, side Side) (Op, error) {
	switch c.Kind {
	case Insert:
		return appendComponent(dest, Component{
			Kind: Insert,
			Text: c.Text,
			Pos:  transformPosition(c.Pos, otherC, side == Right),
		}), nil

	case Delete:
		switch otherC.Kind {
		case Insert:
			// The other operation inserted inside the range being deleted, so
			// the delete splits around it.
			s := c.Text
			if c.Pos < otherC.Pos {
				dest = appendComponent(dest, Component{
					Kind: Delete, Text: s.Slice(0, otherC.Pos-c.Pos), Pos: c.Pos,
				})
				s = s.From(otherC.Pos - c.Pos)
			}
			if s.Len() != 0 {
				dest = appendComponent(dest, Component{
					Kind: Delete, Text: s, Pos: c.Pos + otherC.Len(),
				})
			}
			return dest, nil

		case Delete:
			switch {
			case c.Pos >= otherC.Pos+otherC.Len():
				return appendComponent(dest, Component{
					Kind: Delete, Text: c.Text, Pos: c.Pos - otherC.Len(),
				}), nil
			case c.Pos+c.Len() <= otherC.Pos:
				return appendComponent(dest, c), nil
			default:
				// The two deletes overlap; only the part nobody else removed
				// is still there to remove.
				var remaining Text
				if c.Pos < otherC.Pos {
					remaining = Concat(remaining, c.Text.Slice(0, otherC.Pos-c.Pos))
				}
				if c.Pos+c.Len() > otherC.Pos+otherC.Len() {
					remaining = Concat(remaining, c.Text.From(otherC.Pos+otherC.Len()-c.Pos))
				}

				// Both operations claim to have deleted the overlapping text.
				// If they disagree about what it said, one of them was built
				// against a document that never existed.
				start := max(c.Pos, otherC.Pos)
				end := min(c.Pos+c.Len(), otherC.Pos+otherC.Len())
				if !c.Text.Slice(start-c.Pos, end-c.Pos).
					Equal(otherC.Text.Slice(start-otherC.Pos, end-otherC.Pos)) {
					return nil, ErrConcurrentDeleteMismatch
				}

				if remaining.Len() != 0 {
					return appendComponent(dest, Component{
						Kind: Delete,
						Text: remaining,
						Pos:  transformPosition(c.Pos, otherC, false),
					}), nil
				}
				return dest, nil
			}

		case Comment:
			return appendComponent(dest, c), nil
		}

	case Comment:
		switch otherC.Kind {
		case Insert:
			if c.Pos < otherC.Pos && otherC.Pos < c.Pos+c.Len() {
				// The insert landed inside the commented range, so the comment
				// grows to cover it.
				offset := otherC.Pos - c.Pos
				return appendComponent(dest, Component{
					Kind:   Comment,
					Text:   Concat(c.Text.Slice(0, offset), otherC.Text, c.Text.From(offset)),
					Pos:    c.Pos,
					Thread: c.Thread,
				}), nil
			}
			return appendComponent(dest, Component{
				Kind:   Comment,
				Text:   c.Text,
				Pos:    transformPosition(c.Pos, otherC, true),
				Thread: c.Thread,
			}), nil

		case Delete:
			switch {
			case c.Pos >= otherC.Pos+otherC.Len():
				return appendComponent(dest, Component{
					Kind: Comment, Text: c.Text, Pos: c.Pos - otherC.Len(), Thread: c.Thread,
				}), nil
			case c.Pos+c.Len() <= otherC.Pos:
				return appendComponent(dest, c), nil
			default:
				// The delete removed part of the commented text; the comment
				// shrinks to what is left.
				var remaining Text
				if c.Pos < otherC.Pos {
					remaining = Concat(remaining, c.Text.Slice(0, otherC.Pos-c.Pos))
				}
				if c.Pos+c.Len() > otherC.Pos+otherC.Len() {
					remaining = Concat(remaining, c.Text.From(otherC.Pos+otherC.Len()-c.Pos))
				}

				start := max(c.Pos, otherC.Pos)
				end := min(c.Pos+c.Len(), otherC.Pos+otherC.Len())
				if !c.Text.Slice(start-c.Pos, end-c.Pos).
					Equal(otherC.Text.Slice(start-otherC.Pos, end-otherC.Pos)) {
					return nil, ErrConcurrentDeleteMismatch
				}

				// Unlike the delete case, an emptied comment is still appended:
				// appendComponent only drops empty inserts and deletes.
				return appendComponent(dest, Component{
					Kind:   Comment,
					Text:   remaining,
					Pos:    transformPosition(c.Pos, otherC, false),
					Thread: c.Thread,
				}), nil
			}

		case Comment:
			return appendComponent(dest, c), nil
		}
	}
	return nil, ErrBadComponent
}

// TransformComponent transforms a single component against another. It is
// exported for the tests that exercise the transform table directly, matching
// text._tc.
func TransformComponent(dest Op, c, otherC Component, side Side) (Op, error) {
	if err := (Op{c}).Validate(); err != nil {
		return nil, err
	}
	if err := (Op{otherC}).Validate(); err != nil {
		return nil, err
	}
	return transformComponent(dest, c, otherC, side)
}

// TransformX transforms two concurrent operations against each other, returning
// the pair that makes them converge: applying leftOp then newRightOp reaches
// the same document as applying rightOp then newLeftOp.
func TransformX(leftOp, rightOp Op) (newLeftOp, newRightOp Op, err error) {
	if err := leftOp.Validate(); err != nil {
		return nil, nil, err
	}
	if err := rightOp.Validate(); err != nil {
		return nil, nil, err
	}
	return transformX(leftOp.Clone(), rightOp)
}

func transformX(leftOp, rightOp Op) (Op, Op, error) {
	newRightOp := Op{}

	for _, rc := range rightOp {
		rightComponent := &rc
		newLeftOp := Op{}

		k := 0
		for k < len(leftOp) {
			nextC := Op{}
			var err error
			newLeftOp, err = transformComponent(newLeftOp, leftOp[k], *rightComponent, Left)
			if err != nil {
				return nil, nil, err
			}
			nextC, err = transformComponent(nextC, *rightComponent, leftOp[k], Right)
			if err != nil {
				return nil, nil, err
			}
			k++

			switch len(nextC) {
			case 1:
				rightComponent = &nextC[0]
			case 0:
				// The right component was cancelled out entirely, so the rest
				// of the left operation passes through untouched.
				for _, l := range leftOp[k:] {
					newLeftOp = appendComponent(newLeftOp, l)
				}
				rightComponent = nil
			default:
				// It split into several components; transform the remainder of
				// the left operation against all of them.
				lRest, rRest, err := transformX(leftOp[k:].Clone(), nextC)
				if err != nil {
					return nil, nil, err
				}
				for _, l := range lRest {
					newLeftOp = appendComponent(newLeftOp, l)
				}
				for _, r := range rRest {
					newRightOp = appendComponent(newRightOp, r)
				}
				rightComponent = nil
			}
			if rightComponent == nil {
				break
			}
		}

		if rightComponent != nil {
			newRightOp = appendComponent(newRightOp, *rightComponent)
		}
		leftOp = newLeftOp
	}

	return leftOp, newRightOp, nil
}

// Transform rewrites op so it can be applied after otherOp was.
//
// side breaks the tie when both operations insert at the same position: the
// left one is treated as having happened first. document-updater always
// transforms the incoming update as 'left' against the ops already applied.
func Transform(op, otherOp Op, side Side) (Op, error) {
	if len(otherOp) == 0 {
		return op, nil
	}
	if err := op.Validate(); err != nil {
		return nil, err
	}
	if err := otherOp.Validate(); err != nil {
		return nil, err
	}

	if len(op) == 1 && len(otherOp) == 1 {
		return transformComponent(Op{}, op[0], otherOp[0], side)
	}

	if side == Left {
		left, _, err := transformX(op.Clone(), otherOp)
		return left, err
	}
	_, right, err := transformX(otherOp.Clone(), op)
	return right, err
}

// Invert returns the operation that undoes op. Comments cannot be inverted,
// and never are: only inserts and deletes reach this.
func Invert(op Op) Op {
	out := make(Op, 0, len(op))
	for i := len(op) - 1; i >= 0; i-- {
		c := op[i]
		switch c.Kind {
		case Insert:
			out = append(out, Component{Kind: Delete, Text: c.Text, Pos: c.Pos})
		default:
			out = append(out, Component{Kind: Insert, Text: c.Text, Pos: c.Pos})
		}
	}
	return out
}
