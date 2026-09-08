package documents

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Service is the documents API.
type Service struct {
	projects *projects.Store
	client   *Client
	storage  *Storage
}

// NewService builds it.
func NewService(projectStore *projects.Store, client *Client, storage *Storage) *Service {
	return &Service{projects: projectStore, client: client, storage: storage}
}

// Get reads a document.
//
// This is how the editor loads a file it is about to show. Once it is shown,
// changes travel over the socket instead, so this is the only slow read: after
// it, nothing in the editing loop comes through here.
func (s *Service) Get(w http.ResponseWriter, r *http.Request) error {
	project, entry, _, err := s.resolve(r)
	if err != nil {
		return err
	}
	doc, err := s.client.Get(r.Context(), project.ID, entry.ID)
	if errors.Is(err, ErrNotFound) {
		return apierr.NotFound
	}
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.JSON(w, http.StatusOK, map[string]any{
		"id":      doc.ID,
		"name":    entry.Name,
		"path":    entry.Path,
		"lines":   doc.Lines,
		"version": doc.Version,
		"ranges":  doc.Ranges,
	})
}

// SetContent replaces a document's text in one go.
//
// Not what typing does -- that is the socket -- but what everything else does:
// an upload overwriting a file, a template being applied, a change made by a
// script. It goes through document-updater rather than to storage so that
// anybody with the file open sees it change instead of quietly working on a
// copy that is about to be overwritten.
func (s *Service) SetContent(w http.ResponseWriter, r *http.Request) error {
	project, entry, access, err := s.resolve(r)
	if err != nil {
		return err
	}
	if !access.CanWrite() {
		return apierr.Forbidden.WithMessage("You have read-only access to this project.")
	}
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}

	var in struct {
		Content string   `json:"content"`
		Lines   []string `json:"lines"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	lines := in.Lines
	if lines == nil {
		// A whole file as one string is what an editor holds and what an
		// upload produces; splitting it here means neither has to know that
		// the storage format is lines.
		lines = strings.Split(strings.ReplaceAll(in.Content, "\r\n", "\n"), "\n")
	}

	if err := s.client.SetContent(r.Context(), project.ID, entry.ID, user.ID, lines, "api"); err != nil {
		return apierr.Internal.WithCause(err)
	}
	if err := s.projects.Touch(r.Context(), project.ID, user.ID); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.NoContent(w)
}

// resolve is the check every handler here starts with: who is asking, whether
// they may see the project, and whether the document is really in it.
//
// That last part is why a document id alone is never enough. Without it, an id
// belonging to somebody else's project could be read through a project of your
// own, and the access check would have passed.
func (s *Service) resolve(r *http.Request) (*projects.Project, projects.Entry, projects.Access, error) {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return nil, projects.Entry{}, projects.AccessNone, err
	}
	projectID, err := bson.ObjectIDFromHex(r.PathValue("id"))
	if err != nil {
		return nil, projects.Entry{}, projects.AccessNone, apierr.NotFound
	}
	docID, err := bson.ObjectIDFromHex(r.PathValue("docId"))
	if err != nil {
		return nil, projects.Entry{}, projects.AccessNone, apierr.NotFound
	}
	project, access, err := s.projects.Get(r.Context(), projectID, user.ID)
	if errors.Is(err, projects.ErrNotFound) {
		return nil, projects.Entry{}, projects.AccessNone, apierr.NotFound
	}
	if err != nil {
		return nil, projects.Entry{}, projects.AccessNone, apierr.Internal.WithCause(err)
	}
	entry, ok := project.Find(docID)
	if !ok || entry.Kind != projects.EntryDoc {
		return nil, projects.Entry{}, projects.AccessNone, apierr.NotFound
	}
	return project, entry, access, nil
}
