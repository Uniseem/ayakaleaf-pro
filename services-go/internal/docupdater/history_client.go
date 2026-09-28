package docupdater

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// HistoryClient asks project-history to work through its queue.
//
// The queue itself is in Redis and project-history drains it on its own; this
// only says "now", which matters when the project is about to be deleted or
// when somebody is waiting to read the history they just made.
type HistoryClient struct {
	baseURL string
	client  *http.Client
	log     *slog.Logger
}

// NewHistoryClient builds the client.
func NewHistoryClient(baseURL string, log *slog.Logger) *HistoryClient {
	return &HistoryClient{
		baseURL: baseURL,
		// Longer than a call to web: this waits for another service to work
		// through a queue rather than for a single database read.
		client: &http.Client{Timeout: 30 * time.Second},
		log:    log,
	}
}

// FlushOptions says how urgently the queue should be drained.
type FlushOptions struct {
	// Background asks project-history to get to it when it can.
	Background bool
	// SkipHistoryFlush skips the call. real-time sets it while shutting down,
	// where the queue will be drained by whoever picks the project up next.
	SkipHistoryFlush bool
}

// FlushProjectChanges asks project-history to drain a project's queue and waits
// for the answer.
func (c *HistoryClient) FlushProjectChanges(ctx context.Context, projectID string, opts FlushOptions) error {
	if opts.SkipHistoryFlush {
		c.log.Debug("skipping flush of project history", slog.String("project", projectID))
		return nil
	}
	endpoint, err := url.JoinPath(c.baseURL, "project", projectID, "flush")
	if err != nil {
		return err
	}
	if opts.Background {
		endpoint += "?background=true"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	res, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("project history api request failed for %s: %w", projectID, err)
	}
	defer func() { _ = res.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("project history api request failed for %s: status %d",
			projectID, res.StatusCode)
	}
	return nil
}

// FlushProjectChangesAsync does the same without waiting.
//
// The callers that use it are already holding a document lock or answering a
// request, and none of them can do anything with the answer.
func (c *HistoryClient) FlushProjectChangesAsync(ctx context.Context, projectID string) {
	// Detached from the request: the caller is about to answer and cancel it,
	// and the flush should still happen.
	ctx = context.WithoutCancel(ctx)
	go func() {
		if err := c.FlushProjectChanges(ctx, projectID, FlushOptions{Background: true}); err != nil {
			c.log.Error("failed to flush in background",
				slog.String("project", projectID), slog.String("err", err.Error()))
		}
	}()
}
