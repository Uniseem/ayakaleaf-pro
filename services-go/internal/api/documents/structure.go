package documents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Telling the history what changed.
//
// A file appearing in a project is not only a change to the project document:
// it is an event the history has to record, or the version somebody restores
// later will not have the file in it. document-updater owns that queue, so
// every change to the tree is reported to it here.
//
// The updates are built as plain maps rather than a struct with omitempty
// tags, because the difference between a field that is absent and one that is
// empty is meaningful downstream: a rename with an empty new path is a delete.

// StructureUpdate is one change to a project's tree.
type StructureUpdate map[string]any

// AddedDoc reports a new document.
//
// The text goes as one string and not as the lines it is stored in: that is
// what the history reads, and a list of lines arrives there as nothing at all.
// A document added with the wrong shape is recorded as an empty file, and
// every later edit to it fails to apply against text that is not there.
func AddedDoc(id bson.ObjectID, path string, lines []string) StructureUpdate {
	return StructureUpdate{
		"type": "add-doc", "id": id.Hex(), "pathname": path,
		"docLines": strings.Join(lines, "\n"),
	}
}

// AddedFile reports a new binary file, already written to the blob store.
func AddedFile(id bson.ObjectID, path, url, hash string) StructureUpdate {
	update := StructureUpdate{
		"type": "add-file", "id": id.Hex(), "pathname": path, "url": url,
	}
	if hash != "" {
		update["hash"] = hash
		// The blob is there already, so the history does not have to fetch the
		// file to work out what to store.
		update["createdBlob"] = true
	}
	return update
}

// RenamedDoc and RenamedFile report a move. An empty new path is a delete,
// which is why the field is always present.
func RenamedDoc(id bson.ObjectID, from, to string) StructureUpdate {
	return StructureUpdate{
		"type": "rename-doc", "id": id.Hex(), "pathname": from, "newPathname": to,
	}
}

func RenamedFile(id bson.ObjectID, from, to string) StructureUpdate {
	return StructureUpdate{
		"type": "rename-file", "id": id.Hex(), "pathname": from, "newPathname": to,
	}
}

// ReportStructure tells document-updater what changed.
//
// The version is the project's, after the change: the history orders by it, so
// reporting a change under a version that was already used puts it in the
// wrong place.
func (c *Client) ReportStructure(
	ctx context.Context,
	projectID bson.ObjectID,
	historyID string,
	userID bson.ObjectID,
	version int64,
	source string,
	updates []StructureUpdate,
) error {
	if len(updates) == 0 {
		return nil
	}
	payload, err := json.Marshal(map[string]any{
		"projectHistoryId": historyID,
		"userId":           userID.Hex(),
		"version":          version,
		"source":           source,
		"updates":          updates,
	})
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/project/%s", c.baseURL, projectID.Hex())
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
