package socketio

import (
	"encoding/json"
	"reflect"
	"testing"
)

// The framing is the one thing that cannot be approximated: the client on the
// other side is a fork of socket.io 0.9 and will silently ignore anything it
// does not recognise.
func TestDecodeFrames(t *testing.T) {
	cases := []struct {
		frame string
		want  Packet
	}{
		{"0::", Packet{Type: PacketDisconnect}},
		{"1::", Packet{Type: PacketConnect}},
		{"2::", Packet{Type: PacketHeartbeat}},
		{"3:::hello", Packet{Type: PacketMessage, Data: "hello"}},
		{`5:::{"name":"joinDoc","args":["doc-1",0]}`, Packet{
			Type: PacketEvent, Data: `{"name":"joinDoc","args":["doc-1",0]}`,
			Name: "joinDoc",
			Args: []json.RawMessage{json.RawMessage(`"doc-1"`), json.RawMessage(`0`)},
		}},
		// An id means the client wants an acknowledgement; "+" means it wants
		// to send one itself rather than have the server ack automatically.
		{"5:7+::{\"name\":\"x\"}", Packet{
			Type: PacketEvent, ID: "7", Ack: true, Data: `{"name":"x"}`, Name: "x",
		}},
		{"1::/chat", Packet{Type: PacketConnect, Endpoint: "/chat"}},
	}
	for _, c := range cases {
		got, err := Decode(c.frame)
		if err != nil {
			t.Errorf("Decode(%q) errored: %v", c.frame, err)
			continue
		}
		if got.Type != c.want.Type || got.ID != c.want.ID || got.Ack != c.want.Ack ||
			got.Endpoint != c.want.Endpoint || got.Data != c.want.Data || got.Name != c.want.Name {
			t.Errorf("Decode(%q) = %+v, want %+v", c.frame, got, c.want)
		}
		if c.want.Args != nil && !reflect.DeepEqual(got.Args, c.want.Args) {
			t.Errorf("Decode(%q) args = %v, want %v", c.frame, got.Args, c.want.Args)
		}
	}

	for _, bad := range []string{"", "nonsense", "9::"} {
		if _, err := Decode(bad); err == nil {
			t.Errorf("Decode(%q) should have failed", bad)
		}
	}
}

func TestEncodeFrames(t *testing.T) {
	cases := map[string]Packet{
		"1::":      {Type: PacketConnect},
		"2::":      {Type: PacketHeartbeat},
		"8::":      {Type: PacketNoop},
		"3:::hi":   {Type: PacketMessage, Data: "hi"},
		"7:::2":    {Type: PacketError, Data: "2"},
		"1::/chat": {Type: PacketConnect, Endpoint: "/chat"},
	}
	for want, p := range cases {
		if got := Encode(p); got != want {
			t.Errorf("Encode(%+v) = %q, want %q", p, got, want)
		}
	}

	frame, err := EncodeEvent("joinDocResponse", "doc-1", []string{"a", "b"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	const want = `5:::{"args":["doc-1",["a","b"],3],"name":"joinDocResponse"}`
	if frame != want {
		t.Errorf("EncodeEvent = %q, want %q", frame, want)
	}
	// Whatever the key order, it has to decode back to the same event.
	back, err := Decode(frame)
	if err != nil || back.Name != "joinDocResponse" || len(back.Args) != 3 {
		t.Errorf("round trip = %+v, %v", back, err)
	}
}

func TestHandshake(t *testing.T) {
	if got, want := Handshake("abc", 30, 60, []string{"websocket", "xhr-polling"}),
		"abc:30:60:websocket,xhr-polling"; got != want {
		t.Errorf("Handshake = %q, want %q", got, want)
	}
	// An empty heartbeat field disables heartbeats client-side.
	if got, want := Handshake("abc", 0, 60, []string{"websocket"}), "abc::60:websocket"; got != want {
		t.Errorf("Handshake without heartbeats = %q, want %q", got, want)
	}
}

// The polling payload declares each frame's length in UTF-16 code units,
// because the decoder is JavaScript slicing a string. Counting bytes or runes
// instead would misplace every frame after the first non-ASCII one.
func TestPayloadRoundTrip(t *testing.T) {
	single := []string{"1::"}
	if got := EncodePayload(single); got != "1::" {
		t.Errorf("a single frame should be sent bare, got %q", got)
	}

	frames := []string{"1::", `5:::{"name":"a"}`, "2::"}
	encoded := EncodePayload(frames)
	if got := DecodePayload(encoded); !reflect.DeepEqual(got, frames) {
		t.Errorf("round trip = %v, want %v", got, frames)
	}

	// Astral characters take two UTF-16 units each.
	wide := []string{`3:::café \U0001F600`, "2::"}
	wide[0] = "3:::café 😀"
	if got := DecodePayload(EncodePayload(wide)); !reflect.DeepEqual(got, wide) {
		t.Errorf("wide round trip = %q, want %q", got, wide)
	}

	if got := DecodePayload("1::"); !reflect.DeepEqual(got, []string{"1::"}) {
		t.Errorf("a bare frame should decode to itself, got %v", got)
	}
	if got := DecodePayload(""); got != nil {
		t.Errorf("an empty payload should decode to nothing, got %v", got)
	}
}

// Checked against the framing lib/parser.js produces, so the exact bytes are
// pinned rather than only the round trip.
func TestPayloadWireFormat(t *testing.T) {
	got := EncodePayload([]string{"1::", "2::"})
	const want = "�3�1::�3�2::"
	if got != want {
		t.Errorf("EncodePayload = %q, want %q", got, want)
	}
}
