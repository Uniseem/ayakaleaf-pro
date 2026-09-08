package realtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Errors mirroring services/real-time/app/js/Errors.js, because the client
// distinguishes them: an unauthorised join is final, a rate limit is worth
// retrying.
var (
	ErrNotAuthorized = errors.New("not authorized")
	ErrCorruptedJoin = errors.New("no data returned from joinProject request")
)

// ErrProjectNotFound and ErrTooManyRequests carry codes because the editor
// branches on them: it shows "project not found" for one and retries the
// other.
var (
	ErrProjectNotFound = CodedError("project not found", "ProjectNotFound")
	ErrTooManyRequests = CodedError("rate-limit hit when joining project", "TooManyRequests")
)

// JoinProjectResult is what web answers with. The project document is passed
// through to the client untouched -- real-time does not model it.
type JoinProjectResult struct {
	Project          json.RawMessage `json:"project"`
	PrivilegeLevel   string          `json:"privilegeLevel"`
	IsRestrictedUser bool            `json:"isRestrictedUser"`
	IsTokenMember    bool            `json:"isTokenMember"`
	IsInvitedMember  bool            `json:"isInvitedMember"`
}

// OwnerID reads the project owner out of the opaque project document, which is
// the one field real-time keeps.
func (r *JoinProjectResult) OwnerID() string {
	var doc struct {
		Owner *struct {
			ID string `json:"_id"`
		} `json:"owner"`
	}
	if err := json.Unmarshal(r.Project, &doc); err != nil || doc.Owner == nil {
		return ""
	}
	return doc.Owner.ID
}

// WebClient calls the internal API of web.
type WebClient struct {
	baseURL  string
	user     string
	password string
	client   *http.Client
}

// NewWebClient builds a client for the internal API of web.
func NewWebClient(baseURL, user, password string) *WebClient {
	return &WebClient{
		baseURL: baseURL, user: user, password: password,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// JoinProject asks web whether a user may open a project, and for the project
// document to hand the editor.
//
// This is the authorisation decision for the whole connection: real-time does
// not read permissions itself.
func (c *WebClient) JoinProject(ctx context.Context, projectID string, user *User) (*JoinProjectResult, error) {
	userID := ""
	anonToken := ""
	if user != nil {
		userID = user.ID
		anonToken = user.AnonymousAccessToken
	}
	body, err := json.Marshal(map[string]any{
		"userId": userID, "anonymousAccessToken": anonToken,
	})
	if err != nil {
		return nil, err
	}

	endpoint, err := url.JoinPath(c.baseURL, "project", projectID, "join")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.user, c.password)

	res, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("join project request failed: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusTooManyRequests:
		return nil, ErrTooManyRequests
	case http.StatusForbidden:
		return nil, ErrNotAuthorized
	case http.StatusNotFound:
		return nil, ErrProjectNotFound
	default:
		return nil, fmt.Errorf("non-success status code from web: %d", res.StatusCode)
	}

	var result JoinProjectResult
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, ErrCorruptedJoin
	}
	if len(result.Project) == 0 || string(result.Project) == "null" {
		return nil, ErrCorruptedJoin
	}
	return &result, nil
}

// Doc is a document as document-updater returns it.
type Doc struct {
	Lines   []json.RawMessage `json:"lines"`
	Version int64             `json:"version"`
	Ranges  json.RawMessage   `json:"ranges"`
	Ops     json.RawMessage   `json:"ops"`
	TTLInS  int64             `json:"ttlInS"`
	Type    string            `json:"type"`
}

// DocumentUpdaterClient calls the internal API of document-updater.
type DocumentUpdaterClient struct {
	baseURL string
	client  *http.Client
}

// NewDocumentUpdaterClient builds a document-updater client.
func NewDocumentUpdaterClient(baseURL string) *DocumentUpdaterClient {
	return &DocumentUpdaterClient{baseURL: baseURL, client: &http.Client{Timeout: 30 * time.Second}}
}

// GetDocument fetches a document and the operations since fromVersion, which
// is what lets a reconnecting client catch up rather than reload. A
// fromVersion of -1 asks for the document without any operations.
func (c *DocumentUpdaterClient) GetDocument(ctx context.Context, projectID, docID string, fromVersion int64) (*Doc, error) {
	endpoint := fmt.Sprintf("%s/project/%s/doc/%s?fromVersion=%s&historyOTSupport=true",
		c.baseURL, url.PathEscape(projectID), url.PathEscape(docID),
		strconv.FormatInt(fromVersion, 10))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusUnprocessableEntity:
		// The client asked to resume from a version document-updater no longer
		// holds. The editor reacts by reloading the document from scratch, so
		// this is a normal outcome rather than a fault.
		return nil, ErrMissingOps
	default:
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return nil, fmt.Errorf("doc updater returned a non-success status code: %d: %s",
			res.StatusCode, body)
	}

	var doc Doc
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// CheckDocument verifies a document exists in a project, without asking for
// its operations.
func (c *DocumentUpdaterClient) CheckDocument(ctx context.Context, projectID, docID string) error {
	_, err := c.GetDocument(ctx, projectID, docID, -1)
	return err
}

// FlushProject asks document-updater to write a project out and forget it,
// which real-time does once the last client leaves.
func (c *DocumentUpdaterClient) FlushProject(ctx context.Context, projectID string, shuttingDown bool) error {
	endpoint := fmt.Sprintf("%s/project/%s?background=true", c.baseURL, url.PathEscape(projectID))
	if shuttingDown {
		endpoint += "&shutdown=true"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	res, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("doc updater returned a non-success status code: %d", res.StatusCode)
	}
	return nil
}
