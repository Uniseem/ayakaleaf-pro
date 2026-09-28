// Command project-history is the Go port of services/project-history.
//
// It is what turns the stream of keystrokes into a history somebody can read.
// document-updater and web write updates onto a Redis queue; this service
// takes them off, merges the ones that belong together, works out what the
// project became, and writes that to history-v1 as a chunk of changes. It
// reads the same store back to answer what a project looked like at a version
// and what changed between two of them.
//
// Nothing waits on it. A queue that is not drained means the history falls
// behind, not that the editor stops, which is why the whole service is built
// around flushing a queue under a lock and recording what went wrong when it
// cannot.
package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/config"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/logx"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/mongox"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/obsv"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/projecthistory"
	"github.com/redis/go-redis/v9"
)

func main() {
	log := logx.New("project-history")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Mongo holds what has to survive a restart: which projects are failing,
	// the versions people have named, and how far a resync has got.
	mongoClient, database, err := mongox.Connect(ctx)
	if err != nil {
		log.Error("cannot connect to mongo", logx.Err(err))
		os.Exit(1)
	}
	defer func() { _ = mongoClient.Disconnect(context.Background()) }()

	queueRedis := redisClient("HISTORY")
	lockRedis := redisClient("LOCK")

	redisStore := projecthistory.NewRedisStore(queueRedis)
	store := projecthistory.NewStore(database)
	web := projecthistory.NewWebAPI(projecthistory.WebAPIConfig{
		URL:  webURL(),
		User: config.Env("WEB_API_USER", "overleaf"),
		Pass: config.Env("WEB_API_PASSWORD", "password"),
	}, redisStore)

	history := projecthistory.NewHistoryStore(projecthistory.HistoryStoreConfig{
		Host: historyV1URL(),
		User: config.Env("V1_HISTORY_USER", "staging"),
		Pass: config.Env("V1_HISTORY_PASSWORD", "password"),
		// Storing a large file is one request, so this is minutes.
		Timeout: time.Duration(
			config.EnvInt("V1_REQUEST_TIMEOUT", 300000)) * time.Millisecond,
		FilestoreURL:     filestoreURL(),
		FilestoreEnabled: os.Getenv("FILESTORE_ENABLED") != "false",
		UploadFolder:     config.Env("UPLOAD_FOLDER", os.TempDir()),
		MaxFileSize:      int64(config.EnvInt("MAX_FILE_SIZE_IN_BYTES", 100*1024*1024)),
	})

	metrics := obsv.New(config.Env("METRICS_APP_NAME", "project-history"))
	service := projecthistory.NewService(projecthistory.ServiceConfig{
		Redis:                redisStore,
		Store:                store,
		Web:                  web,
		History:              history,
		Locks:                projecthistory.NewLockManager(lockRedis),
		Logger:               log,
		Metrics:              newMetrics(metrics),
		ShortHistoryQueues:   splitList(os.Getenv("SHORT_HISTORY_QUEUES")),
		HealthCheckProjectID: os.Getenv("HEALTH_CHECK_PROJECT_ID"),
	})

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metrics.Handler())
	mux.Handle("/", metrics.HTTPMiddleware(log, service.Handler()))

	addr := fmt.Sprintf("%s:%d", config.ListenAddress(),
		config.EnvInt("PROJECT_HISTORY_PORT", 3054))
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
		// A request to this service can wait on history-v1 storing a hundred
		// megabytes, so the write timeout is long and only the headers are
		// bounded tightly.
		ReadHeaderTimeout: 30 * time.Second,
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	go func() {
		sig := <-signals
		log.Info("shutting down", slog.String("signal", sig.String()))

		// A flush that is part way through has taken updates off the queue and
		// not yet written them, so it is given time to finish rather than cut
		// off: the updates it holds are not anywhere else.
		shutdownCtx, done := context.WithTimeout(context.Background(), 30*time.Second)
		defer done()
		_ = server.Shutdown(shutdownCtx)
		cancel()
		os.Exit(0)
	}()

	log.Info("project-history starting up", slog.String("addr", addr))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("stopped", logx.Err(err))
		os.Exit(1)
	}
}

// processorMetrics reports what the flush loop does.
type processorMetrics struct {
	obs *obsv.Metrics
}

// newMetrics builds the reporter.
func newMetrics(obs *obsv.Metrics) *processorMetrics {
	return &processorMetrics{obs: obs}
}

// FlushFinished counts a flush that had something to do.
func (m *processorMetrics) FlushFinished(duration time.Duration, queueSize int) {
	m.obs.Inc("history_flush")
}

// UpdatesDiscarded counts the updates the history had already been told about.
func (m *processorMetrics) UpdatesDiscarded(reason string) {
	m.obs.Inc("updates_discarded_" + reason)
}

// redisClient builds a client for one purpose. The queue and the lock are
// separate connections because the Node service keeps them separate: a
// deployment may put them on different instances.
func redisClient(purpose string) *redis.Client {
	options := &redis.Options{
		Addr:     config.RedisAddr(purpose),
		Password: config.RedisPassword(purpose),
		MaxRetries: config.EnvInt(purpose+"_REDIS_MAX_RETRIES_PER_REQUEST",
			config.EnvInt("REDIS_MAX_RETRIES_PER_REQUEST", 20)),
	}
	if config.RedisTLS() {
		options.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return redis.NewClient(options)
}

func hostPort(host, port string) string {
	if strings.Contains(host, ":") {
		return "[" + host + "]:" + port
	}
	return host + ":" + port
}

func webURL() string {
	host := config.Env("WEB_API_HOST", config.Env("WEB_HOST", "127.0.0.1"))
	port := config.Env("WEB_API_PORT", config.Env("WEB_PORT", "3000"))
	return "http://" + hostPort(host, port)
}

// historyV1URL is where the history is actually stored. The whole url can be
// given, because a deployment may put history-v1 behind something that is not
// plain http on the default port.
func historyV1URL() string {
	if full := os.Getenv("V1_HISTORY_FULL_HOST"); full != "" {
		return full
	}
	if url := os.Getenv("V1_HISTORY_URL"); url != "" {
		return url
	}
	host := config.Env("V1_HISTORY_HOST", config.Env("HISTORY_V1_HOST", "127.0.0.1"))
	return "http://" + hostPort(host, "3100") + "/api"
}

func filestoreURL() string {
	return "http://" + hostPort(config.Env("FILESTORE_HOST", "127.0.0.1"), "3009")
}

// splitList reads a comma separated setting.
func splitList(value string) []string {
	if value == "" {
		return nil
	}
	var items []string
	for _, item := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}
