package filestore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/persistor"
)

// ErrInvalidParameters matches InvalidParametersError.
var ErrInvalidParameters = errors.New("key does not match validation regex")

// writeKeyPattern is the guard FileHandler applies before writing or deleting:
// a key must name a template file or a project object, so an arbitrary path
// cannot be created or removed through the API.
var writeKeyPattern = regexp.MustCompile(`(?i)^[0-9a-f]{24}/([0-9a-f]{24}|v/[0-9]+/[a-z0-9]+)`)

// GetOptions are the query parameters that select a rendering.
type GetOptions struct {
	Format   string
	Style    string
	Start    int64
	End      int64
	HasRange bool
}

// Handler is filestore's business logic, the counterpart of FileHandler.js.
type Handler struct {
	persistor  persistor.Persistor
	converter  *Converter
	stores     Stores
	uploadDir  string
	allowRedir bool
}

// NewHandler builds a Handler.
func NewHandler(p persistor.Persistor, c *Converter, stores Stores, uploadDir string, allowRedirects bool) *Handler {
	return &Handler{persistor: p, converter: c, stores: stores, uploadDir: uploadDir, allowRedir: allowRedirects}
}

// InsertFile stores an object, refusing keys outside the expected shape.
func (h *Handler) InsertFile(ctx context.Context, t Target, r io.Reader) error {
	if !writeKeyPattern.MatchString(ConvertedFolderKey(t.Key)) {
		return fmt.Errorf("%w: %s", ErrInvalidParameters, t.Key)
	}
	return h.persistor.SendStream(ctx, t.Bucket, t.Key, r, opts(t, GetOptions{}))
}

// DeleteFile removes an object and, for template files, the cache of
// renderings derived from it.
func (h *Handler) DeleteFile(ctx context.Context, t Target) error {
	converted := ConvertedFolderKey(t.Key)
	if !writeKeyPattern.MatchString(converted) {
		return fmt.Errorf("%w: %s", ErrInvalidParameters, t.Key)
	}
	if err := h.persistor.DeleteObject(ctx, t.Bucket, t.Key, opts(t, GetOptions{})); err != nil {
		return err
	}
	if h.converter.Enabled() && t.Bucket == h.stores.TemplateFiles {
		return h.persistor.DeleteDirectory(ctx, t.Bucket, converted, opts(t, GetOptions{}))
	}
	return nil
}

// GetFileSize reports an object's size, for HEAD requests.
func (h *Handler) GetFileSize(ctx context.Context, t Target) (int64, error) {
	return h.persistor.GetObjectSize(ctx, t.Bucket, t.Key, opts(t, GetOptions{}))
}

// GetFile opens an object, rendering and caching a converted copy when a
// format or style was asked for.
func (h *Handler) GetFile(ctx context.Context, t Target, o GetOptions) (io.ReadCloser, error) {
	if o.Format == "" && o.Style == "" {
		return h.persistor.GetObject(ctx, t.Bucket, t.Key, opts(t, o))
	}
	return h.getConverted(ctx, t, o)
}

// RedirectURL returns a URL the client can fetch directly, or "" to have the
// file proxied.
//
// Only plain reads of a known store can be redirected: a range, a conversion
// or an unrecognised bucket all have to go through this service.
func (h *Handler) RedirectURL(ctx context.Context, t Target, o GetOptions) (string, error) {
	if !h.allowRedir || o.HasRange || o.Format != "" || o.Style != "" {
		return "", nil
	}
	if !h.knownStore(t.Bucket) {
		return "", nil
	}
	return h.persistor.RedirectURL(ctx, t.Bucket, t.Key)
}

func (h *Handler) knownStore(bucket string) bool {
	for _, s := range []string{h.stores.TemplateFiles, h.stores.ProjectBlobs, h.stores.GlobalBlobs} {
		if s != "" && s == bucket {
			return true
		}
	}
	return false
}

func (h *Handler) getConverted(ctx context.Context, t Target, o GetOptions) (io.ReadCloser, error) {
	cacheKey := CachedKey(t.Key, o.Format, o.Style)
	exists, err := h.persistor.ObjectExists(ctx, t.Bucket, cacheKey, opts(t, o))
	if err != nil {
		return nil, err
	}
	if exists {
		return h.persistor.GetObject(ctx, t.Bucket, cacheKey, opts(t, o))
	}
	return h.convertAndCache(ctx, t, cacheKey, o)
}

func (h *Handler) convertAndCache(ctx context.Context, t Target, cacheKey string, o GetOptions) (io.ReadCloser, error) {
	sourcePath, err := h.writeToDisk(ctx, t, o)
	if err != nil {
		return nil, fmt.Errorf("%w: unable to write file to disk: %v", ErrConversion, err)
	}
	defer func() { _ = os.Remove(sourcePath) }()

	var convertedPath string
	switch {
	case o.Format != "":
		convertedPath, err = h.converter.Convert(ctx, sourcePath, o.Format)
	case o.Style == "thumbnail":
		convertedPath, err = h.converter.Thumbnail(ctx, sourcePath)
	case o.Style == "preview":
		convertedPath, err = h.converter.Preview(ctx, sourcePath)
	default:
		return nil, fmt.Errorf("%w: invalid file conversion options", ErrConversion)
	}
	if err != nil {
		return nil, err
	}

	if err := h.converter.CompressPng(ctx, convertedPath); err != nil {
		_ = os.Remove(convertedPath)
		return nil, fmt.Errorf("%w: %v", ErrConversion, err)
	}
	if err := h.persistor.SendFile(ctx, t.Bucket, cacheKey, convertedPath, opts(t, o)); err != nil {
		_ = os.Remove(convertedPath)
		return nil, fmt.Errorf("%w: %v", ErrConversion, err)
	}

	// Serve the local copy rather than reading back what was just uploaded:
	// S3 gives only eventual consistency for a key that was checked with HEAD
	// before it existed, which used to produce spurious 403s.
	f, err := os.Open(convertedPath)
	if err != nil {
		_ = os.Remove(convertedPath)
		return nil, err
	}
	return &removeOnClose{File: f, path: convertedPath}, nil
}

// writeToDisk copies an object to a local file for the converter to read.
func (h *Handler) writeToDisk(ctx context.Context, t Target, o GetOptions) (string, error) {
	if err := os.MkdirAll(h.uploadDir, 0o755); err != nil {
		return "", err
	}
	src, err := h.persistor.GetObject(ctx, t.Bucket, t.Key, opts(t, o))
	if err != nil {
		return "", err
	}
	defer func() { _ = src.Close() }()

	f, err := os.CreateTemp(h.uploadDir, "convert-*")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if _, err := io.Copy(f, src); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// removeOnClose deletes the backing file once the response has been sent.
type removeOnClose struct {
	*os.File
	path string
}

func (r *removeOnClose) Close() error {
	err := r.File.Close()
	_ = os.Remove(r.path)
	return err
}

func opts(t Target, o GetOptions) persistor.GetOptions {
	return persistor.GetOptions{
		Start:             o.Start,
		End:               o.End,
		HasRange:          o.HasRange,
		UseSubdirectories: t.UseSubdirectories,
	}
}
