package documents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Storage is docstore: where a document's text lives when nobody has it open.
//
// Everything the editor does goes through document-updater instead, which is a
// deliberate split: this is only for the two moments when there is no editing
// session at all -- a document coming into existence, and one being deleted.
type Storage struct {
	baseURL string
	http    *http.Client
}

// NewStorage builds one.
func NewStorage(baseURL string) *Storage {
	return &Storage{baseURL: baseURL, http: &http.Client{Timeout: 30 * time.Second}}
}

// Create writes a new document.
func (s *Storage) Create(ctx context.Context, projectID, docID bson.ObjectID, lines []string) error {
	if lines == nil {
		lines = []string{""}
	}
	payload, err := json.Marshal(map[string]any{
		"lines":   lines,
		"version": 0,
		"ranges":  map[string]any{},
	})
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/project/%s/doc/%s", s.baseURL, projectID.Hex(), docID.Hex())
	return s.send(ctx, http.MethodPost, endpoint, payload)
}

// Delete marks a document deleted.
//
// The text is kept. A file somebody removed by accident is one of the few
// things people ask for back, and a row in a collection is a small price for
// being able to say yes.
func (s *Storage) Delete(ctx context.Context, projectID, docID bson.ObjectID, name string) error {
	payload, err := json.Marshal(map[string]any{
		"deleted":   true,
		"name":      name,
		"deletedAt": time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/project/%s/doc/%s", s.baseURL, projectID.Hex(), docID.Hex())
	return s.send(ctx, http.MethodPatch, endpoint, payload)
}

func (s *Storage) send(ctx context.Context, method, endpoint string, payload []byte) error {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 300 {
		return fmt.Errorf("docstore answered %s", response.Status)
	}
	return nil
}

// DestroyProject removes every document a project has, for good.
//
// Not the same as deleting one: a deleted document is marked and kept, so it
// can be listed and restored. This is what happens when the project itself
// goes and there is nothing left to restore into.
func (s *Storage) DestroyProject(ctx context.Context, projectID bson.ObjectID) error {
	endpoint := fmt.Sprintf("%s/project/%s/destroy", s.baseURL, projectID.Hex())
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := s.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 300 {
		return fmt.Errorf("docstore answered %s", response.Status)
	}
	return nil
}
