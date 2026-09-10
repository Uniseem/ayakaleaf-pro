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
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/compile"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/documents"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/githubsync"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/history"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/internalapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/settings"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/tokens"
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

	// The addresses of the other services come from the same variables every
	// service in the deployment reads, so there is nothing new to set: a
	// container that can already run the editor can run this.
	filestoreURL := serviceURL("FILESTORE", "3009")

	rdb := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr("web"),
		Password: config.RedisPassword("web"),
	})
	defer func() { _ = rdb.Close() }()

	pubsubRedis := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr("PUBSUB"),
		Password: config.RedisPassword("PUBSUB"),
	})
	defer func() { _ = pubsubRedis.Close() }()

	userStore := users.NewStore(db)
	// Indexes are ensured at startup rather than by a migration: this service
	// depends on the email one being unique, and a service that needs a
	// constraint should not assume somebody else applied it.
	if err := userStore.EnsureIndexes(ctx); err != nil {
		log.Warn("could not ensure the user indexes", logx.Err(err))
	}

	projectStore := projects.NewStore(db)
	if err := projectStore.EnsureIndexes(ctx); err != nil {
		log.Warn("could not ensure the project indexes", logx.Err(err))
	}

	tokenStore := tokens.NewStore(db)
	if err := tokenStore.EnsureIndexes(ctx); err != nil {
		log.Warn("could not ensure the token indexes", logx.Err(err))
	}

	// The two history services, and the blob store they share with filestore.
	histories := history.NewClient(history.Options{
		ProjectHistoryURL: serviceURL("PROJECT_HISTORY", "3054"),
		HistoryV1URL:      config.Env("V1_HISTORY_URL", "http://127.0.0.1:3100/api"),
		FilestoreURL:      filestoreURL,
		HistoryV1User:     config.Env("V1_HISTORY_USER", "staging"),
		HistoryV1Password: config.Env("STAGING_PASSWORD", ""),
	})
	// Which blobs are shared by every project decides how their addresses are
	// built, and getting it wrong is a file that cannot be fetched.
	if err := histories.LoadGlobalBlobs(ctx, db); err != nil {
		log.Warn("could not read the list of shared blobs", logx.Err(err))
	}

	githubStore := githubsync.NewStore(db, gitSecret())
	if err := githubStore.EnsureIndexes(ctx); err != nil {
		log.Warn("could not ensure the github sync indexes", logx.Err(err))
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
		Log:       log,
		Users:     userStore,
		Sessions:  sessions,
		Settings:  siteSettings,
		Projects:  projectStore,
		Documents: documents.NewClient(serviceURL("DOCUPDATER", "3003")),
		Storage:   documents.NewStorage(serviceURL("DOCSTORE", "3016")),
		Compiler:  compile.NewClient(serviceURL("CLSI", "3013")),
		History:   histories,
		Tokens:    tokenStore,
		// The git container reaches this service by the name the deployment
		// gives it, which is not the address people type into a browser.
		GitBaseURL:        config.Env("GIT_BRIDGE_CALLBACK_BASE_URL", "http://ayakaleaf"),
		GitSecret:         gitSecret(),
		ChatURL:           serviceURL("CHAT", "3010"),
		ProjectHistoryURL: serviceURL("PROJECT_HISTORY", "3054"),
		Database:          db,
		AllowedOrigins:    allowedOrigins(siteSettings),
		// Telling the editors of a project that something happened to it goes
		// through the same channel real-time subscribes to, so it has to be
		// the pub/sub Redis and the same sharding setting real-time was given.
		PubSub:                      pubsubRedis,
		PublishOnIndividualChannels: os.Getenv("PUBLISH_ON_INDIVIDUAL_CHANNELS") != "",
	})

	// The password-reset tokens are unique and expire on their own, both of
	// which are the index's job.
	if err := server.EnsureAuthIndexes(ctx); err != nil {
		log.Warn("could not ensure the password reset indexes", logx.Err(err))
	}

	// The other services in this deployment ask this one things -- whether
	// somebody may open a project, what a stored document says, where a
	// project's history is. That is a different surface from the one a browser
	// reaches, with different shapes and its own authentication, so it is a
	// second listener on the port those services already look for it on.
	internal := internalapi.New(internalapi.Options{
		Log:        log,
		Projects:   projectStore,
		Users:      userStore,
		Storage:    documents.NewStorage(serviceURL("DOCSTORE", "3016")),
		Docupdater: documents.NewClient(serviceURL("DOCUPDATER", "3003")),
		ChatURL:    serviceURL("CHAT", "3010"),
		User:       config.Env("WEB_API_USER", "overleaf"),
		Password:   config.Env("WEB_API_PASSWORD", "password"),
	})
	internalAddr := fmt.Sprintf("%s:%d",
		config.ListenAddress(), config.EnvInt("WEB_API_PORT", 3000))
	internalServer := &http.Server{
		Addr:              internalAddr,
		Handler:           internal.Handler(),
		ReadHeaderTimeout: 30 * time.Second,
		WriteTimeout:      internalapi.ReadTimeout(),
	}
	go func() {
		log.Info("internal api listening", slog.String("addr", internalAddr))
		if err := internalServer.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			// Without this the editor cannot open a project: real-time asks
			// here before it lets anybody in.
			log.Log(ctx, logx.LevelFatal, "The internal API stopped. Exiting.",
				logx.Err(err), slog.String("addr", internalAddr))
			os.Exit(1)
		}
	}()

	addr := fmt.Sprintf("%s:%d", config.ListenAddress(), config.EnvInt("API_PORT", 3400))
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

// serviceURL is where one of the other services in this deployment listens.
//
// The names are the ones the Node services agreed on years ago and that the
// container still exports, so this reads the deployment as it is rather than
// asking an operator to describe it again.
func serviceURL(name, defaultPort string) string {
	return "http://" + config.Env(name+"_HOST", "127.0.0.1") +
		":" + config.Env(name+"_PORT", defaultPort)
}

// gitSecret signs the download links handed to the git container.
//
// The session secret, because it is already the deployment's one secret and
// these links are of the same kind: something this service produced, that
// something else presents back, and that nobody else can forge.
func gitSecret() string {
	secrets := config.SessionSecrets()
	if len(secrets) == 0 {
		return ""
	}
	return secrets[0]
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
