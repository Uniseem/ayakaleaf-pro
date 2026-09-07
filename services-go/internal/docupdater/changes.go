package docupdater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/rangestracker"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/textot"
)

// ErrHistoryRangesNotSupported is returned for a document whose tracked changes
// are also recorded in the history.
//
// That mode stores the markers a second time, in the history queue, in a form
// this port does not produce. Doing the visible half of the work and leaving
// the history without its half would put the two permanently out of step, so
// such a document is refused instead.
var ErrHistoryRangesNotSupported = errors.New(
	"history ranges support is not implemented by this implementation")

// AcceptChanges makes tracked changes permanent.
//
// The text does not move: a tracked insert is already in the document and a
// tracked delete has already been taken out of it. Accepting only removes the
// markers, so nothing is published to the editors and no operation is recorded.
//
// It returns the authors of the changes that were accepted, which is what the
// caller tells the collaborators about.
func (m *DocumentManager) AcceptChanges(ctx context.Context, projectID, docID string,
	changeIDs []string) ([]string, error) {
	loaded, err := m.GetDoc(ctx, projectID, docID)
	if err != nil {
		return nil, err
	}
	if !loaded.Loaded() {
		return nil, fmt.Errorf("%w: document %s", ErrNotFound, docID)
	}
	if loaded.Type() != TypeShareJSTextOT {
		return nil, &OTTypeMismatchError{Got: loaded.Type(), Want: TypeShareJSTextOT}
	}
	if loaded.HistoryRangesSupport {
		return nil, ErrHistoryRangesNotSupported
	}

	changes, comments, err := decodeRanges(loaded.Ranges)
	if err != nil {
		return nil, err
	}
	// Read the authors before the markers go, since they are what carries them.
	authors := changeAuthors(loaded.Ranges, changeIDs)

	tracker := rangestracker.New(changes, comments)
	tracker.RemoveChangeIDs(changeIDs)
	newRanges, err := encodeRanges(tracker)
	if err != nil {
		return nil, err
	}

	if err := m.redis.UpdateDocument(ctx, projectID, docID, loaded.Lines,
		loaded.Version, nil, newRanges, ""); err != nil {
		return nil, err
	}
	return authors, nil
}

// AcceptChangesWithLock takes the document lock first.
func (m *DocumentManager) AcceptChangesWithLock(ctx context.Context, projectID, docID string,
	changeIDs []string) ([]string, error) {
	var authors []string
	err := m.locker.WithLock(ctx, docID, func(ctx context.Context) error {
		var err error
		authors, err = m.AcceptChanges(ctx, projectID, docID, changeIDs)
		return err
	})
	return authors, err
}

// RejectChanges undoes tracked changes.
//
// Unlike accepting, this does move the text: a tracked insert has to be taken
// back out and a tracked delete put back in. So it goes through the ordinary
// update path, as an edit by whoever rejected it, and everyone watching the
// document sees it happen.
func (m *DocumentManager) RejectChanges(ctx context.Context, projectID, docID string,
	changeIDs []string, userID string) ([]string, error) {
	loaded, err := m.GetDoc(ctx, projectID, docID)
	if err != nil {
		return nil, err
	}
	if !loaded.Loaded() {
		return nil, fmt.Errorf("%w: document %s", ErrNotFound, docID)
	}
	if loaded.Type() != TypeShareJSTextOT {
		return nil, &OTTypeMismatchError{Got: loaded.Type(), Want: TypeShareJSTextOT}
	}

	changes, _, err := decodeRanges(loaded.Ranges)
	if err != nil {
		return nil, err
	}
	wanted := make(map[string]bool, len(changeIDs))
	for _, id := range changeIDs {
		wanted[id] = true
	}
	var rejecting []*rangestracker.Change
	for _, change := range changes {
		if wanted[change.ID] {
			rejecting = append(rejecting, change)
		}
	}

	// Back to front, so that undoing one change does not move the next one.
	sort.SliceStable(rejecting, func(i, j int) bool {
		return rejecting[i].Op.Pos > rejecting[j].Op.Pos
	})

	op := textot.Op{}
	rejected := make([]string, 0, len(rejecting))
	for _, change := range rejecting {
		rejected = append(rejected, change.ID)
		switch change.Op.Kind {
		case textot.Insert:
			// The insert is undone by deleting what it added.
			op = append(op, textot.Component{
				Kind: textot.Delete, Pos: change.Op.Pos, Text: change.Op.Text, Undo: true,
			})
		case textot.Delete:
			// The delete is undone by putting back what it took out.
			op = append(op, textot.Component{
				Kind: textot.Insert, Pos: change.Op.Pos, Text: change.Op.Text, Undo: true,
			})
		}
	}

	update, err := buildRejectUpdate(docID, op, loaded.Version, userID)
	if err != nil {
		return nil, err
	}
	if m.updates == nil {
		return nil, fmt.Errorf("rejecting changes needs an update manager")
	}
	if err := m.updates.ApplyUpdate(ctx, projectID, docID, update); err != nil {
		return nil, err
	}
	return rejected, nil
}

// RejectChangesWithLock takes the document lock first.
func (m *DocumentManager) RejectChangesWithLock(ctx context.Context, projectID, docID string,
	changeIDs []string, userID string) ([]string, error) {
	var rejected []string
	err := m.locker.WithLock(ctx, docID, func(ctx context.Context) error {
		var err error
		rejected, err = m.RejectChanges(ctx, projectID, docID, changeIDs, userID)
		return err
	})
	return rejected, err
}

// buildRejectUpdate renders the undo as an update.
func buildRejectUpdate(docID string, op textot.Op, version int64, userID string) (*Update, error) {
	meta := map[string]any{
		"user_id": nullIfEmpty(userID),
		// An ISO timestamp rather than the milliseconds an edit from the editor
		// carries, which is what the Node service puts here.
		"ts": time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
	}
	encodedMeta, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	encodedOp, err := json.Marshal(op)
	if err != nil {
		return nil, err
	}
	return &Update{Doc: docID, Op: encodedOp, V: version, Meta: encodedMeta}, nil
}

// GetComment returns one comment thread marker.
func (m *DocumentManager) GetComment(ctx context.Context, projectID, docID, commentID string) (json.RawMessage, error) {
	loaded, err := m.GetDoc(ctx, projectID, docID)
	if err != nil {
		return nil, err
	}
	_, comments, err := decodeRanges(loaded.Ranges)
	if err != nil {
		return nil, err
	}
	for _, comment := range comments {
		if comment.ID == commentID {
			return json.Marshal(comment)
		}
	}
	return nil, fmt.Errorf("%w: comment %s", ErrNotFound, commentID)
}

// GetCommentWithLock takes the document lock first.
func (m *DocumentManager) GetCommentWithLock(ctx context.Context, projectID, docID, commentID string) (json.RawMessage, error) {
	var comment json.RawMessage
	err := m.locker.WithLock(ctx, docID, func(ctx context.Context) error {
		var err error
		comment, err = m.GetComment(ctx, projectID, docID, commentID)
		return err
	})
	return comment, err
}

// DeleteComment removes a comment thread marker from a document.
//
// The text it was attached to stays; only the marker goes.
func (m *DocumentManager) DeleteComment(ctx context.Context, projectID, docID, commentID string) error {
	loaded, err := m.GetDoc(ctx, projectID, docID)
	if err != nil {
		return err
	}
	if !loaded.Loaded() {
		return fmt.Errorf("%w: document %s", ErrNotFound, docID)
	}
	if loaded.Type() != TypeShareJSTextOT {
		return &OTTypeMismatchError{Got: loaded.Type(), Want: TypeShareJSTextOT}
	}
	if loaded.HistoryRangesSupport {
		return ErrHistoryRangesNotSupported
	}

	changes, comments, err := decodeRanges(loaded.Ranges)
	if err != nil {
		return err
	}
	tracker := rangestracker.New(changes, comments)
	tracker.RemoveCommentID(commentID)
	newRanges, err := encodeRanges(tracker)
	if err != nil {
		return err
	}

	return m.redis.UpdateDocument(ctx, projectID, docID, loaded.Lines,
		loaded.Version, nil, newRanges, "")
}

// DeleteCommentWithLock takes the document lock first.
func (m *DocumentManager) DeleteCommentWithLock(ctx context.Context, projectID, docID, commentID string) error {
	return m.locker.WithLock(ctx, docID, func(ctx context.Context) error {
		return m.DeleteComment(ctx, projectID, docID, commentID)
	})
}

// UpdateCommentState records a comment thread as resolved or reopened.
//
// Without history ranges support there is nothing to do here: the resolved flag
// lives with the thread in the chat service, and this service is only told so
// that it can pass the fact to the history. The call is still checked, so a
// document that has gone missing is reported as such rather than silently
// accepted.
func (m *DocumentManager) UpdateCommentState(ctx context.Context, projectID, docID,
	commentID, userID string, resolved bool) error {
	loaded, err := m.GetDoc(ctx, projectID, docID)
	if err != nil {
		return err
	}
	if !loaded.Loaded() {
		return fmt.Errorf("%w: document %s", ErrNotFound, docID)
	}
	if loaded.HistoryRangesSupport {
		return ErrHistoryRangesNotSupported
	}
	return nil
}

// UpdateCommentStateWithLock takes the document lock first.
func (m *DocumentManager) UpdateCommentStateWithLock(ctx context.Context, projectID, docID,
	commentID, userID string, resolved bool) error {
	return m.locker.WithLock(ctx, docID, func(ctx context.Context) error {
		return m.UpdateCommentState(ctx, projectID, docID, commentID, userID, resolved)
	})
}
