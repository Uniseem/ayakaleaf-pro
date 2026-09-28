package realtime

import (
	"encoding/json"
	"strings"
)

// encodeForWebsockets is the Go equivalent of the Node service's
//
//	unescape(encodeURIComponent(text))
//
// It rewrites a string so that every byte of its UTF-8 encoding becomes one
// code point in the range U+0000..U+00FF. The editor reverses it with
// decodeURIComponent(escape(text)).
//
// The reason is the socket.io 0.9 framing: a polling payload declares frame
// lengths in UTF-16 code units, and any character outside the BMP is two of
// them. Sending text where one character is one code unit is what keeps those
// lengths in agreement across the wire.
func encodeForWebsockets(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, c := range []byte(s) {
		b.WriteRune(rune(c))
	}
	return b.String()
}

// encodeLinesForWebsockets applies the encoding to each line of a document.
func encodeLinesForWebsockets(lines []json.RawMessage) ([]json.RawMessage, error) {
	encoded := make([]json.RawMessage, 0, len(lines))
	for _, raw := range lines {
		var line string
		if err := json.Unmarshal(raw, &line); err != nil {
			// A line that is not a string belongs to a document type that is
			// passed through unchanged.
			encoded = append(encoded, raw)
			continue
		}
		out, err := json.Marshal(encodeForWebsockets(line))
		if err != nil {
			return nil, err
		}
		encoded = append(encoded, out)
	}
	return encoded, nil
}

// stripComments empties the comments of a ranges object, which is how a
// restricted user is kept from seeing them.
func stripComments(ranges json.RawMessage) json.RawMessage {
	decoded := decodeRanges(ranges)
	if decoded == nil {
		return ranges
	}
	decoded["comments"] = []any{}
	if out, err := json.Marshal(decoded); err == nil {
		return out
	}
	return ranges
}

// encodeRangesForWebsockets applies the same encoding to the text carried
// inside comments and tracked changes, which the editor decodes the same way.
func encodeRangesForWebsockets(ranges json.RawMessage) json.RawMessage {
	decoded := decodeRanges(ranges)
	if decoded == nil {
		return ranges
	}
	encodeOpFields(decoded["comments"], "c")
	encodeOpFields(decoded["changes"], "i", "d")
	if out, err := json.Marshal(decoded); err == nil {
		return out
	}
	return ranges
}

// encodeOpFields encodes the named string fields of each entry's op.
func encodeOpFields(entries any, fields ...string) {
	list, ok := entries.([]any)
	if !ok {
		return
	}
	for _, entry := range list {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		op, ok := item["op"].(map[string]any)
		if !ok {
			continue
		}
		for _, field := range fields {
			if text, ok := op[field].(string); ok {
				op[field] = encodeForWebsockets(text)
			}
		}
	}
}

// decodeRanges parses a ranges object, returning nil when there is nothing to
// change.
func decodeRanges(ranges json.RawMessage) map[string]any {
	if len(ranges) == 0 || string(ranges) == "null" {
		return nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(ranges, &decoded); err != nil {
		return nil
	}
	return decoded
}
