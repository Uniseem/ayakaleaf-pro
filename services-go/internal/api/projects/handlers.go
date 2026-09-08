package projects

import (
	"context"
	"errors"
	"net/http"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Seeder gives a new project its first file.
//
// An empty project is a dead end: there is nothing to open and nothing to
// compile, and the first thing anybody would have to do is work out how to
// make a file. What does the seeding is the service that owns document text,
// which is why this is an interface here rather than a call.
type Seeder interface {
	SeedNewProject(ctx context.Context, project *Project, ownerID bson.ObjectID) error
}

// Service is the projects API.
type Service struct {
	store  *Store
	seeder Seeder
}

// NewService builds it.
func NewService(store *Store, seeder Seeder) *Service {
	return &Service{store: store, seeder: seeder}
}

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
	if s.seeder != nil {
		if err := s.seeder.SeedNewProject(r.Context(), project, user.ID); err != nil {
			// A project that could not be given its first file is not the
			// project that was asked for, and leaving it in somebody's list
			// would leave them with something they cannot use and did not
			// choose to make.
			_ = s.store.Delete(r.Context(), project.ID)
			return apierr.Internal.WithCause(err).
				WithMessage("The project could not be created. Try again.")
		}
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
		// The tree flattened, each entry with its path. The nested form is in
		// the project itself; this is what an editor actually wants, and
		// working it out here means every client does not do it differently.
		"files": project.Entries(),
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
