package projects

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// SetHistoryID records what the history services call this project.
//
// Written once, when the project is created and its history is started. A
// project without it has nowhere to record changes, which is why creating one
// fails rather than leaving it half made.
func (s *Store) SetHistoryID(ctx context.Context, id bson.ObjectID, historyID string) error {
	_, err := s.projects.UpdateByID(ctx, id, bson.M{
		"$set": bson.M{"overleaf.history.id": historyID},
	})
	return err
}
