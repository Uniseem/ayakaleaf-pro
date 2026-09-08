package documents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Reading and writing a document in storage.
//
// Used by the parts of the deployment that work on stored documents rather
// than open ones: document-updater loading a document nobody has open, and
// writing it back when everybody closes it.

// Stored is a document as docstore holds it.
type Stored struct {
	Lines   []string        `json:"lines"`
	Version int64           `json:"version"`
	Ranges  json.RawMessage `json:"ranges"`
	Rev     int64           `json:"rev"`
}

// Get reads a document out of storage.
//
// peek reads it without counting as a read, which is what a service checking
// on a document wants: an archived document is not brought back for it.
func (s *Storage) Get(ctx context.Context, projectID, docID bson.ObjectID, peek bool) (*Stored, error) {
	endpoint := fmt.Sprintf("%s/project/%s/doc/%s", s.baseURL, projectID.Hex(), docID.Hex())
	if peek {
		endpoint += "?peek=true"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := s.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, ErrNotFound
	default:
		return nil, fmt.Errorf("docstore answered %s", response.Status)
	}

	var stored Stored
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<20)).Decode(&stored); err != nil {
		return nil, err
	}
	return &stored, nil
}

// Written is what storage says about a write.
type Written struct {
	// Modified is false when the text was already what was sent, which is how
	// a caller knows not to record a change nobody made.
	Modified bool  `json:"modified"`
	Rev      int64 `json:"rev"`
}

// Update writes a document's text, ranges and version.
func (s *Storage) Update(
	ctx context.Context,
	projectID, docID bson.ObjectID,
	lines []string,
	version int64,
	ranges json.RawMessage,
) (*Written, error) {
	if lines == nil {
		lines = []string{""}
	}
	if len(ranges) == 0 {
		ranges = json.RawMessage("{}")
	}
	payload, err := json.Marshal(map[string]any{
		"lines": lines, "version": version, "ranges": ranges,
	})
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s/project/%s/doc/%s", s.baseURL, projectID.Hex(), docID.Hex())
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := s.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 300 {
		return nil, fmt.Errorf("docstore answered %s", response.Status)
	}
	var written Written
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&written); err != nil {
		return nil, err
	}
	return &written, nil
}
