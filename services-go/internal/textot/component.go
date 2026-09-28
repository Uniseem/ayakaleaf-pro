package textot

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Kind is what a component does. Exactly one of insert, delete or comment.
type Kind uint8

// The three component kinds, named after the JSON field that carries them.
const (
	Insert Kind = iota
	Delete
	Comment
)

// Component is one insert, delete or comment at a position.
//
//	{i:'str', p:100}  insert 'str' at position 100
//	{d:'str', p:100}  delete 'str' at position 100
//	{c:'str', p:100, t:'thread'}  comment on 'str' at position 100
//
// Components in an operation are executed in order, so each position assumes
// the previous components have already been applied.
type Component struct {
	Kind Kind
	Pos  int
	Text Text

	// Thread is the comment thread id ("t"). Only comments carry it.
	Thread string
	// Undo marks a component produced by an undo ("u"). RangesManager reads
	// it to decide whether an insert is rejecting a tracked delete.
	Undo bool
}

// Op is a list of components.
type Op []Component

// Len is the length of the component's text, in UTF-16 code units.
func (c Component) Len() int { return c.Text.Len() }

// componentJSON is the wire form. The field set is closed: i, d, c, p, t and
// u are the only ones the client, the ranges tracker and the OT code between
// them ever use.
type componentJSON struct {
	I *string `json:"i,omitempty"`
	D *string `json:"d,omitempty"`
	C *string `json:"c,omitempty"`
	P int     `json:"p"`
	T string  `json:"t,omitempty"`
	U bool    `json:"u,omitempty"`
}

// MarshalJSON renders a component in the shape the rest of the stack expects.
func (c Component) MarshalJSON() ([]byte, error) {
	out := componentJSON{P: c.Pos, U: c.Undo}
	text := c.Text.String()
	switch c.Kind {
	case Insert:
		out.I = &text
	case Delete:
		out.D = &text
	case Comment:
		out.C = &text
		out.T = c.Thread
	default:
		return nil, fmt.Errorf("textot: unknown component kind %d", c.Kind)
	}
	return json.Marshal(out)
}

// ErrBadComponent is returned for a component that names no operation, or more
// than one.
var ErrBadComponent = errors.New("component needs an i, d or c field")

// ErrNegativePosition is returned for a component positioned before the start
// of the document.
var ErrNegativePosition = errors.New("position cannot be negative")

// UnmarshalJSON parses a component.
//
// The Node check is `(i is string) ^ (d is string) ^ (c is string)`, which by
// the nature of exclusive-or also accepts a component carrying all three. This
// rejects that instead: it is not a shape any client produces, and guessing
// which field wins is worse than refusing the update.
func (c *Component) UnmarshalJSON(data []byte) error {
	var in componentJSON
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}

	count := 0
	if in.I != nil {
		count++
		c.Kind, c.Text = Insert, T(*in.I)
	}
	if in.D != nil {
		count++
		c.Kind, c.Text = Delete, T(*in.D)
	}
	if in.C != nil {
		count++
		c.Kind, c.Text = Comment, T(*in.C)
	}
	if count != 1 {
		return ErrBadComponent
	}
	if in.P < 0 {
		return ErrNegativePosition
	}
	c.Pos, c.Thread, c.Undo = in.P, in.T, in.U
	return nil
}

// UnmarshalJSON accepts either a list of components or a single unwrapped one.
//
// text.normalize allows the unwrapped form, and the editor still sends it for
// single-keystroke edits.
func (o *Op) UnmarshalJSON(data []byte) error {
	for _, b := range data {
		switch b {
		case ' ', '\t', '\r', '\n':
			continue
		case '{':
			var single Component
			if err := json.Unmarshal(data, &single); err != nil {
				return err
			}
			*o = Op{single}
			return nil
		}
		break
	}
	var list []Component
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	*o = list
	return nil
}

// Validate checks every component, the way checkValidOp does before each
// transform and apply.
func (o Op) Validate() error {
	for _, c := range o {
		if c.Kind != Insert && c.Kind != Delete && c.Kind != Comment {
			return ErrBadComponent
		}
		if c.Pos < 0 {
			return ErrNegativePosition
		}
	}
	return nil
}

// Clone returns a copy that shares no memory with the original, so a transform
// cannot alter its input.
func (o Op) Clone() Op {
	out := make(Op, len(o))
	for i, c := range o {
		text := make(Text, len(c.Text))
		copy(text, c.Text)
		c.Text = text
		out[i] = c
	}
	return out
}
