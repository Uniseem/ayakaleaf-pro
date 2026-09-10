package editorevents

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

// The envelope is a contract with real-time, which reads these exact field
// names off the channel. A rename here is a message that parses into an empty
// struct there and is silently dropped, so the shape is asserted rather than
// assumed.
func TestEnvelopeShape(t *testing.T) {
	blob, err := json.Marshal(envelope{
		RoomID:  "6aa14410a5e8e57e9c4f07ef",
		Message: "new-chat-message",
		Payload: []any{map[string]string{"id": "m1", "content": "hello"}},
		ID:      "web:host:abcd-0",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Decoded into the shape real-time declares for it.
	var read struct {
		RoomID  string            `json:"room_id"`
		Message string            `json:"message"`
		Payload []json.RawMessage `json:"payload"`
		ID      string            `json:"_id"`
	}
	if err := json.Unmarshal(blob, &read); err != nil {
		t.Fatalf("real-time could not read the envelope: %v", err)
	}

	if read.RoomID != "6aa14410a5e8e57e9c4f07ef" {
		t.Errorf("room_id = %q", read.RoomID)
	}
	if read.Message != "new-chat-message" {
		t.Errorf("message = %q", read.Message)
	}
	if read.ID == "" {
		t.Error("_id is empty, so a duplicate delivery cannot be recognised")
	}
	// The payload is the argument list of an emit, so one object argument is a
	// one-element array. An object here would reach the browser as no
	// arguments at all.
	if len(read.Payload) != 1 {
		t.Fatalf("payload has %d arguments, want 1", len(read.Payload))
	}
	var arg map[string]string
	if err := json.Unmarshal(read.Payload[0], &arg); err != nil {
		t.Fatalf("the argument is not the object that was sent: %v", err)
	}
	if arg["content"] != "hello" {
		t.Errorf("argument content = %q", arg["content"])
	}
}

// A message with no arguments still has to carry an empty list, because
// real-time reads the payload as a list and a null yields nothing to forward.
func TestEmptyPayloadIsAList(t *testing.T) {
	blob, err := json.Marshal(envelope{
		RoomID:  "p1",
		Message: "projectNameUpdated",
		Payload: []any{},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := string(blob); !json.Valid(blob) || !contains(got, `"payload":[]`) {
		t.Errorf("payload is not an empty list: %s", got)
	}
}

// Ids are unique per message, or two different events look like one delivered
// twice.
func TestMessageIDsDiffer(t *testing.T) {
	p := New(nil, false, slog.Default())
	first := p.messageID()
	second := p.messageID()
	if first == second {
		t.Errorf("two messages share the id %q", first)
	}
}

// Without Redis, and without a project, publishing is a no-op rather than a
// panic: the thing being announced has already happened.
func TestEmitWithoutRedisIsSafe(t *testing.T) {
	New(nil, false, slog.Default()).Emit(context.Background(), "p1", "new-chat-message", 1)

	var absent *Publisher
	absent.Emit(context.Background(), "p1", "new-chat-message")
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
