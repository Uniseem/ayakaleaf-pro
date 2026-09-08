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
	"log/slog"
	"net/http"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/auth"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/oauth"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/settings"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/users"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/session"
)

// Server holds everything the routes are served from.
type Server struct {
	log      *slog.Logger
	users    *users.Store
	sessions *session.Store
	settings *settings.Store
	auth     *auth.Service
	projects *projects.Service
	oauth    *oauth.Service
	origins  []string
}

// Options is what a Server needs.
type Options struct {
	Log      *slog.Logger
	Users    *users.Store
	Sessions *session.Store
	Settings *settings.Store
	Projects *projects.Store
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
		auth:     auth.New(opts.Users, opts.Sessions, opts.Settings),
		projects: projects.NewService(opts.Projects),
		origins:  opts.AllowedOrigins,
	}
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

	// Liveness. Outside the session middleware so that a broken Redis does not
	// make the service look dead to whatever is watching it.
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

	// The admin pages.
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
