// Package persistor is the object-storage layer shared by the ported
// services.
//
// It covers the operations filestore and docstore actually perform, against
// the two backends a server-ce deployment can select: the local filesystem
// (the default) and S3. @overleaf/object-persistor additionally carries GCS,
// per-project client-side encryption and cross-backend migration; none of
// those are reachable from server-ce's settings.js, which offers only
// OVERLEAF_FILESTORE_BACKEND=s3 or the filesystem.
package persistor

import (
	"context"
	"errors"
	"io"
)

// ErrNotFound is returned when an object or its containing location is
// missing. The HTTP layer turns it into a 404.
var ErrNotFound = errors.New("object not found")

// GetOptions selects part of an object.
type GetOptions struct {
	// Start and End are an inclusive byte range, as parsed from a Range
	// header. End is only meaningful when HasRange is set.
	Start, End int64
	HasRange   bool
	// UseSubdirectories keeps "/" in the key as a real directory separator on
	// the filesystem backend. History blobs are stored that way; everything
	// else is flattened.
	UseSubdirectories bool
}

// Persistor is the storage surface the services need.
type Persistor interface {
	// GetObject opens an object for reading. The caller closes the reader.
	GetObject(ctx context.Context, bucket, key string, opts GetOptions) (io.ReadCloser, error)
	// GetObjectSize reports an object's size in bytes.
	GetObjectSize(ctx context.Context, bucket, key string, opts GetOptions) (int64, error)
	// SendStream writes an object from a reader.
	SendStream(ctx context.Context, bucket, key string, r io.Reader, opts GetOptions) error
	// SendFile writes an object from a local file.
	SendFile(ctx context.Context, bucket, key, fsPath string, opts GetOptions) error
	// DeleteObject removes one object. A missing object is not an error.
	DeleteObject(ctx context.Context, bucket, key string, opts GetOptions) error
	// DeleteDirectory removes everything under a key prefix.
	DeleteDirectory(ctx context.Context, bucket, prefix string, opts GetOptions) error
	// ObjectExists reports whether an object is present.
	ObjectExists(ctx context.Context, bucket, key string, opts GetOptions) (bool, error)
	// RedirectURL returns a signed URL a client can fetch directly, or an
	// empty string when the backend cannot offer one.
	RedirectURL(ctx context.Context, bucket, key string) (string, error)
}
