package oid

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"507f1f77bcf86cd799439011", true},   // 24 hex
		{"507F1F77BCF86CD799439011", true},   // 24 hex, upper case
		{"mock-user-id", true},               // 12 ASCII chars, as Node accepts
		{"malformed-user", false},            // 14 chars
		{"malformed-project", false},         // 17 chars
		{"malformed-thread-id", false},       // 19 chars
		{"", false},                          // empty
		{"507f1f77bcf86cd79943901", false},   // 23 chars
		{"507f1f77bcf86cd7994390111", false}, // 25 chars
		{"507f1f77bcf86cd79943901z", false},  // 24 chars, not hex
		{"héllo wörld!", false},              // 12 runes but >12 UTF-8 bytes
	}
	for _, c := range cases {
		if got := IsValid(c.in); got != c.want {
			t.Errorf("IsValid(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseHexRoundTrip(t *testing.T) {
	const in = "507f1f77bcf86cd799439011"
	id, ok := Parse(in)
	if !ok {
		t.Fatalf("Parse(%q) failed", in)
	}
	if got := id.Hex(); got != in {
		t.Errorf("round trip = %q, want %q", got, in)
	}
}
