// Package documents is the API's side of editing a file.
//
// The content itself lives in document-updater while anybody has the project
// open, and in docstore when nobody does. This package does not hold either:
// it decides who may touch a document and then asks the service that owns it,
// so there is one place that answers "may this person edit this" and one place
// that answers "what does it say".
package documents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ErrNotFound is returned for a document that does not exist in a project.
var ErrNotFound = errors.New("no such document")

// Doc is a text document as the editor needs it.
type Doc struct {
	ID      string   `json:"id"`
	Lines   []string `json:"lines"`
	Version int      `json:"version"`
	// Ranges carries the tracked changes and comments attached to the text.
	Ranges json.RawMessage `json:"ranges,omitempty"`
}

// Client talks to document-updater.
//
// Every read goes through it rather than to docstore, because a document
// somebody has open is only correct in document-updater: reading the stored
// copy would hand somebody a version without the last few minutes of typing.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient builds one.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Get reads a document at its current version.
func (c *Client) Get(ctx context.Context, projectID, docID bson.ObjectID) (*Doc, error) {
	endpoint := fmt.Sprintf("%s/project/%s/doc/%s", c.baseURL, projectID.Hex(), docID.Hex())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, ErrNotFound
	default:
		return nil, fmt.Errorf("document-updater answered %s", response.Status)
	}

	var body struct {
		Lines   []string        `json:"lines"`
		Version int             `json:"version"`
		Ranges  json.RawMessage `json:"ranges"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 32<<20)).Decode(&body); err != nil {
		return nil, err
	}
	return &Doc{ID: docID.Hex(), Lines: body.Lines, Version: body.Version, Ranges: body.Ranges}, nil
}

// SetContent replaces a document's text.
//
// Used by things that change a file without an editing session -- an upload
// overwriting a file, a template being applied -- and never by the editor,
// which sends operations over the socket instead.
func (c *Client) SetContent(ctx context.Context, projectID, docID, userID bson.ObjectID, lines []string, source string) error {
	payload, err := json.Marshal(map[string]any{
		"lines":  lines,
		"source": source,
		"user_id": userID.Hex(),
	})
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/project/%s/doc/%s", c.baseURL, projectID.Hex(), docID.Hex())
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 300 {
		return fmt.Errorf("document-updater answered %s", response.Status)
	}
	return nil
}

// Flush writes a project's documents back to storage.
//
// Called before anything reads the stored copy -- a download, a compile that
// reads from disk -- because until it happens the stored copy is behind.
func (c *Client) Flush(ctx context.Context, projectID bson.ObjectID) error {
	endpoint := fmt.Sprintf("%s/project/%s/flush", c.baseURL, projectID.Hex())
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 300 {
		return fmt.Errorf("document-updater answered %s", response.Status)
	}
	return nil
}
