package projecthistory

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/histmodel"
)

// When a document is sent to the history whole -- on a resync, or when it is
// first added -- its text goes into one blob and its marks into another. This
// builds the second one.
//
// The marks arrive in the editor's form: a flat list of tracked changes and a
// flat list of comment operations, each an operation with metadata. The history
// keeps them as merged ranges over the text it stores, so the two have to be
// sorted, converted to history positions, and merged.

// ErrNotAddDocUpdate is an update that does not carry a document.
var ErrNotAddDocUpdate = errors.New("not an AddFileUpdate")

// ErrCommentStateMismatch is two operations on one comment thread that
// disagree about whether it is resolved.
var ErrCommentStateMismatch = errors.New("mismatching resolved status for comment")

// RangeBlobData is what gets stored beside a document's text.
type RangeBlobData struct {
	TrackedChanges []histmodel.TrackedChange `json:"trackedChanges"`
	Comments       []*histmodel.Comment      `json:"comments"`
}

// updateRanges is the marks as the editor sends them.
type updateRanges struct {
	Changes  []rangeEntry `json:"changes"`
	Comments []rangeEntry `json:"comments"`
}

// rangeEntry is one mark: the operation it was made by, and who made it.
type rangeEntry struct {
	Op       Op            `json:"op"`
	Metadata rangeMetadata `json:"metadata"`
}

// rangeMetadata is who made a mark and when.
type rangeMetadata struct {
	UserID string          `json:"user_id"`
	TS     json.RawMessage `json:"ts"`
}

// CreateRangeBlobDataFromUpdate builds the ranges blob for a document being
// sent whole. It returns nil when the document has no marks, in which case
// there is no second blob to store.
func CreateRangeBlobDataFromUpdate(update *Update) (*RangeBlobData, error) {
	if update.Doc == "" || !hasField(update, "docLines") {
		return nil, ErrNotAddDocUpdate
	}

	raw, ok := update.Rest["ranges"]
	if !ok || string(raw) == "null" {
		return nil, nil
	}
	var ranges updateRanges
	if err := json.Unmarshal(raw, &ranges); err != nil {
		return nil, err
	}
	if len(ranges.Changes) == 0 && len(ranges.Comments) == 0 {
		return nil, nil
	}

	trackedChanges, err := trackedChangesFromRanges(ranges.Changes)
	if err != nil {
		return nil, err
	}
	comments, err := commentsFromRanges(ranges.Comments)
	if err != nil {
		return nil, err
	}
	return &RangeBlobData{TrackedChanges: trackedChanges, Comments: comments}, nil
}

// trackedChangesFromRanges merges the tracked changes into the history's form.
func trackedChangesFromRanges(entries []rangeEntry) ([]histmodel.TrackedChange, error) {
	sorted := append([]rangeEntry(nil), entries...)
	// By position, and a deletion before an insertion at the same one: the
	// deleted text is not in what the editor showed, so both were made against
	// the same position, and the deletion is the earlier of the two in the text
	// the history keeps.
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Op.Pos != sorted[j].Op.Pos {
			return sorted[i].Op.Pos < sorted[j].Op.Pos
		}
		return sorted[i].Op.IsDelete() && sorted[j].Op.IsInsert()
	})

	list := histmodel.NewTrackedChangeList([]histmodel.TrackedChange{})
	for _, entry := range sorted {
		var kind string
		var length int
		switch {
		case entry.Op.IsDelete():
			kind, length = "delete", utf16Len(*entry.Op.Delete)
		case entry.Op.IsInsert():
			kind, length = "insert", utf16Len(*entry.Op.Insert)
		default:
			continue
		}
		pos := entry.Op.Pos
		if entry.Op.HPos != nil {
			pos = *entry.Op.HPos
		}
		timestamp, err := parseRangeTimestamp(entry.Metadata.TS)
		if err != nil {
			return nil, err
		}
		err = list.Add(histmodel.TrackedChange{
			Range: histmodel.Range{Pos: pos, Length: length},
			Tracking: &histmodel.TrackingProps{
				Kind: kind, UserID: entry.Metadata.UserID, TS: timestamp,
			},
		})
		if err != nil {
			return nil, err
		}
	}
	changes := list.Changes()
	if changes == nil {
		// An empty list is written as one, not as nothing.
		changes = []histmodel.TrackedChange{}
	}
	return changes, nil
}

// commentsFromRanges gathers the comment operations into one comment per
// thread.
func commentsFromRanges(entries []rangeEntry) ([]*histmodel.Comment, error) {
	sorted := append([]rangeEntry(nil), entries...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Op.Pos < sorted[j].Op.Pos
	})

	list := histmodel.NewCommentList(nil)
	for _, entry := range sorted {
		id := entry.Op.Thread
		resolved := entry.Op.Resolved != nil && *entry.Op.Resolved

		comment := list.GetComment(id)
		if comment == nil {
			var err error
			comment, err = histmodel.NewComment(id, nil, resolved)
			if err != nil {
				return nil, err
			}
			list.Add(comment)
		} else if comment.Resolved != resolved {
			return nil, ErrCommentStateMismatch
		}

		length := 0
		if entry.Op.Comment != nil {
			length = utf16Len(*entry.Op.Comment)
		}
		if length == 0 {
			// A comment on nothing is one that was left attached to text that
			// has since gone; it keeps its thread but no range.
			continue
		}
		pos := entry.Op.Pos
		if entry.Op.HPos != nil {
			pos = *entry.Op.HPos
		}
		merged, err := histmodel.NewComment(id,
			append(append([]histmodel.Range(nil), comment.Ranges...),
				histmodel.Range{Pos: pos, Length: length}), resolved)
		if err != nil {
			return nil, err
		}
		list.Add(merged)
	}
	return list.ToArray(), nil
}

// parseRangeTimestamp reads the time a mark was made. It arrives either as
// milliseconds or as a date, depending on how far it has travelled.
func parseRangeTimestamp(raw json.RawMessage) (time.Time, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return time.Time{}, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		parsed, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			// Not a date either: the other side would make an invalid one out
			// of it rather than refusing the update.
			return time.Time{}, nil
		}
		return parsed.UTC(), nil
	}
	var millis float64
	if err := json.Unmarshal(raw, &millis); err != nil {
		return time.Time{}, fmt.Errorf("unreadable timestamp %s", raw)
	}
	return time.UnixMilli(int64(millis)).UTC(), nil
}
