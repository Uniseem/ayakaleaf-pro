package documents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Resync hands document-updater a project's whole file tree, to be written
// into the history.
//
// The way back when the history's idea of a project has drifted from the
// project itself: rather than working out the difference, the tree is sent
// again and the history is rebuilt from it.
func (c *Client) Resync(
	ctx context.Context,
	projectID bson.ObjectID,
	historyID string,
	docs, files []map[string]any,
	rangesMigration string,
	structureOnly bool,
) error {
	body := map[string]any{
		"projectHistoryId": historyID,
		"docs":             docs,
		"files":            files,
	}
	if rangesMigration != "" {
		body["historyRangesMigration"] = rangesMigration
	}
	if structureOnly {
		body["resyncProjectStructureOnly"] = true
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf("%s/project/%s/history/resync", c.baseURL, projectID.Hex())
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
