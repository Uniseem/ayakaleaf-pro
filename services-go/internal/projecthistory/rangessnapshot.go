package projecthistory

import (
	"context"
	"fmt"
	"strings"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/histmodel"
)

// The editor asks what a document's marks were at some version, and it asks in
// its own terms: positions in the text as it is shown, with the tracked
// deletions taken out. The history keeps them in its terms, with the deletions
// still in.
//
// So every position has to be moved back by the deletions in front of it, and
// a comment that runs over one has to be shortened: the words it covered are
// not there any more as far as the editor is concerned.

// RangesSnapshot is a document's marks in the editor's form.
type RangesSnapshot struct {
	Changes  []RangeChange  `json:"changes"`
	Comments []RangeComment `json:"comments"`
}

// RangeChange is one tracked change.
type RangeChange struct {
	Op       map[string]any    `json:"op"`
	Metadata map[string]string `json:"metadata"`
}

// RangeComment is one comment thread.
type RangeComment struct {
	Op map[string]any `json:"op"`
	ID string         `json:"id,omitempty"`
}

// GetRangesSnapshot is a document's marks as they were at a version.
func (s *Snapshots) GetRangesSnapshot(ctx context.Context, projectID string,
	version int, pathname string) (*RangesSnapshot, error) {

	file, historyID, err := s.FileAtVersion(ctx, projectID, version, pathname)
	if err != nil {
		return nil, err
	}
	if !file.Data.IsEditable() {
		// A binary file has neither tracked changes nor comments.
		return &RangesSnapshot{Changes: []RangeChange{}, Comments: []RangeComment{}}, nil
	}
	if err := file.LoadEager(ctx, s.store.BlobStoreFor(historyID)); err != nil {
		return nil, err
	}

	data := file.StringData()
	if data == nil {
		return nil, fmt.Errorf("%w: unable to read file contents", ErrFileContentEmpty)
	}
	return docUpdaterCompatibleRanges(data), nil
}

// docUpdaterCompatibleRanges turns a loaded document's marks into the form the
// editor works in.
func docUpdaterCompatibleRanges(file *histmodel.StringFileData) *RangesSnapshot {
	content := file.Content
	trackedChanges := file.TrackedChanges.Sorted()

	snapshot := &RangesSnapshot{
		Changes: make([]RangeChange, 0, len(trackedChanges)),
	}

	// Each mark moves back by the length of every tracked deletion before it.
	deletionOffset := 0
	for _, change := range trackedChanges {
		if change.Tracking == nil {
			continue
		}
		deletion := change.Tracking.Kind == "delete"
		text := sliceUTF16(content, change.Range.Start(), change.Range.End())

		op := map[string]any{"p": change.Range.Start() - deletionOffset}
		if deletion {
			op["d"] = text
		} else {
			op["i"] = text
		}
		snapshot.Changes = append(snapshot.Changes, RangeChange{
			Op: op,
			Metadata: map[string]string{
				"ts":      change.Tracking.TS.UTC().Format("2006-01-02T15:04:05.000Z"),
				"user_id": change.Tracking.UserID,
			},
		})
		if deletion {
			deletionOffset += change.Range.Length
		}
	}

	var deletions []histmodel.TrackedChange
	for _, change := range trackedChanges {
		if change.Tracking != nil && change.Tracking.Kind == "delete" {
			deletions = append(deletions, change)
		}
	}

	comments := file.Comments.ToArray()
	snapshot.Comments = make([]RangeComment, 0, len(comments))
	for _, comment := range comments {
		if len(comment.Ranges) == 0 {
			// A comment whose text has gone: it is kept as a thread attached
			// to nothing, at the start of the document.
			snapshot.Comments = append(snapshot.Comments, RangeComment{
				Op: map[string]any{
					"p": 0, "c": "", "t": comment.ID, "resolved": comment.Resolved,
				},
			})
			continue
		}

		// A comment over several stretches is shown as one comment covering
		// all of them, because that is all the editor can show.
		start := comment.Ranges[0].Start()
		end := comment.Ranges[len(comment.Ranges)-1].End()

		index := 0
		position := start
		for index < len(deletions) && deletions[index].Range.End() <= start {
			// Wholly before the comment: it moves the comment back.
			position -= deletions[index].Range.Length
			index++
		}
		if index < len(deletions) && deletions[index].Range.Start() < start {
			// Overlapping the front of the comment: the comment starts inside
			// text that is not shown, so it starts where that text does.
			position -= start - deletions[index].Range.Start()
		}

		var text strings.Builder
		cursor := start
		for cursor < end {
			if index >= len(deletions) || deletions[index].Range.Start() >= end {
				text.WriteString(sliceUTF16(content, cursor, end))
				break
			}
			deletion := deletions[index]
			if deletion.Range.Start() > cursor {
				text.WriteString(sliceUTF16(content, cursor, deletion.Range.Start()))
			}
			if deletion.Range.End() <= end {
				cursor = deletion.Range.End()
				index++
				continue
			}
			break
		}

		snapshot.Comments = append(snapshot.Comments, RangeComment{
			Op: map[string]any{
				"p": position, "c": text.String(), "t": comment.ID,
				"resolved": comment.Resolved,
			},
			ID: comment.ID,
		})
	}
	return snapshot
}

// GetFileMetadataSnapshot is a file's metadata as it was at a version.
func (s *Snapshots) GetFileMetadataSnapshot(ctx context.Context, projectID string,
	version int, pathname string) (map[string]any, error) {

	file, _, err := s.FileAtVersion(ctx, projectID, version, pathname)
	if err != nil {
		return nil, err
	}
	fields := metadataFields(file.Metadata)
	if len(fields) == 0 {
		// Nothing rather than an empty object: the editor tells a file with no
		// metadata from one whose metadata is empty.
		return nil, nil
	}

	metadata := map[string]any{}
	for key, value := range fields {
		metadata[key] = value
	}
	return metadata, nil
}
