// Package githubsync keeps a project and a GitHub repository in step.
//
// Not a git client: everything here is done through GitHub's own API, which
// means the merges are GitHub's merges. That is the point -- three-way merging
// text is where a sync goes wrong quietly, and the side that already does it
// for every pull request is a better place for it than this one.
package githubsync

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Where GitHub is. Not configurable: this talks to GitHub, and an installation
// that wanted something else would need more than a different address.
const (
	githubSite = "https://github.com"
	githubAPI  = "https://api.github.com"
)

var (
	// ErrNoToken means this person has not connected an account.
	ErrNoToken = errors.New("no github account is connected")
	// ErrNotFound is a repository, branch or commit GitHub does not have --
	// or does not admit to having, which for a private repository is the same
	// answer.
	ErrNotFound = errors.New("not found on github")
	// ErrForbidden is an account that may read something but not write it.
	ErrForbidden = errors.New("not allowed on github")
	// ErrConflict is a merge GitHub would not do, and the whole reason the
	// conflict flow exists.
	ErrConflict = errors.New("the merge has conflicts")
	// ErrRateLimited is GitHub asking to be left alone for a while.
	ErrRateLimited = errors.New("github is rate limiting this account")
	// ErrExists is a repository name already taken.
	ErrExists = errors.New("that repository already exists")
)

// Credentials are what an administrator registered with GitHub.
type Credentials struct {
	ClientID     string
	ClientSecret string
	// CallbackURL is where GitHub sends somebody back to. It has to match what
	// was registered there exactly, which is why it is built from the site's
	// own address and not from the request.
	CallbackURL string
}

// api talks to GitHub.
type api struct {
	http *http.Client
}

func newAPI() *api {
	return &api{http: &http.Client{Timeout: 2 * time.Minute}}
}

// User is the account a token belongs to.
type User struct {
	Login     string `json:"login"`
	Name      string `json:"name,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// Org is one organisation an account belongs to.
type Org struct {
	Login string `json:"login"`
}

// Repo is one repository, as much of it as anything here needs.
type Repo struct {
	Name              string `json:"name"`
	FullName          string `json:"fullName"`
	DefaultBranchName string `json:"defaultBranchName"`
	Private           bool   `json:"private"`
}

// Commit is one commit in a comparison.
type Commit struct {
	SHA     string `json:"sha"`
	Message string `json:"message"`
	Author  struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Date  string `json:"date"`
	} `json:"author"`
}

// TreeEntry is a path and the blob at it. A nil hash deletes the path, which
// is how GitHub's tree API says "remove this".
type TreeEntry struct {
	Path string
	SHA  string
}

// --- the OAuth dance --------------------------------------------------------

// AuthorizeURL is where somebody is sent to connect their account.
//
// The scopes are the ones a sync needs and no more: repositories to read and
// write, organisations to list the ones a repository could be made in.
func AuthorizeURL(creds Credentials, state string) string {
	query := url.Values{}
	query.Set("client_id", creds.ClientID)
	query.Set("redirect_uri", creds.CallbackURL)
	query.Set("scope", "read:org,repo,workflow")
	query.Set("state", state)
	return githubSite + "/login/oauth/authorize?" + query.Encode()
}

// Exchange turns the code GitHub sent back into a token.
func (a *api) Exchange(ctx context.Context, creds Credentials, code string) (string, error) {
	var answer struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error_description"`
	}
	err := a.call(ctx, request{
		method: http.MethodPost,
		url:    githubSite + "/login/oauth/access_token",
		body: map[string]any{
			"code":          code,
			"client_id":     creds.ClientID,
			"client_secret": creds.ClientSecret,
			"redirect_uri":  creds.CallbackURL,
		},
	}, &answer)
	if err != nil {
		return "", err
	}
	if answer.AccessToken == "" {
		if answer.Error != "" {
			return "", errors.New(answer.Error)
		}
		return "", errors.New("github did not return a token")
	}
	return answer.AccessToken, nil
}

// Revoke asks GitHub to forget a token.
//
// Best effort on the way out: an account being unlinked here should stop
// working there too, but somebody who asked to disconnect has disconnected
// whether or not GitHub answers.
func (a *api) Revoke(ctx context.Context, creds Credentials, token string) error {
	return a.call(ctx, request{
		method: http.MethodDelete,
		url:    githubAPI + "/applications/" + creds.ClientID + "/token",
		basic:  [2]string{creds.ClientID, creds.ClientSecret},
		body:   map[string]any{"access_token": token},
	}, nil)
}

// --- who and what ----------------------------------------------------------

// User is the account a token belongs to.
func (a *api) User(ctx context.Context, token string) (*User, error) {
	var user User
	if err := a.call(ctx, request{url: githubAPI + "/user", token: token}, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// Orgs are the organisations an account belongs to.
func (a *api) Orgs(ctx context.Context, token string) ([]Org, error) {
	orgs := []Org{}
	err := a.call(ctx, request{
		url: githubAPI + "/user/orgs?per_page=100", token: token,
	}, &orgs)
	if err != nil {
		return nil, err
	}
	return orgs, nil
}

// Repos are the repositories an account can see, all of them.
func (a *api) Repos(ctx context.Context, token string) ([]Repo, error) {
	all := []Repo{}
	for page := 1; page <= 20; page++ {
		var raw []struct {
			Name          string `json:"name"`
			FullName      string `json:"full_name"`
			DefaultBranch string `json:"default_branch"`
			Private       bool   `json:"private"`
		}
		link, err := a.callWithHeaders(ctx, request{
			url:   githubAPI + "/user/repos?per_page=100&page=" + strconv.Itoa(page),
			token: token,
		}, &raw)
		if err != nil {
			return nil, err
		}
		for _, repo := range raw {
			all = append(all, Repo{
				Name: repo.Name, FullName: repo.FullName,
				DefaultBranchName: repo.DefaultBranch, Private: repo.Private,
			})
		}
		// GitHub says in a header whether there is more. Following that rather
		// than counting is what makes the loop end when it should.
		if !strings.Contains(link.Get("Link"), `rel="next"`) {
			break
		}
	}
	return all, nil
}

// CanPush says whether this account may write to a repository.
func (a *api) CanPush(ctx context.Context, token, repo string) (bool, error) {
	var answer struct {
		Permissions struct {
			Push bool `json:"push"`
		} `json:"permissions"`
	}
	if err := a.call(ctx, request{url: githubAPI + "/repos/" + repo, token: token}, &answer); err != nil {
		return false, err
	}
	return answer.Permissions.Push, nil
}

// CreateRepo makes a repository, in an organisation if one was named.
func (a *api) CreateRepo(ctx context.Context, token string, options RepoOptions) (*Repo, error) {
	endpoint := githubAPI + "/user/repos"
	if options.Org != "" {
		endpoint = githubAPI + "/orgs/" + options.Org + "/repos"
	}
	var created struct {
		Name          string `json:"name"`
		FullName      string `json:"full_name"`
		DefaultBranch string `json:"default_branch"`
		Private       bool   `json:"private"`
	}
	err := a.call(ctx, request{
		method: http.MethodPost, url: endpoint, token: token,
		body: map[string]any{
			"name":        options.Name,
			"description": options.Description,
			"private":     !options.Public,
			// With a first commit, so there is a branch to push onto. An empty
			// repository has no default branch and nothing to update.
			"auto_init": true,
		},
	}, &created)
	if err != nil {
		return nil, err
	}
	return &Repo{
		Name: created.Name, FullName: created.FullName,
		DefaultBranchName: created.DefaultBranch, Private: created.Private,
	}, nil
}

// RepoOptions is what a new repository should be.
type RepoOptions struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Public      bool   `json:"isPublic,omitempty"`
	Org         string `json:"org,omitempty"`
}

// --- blobs, trees, commits, branches ---------------------------------------

// UploadBlob writes bytes into a repository and answers with git's name for
// them.
func (a *api) UploadBlob(ctx context.Context, token, repo string, content []byte) (string, error) {
	var answer struct {
		SHA string `json:"sha"`
	}
	err := a.call(ctx, request{
		method: http.MethodPost,
		url:    githubAPI + "/repos/" + repo + "/git/blobs",
		token:  token,
		body: map[string]any{
			"content":  base64.StdEncoding.EncodeToString(content),
			"encoding": "base64",
		},
	}, &answer)
	if err != nil {
		return "", err
	}
	return answer.SHA, nil
}

// ReadFile is the bytes of one file at a commit.
func (a *api) ReadFile(ctx context.Context, token, repo, ref, path string) ([]byte, error) {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	endpoint := githubAPI + "/repos/" + repo + "/contents/" +
		strings.Join(parts, "/") + "?ref=" + url.QueryEscape(ref)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/vnd.github.raw")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	response, err := a.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if err := statusError(response.StatusCode, nil); err != nil {
		return nil, err
	}
	return io.ReadAll(io.LimitReader(response.Body, 512<<20))
}

// Tree is every file at a commit, flat.
func (a *api) Tree(ctx context.Context, token, repo, commit string) (map[string]string, error) {
	var answer struct {
		Tree []struct {
			Path string `json:"path"`
			SHA  string `json:"sha"`
			Type string `json:"type"`
		} `json:"tree"`
	}
	err := a.call(ctx, request{
		url:   githubAPI + "/repos/" + repo + "/git/trees/" + commit + "?recursive=1",
		token: token,
	}, &answer)
	if err != nil {
		return nil, err
	}
	blobs := map[string]string{}
	for _, entry := range answer.Tree {
		if entry.Type == "blob" {
			blobs[entry.Path] = entry.SHA
		}
	}
	return blobs, nil
}

// CommitTree is the tree a commit points at, which is what a new tree is built
// on top of.
func (a *api) CommitTree(ctx context.Context, token, repo, commit string) (string, error) {
	var answer struct {
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
	}
	err := a.call(ctx, request{
		url:   githubAPI + "/repos/" + repo + "/git/commits/" + commit,
		token: token,
	}, &answer)
	if err != nil {
		return "", err
	}
	return answer.Tree.SHA, nil
}

// CreateTree writes a tree: the entries given, on top of a base tree.
//
// An entry with no hash removes that path, which is how a delete is expressed
// when there is no working copy to delete from.
func (a *api) CreateTree(ctx context.Context, token, repo string, entries []TreeEntry, base string) (string, error) {
	tree := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		item := map[string]any{
			"path": entry.Path,
			"mode": "100644",
			"type": "blob",
		}
		if entry.SHA == "" {
			item["sha"] = nil
		} else {
			item["sha"] = entry.SHA
		}
		tree = append(tree, item)
	}
	body := map[string]any{"tree": tree}
	if base != "" {
		body["base_tree"] = base
	}

	var answer struct {
		SHA string `json:"sha"`
	}
	err := a.call(ctx, request{
		method: http.MethodPost,
		url:    githubAPI + "/repos/" + repo + "/git/trees",
		token:  token, body: body,
	}, &answer)
	if err != nil {
		return "", err
	}
	return answer.SHA, nil
}

// CreateCommit writes a commit.
func (a *api) CreateCommit(ctx context.Context, token, repo, tree, message string, parents []string) (string, error) {
	if parents == nil {
		parents = []string{}
	}
	var answer struct {
		SHA string `json:"sha"`
	}
	err := a.call(ctx, request{
		method: http.MethodPost,
		url:    githubAPI + "/repos/" + repo + "/git/commits",
		token:  token,
		body:   map[string]any{"tree": tree, "message": message, "parents": parents},
	}, &answer)
	if err != nil {
		return "", err
	}
	return answer.SHA, nil
}

// BranchHead is where a branch points.
func (a *api) BranchHead(ctx context.Context, token, repo, branch string) (string, error) {
	var answer struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	err := a.call(ctx, request{
		url:   githubAPI + "/repos/" + repo + "/git/ref/heads/" + url.PathEscape(branch),
		token: token,
	}, &answer)
	if err != nil {
		return "", err
	}
	return answer.Object.SHA, nil
}

// CreateBranch points a new name at a commit.
func (a *api) CreateBranch(ctx context.Context, token, repo, branch, sha string) error {
	return a.call(ctx, request{
		method: http.MethodPost,
		url:    githubAPI + "/repos/" + repo + "/git/refs",
		token:  token,
		body:   map[string]any{"ref": "refs/heads/" + branch, "sha": sha},
	}, nil)
}

// UpdateBranch moves a branch. Without force it fails rather than losing
// commits, which is what makes it a fast-forward attempt.
func (a *api) UpdateBranch(ctx context.Context, token, repo, branch, sha string, force bool) error {
	return a.call(ctx, request{
		method: http.MethodPatch,
		url:    githubAPI + "/repos/" + repo + "/git/refs/heads/" + url.PathEscape(branch),
		token:  token,
		body:   map[string]any{"sha": sha, "force": force},
	}, nil)
}

// DeleteBranch removes a branch.
func (a *api) DeleteBranch(ctx context.Context, token, repo, branch string) error {
	return a.call(ctx, request{
		method: http.MethodDelete,
		url:    githubAPI + "/repos/" + repo + "/git/refs/heads/" + url.PathEscape(branch),
		token:  token,
	}, nil)
}

// Merge asks GitHub to merge one branch into another.
//
// This is the whole reason the sync works through the API: the merge is
// GitHub's, done the way it does every pull request, and a conflict comes back
// as a refusal rather than as a file full of markers.
func (a *api) Merge(ctx context.Context, token, repo, base, head string) (string, error) {
	var answer struct {
		SHA string `json:"sha"`
	}
	err := a.call(ctx, request{
		method: http.MethodPost,
		url:    githubAPI + "/repos/" + repo + "/merges",
		token:  token,
		body:   map[string]any{"base": base, "head": head},
	}, &answer)
	if err != nil {
		return "", err
	}
	return answer.SHA, nil
}

// Compare is what happened on a branch since a commit.
func (a *api) Compare(ctx context.Context, token, repo, from, to string) ([]Commit, bool, error) {
	var answer struct {
		Status  string `json:"status"`
		Commits []struct {
			SHA    string `json:"sha"`
			Commit struct {
				Message string `json:"message"`
				Author  struct {
					Name  string `json:"name"`
					Email string `json:"email"`
					Date  string `json:"date"`
				} `json:"author"`
			} `json:"commit"`
		} `json:"commits"`
	}
	endpoint := githubAPI + "/repos/" + repo + "/compare/" +
		url.PathEscape(from) + "..." + url.PathEscape(to)
	if err := a.call(ctx, request{url: endpoint, token: token}, &answer); err != nil {
		return nil, false, err
	}

	commits := make([]Commit, 0, len(answer.Commits))
	for _, entry := range answer.Commits {
		commit := Commit{SHA: entry.SHA, Message: entry.Commit.Message}
		commit.Author.Name = entry.Commit.Author.Name
		commit.Author.Email = entry.Commit.Author.Email
		commit.Author.Date = entry.Commit.Author.Date
		commits = append(commits, commit)
	}
	// "behind" as well as "diverged": both mean the repository is no longer
	// the thing this project was last synced against.
	return commits, answer.Status == "diverged" || answer.Status == "behind", nil
}

// Zipball is a repository at a commit, as an archive.
func (a *api) Zipball(ctx context.Context, token, repo, ref string) (io.ReadCloser, error) {
	endpoint := githubAPI + "/repos/" + repo + "/zipball/" + url.PathEscape(ref)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	response, err := a.http.Do(request)
	if err != nil {
		return nil, err
	}
	if err := statusError(response.StatusCode, nil); err != nil {
		_ = response.Body.Close()
		return nil, err
	}
	return response.Body, nil
}

// --- one way of making a request -------------------------------------------

type request struct {
	method string
	url    string
	token  string
	body   any
	basic  [2]string
}

func (a *api) call(ctx context.Context, spec request, into any) error {
	_, err := a.callWithHeaders(ctx, spec, into)
	return err
}

func (a *api) callWithHeaders(ctx context.Context, spec request, into any) (http.Header, error) {
	method := spec.method
	if method == "" {
		method = http.MethodGet
	}

	var payload io.Reader
	if spec.body != nil {
		encoded, err := json.Marshal(spec.body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, spec.url, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if spec.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if spec.token != "" {
		req.Header.Set("Authorization", "Bearer "+spec.token)
	}
	if spec.basic[0] != "" {
		req.SetBasicAuth(spec.basic[0], spec.basic[1])
	}

	response, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if err := statusError(response.StatusCode, raw); err != nil {
		return response.Header, err
	}
	if into == nil || len(raw) == 0 {
		return response.Header, nil
	}
	return response.Header, json.Unmarshal(raw, into)
}

// statusError turns GitHub's answer into one of the errors the sync knows how
// to act on.
//
// The distinctions matter: a conflict is a normal step in a sync, a rate limit
// is worth waiting out, and a 404 on a private repository means the token
// cannot see it rather than that it is gone.
func statusError(status int, body []byte) error {
	switch {
	case status < 300:
		return nil
	case status == http.StatusUnauthorized:
		return ErrNoToken
	case status == http.StatusNotFound:
		return ErrNotFound
	case status == http.StatusConflict:
		return ErrConflict
	case status == http.StatusUnprocessableEntity:
		if bytes.Contains(body, []byte("already exists")) {
			return ErrExists
		}
		return fmt.Errorf("github refused: %s", summary(body))
	case status == http.StatusForbidden:
		if bytes.Contains(body, []byte("rate limit")) {
			return ErrRateLimited
		}
		return ErrForbidden
	case status == http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		return fmt.Errorf("github answered %d: %s", status, summary(body))
	}
}

func summary(body []byte) string {
	var answer struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &answer) == nil && answer.Message != "" {
		return answer.Message
	}
	if len(body) > 200 {
		return string(body[:200])
	}
	return string(body)
}
