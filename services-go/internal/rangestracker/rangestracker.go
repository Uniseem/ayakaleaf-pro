// Package rangestracker keeps a document's tracked changes and comments in
// step with the edits applied to it.
//
// It is the Go port of libraries/ranges-tracker. What it stores is not the
// document but a set of markers into it: an insert marks text that is there, a
// delete marks text that is no longer there, and a comment marks a span
// somebody attached a thread to. Every edit moves them.
//
// The rules it implements, which are meant to match Word:
//
//   - Text inserted at a delete goes to the left of it.
//   - Deleting text that was itself tracked as inserted removes the insert
//     marker rather than adding a delete marker.
//   - Deletes that overlap other deletes merge.
//   - Inserts by different users never merge; an insert inside another user's
//     insert splits it in two.
//
// Getting any of this wrong is invisible at the time. The document is correct
// and the markers are not, so a comment ends up attached to the wrong words and
// nobody finds out until they read it.
package rangestracker

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/textot"
)

// Errors raised when the markers no longer agree with the document.
var (
	// ErrInsertMismatch means a tracked insert no longer covers the text it
	// claims to.
	ErrInsertMismatch = errors.New("insertion does not match text in document")
	// ErrCommentMismatch means a comment no longer covers the text it was
	// attached to.
	ErrCommentMismatch = errors.New("comment does not match text in document")
	// ErrDeletedCommentMismatch means a delete and a comment disagree about the
	// text between them.
	ErrDeletedCommentMismatch = errors.New("deleted content does not match comment content")
	// ErrDeletionMismatch means a delete no longer matches the text it removes.
	ErrDeletionMismatch = errors.New("deletion does not match text in document")
)

// Metadata is what is recorded alongside a marker: who made it and when, plus
// whatever else the caller attached.
//
// It is carried as raw JSON so fields this package does not read survive being
// stored and loaded again.
type Metadata map[string]json.RawMessage

// UserID reads the user this marker belongs to.
func (m Metadata) UserID() string {
	raw, ok := m["user_id"]
	if !ok {
		return ""
	}
	var id string
	if err := json.Unmarshal(raw, &id); err != nil {
		return ""
	}
	return id
}

// Clone copies the metadata, so two markers cannot share one map.
func (m Metadata) Clone() Metadata {
	out := make(Metadata, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Change is one tracked insert or delete.
type Change struct {
	ID       string           `json:"id"`
	Op       textot.Component `json:"op"`
	Metadata Metadata         `json:"metadata"`
}

// Comment is a span of text a thread is attached to.
type Comment struct {
	ID       string           `json:"id"`
	Op       textot.Component `json:"op"`
	Metadata Metadata         `json:"metadata,omitempty"`
}

// Ranges is what is stored against a document.
type Ranges struct {
	Changes  []*Change  `json:"changes,omitempty"`
	Comments []*Comment `json:"comments,omitempty"`
}

// DirtyState records which markers changed, so a caller can tell what to
// persist and who to notify.
type DirtyState struct {
	Added   map[string]bool
	Removed map[string]bool
	Moved   map[string]bool
}

func newDirtyState() DirtyState {
	return DirtyState{
		Added: map[string]bool{}, Removed: map[string]bool{}, Moved: map[string]bool{},
	}
}

// Tracker holds a document's markers and moves them as edits arrive.
type Tracker struct {
	Changes  []*Change
	Comments []*Comment

	// TrackChanges turns on recording edits as tracked changes rather than
	// applying them silently. It follows the update: an edit made with track
	// changes on carries the id seed to use.
	TrackChanges bool

	idSeed      string
	idIncrement int

	changeDirty  DirtyState
	commentDirty DirtyState
}

// New builds a tracker over existing markers. It takes ownership of them.
func New(changes []*Change, comments []*Comment) *Tracker {
	return &Tracker{
		Changes: changes, Comments: comments,
		idSeed:       GenerateIDSeed(),
		changeDirty:  newDirtyState(),
		commentDirty: newDirtyState(),
	}
}

// SetIDSeed fixes the prefix new marker ids are built from, and restarts the
// counter.
//
// The seed comes from the update that is being applied, so that every marker an
// edit creates has a predictable id and a retry of that edit produces the same
// ones.
func (t *Tracker) SetIDSeed(seed string) {
	t.idSeed = seed
	t.idIncrement = 0
}

// IDSeed returns the current seed.
func (t *Tracker) IDSeed() string { return t.idSeed }

// GenerateIDSeed builds the first eighteen characters of a Mongo ObjectId,
// leaving six for the counter.
func GenerateIDSeed() string {
	pid := strconv.FormatInt(rand.Int64N(32767), 16)
	machine := strconv.FormatInt(rand.Int64N(16777216), 16)
	timestamp := strconv.FormatInt(time.Now().Unix(), 16)
	return leftPad(timestamp, 8) + leftPad(machine, 6) + leftPad(pid, 4)
}

// leftPad pads with zeroes to the given width, and truncates from the left when
// the value is already longer -- which is what "00000000".substr(0, 8 - n)
// does when n exceeds the width.
func leftPad(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return "00000000000000000000"[:width-len(value)] + value
}

// newID returns the next marker id.
func (t *Tracker) newID() string {
	t.idIncrement++
	increment := strconv.FormatInt(int64(t.idIncrement), 16)
	return t.idSeed + leftPad(increment, 6)
}

// ChangeDirtyState reports which tracked changes were added, removed or moved
// since the tracker was built.
func (t *Tracker) ChangeDirtyState() DirtyState { return t.changeDirty }

// CommentDirtyState reports the same for comments.
func (t *Tracker) CommentDirtyState() DirtyState { return t.commentDirty }

// ResetDirtyState forgets what has changed.
func (t *Tracker) ResetDirtyState() {
	t.changeDirty = newDirtyState()
	t.commentDirty = newDirtyState()
}

// GetComment finds a comment by id.
func (t *Tracker) GetComment(id string) *Comment {
	for _, c := range t.Comments {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// RemoveCommentID drops a comment.
func (t *Tracker) RemoveCommentID(id string) {
	comment := t.GetComment(id)
	if comment == nil {
		return
	}
	remaining := t.Comments[:0]
	for _, c := range t.Comments {
		if c.ID != id {
			remaining = append(remaining, c)
		}
	}
	t.Comments = remaining
	t.commentDirty.Removed[comment.ID] = true
}

// MoveCommentID puts a comment somewhere else.
func (t *Tracker) MoveCommentID(id string, position int, text textot.Text) {
	for _, comment := range t.Comments {
		if comment.ID == id {
			comment.Op.Pos = position
			comment.Op.Text = text
			t.commentDirty.Moved[comment.ID] = true
		}
	}
}

// GetChange finds a tracked change by id.
func (t *Tracker) GetChange(id string) *Change {
	for _, c := range t.Changes {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// GetChanges returns the changes with the given ids.
func (t *Tracker) GetChanges(ids []string) []*Change {
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	var found []*Change
	for _, change := range t.Changes {
		if wanted[change.ID] {
			found = append(found, change)
		}
	}
	return found
}

// RemoveChangeIDs drops tracked changes, which is what accepting or rejecting
// them does.
func (t *Tracker) RemoveChangeIDs(ids []string) {
	if len(ids) == 0 {
		return
	}
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	remaining := make([]*Change, 0, len(t.Changes))
	for _, change := range t.Changes {
		if wanted[change.ID] {
			t.changeDirty.Removed[change.ID] = true
		} else {
			remaining = append(remaining, change)
		}
	}
	t.Changes = remaining
}

// Validate checks that every marker still covers the text it claims to.
//
// This is the check that catches a mistake in the arithmetic below before it
// reaches the stored document.
func (t *Tracker) Validate(text textot.Text) error {
	for _, change := range t.Changes {
		if change.Op.Kind != textot.Insert {
			continue
		}
		content := text.Slice(change.Op.Pos, change.Op.Pos+change.Op.Len())
		if !content.Equal(change.Op.Text) {
			return fmt.Errorf("%w: %q is not %q", ErrInsertMismatch,
				content.String(), change.Op.Text.String())
		}
	}
	for _, comment := range t.Comments {
		content := text.Slice(comment.Op.Pos, comment.Op.Pos+comment.Op.Len())
		if !content.Equal(comment.Op.Text) {
			return fmt.Errorf("%w: %q is not %q", ErrCommentMismatch,
				content.String(), comment.Op.Text.String())
		}
	}
	return nil
}

// TrackedDeletesLength is how much text is marked deleted but still recorded,
// which history needs to work out the document's length as it stores it.
func (t *Tracker) TrackedDeletesLength() int {
	length := 0
	for _, change := range t.Changes {
		if change.Op.Kind == textot.Delete {
			length += change.Op.Len()
		}
	}
	return length
}

// ApplyOp moves the markers to account for one edit.
func (t *Tracker) ApplyOp(op textot.Component, metadata Metadata) error {
	if metadata == nil {
		metadata = Metadata{}
	}
	if _, ok := metadata["ts"]; !ok {
		metadata["ts"], _ = json.Marshal(time.Now().UTC().Format("2006-01-02T15:04:05.000Z"))
	}

	switch op.Kind {
	case textot.Insert:
		t.applyInsertToChanges(op, metadata)
		t.applyInsertToComments(op)
		return nil
	case textot.Delete:
		if err := t.applyDeleteToChanges(op, metadata); err != nil {
			return err
		}
		return t.applyDeleteToComments(op)
	case textot.Comment:
		t.addComment(op, metadata)
		return nil
	default:
		return errors.New("unknown op type")
	}
}

// ApplyOps moves the markers for a list of edits.
func (t *Tracker) ApplyOps(ops textot.Op, metadata Metadata) error {
	for _, op := range ops {
		if err := t.ApplyOp(op, metadata); err != nil {
			return err
		}
	}
	return nil
}

func (t *Tracker) addComment(op textot.Component, metadata Metadata) {
	if existing := t.GetComment(op.Thread); existing != nil {
		t.MoveCommentID(op.Thread, op.Pos, op.Text)
		return
	}
	id := op.Thread
	if id == "" {
		id = t.newID()
	}
	comment := &Comment{
		ID: id,
		// Copied, because the op will be modified in place as later edits
		// arrive.
		Op: textot.Component{
			Kind: textot.Comment, Text: cloneText(op.Text),
			Pos: op.Pos, Thread: op.Thread,
		},
		Metadata: metadata,
	}
	t.Comments = append(t.Comments, comment)
	t.commentDirty.Added[comment.ID] = true
}

func (t *Tracker) applyInsertToComments(op textot.Component) {
	for _, comment := range t.Comments {
		switch {
		case op.Pos <= comment.Op.Pos:
			comment.Op.Pos += op.Len()
			t.commentDirty.Moved[comment.ID] = true
		case op.Pos < comment.Op.Pos+comment.Op.Len():
			// The insert landed inside the commented span, so the comment
			// grows to cover it.
			offset := op.Pos - comment.Op.Pos
			comment.Op.Text = textot.Concat(
				comment.Op.Text.Slice(0, offset), op.Text, comment.Op.Text.From(offset))
			t.commentDirty.Moved[comment.ID] = true
		}
	}
}

func (t *Tracker) applyDeleteToComments(op textot.Component) error {
	opStart, opLength := op.Pos, op.Len()
	opEnd := opStart + opLength

	for _, comment := range t.Comments {
		commentStart := comment.Op.Pos
		commentEnd := commentStart + comment.Op.Len()
		commentLength := commentEnd - commentStart

		switch {
		case opEnd <= commentStart:
			comment.Op.Pos -= opLength
			t.commentDirty.Moved[comment.ID] = true
		case opStart >= commentEnd:
			// Entirely after the comment; nothing moves.
		default:
			var remainingBefore, remainingAfter textot.Text
			if opStart > commentStart {
				remainingBefore = comment.Op.Text.Slice(0, opStart-commentStart)
			}
			if opEnd < commentEnd {
				remainingAfter = comment.Op.Text.From(opEnd - commentStart)
			}

			// The delete and the comment must agree about the text between
			// them, or one of them was built against a document that never
			// existed.
			deletedComment := comment.Op.Text.Slice(
				remainingBefore.Len(), commentLength-remainingAfter.Len())
			offset := max(0, commentStart-opStart)
			deletedOpContent := op.Text.From(offset).Slice(0, deletedComment.Len())
			if !deletedComment.Equal(deletedOpContent) {
				return fmt.Errorf("%w: %q is not %q", ErrDeletedCommentMismatch,
					deletedComment.String(), deletedOpContent.String())
			}

			comment.Op.Pos = min(commentStart, opStart)
			comment.Op.Text = textot.Concat(remainingBefore, remainingAfter)
			t.commentDirty.Moved[comment.ID] = true
		}
	}
	return nil
}

func cloneText(text textot.Text) textot.Text {
	out := make(textot.Text, len(text))
	copy(out, text)
	return out
}
