// Package historystore keeps what a project used to look like.
//
// Two things, and they are stored differently because they behave differently.
//
// Blobs are the bytes of files, named by the hash of their contents. They
// never change: the same bytes are the same blob, so writing one twice writes
// nothing the second time, and nothing is ever updated in place.
//
// Chunks are runs of the history -- a snapshot and the changes after it.
// Reading a version means taking the chunk it falls in and moving its snapshot
// forward, which is why a history of ten thousand edits is not ten thousand
// copies of the project.
//
// The index for both is in Mongo and the content is in the object store, which
// is the split that lets the content be a filesystem on a small deployment and
// S3 on a large one without either half knowing.
package historystore

import (
	"context"
	"errors"
	"strings"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/persistor"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var (
	// ErrNotFound is a blob, chunk or project that is not there.
	ErrNotFound = errors.New("not in the history store")
	// ErrConflict is a write that assumed a version the history has moved past.
	// It is not a failure: it means somebody else wrote first, and the caller
	// should read again.
	ErrConflict = errors.New("the history has moved on")
	// ErrBadRequest is a request that could never be right.
	ErrBadRequest = errors.New("bad request")
)

// Buckets is where each kind of thing is kept.
type Buckets struct {
	// ProjectBlobs holds the files of one project each.
	ProjectBlobs string
	// GlobalBlobs holds the ones shared by every project -- an empty file, a
	// handful of common ones.
	GlobalBlobs string
	// Chunks holds the history itself.
	Chunks string
}

// Store is the history.
type Store struct {
	db      *mongo.Database
	objects persistor.Persistor
	buckets Buckets

	chunks       *mongo.Collection
	blobs        *mongo.Collection
	shardedBlobs *mongo.Collection
	globalBlobs  *mongo.Collection
}

// New builds one.
func New(db *mongo.Database, objects persistor.Persistor, buckets Buckets) *Store {
	return &Store{
		db:      db,
		objects: objects,
		buckets: buckets,

		chunks:       db.Collection("projectHistoryChunks"),
		blobs:        db.Collection("projectHistoryBlobs"),
		shardedBlobs: db.Collection("projectHistoryShardedBlobs"),
		globalBlobs:  db.Collection("projectHistoryGlobalBlobs"),
	}
}

// objectKey is where a project's things live in the object store.
//
// The id is padded to nine digits and reversed before it is cut into
// directories. The reversal is what matters: ids are allocated in sequence and
// an object store partitions by key prefix, so keys sharing a leading run
// would all land on one partition. Getting this wrong puts data at paths
// nothing else can find, so it is the same rule the service this replaces
// used, character for character.
func objectKey(id string) string {
	padded := id
	for len(padded) < 9 {
		padded = "0" + padded
	}
	reversed := reverse(padded)
	return reversed[0:3] + "/" + reversed[3:6] + "/" + reversed[6:]
}

// pad is the same padding, for a chunk id used as a filename.
func pad(id string) string {
	for len(id) < 9 {
		id = "0" + id
	}
	return id
}

func reverse(text string) string {
	letters := []byte(text)
	for i, j := 0, len(letters)-1; i < j; i, j = i+1, j-1 {
		letters[i], letters[j] = letters[j], letters[i]
	}
	return string(letters)
}

// validHash says whether a string could be a blob's name.
func validHash(hash string) bool {
	if len(hash) != 40 {
		return false
	}
	for _, letter := range hash {
		if !strings.ContainsRune("0123456789abcdef", letter) {
			return false
		}
	}
	return true
}

// notFound turns the object store's absence into this package's.
func notFound(err error) bool {
	return errors.Is(err, persistor.ErrNotFound)
}

var _ = context.Background
