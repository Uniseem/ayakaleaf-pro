// Package session reads and writes the sessions the whole site runs on.
//
// The read half already existed inside real-time, which authenticates sockets
// with the same cookie the browser sends to the API. This adds the write half,
// because the API now issues sessions rather than proxying to a Node service
// that did.
//
// The wire format is kept exactly as express-session and cookie-parser wrote
// it: a signed "s:<id>.<signature>" cookie, and JSON under "sess:<id>" in
// Redis. Nothing forces that choice any more, but changing it would sign
// everybody out on the deploy that changed it, and would need every service
// that reads a session to move in the same release. The format is not the
// interesting part; the boundary is.
package session

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrMissing is returned when a request carries no usable session. The caller
// decides whether that is an error or simply an anonymous visitor.
var ErrMissing = errors.New("no session")

// DefaultPrefix is where connect-redis keeps sessions.
const DefaultPrefix = "sess:"

// User is the identity carried in a session.
//
// It is a copy of the fields a page needs, not a reference to the user
// document: reading a session must not cost a database round trip, which is
// why a change to a name only shows up on the next sign-in.
type User struct {
	ID        string `json:"_id"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Email     string `json:"email"`
	IsAdmin   bool   `json:"isAdmin,omitempty"`

	// AnonymousAccessToken authorises a link-shared project for a visitor who
	// is not signed in. Never persisted with the user.
	AnonymousAccessToken string `json:"-"`
}

// Cookie is the part of the session document express-session owns. It is
// written back verbatim so that anything else reading the session sees what it
// expects.
type Cookie struct {
	OriginalMaxAge int64  `json:"originalMaxAge"`
	Expires        string `json:"expires"`
	HTTPOnly       bool   `json:"httpOnly"`
	Path           string `json:"path"`
	Secure         bool   `json:"secure"`
	SameSite       string `json:"sameSite,omitempty"`
}

// Passport is where the sign-in state lives, under the name passport gave it.
type Passport struct {
	User *User `json:"user,omitempty"`
}

// Session is a whole session document.
type Session struct {
	Cookie   *Cookie   `json:"cookie,omitempty"`
	Passport *Passport `json:"passport,omitempty"`

	// User is the older shape, still read because sessions written before
	// passport are still live.
	User *User `json:"user,omitempty"`

	// AnonTokenAccess maps a project id to the link-sharing token the visitor
	// followed to reach it.
	AnonTokenAccess map[string]string `json:"anonTokenAccess,omitempty"`

	// CSRFSecret is the per-session secret the tokens are derived from.
	CSRFSecret string `json:"csrfSecret,omitempty"`

	// Redirect is where to send somebody after they sign in.
	Redirect string `json:"postLoginRedirect,omitempty"`

	// Extra keeps anything this service does not model, so writing a session
	// back never drops a field somebody else put there.
	Extra map[string]json.RawMessage `json:"-"`
}

// Identity returns the signed-in user, or nil for an anonymous visitor.
func (s *Session) Identity() *User {
	if s == nil {
		return nil
	}
	if s.Passport != nil && s.Passport.User != nil {
		return s.Passport.User
	}
	return s.User
}

// Store reads and writes sessions in Redis.
type Store struct {
	redis      *redis.Client
	cookieName string
	// secrets are tried in order when verifying and the first is used when
	// signing, which is what makes a rotation possible without signing
	// everybody out.
	secrets []string
	prefix  string
	ttl     time.Duration
	secure  bool
}

// Options configures a Store.
type Options struct {
	CookieName string
	Secrets    []string
	Prefix     string
	// TTL is how long a session lives, both in Redis and in the cookie.
	TTL time.Duration
	// Secure marks the cookie so it is only sent over HTTPS.
	Secure bool
}

// NewStore builds a Store.
func NewStore(client *redis.Client, opts Options) *Store {
	if opts.Prefix == "" {
		opts.Prefix = DefaultPrefix
	}
	if opts.CookieName == "" {
		opts.CookieName = "overleaf.sid"
	}
	if opts.TTL <= 0 {
		opts.TTL = 5 * 24 * time.Hour
	}
	return &Store{
		redis:      client,
		cookieName: opts.CookieName,
		secrets:    opts.Secrets,
		prefix:     opts.Prefix,
		ttl:        opts.TTL,
		secure:     opts.Secure,
	}
}

// CookieName is the name of the cookie this store reads and writes.
func (s *Store) CookieName() string { return s.cookieName }

// Get resolves the session behind a request's cookies.
//
// A cookie whose signature does not verify is treated as absent rather than as
// an error: that is what a client holds after a secret rotation, and it should
// be told to sign in again.
func (s *Store) Get(ctx context.Context, r *http.Request) (*Session, string, error) {
	cookie, err := r.Cookie(s.cookieName)
	if err != nil {
		return nil, "", ErrMissing
	}
	id, ok := s.unsign(decodeCookieValue(cookie.Value))
	if !ok {
		return nil, "", ErrMissing
	}
	return s.Load(ctx, id)
}

// Load reads a session by its id.
func (s *Store) Load(ctx context.Context, id string) (*Session, string, error) {
	raw, err := s.redis.Get(ctx, s.prefix+id).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, "", ErrMissing
	}
	if err != nil {
		return nil, "", err
	}

	var sess Session
	if err := json.Unmarshal(raw, &sess); err != nil {
		return nil, "", err
	}
	// Keep whatever this service does not model, so a later Save does not drop
	// a field another service wrote.
	var all map[string]json.RawMessage
	if err := json.Unmarshal(raw, &all); err == nil {
		known := map[string]bool{
			"cookie": true, "passport": true, "user": true,
			"anonTokenAccess": true, "csrfSecret": true, "postLoginRedirect": true,
		}
		for k, v := range all {
			if !known[k] {
				if sess.Extra == nil {
					sess.Extra = map[string]json.RawMessage{}
				}
				sess.Extra[k] = v
			}
		}
	}
	return &sess, id, nil
}

// Save writes a session under an existing id.
func (s *Store) Save(ctx context.Context, id string, sess *Session) error {
	if sess.Cookie == nil {
		sess.Cookie = s.newCookieState()
	}
	raw, err := s.marshal(sess)
	if err != nil {
		return err
	}
	return s.redis.Set(ctx, s.prefix+id, raw, s.ttl).Err()
}

// Create makes a new session, writes it, and sets the cookie on the response.
//
// A fresh id rather than a reused one: signing in on a session an attacker
// planted is session fixation, and the only defence that always works is a new
// identifier at the moment the identity changes.
func (s *Store) Create(ctx context.Context, w http.ResponseWriter, sess *Session) (string, error) {
	id, err := newSessionID()
	if err != nil {
		return "", err
	}
	sess.Cookie = s.newCookieState()
	raw, err := s.marshal(sess)
	if err != nil {
		return "", err
	}
	if err := s.redis.Set(ctx, s.prefix+id, raw, s.ttl).Err(); err != nil {
		return "", err
	}
	s.SetCookie(w, id)
	return id, nil
}

// Destroy removes a session and clears the cookie.
func (s *Store) Destroy(ctx context.Context, w http.ResponseWriter, id string) error {
	if id != "" {
		if err := s.redis.Del(ctx, s.prefix+id).Err(); err != nil {
			return err
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// SetCookie writes the signed session cookie.
func (s *Store) SetCookie(w http.ResponseWriter, id string) {
	http.SetCookie(w, &http.Cookie{
		Name: s.cookieName,
		// Percent-encoded the way express does it, because that is what the
		// other readers undo.
		Value:    url.PathEscape(s.sign(id)),
		Path:     "/",
		MaxAge:   int(s.ttl / time.Second),
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Store) newCookieState() *Cookie {
	return &Cookie{
		OriginalMaxAge: int64(s.ttl / time.Millisecond),
		Expires:        time.Now().UTC().Add(s.ttl).Format("2006-01-02T15:04:05.000Z"),
		HTTPOnly:       true,
		Path:           "/",
		Secure:         s.secure,
		SameSite:       "lax",
	}
}

// marshal writes the session with anything it does not model put back.
func (s *Store) marshal(sess *Session) ([]byte, error) {
	known, err := json.Marshal(sess)
	if err != nil {
		return nil, err
	}
	if len(sess.Extra) == 0 {
		return known, nil
	}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(known, &merged); err != nil {
		return nil, err
	}
	for k, v := range sess.Extra {
		if _, taken := merged[k]; !taken {
			merged[k] = v
		}
	}
	return json.Marshal(merged)
}

// newSessionID produces an id of the shape express-session's uid-safe makes:
// 24 random bytes in url-safe base64 without padding.
func newSessionID() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// sign produces a cookie value cookie-parser accepts.
func (s *Store) sign(value string) string {
	secret := ""
	for _, candidate := range s.secrets {
		if candidate != "" {
			secret = candidate
			break
		}
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(value))
	signature := strings.TrimRight(base64.StdEncoding.EncodeToString(mac.Sum(nil)), "=")
	return "s:" + value + "." + signature
}

// unsign verifies a cookie signed by cookie-parser.
//
// The format is "s:<value>.<signature>", where the signature is the base64
// HMAC-SHA256 of the value with the "=" padding stripped. Every configured
// secret is tried, so a rotation does not invalidate live sessions.
func (s *Store) unsign(raw string) (string, bool) {
	if !strings.HasPrefix(raw, "s:") {
		return "", false
	}
	body := raw[2:]
	dot := strings.LastIndex(body, ".")
	if dot < 0 {
		return "", false
	}
	value, signature := body[:dot], body[dot+1:]

	for _, secret := range s.secrets {
		if secret == "" {
			continue
		}
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(value))
		expected := strings.TrimRight(base64.StdEncoding.EncodeToString(mac.Sum(nil)), "=")
		// Constant time, so the comparison cannot leak the expected signature
		// one byte at a time.
		if subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) == 1 {
			return value, true
		}
	}
	return "", false
}

// decodeCookieValue undoes the percent-encoding express applies when it sets a
// cookie.
//
// PathUnescape rather than QueryUnescape: the latter turns "+" into a space,
// and "+" is in the base64 alphabet the signature is written in.
func decodeCookieValue(raw string) string {
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		// Not percent-encoded at all, which is what a non-browser client
		// sends.
		return raw
	}
	return decoded
}
