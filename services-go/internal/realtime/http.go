package realtime

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// connectedClientView is what the /clients endpoints report. The acceptance
// tests read it to check internal state, and operators read it to see who is
// on an instance.
type connectedClientView struct {
	ClientID      string   `json:"client_id"`
	ProjectID     string   `json:"project_id,omitempty"`
	UserID        string   `json:"user_id,omitempty"`
	FirstName     string   `json:"first_name,omitempty"`
	LastName      string   `json:"last_name,omitempty"`
	Email         string   `json:"email,omitempty"`
	ConnectedTime string   `json:"connected_time,omitempty"`
	Rooms         []string `json:"rooms"`
}

func (s *Service) clientView(clientID string) (connectedClientView, bool) {
	c := s.io.Client(clientID)
	if c == nil {
		return connectedClientView{}, false
	}
	view := connectedClientView{ClientID: clientID, Rooms: c.Rooms()}
	if cc := s.context(c); cc != nil {
		cc.mu.RLock()
		view.ProjectID = cc.projectID
		if cc.user != nil {
			view.UserID = cc.user.ID
			view.FirstName = cc.user.FirstName
			view.LastName = cc.user.LastName
			view.Email = cc.user.Email
		}
		if !cc.connectedTime.IsZero() {
			view.ConnectedTime = cc.connectedTime.UTC().Format("2006-01-02T15:04:05.000Z")
		}
		cc.mu.RUnlock()
	}
	return view, true
}

// Handler builds the HTTP surface: the socket.io endpoints, the internal API
// web calls, and the health checks the load balancer polls.
//
// monitor wraps the plain HTTP routes only. The socket.io endpoints are left
// bare on purpose: a websocket upgrade hijacks the connection, and a
// middleware that wraps the ResponseWriter takes that ability away.
func (s *Service) Handler(socketIOClientJS string, monitor func(http.Handler) http.Handler) http.Handler {
	mux := http.NewServeMux()

	// The editor loads its socket.io client from this service, exactly as the
	// Node app.js serves it out of the socket.io-client package.
	if socketIOClientJS != "" {
		mux.HandleFunc("GET /socket.io/socket.io.js", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, socketIOClientJS)
		})
	}

	// A 200 here is what keeps the instance in the load balancer.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		if s.shutDownInProgress.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("real-time is open"))
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		if s.shutDownInProgress.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("real-time is alive"))
	})

	mux.HandleFunc("GET /health_check", func(w http.ResponseWriter, r *http.Request) {
		if s.shutDownComplete.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		s.healthCheck(w, r)
	})
	mux.HandleFunc("GET /health_check/redis", s.healthCheck)

	mux.HandleFunc("GET /clients", func(w http.ResponseWriter, r *http.Request) {
		clients := s.io.Clients()
		views := make([]connectedClientView, 0, len(clients))
		for _, c := range clients {
			if view, ok := s.clientView(c.ID); ok {
				views = append(views, view)
			}
		}
		writeJSON(w, views)
	})
	mux.HandleFunc("GET /clients/{client_id}", func(w http.ResponseWriter, r *http.Request) {
		view, ok := s.clientView(r.PathValue("client_id"))
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, view)
	})

	mux.HandleFunc("POST /project/{project_id}/message/{message}", s.sendMessage)
	mux.HandleFunc("GET /project/{projectId}/count-connected-clients", s.countConnectedClients)
	mux.HandleFunc("POST /drain", s.startDrain)
	mux.HandleFunc("POST /client/{client_id}/disconnect", s.disconnectClient)

	mux.HandleFunc("GET /debug/events", func(w http.ResponseWriter, r *http.Request) {
		// Kept as a route so tooling that pokes it does not 404; this port
		// logs every event at debug level instead of counting down.
		_, _ = w.Write([]byte("debug events are logged at debug level"))
	})

	root := http.NewServeMux()
	root.Handle("/socket.io/1/", s.io)
	if monitor != nil {
		root.Handle("/", monitor(mux))
	} else {
		root.Handle("/", mux)
	}
	return root
}

func (s *Service) healthCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.realtimeRedis.Ping(ctx).Err(); err != nil {
		s.log.Error("failed redis health check", slog.String("err", err.Error()))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if s.health.IsFailing() {
		s.log.Error("failed pubsub health check", slog.Any("pubSubErrors", s.health.Status()))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// sendMessage is how web pushes an event to a project: a rename, a new file, a
// changed permission.
func (s *Service) sendMessage(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")
	message := r.PathValue("message")
	s.log.Debug("sending message", slog.String("message", message))

	body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ctx := r.Context()

	// An array body is a batch: each element is sent as its own event, which
	// is how web pushes several entity updates in one call.
	var batch []json.RawMessage
	if json.Unmarshal(body, &batch) == nil && strings.HasPrefix(strings.TrimSpace(string(body)), "[") {
		for _, payload := range batch {
			s.EmitToRoom(ctx, projectID, message, payload)
		}
	} else {
		s.EmitToRoom(ctx, projectID, message, json.RawMessage(body))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) countConnectedClients(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")
	count, err := s.users.CountConnectedClients(r.Context(), projectID)
	if err != nil {
		s.log.Error("count connected clients failed", slog.String("project", projectID),
			slog.String("err", err.Error()))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"nConnectedClients": count})
}

func (s *Service) startDrain(w http.ResponseWriter, r *http.Request) {
	rate := 4.0
	if v := r.URL.Query().Get("rate"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			rate = parsed
		} else {
			rate = 0
		}
	}
	s.log.Info("setting client drain rate", slog.Float64("rate", rate))
	s.drain.StartDrain(rate, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) disconnectClient(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("client_id")
	c := s.io.Client(clientID)
	if c == nil {
		s.log.Debug("api: client already disconnected", slog.String("client", clientID))
		w.WriteHeader(http.StatusNotFound)
		return
	}
	s.log.Info("api: requesting client disconnect", slog.String("client", clientID))
	c.Close()
	// The caller waits for the disconnect to have taken effect, which is what
	// makes this usable from a test.
	select {
	case <-c.Done():
	case <-time.After(5 * time.Second):
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
