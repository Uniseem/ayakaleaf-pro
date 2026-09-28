package docupdater

import (
	"encoding/json"
	"sort"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/rangestracker"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/textot"
)

// History records a document with its tracked deletions still in the text,
// where the editor shows it with them taken out. So every position the editor
// works in is short by the length of the tracked deletions before it, and a
// marker sent to history carries a second position measured in the longer text.
//
// This is that conversion. It is what makes a comment come back attached to the
// same words when a version is restored, rather than to whatever now sits at
// the position it was stored under.

// addTrackedDeletesToContent puts the tracked deletions back into the text, in
// the order the changes give them.
func addTrackedDeletesToContent(content textot.Text, changes []*rangestracker.Change) textot.Text {
	var cursor int
	result := textot.Text{}
	for _, change := range changes {
		if change.Op.Kind != textot.Delete {
			continue
		}
		result = textot.Concat(result, content.Slice(cursor, change.Op.Pos))
		cursor = change.Op.Pos
		result = textot.Concat(result, change.Op.Text)
	}
	return textot.Concat(result, content.From(cursor))
}

// historyRanges is the markers as history stores them.
type historyRanges struct {
	Changes  []json.RawMessage `json:"changes,omitempty"`
	Comments []json.RawMessage `json:"comments,omitempty"`
}

// toHistoryRanges converts the markers from the positions the editor works in
// to the positions the history stores.
//
// A marker that sits after a tracked deletion gains an "hpos", its position in
// the longer text. A comment that spans one gains an "hlen" as well, because
// the text it covers is longer there than what the editor shows.
func toHistoryRanges(changes []*rangestracker.Change, comments []*rangestracker.Comment) historyRanges {
	// The changes are already in order; the comments are not.
	sorted := append([]*rangestracker.Comment(nil), comments...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Op.Pos < sorted[j].Op.Pos
	})

	var out historyRanges
	// offset is how far ahead of the editor the history is by this point.
	var offset int
	next := 0
	// pending are the comments that have started but may still turn out to
	// span a tracked deletion further on.
	var pending []map[string]json.RawMessage

	for _, change := range changes {
		out.Changes = append(out.Changes, withHistoryPosition(change.Op.Pos, offset, change))

		// Only a tracked deletion moves anything.
		if change.Op.Kind != textot.Delete {
			continue
		}

		// Everything starting before this deletion has to be considered now.
		for next < len(sorted) && sorted[next].Op.Pos < change.Op.Pos {
			pending = append(pending, decodeMarker(
				withHistoryPosition(sorted[next].Op.Pos, offset, sorted[next])))
			next++
		}

		// A comment that ends before the deletion cannot be affected by it.
		var stillPending []map[string]json.RawMessage
		for _, comment := range pending {
			if commentEnd(comment) <= change.Op.Pos {
				out.Comments = append(out.Comments, encodeMarker(comment))
				continue
			}
			stillPending = append(stillPending, comment)
		}
		pending = stillPending

		// What is left spans the deletion, so in the history it covers the
		// deleted text as well.
		for _, comment := range pending {
			setCommentHistoryLength(comment, change.Op.Len())
		}

		offset += change.Op.Len()
	}

	for _, comment := range pending {
		out.Comments = append(out.Comments, encodeMarker(comment))
	}
	// Anything after the last tracked change only moves by the offset.
	for ; next < len(sorted); next++ {
		out.Comments = append(out.Comments,
			withHistoryPosition(sorted[next].Op.Pos, offset, sorted[next]))
	}
	return out
}

// withHistoryPosition renders a marker with its position in the history text
// added, leaving everything else as it was.
func withHistoryPosition(position, offset int, marker any) json.RawMessage {
	encoded, err := json.Marshal(marker)
	if err != nil {
		return nil
	}
	if offset == 0 {
		// The two texts agree up to here, so there is nothing to add.
		return encoded
	}

	decoded := decodeMarker(encoded)
	op, ok := decoded["op"]
	if !ok {
		return encoded
	}
	var opFields map[string]json.RawMessage
	if err := json.Unmarshal(op, &opFields); err != nil {
		return encoded
	}
	opFields["hpos"], _ = json.Marshal(position + offset)
	decoded["op"], _ = json.Marshal(opFields)
	return encodeMarker(decoded)
}

// commentEnd is where a comment stops, in editor positions.
func commentEnd(marker map[string]json.RawMessage) int {
	var op struct {
		P int    `json:"p"`
		C string `json:"c"`
	}
	if err := json.Unmarshal(marker["op"], &op); err != nil {
		return 0
	}
	return op.P + textot.T(op.C).Len()
}

// setCommentHistoryLength grows the length a comment covers in the history text
// by the length of a tracked deletion it spans.
func setCommentHistoryLength(marker map[string]json.RawMessage, by int) {
	var opFields map[string]json.RawMessage
	if err := json.Unmarshal(marker["op"], &opFields); err != nil {
		return
	}

	length := 0
	if raw, ok := opFields["hlen"]; ok {
		_ = json.Unmarshal(raw, &length)
	} else {
		var text string
		if err := json.Unmarshal(opFields["c"], &text); err == nil {
			length = textot.T(text).Len()
		}
	}
	opFields["hlen"], _ = json.Marshal(length + by)
	marker["op"], _ = json.Marshal(opFields)
}

func decodeMarker(raw json.RawMessage) map[string]json.RawMessage {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return map[string]json.RawMessage{}
	}
	return fields
}

func encodeMarker(fields map[string]json.RawMessage) json.RawMessage {
	encoded, err := json.Marshal(fields)
	if err != nil {
		return nil
	}
	return encoded
}
