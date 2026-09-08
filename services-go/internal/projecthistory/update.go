package projecthistory

import (
	"encoding/json"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/histmodel"
)

// An update is what document-updater puts on the queue. There are several
// shapes of them and they arrive mixed together: an edit to a document, a file
// added or renamed, a comment resolved, a whole document sent for a resync.
//
// They are modelled with the fields this service reads and a map for the rest,
// because most of what they carry is passed through to the history untouched
// and a field this service does not know about is still a field the history
// expects to arrive.

// Op is one operation inside an update, in the form document-updater sends.
type Op struct {
	Pos int `json:"p"`
	// Exactly one of these says what the operation does: insert, delete,
	// retain, or comment.
	Insert *string `json:"i,omitempty"`
	Delete *string `json:"d,omitempty"`
	// Retain is the text left alone, not a length: a retain says what it is
	// passing over so that the mark it carries can be checked against it.
	Retain  *string `json:"r,omitempty"`
	Comment *string `json:"c,omitempty"`

	// Tracking is the mark a retain puts on the text it passes over, which is
	// how a resync takes a tracked change off or puts one on.
	Tracking json.RawMessage `json:"tracking,omitempty"`

	// Undo marks an operation that reverses an earlier one, which the history
	// treats differently: a tracked delete that is an undo is the rejection of
	// a tracked insert.
	Undo bool `json:"u,omitempty"`

	// HPos and HLen are the position and length in the history text, which
	// still holds the tracked deletions the editor has taken out.
	HPos *int `json:"hpos,omitempty"`
	HLen *int `json:"hlen,omitempty"`

	// CommentIDs are the comment threads an insert belongs to.
	CommentIDs []string `json:"commentIds,omitempty"`
	// Thread is the comment thread a comment operation is about.
	Thread string `json:"t,omitempty"`

	// TrackedDeleteRejection marks an insert that put back exactly what a
	// tracked delete had removed.
	TrackedDeleteRejection bool `json:"trackedDeleteRejection,omitempty"`
	// TrackedChanges are the marks a delete ran over.
	TrackedChanges []TrackedChangeInOp `json:"trackedChanges,omitempty"`

	// Resolved is carried by a comment operation whose thread is resolved.
	Resolved *bool `json:"resolved,omitempty"`

	// CommentID, Ranges and DeleteComment are the operations a project using
	// the history's own operation type sends about a comment. They name a
	// thread directly rather than describing the text it covers.
	CommentID     *string           `json:"commentId,omitempty"`
	Ranges        []histmodel.Range `json:"ranges,omitempty"`
	DeleteComment *string           `json:"deleteComment,omitempty"`
}

// Millis is a time as an update carries it.
//
// The editor writes it as a number of milliseconds and a resync writes it as a
// date, so it is read as either and written back as whichever it arrived as: an
// update this service passes on should reach the history the way it was sent.
type Millis struct {
	Value int64
	raw   json.RawMessage
}

// NewMillis builds a time from a number of milliseconds.
func NewMillis(value int64) *Millis { return &Millis{Value: value} }

// UnmarshalJSON reads a time in either form.
func (m *Millis) UnmarshalJSON(data []byte) error {
	m.raw = append([]byte(nil), data...)

	var number int64
	if err := json.Unmarshal(data, &number); err == nil {
		m.Value = number
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		// A date that cannot be read is not an error: the other side makes an
		// invalid time out of it rather than refusing the update.
		if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
			m.Value = parsed.UnixMilli()
		}
		return nil
	}
	return nil
}

// MarshalJSON writes the time as it arrived.
func (m Millis) MarshalJSON() ([]byte, error) {
	if len(m.raw) > 0 {
		return m.raw, nil
	}
	return json.Marshal(m.Value)
}

// TrackedChangeInOp is one mark caught inside a delete.
type TrackedChangeInOp struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
}

// IsInsert reports whether the operation puts text in.
func (o *Op) IsInsert() bool { return o != nil && o.Insert != nil }

// IsDelete reports whether the operation takes text out.
func (o *Op) IsDelete() bool { return o != nil && o.Delete != nil }

// IsComment reports whether the operation is about a comment.
func (o *Op) IsComment() bool { return o != nil && o.Comment != nil }

// Size is the length of the text the operation names, in UTF-16 code units.
func (o *Op) Size() int {
	switch {
	case o.IsInsert():
		return utf16Len(*o.Insert)
	case o.IsDelete():
		return utf16Len(*o.Delete)
	}
	return 0
}

// HistoryOffset is how far ahead of the editor the history is at this
// operation.
//
// Two operations from the same update can have different offsets, because each
// was measured against a different state of the tracked changes. Merging two
// with different offsets would put one of them in the wrong place.
func (o *Op) HistoryOffset() int {
	if o.HPos == nil {
		return 0
	}
	return *o.HPos - o.Pos
}

// Meta is the who, when and where of an update.
type Meta struct {
	// TS is when the update was made. It arrives either as milliseconds or as
	// a date, depending on what wrote it, and is passed on as it came.
	TS     *Millis `json:"ts,omitempty"`
	UserID string  `json:"user_id,omitempty"`
	// Type is "external" for a change that did not come from the editor.
	Type   string `json:"type,omitempty"`
	Source string `json:"source,omitempty"`
	// TC is the id seed for tracked changes, set when the edit was made with
	// track changes on.
	TC string `json:"tc,omitempty"`
	// Resync marks an update that is part of putting a project back in step
	// with the history.
	Resync bool `json:"resync,omitempty"`

	Pathname string `json:"pathname,omitempty"`
	// DocLength is how long the document was before the update, as the editor
	// counts it, and HistoryDocLength as the history counts it.
	DocLength        *int   `json:"doc_length,omitempty"`
	HistoryDocLength *int   `json:"history_doc_length,omitempty"`
	DocHash          string `json:"doc_hash,omitempty"`

	Origin json.RawMessage `json:"origin,omitempty"`
}

// Update is one entry on the queue.
type Update struct {
	Doc      string `json:"doc,omitempty"`
	Pathname string `json:"pathname,omitempty"`
	// Op is the operations of a text update. After the compressor has split it
	// there is exactly one.
	Op   []Op   `json:"op,omitempty"`
	V    *int64 `json:"v,omitempty"`
	Meta Meta   `json:"meta"`

	// Rest is every field this service does not read, kept so that it reaches
	// the history as it arrived.
	Rest map[string]json.RawMessage `json:"-"`

	// Raw is the exact text the update was read from, kept only for the one
	// update that may have to be put back on the queue: taking it off again
	// is done by value, so the value has to be the one that was written.
	Raw string `json:"-"`
}

// knownUpdateFields are the ones the struct above covers. Anything else goes
// into Rest.
var knownUpdateFields = map[string]bool{
	"doc": true, "pathname": true, "op": true, "v": true, "meta": true,
}

// UnmarshalJSON reads an update, keeping whatever it does not model.
func (u *Update) UnmarshalJSON(data []byte) error {
	type fields Update
	var decoded fields
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*u = Update(decoded)

	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return err
	}
	for key, value := range all {
		if !knownUpdateFields[key] {
			if u.Rest == nil {
				u.Rest = map[string]json.RawMessage{}
			}
			u.Rest[key] = value
		}
	}
	return nil
}

// MarshalJSON writes the update, putting back whatever was not modelled.
func (u *Update) MarshalJSON() ([]byte, error) {
	type fields Update
	encoded, err := json.Marshal(fields(*u))
	if err != nil {
		return nil, err
	}
	if len(u.Rest) == 0 {
		return encoded, nil
	}

	var all map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &all); err != nil {
		return nil, err
	}
	for key, value := range u.Rest {
		all[key] = value
	}
	return json.Marshal(all)
}

// Timestamp is when an update was made, in milliseconds, or zero when it does
// not say.
func (m *Meta) Timestamp() int64 {
	if m == nil || m.TS == nil {
		return 0
	}
	return m.TS.Value
}

// IsTextUpdate reports whether the update edits a document.
//
// An update with an empty list of operations is still one: something sent it,
// it changed nothing, and it is dropped further along rather than refused
// here.
func (u *Update) IsTextUpdate() bool {
	return u.Doc != "" && u.Op != nil && u.Meta.Pathname != "" &&
		u.Meta.DocLength != nil
}

// IsTracked reports whether the update was made with track changes on.
func (u *Update) IsTracked() bool { return u.Meta.TC != "" }

// clone copies an update so that it can be changed without touching the
// original. The metadata is copied too, because that is what gets changed.
func (u *Update) clone() *Update {
	copied := *u
	copied.Op = append([]Op(nil), u.Op...)
	if u.Rest != nil {
		copied.Rest = make(map[string]json.RawMessage, len(u.Rest))
		for key, value := range u.Rest {
			copied.Rest[key] = value
		}
	}
	return &copied
}

// cloneWithOp copies an update and gives it a different operation.
func (u *Update) cloneWithOp(ops ...Op) *Update {
	copied := u.clone()
	copied.Op = ops
	return copied
}

// utf16Len is the length of a string in UTF-16 code units, which is what every
// position in an update is counted in.
func utf16Len(text string) int {
	var length int
	for _, r := range text {
		length++
		if r > 0xFFFF {
			length++
		}
	}
	return length
}

// minInt is the smaller of two numbers.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
