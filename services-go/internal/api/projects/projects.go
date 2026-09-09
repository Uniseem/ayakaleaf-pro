// Package projects owns the project document: what a project is, who may see
// it, and the tree of files inside it.
//
// Access is decided here and nowhere else. Every read and write takes the
// person asking, so there is no way to reach a project without having said who
// wants it -- which is the failure mode of the service this replaces, where
// the check and the fetch are separate calls and a handler can do the second
// without the first.
package projects

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var (
	// ErrNotFound is returned when no project matches, and also when one does
	// but the person asking may not see it: telling those apart tells somebody
	// that a project exists.
	ErrNotFound = errors.New("no such project")
	// ErrForbidden is returned when somebody may see a project but not do this
	// to it.
	ErrForbidden = errors.New("not allowed")
	// ErrNameTaken is returned when a move or a rename would put two things
	// with the same name in one folder.
	ErrNameTaken = errors.New("name already taken")
)

// Access is what somebody may do with a project.
type Access string

const (
	AccessNone   Access = ""
	AccessRead   Access = "readOnly"
	AccessWrite  Access = "readAndWrite"
	AccessReview Access = "review"
	AccessOwner  Access = "owner"
)

// CanWrite says whether this access allows changing the project.
func (a Access) CanWrite() bool { return a == AccessWrite || a == AccessOwner }

// CanAdmin says whether this access allows changing who else has access.
func (a Access) CanAdmin() bool { return a == AccessOwner }

// Folder is a directory in a project.
type Folder struct {
	ID       bson.ObjectID `bson:"_id" json:"id"`
	Name     string        `bson:"name" json:"name"`
	Folders  []Folder      `bson:"folders,omitempty" json:"folders,omitempty"`
	Docs     []DocRef      `bson:"docs,omitempty" json:"docs,omitempty"`
	FileRefs []FileRef     `bson:"fileRefs,omitempty" json:"fileRefs,omitempty"`
}

// DocRef is a text document in the tree. Its content lives in docstore.
type DocRef struct {
	ID   bson.ObjectID `bson:"_id" json:"id"`
	Name string        `bson:"name" json:"name"`
}

// FileRef is a binary file in the tree. Its content lives in filestore.
type FileRef struct {
	ID      bson.ObjectID `bson:"_id" json:"id"`
	Name    string        `bson:"name" json:"name"`
	Hash    string        `bson:"hash,omitempty" json:"hash,omitempty"`
	Created time.Time     `bson:"created,omitempty" json:"created,omitempty"`
	Size    int64         `bson:"size,omitempty" json:"size,omitempty"`
}

// Project is a project.
type Project struct {
	ID                   bson.ObjectID   `bson:"_id" json:"id"`
	Name                 string          `bson:"name" json:"name"`
	OwnerRef             bson.ObjectID   `bson:"owner_ref" json:"ownerId"`
	Collaborators        []bson.ObjectID `bson:"collaberator_refs,omitempty" json:"-"`
	ReadOnly             []bson.ObjectID `bson:"readOnly_refs,omitempty" json:"-"`
	TokenAccessReadWrite []bson.ObjectID `bson:"tokenAccessReadAndWrite_refs,omitempty" json:"-"`
	TokenAccessReadOnly  []bson.ObjectID `bson:"tokenAccessReadOnly_refs,omitempty" json:"-"`

	RootFolder []Folder      `bson:"rootFolder,omitempty" json:"rootFolder,omitempty"`
	RootDocID  bson.ObjectID `bson:"rootDoc_id,omitempty" json:"rootDocId,omitempty"`
	Compiler   string        `bson:"compiler,omitempty" json:"compiler,omitempty"`
	ImageName  string        `bson:"imageName,omitempty" json:"imageName,omitempty"`
	SpellCheck string        `bson:"spellCheckLanguage,omitempty" json:"spellCheckLanguage,omitempty"`

	// Version counts changes to the tree. The history is ordered by it.
	Version int64 `bson:"version,omitempty" json:"version,omitempty"`

	LastUpdated   time.Time       `bson:"lastUpdated,omitempty" json:"lastUpdated,omitempty"`
	LastUpdatedBy *bson.ObjectID  `bson:"lastUpdatedBy,omitempty" json:"lastUpdatedBy,omitempty"`
	Archived      []bson.ObjectID `bson:"archived,omitempty" json:"-"`
	Trashed       []bson.ObjectID `bson:"trashed,omitempty" json:"-"`

	PublicAccessLevel string `bson:"publicAccesLevel,omitempty" json:"publicAccessLevel,omitempty"`
	Description       string `bson:"description,omitempty" json:"description,omitempty"`

	// Overleaf is where the project's history lives. Only the id is modelled:
	// the rest of what is under there belongs to the history services, and a
	// write here would be this service having an opinion about it.
	Overleaf *Overleaf `bson:"overleaf,omitempty" json:"-"`
}

// Overleaf is the part of a project document the history services own.
type Overleaf struct {
	History History `bson:"history" json:"history"`
}

// History names a project in the history store.
type History struct {
	// ID is the project's name in the history store. Stored as a number by
	// some versions and as a string by others, so it is read as either and
	// used as text.
	ID any `bson:"id,omitempty" json:"id,omitempty"`
}

// HistoryID is what the history services call this project.
//
// Empty means the project has no history yet, which is true of one made
// before anything was written to it. Everything that reads a version has to
// cope with that rather than assume it.
func (p *Project) HistoryID() string {
	if p.Overleaf == nil {
		return ""
	}
	switch id := p.Overleaf.History.ID.(type) {
	case string:
		return id
	case int32:
		return strconv.FormatInt(int64(id), 10)
	case int64:
		return strconv.FormatInt(id, 10)
	case float64:
		return strconv.FormatInt(int64(id), 10)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", id)
	}
}

// AccessFor says what somebody may do with this project.
func (p *Project) AccessFor(userID bson.ObjectID) Access {
	if p.OwnerRef == userID {
		return AccessOwner
	}
	if contains(p.Collaborators, userID) || contains(p.TokenAccessReadWrite, userID) {
		return AccessWrite
	}
	if contains(p.ReadOnly, userID) || contains(p.TokenAccessReadOnly, userID) {
		return AccessRead
	}
	return AccessNone
}

// IsArchivedFor and IsTrashedFor are per person: one collaborator archiving a
// project does not archive it for everybody.
func (p *Project) IsArchivedFor(userID bson.ObjectID) bool { return contains(p.Archived, userID) }
func (p *Project) IsTrashedFor(userID bson.ObjectID) bool  { return contains(p.Trashed, userID) }

func contains(ids []bson.ObjectID, id bson.ObjectID) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

// Store is the projects collection.
type Store struct {
	projects *mongo.Collection
}

// NewStore builds a Store.
func NewStore(db *mongo.Database) *Store {
	return &Store{projects: db.Collection("projects")}
}

// EnsureIndexes creates what listing somebody's projects relies on.
func (s *Store) EnsureIndexes(ctx context.Context) error {
	_, err := s.projects.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "owner_ref", Value: 1}}, Options: options.Index().SetName("owner_ref_1")},
		{Keys: bson.D{{Key: "collaberator_refs", Value: 1}}, Options: options.Index().SetName("collaberator_refs_1")},
		{Keys: bson.D{{Key: "readOnly_refs", Value: 1}}, Options: options.Index().SetName("readOnly_refs_1")},
	})
	if err != nil {
		var cmdErr mongo.CommandError
		if errors.As(err, &cmdErr) && (cmdErr.Code == 85 || cmdErr.Code == 86) {
			return nil
		}
		return err
	}
	return nil
}

// Summary is a project as it appears in a list: enough to draw a row, and no
// file tree.
type Summary struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	OwnerID       string    `json:"ownerId"`
	Access        Access    `json:"access"`
	LastUpdated   time.Time `json:"lastUpdated"`
	LastUpdatedBy string    `json:"lastUpdatedBy,omitempty"`
	Archived      bool      `json:"archived"`
	Trashed       bool      `json:"trashed"`
}

// ListFor reads every project somebody can see.
func (s *Store) ListFor(ctx context.Context, userID bson.ObjectID) ([]Summary, error) {
	filter := bson.M{"$or": []bson.M{
		{"owner_ref": userID},
		{"collaberator_refs": userID},
		{"readOnly_refs": userID},
		{"tokenAccessReadAndWrite_refs": userID},
		{"tokenAccessReadOnly_refs": userID},
	}}
	// Only the fields a list needs: a project's file tree can be large, and
	// fetching a hundred of them to show a hundred names is the difference
	// between a fast list and a slow one.
	projection := bson.M{
		"name": 1, "owner_ref": 1, "lastUpdated": 1, "lastUpdatedBy": 1,
		"collaberator_refs": 1, "readOnly_refs": 1,
		"tokenAccessReadAndWrite_refs": 1, "tokenAccessReadOnly_refs": 1,
		"archived": 1, "trashed": 1,
	}
	cursor, err := s.projects.Find(ctx, filter,
		options.Find().SetProjection(projection).SetSort(bson.D{{Key: "lastUpdated", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	summaries := []Summary{}
	for cursor.Next(ctx) {
		var project Project
		if err := cursor.Decode(&project); err != nil {
			return nil, err
		}
		summary := Summary{
			ID:          project.ID.Hex(),
			Name:        project.Name,
			OwnerID:     project.OwnerRef.Hex(),
			Access:      project.AccessFor(userID),
			LastUpdated: project.LastUpdated,
			Archived:    project.IsArchivedFor(userID),
			Trashed:     project.IsTrashedFor(userID),
		}
		if project.LastUpdatedBy != nil {
			summary.LastUpdatedBy = project.LastUpdatedBy.Hex()
		}
		summaries = append(summaries, summary)
	}
	return summaries, cursor.Err()
}

// Get reads a project somebody may see, and says what they may do with it.
func (s *Store) Get(ctx context.Context, id, userID bson.ObjectID) (*Project, Access, error) {
	var project Project
	err := s.projects.FindOne(ctx, bson.M{"_id": id}).Decode(&project)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, AccessNone, ErrNotFound
	}
	if err != nil {
		return nil, AccessNone, err
	}
	access := project.AccessFor(userID)
	if access == AccessNone {
		// Not ErrForbidden: a project somebody may not see should not be
		// distinguishable from one that does not exist.
		return nil, AccessNone, ErrNotFound
	}
	return &project, access, nil
}

// Create makes an empty project owned by somebody.
func (s *Store) Create(ctx context.Context, ownerID bson.ObjectID, name, compiler string) (*Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Untitled"
	}
	if compiler == "" {
		compiler = "pdflatex"
	}
	now := time.Now().UTC()
	project := &Project{
		ID:          bson.NewObjectID(),
		Name:        name,
		OwnerRef:    ownerID,
		Compiler:    compiler,
		LastUpdated: now,
		RootFolder: []Folder{{
			ID:   bson.NewObjectID(),
			Name: "rootFolder",
		}},
	}
	if _, err := s.projects.InsertOne(ctx, project); err != nil {
		return nil, err
	}
	return project, nil
}

// Rename changes a project's name.
func (s *Store) Rename(ctx context.Context, id bson.ObjectID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("a project needs a name")
	}
	_, err := s.projects.UpdateByID(ctx, id, bson.M{
		"$set": bson.M{"name": name, "lastUpdated": time.Now().UTC()},
	})
	return err
}

// SetArchived and SetTrashed move a project out of somebody's list without
// touching anybody else's.
func (s *Store) SetArchived(ctx context.Context, id, userID bson.ObjectID, archived bool) error {
	return s.toggleMembership(ctx, id, userID, "archived", archived)
}

func (s *Store) SetTrashed(ctx context.Context, id, userID bson.ObjectID, trashed bool) error {
	return s.toggleMembership(ctx, id, userID, "trashed", trashed)
}

func (s *Store) toggleMembership(ctx context.Context, id, userID bson.ObjectID, field string, member bool) error {
	op := "$pull"
	if member {
		op = "$addToSet"
	}
	_, err := s.projects.UpdateByID(ctx, id, bson.M{op: bson.M{field: userID}})
	return err
}

// Delete removes a project for good. Only an owner reaches this.
func (s *Store) Delete(ctx context.Context, id bson.ObjectID) error {
	_, err := s.projects.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// Touch records that somebody changed a project.
func (s *Store) Touch(ctx context.Context, id, userID bson.ObjectID) error {
	_, err := s.projects.UpdateByID(ctx, id, bson.M{
		"$set": bson.M{"lastUpdated": time.Now().UTC(), "lastUpdatedBy": userID},
	})
	return err
}

// GrantAccess puts somebody on one of a project's access lists.
//
// $addToSet rather than $push: granting twice is what a double click is, and
// the same person twice on one list would show as two collaborators.
func (s *Store) GrantAccess(ctx context.Context, id, userID bson.ObjectID, privilege string) error {
	field := "collaberator_refs"
	if privilege == "readOnly" {
		field = "readOnly_refs"
	}
	_, err := s.projects.UpdateByID(ctx, id, bson.M{
		"$addToSet": bson.M{field: userID},
		"$set":      bson.M{"lastUpdated": time.Now().UTC()},
	})
	return err
}

// RemoveAccess takes somebody off every access list a project has.
//
// Every list, including the two token ones. Removing a person from the
// collaborators while a link they have already used still admits them is not a
// removal -- it is a removal that looks like one and is not.
func (s *Store) RemoveAccess(ctx context.Context, id, userID bson.ObjectID) error {
	_, err := s.projects.UpdateByID(ctx, id, bson.M{
		"$pull": bson.M{
			"collaberator_refs":            userID,
			"readOnly_refs":                userID,
			"tokenAccessReadAndWrite_refs": userID,
			"tokenAccessReadOnly_refs":     userID,
		},
		"$set": bson.M{"lastUpdated": time.Now().UTC()},
	})
	return err
}

// SetPublicAccessLevel turns link sharing on or off.
func (s *Store) SetPublicAccessLevel(ctx context.Context, id bson.ObjectID, level string) error {
	_, err := s.projects.UpdateByID(ctx, id, bson.M{
		"$set": bson.M{"publicAccesLevel": level, "lastUpdated": time.Now().UTC()},
	})
	return err
}

// ByID reads a project without asking whether anybody may see it.
//
// Only for a caller that has its own reason to be looking: accepting an
// invitation is the case this exists for, because the whole point is that the
// person does not have access yet and is about to be given it. Everything else
// should use Get, which refuses a project the person cannot see.
func (s *Store) ByID(ctx context.Context, id bson.ObjectID) (*Project, error) {
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
