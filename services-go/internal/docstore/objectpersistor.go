package docstore

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// newObjectPersistor builds the archiving backend named by the configuration.
//
// Only the operations the archiver performs are covered. The Node service
// reaches these through @overleaf/object-persistor, which additionally carries
// cross-backend migration and per-project encryption; archiving never uses
// either, so neither is ported.
func newObjectPersistor(ctx context.Context, cfg ArchiveConfig) (Persistor, error) {
	switch cfg.Backend {
	case "gcs":
		return newGCSPersistor(ctx)
	default:
		return nil, fmt.Errorf(
			"docstore: archiving backend %q is not implemented in the Go port", cfg.Backend)
	}
}

// gcsPersistor stores archived docs in Google Cloud Storage.
type gcsPersistor struct {
	client *storage.Client
}

func newGCSPersistor(ctx context.Context) (Persistor, error) {
	var opts []option.ClientOption
	// The tests run against fake-gcs-server. GCS_API_ENDPOINT is the host
	// root, which is what this client wants; STORAGE_EMULATOR_HOST carries the
	// "/storage/v1" suffix the Node client expects, and passing that through
	// leaves uploads working while downloads resolve to a path that does not
	// exist. Prefer the clean root, and fall back to trimming the suffix.
	// STORAGE_EMULATOR_HOST takes precedence over WithEndpoint inside the
	// client, so it has to be normalised rather than overridden.
	if raw := os.Getenv("STORAGE_EMULATOR_HOST"); raw != "" {
		root := strings.TrimSuffix(strings.TrimSuffix(raw, "/"), "/storage/v1")
		if root != raw {
			if err := os.Setenv("STORAGE_EMULATOR_HOST", root); err != nil {
				return nil, err
			}
		}
		opts = append(opts, option.WithoutAuthentication())
	} else if endpoint := os.Getenv("GCS_API_ENDPOINT"); endpoint != "" {
		opts = append(opts, option.WithEndpoint(endpoint), option.WithoutAuthentication())
	}
	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &gcsPersistor{client: client}, nil
}

func (p *gcsPersistor) SendStream(ctx context.Context, bucket, key string, body []byte, sourceMd5 string) error {
	w := p.client.Bucket(bucket).Object(key).NewWriter(ctx)
	if sourceMd5 != "" {
		sum, err := hex.DecodeString(sourceMd5)
		if err != nil {
			return fmt.Errorf("docstore: invalid source md5 %q: %w", sourceMd5, err)
		}
		// Setting MD5 makes the server reject a corrupted upload, which is
		// what the sourceMd5 option buys in the Node implementation.
		w.MD5 = sum
	}
	if _, err := w.Write(body); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

func (p *gcsPersistor) GetObjectStream(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	r, err := p.client.Bucket(bucket).Object(key).NewReader(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, bucket, key)
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (p *gcsPersistor) GetObjectMd5Hash(ctx context.Context, bucket, key string) (string, error) {
	attrs, err := p.client.Bucket(bucket).Object(key).Attrs(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return "", fmt.Errorf("%w: %s/%s", ErrNotFound, bucket, key)
	}
	if err != nil {
		return "", err
	}
	if len(attrs.MD5) == 0 {
		// GCS omits the hash for composite objects; the Node persistor falls
		// back to the base64 field, which is empty here too.
		return "", nil
	}
	return hex.EncodeToString(attrs.MD5), nil
}

func (p *gcsPersistor) DeleteDirectory(ctx context.Context, bucket, prefix string) error {
	// Object keys are "<projectId>/<docId>", so the directory prefix needs the
	// separator to avoid matching a project whose id merely starts the same.
	it := p.client.Bucket(bucket).Objects(ctx, &storage.Query{Prefix: prefix + "/"})
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return nil
		}
		if err != nil {
			// Nothing was ever archived into this bucket, so there is nothing
			// to remove; destroying the project still has to succeed.
			if errors.Is(err, storage.ErrBucketNotExist) {
				return nil
			}
			return err
		}
		if err := p.client.Bucket(bucket).Object(attrs.Name).Delete(ctx); err != nil &&
			!errors.Is(err, storage.ErrObjectNotExist) {
			return err
		}
	}
}
