package notifications

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Store is the data access layer for the notifications service, reading and
// writing the same `notifications` documents as the Node implementation.
type Store struct {
	notifications *mongo.Collection
}

// NewStore binds a Store to the notifications collection of db.
func NewStore(db *mongo.Database) *Store {
	return &Store{notifications: db.Collection("notifications")}
}

// Notification is the request body accepted by AddNotification. Fields absent
// from the request stay nil and are written as null, which is what the Node
// driver does with an undefined value.
type Notification struct {
	Key         string `json:"key"`
	MessageOpts any    `json:"messageOpts"`
	TemplateKey any    `json:"templateKey"`
	Expires     any    `json:"expires"`
	ForceCreate bool   `json:"forceCreate"`
}

// GetUserNotifications returns a user's unread notifications, in stored field
// order so the JSON matches the Node service byte for byte.
func (s *Store) GetUserNotifications(ctx context.Context, userID bson.ObjectID) ([]bson.D, error) {
	cur, err := s.notifications.Find(ctx, bson.D{
		{Key: "user_id", Value: userID},
		{Key: "templateKey", Value: bson.D{{Key: "$exists", Value: true}}},
	})
	if err != nil {
		return nil, err
	}
	docs := []bson.D{}
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}

// AddNotification upserts a notification for a user.
//
// An existing notification with the same key is left untouched unless the
// request sets forceCreate, matching the Node implementation's early return.
func (s *Store) AddNotification(ctx context.Context, userID bson.ObjectID, n Notification) error {
	count, err := s.notifications.CountDocuments(ctx, bson.D{
		{Key: "user_id", Value: userID},
		{Key: "key", Value: n.Key},
	})
	if err != nil {
		return err
	}
	if count != 0 && !n.ForceCreate {
		return nil
	}

	doc := bson.D{
		{Key: "user_id", Value: userID},
		{Key: "key", Value: n.Key},
		{Key: "messageOpts", Value: n.MessageOpts},
		{Key: "templateKey", Value: n.TemplateKey},
	}
	// A TTL index on `expires` removes the document once it is due; documents
	// without the field are ignored by that index.
	if n.Expires != nil {
		expires, err := parseJSDate(n.Expires)
		if err != nil {
			return err
		}
		doc = append(doc, bson.E{Key: "expires", Value: expires})
	}

	_, err = s.notifications.UpdateOne(ctx,
		bson.D{{Key: "user_id", Value: userID}, {Key: "key", Value: n.Key}},
		bson.D{{Key: "$set", Value: doc}},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

// parseJSDate converts the JSON forms `new Date(value)` accepts: an ISO 8601
// string, or a millisecond epoch number.
func parseJSDate(v any) (time.Time, error) {
	switch value := v.(type) {
	case string:
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
			if t, err := time.Parse(layout, value); err == nil {
				return t.UTC(), nil
			}
		}
		return time.Time{}, fmt.Errorf("notifications: invalid expires date %q", value)
	case float64:
		return time.UnixMilli(int64(value)).UTC(), nil
	case int64:
		return time.UnixMilli(value).UTC(), nil
	default:
		return time.Time{}, errors.New("notifications: expires must be an ISO date string or epoch milliseconds")
	}
}

// RemoveNotificationID marks one notification read by dropping the fields that
// make it renderable, keeping the document so it is not re-created.
func (s *Store) RemoveNotificationID(ctx context.Context, userID, notificationID bson.ObjectID) error {
	_, err := s.notifications.UpdateOne(ctx,
		bson.D{{Key: "user_id", Value: userID}, {Key: "_id", Value: notificationID}},
		bson.D{{Key: "$unset", Value: bson.D{
			{Key: "templateKey", Value: true},
			{Key: "messageOpts", Value: true},
		}}},
	)
	return err
}

// RemoveNotificationKey marks a user's notification with the given key read.
func (s *Store) RemoveNotificationKey(ctx context.Context, userID bson.ObjectID, key string) error {
	_, err := s.notifications.UpdateOne(ctx,
		bson.D{{Key: "user_id", Value: userID}, {Key: "key", Value: key}},
		bson.D{{Key: "$unset", Value: bson.D{{Key: "templateKey", Value: true}}}},
	)
	return err
}

// RemoveNotificationByKeyOnly marks one notification with the given key read,
// regardless of owner.
func (s *Store) RemoveNotificationByKeyOnly(ctx context.Context, key string) error {
	_, err := s.notifications.UpdateOne(ctx,
		bson.D{{Key: "key", Value: key}},
		bson.D{{Key: "$unset", Value: bson.D{{Key: "templateKey", Value: true}}}},
	)
	return err
}

// CountNotificationsByKeyOnly counts unread notifications with the given key.
func (s *Store) CountNotificationsByKeyOnly(ctx context.Context, key string) (int64, error) {
	return s.notifications.CountDocuments(ctx, bson.D{
		{Key: "key", Value: key},
		{Key: "templateKey", Value: bson.D{{Key: "$exists", Value: true}}},
	})
}

// DeleteUnreadNotificationsByKeyOnlyBulk hard-deletes every unread
// notification with the given key and reports how many were removed.
func (s *Store) DeleteUnreadNotificationsByKeyOnlyBulk(ctx context.Context, key string) (int64, error) {
	res, err := s.notifications.DeleteMany(ctx, bson.D{
		{Key: "key", Value: key},
		{Key: "templateKey", Value: bson.D{{Key: "$exists", Value: true}}},
	})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// DeleteByUserID removes a user's notifications. It backs the health check's
// cleanup step.
func (s *Store) DeleteByUserID(ctx context.Context, userID bson.ObjectID) error {
	_, err := s.notifications.DeleteMany(ctx, bson.D{{Key: "user_id", Value: userID}})
	return err
}
