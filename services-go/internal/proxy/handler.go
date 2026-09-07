package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/obsv"
)

// Config mirrors services/linked-url-proxy/config/settings.defaults.cjs.
type Config struct {
	MaxRedirects     int
	FetchTimeout     time.Duration
	BlockedNetworks  []netip.Prefix
	AllowedResources *regexp.Regexp
	UserAgent        string
	MaxUploadSize    int64
}

// Server proxies user-supplied URLs, refusing those that resolve to addresses
// the instance must not reach.
type Server struct {
	cfg     Config
	log     *slog.Logger
	metrics *obsv.Metrics
}

// NewServer builds the linked-url-proxy HTTP server.
func NewServer(cfg Config, log *slog.Logger, metrics *obsv.Metrics) *Server {
	return &Server{cfg: cfg, log: log, metrics: metrics}
}

// Handler returns the fully wired proxy router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /{$}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.proxy(w, obsv.WithRoutePath(r, ""))
	}))
	mux.Handle("GET /status", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = obsv.WithRoutePath(r, "status")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"linked-url-proxy is up"}`))
	}))
	mux.Handle("GET /metrics", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.metrics.Handler().ServeHTTP(w, obsv.WithRoutePath(r, "metrics"))
	}))
	return s.metrics.HTTPMiddleware(s.log, mux)
}

// statusError carries the HTTP status the Node controller attaches as
// err.info.status.
type statusError struct {
	status int
	msg    string
}

func (e *statusError) Error() string { return e.msg }

func errf(status int, format string, args ...any) *statusError {
	return &statusError{status: status, msg: fmt.Sprintf(format, args...)}
}

func statusOf(err error) int {
	var se *statusError
	if errors.As(err, &se) {
		return se.status
	}
	return http.StatusInternalServerError
}

func (s *Server) proxy(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("url")
	if target == "" {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Missing ?url parameter"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.FetchTimeout)
	defer cancel()

	res, transport, err := s.fetch(ctx, target, 0)
	if err != nil {
		s.log.Info("linked-url-proxy request failed",
			slog.String("linkedUrl", err.Error()),
			slog.Int("status", statusOf(err)),
			slog.String("url", r.URL.RequestURI()),
		)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(statusOf(err))
		_, _ = w.Write([]byte("Error: " + err.Error()))
		return
	}
	defer func() {
		_ = res.Body.Close()
		transport.CloseIdleConnections()
	}()

	contentType := res.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(res.StatusCode)

	if _, err := io.Copy(w, res.Body); err != nil {
		// Headers are already on the wire, so the only signal left is to drop
		// the connection, which is what destroying the response does in Node.
		s.log.Info("linked-url-proxy stream failed",
			slog.String("err", err.Error()), slog.String("url", r.URL.RequestURI()))
	}
}

// fetch validates a URL and performs one request, following redirects itself
// so that every hop is re-validated.
//
// The returned transport must be closed by the caller once the body is read.
func (s *Server) fetch(ctx context.Context, rawURL string, redirectCount int) (*http.Response, *http.Transport, error) {
	if redirectCount > s.cfg.MaxRedirects {
		return nil, nil, errf(421, "Too many redirects")
	}

	sanitized := sanitizeURL(rawURL)
	if sanitized == "" {
		return nil, nil, errf(http.StatusBadRequest, "Invalid or unsafe URL: %s", rawURL)
	}
	parsed, err := url.Parse(sanitized)
	if err != nil {
		return nil, nil, errf(http.StatusBadRequest, "Invalid or unsafe URL: %s", rawURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, nil, errf(http.StatusBadRequest, "%s: protocol is not allowed", parsed.Scheme)
	}
	normalizedPath, ok := normalizePath(parsed.Path)
	if !ok {
		return nil, nil, errf(http.StatusBadRequest, "Invalid or unsafe URL path: %s", parsed.Path)
	}
	parsed.Path = normalizedPath

	hostname := parsed.Hostname()
	if hostname == "" {
		return nil, nil, errf(http.StatusBadRequest, "Invalid or unsafe URL: %s", rawURL)
	}
	normalizedURL := parsed.String()

	pinned, err := s.checkURLAccess(ctx, hostname, normalizedURL)
	if err != nil {
		return nil, nil, err
	}

	// Pin the connection to the address that was just validated. Resolving
	// again inside the dialer would reopen the DNS-rebinding window between
	// the check and the connect.
	port := parsed.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[parsed.Scheme]
	}
	pinnedAddr := net.JoinHostPort(pinned.String(), port)

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, pinnedAddr)
		},
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: s.cfg.FetchTimeout,
	}
	client := &http.Client{
		Transport: transport,
		// Redirects are handled here, one validated hop at a time.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, normalizedURL, nil)
	if err != nil {
		return nil, nil, errf(422, "%s", err.Error())
	}
	req.Header.Set("User-Agent", s.cfg.UserAgent)

	res, err := client.Do(req)
	if err != nil {
		transport.CloseIdleConnections()
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, nil, errf(http.StatusRequestTimeout, "request timeout")
		}
		return nil, nil, errf(422, "%s", err.Error())
	}

	if res.StatusCode >= 300 && res.StatusCode < 400 {
		location := res.Header.Get("Location")
		_ = res.Body.Close()
		transport.CloseIdleConnections()
		if location == "" {
			return nil, nil, errf(421, "Redirect response missing Location header")
		}
		next, err := parsed.Parse(location)
		if err != nil {
			return nil, nil, errf(421, "Redirect response has an invalid Location header")
		}
		return s.fetch(ctx, next.String(), redirectCount+1)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		_ = res.Body.Close()
		transport.CloseIdleConnections()
		return nil, nil, errf(res.StatusCode, "upstream responded with status %d", res.StatusCode)
	}

	if raw := res.Header.Get("Content-Length"); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n > s.cfg.MaxUploadSize {
			_ = res.Body.Close()
			transport.CloseIdleConnections()
			return nil, nil, errf(http.StatusRequestEntityTooLarge, "file too large")
		}
	}
	return res, transport, nil
}

// checkURLAccess resolves a hostname and refuses any answer that points at an
// address the instance must not reach. It returns the address to connect to.
func (s *Server) checkURLAccess(ctx context.Context, hostname, targetURL string) (netip.Addr, error) {
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", hostname)
	if err != nil || len(addrs) == 0 {
		return netip.Addr{}, errf(421, "DNS lookup failed for %s", hostname)
	}
	// An explicitly allowed resource skips the address checks, which is how
	// an operator whitelists an internal host on purpose.
	if s.cfg.AllowedResources != nil && s.cfg.AllowedResources.MatchString(targetURL) {
		return addrs[0].Unmap(), nil
	}
	for _, addr := range addrs {
		if IsBlocked(addr, s.cfg.BlockedNetworks) {
			return netip.Addr{}, errf(http.StatusForbidden, "Blocked IP address: %s", addr.Unmap().String())
		}
	}
	return addrs[0].Unmap(), nil
}

// sanitizeURL strips the ASCII control characters and surrounding whitespace
// that are used to smuggle a scheme past a naive parser, e.g. "java\nscript:".
// The http/https check in fetch is the real gate; this removes the obfuscation
// that would otherwise reach it.
func sanitizeURL(raw string) string {
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		if r <= 0x1f || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// normalizePath resolves "." and ".." segments and collapses repeated
// separators, rejecting a path that escapes the root.
func normalizePath(p string) (string, bool) {
	if p == "" {
		return "/", true
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	cleaned := path.Clean(p)
	if strings.HasPrefix(cleaned, "..") {
		return "", false
	}
	// path.Clean drops a meaningful trailing slash; put it back.
	if strings.HasSuffix(p, "/") && !strings.HasSuffix(cleaned, "/") {
		cleaned += "/"
	}
	return cleaned, true
}
