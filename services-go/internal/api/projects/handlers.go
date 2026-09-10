package projects

import (
	"context"
	"errors"
	"log/slog"
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
	// SeedExampleProject fills it with a worked example instead of one empty
	// file. A blank project is a blank page, which is where somebody who has
	// not written LaTeX before gets stuck.
	SeedExampleProject(ctx context.Context, project *Project, ownerID bson.ObjectID) error
}

// Forgetter is told when a project goes, so that what refers to it can stop.
//
// An interface rather than a call, for the same reason Seeder is: tags are not
// this package's business, and it should not import them to say goodbye.
type Forgetter interface {
	// historyID is the name the history services know the project by, which
	// is not its own id and is only readable from the record that is about to
	// go. It is empty for a project whose history was never started.
	ForgetProject(ctx context.Context, projectID bson.ObjectID, historyID string) error
}

// Service is the projects API.
type Service struct {
	store  *Store
	seeder Seeder
	// forgetters are everything that keeps something of its own about a
	// project: its documents, its chat, its history, the invitations to it.
	// A list because a project is not one thing in one collection, and
	// deleting only the record that names it leaves every byte of the text
	// on disk under an id nothing points at any more.
	forgetters []Forgetter
	log        *slog.Logger
}

// NewService builds it.
func NewService(store *Store, seeder Seeder) *Service {
	return &Service{store: store, seeder: seeder}
}

// OnDelete registers something to tell when a project is deleted.
func (s *Service) OnDelete(forgetters ...Forgetter) {
	s.forgetters = append(s.forgetters, forgetters...)
}

// Logs is where to report what could not be tidied up.
func (s *Service) Logs(log *slog.Logger) {
	s.log = log
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
		// Template is "blank" or "example". Anything else is refused rather
		// than quietly treated as blank: somebody who asked for a starting
		// point and got an empty file would think the feature was broken.
		Template string `json:"template,omitempty"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	switch in.Template {
	case "", "blank", "example":
	default:
		return apierr.BadRequest.WithField("template").
			WithMessage("There is no such template.")
	}
	project, err := s.store.Create(r.Context(), user.ID, in.Name, in.Compiler)
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	if s.seeder != nil {
		seed := s.seeder.SeedNewProject
		if in.Template == "example" {
			seed = s.seeder.SeedExampleProject
		}
		if err := seed(r.Context(), project, user.ID); err != nil {
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
	project, access, err := s.store.Get(r.Context(), id, user.ID)
	if errors.Is(err, ErrNotFound) {
		return apierr.NotFound
	}
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	if !access.CanAdmin() {
		return apierr.Forbidden.WithMessage("Only the owner can delete a project.")
	}
	// What the project owns goes first, and the record that names it last.
	// The other order loses the way back: the history id and the list of
	// documents are in that record, and without it there is nothing left to
	// say which rows belonged to this project rather than to another.
	//
	// One that fails does not stop the others or the delete. The person asked
	// for the project to go; leaving it in place because a chat room could not
	// be tidied would be answering a different question. What is left behind
	// is unreachable rather than visible, and is logged.
	for _, forgetter := range s.forgetters {
		if err := forgetter.ForgetProject(r.Context(), id, project.HistoryID()); err != nil && s.log != nil {
			s.log.Error("something a deleted project owned could not be removed",
				slog.String("projectId", id.Hex()), slog.Any("err", err))
		}
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
