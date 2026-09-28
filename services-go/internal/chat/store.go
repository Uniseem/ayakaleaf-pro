package chat

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// GlobalThread is the sentinel the HTTP layer uses for a project's global
// chat; on disk it is a room document with no thread_id field.
const GlobalThread = "GLOBAL"

var (
	// ErrMissingThread mirrors ThreadManager.MissingThreadError.
	ErrMissingThread = errors.New("thread not found")
	// ErrMissingMessage mirrors MessageManager.MissingMessageError.
	ErrMissingMessage = errors.New("message not found")
)

// Store is the data access layer for the chat service. It reads and writes the
// same rooms and messages documents as the Node implementation.
type Store struct {
	rooms    *mongo.Collection
	messages *mongo.Collection
}

// NewStore binds a Store to the chat collections of db.
func NewStore(db *mongo.Database) *Store {
	return &Store{
		rooms:    db.Collection("rooms"),
		messages: db.Collection("messages"),
	}
}

// threadFilter builds the room selector for a thread, where a nil threadID
// means the project's global thread.
func threadFilter(projectID bson.ObjectID, threadID *bson.ObjectID) bson.D {
	if threadID == nil {
		return bson.D{
			{Key: "project_id", Value: projectID},
			{Key: "thread_id", Value: bson.D{{Key: "$exists", Value: false}}},
		}
	}
	return bson.D{
		{Key: "project_id", Value: projectID},
		{Key: "thread_id", Value: *threadID},
	}
}

// FindOrCreateThread returns the room for a thread, creating it if absent.
func (s *Store) FindOrCreateThread(ctx context.Context, projectID bson.ObjectID, threadID *bson.ObjectID) (Room, error) {
	set := bson.D{{Key: "project_id", Value: projectID}}
	if threadID != nil {
		set = append(set, bson.E{Key: "thread_id", Value: *threadID})
	}
	opts := options.FindOneAndUpdate().
		SetUpsert(true).
		SetReturnDocument(options.After)

	var room Room
	err := s.rooms.FindOneAndUpdate(ctx,
		threadFilter(projectID, threadID),
		bson.D{{Key: "$set", Value: set}},
		opts,
	).Decode(&room)
	return room, err
}

// FindThread returns an existing room, or ErrMissingThread.
func (s *Store) FindThread(ctx context.Context, projectID bson.ObjectID, threadID *bson.ObjectID) (Room, error) {
	var room Room
	err := s.rooms.FindOne(ctx, threadFilter(projectID, threadID)).Decode(&room)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return room, ErrMissingThread
	}
	return room, err
}

// FindAllThreadRooms returns every non-global room in a project.
func (s *Store) FindAllThreadRooms(ctx context.Context, projectID bson.ObjectID) ([]Room, error) {
	return s.findRooms(ctx, bson.D{
		{Key: "project_id", Value: projectID},
		{Key: "thread_id", Value: bson.D{{Key: "$exists", Value: true}}},
	})
}

// FindAllRooms returns every room in a project, the global thread included.
func (s *Store) FindAllRooms(ctx context.Context, projectID bson.ObjectID) ([]Room, error) {
	return s.findRooms(ctx, bson.D{{Key: "project_id", Value: projectID}})
}

// FindThreadsByID returns the rooms for the given thread ids.
func (s *Store) FindThreadsByID(ctx context.Context, projectID bson.ObjectID, threadIDs []bson.ObjectID) ([]Room, error) {
	return s.findRooms(ctx, bson.D{
		{Key: "project_id", Value: projectID},
		{Key: "thread_id", Value: bson.D{{Key: "$in", Value: threadIDs}}},
	})
}

func (s *Store) findRooms(ctx context.Context, filter bson.D) ([]Room, error) {
	cur, err := s.rooms.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	rooms := []Room{}
	if err := cur.All(ctx, &rooms); err != nil {
		return nil, err
	}
	return rooms, nil
}

// ResolveThread marks a thread resolved by userID at the current time.
func (s *Store) ResolveThread(ctx context.Context, projectID, threadID bson.ObjectID, userID string) error {
	_, err := s.rooms.UpdateOne(ctx,
		bson.D{{Key: "project_id", Value: projectID}, {Key: "thread_id", Value: threadID}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "resolved", Value: Resolved{
			UserID: userID,
			TS:     time.Now().UTC(),
		}}}}},
	)
	return err
}

// ReopenThread clears a thread's resolved marker.
func (s *Store) ReopenThread(ctx context.Context, projectID, threadID bson.ObjectID) error {
	_, err := s.rooms.UpdateOne(ctx,
		bson.D{{Key: "project_id", Value: projectID}, {Key: "thread_id", Value: threadID}},
		bson.D{{Key: "$unset", Value: bson.D{{Key: "resolved", Value: true}}}},
	)
	return err
}

// DeleteThread removes a thread's room and returns its id, creating the room
// first if it did not exist -- matching the Node implementation, which calls
// findOrCreateThread before deleting.
func (s *Store) DeleteThread(ctx context.Context, projectID bson.ObjectID, threadID *bson.ObjectID) (bson.ObjectID, error) {
	room, err := s.FindOrCreateThread(ctx, projectID, threadID)
	if err != nil {
		return bson.ObjectID{}, err
	}
	_, err = s.rooms.DeleteOne(ctx, bson.D{{Key: "_id", Value: room.ID}})
	return room.ID, err
}

// DeleteAllThreadsInProject removes every room in a project.
func (s *Store) DeleteAllThreadsInProject(ctx context.Context, projectID bson.ObjectID) error {
	_, err := s.rooms.DeleteMany(ctx, bson.D{{Key: "project_id", Value: projectID}})
	return err
}

// ResolvedThreadIDs lists the ids of a project's resolved threads.
func (s *Store) ResolvedThreadIDs(ctx context.Context, projectID bson.ObjectID) ([]string, error) {
	cur, err := s.rooms.Find(ctx,
		bson.D{
			{Key: "project_id", Value: projectID},
			{Key: "thread_id", Value: bson.D{{Key: "$exists", Value: true}}},
			{Key: "resolved", Value: bson.D{{Key: "$exists", Value: true}}},
		},
		options.Find().SetProjection(bson.D{{Key: "thread_id", Value: 1}}),
	)
	if err != nil {
		return nil, err
	}
	var rooms []Room
	if err := cur.All(ctx, &rooms); err != nil {
		return nil, err
	}
	ids := []string{}
	for _, r := range rooms {
		if r.ThreadID != nil {
			ids = append(ids, r.ThreadID.Hex())
		}
	}
	return ids, nil
}

// DuplicateThread creates a copy of a thread's room under a fresh thread id,
// carrying over its resolved state.
func (s *Store) DuplicateThread(ctx context.Context, projectID, threadID bson.ObjectID) (oldRoom, newRoom Room, err error) {
	err = s.rooms.FindOne(ctx, bson.D{
		{Key: "project_id", Value: projectID},
		{Key: "thread_id", Value: threadID},
	}).Decode(&oldRoom)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return oldRoom, newRoom, ErrMissingThread
	}
	if err != nil {
		return oldRoom, newRoom, err
	}

	newThreadID := bson.NewObjectID()
	newRoom = Room{
		ProjectID: oldRoom.ProjectID,
		ThreadID:  &newThreadID,
		Resolved:  oldRoom.Resolved,
	}
	res, err := s.rooms.InsertOne(ctx, newRoom)
	if err != nil {
		return oldRoom, newRoom, err
	}
	newRoom.ID = res.InsertedID.(bson.ObjectID)
	return oldRoom, newRoom, nil
}

// RoomMapping records one source room and its clone.
type RoomMapping struct{ From, To bson.ObjectID }

// CloneThreads copies every comment thread of a project into another project.
func (s *Store) CloneThreads(ctx context.Context, sourceProjectID, targetProjectID bson.ObjectID) ([]RoomMapping, error) {
	rooms, err := s.FindAllThreadRooms(ctx, sourceProjectID)
	if err != nil {
		return nil, err
	}
	mapping := make([]RoomMapping, 0, len(rooms))
	docs := make([]any, 0, len(rooms))
	for _, room := range rooms {
		clone := room
		clone.ID = bson.NewObjectID()
		clone.ProjectID = targetProjectID
		mapping = append(mapping, RoomMapping{From: room.ID, To: clone.ID})
		docs = append(docs, clone)
	}
	// Mongo rejects an empty batch, so a project with no comment threads is a
	// no-op here rather than an error.
	if len(docs) == 0 {
		return mapping, nil
	}
	if _, err := s.rooms.InsertMany(ctx, docs); err != nil {
		return nil, err
	}
	return mapping, nil
}

// CreateMessage inserts a message into a room.
func (s *Store) CreateMessage(ctx context.Context, roomID, userID bson.ObjectID, content string, ts Millis) (Message, error) {
	msg := Message{Content: content, RoomID: roomID, UserID: userID, Timestamp: ts}
	res, err := s.messages.InsertOne(ctx, msg)
	if err != nil {
		return msg, err
	}
	msg.ID = res.InsertedID.(bson.ObjectID)
	return msg, nil
}

// GetMessages returns up to limit messages in a room, newest first, optionally
// restricted to those older than before.
func (s *Store) GetMessages(ctx context.Context, roomID bson.ObjectID, limit int64, before *Millis) ([]Message, error) {
	filter := bson.D{{Key: "room_id", Value: roomID}}
	if before != nil {
		filter = append(filter, bson.E{Key: "timestamp", Value: bson.D{{Key: "$lt", Value: *before}}})
	}
	cur, err := s.messages.Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "timestamp", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	messages := []Message{}
	if err := cur.All(ctx, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

// FindAllMessagesInRooms returns every message across the given rooms.
func (s *Store) FindAllMessagesInRooms(ctx context.Context, roomIDs []bson.ObjectID) ([]Message, error) {
	cur, err := s.messages.Find(ctx, bson.D{
		{Key: "room_id", Value: bson.D{{Key: "$in", Value: roomIDs}}},
	})
	if err != nil {
		return nil, err
	}
	messages := []Message{}
	if err := cur.All(ctx, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

// GetMessage returns one message in a room, or ErrMissingMessage.
func (s *Store) GetMessage(ctx context.Context, roomID, messageID bson.ObjectID) (Message, error) {
	var msg Message
	err := s.messages.FindOne(ctx, bson.D{
		{Key: "_id", Value: messageID},
		{Key: "room_id", Value: roomID},
	}).Decode(&msg)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return msg, ErrMissingMessage
	}
	return msg, err
}

// UpdateMessage edits a message's content, optionally restricted to one
// author. It reports whether a document was modified.
func (s *Store) UpdateMessage(ctx context.Context, roomID, messageID bson.ObjectID, userID *bson.ObjectID, content string, ts Millis) (bool, error) {
	filter := bson.D{
		{Key: "_id", Value: messageID},
		{Key: "room_id", Value: roomID},
	}
	if userID != nil {
		filter = append(filter, bson.E{Key: "user_id", Value: *userID})
	}
	res, err := s.messages.UpdateOne(ctx, filter, bson.D{{Key: "$set", Value: bson.D{
		{Key: "content", Value: content},
		{Key: "edited_at", Value: ts},
	}}})
	if err != nil {
		return false, err
	}
	return res.ModifiedCount == 1, nil
}

// DeleteMessage removes one message from a room.
func (s *Store) DeleteMessage(ctx context.Context, roomID, messageID bson.ObjectID) error {
	_, err := s.messages.DeleteOne(ctx, bson.D{
		{Key: "_id", Value: messageID},
		{Key: "room_id", Value: roomID},
	})
	return err
}

// DeleteUserMessage removes one message, but only if userID wrote it.
func (s *Store) DeleteUserMessage(ctx context.Context, userID, roomID, messageID bson.ObjectID) error {
	_, err := s.messages.DeleteOne(ctx, bson.D{
		{Key: "_id", Value: messageID},
		{Key: "user_id", Value: userID},
		{Key: "room_id", Value: roomID},
	})
	return err
}

// DeleteAllMessagesInRoom removes every message in a room.
func (s *Store) DeleteAllMessagesInRoom(ctx context.Context, roomID bson.ObjectID) error {
	_, err := s.messages.DeleteMany(ctx, bson.D{{Key: "room_id", Value: roomID}})
	return err
}

// DeleteAllMessagesInRooms removes every message across the given rooms.
func (s *Store) DeleteAllMessagesInRooms(ctx context.Context, roomIDs []bson.ObjectID) error {
	_, err := s.messages.DeleteMany(ctx, bson.D{
		{Key: "room_id", Value: bson.D{{Key: "$in", Value: roomIDs}}},
	})
	return err
}

// DuplicateRoomToOtherRoom copies a room's messages into another room, keeping
// author and timestamp so the clone reads identically.
func (s *Store) DuplicateRoomToOtherRoom(ctx context.Context, sourceRoomID, targetRoomID bson.ObjectID) error {
	sourceMessages, err := s.FindAllMessagesInRooms(ctx, []bson.ObjectID{sourceRoomID})
	if err != nil {
		return err
	}
	if len(sourceMessages) == 0 {
		return nil
	}
	docs := make([]any, 0, len(sourceMessages))
	for _, m := range sourceMessages {
		docs = append(docs, Message{
			RoomID:    targetRoomID,
			Content:   m.Content,
			Timestamp: m.Timestamp,
			UserID:    m.UserID,
		})
	}
	_, err = s.messages.InsertMany(ctx, docs)
	return err
}
