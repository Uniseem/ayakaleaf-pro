package projecthistory

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/histmodel"
)

// history-v1 is the service that actually stores the history: the chunks of
// changes, and the blobs the changes name. This is the client for it, and the
// only place this service talks to it.
//
// A blob is content addressed the way git addresses one: the sha1 of the bytes
// with a header in front saying how many there are. So a file already in the
// history can be recognised without sending it again, and two projects with
// the same file share one blob.

// ErrInvalidUpdateForBlob is an update that names no content to store.
var ErrInvalidUpdateForBlob = errors.New("invalid update for blob creation")

// ErrInvalidFileForBlob is a file url that is not one this can read.
var ErrInvalidFileForBlob = errors.New("invalid file for blob creation")

// ErrInvalidProjectForBlob is a file url belonging to another project.
var ErrInvalidProjectForBlob = errors.New("invalid project for blob creation")

// ErrFilestoreDisabled is a file that would have to be read from filestore
// when reading from filestore has been turned off.
var ErrFilestoreDisabled = errors.New("blocking filestore read")

// ErrNoFilestoreURL is a file with no url and no blob already stored.
var ErrNoFilestoreURL = errors.New("no filestore URL provided and blob was not created")

// ErrUnexpectedResponse is a chunk that came back in a shape this cannot read.
var ErrUnexpectedResponse = errors.New("unexpected response")

// StatusError is a request the history store refused.
//
// The message carries the status code because that is how the caller tells a
// project that is missing from one that is merely unreachable, and the error
// recorder reads it back out of what it stored.
type StatusError struct {
	Method     string
	URL        string
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("history store a non-success status code: %d", e.StatusCode)
}

// StatusCodeOf reports the status an error carries, or zero.
func StatusCodeOf(err error) int {
	var status *StatusError
	if errors.As(err, &status) {
		return status.StatusCode
	}
	return 0
}

// HistoryStoreConfig is where history-v1 and filestore are and how to reach
// them.
type HistoryStoreConfig struct {
	// Host is the base url of history-v1, including its /api prefix.
	Host string
	User string
	Pass string
	// Timeout is how long any one request may take. Storing a large file is
	// one request, so this is minutes rather than seconds.
	Timeout time.Duration

	FilestoreURL     string
	FilestoreEnabled bool
	// UploadFolder is where a file is buffered while its hash is computed.
	UploadFolder string
}

// HistoryStore talks to history-v1.
type HistoryStore struct {
	config HistoryStoreConfig
	client *http.Client
}

// NewHistoryStore builds a client.
func NewHistoryStore(config HistoryStoreConfig) *HistoryStore {
	if config.Timeout <= 0 {
		config.Timeout = 300 * time.Second
	}
	if config.UploadFolder == "" {
		config.UploadFolder = os.TempDir()
	}
	return &HistoryStore{
		config: config,
		client: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        64,
				MaxIdleConnsPerHost: 16,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// buildURL puts a path and a query onto the host.
func (s *HistoryStore) buildURL(path string, query url.Values) string {
	base := strings.TrimSuffix(s.config.Host, "/")
	built := base + "/" + strings.TrimPrefix(path, "/")
	if len(query) > 0 {
		built += "?" + query.Encode()
	}
	return built
}

// request makes one call to the history store and hands back the body.
func (s *HistoryStore) request(ctx context.Context, method, path string,
	query url.Values, body io.Reader, contentType string,
	contentLength int64) ([]byte, error) {

	target := s.buildURL(path, query)

	ctx, cancel := context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(s.config.User, s.config.Pass)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if contentLength >= 0 {
		req.ContentLength = contentLength
	}

	res, err := s.client.Do(req)
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

// getJSON makes a GET and reads the body as JSON.
func (s *HistoryStore) getJSON(ctx context.Context, path string,
	query url.Values, into any) error {

	payload, err := s.request(ctx, http.MethodGet, path, query, nil, "", -1)
	if err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	return json.Unmarshal(payload, into)
}

// ChunkResponse is a chunk as the history store returns it.
type ChunkResponse struct {
	Chunk *histmodel.Chunk `json:"chunk"`
}

// GetMostRecentChunk fetches the end of a project's history.
func (s *HistoryStore) GetMostRecentChunk(ctx context.Context,
	historyID string) (*ChunkResponse, error) {

	return s.requestChunk(ctx, fmt.Sprintf("projects/%s/latest/history", historyID))
}

// GetChunkAtVersion fetches the chunk a version falls in.
func (s *HistoryStore) GetChunkAtVersion(ctx context.Context, historyID string,
	version int) (*ChunkResponse, error) {

	return s.requestChunk(ctx,
		fmt.Sprintf("projects/%s/versions/%d/history", historyID, version))
}

// requestChunk fetches a chunk and checks it is one.
func (s *HistoryStore) requestChunk(ctx context.Context, path string) (*ChunkResponse, error) {
	var response ChunkResponse
	if err := s.getJSON(ctx, path, nil, &response); err != nil {
		return nil, err
	}
	if response.Chunk == nil || response.Chunk.History == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnexpectedResponse, path)
	}
	return &response, nil
}

// RawVersion is where a project's history has got to, without the changes.
type RawVersion struct {
	StartVersion int       `json:"startVersion"`
	EndVersion   int       `json:"endVersion"`
	EndTimestamp time.Time `json:"endTimestamp"`
}

// GetMostRecentVersionRaw asks only how far the history goes.
func (s *HistoryStore) GetMostRecentVersionRaw(ctx context.Context,
	historyID string, readOnly bool) (*RawVersion, error) {

	query := url.Values{}
	if readOnly {
		query.Set("readOnly", "true")
	}
	var version RawVersion
	err := s.getJSON(ctx, fmt.Sprintf("projects/%s/latest/history/raw", historyID),
		query, &version)
	if err != nil {
		return nil, err
	}
	return &version, nil
}

// DocVersions is where document-updater had got to for each document.
type DocVersions map[string]docVersion

// ProjectStructureAndDocVersions is what the editor's side of the project was
// at when the history was last written.
type ProjectStructureAndDocVersions struct {
	Project string      `json:"project"`
	Docs    DocVersions `json:"docs"`
}

// MostRecentVersion is everything the sync needs about the end of the history.
type MostRecentVersion struct {
	Version    int
	Versions   ProjectStructureAndDocVersions
	LastChange *histmodel.Change
	Chunk      *ChunkResponse
}

// GetMostRecentVersion reads the end of the history and works out what the
// project was at.
//
// An error here does not mean nothing was read: the versions are returned
// alongside it, because a history whose versions are out of order is still a
// history the caller has to do something with.
func (s *HistoryStore) GetMostRecentVersion(ctx context.Context, projectID,
	historyID string) (*MostRecentVersion, error) {

	chunk, err := s.GetMostRecentChunk(ctx, historyID)
	if err != nil {
		return nil, err
	}

	changes := chunk.Chunk.History.Changes
	result := &MostRecentVersion{
		Version: chunk.Chunk.StartVersion + len(changes),
		Chunk:   chunk,
	}
	if len(changes) > 0 {
		// The last change by time, not by position: the ordering that matters
		// here is when they were made.
		sorted := append([]*histmodel.Change(nil), changes...)
		sort.SliceStable(sorted, func(i, j int) bool {
			return sorted[i].Timestamp.Before(sorted[j].Timestamp)
		})
		result.LastChange = sorted[len(sorted)-1]
	}

	projectVersion, projectErr := latestProjectVersion(chunk)
	docVersions, docErr := latestDocVersions(chunk)
	result.Versions = ProjectStructureAndDocVersions{
		Project: projectVersion, Docs: docVersions,
	}
	if projectErr != nil {
		return result, projectErr
	}
	return result, docErr
}

// latestProjectVersion is the highest project version in a chunk.
func latestProjectVersion(chunk *ChunkResponse) (string, error) {
	version := ""
	if chunk.Chunk.History.Snapshot != nil {
		version = chunk.Chunk.History.Snapshot.ProjectVersion
	}
	var firstErr error
	for index, change := range chunk.Chunk.History.Changes {
		if change.ProjectVersion == "" {
			continue
		}
		if version != "" && versionLess(change.ProjectVersion, version) {
			if firstErr == nil {
				firstErr = fmt.Errorf("%w: change %d is at %s, after %s",
					ErrOpsOutOfOrder, index, change.ProjectVersion, version)
			}
			continue
		}
		version = change.ProjectVersion
	}
	return version, firstErr
}

// latestDocVersions is the highest version of each document in a chunk.
func latestDocVersions(chunk *ChunkResponse) (DocVersions, error) {
	versions := DocVersions{}
	if chunk.Chunk.History.Snapshot != nil {
		if err := decodeDocVersions(chunk.Chunk.History.Snapshot.V2DocVersions,
			versions); err != nil {
			return nil, err
		}
	}

	var firstErr error
	for _, change := range chunk.Chunk.History.Changes {
		inChange := DocVersions{}
		if err := decodeDocVersions(change.V2DocVersions, inChange); err != nil {
			return nil, err
		}
		// In the order the document ids appear, which is the order the other
		// side walks them in.
		for _, docID := range orderedKeys(change.V2DocVersions) {
			info := inChange[docID]
			if existing, ok := versions[docID]; ok && info.V < existing.V {
				if firstErr == nil {
					firstErr = fmt.Errorf("%w: doc %s is at %d, after %d",
						ErrOpsOutOfOrder, docID, info.V, existing.V)
				}
				continue
			}
			versions[docID] = info
		}
	}
	return versions, firstErr
}

// decodeDocVersions reads a document version map, leaving it as it was when
// there is none.
func decodeDocVersions(raw json.RawMessage, into DocVersions) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	decoded := DocVersions{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	for key, value := range decoded {
		into[key] = value
	}
	return nil
}

// orderedKeys is the keys of a JSON object in the order they were written,
// which is the order the other side reads them in.
func orderedKeys(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if _, err := decoder.Token(); err != nil {
		return nil
	}
	var keys []string
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return keys
		}
		key, ok := token.(string)
		if !ok {
			return keys
		}
		keys = append(keys, key)
		var skipped json.RawMessage
		if err := decoder.Decode(&skipped); err != nil {
			return keys
		}
	}
	return keys
}

// GetProjectBlob reads a blob's contents.
func (s *HistoryStore) GetProjectBlob(ctx context.Context, historyID,
	hash string) ([]byte, error) {

	return s.request(ctx, http.MethodGet,
		fmt.Sprintf("projects/%s/blobs/%s", historyID, hash), nil, nil, "", -1)
}

// GetProjectBlobStream opens a blob without reading it into memory. The caller
// closes the body.
func (s *HistoryStore) GetProjectBlobStream(ctx context.Context, historyID,
	hash string) (io.ReadCloser, error) {

	target := s.buildURL(fmt.Sprintf("projects/%s/blobs/%s", historyID, hash), nil)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(s.config.User, s.config.Pass)

	res, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		res.Body.Close()
		return nil, &StatusError{
			Method: http.MethodGet, URL: target, StatusCode: res.StatusCode,
			Body: string(payload),
		}
	}
	return res.Body, nil
}

// SendChangesResult is what the history store says after taking changes.
type SendChangesResult struct {
	ResyncNeeded bool `json:"resyncNeeded"`
}

// SendChanges appends changes to a project's history.
func (s *HistoryStore) SendChanges(ctx context.Context, historyID string,
	changes []*histmodel.Change, endVersion int) (*SendChangesResult, error) {

	body, err := json.Marshal(changes)
	if err != nil {
		return nil, err
	}
	query := url.Values{"end_version": []string{strconv.Itoa(endVersion)}}
	payload, err := s.request(ctx, http.MethodPost,
		fmt.Sprintf("projects/%s/legacy_changes", historyID), query,
		bytes.NewReader(body), "application/json", int64(len(body)))
	if err != nil {
		return nil, err
	}

	result := &SendChangesResult{}
	if len(payload) > 0 {
		// A history store that answers with nothing has taken the changes and
		// asked for no resync, which is the default.
		_ = json.Unmarshal(payload, result)
	}
	return result, nil
}

// InitializeProject makes a history for a project and returns its id.
func (s *HistoryStore) InitializeProject(ctx context.Context,
	historyID string) (string, error) {

	var body []byte
	var err error
	if historyID == "" {
		body = []byte("true")
	} else {
		body, err = json.Marshal(map[string]string{"projectId": historyID})
		if err != nil {
			return "", err
		}
	}

	payload, err := s.request(ctx, http.MethodPost, "projects", nil,
		bytes.NewReader(body), "application/json", int64(len(body)))
	if err != nil {
		return "", err
	}

	var project struct {
		ProjectID json.RawMessage `json:"projectId"`
	}
	if err := json.Unmarshal(payload, &project); err != nil {
		return "", err
	}
	id := jsonScalarString(project.ProjectID)
	if id == "" {
		return "", errors.New("history store did not return a project id")
	}
	return id, nil
}

// DeleteProject removes a project's history.
func (s *HistoryStore) DeleteProject(ctx context.Context, historyID string) error {
	_, err := s.request(ctx, http.MethodDelete,
		fmt.Sprintf("projects/%s", historyID), nil, nil, "", -1)
	return err
}

// CloneProject copies one project's history onto another.
func (s *HistoryStore) CloneProject(ctx context.Context, sourceID,
	targetID string) error {

	body, err := json.Marshal(map[string]string{"targetProjectId": targetID})
	if err != nil {
		return err
	}
	_, err = s.request(ctx, http.MethodPost,
		fmt.Sprintf("projects/%s/clone", sourceID), nil,
		bytes.NewReader(body), "application/json", int64(len(body)))
	return err
}

// blobExists asks whether a blob is already stored, without fetching it.
func (s *HistoryStore) blobExists(ctx context.Context, historyID,
	hash string) (bool, error) {

	if hash == "" {
		return false, nil
	}
	_, err := s.request(ctx, http.MethodHead,
		fmt.Sprintf("projects/%s/blobs/%s", historyID, hash), nil, nil, "", -1)
	if err != nil {
		if StatusCodeOf(err) == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// BlobHashes are the blobs one update's contents were stored in.
type BlobHashes struct {
	File   string `json:"file,omitempty"`
	Ranges string `json:"ranges,omitempty"`
}

// filestorePath matches the url of a file in filestore.
var filestorePath = regexp.MustCompile(`^/project/([0-9a-f]{24})/file/([0-9a-f]{24})$`)

// rewriteFilestoreURL points a file url at the filestore this service is
// configured with, so that a project stored in one place is not fetched from
// another.
func (s *HistoryStore) rewriteFilestoreURL(rawURL, projectID string) (string, string, error) {
	if rawURL == "" {
		return "", "", nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", "", ErrInvalidFileForBlob
	}
	match := filestorePath.FindStringSubmatch(parsed.Path)
	if match == nil {
		return "", "", ErrInvalidFileForBlob
	}
	if match[1] != projectID {
		return "", "", ErrInvalidProjectForBlob
	}
	fileID := match[2]
	return fmt.Sprintf("%s/project/%s/file/%s",
		strings.TrimSuffix(s.config.FilestoreURL, "/"), projectID, fileID), fileID, nil
}

// CreateBlobForUpdate stores whatever content an update carries and returns
// the hashes it was stored under.
func (s *HistoryStore) CreateBlobForUpdate(ctx context.Context, projectID,
	historyID string, update *Update) (*BlobHashes, error) {

	switch {
	case update.Doc != "" && hasField(update, "docLines"):
		return s.createDocBlobs(ctx, projectID, historyID, update)

	case hasField(update, "file") &&
		(hasField(update, "url") || isTrue(update.Rest["createdBlob"])):
		return s.createFileBlob(ctx, projectID, historyID, update)
	}
	return nil, ErrInvalidUpdateForBlob
}

// createDocBlobs stores a document's text, and its marks if it has any.
func (s *HistoryStore) createDocBlobs(ctx context.Context, projectID,
	historyID string, update *Update) (*BlobHashes, error) {

	ranges, err := CreateRangeBlobDataFromUpdate(update)
	if err != nil {
		return nil, err
	}

	docLines := rawString(update.Rest["docLines"])
	fileHash, err := s.createBlobFromBytes(ctx, historyID, []byte(docLines))
	if err != nil {
		return nil, err
	}
	if ranges == nil {
		return &BlobHashes{File: fileHash}, nil
	}

	encoded, err := json.Marshal(ranges)
	if err != nil {
		return nil, err
	}
	rangesHash, err := s.createBlobFromBytes(ctx, historyID, encoded)
	if err != nil {
		return nil, err
	}
	return &BlobHashes{File: fileHash, Ranges: rangesHash}, nil
}

// createFileBlob stores a binary file, fetching it from filestore unless it is
// already in the history.
func (s *HistoryStore) createFileBlob(ctx context.Context, projectID,
	historyID string, update *Update) (*BlobHashes, error) {

	hash := rawString(update.Rest["hash"])
	filestoreURL, _, err := s.rewriteFilestoreURL(
		rawString(update.Rest["url"]), projectID)
	if err != nil {
		return nil, err
	}

	exists, err := s.blobExists(ctx, historyID, hash)
	if err != nil {
		return nil, fmt.Errorf("error checking whether blob exists: %w", err)
	}
	if exists {
		return &BlobHashes{File: hash}, nil
	}

	if filestoreURL == "" {
		return nil, ErrNoFilestoreURL
	}
	if !s.config.FilestoreEnabled {
		return nil, ErrFilestoreDisabled
	}

	content, err := s.fetchFilestore(ctx, filestoreURL)
	if err != nil {
		return nil, err
	}
	// A file filestore has lost is stored as an empty one rather than left to
	// fail: the project still has to be able to move forward.
	fileHash, err := s.createBlobFromReader(ctx, historyID, content.reader, content.length)
	content.close()
	if err != nil {
		return nil, err
	}
	return &BlobHashes{File: fileHash}, nil
}

// filestoreContent is a file being read out of filestore.
type filestoreContent struct {
	reader io.Reader
	length int64
	close  func()
}

// fetchFilestore opens a file in filestore, treating one that is not there as
// an empty file.
func (s *HistoryStore) fetchFilestore(ctx context.Context,
	target string) (*filestoreContent, error) {

	ctx, cancel := context.WithTimeout(ctx, s.config.Timeout)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	res, err := s.client.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("error from filestore %s: %w", target, err)
	}
	if res.StatusCode == http.StatusNotFound {
		res.Body.Close()
		cancel()
		return &filestoreContent{reader: bytes.NewReader(nil), length: 0,
			close: func() {}}, nil
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		res.Body.Close()
		cancel()
		return nil, fmt.Errorf("error from filestore %s: %w", target,
			&StatusError{Method: http.MethodGet, URL: target,
				StatusCode: res.StatusCode, Body: string(payload)})
	}
	return &filestoreContent{
		reader: res.Body, length: res.ContentLength,
		close: func() { res.Body.Close(); cancel() },
	}, nil
}

// createBlobFromBytes stores content that is already in memory.
func (s *HistoryStore) createBlobFromBytes(ctx context.Context, historyID string,
	content []byte) (string, error) {

	hash := BlobHash(content)
	err := s.putBlob(ctx, historyID, hash, bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return "", err
	}
	return hash, nil
}

// createBlobFromReader stores content that is arriving from somewhere else.
//
// The hash names the content with a header in front saying how long it is, so
// nothing can be sent until all of it has been seen. Content that is small
// enough is held in memory; anything larger is spooled to disk rather than
// kept there, because a file can be a hundred megabytes and several can be
// going at once.
func (s *HistoryStore) createBlobFromReader(ctx context.Context, historyID string,
	content io.Reader, length int64) (string, error) {

	buffered, err := io.ReadAll(io.LimitReader(content, maxInMemoryBlob+1))
	if err != nil {
		return "", err
	}
	if int64(len(buffered)) <= maxInMemoryBlob {
		return s.createBlobFromBytes(ctx, historyID, buffered)
	}

	file, err := os.CreateTemp(s.config.UploadFolder, "project-history-blob-")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	defer file.Close()

	written, err := io.Copy(file, io.MultiReader(bytes.NewReader(buffered), content))
	if err != nil {
		return "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	hasher := sha1.New()
	fmt.Fprintf(hasher, "blob %d\x00", written)
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	hash := hex.EncodeToString(hasher.Sum(nil))
	if err := s.putBlob(ctx, historyID, hash, file, written); err != nil {
		return "", err
	}
	return hash, nil
}

// maxInMemoryBlob is how much content is hashed without going to disk first.
const maxInMemoryBlob = 8 << 20

// putBlob sends content to the history store under its hash.
func (s *HistoryStore) putBlob(ctx context.Context, historyID, hash string,
	content io.Reader, length int64) error {

	_, err := s.request(ctx, http.MethodPut,
		fmt.Sprintf("projects/%s/blobs/%s", historyID, hash), nil,
		content, "", length)
	return err
}

// BlobHash is the name content is stored under: the sha1 of the content with a
// header saying how long it is, which is how git names a blob.
func BlobHash(content []byte) string {
	hasher := sha1.New()
	fmt.Fprintf(hasher, "blob %d\x00", len(content))
	hasher.Write(content)
	return hex.EncodeToString(hasher.Sum(nil))
}

// jsonScalarString reads a JSON value that may be a string or a number as a
// string, which is how project ids arrive: history-v1 numbers them, and
// everything else names them.
func jsonScalarString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		return number.String()
	}
	return ""
}

// isTrue reports whether a JSON value is true.
func isTrue(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "true"
}
