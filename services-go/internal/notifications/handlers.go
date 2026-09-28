package notifications

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/bsonjson"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/obsv"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/oid"
)

// Server holds the notifications service's dependencies.
type Server struct {
	store   *Store
	log     *slog.Logger
	metrics *obsv.Metrics
	// selfURL is the base address the health check calls back into, matching
	// HealthCheckController's use of http://127.0.0.1:{port}.
	selfURL string
	client  *http.Client
}

// NewServer builds the notifications HTTP server.
func NewServer(store *Store, log *slog.Logger, metrics *obsv.Metrics, selfURL string, client *http.Client) *Server {
	return &Server{store: store, log: log, metrics: metrics, selfURL: selfURL, client: client}
}

// Handler returns the fully wired notifications router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Express route labels drop the ":" from parameters, unlike the OpenAPI
	// "{param}" form the chat service uses.
	route := func(method, pattern, label string, h http.HandlerFunc) {
		mux.Handle(method+" "+pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h(w, obsv.WithRoutePath(r, label))
		}))
	}

	route("POST", "/user/{user_id}", "user_user_id", s.addNotification)
	route("GET", "/user/{user_id}", "user_user_id", s.getUserNotifications)
	route("DELETE", "/user/{user_id}", "user_user_id", s.removeNotificationKey)
	route("DELETE", "/user/{user_id}/notification/{notification_id}",
		"user_user_id_notification_notification_id", s.removeNotificationID)
	route("DELETE", "/key/{key}", "key_key", s.removeNotificationByKeyOnly)
	route("GET", "/key/{key}/count", "key_key_count", s.countNotificationsByKeyOnly)
	route("DELETE", "/key/{key}/bulk", "key_key_bulk", s.deleteUnreadNotificationsByKeyOnlyBulk)

	route("GET", "/status", "status", func(w http.ResponseWriter, r *http.Request) {
		sendText(w, http.StatusOK, "notifications is up")
	})
	route("GET", "/health_check", "health_check", s.healthCheck)
	route("GET", "/metrics", "metrics", func(w http.ResponseWriter, r *http.Request) {
		s.metrics.Handler().ServeHTTP(w, r)
	})

	// app.get('*', ...) in the Node service: a matched express route whose
	// metric label is the empty string.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		sendStatus(w, http.StatusNotFound)
	})

	return s.metrics.HTTPMiddleware(s.log, mux)
}

// --- response helpers ---------------------------------------------------

// sendStatus mirrors express's res.sendStatus(code): the status text as a
// plain-text body.
func sendStatus(w http.ResponseWriter, status int) {
	sendText(w, status, http.StatusText(status))
}

func sendText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func sendJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// userParam validates the user_id path parameter. A malformed id fails the
// zod schema in the Node service, which the error handler turns into a 404.
func (s *Server) userParam(w http.ResponseWriter, r *http.Request) (bson.ObjectID, bool) {
	id, ok := oid.Parse(r.PathValue("user_id"))
	if !ok {
		sendStatus(w, http.StatusNotFound)
		return id, false
	}
	return id, true
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("notifications request failed",
		slog.String("method", r.Method),
		slog.String("url", r.URL.RequestURI()),
		slog.String("err", err.Error()),
	)
	sendStatus(w, http.StatusInternalServerError)
}

// --- handlers -----------------------------------------------------------

func (s *Server) getUserNotifications(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userParam(w, r)
	if !ok {
		return
	}
	s.metrics.Inc("getUserNotifications")

	docs, err := s.store.GetUserNotifications(r.Context(), userID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	sendJSON(w, http.StatusOK, bsonjson.Documents(docs))
}

func (s *Server) addNotification(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userParam(w, r)
	if !ok {
		return
	}
	s.metrics.Inc("addNotification")

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		// The zod schema requires an object body; a non-object is an invalid
		// request rather than invalid params.
		sendStatus(w, http.StatusBadRequest)
		return
	}

	n := Notification{}
	if v, present := body["key"]; present {
		if str, isStr := v.(string); isStr {
			n.Key = str
		}
	}
	n.MessageOpts = body["messageOpts"]
	n.TemplateKey = body["templateKey"]
	n.Expires = body["expires"]
	if v, present := body["forceCreate"]; present {
		if b, isBool := v.(bool); isBool {
			n.ForceCreate = b
		}
	}

	if err := s.store.AddNotification(r.Context(), userID, n); err != nil {
		// The Node controller swallows the error and answers 500.
		s.internalError(w, r, err)
		return
	}
	sendStatus(w, http.StatusOK)
}

func (s *Server) removeNotificationID(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userParam(w, r)
	if !ok {
		return
	}
	notificationID, valid := oid.Parse(r.PathValue("notification_id"))
	if !valid {
		sendStatus(w, http.StatusNotFound)
		return
	}
	s.metrics.Inc("removeNotificationId")

	if err := s.store.RemoveNotificationID(r.Context(), userID, notificationID); err != nil {
		s.internalError(w, r, err)
		return
	}
	sendStatus(w, http.StatusOK)
}

func (s *Server) removeNotificationKey(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userParam(w, r)
	if !ok {
		return
	}
	var body struct {
		Key *string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Key == nil {
		sendStatus(w, http.StatusBadRequest)
		return
	}
	s.metrics.Inc("removeNotificationKey")

	if err := s.store.RemoveNotificationKey(r.Context(), userID, *body.Key); err != nil {
		s.internalError(w, r, err)
		return
	}
	sendStatus(w, http.StatusOK)
}

func (s *Server) removeNotificationByKeyOnly(w http.ResponseWriter, r *http.Request) {
	s.metrics.Inc("removeNotificationKey")
	if err := s.store.RemoveNotificationByKeyOnly(r.Context(), r.PathValue("key")); err != nil {
		s.internalError(w, r, err)
		return
	}
	sendStatus(w, http.StatusOK)
}

func (s *Server) countNotificationsByKeyOnly(w http.ResponseWriter, r *http.Request) {
	count, err := s.store.CountNotificationsByKeyOnly(r.Context(), r.PathValue("key"))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]int64{"count": count})
}

func (s *Server) deleteUnreadNotificationsByKeyOnlyBulk(w http.ResponseWriter, r *http.Request) {
	count, err := s.store.DeleteUnreadNotificationsByKeyOnlyBulk(r.Context(), r.PathValue("key"))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]int64{"count": count})
}
