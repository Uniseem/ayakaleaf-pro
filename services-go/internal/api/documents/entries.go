package documents

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/users"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Making and unmaking the things in a project.
//
// These live here rather than with the rest of the project handlers because
// creating a document is two writes -- the text, and the tree -- and the
// service that knows where text goes is this one. A handler that only knew
// about the tree would leave a project pointing at a document that was never
// written.

// starter is what a new project's first file says.
//
// Enough that pressing compile produces a page rather than an error, and
// little enough that nobody has to delete much before starting.
const starter = "\\documentclass{article}\n" +
	"\n" +
	"\\title{%s}\n" +
	"\\author{}\n" +
	"\\date{\\today}\n" +
	"\n" +
	"\\begin{document}\n" +
	"\\maketitle\n" +
	"\n" +
	"\\end{document}\n"

// SeedNewProject gives a project its first document.
func (s *Service) SeedNewProject(ctx context.Context, project *projects.Project, ownerID bson.ObjectID) error {
	folderID, ok := project.RootFolderID()
	if !ok {
		return errors.New("the project has no root folder")
	}
	docID := bson.NewObjectID()
	lines := strings.Split(strings.Replace(starter, "%s", project.Name, 1), "\n")
	if err := s.storage.Create(ctx, project.ID, docID, lines); err != nil {
		return err
	}
	return s.projects.AddDoc(ctx, project, folderID, projects.DocRef{ID: docID, Name: "main.tex"})
}

// Create makes a document.
func (s *Service) Create(w http.ResponseWriter, r *http.Request) error {
	project, user, err := s.writable(r)
	if err != nil {
		return err
	}
	var in struct {
		Name     string `json:"name"`
		FolderID string `json:"folderId,omitempty"`
		Content  string `json:"content,omitempty"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	name := strings.TrimSpace(in.Name)
	folder, folderID, err := s.folderFor(project, in.FolderID)
	if err != nil {
		return err
	}
	if err := projects.ValidName(name); err != nil {
		return apierr.BadRequest.WithField("name").WithMessage(capitalise(err.Error()) + ".")
	}
	if folder.Taken(name) {
		return apierr.Conflict.WithField("name").
			WithMessage("There is already something called that here.")
	}

	docID := bson.NewObjectID()
	lines := strings.Split(strings.ReplaceAll(in.Content, "\r\n", "\n"), "\n")
	if err := s.storage.Create(r.Context(), project.ID, docID, lines); err != nil {
		return apierr.Internal.WithCause(err)
	}
	if err := s.projects.AddDoc(r.Context(), project, folderID, projects.DocRef{ID: docID, Name: name}); err != nil {
		return apierr.Internal.WithCause(err)
	}
	_ = s.projects.Touch(r.Context(), project.ID, user.ID)

	return httpapi.JSON(w, http.StatusCreated, map[string]any{
		"file": projects.Entry{
			ID: docID, Name: name, Kind: projects.EntryDoc, Parent: folderID,
			Path: pathIn(project, folderID, name),
		},
	})
}

// CreateFolder makes a folder.
func (s *Service) CreateFolder(w http.ResponseWriter, r *http.Request) error {
	project, user, err := s.writable(r)
	if err != nil {
		return err
	}
	var in struct {
		Name     string `json:"name"`
		FolderID string `json:"folderId,omitempty"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	name := strings.TrimSpace(in.Name)
	folder, folderID, err := s.folderFor(project, in.FolderID)
	if err != nil {
		return err
	}
	if err := projects.ValidName(name); err != nil {
		return apierr.BadRequest.WithField("name").WithMessage(capitalise(err.Error()) + ".")
	}
	if folder.Taken(name) {
		return apierr.Conflict.WithField("name").
			WithMessage("There is already something called that here.")
	}

	created := projects.Folder{ID: bson.NewObjectID(), Name: name}
	if err := s.projects.AddFolder(r.Context(), project, folderID, created); err != nil {
		return apierr.Internal.WithCause(err)
	}
	_ = s.projects.Touch(r.Context(), project.ID, user.ID)

	return httpapi.JSON(w, http.StatusCreated, map[string]any{
		"file": projects.Entry{
			ID: created.ID, Name: name, Kind: projects.EntryFolder, Parent: folderID,
			Path: pathIn(project, folderID, name),
		},
	})
}

// Rename changes what something is called.
func (s *Service) Rename(w http.ResponseWriter, r *http.Request) error {
	project, user, err := s.writable(r)
	if err != nil {
		return err
	}
	entry, err := entryIn(project, r.PathValue("entryId"))
	if err != nil {
		return err
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	name := strings.TrimSpace(in.Name)
	if err := projects.ValidName(name); err != nil {
		return apierr.BadRequest.WithField("name").WithMessage(capitalise(err.Error()) + ".")
	}
	folder, ok := project.FolderByID(entry.Parent)
	if ok && !strings.EqualFold(entry.Name, name) && folder.Taken(name) {
		return apierr.Conflict.WithField("name").
			WithMessage("There is already something called that here.")
	}
	if err := s.projects.RenameEntry(r.Context(), project, entry.ID, name); err != nil {
		return apierr.Internal.WithCause(err)
	}
	_ = s.projects.Touch(r.Context(), project.ID, user.ID)
	return httpapi.NoContent(w)
}

// Delete removes something from the project.
func (s *Service) Delete(w http.ResponseWriter, r *http.Request) error {
	project, user, err := s.writable(r)
	if err != nil {
		return err
	}
	entry, err := entryIn(project, r.PathValue("entryId"))
	if err != nil {
		return err
	}
	if err := s.projects.RemoveEntry(r.Context(), project, entry.ID); err != nil {
		return apierr.Internal.WithCause(err)
	}
	if entry.Kind == projects.EntryDoc {
		// The tree is what a person sees, so it is updated first, and this
		// failing does not fail the request: what is left behind is a row
		// nothing points at.
		_ = s.storage.Delete(r.Context(), project.ID, entry.ID, entry.Name)
	}
	_ = s.projects.Touch(r.Context(), project.ID, user.ID)
	return httpapi.NoContent(w)
}

// SetRootDoc chooses the document a compile starts from.
func (s *Service) SetRootDoc(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.writable(r)
	if err != nil {
		return err
	}
	var in struct {
		DocID string `json:"docId"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	entry, err := entryIn(project, in.DocID)
	if err != nil {
		return err
	}
	if entry.Kind != projects.EntryDoc {
		return apierr.BadRequest.WithField("docId").
			WithMessage("Only a document can be the one that gets compiled.")
	}
	if err := s.projects.SetRootDoc(r.Context(), project.ID, entry.ID); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.NoContent(w)
}

// writable is the check the handlers above share: the project exists, this
// person may see it, and they may change it.
func (s *Service) writable(r *http.Request) (*projects.Project, *users.User, error) {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return nil, nil, err
	}
	id, err := bson.ObjectIDFromHex(r.PathValue("id"))
	if err != nil {
		return nil, nil, apierr.NotFound
	}
	project, access, err := s.projects.Get(r.Context(), id, user.ID)
	if errors.Is(err, projects.ErrNotFound) {
		return nil, nil, apierr.NotFound
	}
	if err != nil {
		return nil, nil, apierr.Internal.WithCause(err)
	}
	if !access.CanWrite() {
		return nil, nil, apierr.Forbidden.WithMessage("You have read-only access to this project.")
	}
	return project, user, nil
}

// folderFor reads the folder a new thing goes into, defaulting to the root.
func (s *Service) folderFor(project *projects.Project, raw string) (projects.Folder, bson.ObjectID, error) {
	if strings.TrimSpace(raw) == "" {
		id, ok := project.RootFolderID()
		if !ok {
			return projects.Folder{}, bson.ObjectID{}, apierr.Internal.
				WithMessage("This project has no root folder.")
		}
		folder, _ := project.FolderByID(id)
		return folder, id, nil
	}
	id, err := bson.ObjectIDFromHex(raw)
	if err != nil {
		return projects.Folder{}, bson.ObjectID{}, apierr.BadRequest.WithField("folderId").
			WithMessage("That is not a folder in this project.")
	}
	folder, ok := project.FolderByID(id)
	if !ok {
		return projects.Folder{}, bson.ObjectID{}, apierr.BadRequest.WithField("folderId").
			WithMessage("That is not a folder in this project.")
	}
	return folder, id, nil
}

// entryIn reads an id and finds what it names in the project.
func entryIn(project *projects.Project, raw string) (projects.Entry, error) {
	id, err := bson.ObjectIDFromHex(raw)
	if err != nil {
		return projects.Entry{}, apierr.NotFound
	}
	entry, ok := project.Find(id)
	if !ok {
		return projects.Entry{}, apierr.NotFound
	}
	return entry, nil
}

// pathIn is where something new will be, for the answer.
func pathIn(project *projects.Project, folderID bson.ObjectID, name string) string {
	if root, ok := project.RootFolderID(); ok && root == folderID {
		return name
	}
	for _, entry := range project.Entries() {
		if entry.ID == folderID {
			return entry.Path + "/" + name
		}
	}
	return name
}

func capitalise(text string) string {
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}
