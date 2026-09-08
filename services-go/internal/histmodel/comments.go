package histmodel

import (
	"encoding/json"
	"errors"
	"sort"
)

// A comment covers one or more stretches of a document. More than one because
// an edit inside it splits it: text typed in the middle of a commented phrase
// is not part of the comment unless the editor says it is, so the comment comes
// out of that edit in two pieces.
//
// This is the port of Comment and CommentList.

// Comment is one thread and the stretches of the document it covers.
type Comment struct {
	ID     string
	Ranges []Range
	// Resolved is whether the thread has been marked as dealt with.
	Resolved bool
}

// NewComment builds a comment, putting its ranges in order and joining the ones
// that touch.
func NewComment(id string, ranges []Range, resolved bool) (*Comment, error) {
	merged, err := mergeCommentRanges(ranges)
	if err != nil {
		return nil, err
	}
	return &Comment{ID: id, Ranges: merged, Resolved: resolved}, nil
}

// IsEmpty reports whether the comment covers nothing, which is what happens
// when the text it was attached to is deleted.
func (c *Comment) IsEmpty() bool { return len(c.Ranges) == 0 }

// mergeCommentRanges puts ranges in order and joins the ones that meet.
func mergeCommentRanges(ranges []Range) ([]Range, error) {
	sorted := append([]Range(nil), ranges...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Start() < sorted[j].Start()
	})

	var merged []Range
	for _, r := range sorted {
		if r.IsEmpty() {
			continue
		}
		if len(merged) == 0 {
			merged = append(merged, r)
			continue
		}
		last := merged[len(merged)-1]
		if last.Overlaps(r) {
			return nil, errors.New("comment ranges cannot overlap")
		}
		if last.CanMerge(r) {
			joined, err := last.Merge(r)
			if err != nil {
				return nil, err
			}
			merged[len(merged)-1] = joined
			continue
		}
		merged = append(merged, r)
	}
	return merged, nil
}

// ApplyInsert moves the comment to account for text put in at a position.
//
// extend says whether the new text belongs to this comment. Text typed against
// the edge of a comment, or inside it, is not part of it unless the editor said
// so -- which is what stops a comment growing every time somebody types near
// it.
func (c *Comment) ApplyInsert(cursor, length int, extend bool) (*Comment, error) {
	var ranges []Range
	var extended bool

	for _, r := range c.Ranges {
		switch {
		case cursor == r.End():
			// Right after the comment.
			if extend {
				ranges = append(ranges, r.ExtendBy(length))
				extended = true
			} else {
				ranges = append(ranges, r)
			}

		case cursor == r.Start():
			// Right at the start of the comment.
			if extend {
				ranges = append(ranges, r.ExtendBy(length))
				extended = true
			} else {
				ranges = append(ranges, r.MoveBy(length))
			}

		case r.StartIsAfter(cursor):
			// Before the comment, so the comment moves along.
			ranges = append(ranges, r.MoveBy(length))

		case r.ContainsCursor(cursor):
			// Inside the comment. Either it grows, or it splits in two around
			// the text that is not part of it.
			if extend {
				ranges = append(ranges, r.ExtendBy(length))
				extended = true
				continue
			}
			before, _, after, err := r.InsertAt(cursor, length)
			if err != nil {
				return nil, err
			}
			ranges = append(ranges, Range{Pos: r.Pos, Length: before.Length}, after)

		default:
			// After the comment, which is unaffected.
			ranges = append(ranges, r)
		}
	}

	// Text that belongs to the comment but landed outside all of its stretches
	// becomes a new one.
	if extend && !extended {
		ranges = append(ranges, Range{Pos: cursor, Length: length})
	}
	return NewComment(c.ID, ranges, c.Resolved)
}

// ApplyDelete moves the comment to account for text taken out.
func (c *Comment) ApplyDelete(deleted Range) (*Comment, error) {
	var ranges []Range
	for _, r := range c.Ranges {
		switch {
		case r.Overlaps(deleted):
			ranges = append(ranges, r.Subtract(deleted))
		case r.StartsAfter(deleted):
			ranges = append(ranges, r.MoveBy(-deleted.Length))
		default:
			ranges = append(ranges, r)
		}
	}
	return NewComment(c.ID, ranges, c.Resolved)
}

// ApplyTextOperation moves the comment to account for a whole operation.
func (c *Comment) ApplyTextOperation(operation *TextOperation) (*Comment, error) {
	comment := c
	var cursor int
	for _, op := range operation.Ops {
		switch typed := op.(type) {
		case *RetainOp:
			cursor += typed.Length
		case *InsertOp:
			extend := containsID(typed.CommentIDs, comment.ID)
			moved, err := comment.ApplyInsert(cursor, typed.Len(), extend)
			if err != nil {
				return nil, err
			}
			comment = moved
			cursor += typed.Len()
		case *RemoveOp:
			moved, err := comment.ApplyDelete(Range{Pos: cursor, Length: typed.Length})
			if err != nil {
				return nil, err
			}
			comment = moved
		}
	}
	return comment, nil
}

// rawComment is the wire form.
type rawComment struct {
	ID       string  `json:"id"`
	Ranges   []Range `json:"ranges"`
	Resolved bool    `json:"resolved,omitempty"`
}

// MarshalJSON writes the comment.
func (c *Comment) MarshalJSON() ([]byte, error) {
	ranges := c.Ranges
	if ranges == nil {
		ranges = []Range{}
	}
	return json.Marshal(rawComment{ID: c.ID, Ranges: ranges, Resolved: c.Resolved})
}

// UnmarshalJSON reads a comment.
func (c *Comment) UnmarshalJSON(data []byte) error {
	var raw rawComment
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	built, err := NewComment(raw.ID, raw.Ranges, raw.Resolved)
	if err != nil {
		return err
	}
	*c = *built
	return nil
}

// CommentList is the comments on one file.
//
// The order they arrived in is kept, because it is the order they are written
// back in and the two implementations have to produce the same bytes.
type CommentList struct {
	order    []string
	comments map[string]*Comment
}

// NewCommentList builds a list.
func NewCommentList(comments []*Comment) *CommentList {
	list := &CommentList{comments: map[string]*Comment{}}
	for _, comment := range comments {
		list.Add(comment)
	}
	return list
}

// Length is how many comments there are.
func (l *CommentList) Length() int { return len(l.comments) }

// ToArray returns the comments in the order they were added.
func (l *CommentList) ToArray() []*Comment {
	out := make([]*Comment, 0, len(l.order))
	for _, id := range l.order {
		out = append(out, l.comments[id])
	}
	return out
}

// GetComment returns one thread, or nil.
func (l *CommentList) GetComment(id string) *Comment { return l.comments[id] }

// Add puts a comment in, replacing one with the same id in its existing place.
func (l *CommentList) Add(comment *Comment) {
	if _, existing := l.comments[comment.ID]; !existing {
		l.order = append(l.order, comment.ID)
	}
	l.comments[comment.ID] = comment
}

// Delete takes a comment out.
func (l *CommentList) Delete(id string) bool {
	if _, ok := l.comments[id]; !ok {
		return false
	}
	delete(l.comments, id)
	for i, existing := range l.order {
		if existing == id {
			l.order = append(l.order[:i], l.order[i+1:]...)
			break
		}
	}
	return true
}

// ApplyInsert moves every comment to account for text put in.
func (l *CommentList) ApplyInsert(r Range, commentIDs []string) error {
	for _, id := range l.order {
		moved, err := l.comments[id].ApplyInsert(r.Pos, r.Length, containsID(commentIDs, id))
		if err != nil {
			return err
		}
		l.comments[id] = moved
	}
	return nil
}

// ApplyDelete moves every comment to account for text taken out.
func (l *CommentList) ApplyDelete(r Range) error {
	for _, id := range l.order {
		moved, err := l.comments[id].ApplyDelete(r)
		if err != nil {
			return err
		}
		l.comments[id] = moved
	}
	return nil
}

// IDsCoveringRange returns the threads that cover the whole of a range.
func (l *CommentList) IDsCoveringRange(r Range) []string {
	var ids []string
	for _, id := range l.order {
		for _, existing := range l.comments[id].Ranges {
			if existing.Contains(r) {
				ids = append(ids, id)
				break
			}
		}
	}
	return ids
}

// MarshalJSON writes the list.
func (l *CommentList) MarshalJSON() ([]byte, error) {
	if l == nil {
		return json.Marshal([]*Comment{})
	}
	return json.Marshal(l.ToArray())
}

// UnmarshalJSON reads a list.
func (l *CommentList) UnmarshalJSON(data []byte) error {
	var comments []*Comment
	if err := json.Unmarshal(data, &comments); err != nil {
		return err
	}
	*l = *NewCommentList(comments)
	return nil
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
