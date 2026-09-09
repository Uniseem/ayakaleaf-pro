// Package tags is the labels somebody puts on their own projects.
//
// A tag belongs to a person, not to a project: two people sharing a project
// each file it where they like, and neither sees the other's filing. That is
// why the project ids live on the tag rather than the tags on the project.
package tags

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Limits, so that one account cannot fill the collection.
const (
	maxTagsPerUser = 200
	maxNameLength  = 50
)

// Tag is a label and the projects it is on.
type Tag struct {
	ID         bson.ObjectID   `bson:"_id" json:"id"`
	UserID     bson.ObjectID   `bson:"user_id" json:"-"`
	Name       string          `bson:"name" json:"name"`
	Color      string          `bson:"color,omitempty" json:"color,omitempty"`
	ProjectIDs []bson.ObjectID `bson:"project_ids,omitempty" json:"-"`
	Created    time.Time       `bson:"created,omitempty" json:"-"`
}

// answer is a tag as the client reads it, with ids as strings.
type answer struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Color      string   `json:"color,omitempty"`
	ProjectIDs []string `json:"projectIds"`
}

func (t Tag) answer() answer {
	ids := make([]string, 0, len(t.ProjectIDs))
	for _, id := range t.ProjectIDs {
		ids = append(ids, id.Hex())
	}
	return answer{ID: t.ID.Hex(), Name: t.Name, Color: t.Color, ProjectIDs: ids}
}

// Service answers the tag endpoints.
type Service struct {
	tags *mongo.Collection
}

// New builds it.
func New(db *mongo.Database) *Service {
	return &Service{tags: db.Collection("tags")}
}

// EnsureIndexes creates what this package relies on.
//
// The unique index is on the pair, not on the name: two people may both have a
// tag called "thesis", and one person may not have two.
func (s *Service) EnsureIndexes(ctx context.Context) error {
	_, err := s.tags.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("user_name_unique"),
	})
	return err
}

// List answers with everything this person has tagged.
func (s *Service) List(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	cursor, err := s.tags.Find(r.Context(), bson.M{"user_id": user.ID},
		options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	var held []Tag
	if err := cursor.All(r.Context(), &held); err != nil {
		return apierr.Internal.WithCause(err)
	}
	out := make([]answer, 0, len(held))
	for _, tag := range held {
		out = append(out, tag.answer())
	}
	return httpapi.JSON(w, http.StatusOK, map[string]any{"tags": out})
}

// Create makes a tag.
func (s *Service) Create(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	var in struct {
		Name  string `json:"name"`
		Color string `json:"color,omitempty"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	name, err := validName(in.Name)
	if err != nil {
		return err
	}

	count, err := s.tags.CountDocuments(r.Context(), bson.M{"user_id": user.ID})
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	if count >= maxTagsPerUser {
		return apierr.BadRequest.
			WithMessage("That is as many tags as one account may have.")
	}

	tag := Tag{
		ID:      bson.NewObjectID(),
		UserID:  user.ID,
		Name:    name,
		Color:   strings.TrimSpace(in.Color),
		Created: time.Now().UTC(),
	}
	if _, err := s.tags.InsertOne(r.Context(), tag); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return apierr.Conflict.WithField("name").
				WithMessage("You already have a tag called that.")
		}
		return apierr.Internal.WithCause(err)
	}
	return httpapi.JSON(w, http.StatusCreated, map[string]any{"tag": tag.answer()})
}

// Update renames a tag or recolours it.
func (s *Service) Update(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	id, err := idFrom(r, "tagId")
	if err != nil {
		return err
	}
	var in struct {
		Name  string `json:"name"`
		Color string `json:"color,omitempty"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	name, err := validName(in.Name)
	if err != nil {
		return err
	}

	// Scoped to the person as well as the id, so that knowing an id is not
	// enough to rename somebody else's tag.
	result, err := s.tags.UpdateOne(r.Context(),
		bson.M{"_id": id, "user_id": user.ID},
		bson.M{"$set": bson.M{"name": name, "color": strings.TrimSpace(in.Color)}})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return apierr.Conflict.WithField("name").
				WithMessage("You already have a tag called that.")
		}
		return apierr.Internal.WithCause(err)
	}
	if result.MatchedCount == 0 {
		return apierr.NotFound
	}
	return httpapi.NoContent(w)
}

// Delete removes a tag. The projects it was on are untouched.
func (s *Service) Delete(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	id, err := idFrom(r, "tagId")
	if err != nil {
		return err
	}
	result, err := s.tags.DeleteOne(r.Context(), bson.M{"_id": id, "user_id": user.ID})
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	if result.DeletedCount == 0 {
		return apierr.NotFound
	}
	return httpapi.NoContent(w)
}

// AddProject puts a tag on a project.
//
// $addToSet rather than $push: tagging something twice is what a double click
// is, and it should not produce two entries.
func (s *Service) AddProject(w http.ResponseWriter, r *http.Request) error {
	return s.changeProject(w, r, true)
}

// RemoveProject takes a tag off a project.
func (s *Service) RemoveProject(w http.ResponseWriter, r *http.Request) error {
	return s.changeProject(w, r, false)
}

func (s *Service) changeProject(w http.ResponseWriter, r *http.Request, add bool) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	tagID, err := idFrom(r, "tagId")
	if err != nil {
		return err
	}
	projectID, err := idFrom(r, "projectId")
	if err != nil {
		return err
	}

	// Whether the person may see the project is not checked here on purpose.
	// A tag is private to its owner and holds only an id; being able to write
	// an id into your own list tells you nothing about whether it exists.
	update := bson.M{"$addToSet": bson.M{"project_ids": projectID}}
	if !add {
		update = bson.M{"$pull": bson.M{"project_ids": projectID}}
	}
	result, err := s.tags.UpdateOne(r.Context(),
		bson.M{"_id": tagID, "user_id": user.ID}, update)
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	if result.MatchedCount == 0 {
		return apierr.NotFound
	}
	return httpapi.NoContent(w)
}

// ForgetProject takes a deleted project off everybody's tags.
//
// A tag knows nothing about history, so the history id is not used here.
func (s *Service) ForgetProject(ctx context.Context, projectID bson.ObjectID, _ string) error {
	_, err := s.tags.UpdateMany(ctx,
		bson.M{"project_ids": projectID},
		bson.M{"$pull": bson.M{"project_ids": projectID}})
	return err
}

func validName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", apierr.BadRequest.WithField("name").
			WithMessage("A tag needs a name.")
	}
	if len(name) > maxNameLength {
		return "", apierr.BadRequest.WithField("name").
			WithMessage("That name is too long.")
	}
	return name, nil
}

func idFrom(r *http.Request, key string) (bson.ObjectID, error) {
	id, err := bson.ObjectIDFromHex(r.PathValue(key))
	if err != nil {
		return bson.ObjectID{}, apierr.NotFound
	}
	return id, nil
}

// ErrNotFound is returned when a tag does not exist or is not this person's.
var ErrNotFound = errors.New("no such tag")
