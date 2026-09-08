package projects

import (
	"context"
	"errors"
	"net/http"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Service is the projects API.
type Service struct {
	store *Store
}

// NewService builds it.
func NewService(store *Store) *Service { return &Service{store: store} }

// List answers with every project somebody can see.
func (s *Service) List(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	summaries, err := s.store.ListFor(r.Context(), user.ID)
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.JSON(w, http.StatusOK, map[string]any{"projects": summaries})
}

// Create makes a project.
func (s *Service) Create(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	var in struct {
		Name     string `json:"name"`
		Compiler string `json:"compiler,omitempty"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	project, err := s.store.Create(r.Context(), user.ID, in.Name, in.Compiler)
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.JSON(w, http.StatusCreated, map[string]any{
		"project": Summary{
			ID:          project.ID.Hex(),
			Name:        project.Name,
			OwnerID:     project.OwnerRef.Hex(),
			Access:      AccessOwner,
			LastUpdated: project.LastUpdated,
		},
	})
}

// Get answers with one project and what the person asking may do with it.
func (s *Service) Get(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	id, err := projectID(r)
	if err != nil {
		return err
	}
	project, access, err := s.store.Get(r.Context(), id, user.ID)
	if errors.Is(err, ErrNotFound) {
		return apierr.NotFound
	}
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.JSON(w, http.StatusOK, map[string]any{
		"project": project,
		"access":  access,
	})
}

// Rename changes a project's name.
func (s *Service) Rename(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.writable(r)
	if err != nil {
		return err
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	if err := s.store.Rename(r.Context(), project.ID, in.Name); err != nil {
		return apierr.BadRequest.WithCause(err).WithField("name").
			WithMessage("A project needs a name.")
	}
	return httpapi.NoContent(w)
}

// Archive and Trash move a project out of one person's list. They are per
// person: one collaborator archiving a project does not archive it for
// everybody, which is why they take the reader's id and not just the project.
func (s *Service) Archive(w http.ResponseWriter, r *http.Request) error {
	return s.setMembership(w, r, s.store.SetArchived)
}

func (s *Service) Trash(w http.ResponseWriter, r *http.Request) error {
	return s.setMembership(w, r, s.store.SetTrashed)
}

type membershipSetter func(ctx context.Context, id, userID bson.ObjectID, on bool) error

func (s *Service) setMembership(w http.ResponseWriter, r *http.Request, set membershipSetter) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	id, err := projectID(r)
	if err != nil {
		return err
	}
	// Read it first, so that this cannot mark a project nobody is allowed to
	// see -- the id alone is not permission.
	if _, _, err := s.store.Get(r.Context(), id, user.ID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierr.NotFound
		}
		return apierr.Internal.WithCause(err)
	}
	var in struct {
		On bool `json:"on"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	if err := set(r.Context(), id, user.ID, in.On); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.NoContent(w)
}

// Delete removes a project. Only its owner may.
func (s *Service) Delete(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	id, err := projectID(r)
	if err != nil {
		return err
	}
	_, access, err := s.store.Get(r.Context(), id, user.ID)
	if errors.Is(err, ErrNotFound) {
		return apierr.NotFound
	}
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	if !access.CanAdmin() {
		return apierr.Forbidden.WithMessage("Only the owner can delete a project.")
	}
	if err := s.store.Delete(r.Context(), id); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.NoContent(w)
}

// writable reads the project and refuses unless the person may change it.
func (s *Service) writable(r *http.Request) (*Project, Access, error) {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return nil, AccessNone, err
	}
	id, err := projectID(r)
	if err != nil {
		return nil, AccessNone, err
	}
	project, access, err := s.store.Get(r.Context(), id, user.ID)
	if errors.Is(err, ErrNotFound) {
		return nil, AccessNone, apierr.NotFound
	}
	if err != nil {
		return nil, AccessNone, apierr.Internal.WithCause(err)
	}
	if !access.CanWrite() {
		return nil, access, apierr.Forbidden.WithMessage("You have read-only access to this project.")
	}
	return project, access, nil
}

// projectID reads the id out of the path.
func projectID(r *http.Request) (bson.ObjectID, error) {
	raw := r.PathValue("id")
	id, err := bson.ObjectIDFromHex(raw)
	if err != nil {
		return bson.ObjectID{}, apierr.NotFound
	}
	return id, nil
}
