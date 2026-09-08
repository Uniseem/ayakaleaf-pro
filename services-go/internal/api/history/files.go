package history

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Reading a project as it was.
//
// The three things anything comparing two versions needs: what files there
// were, what changed between two of them, and the bytes of one file. All of it
// is project-history's; none of it is stored here.

// Change is one entry in the difference between two versions.
type Change struct {
	Pathname string `json:"pathname"`
	// NewPathname is set when the operation is a rename.
	NewPathname string `json:"newPathname,omitempty"`
	// Operation is added, removed, edited or renamed. An entry with none is a
	// file that did not change, which the diff includes.
	Operation string `json:"operation,omitempty"`
}

// PathsAtVersion is every file a project had at a version.
func (c *Client) PathsAtVersion(ctx context.Context, projectID string, version int) ([]string, error) {
	var body struct {
		Paths []string `json:"paths"`
	}
	url := c.projectHistory + "/project/" + projectID +
		"/paths/version/" + strconv.Itoa(version)
	if err := c.get(ctx, url, &body); err != nil {
		return nil, err
	}
	return body.Paths, nil
}

// Diff is what changed between two versions.
func (c *Client) Diff(ctx context.Context, projectID string, from, to int) ([]Change, error) {
	var body struct {
		Diff []Change `json:"diff"`
	}
	url := c.projectHistory + "/project/" + projectID + "/filetree/diff" +
		"?from=" + strconv.Itoa(from) + "&to=" + strconv.Itoa(to)
	if err := c.get(ctx, url, &body); err != nil {
		return nil, err
	}
	return body.Diff, nil
}

// FileAtVersion is the bytes of one file as it was at a version.
func (c *Client) FileAtVersion(ctx context.Context, projectID string, version int, path string) ([]byte, error) {
	// Each segment escaped, but not the slashes between them: the path is a
	// path in the request, not one value.
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	endpoint := c.projectHistory + "/project/" + projectID +
		"/version/" + strconv.Itoa(version) + "/" + strings.Join(parts, "/")

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if response.StatusCode >= 300 {
		return nil, ErrNotFound
	}
	return io.ReadAll(io.LimitReader(response.Body, 512<<20))
}
