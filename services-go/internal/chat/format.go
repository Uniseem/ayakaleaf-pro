package chat

import (
	"sort"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ClientMessage is the message shape the chat API returns.
//
// Field order matches MessageFormatter.formatMessageForClientSide followed by
// the room_id the send handlers attach, so responses are byte-comparable with
// the Node service during a shadow-traffic comparison.
type ClientMessage struct {
	ID        string  `json:"id"`
	Content   string  `json:"content"`
	Timestamp Millis  `json:"timestamp"`
	UserID    string  `json:"user_id"`
	EditedAt  *Millis `json:"edited_at,omitempty"`
	RoomID    string  `json:"room_id,omitempty"`
}

// ClientThread is one entry of the thread map returned by the threads
// endpoints.
type ClientThread struct {
	Messages         []ClientMessage `json:"messages"`
	Resolved         bool            `json:"resolved,omitempty"`
	ResolvedAt       *JSDate         `json:"resolved_at,omitempty"`
	ResolvedByUserID string          `json:"resolved_by_user_id,omitempty"`
}

// formatMessage renders a stored message for the client.
func formatMessage(m Message) ClientMessage {
	return ClientMessage{
		ID:        m.ID.Hex(),
		Content:   m.Content,
		Timestamp: m.Timestamp,
		UserID:    m.UserID.Hex(),
		EditedAt:  m.EditedAt,
	}
}

func formatMessages(messages []Message) []ClientMessage {
	out := make([]ClientMessage, 0, len(messages))
	for _, m := range messages {
		out = append(out, formatMessage(m))
	}
	return out
}

// groupMessagesByThreads buckets messages into their threads.
//
// A thread only appears once it has at least one message, matching the Node
// implementation, where the thread object is created lazily while walking the
// messages. Callers rely on that: deleting a thread's last message removes it
// from the listing.
func groupMessagesByThreads(rooms []Room, messages []Message) map[string]*ClientThread {
	roomsByID := make(map[string]Room, len(rooms))
	for _, r := range rooms {
		roomsByID[r.ID.Hex()] = r
	}

	threads := map[string]*ClientThread{}
	for _, msg := range messages {
		room, ok := roomsByID[msg.RoomID.Hex()]
		if !ok || room.ThreadID == nil {
			continue
		}
		threadID := room.ThreadID.Hex()
		thread, ok := threads[threadID]
		if !ok {
			thread = &ClientThread{Messages: []ClientMessage{}}
			if room.Resolved != nil {
				resolvedAt := JSDate{room.Resolved.TS}
				thread.Resolved = true
				thread.ResolvedAt = &resolvedAt
				thread.ResolvedByUserID = room.Resolved.UserID
			}
			threads[threadID] = thread
		}
		thread.Messages = append(thread.Messages, formatMessage(msg))
	}

	for _, thread := range threads {
		// Array.prototype.sort is stable, so equal timestamps keep their
		// relative order; SliceStable preserves that.
		msgs := thread.Messages
		sort.SliceStable(msgs, func(i, j int) bool {
			return msgs[i].Timestamp < msgs[j].Timestamp
		})
	}
	return threads
}

// roomIDs projects the room ids out of a slice of rooms.
func roomIDs(rooms []Room) []bson.ObjectID {
	ids := make([]bson.ObjectID, 0, len(rooms))
	for _, r := range rooms {
		ids = append(ids, r.ID)
	}
	return ids
}
