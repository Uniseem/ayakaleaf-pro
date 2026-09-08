// Command history serves the history store: what a project used to look like.
//
// It holds the bytes of every version of every file, and the runs of changes
// between them. Nothing outside the deployment reaches it -- it has no idea who
// anybody is, and every project's contents are behind one shared password on a
// loopback port.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/config"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/historystore"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/httpx"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/logx"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/mongox"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/persistor"
)

func main() {
	log := logx.New("history")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, db, err := mongox.Connect(ctx)
	if err != nil {
		log.Log(ctx, logx.LevelFatal, "Cannot connect to mongo. Exiting.", logx.Err(err))
		os.Exit(1)
	}
	defer func() {
		shutdown, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		_ = client.Disconnect(shutdown)
	}()

	objects, err := buildPersistor(ctx)
	if err != nil {
		log.Log(ctx, logx.LevelFatal, "Cannot reach the object store. Exiting.", logx.Err(err))
		os.Exit(1)
	}

	store := historystore.New(db, objects, historystore.Buckets{
		ProjectBlobs: config.Env("OVERLEAF_HISTORY_PROJECT_BLOBS_BUCKET",
			"/var/lib/overleaf/data/history/overleaf-project-blobs"),
		GlobalBlobs: config.Env("OVERLEAF_HISTORY_BLOBS_BUCKET",
			"/var/lib/overleaf/data/history/overleaf-global-blobs"),
		Chunks: config.Env("OVERLEAF_HISTORY_CHUNKS_BUCKET",
			"/var/lib/overleaf/data/history/overleaf-chunks"),
	})
	// The uniqueness rule on a project's chunk versions is what stops two
	// writers extending the history from the same place, so it is ensured at
	// startup rather than assumed.
	if err := store.EnsureIndexes(ctx); err != nil {
		log.Warn("could not ensure the history indexes", logx.Err(err))
	}

	server := historystore.NewServer(store, log,
		config.Env("V1_HISTORY_USER", "staging"),
		config.Env("STAGING_PASSWORD", ""),
		int64(config.EnvInt("MAX_BLOB_SIZE", 512))*1024*1024,
	)

	addr := fmt.Sprintf("%s:%d", config.ListenAddress(), config.EnvInt("HISTORY_V1_PORT", 3100))
	srv := &http.Server{
		Addr:              addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 30 * time.Second,
		// Long: a request here can be a whole project's worth of changes, or a
		// file of several hundred megabytes.
		WriteTimeout: 10 * time.Minute,
	}
	log.Info("history listening", slog.String("addr", addr))
	if err := httpx.ListenAndServe(srv, log); err != nil {
		log.Error("history HTTP server stopped", logx.Err(err), slog.String("addr", addr))
		os.Exit(1)
	}
}

// buildPersistor selects where the contents live.
//
// The same two backends the rest of the deployment offers: a filesystem for a
// small installation, S3 for one that has outgrown it.
func buildPersistor(ctx context.Context) (persistor.Persistor, error) {
	switch config.Env("OVERLEAF_HISTORY_BACKEND", "fs") {
	case "fs":
		// With subdirectories: a history key is a path, and flattening it
		// would put every blob of every project in one directory.
		return persistor.NewFS(true), nil
	case "s3":
		return persistor.NewS3(ctx, persistor.S3Config{
			Key:             config.Env("OVERLEAF_HISTORY_S3_ACCESS_KEY_ID", os.Getenv("AWS_ACCESS_KEY_ID")),
			Secret:          config.Env("OVERLEAF_HISTORY_S3_SECRET_ACCESS_KEY", os.Getenv("AWS_SECRET_ACCESS_KEY")),
			Endpoint:        config.Env("OVERLEAF_HISTORY_S3_ENDPOINT", os.Getenv("AWS_S3_ENDPOINT")),
			Region:          config.Env("OVERLEAF_HISTORY_S3_REGION", os.Getenv("AWS_DEFAULT_REGION")),
			PathStyle:       config.Env("OVERLEAF_HISTORY_S3_PATH_STYLE", "") == "true",
			PartSize:        int64(config.EnvInt("AWS_S3_PARTSIZE", 100*1024*1024)),
			SignedURLExpiry: time.Duration(config.EnvInt("LINK_EXPIRY_TIMEOUT", 60000)) * time.Millisecond,
		})
	default:
		return nil, fmt.Errorf("history: backend %q is not one this offers (fs or s3)",
			config.Env("OVERLEAF_HISTORY_BACKEND", "fs"))
	}
}
