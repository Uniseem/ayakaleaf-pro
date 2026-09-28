package realtime

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/redis/go-redis/v9"
)

// ErrMissingSession is what the Node service calls MissingSessionError: no
// usable cookie was presented. The client is told to reauthenticate.
var ErrMissingSession = errors.New("could not look up session by key")

// User is the identity carried in a session.
type User struct {
	ID        string `json:"_id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	// AnonymousAccessToken authorises a link-shared project for a user who is
	// not logged in.
	AnonymousAccessToken string `json:"-"`
}

// Session is the part of an Overleaf session this service reads.
type Session struct {
	Passport *struct {
		User *User `json:"user"`
	} `json:"passport"`
	User *User `json:"user"`
	// AnonTokenAccess maps a project id to the link-sharing token the visitor
	// followed to reach it.
	AnonTokenAccess map[string]string `json:"anonTokenAccess"`
}

// Identity returns the logged-in user, or nil for an anonymous visitor.
func (s *Session) Identity() *User {
	if s == nil {
		return nil
	}
	if s.Passport != nil && s.Passport.User != nil {
		return s.Passport.User
	}
	return s.User
}

// SessionStore reads Overleaf sessions out of Redis.
//
// The editor authenticates a socket with the same signed cookie it uses for
// HTTP, so this has to verify the signature exactly as cookie-parser does and
// read the session under the same key connect-redis writes.
type SessionStore struct {
	redis      *redis.Client
	cookieName string
	// secrets are tried in order, which is what makes a secret rotation
	// possible without logging everyone out.
	secrets []string
	prefix  string
}

// NewSessionStore builds a SessionStore. An empty prefix uses the
// connect-redis default of "sess:".
func NewSessionStore(client *redis.Client, cookieName string, secrets []string, prefix string) *SessionStore {
	if prefix == "" {
		prefix = "sess:"
	}
	return &SessionStore{redis: client, cookieName: cookieName, secrets: secrets, prefix: prefix}
}

// Get resolves the session behind a request's cookies.
func (s *SessionStore) Get(ctx context.Context, r *http.Request) (*Session, string, error) {
	cookie, err := r.Cookie(s.cookieName)
	if err != nil {
		return nil, "", ErrMissingSession
	}
	sessionID, ok := s.unsign(decodeCookieValue(cookie.Value))
	if !ok {
		// A cookie whose signature does not verify is treated as absent, not
		// as an error: it is what a client holds after a secret rotation, and
		// it should be told to reauthenticate.
		return nil, "", ErrMissingSession
	}

	raw, err := s.redis.Get(ctx, s.prefix+sessionID).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, "", ErrMissingSession
	}
	if err != nil {
		return nil, "", err
	}

	var session Session
	if err := json.Unmarshal(raw, &session); err != nil {
		return nil, "", err
	}
	return &session, sessionID, nil
}

// decodeCookieValue undoes the percent-encoding express applies when it sets a
// cookie.
//
// res.cookie() runs the value through encodeURIComponent, so a browser sends
// "s%3A<id>.<sig>" and not "s:<id>.<sig>". cookie-parser decodes it before
// looking at it; Go hands back the raw value, so the decoding has to happen
// here or no browser cookie ever matches.
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

// unsign verifies a cookie signed by cookie-parser.
//
// The format is "s:<value>.<signature>", where the signature is the base64
// HMAC-SHA256 of the value with the "=" padding stripped. Every configured
// secret is tried, so a rotation does not invalidate live sessions.
func (s *SessionStore) unsign(raw string) (string, bool) {
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
