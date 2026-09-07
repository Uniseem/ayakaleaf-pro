package proxy

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/obsv"
)

// newTestServer builds a proxy whose allow-list pattern lets it reach the
// loopback test servers, which the SSRF checks would otherwise refuse.
func newTestServer(t *testing.T, allow string) http.Handler {
	t.Helper()
	cfg := Config{
		MaxRedirects:  5,
		FetchTimeout:  5 * time.Second,
		UserAgent:     "test-agent",
		MaxUploadSize: 1024,
	}
	if allow != "" {
		cfg.AllowedResources = regexp.MustCompile(allow)
	}
	log := slog.New(slog.DiscardHandler)
	return NewServer(cfg, log, obsv.New("linked-url-proxy-test")).Handler()
}

func get(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	path := "/"
	if target != "" {
		path = "/?url=" + url.QueryEscape(target)
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestMissingURLParameter(t *testing.T) {
	rec := get(t, newTestServer(t, ""), "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if body := rec.Body.String(); body != "Missing ?url parameter" {
		t.Errorf("body = %q, want %q", body, "Missing ?url parameter")
	}
}

func TestRejectsNonHTTPSchemes(t *testing.T) {
	h := newTestServer(t, "")
	for _, target := range []string{
		"file:///etc/passwd",
		"ftp://example.com/x",
		"gopher://example.com/",
	} {
		rec := get(t, h, target)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", target, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "protocol is not allowed") {
			t.Errorf("%s: body = %q, want a protocol error", target, rec.Body.String())
		}
	}
}

func TestRejectsControlCharacterSchemeSmuggling(t *testing.T) {
	// "java\nscript:" parses as a relative reference once the control
	// character is stripped, so it must not be fetched.
	rec := get(t, newTestServer(t, ""), "java\nscript:alert(1)")
	if rec.Code < 400 {
		t.Errorf("status = %d, want a client error", rec.Code)
	}
}

func TestBlocksLoopbackTarget(t *testing.T) {
	// No allow-list here, so the loopback address must be refused.
	rec := get(t, newTestServer(t, ""), "http://127.0.0.1:1/secret")
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Blocked IP address") {
		t.Errorf("body = %q, want a blocked-address error", rec.Body.String())
	}
}

func TestBlocksCloudMetadataEndpoint(t *testing.T) {
	rec := get(t, newTestServer(t, ""), "http://169.254.169.254/latest/meta-data/")
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestProxiesAllowedResource(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "test-agent" {
			t.Errorf("User-Agent = %q, want test-agent", got)
		}
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte("a,b,c\n"))
	}))
	defer upstream.Close()

	rec := get(t, newTestServer(t, "^http://127\\.0\\.0\\.1:"), upstream.URL+"/data.csv")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/csv" {
		t.Errorf("Content-Type = %q, want text/csv", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if body := rec.Body.String(); body != "a,b,c\n" {
		t.Errorf("body = %q, want the upstream body", body)
	}
}

func TestRejectsOversizedContentLength(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "99999")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, 99999))
	}))
	defer upstream.Close()

	rec := get(t, newTestServer(t, "^http://127\\.0\\.0\\.1:"), upstream.URL+"/big.bin")
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "file too large") {
		t.Errorf("body = %q, want a size error", rec.Body.String())
	}
}

func TestFollowsRedirectsUpToTheLimit(t *testing.T) {
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hop := strings.TrimPrefix(r.URL.Path, "/hop/")
		if hop == "done" {
			_, _ = w.Write([]byte("arrived"))
			return
		}
		n := 0
		_, _ = fmt.Sscanf(hop, "%d", &n)
		if n >= 3 {
			http.Redirect(w, r, upstream.URL+"/hop/done", http.StatusFound)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("%s/hop/%d", upstream.URL, n+1), http.StatusFound)
	}))
	defer upstream.Close()

	rec := get(t, newTestServer(t, "^http://127\\.0\\.0\\.1:"), upstream.URL+"/hop/0")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "arrived" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "arrived")
	}
}

func TestRejectsRedirectLoop(t *testing.T) {
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, upstream.URL+"/loop", http.StatusFound)
	}))
	defer upstream.Close()

	rec := get(t, newTestServer(t, "^http://127\\.0\\.0\\.1:"), upstream.URL+"/loop")
	if rec.Code != 421 {
		t.Errorf("status = %d, want 421", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Too many redirects") {
		t.Errorf("body = %q, want a redirect-limit error", rec.Body.String())
	}
}

func TestRedirectToBlockedAddressIsRefused(t *testing.T) {
	// A redirect is re-validated, so an open redirect cannot be used to reach
	// an address the first hop would have been refused for.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer upstream.Close()

	rec := get(t, newTestServer(t, "^http://127\\.0\\.0\\.1:\\d+/redirect"), upstream.URL+"/redirect")
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (body %q)", rec.Code, rec.Body.String())
	}
}

func TestStatusEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()
	newTestServer(t, "").ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if want := `{"status":"linked-url-proxy is up"}`; string(body) != want {
		t.Errorf("body = %s, want %s", body, want)
	}
}

func TestNormalizePath(t *testing.T) {
	cases := map[string]string{
		"":               "/",
		"/":              "/",
		"/a/b":           "/a/b",
		"/a//b":          "/a/b",
		"/a/./b":         "/a/b",
		"/a/../b":        "/b",
		"/a/b/":          "/a/b/",
		"/../../etc/pwd": "/etc/pwd",
	}
	for in, want := range cases {
		got, ok := normalizePath(in)
		if !ok {
			t.Errorf("normalizePath(%q) rejected", in)
			continue
		}
		if got != want {
			t.Errorf("normalizePath(%q) = %q, want %q", in, got, want)
		}
	}
}
