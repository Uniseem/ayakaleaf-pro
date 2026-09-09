// Package api is the application's HTTP surface: one process, and inside it
// one package per part of the product.
//
// One process rather than a service per feature. The parts of this application
// share a session, a permission model and a project document, and splitting
// them across processes turns each of those into a distributed problem without
// making any of them more modular -- a package boundary already prevents the
// coupling that matters, and it does so at compile time.
//
// Every route is declared here, in one place. Reading this file is how you
// find out what the API is.
package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/auth"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/chat"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/compile"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/documents"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/gitbridge"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/githubsync"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/history"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/mailer"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/oauth"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projecthistory"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/settings"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/sharing"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/tags"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/tokens"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/users"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/session"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Server holds everything the routes are served from.
type Server struct {
	log       *slog.Logger
	users     *users.Store
	sessions  *session.Store
	settings  *settings.Store
	auth      *auth.Service
	projects  *projects.Service
	oauth     *oauth.Service
	documents *documents.Service
	compile   *compile.Service
	tokens    *tokens.Service
	git       *gitbridge.Service
	github    *githubsync.Service
	chat      *chat.Service
	prefs     *settings.UserService
	tags      *tags.Service
	sharing   *sharing.Service
	histories *projecthistory.Service
	origins   []string
}

// Options is what a Server needs.
type Options struct {
	Log      *slog.Logger
	Users    *users.Store
	Sessions *session.Store
	Settings *settings.Store
	Projects *projects.Store
	// Documents and Compiler are the two services this one does not own: the
	// text of a file while somebody has it open, and the compiler. Both are
	// passed in as clients so that what they are is a deployment decision and
	// not something the routes know.
	Documents *documents.Client
	// Storage is where a document's text goes when it is made and when it is
	// deleted, which are the only two moments there is no editing session to
	// go through instead.
	Storage  *documents.Storage
	Compiler *compile.Client
	// History is where a project's versions and the bytes of its files live.
	History *history.Client
	// Tokens are the personal access tokens git authenticates with.
	Tokens *tokens.Store
	// GitHub is where a project's link to a repository is kept.
	GitHub *githubsync.Store
	// GitBaseURL is the address the git container reaches this service at, and
	// GitSecret signs the download links handed to it.
	GitBaseURL string
	GitSecret  string
	// ProjectHistoryURL is where the summarising history service is. Like
	// chat, it answers for any project id it is given and checks nothing,
	// which is why it is only reachable from inside.
	ProjectHistoryURL string
	// ChatURL is where the chat service is, for the messages people leave on
	// a project. This service is the only thing between it and a browser: it
	// has no idea who may read a project.
	ChatURL string
	// Database is needed for the per-person settings, which live on the user
	// document rather than in a store of their own.
	Database *mongo.Database
	// AllowedOrigins are the addresses a browser may send a state-changing
	// request from. The site's own is enough unless something else embeds it.
	AllowedOrigins []string
}

// New builds the server.
func New(opts Options) *Server {
	server := &Server{
		log:      opts.Log,
		users:    opts.Users,
		sessions: opts.Sessions,
		settings: opts.Settings,
		auth: auth.New(opts.Users, opts.Sessions, opts.Settings).
			WithReset(opts.Database, mailer.New(opts.Settings)),
		chat:    chat.New(opts.Projects, opts.Users, opts.ChatURL),
		prefs:   settings.NewUserService(opts.Database),
		tags:    tags.New(opts.Database),
		sharing: sharing.New(opts.Projects, opts.Users, opts.Database, mailer.New(opts.Settings)),
		origins: opts.AllowedOrigins,
	}
	server.documents = documents.NewService(
		opts.Projects, opts.Documents, opts.Storage, opts.History)
	// A new project is given its first file by the documents service, so the
	// two are wired together here rather than knowing about each other.
	server.histories = projecthistory.New(
		opts.Projects, opts.Users, opts.History, server.documents, opts.ProjectHistoryURL)
	server.projects = projects.NewService(opts.Projects, server.documents)
	// A deleted project has to come off everybody's tags.
	server.projects.OnDelete(server.tags)
	server.compile = compile.NewService(
		opts.Projects, opts.Documents, opts.Compiler, opts.Settings, opts.History)
	server.tokens = tokens.NewService(opts.Tokens)
	server.github = githubsync.NewService(githubsync.Options{
		Log:        opts.Log,
		Store:      opts.GitHub,
		Projects:   opts.Projects,
		Users:      opts.Users,
		Documents:  server.documents,
		Docupdater: opts.Documents,
		History:    opts.History,
		Sessions:   opts.Sessions,
		Settings:   opts.Settings,
	})
	server.git = gitbridge.NewService(gitbridge.Options{
		Log:       opts.Log,
		Projects:  opts.Projects,
		Users:     opts.Users,
		Tokens:    opts.Tokens,
		Documents: server.documents,
		History:   opts.History,
		Settings:  opts.Settings,
		BaseURL:   opts.GitBaseURL,
		Secret:    opts.GitSecret,
	})
	// The OAuth handlers start a session for somebody they identified, which
	// is the auth service's job: passing it in rather than repeating it keeps
	// one place where a session begins.
	server.oauth = oauth.NewService(opts.Users, opts.Sessions, opts.Settings, server.auth.SignInUser)
	return server
}

// Handler is the whole API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	h := func(fn httpapi.Handler) http.HandlerFunc { return httpapi.Wrap(s.log, fn) }

	// Liveness. It asks for no session and reads nothing, and the session
	// middleware treats a Redis it cannot reach as an anonymous visitor rather
	// than as a failure, so a broken Redis does not make this service look
	// dead to whatever is watching it.
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		_ = httpapi.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// What every page needs to know about the site itself, whoever is asking.
	mux.HandleFunc("GET /api/site", h(s.site))

	// Signing up and in.
	mux.HandleFunc("GET /api/auth/status", h(s.auth.Status))
	mux.HandleFunc("POST /api/auth/register", h(s.auth.Register))
	mux.HandleFunc("POST /api/auth/login", h(s.auth.Login))
	mux.HandleFunc("POST /api/auth/logout", h(s.auth.Logout))
	mux.HandleFunc("GET /api/auth/me", h(s.auth.Me))

	// Signing in with somebody else's identity, and attaching one to an
	// account that already exists. Which of the two a request means is decided
	// by whether it arrives signed in.
	mux.HandleFunc("GET /api/auth/providers", h(s.oauth.Linked))
	mux.HandleFunc("GET /api/auth/{provider}", h(s.oauth.Start))
	mux.HandleFunc("GET /api/auth/{provider}/callback", h(s.oauth.Callback))
	mux.HandleFunc("DELETE /api/auth/{provider}/link", h(s.oauth.Unlink))

	// Projects.
	mux.HandleFunc("GET /api/projects", h(s.projects.List))
	mux.HandleFunc("POST /api/projects", h(s.projects.Create))
	mux.HandleFunc("GET /api/projects/{id}", h(s.projects.Get))
	mux.HandleFunc("POST /api/projects/{id}/rename", h(s.projects.Rename))
	mux.HandleFunc("POST /api/projects/{id}/archive", h(s.projects.Archive))
	mux.HandleFunc("POST /api/projects/{id}/trash", h(s.projects.Trash))
	mux.HandleFunc("DELETE /api/projects/{id}", h(s.projects.Delete))

	// The files inside a project, and turning them into a PDF.
	mux.HandleFunc("POST /api/projects/{id}/documents", h(s.documents.Create))
	mux.HandleFunc("GET /api/projects/{id}/documents/{docId}", h(s.documents.Get))
	mux.HandleFunc("POST /api/projects/{id}/documents/{docId}", h(s.documents.SetContent))
	mux.HandleFunc("GET /api/projects/{id}/documents/{docId}/changes", h(s.documents.Changes))
	mux.HandleFunc("POST /api/projects/{id}/documents/{docId}/changes/accept", h(s.documents.AcceptChanges))
	mux.HandleFunc("POST /api/projects/{id}/folders", h(s.documents.CreateFolder))
	mux.HandleFunc("POST /api/projects/{id}/entries/{entryId}/rename", h(s.documents.Rename))
	mux.HandleFunc("DELETE /api/projects/{id}/entries/{entryId}", h(s.documents.Delete))
	mux.HandleFunc("POST /api/projects/{id}/clone", h(s.documents.Clone))
	mux.HandleFunc("GET /api/projects/{id}/history/updates", h(s.histories.Updates))
	mux.HandleFunc("GET /api/projects/{id}/history/diff", h(s.histories.Diff))
	mux.HandleFunc("GET /api/projects/{id}/history/changes", h(s.histories.Changes))
	mux.HandleFunc("GET /api/projects/{id}/history/labels", h(s.histories.Labels))
	mux.HandleFunc("POST /api/projects/{id}/history/labels", h(s.histories.CreateLabel))
	mux.HandleFunc("POST /api/projects/{id}/history/restore", h(s.histories.Restore))

	mux.HandleFunc("GET /api/projects/{id}/sharing", h(s.sharing.List))
	mux.HandleFunc("POST /api/projects/{id}/sharing/invites", h(s.sharing.Invite))
	mux.HandleFunc("DELETE /api/projects/{id}/sharing/invites/{inviteId}", h(s.sharing.RevokeInvite))
	mux.HandleFunc("POST /api/projects/{id}/sharing/members/{userId}", h(s.sharing.SetPrivilege))
	mux.HandleFunc("DELETE /api/projects/{id}/sharing/members/{userId}", h(s.sharing.Remove))
	mux.HandleFunc("POST /api/projects/{id}/sharing/public", h(s.sharing.SetPublicAccess))
	mux.HandleFunc("POST /api/invites/{token}/accept", h(s.sharing.Accept))
	mux.HandleFunc("POST /api/projects/{id}/entries/{entryId}/move", h(s.documents.Move))
	mux.HandleFunc("POST /api/projects/{id}/root-doc", h(s.documents.SetRootDoc))
	mux.HandleFunc("POST /api/projects/{id}/uploads", h(s.documents.Upload))
	mux.HandleFunc("GET /api/projects/{id}/files/{fileId}", h(s.documents.ReadFile))
	mux.HandleFunc("GET /api/projects/{id}/search", h(s.documents.Search))
	mux.HandleFunc("GET /api/projects/{id}/messages", h(s.chat.List))
	mux.HandleFunc("POST /api/projects/{id}/messages", h(s.chat.Send))
	mux.HandleFunc("GET /api/projects/{id}/threads", h(s.chat.Threads))
	mux.HandleFunc("POST /api/projects/{id}/threads/{threadId}/messages", h(s.chat.Comment))
	mux.HandleFunc("POST /api/projects/{id}/threads/{threadId}/resolve", h(s.chat.ResolveThread))
	mux.HandleFunc("POST /api/projects/{id}/threads/{threadId}/reopen", h(s.chat.ReopenThread))
	mux.HandleFunc("DELETE /api/projects/{id}/threads/{threadId}", h(s.chat.DeleteThread))
	mux.HandleFunc("POST /api/projects/{id}/compile", h(s.compile.Compile))
	mux.HandleFunc("POST /api/projects/{id}/compile/stop", h(s.compile.Stop))
	mux.HandleFunc("GET /api/projects/{id}/wordcount", h(s.compile.WordCount))

	// The tokens somebody uses instead of a password, which today means git.
	mux.HandleFunc("GET /api/tokens", h(s.tokens.List))
	mux.HandleFunc("POST /api/tokens", h(s.tokens.Create))
	mux.HandleFunc("DELETE /api/tokens/{tokenId}", h(s.tokens.Delete))

	// What the git container asks for. Its own shapes, its own way of
	// authenticating, and the same access rules as everything else.
	mux.HandleFunc("GET /oauth/token/info", h(s.git.TokenInfo))
	mux.HandleFunc("GET /api/v0/docs/{projectId}", h(s.git.GetDoc))
	mux.HandleFunc("GET /api/v0/docs/{projectId}/saved_vers", h(s.git.GetSavedVers))
	mux.HandleFunc("GET /api/v0/docs/{projectId}/snapshots/{version}", h(s.git.GetSnapshot))
	mux.HandleFunc("POST /api/v0/docs/{projectId}/snapshots", h(s.git.PostSnapshot))
	mux.HandleFunc("GET /api/v0/blobs/{historyId}/{hash}", h(s.git.Blob))

	// GitHub: connecting an account, and keeping one project in step with one
	// repository.
	mux.HandleFunc("GET /api/github/status", h(s.github.Status))
	mux.HandleFunc("GET /api/github/start", h(s.github.Start))
	mux.HandleFunc("GET /api/github/callback", h(s.github.Callback))
	mux.HandleFunc("DELETE /api/github/link", h(s.github.Unlink))
	mux.HandleFunc("GET /api/github/repos", h(s.github.Repos))
	mux.HandleFunc("GET /api/github/owners", h(s.github.Owners))
	mux.HandleFunc("POST /api/github/import", h(s.github.ImportRepo))
	mux.HandleFunc("GET /api/projects/{id}/github", h(s.github.ProjectStatus))
	mux.HandleFunc("GET /api/projects/{id}/github/overview", h(s.github.Overview))
	mux.HandleFunc("POST /api/projects/{id}/github/sync", h(s.github.Merge))
	mux.HandleFunc("POST /api/projects/{id}/github/export", h(s.github.ExportProject))
	mux.HandleFunc("DELETE /api/projects/{id}/github", h(s.github.UnlinkProject))

	// The admin pages.
	mux.HandleFunc("GET /api/tags", h(s.tags.List))
	mux.HandleFunc("POST /api/tags", h(s.tags.Create))
	mux.HandleFunc("POST /api/tags/{tagId}", h(s.tags.Update))
	mux.HandleFunc("DELETE /api/tags/{tagId}", h(s.tags.Delete))
	mux.HandleFunc("POST /api/tags/{tagId}/projects/{projectId}", h(s.tags.AddProject))
	mux.HandleFunc("DELETE /api/tags/{tagId}/projects/{projectId}", h(s.tags.RemoveProject))

	mux.HandleFunc("GET /api/settings", h(s.prefs.Get))
	mux.HandleFunc("POST /api/settings", h(s.prefs.Set))
	mux.HandleFunc("POST /api/settings/password", h(s.auth.ChangePassword))
	mux.HandleFunc("GET /api/settings/sessions", h(s.auth.Sessions))
	mux.HandleFunc("DELETE /api/settings/sessions", h(s.auth.ClearSessions))

	mux.HandleFunc("POST /api/auth/password/reset", h(s.auth.RequestReset))
	mux.HandleFunc("POST /api/auth/password/set", h(s.auth.SetPasswordFromToken))

	mux.HandleFunc("GET /api/admin/settings", h(s.getSettings))
	mux.HandleFunc("POST /api/admin/settings", h(s.putSettings))

	// Anything under /api that no route claimed is a 404 in the API's own
	// shape, not an HTML page from somewhere else.
	mux.HandleFunc("/api/", h(func(w http.ResponseWriter, r *http.Request) error {
		return apierr.NotFound
	}))

	return httpapi.Chain(mux,
		httpapi.Recover(s.log),
		httpapi.RequestID(),
		httpapi.Log(s.log),
		httpapi.NoStore(),
		httpapi.SameOrigin(s.origins),
		httpapi.Sessions(s.sessions, s.users, s.log),
	)
}

// site answers with what the client needs before it knows who anybody is.
func (s *Server) site(w http.ResponseWriter, r *http.Request) error {
	values := s.settings.Values()
	name := values.AppName
	if name == "" {
		name = "Ayakaleaf Pro"
	}
	return httpapi.JSON(w, http.StatusOK, map[string]any{
		"name": name,
		"url":  values.SiteURL,
		// Whether the client should offer to clone a project. Not a secret,
		// and asking here means the editor does not have to guess from a
		// failed request.
		"git": map[string]any{"enabled": s.settings.GitEnabled()},
	})
}

// getSettings answers with everything the admin page needs to draw itself.
func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) error {
	if _, err := httpapi.RequireAdmin(r.Context()); err != nil {
		return err
	}
	return httpapi.JSON(w, http.StatusOK, s.settings.Describe())
}

// putSettings changes settings.
func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) error {
	admin, err := httpapi.RequireAdmin(r.Context())
	if err != nil {
		return err
	}
	var in struct {
		Values map[string]any `json:"values"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	if len(in.Values) == 0 {
		return apierr.BadRequest.WithMessage("Nothing to change.")
	}
	// Anything the catalogue does not name is dropped rather than stored: a
	// settings document is not a place to put arbitrary keys.
	applied := s.settings.Apply(in.Values)
	if len(applied) == 0 {
		return apierr.BadRequest.WithMessage("None of those are settings.")
	}
	if err := s.settings.Set(r.Context(), applied, admin.ID.Hex()); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.JSON(w, http.StatusOK, s.settings.Describe())
}

// EnsureAuthIndexes creates what the password-reset flow relies on.
func (s *Server) EnsureAuthIndexes(ctx context.Context) error {
	return s.auth.EnsureResetIndexes(ctx)
}
