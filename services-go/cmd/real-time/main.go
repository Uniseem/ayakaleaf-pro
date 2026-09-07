// Command real-time is the Go port of services/real-time.
//
// It speaks the socket.io 0.9 protocol the Overleaf editor's forked client
// expects, authorises connections against web, distributes document updates
// through Redis pub/sub, and tracks who is in which project.
//
// Unlike the stateless services ported before it, this one holds live editing
// sessions: restarting it disconnects everyone currently typing. That is why
// it drains rather than stops.
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
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/obsv"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/realtime"
	"github.com/redis/go-redis/v9"
)

func main() {
	log := logx.New("real-time")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	secrets := config.SessionSecrets()
	if len(secrets) == 0 {
		log.Error("no SESSION_SECRET provided")
		os.Exit(1)
	}

	cfg := realtime.Config{
		CookieName:                  config.Env("COOKIE_NAME", "overleaf.sid"),
		SessionSecrets:              secrets,
		SessionPrefix:               config.Env("SESSION_PREFIX", "sess:"),
		MaxUpdateSize:               config.EnvInt("MAX_UPDATE_SIZE", 7*1024*1024+64*1024),
		PendingUpdateListShardCount: config.EnvInt("PENDING_UPDATE_LIST_SHARD_COUNT", 10),
		PublishOnIndividualChannels: os.Getenv("PUBLISH_ON_INDIVIDUAL_CHANNELS") != "",
		ExposeHostname:              os.Getenv("EXPOSE_HOSTNAME") == "true",
		ContinualPubsubTraffic:      os.Getenv("CONTINUAL_PUBSUB_TRAFFIC") != "",
		ShutdownDrainTimeWindow:     config.EnvInt("SHUTDOWN_DRAIN_TIME_WINDOW", 9),
		GracefulReconnectTimeout: time.Duration(
			config.EnvInt("GRACEFUL_RECONNECT_TIMEOUT_MS", (45+30)*1000)) * time.Millisecond,
		BehindProxy:     true,
		TrustedProxyIPs: config.Env("TRUSTED_PROXY_IPS", "loopback"),
		// server-ce replaces the Redis key schema wholesale; its runit script
		// says so with REDIS_KEY_SCHEMA=server-ce.
		Keys: realtime.NewKeySchema(os.Getenv("REDIS_KEY_SCHEMA")),
	}

	pubsubRedis := redisClient("PUBSUB")
	realtimeRedis := redisClient("REAL_TIME")
	docUpdaterRedis := redisClient("DOC_UPDATER")
	sessionRedis := redisClient("SESSIONS")

	service := realtime.New(ctx, realtime.Deps{
		Log:    log,
		Config: cfg,
		Web: realtime.NewWebClient(
			webURL(),
			config.Env("WEB_API_USER", "overleaf"),
			config.Env("WEB_API_PASSWORD", "password"),
		),
		DocumentUpdater: realtime.NewDocumentUpdaterClient(documentUpdaterURL()),
		Sessions: realtime.NewSessionStore(
			sessionRedis, cfg.CookieName, cfg.SessionSecrets, cfg.SessionPrefix),
		PubSubRedis:     pubsubRedis,
		RealtimeRedis:   realtimeRedis,
		DocUpdaterRedis: docUpdaterRedis,
	})

	metrics := obsv.New(config.Env("METRICS_APP_NAME", "real-time"))
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metrics.Handler())
	mux.Handle("/", service.Handler(os.Getenv("SOCKET_IO_CLIENT_JS"),
		func(next http.Handler) http.Handler { return metrics.HTTPMiddleware(log, next) }))

	addr := fmt.Sprintf("%s:%d", config.ListenAddress(), config.EnvInt("REALTIME_PORT", 3026))
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
		// Long-polling holds a GET open for 20 seconds, and a websocket for as
		// long as the user is editing, so there is no useful write timeout.
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout: time.Duration(
			config.EnvInt("KEEPALIVE_TIMEOUT_MS", 5000)) * time.Millisecond,
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	go func() {
		sig := <-signals
		service.DrainAndShutdown(sig.String(), func() {
			shutdownCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
			defer done()
			_ = server.Shutdown(shutdownCtx)
			cancel()
			os.Exit(0)
		})
	}()

	log.Info("realtime starting up", slog.String("addr", addr))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("stopped", logx.Err(err))
		os.Exit(1)
	}
}

// redisClient builds a client for one purpose. The four purposes are separate
// connections because the Node service keeps them separate: a deployment may
// point sessions at one instance and pub/sub at another.
func redisClient(purpose string) *redis.Client {
	options := &redis.Options{
		Addr:     config.RedisAddr(purpose),
		Password: config.RedisPassword(purpose),
		// A publish or a session lookup that cannot complete should fail the
		// request rather than hang the connection it belongs to.
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

func documentUpdaterURL() string {
	host := config.Env("DOCUMENT_UPDATER_HOST", config.Env("DOCUPDATER_HOST", "127.0.0.1"))
	return "http://" + hostPort(host, "3003")
}
