package projecthistory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// The project's history is stored under an id of its own, not the project id,
// and only web knows which is which. So this service asks, and remembers the
// answer: the mapping never changes once a project has a history, and asking
// on every batch of updates would put a request to web in front of every
// flush.
//
// Asking web to resync a project goes the same way. That is web's job because
// the editor's copy of the project is the one that is right; this service can
// only say that its copy no longer matches.

// WebAPIConfig is where web is and how to authenticate to it.
type WebAPIConfig struct {
	URL  string
	User string
	Pass string
}

// WebAPI talks to web.
type WebAPI struct {
	config WebAPIConfig
	client *http.Client
	redis  *RedisStore
	// retryInterval is how long to wait before asking a second time, which is
	// short enough that a flush does not stall on it and long enough for a
	// restarting web to come back.
	retryInterval time.Duration
}

// NewWebAPI builds a client.
func NewWebAPI(config WebAPIConfig, redis *RedisStore) *WebAPI {
	return &WebAPI{
		config: config,
		redis:  redis,
		client: &http.Client{Transport: &http.Transport{
			MaxIdleConns:        32,
			MaxIdleConnsPerHost: 8,
			IdleConnTimeout:     90 * time.Second,
		}},
		retryInterval: 5 * time.Second,
	}
}

// GetHistoryID returns the id a project's history is stored under, or "" when
// the project has none.
func (w *WebAPI) GetHistoryID(ctx context.Context, projectID string) (string, error) {
	if cached, err := w.redis.GetCachedHistoryID(ctx, projectID); err == nil && cached != "" {
		return cached, nil
	}

	details, err := w.projectDetails(ctx, projectID)
	if err != nil {
		return "", err
	}

	historyID := details.Overleaf.History.ID.String()
	if historyID != "" {
		// Best effort: a history id that could not be cached is one more
		// request to web next time, not a failure.
		_ = w.redis.SetCachedHistoryID(ctx, projectID, historyID)
	}
	return historyID, nil
}

// projectDetailsResponse is the part of a project web reports that this
// service reads.
type projectDetailsResponse struct {
	Overleaf struct {
		History struct {
			// The id is a string for a project whose history is in
			// history-v1 and a number for one still in the old store.
			ID json.Number `json:"id"`
		} `json:"history"`
	} `json:"overleaf"`
}

// projectDetails asks web about a project, trying twice.
//
// A project that is not there is not worth retrying, and anything else might
// be web restarting.
func (w *WebAPI) projectDetails(ctx context.Context,
	projectID string) (*projectDetailsResponse, error) {

	target := fmt.Sprintf("%s/project/%s/details",
		strings.TrimSuffix(w.config.URL, "/"), projectID)

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(w.retryInterval):
			}
		}

		payload, err := w.do(ctx, http.MethodGet, target, nil, 16*time.Second)
		if err != nil {
			if StatusCodeOf(err) == http.StatusNotFound {
				return nil, fmt.Errorf("%w: got a 404 from web api", ErrNotFound)
			}
			lastErr = err
			continue
		}

		var details projectDetailsResponse
		if err := json.Unmarshal(payload, &details); err != nil {
			return nil, err
		}
		return &details, nil
	}
	return nil, lastErr
}

// ResyncOptions are the things a resync request can ask for.
type ResyncOptions struct {
	// HistoryRangesMigration is "forwards" or "backwards" when the resync is
	// part of moving a project's tracked changes into the history.
	HistoryRangesMigration string
	// ResyncProjectStructureOnly asks for the file tree without the contents,
	// which is enough when only the tree is out of step.
	ResyncProjectStructureOnly bool
}

// RequestResync asks web to send the project again.
func (w *WebAPI) RequestResync(ctx context.Context, projectID string,
	options ResyncOptions) error {

	body := map[string]any{}
	if options.HistoryRangesMigration != "" {
		body["historyRangesMigration"] = options.HistoryRangesMigration
	}
	if options.ResyncProjectStructureOnly {
		body["resyncProjectStructureOnly"] = options.ResyncProjectStructureOnly
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}

	target := fmt.Sprintf("%s/project/%s/history/resync",
		strings.TrimSuffix(w.config.URL, "/"), projectID)
	// Six minutes, because web reads the whole project out of the database and
	// writes it to the queue before it answers.
	_, err = w.do(ctx, http.MethodPost, target, encoded, 6*time.Minute)
	if err != nil {
		if StatusCodeOf(err) == http.StatusNotFound {
			return fmt.Errorf("%w: got a 404 from web api", ErrNotFound)
		}
		return err
	}
	return nil
}

// do makes one request to web.
func (w *WebAPI) do(ctx context.Context, method, target string, body []byte,
	timeout time.Duration) ([]byte, error) {

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	var req *http.Request
	var err error
	if reader != nil {
		req, err = http.NewRequestWithContext(ctx, method, target, reader)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, target, nil)
	}
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(w.config.User, w.config.Pass)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := w.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	payload, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, &StatusError{
			Method: method, URL: target, StatusCode: res.StatusCode,
			Body: string(payload),
		}
	}
	return payload, nil
}
