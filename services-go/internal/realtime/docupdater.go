package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"strings"
)

// appliedOpsMessage is what document-updater publishes on applied-ops.
type appliedOpsMessage struct {
	DocID       string          `json:"doc_id"`
	Op          json.RawMessage `json:"op"`
	Error       json.RawMessage `json:"error"`
	ID          string          `json:"_id"`
	HealthCheck bool            `json:"health_check"`
	Key         string          `json:"key"`
}

// processAppliedOp distributes one message from the applied-ops channel.
//
// This is the path every keystroke in a shared document travels:
// document-updater applies the operation, publishes it, and every real-time
// instance holding a client in that document forwards it.
func (s *Service) processAppliedOp(channel, raw string) {
	if len(raw) > s.cfg.MaxUpdateSize {
		s.log.Error("data too large to parse", slog.String("channel", channel),
			slog.Int("length", len(raw)))
		return
	}
	var msg appliedOpsMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		s.log.Error("error parsing JSON", slog.String("channel", channel),
			slog.String("err", err.Error()))
		return
	}

	switch {
	case len(msg.Op) > 0:
		s.applyUpdateFromDocumentUpdater(msg.DocID, msg.Op)
	case len(msg.Error) > 0:
		s.processErrorFromDocumentUpdater(msg.DocID, msg.Error, raw)
	case msg.HealthCheck:
		s.health.Record(channel, msg.Key)
	}
}

func (s *Service) applyUpdateFromDocumentUpdater(docID string, rawUpdate json.RawMessage) {
	clients := s.io.ClientsIn(docID)
	if len(clients) == 0 {
		return
	}

	var update map[string]any
	if err := json.Unmarshal(rawUpdate, &update); err != nil {
		s.log.Error("error parsing update", slog.String("doc", docID),
			slog.String("err", err.Error()))
		return
	}

	meta, _ := update["meta"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
		update["meta"] = meta
	}
	// tsRT is the ingestion timestamp this service stamped on the way in. It
	// is ours, not the client's, so it is stripped before the update goes out.
	delete(meta, "tsRT")
	source, _ := meta["source"].(string)
	dup, _ := update["dup"].(bool)

	for _, c := range clients {
		cc := s.context(c)
		if cc == nil {
			continue
		}
		if cc.PublicID == source {
			// The sender gets an acknowledgement, not the operation: it
			// already has the text and only needs the version it landed at.
			_ = c.Emit("otUpdateApplied", map[string]any{
				"v": update["v"], "doc": update["doc"],
			})
			continue
		}
		if dup {
			// A duplicate is only meaningful to the client that sent it.
			continue
		}
		_ = c.Emit("otUpdateApplied", update)
	}
}

func (s *Service) processErrorFromDocumentUpdater(docID string, updateErr json.RawMessage, raw string) {
	for _, c := range s.io.ClientsIn(docID) {
		s.log.Warn("error from document updater, disconnecting client",
			slog.String("doc", docID), slog.String("client", c.ID),
			slog.String("err", string(updateErr)))
		_ = c.Emit("otUpdateError", updateErr, json.RawMessage(raw))
		c.Close()
	}
}

// queueChangeAllowedKeys is the whitelist from DocumentUpdaterManager. The
// update comes from the browser, so anything not on this list is dropped
// rather than handed to document-updater.
var queueChangeAllowedKeys = map[string]bool{
	"doc": true, "op": true, "v": true, "dupIfSource": true,
	"meta": true, "lastV": true, "hash": true,
}

// QueueChange hands an update to document-updater through Redis.
//
// The push order matters: the operation goes onto the per-document list first,
// because document-updater starts processing as soon as the document id
// appears on the shared queue.
func (s *Service) QueueChange(ctx context.Context, projectID, docID string, update map[string]json.RawMessage) error {
	filtered := make(map[string]json.RawMessage, len(update))
	for k, v := range update {
		if queueChangeAllowedKeys[k] {
			filtered[k] = v
		}
	}
	encoded, err := json.Marshal(filtered)
	if err != nil {
		return err
	}
	if strings.ContainsRune(string(encoded), 0) {
		// A null byte in an update means memory corruption somewhere upstream;
		// letting it through would corrupt the stored document.
		return ErrNullBytesInOp
	}
	if len(encoded) > s.cfg.MaxUpdateSize {
		return ErrUpdateTooLarge
	}

	if err := s.docUpdaterRedis.RPush(ctx, s.keys.PendingUpdates(docID), string(encoded)).Err(); err != nil {
		return fmt.Errorf("error pushing update into redis: %w", err)
	}
	if err := s.docUpdaterRedis.RPush(ctx, s.pendingUpdateListKey(), projectID+":"+docID).Err(); err != nil {
		return fmt.Errorf("error pushing doc_id into redis: %w", err)
	}
	return nil
}

// pendingUpdateListKey picks one of the sharded queues at random. The shard
// count must match document-updater's dispatcher count, or updates land on a
// queue nothing is reading.
func (s *Service) pendingUpdateListKey() string {
	shard := rand.IntN(s.cfg.PendingUpdateListShardCount)
	if shard == 0 {
		return "pending-updates-list"
	}
	return "pending-updates-list-" + strconv.Itoa(shard)
}
