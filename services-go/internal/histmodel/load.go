package histmodel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// A file in a snapshot is usually not the file's content but a name for it:
// the hash of the blob it was stored in, how long it is, and the edits made
// since. Reading it means fetching the blob and replaying those edits.
//
// That is what loading is. It is separate from the model because it is the one
// place the model has to reach outside itself, and because most of what this
// service does with a snapshot -- counting files, finding paths, comparing
// hashes -- needs no content at all.

// ErrNotLoadable is a file whose content cannot be fetched, because what is
// known about it is not enough to find it.
var ErrNotLoadable = errors.New("file cannot be loaded")

// BlobStore is where file contents are fetched from.
type BlobStore interface {
	// GetString returns a blob's contents.
	GetString(ctx context.Context, hash string) (string, error)
}

// RangesBlob is the marks stored beside a document's text.
type RangesBlob struct {
	Comments       []*Comment      `json:"comments"`
	TrackedChanges []TrackedChange `json:"trackedChanges"`
}

// Hash is the blob a file's content is in, or "" when the file has been edited
// since and no blob holds what it now says.
func (f *File) Hash() string {
	switch data := f.Data.(type) {
	case *HashFileData:
		return data.Hash
	case *BinaryFileData:
		return data.Hash
	case *LazyStringFileData:
		if len(data.Operations) > 0 {
			return ""
		}
		return data.Hash
	}
	return ""
}

// RangesHash is the blob a file's marks are in, on the same terms.
func (f *File) RangesHash() string {
	switch data := f.Data.(type) {
	case *HashFileData:
		return data.RangesHash
	case *LazyStringFileData:
		if len(data.Operations) > 0 {
			return ""
		}
		return data.RangesHash
	}
	return ""
}

// Content is the file's text, when it has been loaded.
func (f *File) Content() (string, bool) {
	if data, ok := f.Data.(*StringFileData); ok {
		return data.Content, true
	}
	return "", false
}

// StringData is the loaded content of a text file, or nil.
func (f *File) StringData() *StringFileData {
	data, _ := f.Data.(*StringFileData)
	return data
}

// LoadEager fetches a file's content so that it can be read and edited.
//
// A file that is already loaded is left alone, and one that is not text is
// refused: there is nothing to read.
func (f *File) LoadEager(ctx context.Context, store BlobStore) error {
	switch data := f.Data.(type) {
	case *StringFileData:
		return nil

	case *LazyStringFileData:
		loaded, err := data.toEager(ctx, store)
		if err != nil {
			return err
		}
		f.Data = loaded
		return nil

	case *HashFileData:
		// Only the hash is known, so the length has to come from the blob
		// itself. The blob store this service uses cannot report it without
		// fetching the content, so the content is fetched.
		content, err := store.GetString(ctx, data.Hash)
		if err != nil {
			return err
		}
		lazy := &LazyStringFileData{
			Hash: data.Hash, RangesHash: data.RangesHash,
			StringLength_: utf16Len(content),
		}
		loaded, err := lazy.toEager(ctx, store)
		if err != nil {
			return err
		}
		f.Data = loaded
		return nil
	}
	return fmt.Errorf("%w: %T", ErrNotLoadable, f.Data)
}

// LoadLazy makes sure a file at least knows how long it is.
//
// Everything but a file known only by its hash already does, so this is a
// no-op for them, and for that one it is a fetch.
func (f *File) LoadLazy(ctx context.Context, store BlobStore) error {
	data, ok := f.Data.(*HashFileData)
	if !ok {
		return nil
	}
	content, err := store.GetString(ctx, data.Hash)
	if err != nil {
		return err
	}
	f.Data = &LazyStringFileData{
		Hash: data.Hash, RangesHash: data.RangesHash,
		StringLength_: utf16Len(content),
	}
	return nil
}

// toEager fetches the blobs a lazy file names and replays the edits made since
// they were written.
func (f *LazyStringFileData) toEager(ctx context.Context,
	store BlobStore) (*StringFileData, error) {

	content, err := store.GetString(ctx, f.Hash)
	if err != nil {
		return nil, err
	}

	file := NewStringFileData(content, nil, nil)
	if f.RangesHash != "" {
		encoded, err := store.GetString(ctx, f.RangesHash)
		if err != nil {
			return nil, err
		}
		var ranges RangesBlob
		if err := json.Unmarshal([]byte(encoded), &ranges); err != nil {
			return nil, err
		}
		file.Comments = NewCommentList(ranges.Comments)
		file.TrackedChanges = NewTrackedChangeList(ranges.TrackedChanges)
	}

	for index, operation := range f.Operations {
		if err := file.ApplyEdit(operation); err != nil {
			return nil, fmt.Errorf("operation %d of %d failed: %w",
				index, len(f.Operations), err)
		}
	}
	return file, nil
}

// ApplyEdit applies one edit to a loaded file.
//
// The edit is one of the five shapes the history writes, and which one is
// decided by the fields it carries: this is the same reading the reference
// implementation does, in the same order.
func (f *StringFileData) ApplyEdit(raw json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	_, named := fields["commentId"]

	switch {
	case hasField(fields, "textOperation"):
		var operation TextOperation
		if err := json.Unmarshal(raw, &operation); err != nil {
			return err
		}
		return f.Edit(&operation)

	case named && isJSONList(fields["ranges"]):
		var edit struct {
			CommentID string  `json:"commentId"`
			Ranges    []Range `json:"ranges"`
			Resolved  bool    `json:"resolved"`
		}
		if err := json.Unmarshal(raw, &edit); err != nil {
			return err
		}
		comment, err := NewComment(edit.CommentID, edit.Ranges, edit.Resolved)
		if err != nil {
			return err
		}
		f.Comments.Add(comment)
		return nil

	case hasField(fields, "deleteComment"):
		var edit struct {
			DeleteComment string `json:"deleteComment"`
		}
		if err := json.Unmarshal(raw, &edit); err != nil {
			return err
		}
		f.Comments.Delete(edit.DeleteComment)
		return nil

	case named && isJSONBoolean(fields["resolved"]):
		var edit struct {
			CommentID string `json:"commentId"`
			Resolved  bool   `json:"resolved"`
		}
		if err := json.Unmarshal(raw, &edit); err != nil {
			return err
		}
		// Setting the state of a comment that is not there does nothing,
		// rather than creating one with no ranges.
		if existing := f.Comments.GetComment(edit.CommentID); existing != nil {
			updated, err := NewComment(existing.ID, existing.Ranges, edit.Resolved)
			if err != nil {
				return err
			}
			f.Comments.Add(updated)
		}
		return nil

	case hasField(fields, "noOp"):
		return nil
	}
	return fmt.Errorf("%w: %s", ErrBadOperation, raw)
}

func hasField(fields map[string]json.RawMessage, name string) bool {
	_, ok := fields[name]
	return ok
}

func isJSONList(raw json.RawMessage) bool {
	return len(raw) > 0 && raw[0] == '['
}

func isJSONBoolean(raw json.RawMessage) bool {
	return string(raw) == "true" || string(raw) == "false"
}
