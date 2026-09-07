package chat

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/obsv"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/oid"
)

// maxMessageLength is MAX_MESSAGE_LENGTH from MessageHttpController: 10kb,
// about 1,500 words.
const maxMessageLength = 10 * 1024

// defaultMessageLimit is DEFAULT_MESSAGE_LIMIT from MessageHttpController.
const defaultMessageLimit = 50

// Server holds the chat service's dependencies.
type Server struct {
	store   *Store
	log     *slog.Logger
	metrics *obsv.Metrics
}

// NewServer builds the chat HTTP server.
func NewServer(store *Store, log *slog.Logger, metrics *obsv.Metrics) *Server {
	return &Server{store: store, log: log, metrics: metrics}
}

// Handler returns the fully wired chat router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	route := func(method, pattern string, h http.HandlerFunc) {
		// The metric label is the route template, not the concrete URL, so
		// cardinality stays bounded. The transformation matches
		// @overleaf/metrics' getRoutePath(): "/" becomes "_", leading "_"
		// dropped.
		label := strings.TrimPrefix(strings.ReplaceAll(pattern, "/", "_"), "_")
		mux.Handle(method+" "+pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h(w, obsv.WithRoutePath(r, label))
		}))
	}

	route("GET", "/status", s.getStatus)
	route("GET", "/metrics", func(w http.ResponseWriter, r *http.Request) {
		s.metrics.Handler().ServeHTTP(w, r)
	})

	route("GET", "/project/{projectId}/messages", s.getGlobalMessages)
	route("POST", "/project/{projectId}/messages", s.sendGlobalMessage)
	route("GET", "/project/{projectId}/messages/{messageId}", s.getGlobalMessage)
	route("DELETE", "/project/{projectId}/messages/{messageId}", s.deleteGlobalMessage)
	route("POST", "/project/{projectId}/messages/{messageId}/edit", s.editGlobalMessage)

	route("GET", "/project/{projectId}/threads", s.getThreads)
	route("GET", "/project/{projectId}/thread/{threadId}", s.getThread)
	route("DELETE", "/project/{projectId}/thread/{threadId}", s.deleteThread)
	route("POST", "/project/{projectId}/thread/{threadId}/messages", s.sendThreadMessage)
	route("GET", "/project/{projectId}/thread/{threadId}/messages/{messageId}", s.getThreadMessage)
	route("DELETE", "/project/{projectId}/thread/{threadId}/messages/{messageId}", s.deleteMessage)
	route("POST", "/project/{projectId}/thread/{threadId}/messages/{messageId}/edit", s.editMessage)
	route("DELETE", "/project/{projectId}/thread/{threadId}/user/{userId}/messages/{messageId}", s.deleteUserMessage)
	route("POST", "/project/{projectId}/thread/{threadId}/resolve", s.resolveThread)
	route("POST", "/project/{projectId}/thread/{threadId}/reopen", s.reopenThread)

	route("GET", "/project/{projectId}/resolved-thread-ids", s.getResolvedThreadIDs)
	route("DELETE", "/project/{projectId}", s.destroyProject)
	route("POST", "/project/{projectId}/duplicate-comment-threads", s.duplicateCommentThreads)
	route("POST", "/project/{projectId}/generate-thread-data", s.generateThreadData)
	route("POST", "/project/{projectId}/clone-comment-threads", s.cloneCommentThreads)

	// Unmatched routes get exegesis' 404 body. No metric is recorded, because
	// getRoutePath() returns null when express did not match a route.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not found"})
	})

	return s.metrics.HTTPMiddleware(s.log, mux)
}

// --- response helpers ---------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// badRequest sends the plain-string 400 body the controller produces via
// res.status(400).setBody(message).
func badRequest(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusBadRequest, message)
}

// validationError sends exegesis' request-validation failure. Callers assert
// on the message field, so that is what must stay stable.
func validationError(w http.ResponseWriter, detail string) {
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"message": "Validation errors",
		"errors":  []map[string]any{{"message": detail, "location": map[string]string{"in": "body"}}},
	})
}

// fail logs an unexpected error and returns exegesis' 500 body.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("chat request failed",
		slog.String("method", r.Method),
		slog.String("url", r.URL.RequestURI()),
		slog.String("err", err.Error()),
	)
	writeJSON(w, http.StatusInternalServerError,
		map[string]string{"message": "Internal error: " + err.Error()})
}

// --- request parsing ----------------------------------------------------

// decodeBody reads a JSON object body. A missing or empty body decodes to an
// empty map, as express's json parser produces {}.
func decodeBody(r *http.Request) (map[string]any, error) {
	var body map[string]any
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil {
		return map[string]any{}, err
	}
	if body == nil {
		return map[string]any{}, nil
	}
	return body, nil
}

// requiredString applies the OpenAPI `type: string` + `required` check that
// exegesis performs before the controller runs.
func requiredString(body map[string]any, key string) (string, bool) {
	v, present := body[key]
	if !present {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// projectParam validates the projectId path parameter, mirroring readContext.
func (s *Server) projectParam(w http.ResponseWriter, r *http.Request) (bson.ObjectID, bool) {
	raw := r.PathValue("projectId")
	id, ok := oid.Parse(raw)
	if !ok {
		badRequest(w, "Invalid projectId")
		return id, false
	}
	return id, true
}

// threadParam validates the threadId path parameter, mirroring readContext.
func (s *Server) threadParam(w http.ResponseWriter, r *http.Request) (bson.ObjectID, bool) {
	raw := r.PathValue("threadId")
	id, ok := oid.Parse(raw)
	if !ok {
		badRequest(w, "Invalid threadId")
		return id, false
	}
	return id, true
}

// projectAndThread validates both path parameters in the order readContext
// checks them: projectId first, so a request with both malformed reports the
// project error.
func (s *Server) projectAndThread(w http.ResponseWriter, r *http.Request) (bson.ObjectID, bson.ObjectID, bool) {
	projectID, ok := s.projectParam(w, r)
	if !ok {
		return projectID, bson.ObjectID{}, false
	}
	threadID, ok := s.threadParam(w, r)
	if !ok {
		return projectID, threadID, false
	}
	return projectID, threadID, true
}

// jsParseInt mimics JavaScript's parseInt(s, 10): leading whitespace and an
// optional sign, then as many digits as parse, ignoring any trailing text.
func jsParseInt(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	i := 0
	neg := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		neg = s[i] == '-'
		i++
	}
	start := i
	var n int64
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		n = n*10 + int64(s[i]-'0')
		i++
	}
	if i == start {
		return 0, false
	}
	if neg {
		n = -n
	}
	return n, true
}

// --- handlers -----------------------------------------------------------

func (s *Server) getStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, "chat is alive")
}

func (s *Server) getGlobalMessages(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.projectParam(w, r)
	if !ok {
		return
	}
	s.listMessages(w, r, projectID, nil)
}

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request, projectID bson.ObjectID, threadID *bson.ObjectID) {
	limit := int64(defaultMessageLimit)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, ok := jsParseInt(raw); ok {
			limit = n
		}
	}
	var before *Millis
	if raw := r.URL.Query().Get("before"); raw != "" {
		// parseInt returning NaN makes the Node code skip the filter, because
		// NaN is falsy; an unparseable value is therefore ignored here too.
		if n, ok := jsParseInt(raw); ok && n != 0 {
			ms := Millis(n)
			before = &ms
		}
	}

	room, err := s.store.FindOrCreateThread(r.Context(), projectID, threadID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	messages, err := s.store.GetMessages(r.Context(), room.ID, limit, before)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, formatMessages(messages))
}

func (s *Server) sendGlobalMessage(w http.ResponseWriter, r *http.Request) {
	s.sendMessage(w, r, true)
}

func (s *Server) sendThreadMessage(w http.ResponseWriter, r *http.Request) {
	s.sendMessage(w, r, false)
}

func (s *Server) sendMessage(w http.ResponseWriter, r *http.Request, global bool) {
	body, err := decodeBody(r)
	if err != nil {
		validationError(w, "request body must be a JSON object")
		return
	}
	// The Message schema requires user_id and content, and exegesis rejects
	// the request before the controller sees it.
	userIDRaw, ok := requiredString(body, "user_id")
	if !ok {
		validationError(w, "user_id is required and must be a string")
		return
	}
	content, ok := requiredString(body, "content")
	if !ok {
		validationError(w, "content is required and must be a string")
		return
	}

	projectID, ok := s.projectParam(w, r)
	if !ok {
		return
	}
	var threadID *bson.ObjectID
	if !global {
		id, ok := s.threadParam(w, r)
		if !ok {
			return
		}
		threadID = &id
	}

	userID, ok := oid.Parse(userIDRaw)
	if !ok {
		badRequest(w, "Invalid userId")
		return
	}
	if content == "" {
		badRequest(w, "No content provided")
		return
	}
	if len(content) > maxMessageLength {
		badRequest(w, "Content too long (> 10240 bytes)")
		return
	}

	room, err := s.store.FindOrCreateThread(r.Context(), projectID, threadID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	msg, err := s.store.CreateMessage(r.Context(), room.ID, userID, content, NowMillis())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := formatMessage(msg)
	out.RoomID = projectID.Hex()
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) getGlobalMessage(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.projectParam(w, r)
	if !ok {
		return
	}
	s.showMessage(w, r, projectID, nil)
}

func (s *Server) getThreadMessage(w http.ResponseWriter, r *http.Request) {
	projectID, threadID, ok := s.projectAndThread(w, r)
	if !ok {
		return
	}
	s.showMessage(w, r, projectID, &threadID)
}

func (s *Server) showMessage(w http.ResponseWriter, r *http.Request, projectID bson.ObjectID, threadID *bson.ObjectID) {
	messageID, ok := oid.Parse(r.PathValue("messageId"))
	if !ok {
		// The Node code passes the raw value to new ObjectId(), which throws
		// and surfaces as a 500.
		s.fail(w, r, errors.New("invalid messageId"))
		return
	}
	room, err := s.store.FindThread(r.Context(), projectID, threadID)
	if errors.Is(err, ErrMissingThread) {
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	msg, err := s.store.GetMessage(r.Context(), room.ID, messageID)
	if errors.Is(err, ErrMissingMessage) {
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, formatMessage(msg))
}

func (s *Server) getThreads(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.projectParam(w, r)
	if !ok {
		return
	}
	rooms, err := s.store.FindAllThreadRooms(r.Context(), projectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	messages, err := s.store.FindAllMessagesInRooms(r.Context(), roomIDs(rooms))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, groupMessagesByThreads(rooms, messages))
}

func (s *Server) getThread(w http.ResponseWriter, r *http.Request) {
	projectID, threadID, ok := s.projectAndThread(w, r)
	if !ok {
		return
	}
	room, err := s.store.FindThread(r.Context(), projectID, &threadID)
	if errors.Is(err, ErrMissingThread) {
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	messages, err := s.store.FindAllMessagesInRooms(r.Context(), []bson.ObjectID{room.ID})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	threads := groupMessagesByThreads([]Room{room}, messages)
	thread, ok := threads[threadID.Hex()]
	if !ok {
		// A thread with no messages is not represented in the grouping, and
		// the Node code returns 404 for it.
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	writeJSON(w, http.StatusOK, thread)
}

func (s *Server) resolveThread(w http.ResponseWriter, r *http.Request) {
	body, err := decodeBody(r)
	if err != nil {
		validationError(w, "request body must be a JSON object")
		return
	}
	userID, ok := requiredString(body, "user_id")
	if !ok {
		validationError(w, "user_id is required and must be a string")
		return
	}
	projectID, threadID, ok := s.projectAndThread(w, r)
	if !ok {
		return
	}
	if err := s.store.ResolveThread(r.Context(), projectID, threadID, userID); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) reopenThread(w http.ResponseWriter, r *http.Request) {
	projectID, threadID, ok := s.projectAndThread(w, r)
	if !ok {
		return
	}
	if err := s.store.ReopenThread(r.Context(), projectID, threadID); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) deleteThread(w http.ResponseWriter, r *http.Request) {
	projectID, threadID, ok := s.projectAndThread(w, r)
	if !ok {
		return
	}
	roomID, err := s.store.DeleteThread(r.Context(), projectID, &threadID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.store.DeleteAllMessagesInRoom(r.Context(), roomID); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) editMessage(w http.ResponseWriter, r *http.Request) {
	projectID, threadID, ok := s.projectAndThread(w, r)
	if !ok {
		return
	}
	s.applyEdit(w, r, projectID, &threadID)
}

func (s *Server) editGlobalMessage(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.projectParam(w, r)
	if !ok {
		return
	}
	s.applyEdit(w, r, projectID, nil)
}

func (s *Server) applyEdit(w http.ResponseWriter, r *http.Request, projectID bson.ObjectID, threadID *bson.ObjectID) {
	body, err := decodeBody(r)
	if err != nil {
		validationError(w, "request body must be a JSON object")
		return
	}
	content, ok := requiredString(body, "content")
	if !ok {
		validationError(w, "content is required and must be a string")
		return
	}
	messageID, ok := oid.Parse(r.PathValue("messageId"))
	if !ok {
		s.fail(w, r, errors.New("invalid messageId"))
		return
	}

	// The edit endpoints read `userId`, not `user_id`; web sends that spelling
	// and the OpenAPI schema allows it through as an additional property.
	var userID *bson.ObjectID
	if raw, present := body["userId"]; present && raw != nil {
		str, isStr := raw.(string)
		if !isStr {
			s.fail(w, r, errors.New("userId must be a string"))
			return
		}
		id, valid := oid.Parse(str)
		if !valid {
			s.fail(w, r, errors.New("invalid userId"))
			return
		}
		userID = &id
	}

	room, err := s.store.FindOrCreateThread(r.Context(), projectID, threadID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	found, err := s.store.UpdateMessage(r.Context(), room.ID, messageID, userID, content, NowMillis())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) deleteMessage(w http.ResponseWriter, r *http.Request) {
	projectID, threadID, ok := s.projectAndThread(w, r)
	if !ok {
		return
	}
	s.removeMessage(w, r, projectID, &threadID)
}

func (s *Server) deleteGlobalMessage(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.projectParam(w, r)
	if !ok {
		return
	}
	s.removeMessage(w, r, projectID, nil)
}

func (s *Server) removeMessage(w http.ResponseWriter, r *http.Request, projectID bson.ObjectID, threadID *bson.ObjectID) {
	messageID, ok := oid.Parse(r.PathValue("messageId"))
	if !ok {
		s.fail(w, r, errors.New("invalid messageId"))
		return
	}
	room, err := s.store.FindOrCreateThread(r.Context(), projectID, threadID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.store.DeleteMessage(r.Context(), room.ID, messageID); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) deleteUserMessage(w http.ResponseWriter, r *http.Request) {
	projectID, threadID, ok := s.projectAndThread(w, r)
	if !ok {
		return
	}
	userID, ok := oid.Parse(r.PathValue("userId"))
	if !ok {
		s.fail(w, r, errors.New("invalid userId"))
		return
	}
	messageID, ok := oid.Parse(r.PathValue("messageId"))
	if !ok {
		s.fail(w, r, errors.New("invalid messageId"))
		return
	}
	room, err := s.store.FindOrCreateThread(r.Context(), projectID, &threadID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.store.DeleteUserMessage(r.Context(), userID, room.ID, messageID); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) getResolvedThreadIDs(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.projectParam(w, r)
	if !ok {
		return
	}
	ids, err := s.store.ResolvedThreadIDs(r.Context(), projectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"resolvedThreadIds": ids})
}

func (s *Server) destroyProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.projectParam(w, r)
	if !ok {
		return
	}
	rooms, err := s.store.FindAllRooms(r.Context(), projectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.store.DeleteAllMessagesInRooms(r.Context(), roomIDs(rooms)); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.store.DeleteAllThreadsInProject(r.Context(), projectID); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// threadIDList reads the `threads` array shared by the duplicate and generate
// endpoints.
func threadIDList(body map[string]any) ([]string, bool) {
	raw, present := body["threads"]
	if !present {
		return nil, false
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		str, ok := item.(string)
		if !ok {
			return nil, false
		}
		ids = append(ids, str)
	}
	return ids, true
}

func (s *Server) duplicateCommentThreads(w http.ResponseWriter, r *http.Request) {
	body, err := decodeBody(r)
	if err != nil {
		validationError(w, "request body must be a JSON object")
		return
	}
	projectID, ok := s.projectParam(w, r)
	if !ok {
		return
	}
	threadIDs, ok := threadIDList(body)
	if !ok {
		// Iterating a missing `threads` throws in the Node controller.
		s.fail(w, r, errors.New("threads must be an array of strings"))
		return
	}

	result := map[string]map[string]string{}
	for _, raw := range threadIDs {
		threadID, valid := oid.Parse(raw)
		if !valid {
			s.fail(w, r, errors.New("invalid thread id: "+raw))
			return
		}
		oldRoom, newRoom, err := s.store.DuplicateThread(r.Context(), projectID, threadID)
		if errors.Is(err, ErrMissingThread) {
			// Expected when the comment was deleted before duplication.
			result[raw] = map[string]string{"error": "not found"}
			continue
		}
		if err != nil {
			s.log.Error("error duplicating thread", slog.String("err", err.Error()))
			result[raw] = map[string]string{"error": "unknown"}
			continue
		}
		if err := s.store.DuplicateRoomToOtherRoom(r.Context(), oldRoom.ID, newRoom.ID); err != nil {
			s.log.Error("error duplicating thread", slog.String("err", err.Error()))
			result[raw] = map[string]string{"error": "unknown"}
			continue
		}
		result[raw] = map[string]string{"duplicateId": newRoom.ThreadID.Hex()}
	}
	writeJSON(w, http.StatusOK, map[string]any{"newThreads": result})
}

func (s *Server) generateThreadData(w http.ResponseWriter, r *http.Request) {
	body, err := decodeBody(r)
	if err != nil {
		validationError(w, "request body must be a JSON object")
		return
	}
	projectID, ok := s.projectParam(w, r)
	if !ok {
		return
	}
	rawIDs, ok := threadIDList(body)
	if !ok {
		s.fail(w, r, errors.New("threads must be an array of strings"))
		return
	}
	threadIDs := make([]bson.ObjectID, 0, len(rawIDs))
	for _, raw := range rawIDs {
		id, valid := oid.Parse(raw)
		if !valid {
			s.fail(w, r, errors.New("invalid thread id: "+raw))
			return
		}
		threadIDs = append(threadIDs, id)
	}

	rooms, err := s.store.FindThreadsByID(r.Context(), projectID, threadIDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	messages, err := s.store.FindAllMessagesInRooms(r.Context(), roomIDs(rooms))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, groupMessagesByThreads(rooms, messages))
}

func (s *Server) cloneCommentThreads(w http.ResponseWriter, r *http.Request) {
	body, err := decodeBody(r)
	if err != nil {
		validationError(w, "request body must be a JSON object")
		return
	}
	sourceProjectID, ok := s.projectParam(w, r)
	if !ok {
		return
	}

	// new ObjectId(undefined) generates a fresh id in the Node driver, so a
	// missing targetProjectId clones into a new, empty project rather than
	// failing. Preserved deliberately; web always sends the field.
	targetProjectID := bson.NewObjectID()
	if raw, present := body["targetProjectId"]; present && raw != nil {
		str, isStr := raw.(string)
		if !isStr {
			s.fail(w, r, errors.New("targetProjectId must be a string"))
			return
		}
		id, valid := oid.Parse(str)
		if !valid {
			s.fail(w, r, errors.New("invalid targetProjectId"))
			return
		}
		targetProjectID = id
	}

	mapping, err := s.store.CloneThreads(r.Context(), sourceProjectID, targetProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, m := range mapping {
		if err := s.store.DuplicateRoomToOtherRoom(r.Context(), m.From, m.To); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusNoContent, nil)
}
