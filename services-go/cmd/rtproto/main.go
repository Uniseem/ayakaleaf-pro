// Command rtproto is a protocol prototype for the real-time port.
//
// It answers the socket.io 0.9 handshake, upgrades to websocket, keeps
// heartbeats alive, and round-trips the events the editor actually sends. It
// exists to settle one question before any of real-time's 3,549 lines are
// ported: does the forked client Overleaf ships talk to a Go server at all?
package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/config"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/logx"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/socketio"
)

type handler struct{ log *slog.Logger }

func (h *handler) OnConnect(c *socketio.Conn) error {
	h.log.Info("client connected", slog.String("id", c.ID))
	// The editor waits for this before it will send anything.
	return c.Emit("connectionAccepted", nil, c.ID)
}

func (h *handler) OnEvent(c *socketio.Conn, ev socketio.Event) {
	name, args := ev.Name, ev.Args
	h.log.Info("event", slog.String("id", c.ID), slog.String("name", name),
		slog.Int("args", len(args)))

	switch name {
	case "joinProject":
		// Shape only: the real service looks the project up and authorises it.
		_ = c.Emit("joinProjectResponse", nil, map[string]any{
			"project":        map[string]any{"_id": "prototype", "name": "prototype"},
			"privilegeLevel": "owner",
			"connectedUsers": []any{},
		})
	case "joinDoc":
		var docID string
		if len(args) > 0 {
			_ = json.Unmarshal(args[0], &docID)
		}
		// docLines, version, ranges, ops -- the argument order the client's
		// callback expects.
		_ = c.Emit("joinDocResponse", docID,
			[]string{`\documentclass{article}`, `\begin{document}`, "hi", `\end{document}`},
			1, map[string]any{}, []any{})
	case "applyOtUpdate":
		_ = c.Emit("otUpdateApplied", map[string]any{"v": 1})
	case "leaveDoc":
		_ = c.Emit("leaveDocResponse", nil)
	}
}

func (h *handler) OnDisconnect(c *socketio.Conn) {
	h.log.Info("client disconnected", slog.String("id", c.ID))
}

func main() {
	log := logx.New("rtproto")
	h := &handler{log: log}
	srv := socketio.NewServer(h, log)
	// RTPROTO_TRANSPORTS restricts what the handshake offers, so the polling
	// fallback can be exercised without a proxy that blocks upgrades.
	if t := os.Getenv("RTPROTO_TRANSPORTS"); t != "" {
		srv.OfferTransports = strings.Split(t, ",")
	}

	mux := http.NewServeMux()
	mux.Handle("/socket.io/1/", srv)

	// The client itself is served by this service, exactly as app.js does with
	// the file from the socket.io-client package.
	if clientJS := os.Getenv("SOCKET_IO_CLIENT_JS"); clientJS != "" {
		mux.HandleFunc("/socket.io/socket.io.js", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Clean(clientJS))
		})
	}
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("rtproto is up"))
	})

	addr := fmt.Sprintf("%s:%d", config.ListenAddress(), config.EnvInt("RTPROTO_PORT", 3026))
	log.Info("prototype listening", slog.String("addr", addr))
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 30 * time.Second}
	if err := server.ListenAndServe(); err != nil {
		log.Error("stopped", logx.Err(err))
		os.Exit(1)
	}
}
