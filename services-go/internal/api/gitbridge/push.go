package gitbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/documents"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/users"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Receiving a push.
//
// The bridge does not send the files: it sends a list of them and an address
// for each one that changed, and expects an answer straight away. The work
// happens afterwards and the result is posted back, because a push of a large
// project takes longer than the git client will wait on one request.

// maxPushedFile is the most one file in a push may be. A repository can hold
// anything; a project here cannot.
const maxPushedFile = 100 << 20

// pushWork is how long the work after the answer may take before it is
// abandoned. Long, because it is downloading and writing a whole project, and
// bounded, because otherwise a stuck download holds a lock on the project for
// ever.
const pushWork = 15 * time.Minute

type pushedFile struct {
	Name string `json:"name"`
	// URL is set only for a file that changed. One without it is a file the
	// project already has and should keep.
	URL string `json:"url"`
}

type pushRequest struct {
	LatestVerID int          `json:"latestVerId"`
	Files       []pushedFile `json:"files"`
	PostbackURL string       `json:"postbackUrl"`
}

// PostSnapshot takes a push.
func (s *Service) PostSnapshot(w http.ResponseWriter, r *http.Request) error {
	project, user, err := s.authorise(r, true)
	if err != nil {
		return err
	}

	var in pushRequest
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}

	current, err := s.history.LatestVersion(r.Context(), project.ID.Hex())
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	if in.LatestVerID != current.Version {
		// Somebody changed the project since this push was worked out. The
		// bridge pulls and asks the person to try again, which is the same
		// thing git would have told them.
		return httpapi.JSON(w, http.StatusConflict, map[string]any{
			"status": 409, "code": "outOfDate", "message": "Out of Date",
		})
	}

	// Answered before the work, because the work is a whole project and the
	// git client on the other end is waiting.
	if err := httpapi.JSON(w, http.StatusAccepted, map[string]any{
		"status": 202, "code": "accepted", "message": "Accepted",
	}); err != nil {
		return err
	}

	go s.applyPush(project.ID, user, in)
	return nil
}

// applyPush writes a pushed set of files into a project and says how it went.
//
// It runs after the answer, so nothing here can report a failure by returning
// one: everything ends in a postback, including the failures, because the
// bridge is holding a git client open until it hears something.
func (s *Service) applyPush(projectID bson.ObjectID, user *users.User, in pushRequest) {
	ctx, done := context.WithTimeout(context.Background(), pushWork)
	defer done()

	// Checked before anything is written, so a push with one bad path changes
	// nothing rather than half of the project.
	var refused []map[string]any
	for _, file := range in.Files {
		if state := validPath(file.Name); state != "" {
			refused = append(refused, map[string]any{"file": file.Name, "state": state})
		}
	}
	if len(refused) > 0 {
		s.postback(ctx, in.PostbackURL, map[string]any{
			"code": "invalidFiles", "errors": refused,
		})
		return
	}

	project, _, err := s.projects.Get(ctx, projectID, user.ID)
	if err != nil {
		s.failedPush(ctx, in.PostbackURL, projectID, err)
		return
	}

	pushed := make(map[string]bool, len(in.Files))
	for _, file := range in.Files {
		pushed[file.Name] = true
	}

	for _, file := range in.Files {
		if file.URL == "" {
			// Unchanged. The bridge only serves the ones it says it changed,
			// and asking for the rest would be downloading the project back
			// from itself.
			continue
		}
		content, err := s.download(ctx, file.URL)
		if err != nil {
			s.failedPush(ctx, in.PostbackURL, projectID, err)
			return
		}
		project, err = s.write(ctx, project, file.Name, content, user.ID)
		if err != nil {
			s.failedPush(ctx, in.PostbackURL, projectID, err)
			return
		}
	}

	// What the push no longer has, this project should no longer have. Taken
	// from the tree after the writes above, so a file that was just replaced
	// is not deleted for having been a different kind before.
	for _, entry := range project.Entries() {
		if entry.Kind == projects.EntryFolder || pushed[entry.Path] {
			continue
		}
		project, err = s.documents.DeletePath(ctx, project, entry.Path, user.ID, "git-bridge")
		if err != nil {
			s.failedPush(ctx, in.PostbackURL, projectID, err)
			return
		}
	}

	version, err := s.history.LatestVersion(ctx, projectID.Hex())
	if err != nil {
		s.failedPush(ctx, in.PostbackURL, projectID, err)
		return
	}
	s.postback(ctx, in.PostbackURL, map[string]any{
		"code": "upToDate", "latestVerId": version.Version,
	})
}

// write puts one pushed file into the project, as text or as bytes.
func (s *Service) write(
	ctx context.Context,
	project *projects.Project,
	path string,
	content []byte,
	userID bson.ObjectID,
) (*projects.Project, error) {
	// A path that already holds a binary file keeps holding one, whatever the
	// bytes look like now: a PDF that happens to start with readable text is
	// still a PDF, and turning it into a document would corrupt it.
	if existing, ok := project.FindPath(path); ok && existing.Kind == projects.EntryFile {
		return s.documents.UpsertFile(ctx, project, path, content, userID, "git-bridge")
	}
	if documents.LooksLikeText(path, content) {
		lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
		return s.documents.UpsertDoc(ctx, project, path, lines, userID, "git-bridge")
	}
	return s.documents.UpsertFile(ctx, project, path, content, userID, "git-bridge")
}

// download fetches one file from the bridge.
//
// The addresses come from the bridge and are only good until the postback, so
// this is the one moment the file can be read.
func (s *Service) download(ctx context.Context, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := s.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 300 {
		return nil, fmt.Errorf("the git bridge answered %s for a pushed file", response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, maxPushedFile))
}

// failedPush says something went wrong, in the one way the bridge understands.
func (s *Service) failedPush(ctx context.Context, url string, projectID bson.ObjectID, err error) {
	if s.log != nil {
		s.log.Error("a git push could not be applied",
			slog.String("projectId", projectID.Hex()), slog.Any("err", err))
	}
	s.postback(ctx, url, map[string]any{
		"code": "error", "message": "Unexpected Error",
	})
}

func (s *Service) postback(ctx context.Context, url string, body map[string]any) {
	if url == "" {
		return
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.http.Do(request)
	if err != nil {
		if s.log != nil {
			s.log.Warn("could not tell the git bridge how a push went", slog.Any("err", err))
		}
		return
	}
	_ = response.Body.Close()
}

// copyTo is io.Copy, named so the one place that streams a file reads as what
// it is.
func copyTo(w io.Writer, r io.Reader) (int64, error) { return io.Copy(w, r) }
