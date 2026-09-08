// Package httpapi is the plumbing every handler in the API shares: how a
// handler is shaped, how a body is read, how a response is written, and the
// middleware that runs around all of them.
//
// A handler here returns an error instead of writing one. That is the whole
// reason this package exists: it makes "answer with an error" a return
// statement, so no handler can forget to stop after writing one -- which is
// the most common bug in the code this replaces.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/users"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/session"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Handler is what every route is written as.
type Handler func(w http.ResponseWriter, r *http.Request) error

// Wrap turns a Handler into a standard http.HandlerFunc.
func Wrap(log *slog.Logger, h Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			apierr.Write(w, log, err)
		}
	}
}

// MaxBodyBytes is the largest JSON body any handler will read. Uploads have
// their own limit and do not come through here.
const MaxBodyBytes = 1 << 20

// Decode reads a JSON body into v.
//
// Unknown fields are refused rather than ignored: a client sending a field
// this version does not know about is either out of date or wrong, and finding
// that out at the boundary is cheaper than finding it out from behaviour.
func Decode(r *http.Request, v any) error {
	if r.Body == nil {
		return apierr.BadRequest.WithMessage("A body is required.")
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, MaxBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		return apierr.BadRequest.WithCause(err).WithMessage("That request body could not be read.")
	}
	return nil
}

// JSON writes a successful response.
func JSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return nil
	}
	return json.NewEncoder(w).Encode(v)
}

// NoContent answers a request that has nothing to say.
func NoContent(w http.ResponseWriter) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// --- the request's identity -------------------------------------------------

type contextKey int

const (
	sessionKey contextKey = iota
	sessionIDKey
	userKey
	requestIDKey
)

// WithSession puts a session on a request's context.
func WithSession(ctx context.Context, sess *session.Session, id string) context.Context {
	ctx = context.WithValue(ctx, sessionKey, sess)
	return context.WithValue(ctx, sessionIDKey, id)
}

// SessionFrom reads the session, or nil when the visitor is anonymous.
func SessionFrom(ctx context.Context) *session.Session {
	sess, _ := ctx.Value(sessionKey).(*session.Session)
	return sess
}

// SessionIDFrom reads the session id.
func SessionIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(sessionIDKey).(string)
	return id
}

// WithUser puts the signed-in user on a request's context.
func WithUser(ctx context.Context, user *users.User) context.Context {
	return context.WithValue(ctx, userKey, user)
}

// UserFrom reads the signed-in user, or nil.
func UserFrom(ctx context.Context) *users.User {
	user, _ := ctx.Value(userKey).(*users.User)
	return user
}

// RequireUser reads the signed-in user or refuses the request.
func RequireUser(ctx context.Context) (*users.User, error) {
	user := UserFrom(ctx)
	if user == nil {
		return nil, apierr.Unauthorised
	}
	return user, nil
}

// RequireAdmin reads the signed-in administrator or refuses the request.
func RequireAdmin(ctx context.Context) (*users.User, error) {
	user, err := RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	if !user.IsAdmin {
		return nil, apierr.Forbidden
	}
	return user, nil
}

// RequestIDFrom reads the id given to this request.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// --- middleware -------------------------------------------------------------

// Middleware wraps a handler.
type Middleware func(http.Handler) http.Handler

// Chain applies middleware in the order given, so the first listed is the
// outermost and sees the request first.
func Chain(h http.Handler, middleware ...Middleware) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}

// Recover turns a panic into a 500 rather than a dropped connection.
func Recover(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Error("handler panicked",
						slog.Any("panic", recovered),
						slog.String("path", r.URL.Path),
						slog.String("stack", string(debug.Stack())),
					)
					apierr.Write(w, log, apierr.Internal)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// RequestID gives every request an identifier, taken from the proxy when it
// set one so that a trace crosses the boundary.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-Id")
			if id == "" {
				id = bson.NewObjectID().Hex()
			}
			w.Header().Set("X-Request-Id", id)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
		})
	}
}

// Log records how each request went.
func Log(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r)
			log.Info("request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", recorder.status),
				slog.Duration("took", time.Since(started)),
				slog.String("requestId", RequestIDFrom(r.Context())),
			)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if !r.wrote {
		r.status = status
		r.wrote = true
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wrote = true
	return r.ResponseWriter.Write(b)
}

// Sessions resolves the session and the signed-in user for every request.
//
// It never refuses: an anonymous visitor is a request with no user on it, and
// deciding whether that is allowed belongs to the handler, not here.
func Sessions(store *session.Store, userStore *users.Store, log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			sess, id, err := store.Get(ctx, r)
			if err != nil && !errors.Is(err, session.ErrMissing) {
				log.Warn("could not read the session", slog.Any("err", err))
			}
			if sess != nil {
				ctx = WithSession(ctx, sess, id)
				if identity := sess.Identity(); identity != nil {
					if oid, err := bson.ObjectIDFromHex(identity.ID); err == nil {
						if user, err := userStore.ByID(ctx, oid); err == nil {
							ctx = WithUser(ctx, user)
						} else if !errors.Is(err, users.ErrNotFound) {
							log.Warn("could not read the session user", slog.Any("err", err))
						}
					}
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SameOrigin refuses a state-changing request that came from another site.
//
// This is the whole of the CSRF defence. A token in a header would add
// nothing: a site that can read a token can also read the response, and one
// that cannot read the response cannot forge these headers either. What it
// relies on is that Origin and Sec-Fetch-Site cannot be set by a page.
func SameOrigin(allowedOrigins []string) Middleware {
	safe := map[string]bool{http.MethodGet: true, http.MethodHead: true, http.MethodOptions: true}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if safe[r.Method] {
				next.ServeHTTP(w, r)
				return
			}
			switch r.Header.Get("Sec-Fetch-Site") {
			case "same-origin", "none":
				next.ServeHTTP(w, r)
				return
			case "cross-site", "same-site":
				apierr.Write(w, nil, apierr.Forbidden.WithMessage("That request came from another site."))
				return
			}
			// No Sec-Fetch-Site. An older browser, a client that is not a
			// browser -- or, and this is the common one, a browser on a
			// plain-HTTP address, which does not send these headers outside a
			// secure context. Fall back to Origin.
			origin := r.Header.Get("Origin")
			if origin == "" {
				// No Origin on a same-origin form post from an old browser.
				next.ServeHTTP(w, r)
				return
			}
			// Against the address this request actually arrived on, first.
			// A request whose Origin is the host it was sent to is same-origin
			// by definition, and needs nobody to have configured anything --
			// which matters most on a site nobody has set up yet, where the
			// first thing anybody does is a state-changing request.
			if sameHost(origin, r) {
				next.ServeHTTP(w, r)
				return
			}
			// And then against anything an operator added, for a deployment
			// whose public address is not the one requests arrive on.
			for _, allowed := range allowedOrigins {
				if allowed != "" && strings.EqualFold(origin, allowed) {
					next.ServeHTTP(w, r)
					return
				}
			}
			apierr.Write(w, nil, apierr.Forbidden.WithMessage("That request came from another site."))
		})
	}
}

// sameHost says whether an Origin names the address this request arrived on.
//
// The host is what is compared, not the scheme: what a cross-site request
// cannot do is claim this host, and a deployment behind a proxy that terminates
// TLS sees http on this side and https on the browser's.
func sameHost(origin string, r *http.Request) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	host := r.Host
	// Behind the deployment's own proxy, which is where every request comes
	// from, this is the address the browser used.
	if forwarded := r.Header.Get("X-Forwarded-Host"); forwarded != "" {
		host = forwarded
	}
	return strings.EqualFold(parsed.Host, host)
}

// NoStore marks a response as one a cache must not keep. Every API response
// carries it: they are all about one person.
func NoStore() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			next.ServeHTTP(w, r)
		})
	}
}
