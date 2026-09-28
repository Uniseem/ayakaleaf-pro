package histmodel

import (
	"encoding/json"
	"errors"
	"sort"
)

// A tracked change is a stretch of the document somebody wrote or deleted with
// track changes on. The history keeps them as ranges alongside the text, and
// every edit has to move them: an insert before one pushes it along, a delete
// through one shortens it, and a retain that clears the mark takes it away.
//
// This is the port of TrackedChange and TrackedChangeList.

// TrackedChange is one marked stretch of the document.
type TrackedChange struct {
	Range    Range
	Tracking *TrackingProps
}

// CanMerge reports whether two marks are the same person's change, adjacent.
func (t TrackedChange) CanMerge(other TrackedChange) bool {
	if t.Tracking == nil || other.Tracking == nil {
		return false
	}
	return t.Tracking.Kind == other.Tracking.Kind &&
		t.Tracking.UserID == other.Tracking.UserID &&
		t.Range.Touches(other.Range) &&
		t.Range.CanMerge(other.Range)
}

// Merge joins two marks. The caller has checked CanMerge.
//
// The later of the two times is kept, which is when the change as a whole was
// last worked on.
func (t TrackedChange) Merge(other TrackedChange) (TrackedChange, error) {
	if !t.CanMerge(other) {
		return TrackedChange{}, errors.New("cannot merge tracked changes")
	}
	merged, err := t.Range.Merge(other.Range)
	if err != nil {
		return TrackedChange{}, err
	}
	ts := t.Tracking.TS
	if other.Tracking.TS.After(ts) {
		ts = other.Tracking.TS
	}
	return TrackedChange{
		Range:    merged,
		Tracking: &TrackingProps{Kind: t.Tracking.Kind, UserID: t.Tracking.UserID, TS: ts},
	}, nil
}

// IntersectRange returns the part of this mark that falls inside a range.
func (t TrackedChange) IntersectRange(r Range) (TrackedChange, bool) {
	intersection, ok := t.Range.Intersect(r)
	if !ok {
		return TrackedChange{}, false
	}
	return TrackedChange{Range: intersection, Tracking: t.Tracking}, true
}

// rawTrackedChange is the wire form.
type rawTrackedChange struct {
	Range    Range           `json:"range"`
	Tracking json.RawMessage `json:"tracking"`
}

// MarshalJSON writes the mark.
func (t TrackedChange) MarshalJSON() ([]byte, error) {
	var tracking json.RawMessage
	if t.Tracking != nil {
		tracking = t.Tracking.Raw()
	}
	return json.Marshal(rawTrackedChange{Range: t.Range, Tracking: tracking})
}

// UnmarshalJSON reads a mark.
func (t *TrackedChange) UnmarshalJSON(data []byte) error {
	var raw rawTrackedChange
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	tracking, err := trackingFromRaw(raw.Tracking, false)
	if err != nil {
		return err
	}
	props, _ := tracking.(*TrackingProps)
	t.Range, t.Tracking = raw.Range, props
	return nil
}

// TrackedChangeList is the marks on one file, kept in order and never
// overlapping.
type TrackedChangeList struct {
	changes []TrackedChange
}

// NewTrackedChangeList builds a list.
func NewTrackedChangeList(changes []TrackedChange) *TrackedChangeList {
	list := &TrackedChangeList{changes: append([]TrackedChange(nil), changes...)}
	return list
}

// Changes returns the marks, in order.
func (l *TrackedChangeList) Changes() []TrackedChange { return l.changes }

// Length is how many marks there are.
func (l *TrackedChangeList) Length() int { return len(l.changes) }

// InRange returns the marks wholly inside a range.
func (l *TrackedChangeList) InRange(r Range) []TrackedChange {
	var found []TrackedChange
	for _, change := range l.changes {
		if r.Contains(change.Range) {
			found = append(found, change)
		}
	}
	return found
}

// IntersectRange returns the parts of the marks that fall inside a range.
func (l *TrackedChangeList) IntersectRange(r Range) []TrackedChange {
	var found []TrackedChange
	for _, change := range l.changes {
		if part, ok := change.IntersectRange(r); ok && !part.Range.IsEmpty() {
			found = append(found, part)
		}
	}
	return found
}

// RemoveInRange takes out the marks wholly inside a range.
func (l *TrackedChangeList) RemoveInRange(r Range) {
	var kept []TrackedChange
	for _, change := range l.changes {
		if !r.Contains(change.Range) {
			kept = append(kept, change)
		}
	}
	l.changes = kept
}

// Add puts a mark in and joins it to its neighbours where it can.
func (l *TrackedChangeList) Add(change TrackedChange) error {
	l.changes = append(l.changes, change)
	return l.mergeRanges()
}

// ApplyInsert moves the marks for text put in at a position, and records a new
// mark when the insert was itself tracked.
func (l *TrackedChangeList) ApplyInsert(cursor, length int,
	tracking *TrackingProps) error {

	l.applyInsert(cursor, length, tracking)
	return l.mergeRanges()
}

// ApplyRetain marks or unmarks a stretch of text that is being kept.
func (l *TrackedChangeList) ApplyRetain(cursor, length int,
	tracking TrackingDirective) error {

	l.applyRetain(cursor, length, tracking)
	return l.mergeRanges()
}

// ApplyDelete moves the marks for text taken out.
func (l *TrackedChangeList) ApplyDelete(cursor, length int) error {
	l.applyDelete(cursor, length)
	return l.mergeRanges()
}

// Sorted is the marks in order of position, which is the order they are kept
// in.
func (l *TrackedChangeList) Sorted() []TrackedChange { return l.changes }

// mergeRanges puts the marks in order and joins the ones that belong together.
//
// The marks cannot overlap: two people cannot have made the same stretch of
// text at once, and a list where they do is a mistake upstream rather than
// something to be tidied away here.
func (l *TrackedChangeList) mergeRanges() error {
	if len(l.changes) < 2 {
		return nil
	}
	sort.SliceStable(l.changes, func(i, j int) bool {
		return l.changes[i].Range.Start() < l.changes[j].Range.Start()
	})

	merged := []TrackedChange{l.changes[0]}
	for _, current := range l.changes[1:] {
		last := merged[len(merged)-1]
		if last.Range.Overlaps(current.Range) {
			return errors.New("tracked change ranges cannot overlap")
		}
		if current.Range.IsEmpty() {
			return errors.New("a tracked change range cannot be empty")
		}
		if last.CanMerge(current) {
			joined, err := last.Merge(current)
			if err != nil {
				return err
			}
			merged[len(merged)-1] = joined
			continue
		}
		merged = append(merged, current)
	}
	l.changes = merged
	return nil
}

// applyInsert moves the marks to account for text put in at a position, and
// records a new mark when the insert was itself tracked.
func (l *TrackedChangeList) applyInsert(cursor, length int, tracking *TrackingProps) {
	var updated []TrackedChange
	for _, change := range l.changes {
		switch {
		case change.Range.StartIsAfter(cursor) || cursor == change.Range.Start():
			// The insert is before the mark, so the mark moves along. Text put
			// in exactly at the start goes in front of it rather than into it.
			updated = append(updated, TrackedChange{
				Range: change.Range.MoveBy(length), Tracking: change.Tracking,
			})

		case cursor == change.Range.End():
			// Text put in exactly at the end goes after it, so it does not
			// move and does not grow.
			updated = append(updated, change)

		case change.Range.ContainsCursor(cursor):
			// The insert lands inside the mark, which splits it. The middle
			// belongs to the mark only if the insert was itself tracked, and
			// that mark is added below.
			before, _, after, err := change.Range.InsertAt(cursor, length)
			if err != nil {
				updated = append(updated, change)
				continue
			}
			if !before.IsEmpty() {
				updated = append(updated, TrackedChange{Range: before, Tracking: change.Tracking})
			}
			if !after.IsEmpty() {
				updated = append(updated, TrackedChange{Range: after, Tracking: change.Tracking})
			}

		default:
			updated = append(updated, change)
		}
	}

	if tracking != nil {
		updated = append(updated, TrackedChange{
			Range: Range{Pos: cursor, Length: length}, Tracking: tracking,
		})
	}
	l.changes = updated
}

// applyDelete moves the marks to account for text taken out.
func (l *TrackedChangeList) applyDelete(cursor, length int) {
	deleted := Range{Pos: cursor, Length: length}

	var updated []TrackedChange
	for _, change := range l.changes {
		switch {
		case deleted.Contains(change.Range):
			// The whole mark went with the text.

		case deleted.Overlaps(change.Range):
			if shortened := change.Range.Subtract(deleted); !shortened.IsEmpty() {
				updated = append(updated, TrackedChange{
					Range: shortened, Tracking: change.Tracking,
				})
			}

		case change.Range.StartIsAfter(cursor):
			updated = append(updated, TrackedChange{
				Range: change.Range.MoveBy(-length), Tracking: change.Tracking,
			})

		default:
			updated = append(updated, change)
		}
	}
	l.changes = updated
}

// applyRetain changes what a stretch of kept text is marked as.
//
// With no directive there is nothing to do: keeping text does not move
// anything. With one, whatever was marked there is taken off first, and a new
// mark added if the directive sets one rather than clearing.
func (l *TrackedChangeList) applyRetain(cursor, length int, tracking TrackingDirective) {
	if tracking == nil {
		return
	}
	retained := Range{Pos: cursor, Length: length}

	var updated []TrackedChange
	for _, change := range l.changes {
		switch {
		case retained.Contains(change.Range):
			// The mark is wholly inside what is being re-marked, so it goes.

		case retained.Overlaps(change.Range):
			switch {
			case change.Range.Contains(retained):
				// The re-marked stretch is inside the mark, which splits it.
				left, right, err := change.Range.SplitAt(cursor)
				if err != nil {
					updated = append(updated, change)
					continue
				}
				if !left.IsEmpty() {
					updated = append(updated, TrackedChange{Range: left, Tracking: change.Tracking})
				}
				if !right.IsEmpty() && right.Length > length {
					shrunk, err := right.MoveBy(length).ShrinkBy(length)
					if err == nil {
						updated = append(updated, TrackedChange{
							Range: shrunk, Tracking: change.Tracking,
						})
					}
				}

			case retained.Start() <= change.Range.Start():
				// Overlapping the front of the mark.
				_, remaining, err := change.Range.SplitAt(retained.End())
				if err == nil && !remaining.IsEmpty() {
					updated = append(updated, TrackedChange{
						Range: remaining, Tracking: change.Tracking,
					})
				}

			default:
				// Overlapping the back of the mark.
				remaining, _, err := change.Range.SplitAt(cursor)
				if err == nil && !remaining.IsEmpty() {
					updated = append(updated, TrackedChange{
						Range: remaining, Tracking: change.Tracking,
					})
				}
			}

		default:
			updated = append(updated, change)
		}
	}

	if props, ok := tracking.(*TrackingProps); ok {
		updated = append(updated, TrackedChange{Range: retained, Tracking: props})
	}
	l.changes = updated
}

// ApplyTextOperation moves every mark to account for an operation.
//
// The marks are only put back in order at the end. Doing it after each step
// would join runs that a later step is about to split again, and the result
// would depend on the order the steps happen to arrive in.
func (l *TrackedChangeList) ApplyTextOperation(operation *TextOperation) error {
	var cursor int
	for _, op := range operation.Ops {
		switch typed := op.(type) {
		case *InsertOp:
			props, _ := typed.Tracking.(*TrackingProps)
			l.applyInsert(cursor, typed.Len(), props)
			cursor += typed.Len()
		case *RemoveOp:
			l.applyDelete(cursor, typed.Length)
		case *RetainOp:
			l.applyRetain(cursor, typed.Length, typed.Tracking)
			cursor += typed.Length
		}
	}
	return l.mergeRanges()
}

// MarshalJSON writes the list.
func (l *TrackedChangeList) MarshalJSON() ([]byte, error) {
	if l == nil || len(l.changes) == 0 {
		return json.Marshal([]TrackedChange{})
	}
	return json.Marshal(l.changes)
}

// UnmarshalJSON reads a list.
func (l *TrackedChangeList) UnmarshalJSON(data []byte) error {
	var changes []TrackedChange
	if err := json.Unmarshal(data, &changes); err != nil {
		return err
	}
	l.changes = changes
	return nil
}
