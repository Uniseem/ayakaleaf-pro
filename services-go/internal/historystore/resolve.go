package historystore

import (
	"context"
	"encoding/json"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/histmodel"
)

// Filling in what a change only names.
//
// A change that adds a file arrives carrying the hash of its contents and
// nothing else, because the sender has just written the blob and has no reason
// to say more about it. The history needs to know whether those bytes are text:
// a file stored as bytes can only be fetched, while one stored as text can be
// edited, and every later edit to a file added without that is refused as an
// operation on something that is not editable.
//
// The blob index already knows -- it recorded the length when the blob was
// written -- so this looks it up before the change is stored.

// resolveFiles gives every file a change adds the length its blob was recorded
// with, when it has one.
func (s *Store) resolveFiles(ctx context.Context, historyID string, changes []*histmodel.Change) {
	for _, change := range changes {
		if change == nil {
			continue
		}
		for _, operation := range change.Operations {
			add, ok := operation.(*histmodel.AddFileOperation)
			if !ok || add.File == nil {
				continue
			}
			hash := add.File.Hash()
			if hash == "" {
				continue
			}
			// Only a file that says nothing but its hash. Anything that
			// already knows what it is stays as it is.
			if _, isText := add.File.Content(); isText {
				continue
			}
			blob, err := s.FindBlob(ctx, historyID, hash)
			if err != nil || blob == nil || blob.StringLength == nil {
				continue
			}
			resolved, err := textFile(hash, add.File.RangesHash(), *blob.StringLength)
			if err != nil {
				continue
			}
			add.File = resolved
		}
	}
}

// textFile builds a file the history knows to be text.
func textFile(hash, rangesHash string, stringLength int64) (*histmodel.File, error) {
	described := map[string]any{"hash": hash, "stringLength": stringLength}
	if rangesHash != "" {
		described["rangesHash"] = rangesHash
	}
	encoded, err := json.Marshal(described)
	if err != nil {
		return nil, err
	}
	var file histmodel.File
	if err := json.Unmarshal(encoded, &file); err != nil {
		return nil, err
	}
	return &file, nil
}
