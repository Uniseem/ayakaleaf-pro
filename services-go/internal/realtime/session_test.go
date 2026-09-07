package realtime

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
)

// sign produces a cookie value the way cookie-parser does, so the verifier is
// checked against the real format rather than against itself.
func sign(value, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(value))
	sig := strings.TrimRight(base64.StdEncoding.EncodeToString(mac.Sum(nil)), "=")
	return "s:" + value + "." + sig
}

func TestUnsignCookie(t *testing.T) {
	store := NewSessionStore(nil, "overleaf.sid", []string{"current-secret"}, "")

	got, ok := store.unsign(sign("abc123", "current-secret"))
	if !ok || got != "abc123" {
		t.Errorf("unsign = %q, %v; want abc123, true", got, ok)
	}

	// A cookie signed with a secret that is no longer configured must not
	// verify, or a leaked old secret would keep working forever.
	if _, ok := store.unsign(sign("abc123", "some-other-secret")); ok {
		t.Error("a cookie signed with an unknown secret should not verify")
	}

	for _, bad := range []string{"", "abc123", "s:abc123", "s:.sig", "s:abc123.wrong"} {
		if _, ok := store.unsign(bad); ok {
			t.Errorf("unsign(%q) should have failed", bad)
		}
	}
}

// Rotation works by accepting the previous secret while new cookies are signed
// with the current one; dropping that would sign every user out.
func TestUnsignAcceptsRotatedSecrets(t *testing.T) {
	store := NewSessionStore(nil, "overleaf.sid",
		[]string{"new-secret", "upcoming-secret", "old-secret"}, "")

	for _, secret := range []string{"new-secret", "upcoming-secret", "old-secret"} {
		if got, ok := store.unsign(sign("sid-1", secret)); !ok || got != "sid-1" {
			t.Errorf("a cookie signed with %q should verify", secret)
		}
	}
	if _, ok := store.unsign(sign("sid-1", "retired-secret")); ok {
		t.Error("a retired secret should stop working once it is removed")
	}
}

func TestGetWithoutCookie(t *testing.T) {
	store := NewSessionStore(nil, "overleaf.sid", []string{"s"}, "")
	r, _ := http.NewRequest(http.MethodGet, "/", nil)
	if _, _, err := store.Get(r.Context(), r); err != ErrMissingSession {
		t.Errorf("a request with no cookie should report a missing session, got %v", err)
	}
}
