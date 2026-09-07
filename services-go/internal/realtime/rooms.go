package realtime

import (
	"context"
	"log/slog"
	"sync"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/socketio"
)

// Entity kinds tracked by the room manager. A room id alone does not say
// whether it is a project or a doc, and the two are backed by different Redis
// channels, so the mapping has to be remembered until the room empties.
const (
	entityProject = "project"
	entityDoc     = "doc"
)

// RoomManager keeps socket.io room membership and the Redis subscriptions in
// step.
//
// The rule it enforces is the one that makes collaborative editing correct:
// the channel for a room is subscribed before the first client is told it has
// joined, and unsubscribed only after the last one leaves. Subscribing late
// would drop the updates that arrive in between, and the client would not know
// its document had fallen behind.
type RoomManager struct {
	srv      *socketio.Server
	log      *slog.Logger
	projects *ChannelManager
	docs     *ChannelManager

	mu     sync.Mutex
	entity map[string]string
}

// NewRoomManager builds a room manager over the two channel managers.
func NewRoomManager(srv *socketio.Server, projects, docs *ChannelManager, log *slog.Logger) *RoomManager {
	return &RoomManager{
		srv: srv, log: log, projects: projects, docs: docs,
		entity: map[string]string{},
	}
}

// JoinProject puts a client in a project room, subscribing editor-events for
// it if the room was empty.
func (rm *RoomManager) JoinProject(ctx context.Context, c *socketio.Conn, projectID string) error {
	return rm.joinEntity(ctx, c, entityProject, projectID)
}

// JoinDoc puts a client in a doc room, subscribing applied-ops for it if the
// room was empty.
func (rm *RoomManager) JoinDoc(ctx context.Context, c *socketio.Conn, docID string) error {
	return rm.joinEntity(ctx, c, entityDoc, docID)
}

// LeaveDoc removes a client from a doc room.
func (rm *RoomManager) LeaveDoc(ctx context.Context, c *socketio.Conn, docID string) {
	rm.leaveEntity(ctx, c, entityDoc, docID)
}

// LeaveProjectAndDocs removes a client from every room it is in.
//
// socket.io would drop the memberships on disconnect anyway, but the
// subscriptions are ours to manage, so the rooms are left explicitly.
func (rm *RoomManager) LeaveProjectAndDocs(ctx context.Context, c *socketio.Conn) {
	rooms := c.Rooms()
	rm.log.Debug("client leaving project", slog.String("client", c.ID), slog.Any("rooms", rooms))
	for _, id := range rooms {
		rm.mu.Lock()
		entity := rm.entity[id]
		rm.mu.Unlock()
		rm.leaveEntity(ctx, c, entity, id)
	}
}

func (rm *RoomManager) joinEntity(ctx context.Context, c *socketio.Conn, entity, id string) error {
	// The client joins the room straight away, but the join is not complete --
	// and the caller must not answer the client -- until the channel is
	// subscribed.
	before := c.Join(id)
	rm.mu.Lock()
	rm.entity[id] = entity
	rm.mu.Unlock()

	if before > 0 {
		rm.log.Debug("client joined existing room", slog.String("client", c.ID),
			slog.String("entity", entity), slog.String("id", id))
		return nil
	}

	rm.log.Debug("room is now active", slog.String("entity", entity), slog.String("id", id))
	var err error
	switch entity {
	case entityProject:
		err = rm.projects.Subscribe(ctx, id)
	case entityDoc:
		err = rm.docs.Subscribe(ctx, id)
	}
	if err != nil {
		// The client is in the room but its channel is not subscribed, so back
		// the join out rather than leave it silently missing updates.
		c.Leave(id)
		return err
	}
	rm.log.Debug("client joined new room and subscribed to channel",
		slog.String("client", c.ID), slog.String("entity", entity), slog.String("id", id))
	return nil
}

func (rm *RoomManager) leaveEntity(ctx context.Context, c *socketio.Conn, entity, id string) {
	after, wasIn := c.Leave(id)
	if !wasIn {
		// The editor sends leaveDoc for documents it never joined after a
		// reconnect, and a client that disconnects mid-join never entered the
		// room at all.
		rm.log.Debug("ignoring request from client to leave room it is not in",
			slog.String("client", c.ID), slog.String("id", id))
		return
	}
	if entity == "" {
		rm.log.Error("unknown entity when leaving with id", slog.String("id", id))
		return
	}
	if after > 0 {
		return
	}

	rm.log.Debug("room is now empty", slog.String("entity", entity), slog.String("id", id))
	rm.mu.Lock()
	delete(rm.entity, id)
	rm.mu.Unlock()
	switch entity {
	case entityProject:
		rm.projects.Unsubscribe(ctx, id)
	case entityDoc:
		rm.docs.Unsubscribe(ctx, id)
	}
}
