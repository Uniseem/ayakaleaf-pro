// Package httpx holds the small HTTP helpers shared by the Go services.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// JSON writes v as a JSON body with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status line is already sent; all that is left is to drop the
		// connection so the client sees a truncated body rather than a
		// silently short one.
		panic(http.ErrAbortHandler)
	}
}

// Status writes a bare status code with no body, like express's res.sendStatus
// for the 204 case.
func Status(w http.ResponseWriter, status int) {
	w.WriteHeader(status)
}

// Text writes a plain-text body, like express's res.send(string).
func Text(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// ListenAndServe runs srv until SIGINT/SIGTERM, then drains connections.
// It returns once the server has stopped.
func ListenAndServe(srv *http.Server, log *slog.Logger) error {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	errCh := make(chan error, 1)
	go func() {
		log.Info("HTTP server starting up", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-stop:
		log.Info("shutting down", slog.String("addr", srv.Addr))
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			return err
		}
		return <-errCh
	}
}
