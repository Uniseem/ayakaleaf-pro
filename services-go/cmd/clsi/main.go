// Command clsi runs LaTeX.
//
// It writes a project's files into a directory, runs latexmk over them and
// keeps what came out where nginx can serve it. Nothing it produces is served
// by this process: a PDF goes to the browser straight off the disk.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/clsi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/config"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/httpx"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/logx"
)

func main() {
	log := logx.New("clsi")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	options := clsi.Options{
		CompilesDir: config.Env("COMPILES_DIR", "/var/lib/overleaf/data/compiles"),
		OutputDir:   config.Env("OUTPUT_DIR", "/var/lib/overleaf/data/output"),
		User:        config.Env("TEXLIVE_IMAGE_USER", "www-data"),
		Sandboxed: config.Env("SANDBOXED_COMPILES", "") == "true" ||
			config.Env("DOCKER_RUNNER", "") == "true",
		Image:         config.Env("TEX_LIVE_DOCKER_IMAGE", ""),
		AllowedImages: split(config.Env("ALL_TEX_LIVE_DOCKER_IMAGES", "")),
		// The two directories as the docker daemon sees them, which is not how
		// this process sees them: it is in a container and the daemon is not.
		HostCompilesDir: config.Env("SANDBOXED_COMPILES_HOST_DIR_COMPILES",
			config.Env("COMPILES_HOST_DIR", "")),
		HostOutputDir: config.Env("SANDBOXED_COMPILES_HOST_DIR_OUTPUT",
			config.Env("OUTPUT_HOST_DIR", "")),
		MaxTimeout: time.Duration(config.EnvInt("COMPILE_TIMEOUT", 180)) * time.Second,
		ProjectLife: time.Duration(
			config.EnvInt("PROJECT_CACHE_LENGTH_MS", 24*60*60*1000)) * time.Millisecond,
		SeccompProfile: config.Env("SECCOMP_PROFILE", ""),
	}
	if options.Sandboxed && options.HostCompilesDir == "" {
		// The mount would be made from a path the daemon cannot see, and every
		// compile would run against an empty directory and fail with a missing
		// file. Better to say so now.
		log.Log(ctx, logx.LevelFatal,
			"Sandboxed compiles need the compile directory's path on the host. Exiting.")
		os.Exit(1)
	}

	if err := os.MkdirAll(options.CompilesDir, 0o755); err != nil {
		log.Log(ctx, logx.LevelFatal, "Cannot make the compiles directory. Exiting.", logx.Err(err))
		os.Exit(1)
	}
	if err := os.MkdirAll(options.OutputDir, 0o755); err != nil {
		log.Log(ctx, logx.LevelFatal, "Cannot make the output directory. Exiting.", logx.Err(err))
		os.Exit(1)
	}

	service := clsi.New(log, options)
	// Projects nobody has compiled for a while are removed on a timer: a
	// compile directory is the whole project plus everything LaTeX wrote, and
	// they would otherwise only ever grow.
	service.Expire(ctx)

	addr := fmt.Sprintf("%s:%d", config.ListenAddress(), config.EnvInt("CLSI_PORT", 3013))
	srv := &http.Server{
		Addr:              addr,
		Handler:           service.Handler(),
		ReadHeaderTimeout: 30 * time.Second,
		// A compile is allowed to take as long as the site permits, and this
		// request waits for it.
		WriteTimeout: options.MaxTimeout + 2*time.Minute,
	}
	log.Info("clsi listening", slog.String("addr", addr),
		slog.Bool("sandboxed", options.Sandboxed))
	if err := httpx.ListenAndServe(srv, log); err != nil {
		log.Error("clsi HTTP server stopped", logx.Err(err), slog.String("addr", addr))
		os.Exit(1)
	}
}

func split(list string) []string {
	var out []string
	for _, entry := range strings.Split(list, ",") {
		if entry = strings.TrimSpace(entry); entry != "" {
			out = append(out, entry)
		}
	}
	return out
}
