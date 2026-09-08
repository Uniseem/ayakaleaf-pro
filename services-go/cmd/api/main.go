// Command api serves the application's HTTP API.
//
// It is what replaces the Node web service: the same database, the same
// sessions, and the client talks to it instead of to a server that rendered
// pages. Everything it needs to run comes from the environment the container
// already sets, so a deployment does not gain a configuration step by gaining
// this process.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/settings"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/users"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/config"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/httpx"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/logx"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/mongox"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/session"
	"github.com/redis/go-redis/v9"
)

func main() {
	log := logx.New("api")
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

	rdb := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr("web"),
		Password: config.RedisPassword("web"),
	})
	defer func() { _ = rdb.Close() }()

	userStore := users.NewStore(db)
	// Indexes are ensured at startup rather than by a migration: this service
	// depends on the email one being unique, and a service that needs a
	// constraint should not assume somebody else applied it.
	if err := userStore.EnsureIndexes(ctx); err != nil {
		log.Warn("could not ensure the user indexes", logx.Err(err))
	}

	siteSettings := settings.NewStore(db, rdb, log)
	if err := siteSettings.Load(ctx); err != nil {
		log.Log(ctx, logx.LevelFatal, "Cannot read the site settings. Exiting.", logx.Err(err))
		os.Exit(1)
	}
	siteSettings.Watch(ctx)

	sessions := session.NewStore(rdb, session.Options{
		CookieName: config.Env("COOKIE_NAME", "overleaf.sid"),
		Secrets:    config.SessionSecrets(),
		TTL:        time.Duration(config.EnvInt("OVERLEAF_COOKIE_SESSION_LENGTH", 5*24*60*60*1000)) * time.Millisecond,
		Secure:     config.Env("OVERLEAF_SECURE_COOKIE", "") != "",
	})

	server := api.New(api.Options{
		Log:            log,
		Users:          userStore,
		Sessions:       sessions,
		Settings:       siteSettings,
		AllowedOrigins: allowedOrigins(siteSettings),
	})

	addr := fmt.Sprintf("%s:%d", config.ListenAddress(), config.EnvInt("API_PORT", 3100))
	srv := &http.Server{
		Addr:              addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 30 * time.Second,
	}
	log.Info("api listening", slog.String("addr", addr))
	if err := httpx.ListenAndServe(srv, log); err != nil {
		log.Error("api HTTP server stopped", logx.Err(err), slog.String("addr", addr))
		os.Exit(1)
	}
}

// allowedOrigins is where a browser may send a state-changing request from:
// the site's own address, and anything an operator added.
func allowedOrigins(s *settings.Store) []string {
	origins := []string{}
	if siteURL := strings.TrimRight(s.Values().SiteURL, "/"); siteURL != "" {
		origins = append(origins, siteURL)
	}
	for _, extra := range strings.Split(config.Env("API_ALLOWED_ORIGINS", ""), ",") {
		if extra = strings.TrimSpace(extra); extra != "" {
			origins = append(origins, strings.TrimRight(extra, "/"))
		}
	}
	return origins
}
