package histmodel

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// A file in a snapshot is stored in one of several shapes, and which one it is
// depends on what the reader needs from it.
//
//   - A content file carries the text, the comments and the tracked changes,
//     which is what an editable document looks like once it has been loaded.
//   - A hash file names a blob and nothing else, which is how the history
//     stores it: the text lives in the blob store, and a snapshot only has to
//     say which blob.
//   - A hollow file carries lengths and no content at all, which is enough to
//     answer a question about the shape of a project without fetching anything.
//
// Which one a raw file is comes from the fields it carries, not from a tag.

// FileData is the content of a file in whichever shape it is stored.
type FileData interface {
	// IsEditable reports whether the content is text this service can change.
	IsEditable() bool
	// StringLength is the length of the content in UTF-16 code units, or -1
	// when the shape does not know it.
	StringLength() int
	// ByteLength is the length of the content in bytes, or -1.
	ByteLength() int
	// Raw is the JSON form.
	Raw() (json.RawMessage, error)
}

// ErrBadFile is a raw file that is none of the known shapes.
var ErrBadFile = errors.New("bad file data")

// StringFileData is a text file with its markers.
type StringFileData struct {
	Content        string
	Comments       *CommentList
	TrackedChanges *TrackedChangeList
}

// NewStringFileData builds a text file.
func NewStringFileData(content string, comments *CommentList,
	trackedChanges *TrackedChangeList) *StringFileData {
	if comments == nil {
		comments = NewCommentList(nil)
	}
	if trackedChanges == nil {
		trackedChanges = NewTrackedChangeList(nil)
	}
	return &StringFileData{
		Content: content, Comments: comments, TrackedChanges: trackedChanges,
	}
}

// IsEditable reports true.
func (f *StringFileData) IsEditable() bool { return true }

// StringLength is the length of the content in UTF-16 code units.
func (f *StringFileData) StringLength() int { return utf16Len(f.Content) }

// ByteLength is the length of the content in bytes.
func (f *StringFileData) ByteLength() int { return len(f.Content) }

// GetContent returns the text.
//
// filterTrackedDeletes leaves out the stretches somebody has deleted with track
// changes on, which is how the editor shows the document; without it the text
// comes back as the history holds it, deletions included.
func (f *StringFileData) GetContent(filterTrackedDeletes bool) string {
	if !filterTrackedDeletes {
		return f.Content
	}

	var out strings.Builder
	var cursor int
	for _, change := range f.sortedTrackedChanges() {
		if change.Tracking == nil || change.Tracking.Kind != "delete" {
			continue
		}
		if cursor < change.Range.Start() {
			out.WriteString(sliceUTF16(f.Content, cursor, change.Range.Start()))
		}
		cursor = change.Range.End()
	}
	if cursor < f.StringLength() {
		out.WriteString(sliceUTF16(f.Content, cursor, f.StringLength()))
	}
	return out.String()
}

// GetLines returns the document the way docstore holds it: one string per line,
// with the tracked deletions taken out.
func (f *StringFileData) GetLines() []string {
	return strings.Split(f.GetContent(true), "\n")
}

// sortedTrackedChanges returns the marks in position order.
func (f *StringFileData) sortedTrackedChanges() []TrackedChange {
	sorted := append([]TrackedChange(nil), f.TrackedChanges.Changes()...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Range.Start() < sorted[j].Range.Start()
	})
	return sorted
}

// Edit applies an operation to the file, moving the markers with it.
func (f *StringFileData) Edit(operation *TextOperation) error {
	content := f.Content
	if utf16Len(content) != operation.BaseLength {
		return fmt.Errorf(
			"%w: the operation expects a document of %d, and this one is %d",
			ErrApply, operation.BaseLength, utf16Len(content))
	}

	var cursor int
	var result strings.Builder
	for _, op := range operation.Ops {
		switch typed := op.(type) {
		case *RetainOp:
			if cursor+typed.Length > utf16Len(content) {
				return fmt.Errorf("%w: cannot retain more than is left", ErrApply)
			}
			result.WriteString(sliceUTF16(content, cursor, cursor+typed.Length))
			cursor += typed.Length

		case *InsertOp:
			err := f.Comments.ApplyInsert(
				Range{Pos: utf16Len(result.String()), Length: typed.Len()},
				typed.CommentIDs)
			if err != nil {
				return err
			}
			result.WriteString(typed.Text)

		case *RemoveOp:
			err := f.Comments.ApplyDelete(
				Range{Pos: utf16Len(result.String()), Length: typed.Length})
			if err != nil {
				return err
			}
			cursor += typed.Length
		}
	}

	if cursor != utf16Len(content) {
		return fmt.Errorf("%w: the operation did not cover the whole document", ErrApply)
	}
	if utf16Len(result.String()) > maxStringLength {
		return fmt.Errorf("%w: %d", ErrTooLong, utf16Len(result.String()))
	}

	if err := f.TrackedChanges.ApplyTextOperation(operation); err != nil {
		return err
	}
	f.Content = result.String()
	return nil
}

// rawStringFile is the wire form.
type rawStringFile struct {
	Content        string          `json:"content"`
	Comments       json.RawMessage `json:"comments,omitempty"`
	TrackedChanges json.RawMessage `json:"trackedChanges,omitempty"`
}

// Raw writes the file.
func (f *StringFileData) Raw() (json.RawMessage, error) {
	raw := rawStringFile{Content: f.Content}
	if f.Comments.Length() > 0 {
		encoded, err := json.Marshal(f.Comments)
		if err != nil {
			return nil, err
		}
		raw.Comments = encoded
	}
	if f.TrackedChanges.Length() > 0 {
		encoded, err := json.Marshal(f.TrackedChanges)
		if err != nil {
			return nil, err
		}
		raw.TrackedChanges = encoded
	}
	return json.Marshal(raw)
}

// HashFileData names the blobs a file is stored in.
type HashFileData struct {
	Hash string
	// RangesHash names the blob holding the comments and tracked changes,
	// when the file has any.
	RangesHash string
}

// IsEditable reports false: nothing here can be changed without fetching it.
func (f *HashFileData) IsEditable() bool { return false }

// StringLength is unknown for a file stored by hash.
func (f *HashFileData) StringLength() int { return -1 }

// ByteLength is unknown for a file stored by hash.
func (f *HashFileData) ByteLength() int { return -1 }

// Raw writes the file.
func (f *HashFileData) Raw() (json.RawMessage, error) {
	fields := map[string]any{"hash": f.Hash}
	if f.RangesHash != "" {
		fields["rangesHash"] = f.RangesHash
	}
	return json.Marshal(fields)
}

// LazyStringFileData names a blob and says how long the text in it is, so that
// a question about length can be answered without fetching it.
type LazyStringFileData struct {
	Hash          string
	RangesHash    string
	StringLength_ int
	// Operations are the edits made since the blob was written, which have to
	// be applied to it when the content is finally fetched.
	Operations []json.RawMessage
}

// IsEditable reports true: this is a text file, just not loaded yet.
func (f *LazyStringFileData) IsEditable() bool { return true }

// StringLength is the length recorded with the blob.
func (f *LazyStringFileData) StringLength() int { return f.StringLength_ }

// ByteLength is unknown without fetching the blob.
func (f *LazyStringFileData) ByteLength() int { return -1 }

// Raw writes the file.
func (f *LazyStringFileData) Raw() (json.RawMessage, error) {
	fields := map[string]any{"hash": f.Hash, "stringLength": f.StringLength_}
	if f.RangesHash != "" {
		fields["rangesHash"] = f.RangesHash
	}
	if len(f.Operations) > 0 {
		fields["operations"] = f.Operations
	}
	return json.Marshal(fields)
}

// BinaryFileData is a file that is not text: an image, a PDF.
type BinaryFileData struct {
	Hash        string
	ByteLength_ int
}

// IsEditable reports false.
func (f *BinaryFileData) IsEditable() bool { return false }

// StringLength is meaningless for a binary file.
func (f *BinaryFileData) StringLength() int { return -1 }

// ByteLength is the size of the file.
func (f *BinaryFileData) ByteLength() int { return f.ByteLength_ }

// Raw writes the file.
func (f *BinaryFileData) Raw() (json.RawMessage, error) {
	return json.Marshal(map[string]any{"hash": f.Hash, "byteLength": f.ByteLength_})
}

// HollowFileData is a file reduced to its lengths, with no content and no blob.
type HollowFileData struct {
	ByteLength_   int
	StringLength_ int
	// Text says which kind it is, since a hollow file has nothing else to tell
	// them apart by.
	Text bool
}

// IsEditable reports whether the file was text.
func (f *HollowFileData) IsEditable() bool { return f.Text }

// StringLength is the recorded length, or -1 for a binary file.
func (f *HollowFileData) StringLength() int {
	if !f.Text {
		return -1
	}
	return f.StringLength_
}

// ByteLength is the recorded size.
func (f *HollowFileData) ByteLength() int { return f.ByteLength_ }

// Raw writes the file.
func (f *HollowFileData) Raw() (json.RawMessage, error) {
	if f.Text {
		return json.Marshal(map[string]any{"stringLength": f.StringLength_})
	}
	return json.Marshal(map[string]any{"byteLength": f.ByteLength_})
}

// fileDataFromRaw reads a file, working out its shape from the fields it has.
//
// The order of the checks is the reference implementation's and matters: a file
// with both a hash and a byte length is binary, and one with both a hash and a
// string length is text that has not been loaded.
func fileDataFromRaw(raw json.RawMessage) (FileData, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadFile, err)
	}

	_, hasHash := fields["hash"]
	_, hasByteLength := fields["byteLength"]
	_, hasStringLength := fields["stringLength"]
	_, hasContent := fields["content"]

	switch {
	case hasHash && hasByteLength:
		var file struct {
			Hash       string `json:"hash"`
			ByteLength int    `json:"byteLength"`
		}
		if err := json.Unmarshal(raw, &file); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrBadFile, err)
		}
		return &BinaryFileData{Hash: file.Hash, ByteLength_: file.ByteLength}, nil

	case hasHash && hasStringLength:
		var file struct {
			Hash         string            `json:"hash"`
			RangesHash   string            `json:"rangesHash"`
			StringLength int               `json:"stringLength"`
			Operations   []json.RawMessage `json:"operations"`
		}
		if err := json.Unmarshal(raw, &file); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrBadFile, err)
		}
		return &LazyStringFileData{
			Hash: file.Hash, RangesHash: file.RangesHash,
			StringLength_: file.StringLength, Operations: file.Operations,
		}, nil

	case hasHash:
		var file struct {
			Hash       string `json:"hash"`
			RangesHash string `json:"rangesHash"`
		}
		if err := json.Unmarshal(raw, &file); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrBadFile, err)
		}
		return &HashFileData{Hash: file.Hash, RangesHash: file.RangesHash}, nil

	case hasByteLength:
		var file struct {
			ByteLength int `json:"byteLength"`
		}
		if err := json.Unmarshal(raw, &file); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrBadFile, err)
		}
		return &HollowFileData{ByteLength_: file.ByteLength}, nil

	case hasStringLength:
		var file struct {
			StringLength int `json:"stringLength"`
		}
		if err := json.Unmarshal(raw, &file); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrBadFile, err)
		}
		return &HollowFileData{StringLength_: file.StringLength, Text: true}, nil

	case hasContent:
		var file struct {
			Content        string          `json:"content"`
			Comments       *CommentList    `json:"comments"`
			TrackedChanges json.RawMessage `json:"trackedChanges"`
		}
		if err := json.Unmarshal(raw, &file); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrBadFile, err)
		}
		changes := NewTrackedChangeList(nil)
		if len(file.TrackedChanges) > 0 {
			if err := json.Unmarshal(file.TrackedChanges, changes); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrBadFile, err)
			}
		}
		return NewStringFileData(file.Content, file.Comments, changes), nil
	}

	return nil, fmt.Errorf("%w: %s", ErrBadFile, raw)
}

// File is one entry in a snapshot: its content, and whatever metadata the
// project has attached to it.
type File struct {
	Data FileData
	// Metadata is passed through untouched. It is where the linked-file
	// provider records where a file came from, among other things.
	Metadata json.RawMessage
}

// MarshalJSON writes the file, with the metadata alongside the content fields.
func (f *File) MarshalJSON() ([]byte, error) {
	raw, err := f.Data.Raw()
	if err != nil {
		return nil, err
	}
	if len(f.Metadata) == 0 || string(f.Metadata) == "null" {
		return raw, nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	// An empty metadata object is left out, which is what the reference
	// implementation does and what keeps a snapshot from growing a field per
	// file for nothing.
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(f.Metadata, &metadata); err == nil && len(metadata) == 0 {
		return json.Marshal(fields)
	}
	fields["metadata"] = f.Metadata
	return json.Marshal(fields)
}

// UnmarshalJSON reads a file.
func (f *File) UnmarshalJSON(data []byte) error {
	var envelope struct {
		Metadata json.RawMessage `json:"metadata"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	fileData, err := fileDataFromRaw(data)
	if err != nil {
		return err
	}
	f.Data, f.Metadata = fileData, envelope.Metadata
	return nil
}
