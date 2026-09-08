package internalapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Which comments have been resolved.
//
// Comments live in chat and their positions live in the document's ranges, so
// neither service can answer this alone: the ids of the threads somebody
// resolved, narrowed to the ones this document actually has.

type chatClient struct {
	baseURL string
	http    *http.Client
}

func newChatClient(baseURL string) *chatClient {
	return &chatClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// resolvedIn is the resolved threads that appear in these ranges.
//
// Never an error: a document opens without knowing which of its comments are
// resolved, which is a worse-looking editor and not a broken one, and chat
// being down should not stop somebody editing.
func (c *chatClient) resolvedIn(ctx context.Context, projectID bson.ObjectID, ranges json.RawMessage) []string {
	inDoc := commentIDs(ranges)
	if len(inDoc) == 0 {
		return []string{}
	}

	endpoint := c.baseURL + "/project/" + projectID.Hex() + "/resolved-thread-ids"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return []string{}
	}
	response, err := c.http.Do(request)
	if err != nil {
		return []string{}
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return []string{}
	}

	var body struct {
		ResolvedThreadIDs []string `json:"resolvedThreadIds"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&body); err != nil {
		return []string{}
	}

	resolved := []string{}
	for _, id := range body.ResolvedThreadIDs {
		if inDoc[id] {
			resolved = append(resolved, id)
		}
	}
	return resolved
}

// commentIDs reads the comment ids out of a document's ranges.
func commentIDs(ranges json.RawMessage) map[string]bool {
	if len(ranges) == 0 {
		return nil
	}
	var parsed struct {
		Comments []struct {
			ID string `json:"id"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(ranges, &parsed); err != nil {
		return nil
	}
	ids := make(map[string]bool, len(parsed.Comments))
	for _, comment := range parsed.Comments {
		if comment.ID != "" {
			ids[comment.ID] = true
		}
	}
	return ids
}
