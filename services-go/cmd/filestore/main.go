// Command filestore is a drop-in replacement for services/filestore, serving
// the same HTTP API against the same object storage.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/config"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/filestore"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/httpx"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/logx"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/obsv"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/persistor"
)

func main() {
	log := logx.New("filestore")

	backend := os.Getenv("BACKEND")
	if backend == "" {
		// settings.defaults.cjs picks s3 when AWS credentials are present and
		// the filesystem otherwise.
		if os.Getenv("AWS_ACCESS_KEY_ID") != "" || os.Getenv("S3_BUCKET_CREDENTIALS") != "" {
			backend = "s3"
		} else {
			backend = "fs"
		}
	}

	stores := filestore.Stores{
		TemplateFiles: os.Getenv("TEMPLATE_FILES_BUCKET_NAME"),
		ProjectBlobs:  os.Getenv("OVERLEAF_EDITOR_PROJECT_BLOBS_BUCKET"),
		GlobalBlobs:   os.Getenv("OVERLEAF_EDITOR_BLOBS_BUCKET"),
	}

	store, err := buildPersistor(backend)
	if err != nil {
		log.Log(context.Background(), logx.LevelFatal,
			"Cannot set up the storage backend. Exiting.", logx.Err(err))
		os.Exit(1)
	}

	converter := filestore.NewConverter(filestore.ConverterConfig{
		Enabled: os.Getenv("ENABLE_CONVERSIONS") == "true",
		Backend: config.Env("CONVERTER", "imagemagick"),
	})
	uploadDir := config.Env("FILESTORE_UPLOAD_FOLDER", filepath.Join(os.TempDir(), "filestore-uploads"))
	handler := filestore.NewHandler(store, converter, stores, uploadDir,
		os.Getenv("ALLOW_REDIRECTS") == "true")

	var shuttingDown atomic.Bool
	metrics := obsv.New(config.Env("METRICS_APP_NAME", "filestore"))
	server := filestore.NewServer(handler, stores, log, metrics, shuttingDown.Load)

	log.Info("filestore storage policy",
		slog.String("backend", backend),
		slog.Bool("conversions", converter.Enabled()),
		slog.Bool("redirects", os.Getenv("ALLOW_REDIRECTS") == "true"))

	addr := fmt.Sprintf("%s:%d", config.ListenAddress(), config.EnvInt("FILESTORE_PORT", 3009))
	srv := &http.Server{
		Addr:              addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 60 * time.Second,
	}
	srv.RegisterOnShutdown(func() { shuttingDown.Store(true) })

	if err := httpx.ListenAndServe(srv, log); err != nil {
		log.Error("filestore HTTP server stopped", logx.Err(err), slog.String("addr", addr))
		os.Exit(1)
	}
}

// buildPersistor selects the backend. server-ce offers only these two:
// settings.js switches on OVERLEAF_FILESTORE_BACKEND=s3 and otherwise uses the
// filesystem. GCS, per-project encryption and cross-backend migration exist in
// @overleaf/object-persistor but are unreachable from this distribution and
// are not ported.
func buildPersistor(backend string) (persistor.Persistor, error) {
	switch backend {
	case "fs":
		return persistor.NewFS(false), nil
	case "s3":
		return persistor.NewS3(context.Background(), persistor.S3Config{
			Key:             os.Getenv("AWS_ACCESS_KEY_ID"),
			Secret:          os.Getenv("AWS_SECRET_ACCESS_KEY"),
			Endpoint:        os.Getenv("AWS_S3_ENDPOINT"),
			Region:          os.Getenv("AWS_DEFAULT_REGION"),
			PathStyle:       os.Getenv("AWS_S3_PATH_STYLE") == "true",
			PartSize:        int64(config.EnvInt("AWS_S3_PARTSIZE", 100*1024*1024)),
			SignedURLExpiry: time.Duration(config.EnvInt("LINK_EXPIRY_TIMEOUT", 60000)) * time.Millisecond,
		})
	default:
		return nil, fmt.Errorf("filestore: backend %q is not supported by the Go port "+
			"(server-ce offers only fs and s3)", backend)
	}
}
