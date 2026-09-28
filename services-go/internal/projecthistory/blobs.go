package projecthistory

import (
	"context"
	"sync"
	"time"
)

// Before a change can say a file was added it has to name the blob the file's
// contents were stored in, so the contents go to the history store first and
// the change is written afterwards.
//
// A project being resynced sends every file it has, which can be a thousand of
// them, so they are stored a few at a time rather than all at once, and one
// that fails is retried rather than failing the whole batch. The lock on the
// project is extended around each one, because a large file takes long enough
// that the lock would otherwise expire while it is being sent.

// maxConcurrentBlobRequests is how many files are stored at once.
const maxConcurrentBlobRequests = 4

// blobRetryAttempts is how many times storing one file is tried.
const blobRetryAttempts = 3

// blobRetryInterval is how long to wait between attempts.
const blobRetryInterval = 100 * time.Millisecond

// CreateBlobsForUpdates stores the contents of every update that adds a file,
// and returns the updates with the hashes attached.
//
// Every update is returned whether or not its blob was stored, and the first
// failure is returned with them: the caller needs the whole list to say how far
// it got, and a partial list would look like a shorter queue.
func CreateBlobsForUpdates(ctx context.Context, store *HistoryStore, projectID,
	historyID string, updates []*Update,
	extendLock func(context.Context) error) ([]UpdateWithBlob, error) {

	withBlobs := make([]UpdateWithBlob, len(updates))

	var mutex sync.Mutex
	var firstErr error
	recordErr := func(err error) {
		mutex.Lock()
		defer mutex.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}

	work := make(chan int)
	var waiting sync.WaitGroup
	for worker := 0; worker < maxConcurrentBlobRequests; worker++ {
		waiting.Add(1)
		go func() {
			defer waiting.Done()
			for index := range work {
				update := updates[index]
				withBlobs[index] = UpdateWithBlob{Update: update}
				if !isAdd(update) {
					continue
				}

				hashes, err := createBlobWithRetries(ctx, store, projectID,
					historyID, update, extendLock)
				if err != nil {
					recordErr(err)
					continue
				}
				withBlobs[index].FileHash = hashes.File
				withBlobs[index].RangesHash = hashes.Ranges

				// Extended again once the file is stored, so that the time it
				// took does not come out of what the caller has left.
				if err := extendLock(ctx); err != nil {
					recordErr(err)
				}
			}
		}()
	}

	for index := range updates {
		work <- index
	}
	close(work)
	waiting.Wait()

	return withBlobs, firstErr
}

// createBlobWithRetries stores one update's contents, trying again if it
// fails. A resync sends every file a project has, and one of them failing
// should not fail the rest.
func createBlobWithRetries(ctx context.Context, store *HistoryStore, projectID,
	historyID string, update *Update,
	extendLock func(context.Context) error) (*BlobHashes, error) {

	var lastErr error
	for attempt := 0; attempt < blobRetryAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(blobRetryInterval):
			}
		}

		// The lock is extended before each attempt, not once for all of them:
		// a large file can take longer than the lock is held for.
		if err := extendLock(ctx); err != nil {
			lastErr = err
			continue
		}

		hashes, err := store.CreateBlobForUpdate(ctx, projectID, historyID, update)
		if err == nil {
			return hashes, nil
		}
		lastErr = err
	}
	return nil, lastErr
}
