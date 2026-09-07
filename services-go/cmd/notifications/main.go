// Command notifications is a drop-in replacement for services/notifications,
// serving the same HTTP API against the same MongoDB collection.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/config"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/httpx"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/logx"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/mongox"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/notifications"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/obsv"
)

func main() {
	log := logx.New("notifications")

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

	port := config.EnvInt("NOTIFICATIONS_PORT", 3042)
	metrics := obsv.New(config.Env("METRICS_APP_NAME", "notifications"))
	server := notifications.NewServer(
		notifications.NewStore(db),
		log,
		metrics,
		fmt.Sprintf("http://127.0.0.1:%d", port),
		&http.Client{Timeout: 10 * time.Second},
	)

	addr := fmt.Sprintf("%s:%d", config.ListenAddress(), port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 30 * time.Second,
	}
	if err := httpx.ListenAndServe(srv, log); err != nil {
		log.Error("notifications HTTP server stopped", logx.Err(err), slog.String("addr", addr))
		os.Exit(1)
	}
}
