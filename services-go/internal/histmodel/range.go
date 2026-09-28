package histmodel

import (
	"encoding/json"
	"errors"
)

// Range is a stretch of a document, as a starting position and a length, both
// counted in UTF-16 code units.
//
// It is what a tracked change or a comment covers. All the arithmetic of moving
// markers as a document is edited comes down to the handful of operations here,
// which is why they are worth having named rather than written out at each use.
type Range struct {
	Pos    int
	Length int
}

// ErrRange is returned by the operations that cannot be carried out.
var ErrRange = errors.New("invalid range operation")

// Start is where the range begins.
func (r Range) Start() int { return r.Pos }

// End is one past where the range finishes.
func (r Range) End() int { return r.Pos + r.Length }

// IsEmpty reports whether the range covers nothing.
func (r Range) IsEmpty() bool { return r.Length == 0 }

// Equals reports whether two ranges cover the same stretch.
func (r Range) Equals(other Range) bool {
	return r.Pos == other.Pos && r.Length == other.Length
}

// StartsAfter reports whether this range begins at or after the other one ends.
func (r Range) StartsAfter(other Range) bool { return r.Start() >= other.End() }

// StartIsAfter reports whether this range begins after a position.
func (r Range) StartIsAfter(pos int) bool { return r.Start() > pos }

// Contains reports whether this range covers the whole of another.
func (r Range) Contains(other Range) bool {
	return r.Start() <= other.Start() && r.End() >= other.End()
}

// ContainsCursor reports whether a position falls within the range, its ends
// included.
func (r Range) ContainsCursor(cursor int) bool {
	return r.Start() <= cursor && r.End() >= cursor
}

// Overlaps reports whether two ranges have at least one code unit in common.
func (r Range) Overlaps(other Range) bool {
	return r.Start() < other.End() && r.End() > other.Start()
}

// OverlapsStart reports whether this range covers the beginning of another.
func (r Range) OverlapsStart(other Range) bool {
	return r.Start() <= other.Start() && r.End() > other.Start()
}

// OverlapsEnd reports whether this range covers the end of another.
func (r Range) OverlapsEnd(other Range) bool {
	return r.Start() < other.End() && r.End() >= other.End()
}

// Touches reports whether two ranges meet without overlapping.
func (r Range) Touches(other Range) bool {
	return r.End() == other.Start() || r.Start() == other.End()
}

// Subtract returns what is left of this range once another is taken out of it.
func (r Range) Subtract(other Range) Range {
	switch {
	case r.Contains(other):
		return Range{Pos: r.Pos, Length: r.Length - other.Length}
	case other.Contains(r):
		return Range{Pos: r.Pos}
	case other.Overlaps(r):
		if other.Start() < r.Start() {
			// The other range covers the front, so what is left starts where
			// the other one does.
			return Range{Pos: other.Pos, Length: r.Length - (other.End() - r.Start())}
		}
		return Range{Pos: r.Pos, Length: r.Length - (r.End() - other.Start())}
	}
	return r
}

// CanMerge reports whether two ranges can be written as one.
func (r Range) CanMerge(other Range) bool {
	return r.Overlaps(other) || r.Touches(other)
}

// Merge returns the range covering both. The caller has checked CanMerge.
func (r Range) Merge(other Range) (Range, error) {
	if !r.CanMerge(other) {
		return Range{}, errors.New("ranges cannot be merged")
	}
	pos := minInt(r.Pos, other.Pos)
	end := maxInt(r.End(), other.End())
	return Range{Pos: pos, Length: end - pos}, nil
}

// MoveBy shifts the range along.
func (r Range) MoveBy(length int) Range {
	return Range{Pos: r.Pos + length, Length: r.Length}
}

// ExtendBy makes the range longer.
func (r Range) ExtendBy(length int) Range {
	return Range{Pos: r.Pos, Length: r.Length + length}
}

// ShrinkBy makes the range shorter.
func (r Range) ShrinkBy(length int) (Range, error) {
	if r.Length-length < 0 {
		return Range{}, errors.New("cannot shrink a range by more than its length")
	}
	return Range{Pos: r.Pos, Length: r.Length - length}, nil
}

// SplitAt cuts the range in two at a position.
func (r Range) SplitAt(cursor int) (Range, Range, error) {
	if !r.ContainsCursor(cursor) {
		return Range{}, Range{}, errors.New("the cursor must be inside the range")
	}
	before := Range{Pos: r.Pos, Length: cursor - r.Pos}
	return before, Range{Pos: cursor, Length: r.Length - before.Length}, nil
}

// InsertAt cuts the range in two at a position and puts a new range of the
// given length between the halves.
func (r Range) InsertAt(cursor, length int) (Range, Range, Range, error) {
	if !r.ContainsCursor(cursor) {
		return Range{}, Range{}, Range{}, errors.New("the cursor must be inside the range")
	}
	before := Range{Pos: r.Pos, Length: cursor - r.Pos}
	inserted := Range{Pos: cursor, Length: length}
	after := Range{Pos: cursor + length, Length: r.Length - before.Length}
	return before, inserted, after, nil
}

// Intersect returns the stretch two ranges have in common, or false when they
// have none.
func (r Range) Intersect(other Range) (Range, bool) {
	switch {
	case r.Contains(other):
		return other, true
	case other.Contains(r):
		return r, true
	case other.OverlapsStart(r):
		return Range{Pos: r.Pos, Length: other.End() - r.Start()}, true
	case other.OverlapsEnd(r):
		return Range{Pos: other.Pos, Length: r.End() - other.Start()}, true
	}
	return Range{}, false
}

// rawRange is the wire form.
type rawRange struct {
	Pos    int `json:"pos"`
	Length int `json:"length"`
}

// MarshalJSON writes the range.
func (r Range) MarshalJSON() ([]byte, error) {
	return json.Marshal(rawRange{Pos: r.Pos, Length: r.Length})
}

// UnmarshalJSON reads a range.
func (r *Range) UnmarshalJSON(data []byte) error {
	var raw rawRange
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.Pos, r.Length = raw.Pos, raw.Length
	return nil
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
