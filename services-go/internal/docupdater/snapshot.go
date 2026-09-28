package docupdater

import (
	"context"
	"encoding/json"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// SnapshotStore keeps a copy of a document from just before an edit destroyed
// something in it.
//
// An edit that removes the text a comment was attached to takes the comment
// with it, and there is nothing left to recover it from. This is that
// something: the document and its markers as they were, kept for as long as the
// collection is set to keep them.
type SnapshotStore struct {
	snapshots *mongo.Collection
}

// NewSnapshotStore builds the store.
func NewSnapshotStore(database *mongo.Database) *SnapshotStore {
	if database == nil {
		return nil
	}
	return &SnapshotStore{snapshots: database.Collection("docSnapshots")}
}

// RecordSnapshot writes the document down as it was.
func (s *SnapshotStore) RecordSnapshot(ctx context.Context, projectID, docID string,
	version int64, pathname string, lines []string, ranges json.RawMessage) error {

	project, err := bson.ObjectIDFromHex(projectID)
	if err != nil {
		return err
	}
	doc, err := bson.ObjectIDFromHex(docID)
	if err != nil {
		return err
	}

	_, err = s.snapshots.InsertOne(ctx, bson.M{
		"project_id": project,
		"doc_id":     doc,
		"version":    version,
		"lines":      lines,
		"pathname":   pathname,
		"ranges":     rangesForMongo(ranges),
		"ts":         time.Now(),
	})
	return err
}

// rangesForMongo converts the markers to the shapes Mongo stores them in.
//
// The ids and the user ids are object ids there rather than strings, and the
// timestamps are dates. What cannot be converted is kept as it is: these are
// only a record, and a snapshot that stores an id as a string is better than
// one that is not written at all.
func rangesForMongo(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var ranges map[string]any
	if err := json.Unmarshal(raw, &ranges); err != nil {
		return nil
	}

	for _, key := range []string{"changes", "comments"} {
		list, ok := ranges[key].([]any)
		if !ok {
			continue
		}
		for _, element := range list {
			marker, ok := element.(map[string]any)
			if !ok {
				continue
			}
			if id, ok := marker["id"].(string); ok {
				marker["id"] = objectIDOrString(id)
			}
			if op, ok := marker["op"].(map[string]any); ok {
				if thread, ok := op["t"].(string); ok {
					op["t"] = objectIDOrString(thread)
				}
			}
			metadata, ok := marker["metadata"].(map[string]any)
			if !ok {
				continue
			}
			if ts, ok := metadata["ts"].(string); ok {
				if parsed, err := time.Parse(time.RFC3339Nano, ts); err == nil {
					metadata["ts"] = parsed
				}
			}
			if userID, ok := metadata["user_id"].(string); ok {
				metadata["user_id"] = objectIDOrString(userID)
			}
		}
	}
	return ranges
}

// objectIDOrString converts an id where it can, and leaves it alone where it
// cannot.
func objectIDOrString(value string) any {
	if id, err := bson.ObjectIDFromHex(value); err == nil {
		return id
	}
	return value
}
