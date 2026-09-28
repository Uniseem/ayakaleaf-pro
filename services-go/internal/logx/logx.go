// Package logx emits bunyan-compatible JSON log records, so that logs from the
// Go services are indistinguishable from those of the Node services they
// replace and existing log tooling keeps working.
package logx

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// Bunyan numeric levels. See https://github.com/trentm/node-bunyan#levels
const (
	bunyanTrace = 10
	bunyanDebug = 20
	bunyanInfo  = 30
	bunyanWarn  = 40
	bunyanError = 50
	bunyanFatal = 60
)

// LevelFatal extends slog's levels with bunyan's fatal.
const LevelFatal = slog.Level(12)

type handler struct {
	mu       *sync.Mutex
	w        io.Writer
	level    slog.Leveler
	name     string
	hostname string
	pid      int
	attrs    []slog.Attr
}

func bunyanLevel(l slog.Level) int {
	switch {
	case l >= LevelFatal:
		return bunyanFatal
	case l >= slog.LevelError:
		return bunyanError
	case l >= slog.LevelWarn:
		return bunyanWarn
	case l >= slog.LevelInfo:
		return bunyanInfo
	case l >= slog.LevelDebug:
		return bunyanDebug
	default:
		return bunyanTrace
	}
}

func (h *handler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level.Level()
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &next
}

// WithGroup is not used by these services; grouping would change the record
// shape away from bunyan's flat layout, so it is a no-op.
func (h *handler) WithGroup(string) slog.Handler { return h }

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	rec := make(map[string]any, 8+len(h.attrs)+r.NumAttrs())
	put := func(a slog.Attr) {
		switch a.Key {
		// Reserved bunyan core fields must not be shadowed by callers.
		case "name", "hostname", "pid", "level", "msg", "time", "v":
			rec["_"+a.Key] = a.Value.Any()
		default:
			rec[a.Key] = a.Value.Any()
		}
	}
	for _, a := range h.attrs {
		put(a)
	}
	r.Attrs(func(a slog.Attr) bool { put(a); return true })

	rec["name"] = h.name
	rec["hostname"] = h.hostname
	rec["pid"] = h.pid
	rec["level"] = bunyanLevel(r.Level)
	rec["msg"] = r.Message
	rec["time"] = r.Time.UTC().Format(time.RFC3339Nano)
	rec["v"] = 0

	buf, err := json.Marshal(rec)
	if err != nil {
		// Never let an unserialisable field silence the log line.
		buf, _ = json.Marshal(map[string]any{
			"name": h.name, "hostname": h.hostname, "pid": h.pid,
			"level": bunyanError, "msg": "log record could not be serialised: " + r.Message,
			"time": r.Time.UTC().Format(time.RFC3339Nano), "v": 0,
		})
	}
	buf = append(buf, '\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err = h.w.Write(buf)
	return err
}

// ParseLevel maps an @overleaf/logger level name onto a slog level.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trace":
		return slog.Level(-8)
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error", "err":
		return slog.LevelError
	case "fatal":
		return LevelFatal
	default:
		return slog.LevelInfo
	}
}

// defaultLevel mirrors @overleaf/logger: LOG_LEVEL wins, else debug outside
// production and fatal under NODE_ENV=test.
func defaultLevel() slog.Level {
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		return ParseLevel(v)
	}
	switch strings.ToLower(os.Getenv("NODE_ENV")) {
	case "production":
		return slog.LevelInfo
	case "test":
		return LevelFatal
	default:
		return slog.LevelDebug
	}
}

// New returns a logger writing bunyan records for the named service.
func New(name string) *slog.Logger {
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}
	return slog.New(&handler{
		mu:       &sync.Mutex{},
		w:        os.Stdout,
		level:    defaultLevel(),
		name:     name,
		hostname: host,
		pid:      os.Getpid(),
	})
}

// Err wraps an error as a bunyan-style `err` field.
func Err(err error) slog.Attr {
	if err == nil {
		return slog.Any("err", nil)
	}
	return slog.Any("err", map[string]any{"message": err.Error()})
}
