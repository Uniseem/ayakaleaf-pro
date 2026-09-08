// Package histmodel is the data model the history is written in.
//
// It is the Go port of the parts of libraries/overleaf-editor-core that
// project-history uses. history-v1 stays on Node and reads the same model, so
// the two meet as JSON over HTTP: what matters here is that this side produces
// and understands exactly the same JSON.
//
// The text operations in this package are not the ones in internal/textot.
// That is the ShareJS type the editor and document-updater speak, expressed as
// inserts and deletes at positions. This one is the history type, expressed as
// a scan over the whole document -- retain so much, insert this, remove that --
// with the tracked changes and comments carried along in the scan.
package histmodel

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// TrackingDirective says what a piece of a document is marked as.
//
// It is either a tracked change -- an insertion or a deletion somebody made
// with track changes on -- or an instruction to clear whatever mark was there.
type TrackingDirective interface {
	// Type is "insert", "delete" or "none".
	Type() string
	// Equals reports whether two directives say the same thing.
	Equals(other TrackingDirective) bool
	// CanMergeWith reports whether two adjacent runs carrying these
	// directives can be written as one.
	CanMergeWith(other TrackingDirective) bool
	// MergeWith returns the directive for the merged run.
	MergeWith(other TrackingDirective) TrackingDirective
	// Raw is the JSON form.
	Raw() json.RawMessage
}

// TrackingProps marks a run of text as somebody's tracked change.
type TrackingProps struct {
	// Kind is "insert" or "delete".
	Kind   string
	UserID string
	TS     time.Time
}

// Type reports the kind of change.
func (t *TrackingProps) Type() string { return t.Kind }

// Equals reports whether two marks are the same mark.
func (t *TrackingProps) Equals(other TrackingDirective) bool {
	o, ok := other.(*TrackingProps)
	if !ok {
		return false
	}
	return t.Kind == o.Kind && t.UserID == o.UserID && t.TS.Equal(o.TS)
}

// CanMergeWith reports whether two runs marked this way can be written as one.
//
// The timestamps need not agree: two keystrokes by the same person a second
// apart are one tracked change, and the earlier time is the one kept.
func (t *TrackingProps) CanMergeWith(other TrackingDirective) bool {
	o, ok := other.(*TrackingProps)
	if !ok {
		return false
	}
	return t.Kind == o.Kind && t.UserID == o.UserID
}

// MergeWith returns the mark for the merged run, which carries the earlier of
// the two times.
func (t *TrackingProps) MergeWith(other TrackingDirective) TrackingDirective {
	o, ok := other.(*TrackingProps)
	if !ok {
		return t
	}
	ts := t.TS
	if o.TS.Before(ts) {
		ts = o.TS
	}
	return &TrackingProps{Kind: t.Kind, UserID: t.UserID, TS: ts}
}

// Raw is the JSON form.
func (t *TrackingProps) Raw() json.RawMessage {
	encoded, _ := json.Marshal(map[string]string{
		"type": t.Kind, "userId": t.UserID, "ts": formatTimestamp(t.TS),
	})
	return encoded
}

// ClearTracking is the instruction to take a mark off a run of text, which is
// what accepting a tracked change comes to.
type ClearTracking struct{}

// Type reports "none".
func (ClearTracking) Type() string { return "none" }

// Equals reports whether the other directive also clears.
func (ClearTracking) Equals(other TrackingDirective) bool {
	_, ok := other.(ClearTracking)
	return ok
}

// CanMergeWith reports whether the other directive also clears.
func (ClearTracking) CanMergeWith(other TrackingDirective) bool {
	_, ok := other.(ClearTracking)
	return ok
}

// MergeWith returns the same instruction.
func (c ClearTracking) MergeWith(TrackingDirective) TrackingDirective { return c }

// Raw is the JSON form.
func (ClearTracking) Raw() json.RawMessage { return json.RawMessage(`{"type":"none"}`) }

// ErrInvalidTracking is returned for a tracking directive that names no kind.
var ErrInvalidTracking = errors.New("invalid tracking directive")

// trackingFromRaw reads a tracking directive.
//
// allowClear says whether "none" is meaningful here. On a retain it is: it
// takes the mark off the text being kept. On an insert it is not -- there is no
// mark on text that is only now being written -- and the reference
// implementation reads it as an ordinary mark, which leaves it with no
// timestamp and fails when it comes to be written back. So an insert marked
// that way is refused rather than stored and found unreadable later.
func trackingFromRaw(raw json.RawMessage, allowClear bool) (TrackingDirective, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var fields struct {
		Type   string `json:"type"`
		UserID string `json:"userId"`
		TS     string `json:"ts"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidTracking, err)
	}
	if fields.Type == "none" && allowClear {
		return ClearTracking{}, nil
	}
	ts, err := parseTimestamp(fields.TS)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidTracking, err)
	}
	return &TrackingProps{Kind: fields.Type, UserID: fields.UserID, TS: ts}, nil
}

// trackingEquals compares two directives either of which may be absent.
func trackingEquals(a, b TrackingDirective) bool {
	if a == nil {
		return b == nil
	}
	if b == nil {
		return false
	}
	return a.Equals(b)
}

// parseTimestamp reads a time as the history writes it.
//
// It is an ISO string in the JSON, but a number of milliseconds is accepted
// too: a Date built from one is the same Date, and some of what reaches this
// comes from the editor rather than from the history.
func parseTimestamp(value string) (time.Time, error) {
	if value == "" {
		// The reference implementation makes a Date from it and writes it back
		// as an ISO string, which fails on a missing one.
		return time.Time{}, errors.New("missing timestamp")
	}
	if ts, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return ts.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("unreadable timestamp %q", value)
}

// formatTimestamp renders a time the way JSON.stringify renders a Date.
func formatTimestamp(ts time.Time) string {
	return ts.UTC().Format("2006-01-02T15:04:05.000Z")
}
