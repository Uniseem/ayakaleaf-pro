package historystore

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/persistor"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Blobs.
//
// A blob is named by git's hash of its contents, so the same bytes written
// twice are one blob and a write is never an update. The index says which
// blobs a project has and how big they are; the bytes are in the object store
// under a key built from the hash.
//
// The index is one document per project with the blobs in buckets keyed by the
// first three hex digits of the hash. A bucket that fills up spills into a
// second collection sharded by the first digit -- a project with a hundred
// thousand files would otherwise be one Mongo document too large to update.

// maxInBucket is how many blobs share a bucket before they spill.
const maxInBucket = 8

// Blob is what the index knows about one.
type Blob struct {
	Hash       string `json:"hash" bson:"-"`
	ByteLength int64  `json:"byteLength" bson:"b"`
	// StringLength is set for a blob the history holds as text, and is what
	// tells an editable file from a binary one.
	StringLength *int64 `json:"stringLength,omitempty" bson:"s,omitempty"`
}

// blobRecord is the stored shape. The hash is binary rather than text because
// there are a great many of them.
type blobRecord struct {
	Hash         bson.Binary `bson:"h"`
	ByteLength   int64       `bson:"b"`
	StringLength *int64      `bson:"s,omitempty"`
}

// Hash is git's name for a run of bytes, which is what the history stores
// files under and what a git push already knows.
func Hash(content []byte) string {
	sum := sha1.New()
	fmt.Fprintf(sum, "blob %d", len(content))
	sum.Write([]byte{0})
	sum.Write(content)
	return hex.EncodeToString(sum.Sum(nil))
}

// PutBlob writes a blob and records it.
//
// The bytes go first. A record with no object behind it is a project that
// cannot be read; an object with no record is a few kilobytes nobody will look
// at, and the next write of the same content puts the record right.
func (s *Store) PutBlob(ctx context.Context, historyID, hash string, content []byte, text bool) (*Blob, error) {
	if !validHash(hash) {
		return nil, fmt.Errorf("%w: %q is not a blob hash", ErrBadRequest, hash)
	}
	if computed := Hash(content); computed != hash {
		return nil, fmt.Errorf("%w: the content does not have that hash", ErrBadRequest)
	}

	if existing, err := s.FindBlob(ctx, historyID, hash); err == nil {
		// Already here. The same bytes are the same blob, so there is nothing
		// to write and nothing to change.
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	key := blobKey(historyID, hash)
	err := s.objects.SendStream(ctx, s.buckets.ProjectBlobs, key,
		newReader(content), persistor.GetOptions{UseSubdirectories: true})
	if err != nil {
		return nil, err
	}

	blob := &Blob{Hash: hash, ByteLength: int64(len(content))}
	if text {
		length := int64(len([]rune(string(content))))
		blob.StringLength = &length
	}
	if err := s.recordBlob(ctx, historyID, blob); err != nil {
		return nil, err
	}
	return blob, nil
}

// FindBlob is what the index knows about a blob, if it has it.
func (s *Store) FindBlob(ctx context.Context, historyID, hash string) (*Blob, error) {
	if !validHash(hash) {
		return nil, fmt.Errorf("%w: %q is not a blob hash", ErrBadRequest, hash)
	}
	if blob, err := s.findGlobalBlob(ctx, hash); err == nil {
		return blob, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	id, err := bson.ObjectIDFromHex(historyID)
	if err != nil {
		return nil, fmt.Errorf("%w: %q is not a project", ErrBadRequest, historyID)
	}

	bucket := "blobs." + hash[:3]
	var found struct {
		Bucket []blobRecord `bson:"bucket"`
	}
	err = s.blobs.FindOne(ctx, bson.M{"_id": id},
		projected().SetProjection(bson.M{"_id": 0, "bucket": "$" + bucket})).Decode(&found)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if blob := blobIn(found.Bucket, hash); blob != nil {
		return blob, nil
	}
	if len(found.Bucket) < maxInBucket {
		// The bucket has room, so if it were here it would be here.
		return nil, ErrNotFound
	}
	return s.findShardedBlob(ctx, historyID, hash)
}

// ReadBlob opens a blob's bytes.
func (s *Store) ReadBlob(ctx context.Context, historyID, hash string) (io.ReadCloser, *Blob, error) {
	blob, err := s.FindBlob(ctx, historyID, hash)
	if err != nil {
		return nil, nil, err
	}

	bucket, key := s.buckets.ProjectBlobs, blobKey(historyID, hash)
	if global, err := s.findGlobalBlob(ctx, hash); err == nil && global != nil {
		bucket, key = s.buckets.GlobalBlobs, globalBlobKey(hash)
	}

	body, err := s.objects.GetObject(ctx, bucket, key,
		persistor.GetOptions{UseSubdirectories: true})
	if notFound(err) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	return body, blob, nil
}

// CopyBlobs points one project's index at another's blobs.
//
// Used when a project is copied: the bytes are already stored and identical,
// so the copy is an index entry rather than a second copy of every file.
func (s *Store) CopyBlobs(ctx context.Context, fromHistoryID, toHistoryID string) error {
	from, err := bson.ObjectIDFromHex(fromHistoryID)
	if err != nil {
		return fmt.Errorf("%w: %q is not a project", ErrBadRequest, fromHistoryID)
	}
	to, err := bson.ObjectIDFromHex(toHistoryID)
	if err != nil {
		return fmt.Errorf("%w: %q is not a project", ErrBadRequest, toHistoryID)
	}

	var source struct {
		Blobs map[string][]blobRecord `bson:"blobs"`
	}
	err = s.blobs.FindOne(ctx, bson.M{"_id": from}).Decode(&source)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	_, err = s.blobs.UpdateOne(ctx, bson.M{"_id": to},
		bson.M{"$set": bson.M{"blobs": source.Blobs}}, upsert())
	return err
}

// InitialiseBlobs gives a project somewhere to record its blobs.
func (s *Store) InitialiseBlobs(ctx context.Context, historyID string) error {
	id, err := bson.ObjectIDFromHex(historyID)
	if err != nil {
		return fmt.Errorf("%w: %q is not a project", ErrBadRequest, historyID)
	}
	_, err = s.blobs.UpdateOne(ctx, bson.M{"_id": id},
		bson.M{"$setOnInsert": bson.M{"blobs": bson.M{}}}, upsert())
	return err
}

// recordBlob puts a blob in the index, spilling into the sharded collection
// when its bucket is full.
func (s *Store) recordBlob(ctx context.Context, historyID string, blob *Blob) error {
	id, err := bson.ObjectIDFromHex(historyID)
	if err != nil {
		return fmt.Errorf("%w: %q is not a project", ErrBadRequest, historyID)
	}
	raw, err := hex.DecodeString(blob.Hash)
	if err != nil {
		return err
	}
	record := blobRecord{
		Hash:         bson.Binary{Data: raw},
		ByteLength:   blob.ByteLength,
		StringLength: blob.StringLength,
	}

	bucket := "blobs." + blob.Hash[:3]
	result, err := s.blobs.UpdateOne(ctx,
		bson.M{
			"_id": id,
			// Only while there is room. A bucket that has filled up is what
			// sends the rest to the sharded collection.
			"$expr": bson.M{"$lt": bson.A{
				bson.M{"$size": bson.M{"$ifNull": bson.A{"$" + bucket, bson.A{}}}},
				maxInBucket,
			}},
		},
		// No upsert: mongo refuses $expr in the filter of one, and the
		// project's document was made when its history was started.
		bson.M{"$addToSet": bson.M{bucket: record}})
	if err == nil && result.MatchedCount > 0 {
		return nil
	}
	if err != nil && !mongo.IsDuplicateKeyError(err) {
		return err
	}
	return s.recordShardedBlob(ctx, historyID, blob.Hash, record)
}

func (s *Store) recordShardedBlob(ctx context.Context, historyID, hash string, record blobRecord) error {
	id, err := shardedID(historyID, hash[:1])
	if err != nil {
		return err
	}
	bucket := "blobs." + hash[1:4]
	_, err = s.shardedBlobs.UpdateOne(ctx, bson.M{"_id": id},
		bson.M{"$addToSet": bson.M{bucket: record}}, upsert())
	return err
}

func (s *Store) findShardedBlob(ctx context.Context, historyID, hash string) (*Blob, error) {
	id, err := shardedID(historyID, hash[:1])
	if err != nil {
		return nil, err
	}
	bucket := "blobs." + hash[1:4]
	var found struct {
		Blobs []blobRecord `bson:"blobs"`
	}
	err = s.shardedBlobs.FindOne(ctx, bson.M{"_id": id},
		projected().SetProjection(bson.M{"_id": 0, "blobs": "$" + bucket})).Decode(&found)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if blob := blobIn(found.Blobs, hash); blob != nil {
		return blob, nil
	}
	return nil, ErrNotFound
}

// findGlobalBlob looks in the blobs every project shares.
func (s *Store) findGlobalBlob(ctx context.Context, hash string) (*Blob, error) {
	var stored struct {
		ID         string `bson:"_id"`
		ByteLength int64  `bson:"byteLength"`
		StringLen  *int64 `bson:"stringLength,omitempty"`
	}
	err := s.globalBlobs.FindOne(ctx, bson.M{"_id": hash}).Decode(&stored)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &Blob{Hash: hash, ByteLength: stored.ByteLength, StringLength: stored.StringLen}, nil
}

func blobIn(records []blobRecord, hash string) *Blob {
	for _, record := range records {
		if hex.EncodeToString(record.Hash.Data) == hash {
			return &Blob{
				Hash: hash, ByteLength: record.ByteLength,
				StringLength: record.StringLength,
			}
		}
	}
	return nil
}

// shardedID is the key of one shard of one project's spilled blobs.
func shardedID(historyID, shard string) (bson.Binary, error) {
	raw, err := hex.DecodeString(historyID + "0" + shard)
	if err != nil {
		return bson.Binary{}, fmt.Errorf("%w: %q is not a project", ErrBadRequest, historyID)
	}
	return bson.Binary{Data: raw}, nil
}

func blobKey(historyID, hash string) string {
	return objectKey(historyID) + "/" + hash[:2] + "/" + hash[2:]
}

func globalBlobKey(hash string) string {
	return hash[:2] + "/" + hash[2:4] + "/" + hash[4:]
}
