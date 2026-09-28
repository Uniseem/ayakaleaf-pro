package projecthistory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/histmodel"
)

// Bringing one document back into step is three comparisons, not one: its
// text, its comments, and its tracked changes. Each is done against what the
// history holds, and each produces its own update.
//
// The text is a diff. The comments are compared thread by thread. The tracked
// changes cannot be compared as a list at all, because the same marked stretch
// can be stored as one range on one side and two on the other; so both sides
// are walked as a sequence of changes in what the text is marked as, and where
// those disagree a retain is emitted that sets the mark to what the editor
// says it should be.

// expandDocContent works out what one document would have to become.
func (e *SyncUpdateExpander) expandDocContent(ctx context.Context,
	update *Update) error {

	pathname := ConvertPathname(rawString(update.Rest["path"]))
	content, err := update.ResyncDocContent()
	if err != nil {
		return err
	}
	expectedContent := content.Content

	file := e.files.GetFile(pathname)
	if file == nil {
		return fmt.Errorf("%w: unrecognised file: not in snapshot", ErrSync)
	}

	// The hash is only meaningful when the history has no unwritten edits
	// against the file; when it has, there is no blob that says what the file
	// now contains and the contents have to be compared instead.
	hashesMatch := false
	if persistedHash := file.Hash(); persistedHash != "" {
		hashesMatch = persistedHash == BlobHash([]byte(expectedContent))
	}

	historyID, err := e.web.GetHistoryID(ctx, e.projectID)
	if err != nil {
		return err
	}

	if err := file.LoadEager(ctx, e.store.BlobStoreFor(historyID)); err != nil {
		recovered, recoverErr := e.recoverCorruptedFile(update, pathname,
			expectedContent, err)
		if recoverErr != nil {
			return recoverErr
		}
		file = recovered
	}

	data := file.StringData()
	if data == nil {
		return fmt.Errorf("%w: file was not properly loaded", ErrFileContentEmpty)
	}
	persistedContent := data.GetContent(false)

	if !hashesMatch {
		expanded, err := e.queueUpdateForOutOfSyncContent(update, pathname,
			persistedContent, expectedContent)
		if err != nil {
			return err
		}
		if expanded != nil {
			// The marks are moved by the same edits, so that they are compared
			// against the text as it will be rather than as it was.
			for i := range expanded.Op {
				op := &expanded.Op[i]
				switch {
				case op.IsInsert():
					length := utf16Len(*op.Insert)
					if err := data.Comments.ApplyInsert(
						histmodel.Range{Pos: op.Pos, Length: length}, nil); err != nil {
						return err
					}
					if err := data.TrackedChanges.ApplyInsert(op.Pos, length, nil); err != nil {
						return err
					}
				case op.IsDelete():
					length := utf16Len(*op.Delete)
					if err := data.Comments.ApplyDelete(
						histmodel.Range{Pos: op.Pos, Length: length}); err != nil {
						return err
					}
					if err := data.TrackedChanges.ApplyDelete(op.Pos, length); err != nil {
						return err
					}
				}
			}
		}
	}

	if content.HistoryOTRanges != nil {
		e.queueUpdatesForOutOfSyncCommentsHistoryOT(update, pathname,
			data.Comments.ToArray(), content)
	} else {
		e.queueUpdatesForOutOfSyncComments(update, pathname,
			data.Comments.ToArray(), content)
	}

	return e.queueUpdatesForOutOfSyncTrackedChanges(update, pathname,
		data.TrackedChanges.Sorted(), content)
}

// recoverCorruptedFile removes a file the history cannot read and puts the
// editor's copy in its place.
//
// This only happens on a hard resync that asked for it: a file that cannot be
// read might be corrupted, or the history store might merely be unreachable,
// and throwing away a file for the second reason would lose whatever the
// history had of it.
func (e *SyncUpdateExpander) recoverCorruptedFile(update *Update, pathname,
	expectedContent string, cause error) (*histmodel.File, error) {

	if !e.recoverCorruptedFiles || !isDataCorruptionError(cause) {
		return nil, cause
	}
	if utf16Len(expectedContent) > histmodel.MaxStringLength {
		// Removing it would work and adding it back would not, which would
		// leave the project without the file at all.
		return nil, fmt.Errorf("%w: %d code units: %v", ErrTooLong,
			utf16Len(expectedContent), cause)
	}

	removal := &Update{Pathname: pathname, Meta: e.resyncMeta(update)}
	removal.setRest("new_pathname", "")
	e.expanded = append(e.expanded, removal)

	addition := &Update{
		Pathname: pathname, Doc: update.Doc, Meta: e.resyncMeta(update),
	}
	addition.setRest("docLines", expectedContent)
	e.expanded = append(e.expanded, addition)

	// The file is replaced in place so that the marks are compared against an
	// empty document and put back from the editor's copy.
	replacement := &histmodel.File{
		Data: histmodel.NewStringFileData(expectedContent, nil, nil),
	}
	e.files.AddFile(pathname, replacement)
	return replacement, nil
}

// isDataCorruptionError reports whether a file failed to load because what is
// stored is wrong, rather than because it could not be fetched.
func isDataCorruptionError(err error) bool {
	switch {
	case errors.Is(err, histmodel.ErrBadOperation),
		errors.Is(err, histmodel.ErrNotLoadable),
		errors.Is(err, ErrFileContentEmpty):
		return true
	}
	// The marks blob is JSON, and one that will not parse is corrupt.
	var syntax *json.SyntaxError
	var unmarshal *json.UnmarshalTypeError
	if errors.As(err, &syntax) || errors.As(err, &unmarshal) {
		return true
	}
	// An operation that will not apply to the blob it was stored against is
	// the other way the stored file can be wrong.
	return errors.Is(err, histmodel.ErrApply)
}

// queueUpdateForOutOfSyncContent queues the edit that would make the history's
// copy of a document say what the editor's does.
func (e *SyncUpdateExpander) queueUpdateForOutOfSyncContent(update *Update,
	pathname, persistedContent, expectedContent string) (*Update, error) {

	ops := diffAsShareJSOps(persistedContent, expectedContent)
	if len(ops) == 0 {
		return nil, nil
	}

	docLength := utf16Len(persistedContent)
	expanded := &Update{
		Doc: update.Doc, Op: ops,
		Meta: Meta{
			Resync: true, Origin: e.origin, TS: update.Meta.TS,
			Pathname: pathname, DocLength: &docLength,
		},
	}
	e.expanded = append(e.expanded, expanded)
	return expanded, nil
}

// queueUpdatesForOutOfSyncComments queues what would have to change for the
// history's comments to match the editor's.
func (e *SyncUpdateExpander) queueUpdatesForOutOfSyncComments(update *Update,
	pathname string, persisted []*histmodel.Comment, content *ResyncDocContentData) {

	var expected []rangeEntry
	if content.Ranges != nil {
		expected = content.Ranges.Comments
	}
	resolved := map[string]bool{}
	for _, id := range content.ResolvedCommentIDs {
		resolved[id] = true
	}

	expectedByID := map[string]*rangeEntry{}
	for i := range expected {
		expectedByID[expected[i].Op.Thread] = &expected[i]
	}
	persistedByID := map[string]*histmodel.Comment{}
	for _, comment := range persisted {
		persistedByID[comment.ID] = comment
	}

	// A comment the history has and the editor does not is one that was
	// deleted while they were out of step.
	for _, comment := range persisted {
		if _, ok := expectedByID[comment.ID]; ok {
			continue
		}
		deletion := &Update{Pathname: pathname, Meta: e.resyncMeta(update)}
		deletion.setRest("deleteComment", comment.ID)
		e.expanded = append(e.expanded, deletion)
	}

	for i := range expected {
		entry := &expected[i]
		id := entry.Op.Thread
		wantResolved := resolved[id]

		if existing, ok := persistedByID[id]; ok &&
			commentRangesAreInSync(existing, entry) {

			if existing.Resolved == wantResolved {
				continue
			}
			// Only the resolved state differs, which is a smaller change than
			// putting the comment back.
			change := &Update{Pathname: pathname, Meta: e.resyncMeta(update)}
			change.setRest("commentId", id)
			change.setRest("resolved", wantResolved)
			e.expanded = append(e.expanded, change)
			continue
		}

		op := entry.Op
		op.Resolved = &wantResolved
		docLength := utf16Len(content.Content)
		e.expanded = append(e.expanded, &Update{
			Doc: update.Doc, Op: []Op{op},
			Meta: Meta{
				Resync: true, Origin: e.origin, TS: update.Meta.TS,
				Pathname: pathname, DocLength: &docLength,
			},
		})
	}
}

// queueUpdatesForOutOfSyncCommentsHistoryOT does the same for a project whose
// editor speaks the history's own operation type, where the comments arrive in
// the history's form and can be compared to it directly.
func (e *SyncUpdateExpander) queueUpdatesForOutOfSyncCommentsHistoryOT(
	update *Update, pathname string, persisted []*histmodel.Comment,
	content *ResyncDocContentData) {

	var expected []*histmodel.Comment
	if content.HistoryOTRanges != nil {
		expected = content.HistoryOTRanges.Comments
	}

	expectedByID := map[string]*histmodel.Comment{}
	for _, comment := range expected {
		expectedByID[comment.ID] = comment
	}
	persistedByID := map[string]*histmodel.Comment{}
	for _, comment := range persisted {
		persistedByID[comment.ID] = comment
	}

	queue := func(fields map[string]any) {
		op, err := newHistoryOTOp(fields)
		if err != nil {
			return
		}
		e.expanded = append(e.expanded, &Update{
			Doc: update.Doc, Op: []Op{op},
			Meta: Meta{
				Pathname: pathname, Resync: true, Origin: e.origin,
				TS: update.Meta.TS,
			},
		})
	}

	for _, comment := range persisted {
		if _, ok := expectedByID[comment.ID]; ok {
			continue
		}
		queue(map[string]any{"deleteComment": comment.ID})
	}

	for _, comment := range expected {
		existing, ok := persistedByID[comment.ID]
		if ok && commentRangesAreInSyncHistoryOT(existing, comment) {
			if existing.Resolved == comment.Resolved {
				continue
			}
			// Only the resolved state differs, which is a smaller change than
			// putting the comment back.
			queue(map[string]any{
				"commentId": comment.ID, "resolved": comment.Resolved,
			})
			continue
		}
		ranges := comment.Ranges
		if ranges == nil {
			ranges = []histmodel.Range{}
		}
		queue(map[string]any{
			"commentId": comment.ID, "ranges": ranges,
			"resolved": comment.Resolved,
		})
	}
}

// newHistoryOTOp builds an operation that is already in the history's form.
//
// It is built as JSON and read back, so that the operation carries the shape
// it will be written in rather than whatever this service's own operation
// struct can hold.
func newHistoryOTOp(fields map[string]any) (Op, error) {
	encoded, err := json.Marshal(fields)
	if err != nil {
		return Op{}, err
	}
	var op Op
	if err := json.Unmarshal(encoded, &op); err != nil {
		return Op{}, err
	}
	return op, nil
}

// commentRangesAreInSync reports whether a comment covers the same text on
// both sides.
func commentRangesAreInSync(persisted *histmodel.Comment, expected *rangeEntry) bool {
	pos := expected.Op.Pos
	if expected.Op.HPos != nil {
		pos = *expected.Op.HPos
	}
	length := 0
	if expected.Op.Comment != nil {
		length = utf16Len(*expected.Op.Comment)
	}
	if expected.Op.HLen != nil {
		length = *expected.Op.HLen
	}

	if length == 0 {
		// A comment of no length is one whose text has been deleted; the
		// history keeps the thread with no ranges at all.
		return len(persisted.Ranges) == 0
	}
	if len(persisted.Ranges) != 1 {
		// The editor cannot show a comment over two stretches of text, so a
		// comment that has more is not one it could have sent.
		return false
	}
	return persisted.Ranges[0].Pos == pos && persisted.Ranges[0].Length == length
}

// commentRangesAreInSyncHistoryOT compares two comments in the history's own
// form.
func commentRangesAreInSyncHistoryOT(persisted, expected *histmodel.Comment) bool {
	if len(persisted.Ranges) != len(expected.Ranges) {
		return false
	}
	for i := range persisted.Ranges {
		if persisted.Ranges[i] != expected.Ranges[i] {
			return false
		}
	}
	return true
}

// trackingDirective is what a stretch of text is marked as.
type trackingDirective struct {
	Kind   string
	UserID string
	TS     string
}

// none is the absence of a mark.
var noTracking = trackingDirective{Kind: "none"}

// equals reports whether two directives say the same thing.
func (t trackingDirective) equals(other trackingDirective) bool {
	if t.Kind == "none" {
		return other.Kind == "none"
	}
	return t.Kind == other.Kind && t.UserID == other.UserID && t.TS == other.TS
}

// raw is the JSON a retain carries to set this mark.
func (t trackingDirective) raw() json.RawMessage {
	if t.Kind == "none" {
		return json.RawMessage(`{"type":"none"}`)
	}
	encoded, err := json.Marshal(struct {
		Type   string `json:"type"`
		UserID string `json:"userId"`
		TS     string `json:"ts"`
	}{Type: t.Kind, UserID: t.UserID, TS: t.TS})
	if err != nil {
		return json.RawMessage(`{"type":"none"}`)
	}
	return encoded
}

// trackedChangeTransition is a position where what the text is marked as
// changes, on one side or the other.
type trackedChangeTransition struct {
	// Persisted says the change is in what the history holds; otherwise it is
	// in what the editor sent.
	Persisted bool
	Pos       int
	Tracking  trackingDirective
}

// queueUpdatesForOutOfSyncTrackedChanges queues the retains that would make
// the history's marks match the editor's.
func (e *SyncUpdateExpander) queueUpdatesForOutOfSyncTrackedChanges(
	update *Update, pathname string, persisted []histmodel.TrackedChange,
	content *ResyncDocContentData) error {

	var expected []rangeEntry
	if content.Ranges != nil {
		expected = content.Ranges.Changes
	}
	var persistedHistoryOT []histmodel.TrackedChange
	if content.HistoryOTRanges != nil {
		persistedHistoryOT = content.HistoryOTRanges.TrackedChanges
	}

	cursor := 0
	persistedTracking := noTracking
	expectedTracking := noTracking

	var ops []Op
	var current *Op
	var currentTracking trackingDirective

	transitions := trackedChangesTransitions(persisted, expected,
		persistedHistoryOT, utf16Len(content.Content))

	for _, transition := range transitions {
		if transition.Pos > cursor {
			if expectedTracking.equals(persistedTracking) {
				// The two agree over this stretch, so there is nothing to say
				// about it and whatever was being built finishes here.
				if current != nil {
					ops = append(ops, *current)
					current = nil
				}
			} else {
				retained := sliceUTF16(content.Content, cursor, transition.Pos)
				if current != nil && expectedTracking.equals(currentTracking) {
					joined := *current.Retain + retained
					current.Retain = &joined
				} else {
					if current != nil {
						ops = append(ops, *current)
					}
					text := retained
					current = &Op{
						Retain: &text, Pos: cursor,
						Tracking: expectedTracking.raw(),
					}
					currentTracking = expectedTracking
				}
			}
			cursor = transition.Pos
		}

		if transition.Persisted {
			persistedTracking = transition.Tracking
		} else {
			expectedTracking = transition.Tracking
		}
	}

	if current != nil {
		ops = append(ops, *current)
	}
	if len(ops) == 0 {
		return nil
	}

	docLength := utf16Len(content.Content)
	e.expanded = append(e.expanded, &Update{
		Doc: update.Doc, Op: ops,
		Meta: Meta{
			Resync: true, Origin: e.origin, TS: update.Meta.TS,
			Pathname: pathname, DocLength: &docLength,
		},
	})
	return nil
}

// trackedChangesTransitions is every position where the marking changes, on
// either side, in order.
//
// A mark is two transitions: one at its start saying what it is, and one at
// its end saying there is none. Walking them in order gives, for every stretch
// of the document, what each side thinks it is marked as.
func trackedChangesTransitions(persisted []histmodel.TrackedChange,
	expected []rangeEntry, persistedHistoryOT []histmodel.TrackedChange,
	docLength int) []trackedChangeTransition {

	var transitions []trackedChangeTransition

	for _, change := range persisted {
		tracking := trackingDirective{Kind: "none"}
		if change.Tracking != nil {
			tracking = trackingDirective{
				Kind: change.Tracking.Kind, UserID: change.Tracking.UserID,
				TS: change.Tracking.TS.UTC().Format("2006-01-02T15:04:05.000Z"),
			}
		}
		transitions = append(transitions,
			trackedChangeTransition{Persisted: true, Pos: change.Range.Start(),
				Tracking: tracking},
			trackedChangeTransition{Persisted: true, Pos: change.Range.End(),
				Tracking: noTracking})
	}

	for _, change := range persistedHistoryOT {
		tracking := trackingDirective{Kind: "none"}
		if change.Tracking != nil {
			tracking = trackingDirective{
				Kind: change.Tracking.Kind, UserID: change.Tracking.UserID,
				TS: change.Tracking.TS.UTC().Format("2006-01-02T15:04:05.000Z"),
			}
		}
		transitions = append(transitions,
			trackedChangeTransition{Pos: change.Range.Pos, Tracking: tracking},
			trackedChangeTransition{
				Pos: change.Range.Pos + change.Range.Length, Tracking: noTracking})
	}

	for i := range expected {
		entry := &expected[i]
		pos := entry.Op.Pos
		if entry.Op.HPos != nil {
			pos = *entry.Op.HPos
		}

		kind, length := "delete", 0
		if entry.Op.IsInsert() {
			kind, length = "insert", utf16Len(*entry.Op.Insert)
		} else if entry.Op.Delete != nil {
			length = utf16Len(*entry.Op.Delete)
		}
		transitions = append(transitions,
			trackedChangeTransition{Pos: pos, Tracking: trackingDirective{
				Kind: kind, UserID: entry.Metadata.UserID,
				TS: rawTimestampText(entry.Metadata.TS),
			}},
			trackedChangeTransition{Pos: pos + length, Tracking: noTracking})
	}

	transitions = append(transitions,
		trackedChangeTransition{Pos: docLength, Tracking: noTracking})

	sort.SliceStable(transitions, func(i, j int) bool {
		if transitions[i].Pos != transitions[j].Pos {
			return transitions[i].Pos < transitions[j].Pos
		}
		// The end of a mark comes before the start of another at the same
		// position, so that the one starting there wins.
		return transitions[i].Tracking.Kind == "none" &&
			transitions[j].Tracking.Kind != "none"
	})
	return transitions
}

// rawTimestampText is the time a mark carries, as text, so that a time the
// history wrote and a time the editor sent can be compared.
func rawTimestampText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	return string(raw)
}
