// Command linked-url-proxy is a drop-in replacement for
// services/linked-url-proxy, fetching user-supplied URLs under the same SSRF
// restrictions.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/config"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/httpx"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/logx"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/obsv"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/proxy"
)

var cidrSeparator = regexp.MustCompile(`[,\s]+`)

func main() {
	log := logx.New("linked-url-proxy")

	cfg, err := loadConfig()
	if err != nil {
		// The Node service defers this failure to request time and answers
		// 500 on every proxy call; failing at startup surfaces a
		// misconfiguration where an operator will actually see it.
		log.Log(context.Background(), logx.LevelFatal, "invalid configuration. Exiting.", logx.Err(err))
		os.Exit(1)
	}

	// Log the effective policy: an operator debugging a blocked link needs to
	// know whether the allow-list actually loaded.
	log.Info("linked-url-proxy policy",
		slog.Int("blockedNetworks", len(cfg.BlockedNetworks)),
		slog.Bool("allowedResourcesConfigured", cfg.AllowedResources != nil),
		slog.Int64("maxUploadSizeBytes", cfg.MaxUploadSize),
	)

	metrics := obsv.New(config.Env("METRICS_APP_NAME", "linked-url-proxy"))
	server := proxy.NewServer(cfg, log, metrics)

	// The Node service binds to LINKED_URL_PROXY_HOST rather than
	// LISTEN_ADDRESS; kept as-is so existing deployments are unaffected.
	host := config.Env("LINKED_URL_PROXY_HOST", "127.0.0.1")
	addr := fmt.Sprintf("%s:%d", host, config.EnvInt("LINKED_URL_PROXY_PORT", 3066))
	srv := &http.Server{
		Addr:              addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 30 * time.Second,
	}
	if err := httpx.ListenAndServe(srv, log); err != nil {
		log.Error("linked-url-proxy HTTP server stopped", logx.Err(err), slog.String("addr", addr))
		os.Exit(1)
	}
}

func loadConfig() (proxy.Config, error) {
	cfg := proxy.Config{
		MaxRedirects: 5,
		FetchTimeout: 30 * time.Second,
		UserAgent:    "Overleaf Extended CE - LinkedURLProxy (https://github.com/yu-i-i/overleaf-cep)",
		// MAX_UPLOAD_SIZE is in megabytes; the default is 50 MB.
		MaxUploadSize: int64(config.EnvInt("MAX_UPLOAD_SIZE", 50)) * 1024 * 1024,
	}

	var cidrs []string
	for _, entry := range cidrSeparator.Split(os.Getenv("OVERLEAF_LINKED_URL_BLOCKED_NETWORKS"), -1) {
		if entry = strings.TrimSpace(entry); entry != "" {
			cidrs = append(cidrs, entry)
		}
	}
	blocked, err := proxy.ParseBlockedNetworks(cidrs)
	if err != nil {
		return cfg, err
	}
	cfg.BlockedNetworks = blocked

	if pattern := os.Getenv("OVERLEAF_LINKED_URL_ALLOWED_RESOURCES"); pattern != "" {
		allowed, err := regexp.Compile(pattern)
		if err != nil {
			return cfg, fmt.Errorf("OVERLEAF_LINKED_URL_ALLOWED_RESOURCES is not a valid regular expression: %w", err)
		}
		cfg.AllowedResources = allowed
	}
	return cfg, nil
}
