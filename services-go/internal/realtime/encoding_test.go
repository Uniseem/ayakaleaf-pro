package realtime

import (
	"encoding/json"
	"testing"
)

// The expected values come from running the Node expression this replaces:
//
//	unescape(encodeURIComponent(text))
//
// and reading off the code units. Pinning them here is the only way to catch a
// change that would silently mangle every accented character in every document.
func TestEncodeForWebsockets(t *testing.T) {
	cases := []struct {
		in   string
		want []rune
	}{
		{"hello", []rune{104, 101, 108, 108, 111}},
		{"héllo", []rune{104, 195, 169, 108, 108, 111}},
		{"日本語", []rune{230, 151, 165, 230, 156, 172, 232, 170, 158}},
		{"€", []rune{226, 130, 172}},
		// Outside the BMP: four UTF-8 bytes become four code units, which is
		// exactly why this encoding exists -- the frame length the polling
		// transport declares is counted in UTF-16 units.
		{"\U0001F600", []rune{240, 159, 152, 128}},
		{"", nil},
	}
	for _, tc := range cases {
		got := []rune(encodeForWebsockets(tc.in))
		if len(got) != len(tc.want) {
			t.Errorf("encodeForWebsockets(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("encodeForWebsockets(%q) = %v, want %v", tc.in, got, tc.want)
				break
			}
		}
	}
}

func TestEncodeLinesForWebsockets(t *testing.T) {
	lines := []json.RawMessage{
		json.RawMessage(`"\\documentclass{article}"`),
		json.RawMessage(`"café"`),
	}
	encoded, err := encodeLinesForWebsockets(lines)
	if err != nil {
		t.Fatalf("encodeLinesForWebsockets: %v", err)
	}

	var backslashLine, accentLine string
	if err := json.Unmarshal(encoded[0], &backslashLine); err != nil {
		t.Fatalf("decoding line: %v", err)
	}
	if backslashLine != `\documentclass{article}` {
		t.Errorf("an ASCII line must survive unchanged, got %q", backslashLine)
	}
	if err := json.Unmarshal(encoded[1], &accentLine); err != nil {
		t.Fatalf("decoding line: %v", err)
	}
	if accentLine != "cafÃ©" {
		t.Errorf("accented line = %q, want the two-code-unit form", accentLine)
	}
}

func TestStripCommentsLeavesChangesAlone(t *testing.T) {
	ranges := json.RawMessage(`{"comments":[{"id":"c1"}],"changes":[{"id":"ch1"}]}`)
	var got map[string]any
	if err := json.Unmarshal(stripComments(ranges), &got); err != nil {
		t.Fatalf("stripComments produced invalid JSON: %v", err)
	}
	if comments, ok := got["comments"].([]any); !ok || len(comments) != 0 {
		t.Errorf("comments = %v, want an empty list", got["comments"])
	}
	if changes, ok := got["changes"].([]any); !ok || len(changes) != 1 {
		t.Errorf("changes = %v, want the original single change", got["changes"])
	}
}

func TestStripCommentsOnMissingRanges(t *testing.T) {
	for _, ranges := range []json.RawMessage{nil, json.RawMessage("null"), json.RawMessage(`"nope"`)} {
		if got := stripComments(ranges); string(got) != string(ranges) {
			t.Errorf("stripComments(%s) = %s, want it unchanged", ranges, got)
		}
	}
}

func TestEncodeRangesForWebsockets(t *testing.T) {
	ranges := json.RawMessage(`{
		"comments":[{"id":"c1","op":{"c":"café","p":1}}],
		"changes":[{"id":"ch1","op":{"i":"€","p":2}},{"id":"ch2","op":{"d":"x","p":3}}]
	}`)
	var got struct {
		Comments []struct {
			Op map[string]any `json:"op"`
		} `json:"comments"`
		Changes []struct {
			Op map[string]any `json:"op"`
		} `json:"changes"`
	}
	if err := json.Unmarshal(encodeRangesForWebsockets(ranges), &got); err != nil {
		t.Fatalf("encodeRangesForWebsockets produced invalid JSON: %v", err)
	}
	if got.Comments[0].Op["c"] != "cafÃ©" {
		t.Errorf("comment text = %q, want it encoded", got.Comments[0].Op["c"])
	}
	if got.Changes[0].Op["i"] != "â¬" {
		t.Errorf("insert text = %q, want it encoded", got.Changes[0].Op["i"])
	}
	if got.Changes[1].Op["d"] != "x" {
		t.Errorf("delete text = %q, want it unchanged", got.Changes[1].Op["d"])
	}
	// Positions must not be touched: they are offsets into the document, and
	// shifting one corrupts the range.
	if got.Comments[0].Op["p"] != float64(1) {
		t.Errorf("position = %v, want 1", got.Comments[0].Op["p"])
	}
}
