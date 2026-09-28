// Package obsv provides Prometheus metrics wire-compatible with
// @overleaf/metrics, so dashboards and the existing acceptance tests that
// scrape /metrics keep working after a service is swapped for its Go port.
package obsv

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// promClient's default percentiles, matched so that existing recording rules
// and dashboards resolve the same quantile series.
var defaultObjectives = map[float64]float64{
	0.01: 0.01, 0.05: 0.01, 0.5: 0.005, 0.9: 0.001, 0.95: 0.001, 0.99: 0.001, 0.999: 0.0001,
}

const (
	summaryMaxAge     = 60 * time.Second
	summaryAgeBuckets = 10
)

var httpLabels = []string{"method", "status_code", "path"}

// Metrics owns a service's registry and the collectors @overleaf/metrics
// exposes by default.
type Metrics struct {
	Registry    *prometheus.Registry
	registerer  prometheus.Registerer
	requestTime *prometheus.SummaryVec
	requestSize *prometheus.SummaryVec
	counters    map[string]*prometheus.CounterVec
}

// New builds a registry carrying the app/host default labels that
// @overleaf/metrics sets in initialize.js.
func New(appName string) *Metrics {
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}
	reg := prometheus.NewRegistry()
	registerer := prometheus.WrapRegistererWith(
		prometheus.Labels{"app": appName, "host": host}, reg,
	)

	// collectDefaultMetrics({ prefix: '' }) equivalent.
	registerer.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	m := &Metrics{
		Registry:   reg,
		registerer: registerer,
		counters:   map[string]*prometheus.CounterVec{},
		requestTime: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Name: "timer_http_request", Help: "timer_http_request",
			Objectives: defaultObjectives, MaxAge: summaryMaxAge, AgeBuckets: summaryAgeBuckets,
		}, httpLabels),
		requestSize: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Name: "http_request_size_bytes", Help: "http_request_size_bytes",
			Objectives: defaultObjectives, MaxAge: summaryMaxAge, AgeBuckets: summaryAgeBuckets,
		}, httpLabels),
	}
	registerer.MustRegister(m.requestTime, m.requestSize)

	// recordProcessStart() in @overleaf/metrics/initialize.js
	m.Inc("process_startup")
	return m
}

// Inc mirrors Metrics.inc(key): a labelless counter created on first use.
func (m *Metrics) Inc(key string) {
	c, ok := m.counters[key]
	if !ok {
		c = prometheus.NewCounterVec(prometheus.CounterOpts{Name: key, Help: key}, nil)
		m.registerer.MustRegister(c)
		m.counters[key] = c
	}
	c.WithLabelValues().Inc()
}

// Handler serves /metrics in the Prometheus text format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}

type routeKey struct{}

// routePath distinguishes "no route matched" from "matched a route whose label
// is the empty string", which is what express's catch-all "*" route produces.
type routePath struct {
	value   string
	matched bool
}

// WithRoutePath tags a request with the metric `path` label. It is the Go
// equivalent of express populating req.route.path: the template, not the
// concrete URL, so label cardinality stays bounded.
//
// The label has to travel back out to the middleware, which already holds the
// request it was called with. Storing it in a derived request would be
// invisible there, so the middleware installs a mutable holder up front and
// this writes through it -- the Go equivalent of express mutating req.route.
func WithRoutePath(r *http.Request, path string) *http.Request {
	if holder, ok := r.Context().Value(routeKey{}).(*routePath); ok {
		holder.value = path
		holder.matched = true
		return r
	}
	// No middleware in front of this handler, as in a unit test that calls it
	// directly: fall back to carrying the value on a derived request.
	return r.WithContext(context.WithValue(r.Context(), routeKey{},
		&routePath{value: path, matched: true}))
}

// RoutePathOf returns the label set by WithRoutePath. The second result is
// false when no route matched, in which case @overleaf/metrics records nothing.
func RoutePathOf(r *http.Request) (string, bool) {
	if holder, ok := r.Context().Value(routeKey{}).(*routePath); ok {
		return holder.value, holder.matched
	}
	return "", false
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// HTTPMiddleware records timer_http_request and logs one line per request,
// matching @overleaf/metrics' http.monitor().
func (m *Metrics) HTTPMiddleware(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}

		// Installed before routing so the matched handler can write its label
		// back here; see WithRoutePath.
		holder := &routePath{}
		r = r.WithContext(context.WithValue(r.Context(), routeKey{}, holder))
		next.ServeHTTP(rec, r)

		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		elapsed := time.Since(start)

		// getRoutePath() returns null for unmatched routes, and
		// @overleaf/metrics then records nothing.
		if routePath, matched := RoutePathOf(r); matched {
			labels := prometheus.Labels{
				"method": r.Method, "status_code": strconv.Itoa(rec.status), "path": routePath,
			}
			m.requestTime.With(labels).Observe(float64(elapsed.Milliseconds()))
			if n, err := strconv.Atoi(r.Header.Get("Content-Length")); err == nil && n > 0 {
				m.requestSize.With(labels).Observe(float64(n))
			}
		}
		log.Debug(r.Method+" "+r.URL.RequestURI(),
			slog.String("method", r.Method),
			slog.Int("statusCode", rec.status),
			slog.Int64("responseTimeMs", elapsed.Milliseconds()),
		)
	})
}
