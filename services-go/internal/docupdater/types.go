// Package docupdater is the Go port of services/document-updater.
//
// It owns the live state of every open document: the text, its version, the
// operations recently applied to it, and the tracked changes and comments that
// move with them. All of that lives in Redis while somebody is editing, and is
// written back to the database when the last of them leaves.
package docupdater

import (
	"encoding/json"
	"errors"
	"fmt"
)

// OT types. A document is one or the other, and which one is decided by how its
// content is stored: an array of lines is the ShareJS text type, anything else
// is the newer history-ot representation.
const (
	TypeShareJSTextOT = "sharejs-text-ot"
	TypeHistoryOT     = "history-ot"
)

// Errors mirroring services/document-updater/app/js/Errors.js.
var (
	// ErrNotFound is returned for a document that is not loaded, or that
	// belongs to a different project than the caller asked about.
	ErrNotFound = errors.New("document not found")

	// ErrOpRangeNotAvailable means the operations needed to bring an update up
	// to the current version have already been trimmed out of Redis. The
	// client recovers by reloading the document.
	ErrOpRangeNotAvailable = errors.New("doc ops range is not loaded in redis")

	// ErrProjectStateChanged means the set of documents in the project changed
	// while it was being read.
	ErrProjectStateChanged = errors.New("project state changed")

	// ErrDocTooLarge blocks a document that has grown past max_doc_length.
	ErrDocTooLarge = errors.New("doc is too large")

	// ErrRangesTooLarge blocks tracked changes and comments that have grown
	// past their own limit.
	ErrRangesTooLarge = errors.New("ranges are too large")

	// ErrNullBytes catches the memory corruption the Node service found in
	// JSON.stringify: it sometimes returned null bytes at the end of a string.
	// The check outlived the bug, and is kept because a null byte reaching the
	// stored document is unrecoverable.
	ErrNullBytes = errors.New("null bytes found")

	// ErrVersionMismatch means the version in Redis is not the one the update
	// was built against, which should be impossible under the document lock.
	ErrVersionMismatch = errors.New("version mismatch, doc is corrupted")

	// ErrProjectBlocked is returned while a project is being migrated between
	// OT types and must not load documents.
	ErrProjectBlocked = errors.New("project blocked from loading docs")
)

// OTTypeMismatchError is returned when a document's OT type is not the one an
// operation needs. This port implements the ShareJS text type only; a
// history-ot document is refused rather than mishandled.
type OTTypeMismatchError struct {
	Got, Want string
}

func (e *OTTypeMismatchError) Error() string {
	return fmt.Sprintf("ot type mismatch: got %s, want %s", e.Got, e.Want)
}

// OpRangeNotAvailable carries the versions the client needs to recover: it can
// ask for operations from firstVersionInRedis onwards, or reload.
type OpRangeNotAvailable struct {
	FirstVersionInRedis int64
	Version             int64
	TTLInS              int
}

func (e *OpRangeNotAvailable) Error() string { return ErrOpRangeNotAvailable.Error() }

// Unwrap lets errors.Is match ErrOpRangeNotAvailable.
func (e *OpRangeNotAvailable) Unwrap() error { return ErrOpRangeNotAvailable }

// Doc is a document as it is stored in Redis.
type Doc struct {
	// RawLines is exactly what is stored under the docLines key: a JSON array
	// of strings for a ShareJS document, an object for a history-ot one.
	RawLines json.RawMessage
	// Lines is RawLines decoded, and is nil for a history-ot document.
	Lines []string

	Version int64
	// Ranges holds the tracked changes and comments. It is passed through
	// untouched at this layer; only RangesManager looks inside it.
	Ranges json.RawMessage

	Pathname         string
	ProjectHistoryID string

	// UnflushedTime is when the document first had changes not yet written to
	// the database, in milliseconds. Empty means there are none.
	UnflushedTime string
	LastUpdatedAt string
	LastUpdatedBy string

	HistoryRangesSupport bool
	ResolvedCommentIDs   []string
}

// Type reports which OT type the document uses, the way DocumentManager does:
// by whether the stored content is an array.
func (d *Doc) Type() string {
	if d.Lines != nil {
		return TypeShareJSTextOT
	}
	return TypeHistoryOT
}

// Loaded reports whether the document is in Redis at all.
func (d *Doc) Loaded() bool { return len(d.RawLines) > 0 }

// Update is one edit as it travels from the editor to this service.
//
// It is carried as raw JSON rather than a typed struct because it is written
// back to Redis and republished to the other editors verbatim: the fields this
// service does not read still have to survive the round trip.
type Update struct {
	Doc  string          `json:"doc,omitempty"`
	Op   json.RawMessage `json:"op,omitempty"`
	V    int64           `json:"v"`
	Hash string          `json:"hash,omitempty"`
	Meta json.RawMessage `json:"meta,omitempty"`
	Dup  bool            `json:"dup,omitempty"`

	DupIfSource []string `json:"dupIfSource,omitempty"`
	LastV       *int64   `json:"lastV,omitempty"`
}
