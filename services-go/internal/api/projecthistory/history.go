// Package projecthistory is the public face of a project's history.
//
// The history services hold everything and check nothing: they answer for any
// project id given to them, which is why they listen only inside the
// deployment. This is the part that decides whether the person asking may see
// this project's past, and it is the only way in from a browser.
package projecthistory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/history"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/users"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Restorer is what puts a file back, which is the documents service.
//
// An interface because restoring is writing to a project, and that belongs to
// the package that owns writing to projects rather than to this one.
type Restorer interface {
	UpsertDoc(ctx context.Context, project *projects.Project, path string, lines []string, userID bson.ObjectID, source string) (*projects.Project, error)
	UpsertFile(ctx context.Context, project *projects.Project, path string, content []byte, userID bson.ObjectID, source string) (*projects.Project, error)
}

// Service answers the history endpoints.
type Service struct {
	projects *projects.Store
	users    *users.Store
	history  *history.Client
	restore  Restorer
	baseURL  string
	http     *http.Client
}

// New builds it.
func New(
	projectStore *projects.Store,
	userStore *users.Store,
	histories *history.Client,
	restorer Restorer,
	baseURL string,
) *Service {
	return &Service{
		projects: projectStore,
		users:    userStore,
		history:  histories,
		restore:  restorer,
		baseURL:  strings.TrimRight(baseURL, "/"),
		http:     &http.Client{Timeout: 60 * time.Second},
	}
}

// Updates lists the versions of a project, newest first.
//
// Summarised rather than raw: one person typing produces thousands of
// operations, and a list of those is not a history anybody can read. The
// history service groups them into what looks like a sitting of work.
func (s *Service) Updates(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.readable(r)
	if err != nil {
		return err
	}

	query := url.Values{}
	if before := r.URL.Query().Get("before"); before != "" {
		if _, err := strconv.ParseInt(before, 10, 64); err != nil {
			return apierr.BadRequest.WithField("before").WithMessage("That is not a time.")
		}
		query.Set("before", before)
	}
	query.Set("min_count", "20")

	var answer struct {
		Updates             []json.RawMessage `json:"updates"`
		NextBeforeTimestamp int64             `json:"nextBeforeTimestamp,omitempty"`
	}
	if err := s.call(r.Context(),
		fmt.Sprintf("/project/%s/updates?%s", project.ID.Hex(), query.Encode()),
		&answer); err != nil {
		return err
	}

	return httpapi.JSON(w, http.StatusOK, map[string]any{
		"updates":    answer.Updates,
		"nextBefore": answer.NextBeforeTimestamp,
	})
}

// Diff answers with how one file changed between two versions.
func (s *Service) Diff(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.readable(r)
	if err != nil {
		return err
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		return apierr.BadRequest.WithField("path").WithMessage("A file is required.")
	}
	from, to, err := versionRange(r)
	if err != nil {
		return err
	}

	query := url.Values{}
	query.Set("pathname", path)
	query.Set("from", strconv.Itoa(from))
	query.Set("to", strconv.Itoa(to))

	var answer json.RawMessage
	if err := s.call(r.Context(),
		fmt.Sprintf("/project/%s/diff?%s", project.ID.Hex(), query.Encode()),
		&answer); err != nil {
		return err
	}
	return httpapi.JSON(w, http.StatusOK, answer)
}

// Changes answers with which files changed between two versions.
func (s *Service) Changes(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.readable(r)
	if err != nil {
		return err
	}
	from, to, err := versionRange(r)
	if err != nil {
		return err
	}

	query := url.Values{}
	query.Set("from", strconv.Itoa(from))
	query.Set("to", strconv.Itoa(to))

	var answer json.RawMessage
	if err := s.call(r.Context(),
		fmt.Sprintf("/project/%s/filetree/diff?%s", project.ID.Hex(), query.Encode()),
		&answer); err != nil {
		return err
	}
	return httpapi.JSON(w, http.StatusOK, answer)
}

// Labels lists the named versions of a project.
func (s *Service) Labels(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.readable(r)
	if err != nil {
		return err
	}
	labels, err := s.history.Labels(r.Context(), project.ID.Hex())
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.JSON(w, http.StatusOK, map[string]any{"labels": labels})
}

// CreateLabel gives a version a name.
func (s *Service) CreateLabel(w http.ResponseWriter, r *http.Request) error {
	project, user, err := s.writable(r)
	if err != nil {
		return err
	}
	var in struct {
		Comment string `json:"comment"`
		Version int    `json:"version"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Comment) == "" {
		return apierr.BadRequest.WithField("comment").
			WithMessage("A label needs a name.")
	}
	body, err := json.Marshal(map[string]any{
		"comment": strings.TrimSpace(in.Comment),
		"version": in.Version,
		"user_id": user.ID.Hex(),
	})
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	var created json.RawMessage
	if err := s.post(r.Context(),
		fmt.Sprintf("/project/%s/labels", project.ID.Hex()), body, &created); err != nil {
		return err
	}
	return httpapi.JSON(w, http.StatusCreated, map[string]any{"label": created})
}

// Restore puts a file back as it was at a version.
//
// The old content is written as a new edit, not by rewinding: the history is a
// record of what happened, and undoing a mistake is another thing that
// happened. Anybody looking later sees both, which is what makes it possible
// to undo the undo.
func (s *Service) Restore(w http.ResponseWriter, r *http.Request) error {
	project, user, err := s.writable(r)
	if err != nil {
		return err
	}
	var in struct {
		Path    string `json:"path"`
		Version int    `json:"version"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	path := strings.TrimPrefix(strings.TrimSpace(in.Path), "/")
	if path == "" {
		return apierr.BadRequest.WithField("path").WithMessage("A file is required.")
	}
	if in.Version <= 0 {
		return apierr.BadRequest.WithField("version").
			WithMessage("That is not a version.")
	}

	snapshot, err := s.history.Snapshot(r.Context(), project.ID.Hex(), in.Version)
	if err != nil {
		return apierr.Internal.WithCause(err).
			WithMessage("That version could not be read.")
	}
	file, ok := snapshot[path]
	if !ok {
		return apierr.NotFound.
			WithMessage("That file was not in the project at that version.")
	}

	if file.Content != nil {
		lines := strings.Split(*file.Content, "\n")
		if _, err := s.restore.UpsertDoc(r.Context(), project, path, lines, user.ID, "restore"); err != nil {
			return apierr.Internal.WithCause(err)
		}
		return httpapi.NoContent(w)
	}

	if file.Hash == "" {
		return apierr.NotFound.
			WithMessage("That version of the file cannot be read.")
	}
	body, err := s.history.ReadBlob(r.Context(), project.HistoryID(), file.Hash)
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	defer func() { _ = body.Close() }()
	content, err := io.ReadAll(body)
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	if _, err := s.restore.UpsertFile(r.Context(), project, path, content, user.ID, "restore"); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.NoContent(w)
}

func versionRange(r *http.Request) (int, int, error) {
	from, err := strconv.Atoi(r.URL.Query().Get("from"))
	if err != nil {
		return 0, 0, apierr.BadRequest.WithField("from").
			WithMessage("That is not a version.")
	}
	to, err := strconv.Atoi(r.URL.Query().Get("to"))
	if err != nil {
		return 0, 0, apierr.BadRequest.WithField("to").
			WithMessage("That is not a version.")
	}
	if from > to {
		from, to = to, from
	}
	return from, to, nil
}

func (s *Service) call(ctx context.Context, path string, into any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+path, nil)
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	return s.do(request, into)
}

func (s *Service) post(ctx context.Context, path string, body []byte, into any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+path,
		strings.NewReader(string(body)))
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	request.Header.Set("Content-Type", "application/json")
	return s.do(request, into)
}

func (s *Service) do(request *http.Request, into any) error {
	response, err := s.http.Do(request)
	if err != nil {
		return apierr.Internal.WithCause(err).
			WithMessage("The history is not answering at the moment.")
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode >= 400 {
		return apierr.Internal.WithCause(
			fmt.Errorf("history answered %d", response.StatusCode))
	}
	if into == nil {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(into); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return nil
}

func (s *Service) readable(r *http.Request) (*projects.Project, *users.User, error) {
	return s.project(r, false)
}

func (s *Service) writable(r *http.Request) (*projects.Project, *users.User, error) {
	return s.project(r, true)
}

func (s *Service) project(r *http.Request, needWrite bool) (*projects.Project, *users.User, error) {
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
	if needWrite && !access.CanWrite() {
		return nil, nil, apierr.Forbidden.
			WithMessage("You have read-only access to this project.")
	}
	return project, user, nil
}
