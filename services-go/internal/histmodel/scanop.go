package histmodel

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// A text operation is a scan over the whole document: retain so much of it,
// insert this, remove that, until the end. Each step is a ScanOp.
//
// The JSON is terse because there is a great deal of it: a bare number is a
// retain, a negative number a remove, a bare string an insert. The object forms
// carry the tracked-change marks and comment ids that a bare one cannot.

// ScanOp is one step of a text operation.
type ScanOp interface {
	// Len is how much of the document the step covers: what it inserts, keeps
	// or takes out.
	Len() int
	// Equals reports whether two steps are the same step.
	Equals(other ScanOp) bool
	// CanMergeWith reports whether two adjacent steps can be written as one.
	CanMergeWith(other ScanOp) bool
	// MergeWith returns the merged step. The caller has checked CanMergeWith.
	MergeWith(other ScanOp) ScanOp
	// MarshalJSON is the wire form.
	MarshalJSON() ([]byte, error)
}

// Errors from reading and applying operations.
var (
	// ErrUnprocessable is a step that is none of the three kinds.
	ErrUnprocessable = errors.New("invalid scan op")
	// ErrInvalidInsertion is an insert that cannot be stored, such as one
	// containing a character outside the basic plane.
	ErrInvalidInsertion = errors.New("invalid insertion")
	// ErrApply is an operation that does not fit the document it is applied to.
	ErrApply = errors.New("operation cannot be applied")
)

// InsertOp puts text into the document.
type InsertOp struct {
	Text string
	// Tracking marks the text as somebody's tracked insertion.
	Tracking TrackingDirective
	// CommentIDs are the comment threads the text belongs to.
	CommentIDs []string
}

// Len is the length of the inserted text, in UTF-16 code units.
func (o *InsertOp) Len() int { return utf16Len(o.Text) }

// Equals reports whether two inserts are the same insert.
func (o *InsertOp) Equals(other ScanOp) bool {
	p, ok := other.(*InsertOp)
	if !ok || o.Text != p.Text {
		return false
	}
	if !trackingEquals(o.Tracking, p.Tracking) {
		return false
	}
	return sameIDs(o.CommentIDs, p.CommentIDs)
}

// CanMergeWith reports whether two adjacent inserts can be written as one.
func (o *InsertOp) CanMergeWith(other ScanOp) bool {
	p, ok := other.(*InsertOp)
	if !ok {
		return false
	}
	if o.Tracking != nil {
		if p.Tracking == nil || !o.Tracking.CanMergeWith(p.Tracking) {
			return false
		}
	} else if p.Tracking != nil {
		return false
	}
	return sameIDs(o.CommentIDs, p.CommentIDs)
}

// MergeWith joins two inserts.
func (o *InsertOp) MergeWith(other ScanOp) ScanOp {
	p := other.(*InsertOp)
	merged := &InsertOp{Text: o.Text + p.Text, CommentIDs: o.CommentIDs}
	if o.Tracking != nil && p.Tracking != nil {
		merged.Tracking = o.Tracking.MergeWith(p.Tracking)
	} else {
		merged.Tracking = o.Tracking
	}
	return merged
}

// MarshalJSON writes the insert, as a bare string where it carries nothing
// else.
func (o *InsertOp) MarshalJSON() ([]byte, error) {
	if o.Tracking == nil && o.CommentIDs == nil {
		return json.Marshal(o.Text)
	}
	fields := map[string]any{"i": o.Text}
	if o.Tracking != nil {
		fields["tracking"] = o.Tracking.Raw()
	}
	if o.CommentIDs != nil {
		fields["commentIds"] = o.CommentIDs
	}
	return json.Marshal(fields)
}

// RetainOp keeps part of the document, possibly changing what it is marked as.
type RetainOp struct {
	Length int
	// Tracking marks the kept text, or clears the mark it had.
	Tracking TrackingDirective
}

// Len is how much is kept.
func (o *RetainOp) Len() int { return o.Length }

// Equals reports whether two retains are the same retain.
func (o *RetainOp) Equals(other ScanOp) bool {
	p, ok := other.(*RetainOp)
	if !ok || o.Length != p.Length {
		return false
	}
	return trackingEquals(o.Tracking, p.Tracking)
}

// CanMergeWith reports whether two adjacent retains can be written as one.
func (o *RetainOp) CanMergeWith(other ScanOp) bool {
	p, ok := other.(*RetainOp)
	if !ok {
		return false
	}
	if o.Tracking != nil {
		return p.Tracking != nil && o.Tracking.CanMergeWith(p.Tracking)
	}
	return p.Tracking == nil
}

// MergeWith joins two retains.
func (o *RetainOp) MergeWith(other ScanOp) ScanOp {
	p := other.(*RetainOp)
	merged := &RetainOp{Length: o.Length + p.Length}
	if o.Tracking != nil && p.Tracking != nil {
		merged.Tracking = o.Tracking.MergeWith(p.Tracking)
	} else {
		merged.Tracking = o.Tracking
	}
	return merged
}

// MarshalJSON writes the retain, as a bare number where it carries nothing
// else.
func (o *RetainOp) MarshalJSON() ([]byte, error) {
	if o.Tracking == nil {
		return json.Marshal(o.Length)
	}
	return json.Marshal(map[string]any{"r": o.Length, "tracking": o.Tracking.Raw()})
}

// RemoveOp takes part of the document out.
type RemoveOp struct {
	Length int
}

// Len is how much is taken out.
func (o *RemoveOp) Len() int { return o.Length }

// Equals reports whether two removes are the same remove.
func (o *RemoveOp) Equals(other ScanOp) bool {
	p, ok := other.(*RemoveOp)
	return ok && o.Length == p.Length
}

// CanMergeWith reports whether the other step is also a remove.
func (o *RemoveOp) CanMergeWith(other ScanOp) bool {
	_, ok := other.(*RemoveOp)
	return ok
}

// MergeWith joins two removes.
func (o *RemoveOp) MergeWith(other ScanOp) ScanOp {
	return &RemoveOp{Length: o.Length + other.(*RemoveOp).Length}
}

// MarshalJSON writes the remove as a negative number.
func (o *RemoveOp) MarshalJSON() ([]byte, error) {
	return json.Marshal(-o.Length)
}

// scanOpFromRaw reads one step.
//
// Which kind it is comes from its shape: a positive number retains, a negative
// one removes, a string inserts, and an object says which by the field it
// carries.
func scanOpFromRaw(raw json.RawMessage) (ScanOp, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, fmt.Errorf("%w: empty", ErrUnprocessable)
	}

	switch trimmed[0] {
	case '"':
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnprocessable, err)
		}
		return newInsertOp(text, nil, nil)

	case '{':
		return scanOpFromObject(raw)

	default:
		var number float64
		if err := json.Unmarshal(raw, &number); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnprocessable, err)
		}
		if number > 0 {
			return &RetainOp{Length: int(number)}, nil
		}
		if number < 0 {
			return &RemoveOp{Length: int(-number)}, nil
		}
		// Zero is neither: it retains nothing and removes nothing, and the
		// reference implementation refuses it rather than dropping it.
		return nil, fmt.Errorf("%w: %s", ErrUnprocessable, trimmed)
	}
}

func scanOpFromObject(raw json.RawMessage) (ScanOp, error) {
	var fields struct {
		I          *string         `json:"i"`
		R          *float64        `json:"r"`
		Tracking   json.RawMessage `json:"tracking"`
		CommentIDs []string        `json:"commentIds"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnprocessable, err)
	}

	switch {
	case fields.I != nil:
		tracking, err := trackingFromRaw(fields.Tracking, false)
		if err != nil {
			return nil, err
		}
		return newInsertOp(*fields.I, tracking, fields.CommentIDs)

	case fields.R != nil && *fields.R > 0:
		tracking, err := trackingFromRaw(fields.Tracking, true)
		if err != nil {
			return nil, err
		}
		return &RetainOp{Length: int(*fields.R), Tracking: tracking}, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrUnprocessable, raw)
}

// newInsertOp builds an insert, refusing text the history cannot store.
func newInsertOp(text string, tracking TrackingDirective, commentIDs []string) (ScanOp, error) {
	// The history stores text as UTF-16 and its length arithmetic counts code
	// units, so a character outside the basic plane -- which takes two -- would
	// make every length after it disagree. The reference implementation refuses
	// them rather than storing something it will later misread.
	if containsNonBMP(text) {
		return nil, fmt.Errorf("%w: insertion contains non-BMP characters", ErrInvalidInsertion)
	}
	return &InsertOp{Text: text, Tracking: tracking, CommentIDs: commentIDs}, nil
}

// containsNonBMP reports whether the text holds a character outside the basic
// multilingual plane.
func containsNonBMP(text string) bool {
	for _, r := range text {
		if r > 0xFFFF {
			return true
		}
	}
	return false
}

// utf16Len is the length of a string in UTF-16 code units, which is what every
// length in the history means.
func utf16Len(text string) int {
	var length int
	for _, r := range text {
		length++
		if r > 0xFFFF {
			length++
		}
	}
	return length
}

// sameIDs compares two comment id lists, ignoring order, the way the reference
// implementation does.
func sameIDs(a, b []string) bool {
	if a == nil {
		return b == nil
	}
	if len(a) != len(b) {
		return false
	}
	for _, id := range a {
		found := false
		for _, other := range b {
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
