// Command document-updater is the Go port of services/document-updater.
//
// It holds the documents people are editing in Redis, applies the edits
// real-time queues for it, transforms an edit built against an older version
// against what landed in between, moves the tracked changes and comments to
// match, publishes the result to the other editors, records it for the history,
// and writes the document back to the database when the editing stops.
//
// It is the service everything else waits on: web reads documents through it,
// the compile path reads them through it, and every keystroke passes through
// it. Restarting it does not disconnect anybody, but an edit applied wrongly is
// permanent, so it stops by refusing new work and finishing what it has.
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
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/docupdater"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/logx"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/mongox"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/obsv"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/rediskeys"
	"github.com/redis/go-redis/v9"
)

func main() {
	log := logx.New("document-updater")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := docupdater.Config{
		MaxDocLength: config.EnvInt("MAX_DOC_LENGTH", 2*1024*1024),
		// It has to match what real-time was told, or edits are queued on
		// shards nobody is waiting on.
		DispatcherCount: config.EnvInt("DISPATCHER_COUNT", 10),
		SmoothingOffset: time.Duration(
			config.EnvInt("SMOOTHING_OFFSET", 1000)) * time.Millisecond,
		ContinuousBackgroundFlush:   os.Getenv("CONTINUOUS_BACKGROUND_FLUSH") == "true",
		PublishOnIndividualChannels: os.Getenv("PUBLISH_ON_INDIVIDUAL_CHANNELS") == "true",
		GracefulShutdownDelay: time.Duration(
			config.EnvInt("GRACEFUL_SHUTDOWN_DELAY_SECONDS", 10)) * time.Second,
		// server-ce replaces the Redis key schema wholesale; its runit script
		// says so with REDIS_KEY_SCHEMA=server-ce.
		Keys: rediskeys.New(os.Getenv("REDIS_KEY_SCHEMA")),
	}

	// Mongo is used for one thing: keeping a copy of a document from before an
	// edit destroyed a marker in it. The Node service exits when it cannot
	// reach Mongo at startup, so this one does too.
	mongoClient, database, err := mongox.Connect(ctx)
	if err != nil {
		log.Error("cannot connect to mongo", logx.Err(err))
		os.Exit(1)
	}
	defer func() { _ = mongoClient.Disconnect(context.Background()) }()

	web := webURL()
	user := config.Env("WEB_API_USER", "overleaf")
	password := config.Env("WEB_API_PASSWORD", "password")
	history := docupdater.NewHistoryClient(projectHistoryURL(), log)

	service := docupdater.New(ctx, docupdater.Deps{
		Log:             log,
		Config:          cfg,
		DocUpdaterRedis: redisClient("DOC_UPDATER"),
		PubSubRedis:     redisClient("PUBSUB"),
		Web:             docupdater.NewWebClient(web, user, password),
		Persistence:     docupdater.NewPersistenceClient(web, user, password),
		History:         history,
		Snapshots:       docupdater.NewSnapshotStore(database),
	})

	metrics := obsv.New(config.Env("METRICS_APP_NAME", "document-updater"))
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metrics.Handler())
	mux.Handle("/", service.Server(history).Handler(
		func(next http.Handler) http.Handler { return metrics.HTTPMiddleware(log, next) }))

	addr := fmt.Sprintf("%s:%d", config.ListenAddress(),
		config.EnvInt("DOCUPDATER_PORT", 3003))
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	go func() {
		sig := <-signals
		log.Info("received interrupt, cleaning up", slog.String("signal", sig.String()))

		// The delay is what lets the requests already in flight finish, and
		// lets whatever is in front of this stop sending new ones. Only then
		// are the dispatchers stopped, so an edit is never half applied.
		time.Sleep(service.GracefulShutdownDelay())
		log.Info("shutting down", slog.String("signal", sig.String()))

		shutdownCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		_ = server.Shutdown(shutdownCtx)
		cancel()
		os.Exit(0)
	}()

	log.Info("document-updater starting up", slog.String("addr", addr))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("stopped", logx.Err(err))
		os.Exit(1)
	}
}

// redisClient builds a client for one purpose. The purposes are separate
// connections because the Node service keeps them separate: a deployment may
// point pub/sub at a different instance from the documents.
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

// projectHistoryURL is where the history queue is drained. The port is fixed in
// the Node service, so only the host is configurable here too.
func projectHistoryURL() string {
	return "http://" + hostPort(config.Env("PROJECT_HISTORY_HOST", "127.0.0.1"), "3054")
}
