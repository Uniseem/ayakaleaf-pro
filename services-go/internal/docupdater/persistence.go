package docupdater

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	// maxHTTPRequestLength bounds a call to web. This runs while holding the
	// document lock, which expires after thirty seconds, so a slow web must not
	// be allowed to hold everything up.
	maxHTTPRequestLength = 5 * time.Second

	// maxAttempts is one retry, no more: the lock is ticking.
	maxAttempts = 2
	retryDelay  = 10 * time.Millisecond
)

// Errors from the persistence layer.
var (
	// ErrDocumentValidation means web answered, but with a document missing
	// the fields that make it usable.
	ErrDocumentValidation = errors.New("web API response was not a valid document")
	// ErrWebAPIServer is any other unsuccessful answer from web.
	ErrWebAPIServer = errors.New("error accessing web API")
	// ErrFileTooLarge is web refusing a document that exceeds its limit.
	ErrFileTooLarge = errors.New("doc exceeds maximum size")
)

// PersistedDoc is a document as web returns it.
type PersistedDoc struct {
	Lines                []string
	Version              int64
	Ranges               json.RawMessage
	Pathname             string
	ProjectHistoryID     string
	HistoryRangesSupport bool
	ResolvedCommentIDs   []string
	// OTMigrationStage is non-zero for a project that has been migrated to
	// history-ot. This port does not implement that type, so such a document is
	// refused rather than loaded as though it were a ShareJS one.
	OTMigrationStage int
}

// webDoc is the wire form.
type webDoc struct {
	Lines                []string        `json:"lines"`
	Version              *int64          `json:"version"`
	Ranges               json.RawMessage `json:"ranges"`
	Pathname             *string         `json:"pathname"`
	ProjectHistoryID     json.RawMessage `json:"projectHistoryId"`
	HistoryRangesSupport bool            `json:"historyRangesSupport"`
	ResolvedCommentIDs   []string        `json:"resolvedCommentIds"`
	OTMigrationStage     int             `json:"otMigrationStage"`
}

// PersistenceClient reads and writes documents through web, which fronts the
// database.
type PersistenceClient struct {
	baseURL  string
	user     string
	password string
	client   *http.Client
}

// NewPersistenceClient builds a client for the internal API of web.
func NewPersistenceClient(baseURL, user, password string) *PersistenceClient {
	return &PersistenceClient{
		baseURL: baseURL, user: user, password: password,
		client: &http.Client{Timeout: maxHTTPRequestLength},
	}
}

// GetDoc fetches a document. peek asks web not to count the read, which the
// history resync uses.
func (c *PersistenceClient) GetDoc(ctx context.Context, projectID, docID string, peek bool) (*PersistedDoc, error) {
	var doc *PersistedDoc
	err := c.withRetries(ctx, func() error {
		var err error
		doc, err = c.getDocOnce(ctx, projectID, docID, peek)
		return err
	})
	return doc, err
}

func (c *PersistenceClient) getDocOnce(ctx context.Context, projectID, docID string, peek bool) (*PersistedDoc, error) {
	endpoint, err := url.JoinPath(c.baseURL, "project", projectID, "doc", docID)
	if err != nil {
		return nil, err
	}
	if peek {
		endpoint += "?peek=true"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.user, c.password)

	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	if err := statusError(res.StatusCode, projectID, docID); err != nil {
		return nil, err
	}

	var body webDoc
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDocumentValidation, err)
	}
	// Each of these is checked separately because the answer says which part of
	// web is misbehaving, and all three have been seen missing.
	if body.Lines == nil {
		return nil, fmt.Errorf("%w: no doc lines", ErrDocumentValidation)
	}
	if body.Version == nil {
		return nil, fmt.Errorf("%w: no valid doc version", ErrDocumentValidation)
	}
	if body.Pathname == nil {
		return nil, fmt.Errorf("%w: no valid doc pathname", ErrDocumentValidation)
	}

	return &PersistedDoc{
		Lines:                body.Lines,
		Version:              *body.Version,
		Ranges:               body.Ranges,
		Pathname:             *body.Pathname,
		ProjectHistoryID:     stringifyID(body.ProjectHistoryID),
		HistoryRangesSupport: body.HistoryRangesSupport,
		ResolvedCommentIDs:   body.ResolvedCommentIDs,
		OTMigrationStage:     body.OTMigrationStage,
	}, nil
}

// SetDoc writes a document back through web.
func (c *PersistenceClient) SetDoc(
	ctx context.Context, projectID, docID string, lines []string, version int64,
	ranges json.RawMessage, lastUpdatedAt string, lastUpdatedBy string,
) error {
	return c.withRetries(ctx, func() error {
		return c.setDocOnce(ctx, projectID, docID, lines, version, ranges, lastUpdatedAt, lastUpdatedBy)
	})
}

func (c *PersistenceClient) setDocOnce(
	ctx context.Context, projectID, docID string, lines []string, version int64,
	ranges json.RawMessage, lastUpdatedAt string, lastUpdatedBy string,
) error {
	endpoint, err := url.JoinPath(c.baseURL, "project", projectID, "doc", docID)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"lines":         lines,
		"ranges":        ranges,
		"version":       version,
		"lastUpdatedBy": lastUpdatedBy,
		"lastUpdatedAt": lastUpdatedAt,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.user, c.password)

	res, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))

	return statusError(res.StatusCode, projectID, docID)
}

// statusError maps a response code to the error the rest of the service
// distinguishes.
func statusError(status int, projectID, docID string) error {
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusNotFound:
		return fmt.Errorf("%w: doc %s in project %s", ErrNotFound, docID, projectID)
	case status == http.StatusRequestEntityTooLarge:
		return fmt.Errorf("%w: doc %s", ErrFileTooLarge, docID)
	default:
		return &WebAPIError{Status: status}
	}
}

// WebAPIError carries the status so the retry logic can tell a temporary
// failure from a permanent one.
type WebAPIError struct{ Status int }

func (e *WebAPIError) Error() string {
	return fmt.Sprintf("%s: status %d", ErrWebAPIServer, e.Status)
}

// Unwrap lets errors.Is match ErrWebAPIServer.
func (e *WebAPIError) Unwrap() error { return ErrWebAPIServer }

// isRetryable reports whether another attempt could succeed.
//
// Rate limits and server errors are worth retrying; a 404 or a malformed
// document is not, and retrying it only burns the lock.
func isRetryable(err error) bool {
	var webErr *WebAPIError
	if errors.As(err, &webErr) {
		return webErr.Status == http.StatusTooManyRequests ||
			(webErr.Status >= 500 && webErr.Status < 600)
	}
	if errors.Is(err, ErrDocumentValidation) || errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrFileTooLarge) {
		return false
	}
	// What is left is a transport failure: a refused connection, a reset, a
	// timeout. Those are the cases a second attempt exists for.
	return err != nil
}

func (c *PersistenceClient) withRetries(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		if attempt == maxAttempts-1 || !isRetryable(err) {
			return err
		}
		select {
		case <-time.After(retryDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

// stringifyID renders a project history id, which web sends as either a number
// or a string depending on how old the project is.
func stringifyID(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}
	return string(bytes.Trim(raw, `"`))
}
