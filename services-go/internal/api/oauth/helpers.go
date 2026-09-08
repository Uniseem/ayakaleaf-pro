package oauth

import (
	"encoding/json"
	"net/url"
)

// jsonRaw is what a session keeps for the fields it does not model. The OAuth
// state lives there because it belongs to one sign-in attempt, not to the
// session's shape.
type jsonRaw = json.RawMessage

// jsonString writes a value into that map.
func jsonString(value string) jsonRaw {
	encoded, err := json.Marshal(value)
	if err != nil {
		// A string always marshals; this is only here so the caller does not
		// have to handle an error that cannot happen.
		return jsonRaw(`""`)
	}
	return jsonRaw(encoded)
}

// readString reads one back, answering with the empty string for anything that
// is not there or is not a string.
func readString(extra map[string]jsonRaw, key string) string {
	raw, present := extra[key]
	if !present {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

// urlEncode escapes a message for a query string.
func urlEncode(value string) string {
	return url.QueryEscape(value)
}
