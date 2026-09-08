package compile

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/documents"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/history"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Limits is what the settings say a compile may cost.
type Limits interface {
	CompileTimeout() int
	DefaultImageName() string
}

// Service is the compile API.
type Service struct {
	projects  *projects.Store
	documents *documents.Client
	clsi      *Client
	limits    Limits
	// history is asked where a project's binary files are. They are stored as
	// history blobs, addressed by the hash of their contents, so the answer
	// depends on the project's history id and not on the project's own.
	history *history.Client
}

// NewService builds it.
func NewService(
	projectStore *projects.Store,
	docs *documents.Client,
	clsi *Client,
	limits Limits,
	histories *history.Client,
) *Service {
	return &Service{
		projects:  projectStore,
		documents: docs,
		clsi:      clsi,
		limits:    limits,
		history:   histories,
	}
}

// Compile builds a project's PDF.
//
// Read access is enough. Somebody who may read a project may already read
// every character of it, so refusing them a PDF of what they can see would
// protect nothing and would stop a shared read-only link from being useful.
func (s *Service) Compile(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	id, err := projectID(r)
	if err != nil {
		return err
	}
	project, _, err := s.projects.Get(r.Context(), id, user.ID)
	if errors.Is(err, projects.ErrNotFound) {
		return apierr.NotFound
	}
	if err != nil {
		return apierr.Internal.WithCause(err)
	}

	var in Request
	if r.ContentLength > 0 {
		if err := httpapi.Decode(r, &in); err != nil {
			return err
		}
	}

	resources, rootPath, err := s.resourcesFor(r, project, in.RootDocID)
	if err != nil {
		return err
	}

	result, err := s.clsi.Run(r.Context(), id, user.ID, resources, Options{
		Compiler:         orDefault(project.Compiler, "pdflatex"),
		Timeout:          s.limits.CompileTimeout(),
		ImageName:        orDefault(project.ImageName, s.limits.DefaultImageName()),
		Draft:            in.Draft,
		StopOnFirstError: in.StopOnFirstError,
		Incremental:      in.Incremental,
		RootResourcePath: rootPath,
	})
	if err != nil {
		return apierr.Internal.WithCause(err).
			WithMessage("The compiler could not be reached. Try again in a moment.")
	}
	return httpapi.JSON(w, http.StatusOK, result)
}

// Stop cancels a compile.
func (s *Service) Stop(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	id, err := projectID(r)
	if err != nil {
		return err
	}
	if _, _, err := s.projects.Get(r.Context(), id, user.ID); err != nil {
		return apierr.NotFound
	}
	if err := s.clsi.Stop(r.Context(), id, user.ID); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.NoContent(w)
}

// resourcesFor turns a project's tree into the list clsi reads.
//
// Documents are sent as text because they are what changes; binary files are
// sent as a URL for clsi to fetch instead, because moving a project's images
// through this process on every compile would be the slowest thing it does.
func (s *Service) resourcesFor(
	r *http.Request,
	project *projects.Project,
	rootDocID string,
) ([]Resource, string, error) {
	wanted := project.RootDocID
	if rootDocID != "" {
		parsed, err := bson.ObjectIDFromHex(rootDocID)
		if err != nil {
			return nil, "", apierr.BadRequest.WithField("rootDocId").
				WithMessage("That is not a document in this project.")
		}
		if _, ok := project.Find(parsed); !ok {
			return nil, "", apierr.BadRequest.WithField("rootDocId").
				WithMessage("That is not a document in this project.")
		}
		wanted = parsed
	}

	// Everything anybody has typed is in document-updater and not yet in
	// storage, so the text is read from there: a compile of the stored copy
	// would silently leave out the last few minutes of work.
	historyID := project.HistoryID()
	resources := []Resource{}
	rootPath := ""
	var failure error
	project.Walk(func(entry projects.Entry) {
		if failure != nil {
			return
		}
		switch entry.Kind {
		case projects.EntryDoc:
			doc, err := s.documents.Get(r.Context(), project.ID, entry.ID)
			if errors.Is(err, documents.ErrNotFound) {
				return
			}
			if err != nil {
				failure = apierr.Internal.WithCause(err)
				return
			}
			resources = append(resources, Resource{
				Path:    entry.Path,
				Content: strings.Join(doc.Lines, "\n"),
			})
			if entry.ID == wanted {
				rootPath = entry.Path
			}
		case projects.EntryFile:
			if entry.Hash == "" || historyID == "" {
				// A file whose bytes were never written, or a project with no
				// history yet. Sending the compiler an address that answers
				// nothing would fail the whole compile over one image.
				return
			}
			resources = append(resources, Resource{
				Path: entry.Path,
				URL:  s.history.BlobURL(historyID, entry.Hash),
			})
		}
	})
	if failure != nil {
		return nil, "", failure
	}

	if rootPath == "" {
		// No root document set, or one that is no longer in the tree. The
		// first .tex file is a better guess than failing, and is what somebody
		// who has just uploaded a project expects to happen.
		rootPath = firstTeXFile(resources)
	}
	if rootPath == "" {
		return nil, "", apierr.BadRequest.
			WithMessage("This project has no .tex file to compile.")
	}
	return resources, rootPath, nil
}

func firstTeXFile(resources []Resource) string {
	best := ""
	for _, resource := range resources {
		if !strings.HasSuffix(strings.ToLower(resource.Path), ".tex") {
			continue
		}
		// One at the top of the project beats one inside a folder, and
		// main.tex beats everything.
		if strings.EqualFold(resource.Path, "main.tex") {
			return resource.Path
		}
		if best == "" || depth(resource.Path) < depth(best) {
			best = resource.Path
		}
	}
	return best
}

func depth(path string) int { return strings.Count(path, "/") }

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func projectID(r *http.Request) (bson.ObjectID, error) {
	id, err := bson.ObjectIDFromHex(r.PathValue("id"))
	if err != nil {
		return bson.ObjectID{}, apierr.NotFound
	}
	return id, nil
}
