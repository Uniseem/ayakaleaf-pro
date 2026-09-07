package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
)

// restrictedUserMessagePassList is the set of events a restricted user may
// receive. A restricted user is a link-share visitor with limited access: they
// see the document and their own edits, but not the project chat, the
// collaborator list, or anything else broadcast to real members.
//
// The list is a pass list rather than a block list on purpose. A new event
// added to web is hidden from restricted users until somebody decides it is
// safe, which is the right way round for a privacy boundary.
var restrictedUserMessagePassList = map[string]bool{
	"otUpdateApplied":      true,
	"otUpdateError":        true,
	"joinDoc":              true,
	"reciveNewDoc":         true,
	"reciveNewFile":        true,
	"reciveNewFolder":      true,
	"reciveEntityMove":     true,
	"reciveEntityRename":   true,
	"removeEntity":         true,
	"accept-changes":       true,
	"projectNameUpdated":   true,
	"rootDocUpdated":       true,
	"toggle-track-changes": true,
	"projectRenamedOrDeletedByExternalSource": true,
}

// editorEvent is the envelope published on the editor-events channel.
type editorEvent struct {
	RoomID      string            `json:"room_id"`
	Message     string            `json:"message"`
	Payload     []json.RawMessage `json:"payload"`
	ID          string            `json:"_id,omitempty"`
	HealthCheck bool              `json:"health_check,omitempty"`
	Key         string            `json:"key,omitempty"`
}

// EmitToRoom publishes an event to every client in a room, cluster-wide.
//
// It goes through Redis even when the recipients are connected to this same
// process, because in a multi-instance deployment they usually are not.
func (s *Service) EmitToRoom(ctx context.Context, roomID, message string, payload ...any) {
	if roomID == "" {
		s.log.Warn("no room_id provided, ignoring emitToRoom", slog.String("message", message))
		return
	}
	raw := make([]json.RawMessage, 0, len(payload))
	for _, p := range payload {
		encoded, err := json.Marshal(p)
		if err != nil {
			s.log.Error("dropping unencodable payload", slog.String("message", message))
			return
		}
		raw = append(raw, encoded)
	}
	data, err := json.Marshal(editorEvent{RoomID: roomID, Message: message, Payload: raw})
	if err != nil {
		s.log.Error("dropping unencodable event", slog.String("message", message))
		return
	}
	s.editorEvents.Publish(ctx, roomID, string(data), s.cfg.PublishOnIndividualChannels)
}

// EmitToAll publishes an event to every connected client, cluster-wide.
func (s *Service) EmitToAll(ctx context.Context, message string, payload ...any) {
	s.EmitToRoom(ctx, "all", message, payload...)
}

// processEditorEvent distributes one message from the editor-events channel.
func (s *Service) processEditorEvent(channel, raw string) {
	if len(raw) > s.cfg.MaxUpdateSize {
		s.log.Error("data too large to parse", slog.String("channel", channel),
			slog.Int("length", len(raw)))
		return
	}
	var ev editorEvent
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		s.log.Error("error parsing JSON", slog.String("channel", channel),
			slog.String("err", err.Error()))
		return
	}

	switch {
	case ev.RoomID == "all":
		s.io.Broadcast(ev.Message, rawArgs(ev.Payload)...)

	case ev.Message == "clientTracking.refresh" && ev.RoomID != "":
		// web asks every instance to renew the entries of the clients it
		// holds, so the collaborator list reflects who is really there.
		ctx := context.Background()
		for _, c := range s.io.ClientsIn(ev.RoomID) {
			if cc := s.context(c); cc != nil {
				s.users.RefreshClient(ctx, ev.RoomID, cc.PublicID)
			}
		}

	case ev.Message == "canary-applied-op":
		// A measurement probe from document-updater, not an editor event. It
		// is consumed here so it is never forwarded to a client.

	case ev.RoomID != "":
		s.distributeEditorEvent(ev)

	case ev.HealthCheck:
		s.health.Record(channel, ev.Key)
	}
}

func (s *Service) distributeEditorEvent(ev editorEvent) {
	clients := s.io.ClientsIn(ev.RoomID)
	if len(clients) == 0 {
		return
	}
	isRestricted := !restrictedUserMessagePassList[ev.Message]
	args := rawArgs(ev.Payload)

	for _, c := range clients {
		cc := s.context(c)
		if cc == nil {
			continue
		}
		if s.shouldDisconnectClient(cc, ev) {
			s.log.Debug("disconnecting client", slog.String("message", ev.Message),
				slog.String("user", cc.UserID()), slog.String("project", cc.ProjectID()))
			if ev.Message != "project:collaboratorAccessLevel:changed" {
				// The editor shows an explanation for a revoked project; a
				// changed access level is handled by a reload instead.
				_ = c.Emit("project:access:revoked")
			}
			c.Close()
			continue
		}
		if isRestricted && cc.IsRestrictedUser() {
			s.log.Debug("hiding restricted message from client",
				slog.String("message", ev.Message), slog.String("client", c.ID))
			continue
		}
		_ = c.Emit(ev.Message, args...)
	}
}

// shouldDisconnectClient reports whether an event has taken this client's
// access away.
func (s *Service) shouldDisconnectClient(cc *clientContext, ev editorEvent) bool {
	userID := cc.UserID()
	switch ev.Message {
	case "userRemovedFromProject":
		var removed []string
		if len(ev.Payload) > 0 && json.Unmarshal(ev.Payload[0], &removed) == nil {
			for _, id := range removed {
				if id == userID {
					return true
				}
			}
		}
	case "project:publicAccessLevel:changed":
		var info struct {
			NewAccessLevel string `json:"newAccessLevel"`
		}
		if len(ev.Payload) > 0 && json.Unmarshal(ev.Payload[0], &info) == nil {
			// A project turned private drops everyone who was there on a
			// share link, but keeps the invited members.
			return info.NewAccessLevel == "private" && !cc.IsInvitedMember()
		}
	case "project:collaboratorAccessLevel:changed":
		var info struct {
			UserID string `json:"userId"`
		}
		if len(ev.Payload) > 0 && json.Unmarshal(ev.Payload[0], &info) == nil {
			return info.UserID == userID
		}
	}
	return false
}

// rawArgs converts a decoded payload into arguments for an emit.
func rawArgs(payload []json.RawMessage) []any {
	args := make([]any, len(payload))
	for i, p := range payload {
		args[i] = p
	}
	return args
}
