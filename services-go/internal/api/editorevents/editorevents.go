// Package editorevents is how this service tells the editors of a project
// that something happened to it.
//
// It publishes on the same Redis channel real-time subscribes to, in the same
// envelope, because real-time is the only thing holding the browser
// connections and this service is the only thing that knows a chat message was
// stored. Going through Redis rather than calling real-time directly is what
// makes it work when there is more than one real-time process: the recipients
// are usually connected to a different one from whichever handled the request.
//
// What may be sent is deliberately not decided here. real-time keeps a pass
// list of the events a link-share visitor may receive, and a message not on it
// reaches members only. That check belongs on the delivering side, where it
// cannot be forgotten by a new publisher.
package editorevents

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"

	"github.com/redis/go-redis/v9"
)

// envelope is what real-time reads off the editor-events channel.
//
// Payload is the argument list of the emit, so a message with one object
// argument carries a one-element array. The id is unique per message, which is
// how a duplicate delivery is recognised.
type envelope struct {
	RoomID  string `json:"room_id"`
	Message string `json:"message"`
	Payload []any  `json:"payload"`
	ID      string `json:"_id"`
}

// Publisher sends events to the editors of a project.
type Publisher struct {
	pubsub *redis.Client
	log    *slog.Logger

	// individualChannels sends each project's events on their own channel
	// instead of one shared one. Both services must be configured the same way
	// or the messages go somewhere nobody is listening.
	individualChannels bool

	host    string
	random  string
	counter atomic.Uint64
}

// New builds a publisher. A nil client yields a publisher that does nothing,
// so a deployment without Redis still serves every request that does not
// depend on somebody else being told.
func New(pubsub *redis.Client, individualChannels bool, log *slog.Logger) *Publisher {
	host, _ := os.Hostname()
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	return &Publisher{
		pubsub:             pubsub,
		log:                log,
		individualChannels: individualChannels,
		host:               host,
		random:             hex.EncodeToString(buf),
	}
}

// Emit publishes one event to a project's room.
//
// Failures are logged and swallowed. The thing that happened has already
// happened -- the message is stored, the file is renamed -- and refusing the
// request because the notification did not go out would turn a stale panel
// into a lost edit.
func (p *Publisher) Emit(ctx context.Context, projectID, message string, payload ...any) {
	if p == nil || p.pubsub == nil || projectID == "" {
		return
	}
	if payload == nil {
		payload = []any{}
	}

	blob, err := json.Marshal(envelope{
		RoomID:  projectID,
		Message: message,
		Payload: payload,
		ID:      p.messageID(),
	})
	if err != nil {
		p.log.Error("dropping unencodable editor event",
			slog.String("message", message), slog.String("project", projectID))
		return
	}

	channel := "editor-events"
	if p.individualChannels {
		channel = "editor-events:" + projectID
	}
	if err := p.pubsub.Publish(ctx, channel, string(blob)).Err(); err != nil {
		p.log.Warn("failed to publish an editor event",
			slog.String("message", message), slog.String("project", projectID),
			slog.String("err", err.Error()))
	}
}

func (p *Publisher) messageID() string {
	return fmt.Sprintf("web:%s:%s-%d", p.host, p.random, p.counter.Add(1)-1)
}
