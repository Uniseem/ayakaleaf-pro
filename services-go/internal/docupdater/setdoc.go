package docupdater

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/textot"
)

// SetDocOptions are the parts of a write that are not the content.
type SetDocOptions struct {
	// OriginOrSource is either a plain source name ("dropbox") or a JSON
	// object describing where the change came from. Which one it is decides
	// which metadata field it lands in, and history reads them differently.
	OriginOrSource json.RawMessage
	UserID         string
	// Undoing marks every component of the diff as an undo, which is how the
	// tracked changes tell a reverted insert from a new deletion.
	Undoing bool
	// External marks a write that did not come from the editor. Everything
	// through the HTTP API is external; appendToDoc is not.
	External bool
}

// SetDoc replaces a document with the given lines.
//
// The caller hands over the finished text, but the editors, the tracked changes
// and the history all speak in operations, so the difference against what is
// there now is worked out first and applied as an ordinary edit. That is what
// keeps a write through the API and an edit in the browser indistinguishable
// from everything downstream.
func (m *DocumentManager) SetDoc(ctx context.Context, projectID, docID string,
	newLines []string, opts SetDocOptions) (json.RawMessage, error) {
	if newLines == nil {
		return nil, fmt.Errorf("no lines were provided to setDoc")
	}

	loaded, err := m.GetDoc(ctx, projectID, docID)
	if err != nil {
		return nil, err
	}
	if loaded.Type() != TypeShareJSTextOT {
		return nil, &OTTypeMismatchError{Got: loaded.Type(), Want: TypeShareJSTextOT}
	}

	op := diffAsShareJSOp(loaded.Lines, newLines)
	if opts.Undoing {
		for i := range op {
			op[i].Undo = true
		}
	}

	if len(op) > 0 {
		update, err := buildSetDocUpdate(docID, op, loaded.Version, opts)
		if err != nil {
			return nil, err
		}
		if m.updates == nil {
			return nil, fmt.Errorf("setDoc needs an update manager")
		}
		if err := m.updates.ApplyUpdate(ctx, projectID, docID, update); err != nil {
			return nil, err
		}
	}

	// A document somebody has open is left in Redis: they are still editing it
	// and the usual flush will write it back. One this call loaded is dropped
	// again, or it would sit there for ever with nobody to flush it.
	if loaded.AlreadyLoaded {
		return m.FlushDocIfLoaded(ctx, projectID, docID)
	}
	result, err := m.FlushAndDeleteDoc(ctx, projectID, docID, false)
	// The history queue is flushed either way. It is needed when the delete
	// succeeded, and it does no harm when it did not.
	if m.historyAPI != nil {
		m.historyAPI.FlushProjectChangesAsync(ctx, projectID)
	}
	return result, err
}

// SetDocWithLock takes the document lock first.
func (m *DocumentManager) SetDocWithLock(ctx context.Context, projectID, docID string,
	newLines []string, opts SetDocOptions) (json.RawMessage, error) {
	var result json.RawMessage
	err := m.locker.WithLock(ctx, docID, func(ctx context.Context) error {
		var err error
		result, err = m.SetDoc(ctx, projectID, docID, newLines, opts)
		return err
	})
	return result, err
}

// AppendToDoc adds lines to the end of a document.
//
// It is a setDoc underneath, on the current content plus the new lines. The
// size is checked first because the diff and the update that follows it are
// wasted work on a document that will be refused anyway.
func (m *DocumentManager) AppendToDoc(ctx context.Context, projectID, docID string,
	linesToAppend []string, opts SetDocOptions) (json.RawMessage, error) {
	loaded, err := m.GetDoc(ctx, projectID, docID)
	if err != nil {
		return nil, err
	}
	if loaded.Type() != TypeShareJSTextOT {
		return nil, &OTTypeMismatchError{Got: loaded.Type(), Want: TypeShareJSTextOT}
	}

	// The newline that joins the two halves counts towards the limit.
	const newlineSize = 1
	total := totalSizeOfLines(loaded.Lines) + newlineSize + totalSizeOfLines(linesToAppend)
	if total > m.maxDocLength {
		return nil, fmt.Errorf("%w: doc would become too large if appending this text",
			ErrFileTooLarge)
	}

	combined := make([]string, 0, len(loaded.Lines)+len(linesToAppend))
	combined = append(combined, loaded.Lines...)
	combined = append(combined, linesToAppend...)

	// Not marked external: an append is a write this service made on the
	// caller's behalf, and history records it as such.
	opts.External = false
	opts.Undoing = false
	return m.SetDoc(ctx, projectID, docID, combined, opts)
}

// AppendToDocWithLock takes the document lock first.
func (m *DocumentManager) AppendToDocWithLock(ctx context.Context, projectID, docID string,
	linesToAppend []string, opts SetDocOptions) (json.RawMessage, error) {
	var result json.RawMessage
	err := m.locker.WithLock(ctx, docID, func(ctx context.Context) error {
		var err error
		result, err = m.AppendToDoc(ctx, projectID, docID, linesToAppend, opts)
		return err
	})
	return result, err
}

// buildSetDocUpdate renders the edit as an update, in the shape the update
// pipeline and everything downstream of it expect.
func buildSetDocUpdate(docID string, op textot.Op, version int64, opts SetDocOptions) (*Update, error) {
	meta := map[string]json.RawMessage{}
	// A missing user id leaves the field out rather than writing a null: an
	// undefined value disappears when the Node service encodes the update, and
	// the two have to produce the same bytes.
	if opts.UserID != "" {
		encoded, err := json.Marshal(opts.UserID)
		if err != nil {
			return nil, err
		}
		meta["user_id"] = encoded
	}
	if opts.External {
		meta["type"] = json.RawMessage(`"external"`)
	}

	// A string is a source name; an object is an origin. Which of the two it
	// is decides the field, and history treats them differently.
	if origin, source := splitOriginOrSource(opts.OriginOrSource); origin != nil {
		meta["origin"] = origin
	} else if source != nil {
		meta["source"] = source
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

// splitOriginOrSource decides whether the caller named a source or described an
// origin. Anything else is neither.
func splitOriginOrSource(raw json.RawMessage) (origin, source json.RawMessage) {
	if len(raw) == 0 {
		return nil, nil
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return nil, raw
	}
	var asObject map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asObject); err == nil && asObject != nil {
		return raw, nil
	}
	return nil, nil
}

// totalSizeOfLines is the size a document takes up, counting the newline that
// follows each line.
//
// This is the measure the size limit is expressed in, and it is not the same as
// the document length the OT code works with, which has no newline after the
// last line.
func totalSizeOfLines(lines []string) int {
	var size int
	for _, line := range lines {
		size += textot.T(line).Len() + 1
	}
	return size
}
