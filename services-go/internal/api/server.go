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
	origins  []string
}

// Options is what a Server needs.
type Options struct {
	Log      *slog.Logger
	Users    *users.Store
	Sessions *session.Store
	Settings *settings.Store
	// AllowedOrigins are the addresses a browser may send a state-changing
	// request from. The site's own is enough unless something else embeds it.
	AllowedOrigins []string
}

// New builds the server.
func New(opts Options) *Server {
	return &Server{
		log:      opts.Log,
		users:    opts.Users,
		sessions: opts.Sessions,
		settings: opts.Settings,
		auth:     auth.New(opts.Users, opts.Sessions, opts.Settings),
		origins:  opts.AllowedOrigins,
	}
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

	// Signing up and in.
	mux.HandleFunc("GET /api/auth/status", h(s.auth.Status))
	mux.HandleFunc("POST /api/auth/register", h(s.auth.Register))
	mux.HandleFunc("POST /api/auth/login", h(s.auth.Login))
	mux.HandleFunc("POST /api/auth/logout", h(s.auth.Logout))
	mux.HandleFunc("GET /api/auth/me", h(s.auth.Me))

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

// getSettings answers with every stored setting.
func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) error {
	if _, err := httpapi.RequireAdmin(r.Context()); err != nil {
		return err
	}
	return httpapi.JSON(w, http.StatusOK, map[string]any{"values": s.settings.Raw()})
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
	if err := s.settings.Set(r.Context(), in.Values, admin.ID.Hex()); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.JSON(w, http.StatusOK, map[string]any{"values": s.settings.Raw()})
}
