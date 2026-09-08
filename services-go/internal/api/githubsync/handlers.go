package githubsync

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// The pages' side of GitHub sync.
//
// Connecting an account is an OAuth round trip like any other. Everything
// after it is about one project and one repository, and needs the same write
// access to the project that editing it does -- a sync overwrites files, so
// somebody who may only read must not be able to start one.

// stateKey is where the round trip's state is kept while somebody is at
// GitHub. In the session, so a reply that arrives with a state we never issued
// is refused.
const stateKey = "githubSyncState"

// Status is what the account page shows.
type Status struct {
	Enabled   bool   `json:"enabled"`
	Connected bool   `json:"connected"`
	Login     string `json:"login,omitempty"`
}

// Status says whether this site offers GitHub sync and whether this person has
// connected an account to it.
func (s *Service) Status(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	status := Status{Enabled: s.enabled()}
	if !status.Enabled {
		return httpapi.JSON(w, http.StatusOK, status)
	}

	token, err := s.store.Token(r.Context(), user.ID)
	if errors.Is(err, ErrNoToken) {
		return httpapi.JSON(w, http.StatusOK, status)
	}
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	// Asked of GitHub rather than assumed: a token that was revoked there is
	// still stored here, and saying "connected" about it would leave somebody
	// pressing a button that cannot work.
	account, err := s.api.User(r.Context(), token)
	if err != nil {
		return httpapi.JSON(w, http.StatusOK, status)
	}
	status.Connected = true
	status.Login = account.Login
	return httpapi.JSON(w, http.StatusOK, status)
}

// Start sends somebody to GitHub.
func (s *Service) Start(w http.ResponseWriter, r *http.Request) error {
	if _, err := httpapi.RequireUser(r.Context()); err != nil {
		return err
	}
	creds, ok := s.credentials()
	if !ok {
		return apierr.BadRequest.WithMessage("GitHub is not set up on this site.")
	}

	state, err := randomState()
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	sess := httpapi.SessionFrom(r.Context())
	sessionID := httpapi.SessionIDFrom(r.Context())
	if sess == nil || sessionID == "" {
		// Somebody has to be signed in to get here, so a request without a
		// session is one whose session has just gone.
		return apierr.Unauthorised.WithMessage("Sign in again and try that once more.")
	}
	if sess.Extra == nil {
		sess.Extra = map[string]json.RawMessage{}
	}
	sess.Extra[stateKey] = jsonString(state)
	if err := s.sessions.Save(r.Context(), sessionID, sess); err != nil {
		return apierr.Internal.WithCause(err)
	}

	http.Redirect(w, r, AuthorizeURL(creds, state), http.StatusFound)
	return nil
}

// Callback finishes the round trip.
func (s *Service) Callback(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	creds, ok := s.credentials()
	if !ok {
		return s.backToAccount(w, r, "GitHub is not set up on this site.")
	}

	sess := httpapi.SessionFrom(r.Context())
	sessionID := httpapi.SessionIDFrom(r.Context())
	if sess == nil {
		return s.backToAccount(w, r, "That connection did not complete. Try again.")
	}
	expected := readString(sess.Extra, stateKey)
	// Whatever happens next, the state is spent.
	delete(sess.Extra, stateKey)
	if sessionID != "" {
		_ = s.sessions.Save(r.Context(), sessionID, sess)
	}

	state := r.URL.Query().Get("state")
	if expected == "" || state == "" || state != expected {
		// The reply did not come from a request this session started, so it is
		// not this person's.
		return s.backToAccount(w, r, "That sign-in did not come from this site. Try again.")
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		return s.backToAccount(w, r, "GitHub did not complete the connection.")
	}

	token, err := s.api.Exchange(r.Context(), creds, code)
	if err != nil {
		return s.backToAccount(w, r, "GitHub would not confirm that connection.")
	}
	if err := s.store.SaveToken(r.Context(), user.ID, token); err != nil {
		return apierr.Internal.WithCause(err)
	}
	http.Redirect(w, r, "/account", http.StatusFound)
	return nil
}

// Unlink forgets somebody's GitHub account.
func (s *Service) Unlink(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	// Told to GitHub as well, so that disconnecting here is disconnecting.
	// Best effort: the token is going either way.
	if creds, ok := s.credentials(); ok {
		if token, err := s.store.Token(r.Context(), user.ID); err == nil {
			_ = s.api.Revoke(r.Context(), creds, token)
		}
	}
	if err := s.store.ForgetToken(r.Context(), user.ID); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.NoContent(w)
}

// Repos are the repositories this account can see.
func (s *Service) Repos(w http.ResponseWriter, r *http.Request) error {
	token, err := s.tokenFor(r)
	if err != nil {
		return err
	}
	repos, err := s.api.Repos(r.Context(), token)
	if err != nil {
		return githubError(err)
	}
	return httpapi.JSON(w, http.StatusOK, map[string]any{"repos": repos})
}

// Owners are the names a new repository could be made under: this account, and
// the organisations it belongs to.
func (s *Service) Owners(w http.ResponseWriter, r *http.Request) error {
	token, err := s.tokenFor(r)
	if err != nil {
		return err
	}
	account, err := s.api.User(r.Context(), token)
	if err != nil {
		return githubError(err)
	}
	orgs, err := s.api.Orgs(r.Context(), token)
	if err != nil {
		return githubError(err)
	}
	names := make([]string, 0, len(orgs))
	for _, org := range orgs {
		names = append(names, org.Login)
	}
	return httpapi.JSON(w, http.StatusOK, map[string]any{
		"login": account.Login,
		"orgs":  names,
	})
}

// ImportRepo makes a project out of a repository.
func (s *Service) ImportRepo(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	if !s.enabled() {
		return apierr.BadRequest.WithMessage("GitHub is not set up on this site.")
	}
	var in struct {
		Name              string `json:"name"`
		FullName          string `json:"fullName"`
		DefaultBranchName string `json:"defaultBranchName"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.FullName) == "" || strings.TrimSpace(in.DefaultBranchName) == "" {
		return apierr.BadRequest.WithField("fullName").WithMessage("Choose a repository first.")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = shortName(in.FullName)
	}

	project, err := s.Import(r.Context(), user.ID, name, in.FullName, in.DefaultBranchName)
	if err != nil {
		return githubError(err)
	}
	return httpapi.JSON(w, http.StatusCreated, map[string]any{
		"projectId": project.ID.Hex(),
	})
}

// ProjectStatus is what one project's editor shows about its repository.
func (s *Service) ProjectStatus(w http.ResponseWriter, r *http.Request) error {
	project, user, err := s.project(r, false)
	if err != nil {
		return err
	}
	answer := map[string]any{"enabled": s.enabled(), "linked": false}
	if !s.enabled() {
		return httpapi.JSON(w, http.StatusOK, answer)
	}

	state, err := s.store.State(r.Context(), project.ID)
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	if state == nil {
		return httpapi.JSON(w, http.StatusOK, answer)
	}
	answer["linked"] = true
	answer["state"] = state

	// Whether this particular person can push, which is not the same question
	// as whether the project is linked: a collaborator may not be able to.
	if token, err := s.store.Token(r.Context(), user.ID); err == nil {
		if canPush, err := s.api.CanPush(r.Context(), token, state.RepoFullName); err == nil {
			answer["canPush"] = canPush
		} else {
			answer["canPush"] = false
		}
	} else {
		answer["canPush"] = false
	}
	return httpapi.JSON(w, http.StatusOK, answer)
}

// Overview is what a sync would bring in, before doing it.
func (s *Service) Overview(w http.ResponseWriter, r *http.Request) error {
	project, user, err := s.project(r, true)
	if err != nil {
		return err
	}
	state, err := s.store.State(r.Context(), project.ID)
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	if state == nil {
		return apierr.NotFound.WithMessage("This project is not linked to a repository.")
	}
	token, err := s.store.Token(r.Context(), user.ID)
	if err != nil {
		return githubError(err)
	}

	current, err := s.latestVersion(r.Context(), project.ID)
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	answer := map[string]any{
		"mergeStatus":    state.MergeStatus,
		"repoFullName":   state.RepoFullName,
		"projectChanged": current != state.LastSyncVersion,
		"unmergedBranch": state.UnmergedBranchName,
		"commits":        []Commit{},
		"repoRewritten":  state.MergeStatus == StatusDiverged,
	}
	if state.MergeStatus == StatusConflict {
		return httpapi.JSON(w, http.StatusOK, answer)
	}

	commits, diverged, err := s.api.Compare(r.Context(), token,
		state.RepoFullName, state.LastSyncCommit, state.DefaultBranchName)
	if errors.Is(err, ErrNotFound) {
		// The commit this project was anchored to is gone, which is what a
		// force push looks like from here.
		_ = s.store.UpdateState(r.Context(), project.ID, bson.M{"mergeStatus": StatusDiverged})
		answer["repoRewritten"] = true
		answer["mergeStatus"] = StatusDiverged
		return httpapi.JSON(w, http.StatusOK, answer)
	}
	if err != nil {
		return githubError(err)
	}
	if diverged {
		_ = s.store.UpdateState(r.Context(), project.ID, bson.M{"mergeStatus": StatusDiverged})
		answer["mergeStatus"] = StatusDiverged
		answer["repoRewritten"] = true
	}
	answer["commits"] = commits
	return httpapi.JSON(w, http.StatusOK, answer)
}

// Merge runs a sync.
func (s *Service) Merge(w http.ResponseWriter, r *http.Request) error {
	project, user, err := s.project(r, true)
	if err != nil {
		return err
	}
	var in struct {
		Message string `json:"message"`
		// Resolved is somebody saying they merged the conflict branch
		// themselves, which is the only way out of the conflict state.
		Resolved bool `json:"resolved"`
	}
	if r.ContentLength > 0 {
		if err := httpapi.Decode(r, &in); err != nil {
			return err
		}
	}
	message := strings.TrimSpace(in.Message)
	if message == "" {
		message = "Updates from " + project.Name
	}

	result, err := s.Sync(r.Context(), user.ID, project.ID, message, in.Resolved)
	if errors.Is(err, ErrNotLinked) {
		return apierr.NotFound.WithMessage("This project is not linked to a repository.")
	}
	if err != nil {
		return githubError(err)
	}
	return httpapi.JSON(w, http.StatusOK, result)
}

// ExportProject makes a repository out of a project.
func (s *Service) ExportProject(w http.ResponseWriter, r *http.Request) error {
	project, user, err := s.project(r, true)
	if err != nil {
		return err
	}
	var in RepoOptions
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Name) == "" {
		in.Name = slug(project.Name)
	}

	state, err := s.Export(r.Context(), user.ID, project.ID, in)
	if err != nil {
		return githubError(err)
	}
	return httpapi.JSON(w, http.StatusCreated, map[string]any{"state": state})
}

// UnlinkProject stops a project and a repository being told about each other.
func (s *Service) UnlinkProject(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.project(r, true)
	if err != nil {
		return err
	}
	if err := s.store.ForgetState(r.Context(), project.ID); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.NoContent(w)
}

// --- the checks the handlers share -----------------------------------------

func (s *Service) enabled() bool {
	if s.settings == nil || !s.settings.GitHubSyncEnabled() {
		return false
	}
	_, ok := s.settings.GitHubSyncCredentials()
	return ok
}

func (s *Service) credentials() (Credentials, bool) {
	if s.settings == nil || !s.settings.GitHubSyncEnabled() {
		return Credentials{}, false
	}
	return s.settings.GitHubSyncCredentials()
}

func (s *Service) tokenFor(r *http.Request) (string, error) {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return "", err
	}
	if !s.enabled() {
		return "", apierr.BadRequest.WithMessage("GitHub is not set up on this site.")
	}
	token, err := s.store.Token(r.Context(), user.ID)
	if errors.Is(err, ErrNoToken) {
		return "", apierr.BadRequest.WithCode("github_not_connected").
			WithMessage("Connect a GitHub account first.")
	}
	if err != nil {
		return "", apierr.Internal.WithCause(err)
	}
	return token, nil
}

// project reads the project a request is about and checks this person may do
// this to it. A sync overwrites files, so it needs write access.
func (s *Service) project(r *http.Request, write bool) (*projects.Project, *userRef, error) {
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
	if write && !access.CanWrite() {
		return nil, nil, apierr.Forbidden.WithMessage("You have read-only access to this project.")
	}
	return project, &userRef{ID: user.ID}, nil
}

// userRef is the part of a person these handlers need.
type userRef struct{ ID bson.ObjectID }

// githubError turns what GitHub said into what to tell somebody.
func githubError(err error) error {
	switch {
	case errors.Is(err, ErrNoToken):
		return apierr.BadRequest.WithCode("github_not_connected").
			WithMessage("Connect a GitHub account first.")
	case errors.Is(err, ErrNotFound):
		return apierr.NotFound.WithMessage(
			"GitHub does not have that, or this account cannot see it.")
	case errors.Is(err, ErrForbidden):
		return apierr.Forbidden.WithMessage(
			"This GitHub account is not allowed to do that.")
	case errors.Is(err, ErrRateLimited):
		return apierr.TooMany.WithMessage(
			"GitHub is rate limiting this account. Try again in a few minutes.")
	case errors.Is(err, ErrExists):
		return apierr.Conflict.WithField("name").
			WithMessage("A repository by that name already exists.")
	case errors.Is(err, ErrNotLinked):
		return apierr.NotFound.WithMessage("This project is not linked to a repository.")
	default:
		return apierr.Internal.WithCause(err)
	}
}

func randomState() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// shortName is the repository's own name out of owner/name.
func shortName(fullName string) string {
	if slash := strings.LastIndexByte(fullName, '/'); slash >= 0 {
		return fullName[slash+1:]
	}
	return fullName
}

// slug is a project's name as a repository name: what GitHub will accept.
func slug(name string) string {
	var out strings.Builder
	for _, letter := range name {
		switch {
		case letter >= 'a' && letter <= 'z', letter >= 'A' && letter <= 'Z',
			letter >= '0' && letter <= '9', letter == '-', letter == '_', letter == '.':
			out.WriteRune(letter)
		default:
			out.WriteByte('-')
		}
	}
	slugged := strings.Trim(out.String(), "-")
	if slugged == "" {
		return "project"
	}
	return slugged
}

// backToAccount sends somebody back to the page they started from, with a
// message written by this service and never by GitHub: a provider that could
// put words on our pages could put anything there.
func (s *Service) backToAccount(w http.ResponseWriter, r *http.Request, message string) error {
	http.Redirect(w, r, "/account?error="+url.QueryEscape(message), http.StatusFound)
	return nil
}

// jsonString is how a value goes into the session, which holds raw JSON.
func jsonString(value string) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return encoded
}

// readString reads one back, answering with the empty string for anything that
// is not there or is not a string.
func readString(extra map[string]json.RawMessage, key string) string {
	raw, present := extra[key]
	if !present {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}
