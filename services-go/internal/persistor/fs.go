package persistor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FS stores objects on the local filesystem, which is what a server-ce
// deployment uses unless OVERLEAF_FILESTORE_BACKEND is set to s3.
//
// The "bucket" is a directory path.
type FS struct {
	// UseSubdirectories makes every key nested rather than flattened. The
	// per-request option of the same name does the same for one call.
	UseSubdirectories bool
}

// NewFS builds a filesystem persistor.
func NewFS(useSubdirectories bool) *FS {
	return &FS{UseSubdirectories: useSubdirectories}
}

// path maps a key onto a file, mirroring FSPersistor._getFsPath: a trailing
// slash is dropped, and unless subdirectories are in use every "/" becomes an
// "_" so the whole store stays flat.
func (p *FS) path(bucket, key string, opts GetOptions) (string, error) {
	key = strings.TrimSuffix(key, "/")
	if !p.UseSubdirectories && !opts.UseSubdirectories {
		key = strings.ReplaceAll(key, "/", "_")
	}
	full := filepath.Join(bucket, key)

	// filepath.Join cleans the path, so a key containing ".." would otherwise
	// escape the bucket directory.
	rel, err := filepath.Rel(bucket, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("persistor: key %q escapes its bucket", key)
	}
	return full, nil
}

func (p *FS) GetObject(ctx context.Context, bucket, key string, opts GetOptions) (io.ReadCloser, error) {
	fsPath, err := p.path(bucket, key, opts)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(fsPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, fsPath)
	}
	if err != nil {
		return nil, err
	}
	if !opts.HasRange {
		return f, nil
	}
	if _, err := f.Seek(opts.Start, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, err
	}
	// The range is inclusive of End, matching the Range header.
	length := opts.End - opts.Start + 1
	if length < 0 {
		length = 0
	}
	return readCloser{Reader: io.LimitReader(f, length), Closer: f}, nil
}

type readCloser struct {
	io.Reader
	io.Closer
}

func (p *FS) GetObjectSize(ctx context.Context, bucket, key string, opts GetOptions) (int64, error) {
	fsPath, err := p.path(bucket, key, opts)
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(fsPath)
	if errors.Is(err, os.ErrNotExist) {
		return 0, fmt.Errorf("%w: %s", ErrNotFound, fsPath)
	}
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (p *FS) SendStream(ctx context.Context, bucket, key string, r io.Reader, opts GetOptions) error {
	fsPath, err := p.path(bucket, key, opts)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(fsPath), 0o755); err != nil {
		return err
	}
	// Write to a temporary file first, so a reader never sees a half-written
	// object and a failed write leaves the previous one intact.
	tmp, err := os.CreateTemp(filepath.Dir(fsPath), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	if _, err := io.Copy(tmp, r); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, fsPath)
}

func (p *FS) SendFile(ctx context.Context, bucket, key, fsPath string, opts GetOptions) error {
	f, err := os.Open(fsPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return p.SendStream(ctx, bucket, key, f, opts)
}

func (p *FS) DeleteObject(ctx context.Context, bucket, key string, opts GetOptions) error {
	fsPath, err := p.path(bucket, key, opts)
	if err != nil {
		return err
	}
	if err := os.Remove(fsPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (p *FS) DeleteDirectory(ctx context.Context, bucket, prefix string, opts GetOptions) error {
	fsPath, err := p.path(bucket, prefix, opts)
	if err != nil {
		return err
	}
	if p.UseSubdirectories || opts.UseSubdirectories {
		if err := os.RemoveAll(fsPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	// Flattened keys have no directory to remove: the prefix is a filename
	// stem, and its members are siblings named "<stem>_...".
	matches, err := filepath.Glob(fsPath + "_*")
	if err != nil {
		return err
	}
	for _, m := range matches {
		if err := os.RemoveAll(m); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (p *FS) ObjectExists(ctx context.Context, bucket, key string, opts GetOptions) (bool, error) {
	fsPath, err := p.path(bucket, key, opts)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(fsPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// RedirectURL reports that this backend cannot hand out a direct URL, which
// makes the caller proxy the file instead.
func (p *FS) RedirectURL(ctx context.Context, bucket, key string) (string, error) {
	return "", nil
}
