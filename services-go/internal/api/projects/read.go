package projects

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Read reads a project without asking who wants it.
//
// Only for the other services in the deployment, which are not people: a
// request that arrived over the internal port has already been authenticated
// as one of them, and what they need is the project, not a decision about
// somebody's access to it. Nothing a browser can reach uses this.
func (s *Store) Read(ctx context.Context, id bson.ObjectID) (*Project, error) {
	var project Project
	err := s.projects.FindOne(ctx, bson.M{"_id": id}).Decode(&project)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &project, nil
}
