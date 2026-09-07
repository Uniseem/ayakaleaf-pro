// Command chat is a drop-in replacement for services/chat, serving the same
// HTTP API against the same MongoDB collections.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/chat"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/config"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/httpx"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/logx"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/mongox"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/obsv"
)

func main() {
	log := logx.New("chat")

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

	metrics := obsv.New(config.Env("METRICS_APP_NAME", "chat"))
	server := chat.NewServer(chat.NewStore(db), log, metrics)

	addr := fmt.Sprintf("%s:%d", config.ListenAddress(), config.EnvInt("CHAT_PORT", 3010))
	srv := &http.Server{
		Addr:              addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 30 * time.Second,
	}
	if err := httpx.ListenAndServe(srv, log); err != nil {
		log.Error("chat HTTP server stopped", logx.Err(err), slog.String("addr", addr))
		os.Exit(1)
	}
}
