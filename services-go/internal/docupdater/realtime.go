package docupdater

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/rediskeys"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/textot"
	"github.com/redis/go-redis/v9"
)

// maxOpsPerIteration bounds how many queued updates are taken at once, so one
// document with a long backlog cannot hold the lock indefinitely.
const maxOpsPerIteration = 8

// RealTimeBridge is the connection to real-time: the queue of updates coming
// from editors, and the channel applied operations go back out on.
type RealTimeBridge struct {
	redis  *redis.Client
	pubsub *redis.Client
	keys   rediskeys.Schema
	log    *slog.Logger

	// publishOnIndividualChannels sends each document's operations on their own
	// channel instead of one shared one. Both services must be configured the
	// same way or the messages go somewhere nobody is listening.
	publishOnIndividualChannels bool

	host    string
	random  string
	counter atomic.Uint64
}

// NewRealTimeBridge builds the bridge.
func NewRealTimeBridge(client, pubsub *redis.Client, keys rediskeys.Schema,
	publishOnIndividualChannels bool, log *slog.Logger) *RealTimeBridge {
	host, _ := os.Hostname()
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	return &RealTimeBridge{
		redis: client, pubsub: pubsub, keys: keys, log: log,
		publishOnIndividualChannels: publishOnIndividualChannels,
		host:                        host, random: hex.EncodeToString(buf),
	}
}

// GetPendingUpdatesForDoc takes the next batch of updates off a document's
// queue.
//
// The read and the trim are one transaction, so an update cannot be handed out
// twice or dropped between the two.
func (b *RealTimeBridge) GetPendingUpdatesForDoc(ctx context.Context, docID string) ([]*Update, error) {
	key := b.keys.PendingUpdates(docID)

	pipe := b.redis.TxPipeline()
	pipe.LLen(ctx, key)
	batch := pipe.LRange(ctx, key, 0, maxOpsPerIteration-1)
	pipe.LTrim(ctx, key, maxOpsPerIteration, -1)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}

	encoded := batch.Val()
	updates := make([]*Update, 0, len(encoded))
	for _, raw := range encoded {
		var update Update
		if err := json.Unmarshal([]byte(raw), &update); err != nil {
			return nil, fmt.Errorf("malformed queued update: %w", err)
		}
		updates = append(updates, &update)
	}
	return updates, nil
}

// GetUpdatesLength reports how many updates are still queued, which is how the
// service decides whether to keep the lock and carry on.
func (b *RealTimeBridge) GetUpdatesLength(ctx context.Context, docID string) (int64, error) {
	return b.redis.LLen(ctx, b.keys.PendingUpdates(docID)).Result()
}

// appliedOpMessage is what real-time reads off the applied-ops channel.
type appliedOpMessage struct {
	ProjectID string          `json:"project_id,omitempty"`
	DocID     string          `json:"doc_id"`
	Op        json.RawMessage `json:"op,omitempty"`
	Error     string          `json:"error,omitempty"`
	ID        string          `json:"_id"`
}

// SendAppliedOp publishes an applied operation for real-time to fan out.
//
// This happens before the document is written back to Redis, matching the Node
// service: the model emits its event as soon as the operation is applied in
// memory, and the write follows.
func (b *RealTimeBridge) SendAppliedOp(ctx context.Context, projectID, docID string, op *Update) {
	encoded, err := json.Marshal(op)
	if err != nil {
		b.log.Error("dropping unencodable applied op", slog.String("doc", docID))
		return
	}
	b.send(ctx, docID, appliedOpMessage{
		ProjectID: projectID, DocID: docID, Op: encoded, ID: b.messageID(),
	})
}

// SendError tells the editors of a document that their update could not be
// applied. real-time disconnects them, and they reload.
func (b *RealTimeBridge) SendError(ctx context.Context, projectID, docID string, cause error) {
	b.send(ctx, docID, appliedOpMessage{
		ProjectID: projectID, DocID: docID, Error: errorForClient(cause), ID: b.messageID(),
	})
}

// errorForClient is what the editor is told went wrong.
//
// A delete that does not match is reported by a fixed phrase rather than by
// what was expected and what was found: the client only uses it to decide to
// reload, and the two texts are the document, which does not belong in a
// message every other editor of it also receives.
func errorForClient(cause error) string {
	if errors.Is(cause, textot.ErrDeleteMismatch) {
		return "Delete component does not match"
	}
	// The same for a document that is not there: which document it was is in
	// the request the client made, not something it learns from the answer.
	if errors.Is(cause, ErrNotFound) {
		return "doc not found"
	}
	return cause.Error()
}

func (b *RealTimeBridge) send(ctx context.Context, docID string, message appliedOpMessage) {
	blob, err := json.Marshal(message)
	if err != nil {
		b.log.Error("dropping unencodable message", slog.String("doc", docID))
		return
	}
	channel := "applied-ops"
	if b.publishOnIndividualChannels {
		channel = "applied-ops:" + docID
	}
	if err := b.pubsub.Publish(ctx, channel, string(blob)).Err(); err != nil {
		b.log.Error("failed to publish applied op", slog.String("doc", docID),
			slog.String("err", err.Error()))
	}
}

// SendCanaryAppliedOp publishes a measurement of how much traffic an operation
// will cause. real-time consumes it and forwards it to nobody.
func (b *RealTimeBridge) SendCanaryAppliedOp(ctx context.Context, projectID, docID string, op *Update) {
	ack, err := json.Marshal(map[string]any{"v": op.V, "doc": docID})
	if err != nil {
		return
	}
	broadcast := 0
	if !op.Dup {
		// A duplicate is only acknowledged to its sender, never broadcast.
		encoded, err := json.Marshal(op)
		if err != nil {
			return
		}
		broadcast = len(encoded)
	}

	payload, err := json.Marshal(map[string]any{
		"message": "canary-applied-op",
		"payload": map[string]any{
			"ack":       len(ack),
			"broadcast": broadcast,
			"docId":     docID,
			"projectId": projectID,
			"source":    metaString(op.Meta, "source"),
		},
	})
	if err != nil {
		return
	}

	// Published on the editor-events channel, which real-time subscribes to
	// before it finishes accepting a connection.
	channel := "editor-events"
	if b.publishOnIndividualChannels {
		channel = "editor-events:" + projectID
	}
	if err := b.pubsub.Publish(ctx, channel, string(payload)).Err(); err != nil {
		b.log.Debug("failed to publish canary", slog.String("err", err.Error()))
	}
}

// messageID is unique per message, so a duplicate delivery can be recognised.
func (b *RealTimeBridge) messageID() string {
	return fmt.Sprintf("doc:%s:%s-%d", b.host, b.random, b.counter.Add(1)-1)
}
