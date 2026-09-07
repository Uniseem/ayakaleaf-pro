package docstore

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/bsonjson"
)

// Persistor is the slice of object storage the archiver needs. It is
// deliberately narrower than @overleaf/object-persistor, which also carries
// cross-backend migration and per-project encryption that archiving never
// reaches.
type Persistor interface {
	SendStream(ctx context.Context, bucket, key string, body []byte, sourceMd5 string) error
	GetObjectStream(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	GetObjectMd5Hash(ctx context.Context, bucket, key string) (string, error)
	DeleteDirectory(ctx context.Context, bucket, prefix string) error
}

// ArchiveConfig mirrors the Settings.docstore fields the archiver reads.
type ArchiveConfig struct {
	// Backend is BACKEND: empty means archiving is switched off, which is the
	// case in every server-ce deployment -- settings.js never sets it.
	Backend                     string
	Bucket                      string
	KeepSoftDeletedDocsArchived bool
	ArchiveOnSoftDelete         bool
	UnArchiveBatchSize          int64
	ParallelArchiveJobs         int
	MaxDocLength                int
}

// Enabled mirrors DocArchiveManager._isArchivingEnabled.
func (c ArchiveConfig) Enabled() bool {
	return c.Backend != ""
}

// ArchivedDoc is the payload stored in the persistor.
type ArchivedDoc struct {
	Lines  []string
	Ranges bson.D
	Rev    int64
}

// archivedPayload is the on-disk schema, version 1.
type archivedPayload struct {
	Lines   []string        `json:"lines"`
	Ranges  json.RawMessage `json:"ranges,omitempty"`
	Rev     int64           `json:"rev"`
	SchemaV int             `json:"schema_v"`
}

// Archiver moves doc contents between Mongo and object storage.
type Archiver struct {
	store     *Store
	persistor Persistor
	cfg       ArchiveConfig
}

// NewArchiver builds an Archiver.
func NewArchiver(store *Store, persistor Persistor, cfg ArchiveConfig) *Archiver {
	return &Archiver{store: store, persistor: persistor, cfg: cfg}
}

// Enabled reports whether archiving is configured.
func (a *Archiver) Enabled() bool { return a.cfg.Enabled() }

func (a *Archiver) key(projectID, docID bson.ObjectID) string {
	return projectID.Hex() + "/" + docID.Hex()
}

// ArchiveAllDocs archives every unarchived doc of a project.
func (a *Archiver) ArchiveAllDocs(ctx context.Context, projectID bson.ObjectID) error {
	if !a.Enabled() {
		return nil
	}
	docIDs, err := a.store.GetNonArchivedProjectDocIDs(ctx, projectID)
	if err != nil {
		return err
	}
	return a.parallel(ctx, len(docIDs), func(i int) error {
		return a.ArchiveDoc(ctx, projectID, docIDs[i])
	})
}

// ArchiveDoc writes one doc to the persistor and clears it from Mongo.
func (a *Archiver) ArchiveDoc(ctx context.Context, projectID, docID bson.ObjectID) error {
	if !a.Enabled() {
		return nil
	}
	doc, err := a.store.GetDocForArchiving(ctx, projectID, docID)
	if err != nil || doc == nil {
		// Missing, already archived, or locked elsewhere: silently done.
		return err
	}
	if doc.Lines == nil {
		return errors.New("doc has no lines")
	}
	FixCommentIds(doc.Ranges)

	payload := archivedPayload{Lines: doc.Lines, Rev: doc.RevValue(), SchemaV: 1}
	if doc.Ranges != nil {
		encoded, err := json.Marshal(bsonjson.Document(doc.Ranges))
		if err != nil {
			return err
		}
		payload.Ranges = encoded
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// Guards against the memory-corruption bugs that have put null bytes in
	// document contents before now.
	if bytesContainNull(body) {
		return errors.New("null bytes detected")
	}

	sourceMd5 := ""
	if a.cfg.Backend != "s3" {
		sum := md5.Sum(body)
		sourceMd5 = hex.EncodeToString(sum[:])
	}
	if err := a.persistor.SendStream(ctx, a.cfg.Bucket, a.key(projectID, docID), body, sourceMd5); err != nil {
		return err
	}
	return a.store.MarkDocAsArchived(ctx, docID, doc.RevValue())
}

// UnArchiveAllDocs pulls every archived doc of a project back into Mongo.
func (a *Archiver) UnArchiveAllDocs(ctx context.Context, projectID bson.ObjectID) error {
	if !a.Enabled() {
		return nil
	}
	for {
		var (
			docs []Doc
			err  error
		)
		if a.cfg.KeepSoftDeletedDocsArchived {
			docs, err = a.store.GetNonDeletedArchivedProjectDocs(ctx, projectID, a.cfg.UnArchiveBatchSize)
		} else {
			docs, err = a.store.GetArchivedProjectDocs(ctx, projectID, a.cfg.UnArchiveBatchSize)
		}
		if err != nil {
			return err
		}
		if len(docs) == 0 {
			return nil
		}
		if err := a.parallel(ctx, len(docs), func(i int) error {
			return a.UnarchiveDoc(ctx, projectID, docs[i].ID)
		}); err != nil {
			return err
		}
	}
}

// GetDoc reads an archived doc without writing it back to Mongo.
func (a *Archiver) GetDoc(ctx context.Context, projectID, docID bson.ObjectID) (*ArchivedDoc, error) {
	key := a.key(projectID, docID)
	stream, err := a.persistor.GetObjectStream(ctx, a.cfg.Bucket, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = stream.Close() }()

	buf, err := io.ReadAll(stream)
	if err != nil {
		return nil, err
	}
	// S3 verifies integrity itself; other backends are checked here, the way
	// the Node implementation does.
	if a.cfg.Backend != "s3" {
		sourceMd5, err := a.persistor.GetObjectMd5Hash(ctx, a.cfg.Bucket, key)
		if err != nil {
			return nil, err
		}
		sum := md5.Sum(buf)
		if got := hex.EncodeToString(sum[:]); got != sourceMd5 {
			return nil, fmt.Errorf("%w: key %s, source %s, got %s",
				ErrMd5Mismatch, key, sourceMd5, got)
		}
	}
	return deserializeArchivedDoc(buf)
}

// UnarchiveDoc restores one archived doc into Mongo.
func (a *Archiver) UnarchiveDoc(ctx context.Context, projectID, docID bson.ObjectID) error {
	mongoDoc, err := a.store.FindDoc(ctx, projectID, docID,
		bson.D{{Key: "inS3", Value: 1}, {Key: "rev", Value: 1}}, false)
	if err != nil {
		return err
	}
	if mongoDoc == nil || !mongoDoc.IsArchived() {
		// Already unarchived.
		return nil
	}
	if !a.Enabled() {
		return errors.New("found archived doc, but archiving backend is not configured")
	}

	archived, err := a.GetDoc(ctx, projectID, docID)
	if err != nil {
		return err
	}
	if archived.Rev == 0 {
		// Older archives carried no rev; assume the one Mongo still holds.
		archived.Rev = mongoDoc.RevValue()
	}
	return a.store.RestoreArchivedDoc(ctx, projectID, docID, archived)
}

// DestroyProject removes a project's docs from Mongo and, when archiving is
// on, its directory in the persistor.
func (a *Archiver) DestroyProject(ctx context.Context, projectID bson.ObjectID) error {
	if !a.Enabled() {
		return a.store.DestroyProject(ctx, projectID)
	}
	var (
		wg               sync.WaitGroup
		mongoErr, objErr error
	)
	wg.Add(2)
	go func() { defer wg.Done(); mongoErr = a.store.DestroyProject(ctx, projectID) }()
	go func() {
		defer wg.Done()
		objErr = a.persistor.DeleteDirectory(ctx, a.cfg.Bucket, projectID.Hex())
	}()
	wg.Wait()
	return errors.Join(mongoErr, objErr)
}

// parallel runs fn over n items with the configured concurrency, matching
// pMap's behaviour of surfacing the first error.
func (a *Archiver) parallel(ctx context.Context, n int, fn func(int) error) error {
	concurrency := a.cfg.ParallelArchiveJobs
	if concurrency < 1 {
		concurrency = 1
	}
	sem := make(chan struct{}, concurrency)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			errs[i] = fn(i)
		}(i)
	}
	wg.Wait()
	return errors.Join(errs...)
}

func deserializeArchivedDoc(buf []byte) (*ArchivedDoc, error) {
	// The stored value is either a schema-1 object or, for the oldest
	// archives, a bare array of lines.
	trimmed := strings.TrimLeft(string(buf), " \t\r\n")
	if strings.HasPrefix(trimmed, "[") {
		var lines []string
		if err := json.Unmarshal(buf, &lines); err != nil {
			return nil, errors.New("I don't understand the doc format in s3")
		}
		return &ArchivedDoc{Lines: lines}, nil
	}

	var payload struct {
		Lines   []string        `json:"lines"`
		Ranges  json.RawMessage `json:"ranges"`
		Rev     *int64          `json:"rev"`
		SchemaV *int            `json:"schema_v"`
	}
	if err := json.Unmarshal(buf, &payload); err != nil {
		return nil, errors.New("I don't understand the doc format in s3")
	}
	if payload.SchemaV == nil || *payload.SchemaV != 1 || payload.Lines == nil {
		return nil, errors.New("I don't understand the doc format in s3")
	}

	doc := &ArchivedDoc{Lines: payload.Lines}
	if len(payload.Ranges) > 0 && string(payload.Ranges) != "null" {
		ranges, err := bsonjson.DecodeDocument(payload.Ranges)
		if err != nil {
			return nil, err
		}
		if converted, ok := JSONRangesToMongo(ranges).(bson.D); ok {
			doc.Ranges = converted
		}
	}
	if payload.Rev != nil {
		doc.Rev = *payload.Rev
	}
	return doc, nil
}

func bytesContainNull(b []byte) bool {
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	return false
}

// DisabledPersistor stands in when no backend is configured. Every archiving
// entry point checks Enabled() first, so its methods are never reached.
type DisabledPersistor struct{}

var errArchivingDisabled = errors.New("archiving backend is not configured")

func (DisabledPersistor) SendStream(context.Context, string, string, []byte, string) error {
	return errArchivingDisabled
}
func (DisabledPersistor) GetObjectStream(context.Context, string, string) (io.ReadCloser, error) {
	return nil, errArchivingDisabled
}
func (DisabledPersistor) GetObjectMd5Hash(context.Context, string, string) (string, error) {
	return "", errArchivingDisabled
}
func (DisabledPersistor) DeleteDirectory(context.Context, string, string) error {
	return errArchivingDisabled
}
