// Package gitbridge is the API the git bridge talks to.
//
// Cloning and pushing a project over git is done by a separate service, which
// speaks git on one side and this on the other. What it needs is small and
// fixed: the version a project is at, the versions somebody named, the files
// at a version, and a way to hand back a new set of files. Those four things
// are this package, and they are the same four the service has asked for since
// it was written, which is why the shapes here are not the ones the rest of
// the API uses.
//
// Everything it sends arrives with a personal access token rather than a
// session, because a git client has one place to put a secret and it is the
// password prompt.
package gitbridge

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/documents"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/history"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/tokens"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/users"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Settings is what an administrator decided about git access.
type Settings interface {
	GitEnabled() bool
}

// Service is the git bridge API.
type Service struct {
	log       *slog.Logger
	projects  *projects.Store
	users     *users.Store
	tokens    *tokens.Store
	documents *documents.Service
	history   *history.Client
	settings  Settings

	// base is the address the bridge reaches this service at. Attachment
	// links are built from it, and it is the deployment's answer rather than
	// the site's public address: the bridge is inside the deployment and may
	// not be able to resolve the name people type into a browser.
	base string
	// secret signs those attachment links. They carry no session and no
	// token, so the signature is the whole of what makes them safe to hand
	// out.
	secret []byte

	http *http.Client
}

// Options is what a Service needs.
type Options struct {
	Log       *slog.Logger
	Projects  *projects.Store
	Users     *users.Store
	Tokens    *tokens.Store
	Documents *documents.Service
	History   *history.Client
	Settings  Settings
	BaseURL   string
	Secret    string
}

// NewService builds it.
func NewService(opts Options) *Service {
	return &Service{
		log:       opts.Log,
		projects:  opts.Projects,
		users:     opts.Users,
		tokens:    opts.Tokens,
		documents: opts.Documents,
		history:   opts.History,
		settings:  opts.Settings,
		base:      strings.TrimRight(opts.BaseURL, "/"),
		secret:    []byte(opts.Secret),
		http:      &http.Client{Timeout: 5 * time.Minute},
	}
}

// TokenInfo says whether a token is good.
//
// The bridge asks this before it does anything else, because a git client
// gives it one chance to explain a refusal and that chance is the password
// prompt. Answering here means somebody typing the wrong token is told so,
// rather than watching a clone fail with nothing.
func (s *Service) TokenInfo(w http.ResponseWriter, r *http.Request) error {
	presented, ok := bearerOf(r)
	if !ok {
		return refuse(w, "no authentication given", "token_malformed")
	}
	token, err := s.tokens.Verify(r.Context(), presented)
	if errors.Is(err, tokens.ErrExpired) {
		return refuse(w, "the token has expired", "token_expired")
	}
	if errors.Is(err, tokens.ErrNotFound) {
		return refuse(w, "invalid token", "token_invalid")
	}
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.JSON(w, http.StatusOK, map[string]any{
		"accessToken":          token.Partial + "...",
		"accessTokenExpiresAt": token.ExpiresAt,
		"scope":                token.Scope,
	})
}

// GetDoc is where a project's history has got to, and who put it there.
func (s *Service) GetDoc(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.authorise(r, false)
	if err != nil {
		return err
	}
	version, err := s.history.LatestVersion(r.Context(), project.ID.Hex())
	if err != nil {
		return apierr.Internal.WithCause(err)
	}

	// The bridge writes this into the commit it makes, so an unknown author is
	// answered as unknown rather than left out: a commit needs one.
	email, name := "unknown", "unknown"
	if len(version.V2Authors) > 0 {
		if id, err := bson.ObjectIDFromHex(version.V2Authors[0]); err == nil {
			if author, err := s.users.ByID(r.Context(), id); err == nil {
				email = author.Email
				if full := strings.TrimSpace(author.FirstName + " " + author.LastName); full != "" {
					name = full
				} else {
					name = author.Email
				}
			}
		}
	}
	at := version.Timestamp
	if at.IsZero() {
		at = time.Now()
	}

	return httpapi.JSON(w, http.StatusOK, map[string]any{
		"latestVerId": version.Version,
		"latestVerAt": at.UTC().Format(time.RFC3339),
		"latestVerBy": map[string]any{"email": email, "name": name},
	})
}

// GetSavedVers is the versions somebody named.
func (s *Service) GetSavedVers(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.authorise(r, false)
	if err != nil {
		return err
	}
	labels, err := s.history.Labels(r.Context(), project.ID.Hex())
	if err != nil {
		return apierr.Internal.WithCause(err)
	}

	saved := make([]map[string]any, 0, len(labels))
	for _, label := range labels {
		email, name := "unknown", "unknown"
		if label.UserID != nil {
			if id, err := bson.ObjectIDFromHex(*label.UserID); err == nil {
				if author, err := s.users.ByID(r.Context(), id); err == nil {
					email = author.Email
					if full := strings.TrimSpace(author.FirstName + " " + author.LastName); full != "" {
						name = full
					} else {
						name = author.Email
					}
				}
			}
		}
		saved = append(saved, map[string]any{
			"versionId": label.Version,
			"comment":   label.Comment,
			"user":      map[string]any{"email": email, "name": name},
			"createdAt": label.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return httpapi.JSON(w, http.StatusOK, saved)
}

// GetSnapshot is every file a project had at a version.
//
// Text comes back as text, because that is what a commit is made of. Anything
// else comes back as a link the bridge fetches, so a project full of images
// does not travel through this process on its way to a clone.
func (s *Service) GetSnapshot(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.authorise(r, false)
	if err != nil {
		return err
	}
	version, err := strconv.Atoi(r.PathValue("version"))
	if err != nil {
		return apierr.BadRequest.WithMessage("That is not a version.")
	}

	files, err := s.history.Snapshot(r.Context(), project.ID.Hex(), version)
	if errors.Is(err, history.ErrNotFound) {
		return apierr.NotFound.WithMessage("There is no such version of this project.")
	}
	if err != nil {
		return apierr.Internal.WithCause(err)
	}

	historyID := project.HistoryID()
	srcs := [][]string{}
	atts := [][]string{}
	for path, file := range files {
		if bad := validPath(path); bad != "" {
			// A path the history holds but a filesystem will not take. Better
			// to refuse the whole snapshot than to hand the bridge something
			// it will write outside the repository.
			return apierr.BadRequest.WithMessage("This project has a file that cannot be checked out: " + path)
		}
		if file.IsText() {
			srcs = append(srcs, []string{*file.Content, path})
			continue
		}
		if file.Hash == "" || historyID == "" {
			continue
		}
		atts = append(atts, []string{s.attachmentURL(historyID, file.Hash), path})
	}

	return httpapi.JSON(w, http.StatusOK, map[string]any{"srcs": srcs, "atts": atts})
}

// Blob hands over the bytes of one file.
//
// The link is signed and short-lived rather than authenticated, because the
// bridge fetches it as a plain download with no token: what makes it safe is
// that only this service could have produced the address, and only recently.
func (s *Service) Blob(w http.ResponseWriter, r *http.Request) error {
	historyID := r.PathValue("historyId")
	hash := r.PathValue("hash")
	if !s.validSignature(historyID, hash, r.URL.Query().Get("expires"), r.URL.Query().Get("token")) {
		return apierr.Forbidden.WithMessage("That link is not valid any more.")
	}

	body, err := s.history.ReadBlob(r.Context(), historyID, hash)
	if errors.Is(err, history.ErrNotFound) {
		return apierr.NotFound
	}
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	defer func() { _ = body.Close() }()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = copyTo(w, body)
	return nil
}

// --- what makes a request this person's ------------------------------------

// authorise reads the token a request carries and finds the project it is
// about.
//
// Access is checked the same way it is for a browser: the token stands for a
// person, and that person's access to the project is what decides. A token is
// not a way past the check.
func (s *Service) authorise(r *http.Request, write bool) (*projects.Project, *users.User, error) {
	if s.settings != nil && !s.settings.GitEnabled() {
		return nil, nil, apierr.Forbidden.WithMessage("Git access is turned off on this site.")
	}
	presented, ok := bearerOf(r)
	if !ok {
		return nil, nil, unauthorised("no authentication given")
	}
	token, err := s.tokens.Verify(r.Context(), presented)
	if errors.Is(err, tokens.ErrNotFound) || errors.Is(err, tokens.ErrExpired) {
		return nil, nil, unauthorised("invalid token")
	}
	if err != nil {
		return nil, nil, apierr.Internal.WithCause(err)
	}

	user, err := s.users.ByID(r.Context(), token.UserID)
	if err != nil {
		return nil, nil, unauthorised("invalid token")
	}

	projectID, err := bson.ObjectIDFromHex(r.PathValue("projectId"))
	if err != nil {
		return nil, nil, apierr.NotFound
	}
	project, access, err := s.projects.Get(r.Context(), projectID, user.ID)
	if errors.Is(err, projects.ErrNotFound) {
		return nil, nil, apierr.NotFound
	}
	if err != nil {
		return nil, nil, apierr.Internal.WithCause(err)
	}
	if write && !access.CanWrite() {
		return nil, nil, apierr.Forbidden.WithMessage("You have read-only access to this project.")
	}
	return project, user, nil
}

// refuse writes a refusal in the shape the bridge reads.
//
// Not the shape the rest of the API uses: the bridge looks for error_code at
// the top level and decides from it whether to tell somebody their token has
// expired or was never right, and that message is the only thing a git client
// will show them.
func refuse(w http.ResponseWriter, description, code string) error {
	return httpapi.JSON(w, http.StatusUnauthorized, map[string]any{
		"error":             "Unauthorized",
		"error_description": "Unauthorized request: " + description,
		"error_code":        code,
	})
}

// unauthorised is a refusal on the endpoints the bridge calls with a token it
// has already had checked.
func unauthorised(description string) *apierr.Error {
	return apierr.Unauthorised.WithMessage("Unauthorized request: " + description)
}

func bearerOf(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if len(header) < 8 || !strings.EqualFold(header[:7], "bearer ") {
		return "", false
	}
	token := strings.TrimSpace(header[7:])
	return token, token != ""
}

// --- signed attachment links -----------------------------------------------

// attachmentLife is how long one of these links works for. Long enough for a
// clone of a project full of images, short enough that a link found in a log
// later is worth nothing.
const attachmentLife = 30 * time.Minute

func (s *Service) attachmentURL(historyID, hash string) string {
	expires := strconv.FormatInt(time.Now().Add(attachmentLife).Unix(), 10)
	return fmt.Sprintf("%s/api/v0/blobs/%s/%s?expires=%s&token=%s",
		s.base, historyID, hash, expires, s.signature(historyID, hash, expires))
}

func (s *Service) signature(historyID, hash, expires string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(historyID + ":" + hash + ":" + expires))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) validSignature(historyID, hash, expires, presented string) bool {
	if historyID == "" || hash == "" || expires == "" || presented == "" {
		return false
	}
	at, err := strconv.ParseInt(expires, 10, 64)
	if err != nil || time.Now().Unix() > at {
		return false
	}
	return hmac.Equal([]byte(presented), []byte(s.signature(historyID, hash, expires)))
}

// validPath says why a path cannot be used, or "" if it can.
//
// These become real files in a repository somebody clones onto their own
// machine, so a path that climbs out of it, or that names the repository's own
// metadata, is refused rather than written.
func validPath(path string) string {
	switch {
	case path == "":
		return "error"
	case strings.ContainsRune(path, 0):
		return "error"
	case strings.HasPrefix(path, "/"):
		return "error"
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".." || part == "." || part == "" {
			return "error"
		}
		if strings.EqualFold(part, ".git") {
			return "disallowed"
		}
	}
	return ""
}
