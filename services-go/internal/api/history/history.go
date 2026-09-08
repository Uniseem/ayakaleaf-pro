// Package history is the API's view of a project's versions.
//
// Two services own that between them: project-history, which knows what
// happened and when, and history-v1, which holds the content. Neither is
// reachable from outside the deployment, so everything that needs a version --
// the git bridge, a download, a compile listing a project's images -- comes
// through here.
package history

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// ErrNotFound is returned for a version, or a project, that is not there.
var ErrNotFound = errors.New("no such version")

// Version is where a project's history has got to.
type Version struct {
	Version   int       `json:"version"`
	Timestamp time.Time `json:"timestamp,omitempty"`
	// V2Authors are the people who made the most recent change. Usually one.
	V2Authors []string `json:"v2Authors,omitempty"`
}

// Label is a version somebody named.
type Label struct {
	ID        string    `json:"id"`
	Comment   string    `json:"comment"`
	Version   int       `json:"version"`
	UserID    *string   `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

// File is one file of a project at a version.
//
// Exactly one of the two is set. Text arrives as content because that is what
// the history holds; anything else is named by the hash of its bytes, and is
// fetched from the blob store by whoever needs it.
type File struct {
	Content *string `json:"content,omitempty"`
	Hash    string  `json:"hash,omitempty"`
}

// IsText says whether this file is one the history stores as text.
func (f File) IsText() bool { return f.Content != nil }

// Client talks to the two history services.
type Client struct {
	projectHistory string
	historyV1      string
	filestore      string
	v1User         string
	v1Pass         string
	http           *http.Client

	// globalBlobs are the blobs shared by every project -- an empty file, a
	// handful of common ones. They are addressed differently, so a URL built
	// without knowing about them is a 404.
	globalBlobs map[string]struct{}
}

// Options is what a Client needs.
type Options struct {
	ProjectHistoryURL string
	HistoryV1URL      string
	FilestoreURL      string
	HistoryV1User     string
	HistoryV1Password string
}

// NewClient builds one.
func NewClient(opts Options) *Client {
	return &Client{
		projectHistory: strings.TrimRight(opts.ProjectHistoryURL, "/"),
		historyV1:      strings.TrimRight(opts.HistoryV1URL, "/"),
		filestore:      strings.TrimRight(opts.FilestoreURL, "/"),
		v1User:         opts.HistoryV1User,
		v1Pass:         opts.HistoryV1Password,
		// Generous: uploading a blob or reading a whole project at a version
		// is not a quick request, and giving up early leaves the work running
		// and tells somebody it failed.
		http:        &http.Client{Timeout: 5 * time.Minute},
		globalBlobs: map[string]struct{}{},
	}
}

// LoadGlobalBlobs reads the shared blob list.
//
// It is a fixed set written when the deployment was set up, so it is read once
// at startup: a blob that became global afterwards would be addressed as one
// of the project's own, which is a slower answer and not a wrong one.
func (c *Client) LoadGlobalBlobs(ctx context.Context, db *mongo.Database) error {
	cursor, err := db.Collection("projectHistoryGlobalBlobs").Find(ctx, bson.M{})
	if err != nil {
		return err
	}
	defer func() { _ = cursor.Close(ctx) }()
	blobs := map[string]struct{}{}
	for cursor.Next(ctx) {
		var blob struct {
			ID string `bson:"_id"`
		}
		if err := cursor.Decode(&blob); err != nil {
			return err
		}
		blobs[blob.ID] = struct{}{}
	}
	if err := cursor.Err(); err != nil {
		return err
	}
	c.globalBlobs = blobs
	return nil
}

// BlobURL is where the bytes of a file live, as an address inside the
// deployment. It is what a compile hands to the compiler and what an
// attachment link points at.
func (c *Client) BlobURL(historyID, hash string) string {
	if _, global := c.globalBlobs[hash]; global {
		return c.filestore + "/history/global/hash/" + hash
	}
	return c.filestore + "/history/project/" + historyID + "/hash/" + hash
}

// LatestVersion is where a project's history has got to.
func (c *Client) LatestVersion(ctx context.Context, projectID string) (*Version, error) {
	var version Version
	err := c.get(ctx, c.projectHistory+"/project/"+projectID+"/version", &version)
	if err != nil {
		return nil, err
	}
	return &version, nil
}

// Labels are the versions somebody named, newest first.
func (c *Client) Labels(ctx context.Context, projectID string) ([]Label, error) {
	labels := []Label{}
	if err := c.get(ctx, c.projectHistory+"/project/"+projectID+"/labels", &labels); err != nil {
		if errors.Is(err, ErrNotFound) {
			return []Label{}, nil
		}
		return nil, err
	}
	return labels, nil
}

// Snapshot is every file a project had at a version, by path.
func (c *Client) Snapshot(ctx context.Context, projectID string, version int) (map[string]File, error) {
	var body struct {
		Files map[string]struct {
			Data File `json:"data"`
		} `json:"files"`
	}
	url := c.projectHistory + "/project/" + projectID + "/version/" + strconv.Itoa(version)
	if err := c.get(ctx, url, &body); err != nil {
		return nil, err
	}
	files := make(map[string]File, len(body.Files))
	for path, file := range body.Files {
		files[path] = file.Data
	}
	return files, nil
}

// InitialiseProject gives a project a history and answers with its name in
// the history store.
//
// Called once, when the project is made. Everything that records a change
// needs the answer, so a project whose history could not be started is not a
// project anybody can use.
func (c *Client) InitialiseProject(ctx context.Context, projectID string) (string, error) {
	payload, err := json.Marshal(map[string]any{"historyId": projectID})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.projectHistory+"/project", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 300 {
		return "", fmt.Errorf("project-history answered %s", response.Status)
	}

	var body struct {
		Project struct {
			ID any `json:"id"`
		} `json:"project"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&body); err != nil {
		return "", err
	}
	switch id := body.Project.ID.(type) {
	case string:
		return id, nil
	case float64:
		return strconv.FormatInt(int64(id), 10), nil
	case nil:
		// Older answers say nothing, and the id is the one that was asked for.
		return projectID, nil
	default:
		return fmt.Sprintf("%v", id), nil
	}
}

// Flush writes a project's pending updates into its history.
//
// Anything read at a version is only right once this has happened: an edit
// made a moment ago is in a queue until it does.
func (c *Client) Flush(ctx context.Context, projectID string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.projectHistory+"/project/"+projectID+"/flush", nil)
	if err != nil {
		return err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 300 {
		return fmt.Errorf("project-history answered %s", response.Status)
	}
	return nil
}

// ReadBlob reads the bytes of a file out of the blob store.
func (c *Client) ReadBlob(ctx context.Context, historyID, hash string) (io.ReadCloser, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BlobURL(historyID, hash), nil)
	if err != nil {
		return nil, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode == http.StatusNotFound {
		_ = response.Body.Close()
		return nil, ErrNotFound
	}
	if response.StatusCode >= 300 {
		_ = response.Body.Close()
		return nil, fmt.Errorf("filestore answered %s", response.Status)
	}
	return response.Body, nil
}

// UploadBlob writes bytes into a project's blob store under their own hash.
//
// Writing the same bytes twice is not an error and not a second copy: the
// address is the content, so the second write finds it already there.
func (c *Client) UploadBlob(ctx context.Context, historyID, hash string, content []byte) error {
	url := c.historyV1 + "/projects/" + historyID + "/blobs/" + hash
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(content))
	if err != nil {
		return err
	}
	// Set by hand: history-v1 reads it, and Go would otherwise send this
	// with chunked encoding.
	request.ContentLength = int64(len(content))
	request.Header.Set("Content-Type", "application/octet-stream")
	if c.v1User != "" {
		request.SetBasicAuth(c.v1User, c.v1Pass)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 300 {
		return fmt.Errorf("history-v1 answered %s", response.Status)
	}
	return nil
}

// BlobHash is the address a file's bytes will have.
//
// It is git's hash of a blob, which is what the history store uses, so a file
// that came from a git push already has this hash and does not need a second
// naming scheme.
func BlobHash(content []byte) string {
	sum := sha1.New()
	fmt.Fprintf(sum, "blob %d", len(content))
	sum.Write([]byte{0})
	sum.Write(content)
	return hex.EncodeToString(sum.Sum(nil))
}

func (c *Client) get(ctx context.Context, url string, into any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if response.StatusCode >= 300 {
		return fmt.Errorf("%s answered %s", url, response.Status)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 256<<20)).Decode(into)
}
