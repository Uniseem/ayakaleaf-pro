// Command docstore is a drop-in replacement for services/docstore, serving the
// same HTTP API against the same MongoDB collection.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/config"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/docstore"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/httpx"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/logx"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/mongox"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/obsv"
)

func main() {
	log := logx.New("docstore")

	client, db, err := mongox.Connect(context.Background())
	if err != nil {
		log.Log(context.Background(), logx.LevelFatal, "Cannot connect to mongo. Exiting.", logx.Err(err))
		os.Exit(1)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = client.Disconnect(ctx)
	}()

	archiveCfg := docstore.ArchiveConfig{
		Backend:                     os.Getenv("BACKEND"),
		Bucket:                      config.Env("BUCKET_NAME", config.Env("AWS_BUCKET", "bucket")),
		KeepSoftDeletedDocsArchived: os.Getenv("KEEP_SOFT_DELETED_DOCS_ARCHIVED") == "true",
		ArchiveOnSoftDelete:         os.Getenv("ARCHIVE_ON_SOFT_DELETE") == "true",
		UnArchiveBatchSize:          int64(config.EnvInt("UN_ARCHIVE_BATCH_SIZE", 50)),
		ParallelArchiveJobs:         config.EnvInt("PARALLEL_ARCHIVE_JOBS", 5),
		MaxDocLength:                config.EnvInt("MAX_DOC_LENGTH", 2*1024*1024),
	}

	store := docstore.NewStore(db,
		int64(config.EnvInt("MAX_DELETED_DOCS", 2000)),
		time.Duration(config.EnvInt("ARCHIVING_LOCK_DURATION_MS", 60000))*time.Millisecond,
	)

	persistor, err := docstore.NewPersistor(context.Background(), archiveCfg)
	if err != nil {
		log.Log(context.Background(), logx.LevelFatal, "Cannot set up the archiving backend. Exiting.", logx.Err(err))
		os.Exit(1)
	}
	archiver := docstore.NewArchiver(store, persistor, archiveCfg)
	manager := docstore.NewManager(store, archiver, log, archiveCfg)

	port := config.EnvInt("DOCSTORE_PORT", 3016)
	metrics := obsv.New(config.Env("METRICS_APP_NAME", "docstore"))
	server := docstore.NewServer(manager, store, log, metrics, docstore.Config{
		MaxDocLength:       archiveCfg.MaxDocLength,
		MaxJSONRequestSize: int64(config.EnvInt("MAX_JSON_REQUEST_SIZE", 12*1024*1024)),
		HealthCheckProject: os.Getenv("HEALTH_CHECK_PROJECT_ID"),
		SelfURL:            fmt.Sprintf("http://127.0.0.1:%d", port),
	}, &http.Client{Timeout: 30 * time.Second})

	log.Info("docstore archiving policy",
		slog.String("backend", archiveCfg.Backend),
		slog.Bool("enabled", archiveCfg.Enabled()))

	addr := fmt.Sprintf("%s:%d", config.ListenAddress(), port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 60 * time.Second,
	}
	if err := httpx.ListenAndServe(srv, log); err != nil {
		log.Error("docstore HTTP server stopped", logx.Err(err), slog.String("addr", addr))
		os.Exit(1)
	}
}
