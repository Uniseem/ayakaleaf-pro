package projecthistory

import (
	"context"
	"fmt"
	"sync"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/histmodel"
)

// The history is stored as chunks: a snapshot of the whole project, and the
// changes made after it. So the project as it was at any version is that
// chunk's snapshot with the changes up to that version applied, and the
// project as it is now is the latest chunk's snapshot with all of them
// applied.
//
// Nothing here fetches file contents unless it is asked to. A snapshot names
// its files by the blobs they are stored in, and most questions -- which paths
// exist, what a file's hash is, whether it is text -- are answered from the
// names alone.

// maxSnapshotRequests is how many files are fetched at once.
const maxSnapshotRequests = 4

// blobStore reads blobs for one project's history.
type blobStore struct {
	store     *HistoryStore
	historyID string
}

// GetString returns a blob's contents.
func (b *blobStore) GetString(ctx context.Context, hash string) (string, error) {
	content, err := b.store.GetProjectBlob(ctx, b.historyID, hash)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// BlobStoreFor returns a reader for one project's blobs.
func (s *HistoryStore) BlobStoreFor(historyID string) histmodel.BlobStore {
	return &blobStore{store: s, historyID: historyID}
}

// Snapshots reads the project as it was at a version.
type Snapshots struct {
	store *HistoryStore
	web   *WebAPI
}

// NewSnapshots builds the reader.
func NewSnapshots(store *HistoryStore, web *WebAPI) *Snapshots {
	return &Snapshots{store: store, web: web}
}

// LatestSnapshotFromChunk is the project as the chunk leaves it, and the
// version it is then at.
func LatestSnapshotFromChunk(chunk *ChunkResponse) (*histmodel.Snapshot, int, error) {
	if chunk == nil || chunk.Chunk == nil {
		return nil, 0, fmt.Errorf("%w: no chunk", ErrUnexpectedResponse)
	}
	snapshot := chunk.Chunk.History.Snapshot
	if snapshot == nil {
		snapshot = histmodel.NewSnapshot()
	}
	for _, change := range chunk.Chunk.History.Changes {
		if err := snapshot.ApplyChange(change); err != nil {
			return nil, 0, err
		}
	}
	return snapshot, chunk.Chunk.EndVersion(), nil
}

// LatestSnapshotFilesForChunk is the files of the project as the chunk leaves
// it, each knowing at least how long it is.
func (s *Snapshots) LatestSnapshotFilesForChunk(ctx context.Context,
	historyID string, chunk *ChunkResponse) (*histmodel.FileMap, error) {

	snapshot, _, err := LatestSnapshotFromChunk(chunk)
	if err != nil {
		return nil, err
	}

	store := s.store.BlobStoreFor(historyID)
	for _, path := range snapshot.Files.Paths() {
		file := snapshot.Files.GetFile(path)
		if file == nil {
			return nil, fmt.Errorf("%w: %s has no content", ErrSync, path)
		}
		if err := file.LoadLazy(ctx, store); err != nil {
			return nil, err
		}
	}
	// The map keeps the order the files were written in, which is the order a
	// resync walks them in and so the order it queues its changes in.
	return snapshot.Files, nil
}

// LatestSnapshotFiles is the same for the project's latest chunk.
func (s *Snapshots) LatestSnapshotFiles(ctx context.Context, projectID,
	historyID string) (*histmodel.FileMap, error) {

	chunk, err := s.store.GetMostRecentChunk(ctx, historyID)
	if err != nil {
		return nil, err
	}
	return s.LatestSnapshotFilesForChunk(ctx, historyID, chunk)
}

// LatestSnapshot is the project as it is now, and the version it is at.
func (s *Snapshots) LatestSnapshot(ctx context.Context, projectID,
	historyID string) (*histmodel.Snapshot, int, error) {

	chunk, err := s.store.GetMostRecentChunk(ctx, historyID)
	if err != nil {
		return nil, 0, err
	}
	return LatestSnapshotFromChunk(chunk)
}

// SnapshotAtVersion is the project as it was at a version.
func (s *Snapshots) SnapshotAtVersion(ctx context.Context, projectID string,
	version int) (*histmodel.Snapshot, string, error) {

	historyID, err := s.web.GetHistoryID(ctx, projectID)
	if err != nil {
		return nil, "", err
	}
	chunk, err := s.store.GetChunkAtVersion(ctx, historyID, version)
	if err != nil {
		return nil, "", err
	}

	snapshot, err := chunk.Chunk.GetSnapshotAt(version)
	if err != nil {
		return nil, "", err
	}
	return snapshot, historyID, nil
}

// PathsAtVersion is the files the project had at a version.
func (s *Snapshots) PathsAtVersion(ctx context.Context, projectID string,
	version int) ([]string, error) {

	snapshot, _, err := s.SnapshotAtVersion(ctx, projectID, version)
	if err != nil {
		return nil, err
	}
	return snapshot.Files.Paths(), nil
}

// FileAtVersion is one file of the project as it was at a version, loaded.
func (s *Snapshots) FileAtVersion(ctx context.Context, projectID string,
	version int, pathname string) (*histmodel.File, string, error) {

	snapshot, historyID, err := s.SnapshotAtVersion(ctx, projectID, version)
	if err != nil {
		return nil, "", err
	}
	file := snapshot.Files.GetFile(pathname)
	if file == nil {
		return nil, "", fmt.Errorf("%w: %s", ErrNotFound, pathname)
	}
	return file, historyID, nil
}

// FileContentAtVersion is a text file's content as it was at a version, with
// the tracked deletions taken out, which is what the editor shows.
func (s *Snapshots) FileContentAtVersion(ctx context.Context, projectID string,
	version int, pathname string) (string, error) {

	file, historyID, err := s.FileAtVersion(ctx, projectID, version, pathname)
	if err != nil {
		return "", err
	}
	if !file.Data.IsEditable() {
		return "", fmt.Errorf("%w: %s is not text", ErrBadRequest, pathname)
	}
	if err := file.LoadEager(ctx, s.store.BlobStoreFor(historyID)); err != nil {
		return "", err
	}
	return file.StringData().GetContent(true), nil
}

// ProjectSnapshot is the whole project at a version: the content of every file
// that has been edited, and the hash of every one that has not.
type ProjectSnapshot struct {
	ProjectID string                     `json:"projectId"`
	Files     map[string]ProjectFileData `json:"files"`
}

// ProjectFileData is one file of a project snapshot.
type ProjectFileData struct {
	Data ProjectFileContent `json:"data"`
}

// ProjectFileContent is either a file's text or the blob it is stored in.
type ProjectFileContent struct {
	Content *string `json:"content,omitempty"`
	Hash    string  `json:"hash,omitempty"`
}

// GetProjectSnapshot is the whole project at a version, for the git bridge.
//
// Only the files that have been changed since their blob was written are
// fetched: everything else can be read from its blob by the caller, which
// knows how.
func (s *Snapshots) GetProjectSnapshot(ctx context.Context, projectID string,
	version int) (*ProjectSnapshot, error) {

	snapshot, historyID, err := s.SnapshotAtVersion(ctx, projectID, version)
	if err != nil {
		return nil, err
	}

	store := s.store.BlobStoreFor(historyID)
	paths := snapshot.Files.Paths()

	// Loaded a few at a time: a project can have a thousand files and the
	// history store is one service.
	var mutex sync.Mutex
	var firstErr error
	work := make(chan string)
	var waiting sync.WaitGroup
	for worker := 0; worker < maxSnapshotRequests; worker++ {
		waiting.Add(1)
		go func() {
			defer waiting.Done()
			for path := range work {
				file := snapshot.Files.GetFile(path)
				if file == nil || !file.Data.IsEditable() {
					continue
				}
				if file.Hash() != "" && file.RangesHash() == "" {
					// Unchanged since its blob was written and carrying no
					// marks, so the caller can read the blob itself.
					continue
				}
				if err := file.LoadEager(ctx, store); err != nil {
					mutex.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mutex.Unlock()
				}
			}
		}()
	}
	for _, path := range paths {
		work <- path
	}
	close(work)
	waiting.Wait()
	if firstErr != nil {
		return nil, firstErr
	}

	result := &ProjectSnapshot{ProjectID: projectID,
		Files: make(map[string]ProjectFileData, len(paths))}
	for _, path := range paths {
		file := snapshot.Files.GetFile(path)
		if file == nil {
			continue
		}
		if data := file.StringData(); data != nil {
			content := data.GetContent(true)
			result.Files[path] = ProjectFileData{
				Data: ProjectFileContent{Content: &content},
			}
			continue
		}
		result.Files[path] = ProjectFileData{
			Data: ProjectFileContent{Hash: file.Hash()},
		}
	}
	return result, nil
}

// ChangesInChunkSince is the changes made after a version, and the version the
// chunk they are in starts at.
func (s *Snapshots) ChangesInChunkSince(ctx context.Context, projectID,
	historyID string, sinceVersion int) (int, []*histmodel.Change, error) {

	latest, err := s.store.GetMostRecentChunk(ctx, historyID)
	if err != nil {
		return 0, nil, err
	}
	if sinceVersion > latest.Chunk.EndVersion() {
		return 0, nil, fmt.Errorf("%w: requested version past the end of the history",
			ErrBadRequest)
	}

	latestStartVersion := latest.Chunk.StartVersion
	chunk := latest
	if sinceVersion < latestStartVersion {
		chunk, err = s.store.GetChunkAtVersion(ctx, historyID, sinceVersion)
		if err != nil {
			return 0, nil, err
		}
	}

	changes := chunk.Chunk.History.Changes
	offset := sinceVersion - chunk.Chunk.StartVersion
	if offset < 0 {
		offset = 0
	}
	if offset > len(changes) {
		offset = len(changes)
	}
	return latestStartVersion, changes[offset:], nil
}
