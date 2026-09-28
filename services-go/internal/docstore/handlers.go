package docstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/bsonjson"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/obsv"
)

// idPattern is the app.param check: a lowercase 24-character hex string.
// It is stricter than ObjectId.isValid, which also accepts 12-character
// strings, and a value that fails it reaches the generic error handler and
// comes back as a 500 rather than a 400.
var idPattern = regexp.MustCompile(`^[0-9a-f]{24}$`)

// Config holds the settings the HTTP layer reads.
type Config struct {
	MaxDocLength       int
	MaxJSONRequestSize int64
	HealthCheckProject string
	SelfURL            string
}

// Server is the docstore HTTP server.
type Server struct {
	manager *Manager
	log     *slog.Logger
	metrics *obsv.Metrics
	cfg     Config
	client  *http.Client
	store   *Store
}

// NewServer builds the docstore HTTP server.
func NewServer(manager *Manager, store *Store, log *slog.Logger, metrics *obsv.Metrics, cfg Config, client *http.Client) *Server {
	return &Server{manager: manager, store: store, log: log, metrics: metrics, cfg: cfg, client: client}
}

// Handler returns the fully wired docstore router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	route := func(method, pattern, label string, h http.HandlerFunc) {
		mux.Handle(method+" "+pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h(w, obsv.WithRoutePath(r, label))
		}))
	}

	route("GET", "/project/{project_id}/doc-deleted", "project_project_id_doc-deleted", s.getAllDeletedDocs)
	route("GET", "/project/{project_id}/doc", "project_project_id_doc", s.getAllDocs)
	route("GET", "/project/{project_id}/doc-with-ranges", "project_project_id_doc-with-ranges", s.getAllDocsWithRanges)
	route("GET", "/project/{project_id}/doc-versions", "project_project_id_doc-versions", s.getAllDocVersions)
	route("GET", "/project/{project_id}/ranges", "project_project_id_ranges", s.getAllRanges)
	route("GET", "/project/{project_id}/comment-thread-ids", "project_project_id_comment-thread-ids", s.getCommentThreadIds)
	route("GET", "/project/{project_id}/tracked-changes-user-ids", "project_project_id_tracked-changes-user-ids", s.getTrackedChangesUserIds)
	route("GET", "/project/{project_id}/has-ranges", "project_project_id_has-ranges", s.projectHasRanges)

	route("GET", "/project/{project_id}/doc/{doc_id}", "project_project_id_doc_doc_id", s.getDoc)
	route("GET", "/project/{project_id}/doc/{doc_id}/deleted", "project_project_id_doc_doc_id_deleted", s.isDocDeleted)
	route("GET", "/project/{project_id}/doc/{doc_id}/raw", "project_project_id_doc_doc_id_raw", s.getRawDoc)
	route("GET", "/project/{project_id}/doc/{doc_id}/peek", "project_project_id_doc_doc_id_peek", s.peekDoc)
	route("POST", "/project/{project_id}/doc/{doc_id}", "project_project_id_doc_doc_id", s.updateDoc)
	route("PATCH", "/project/{project_id}/doc/{doc_id}", "project_project_id_doc_doc_id", s.patchDoc)
	route("DELETE", "/project/{project_id}/doc/{doc_id}", "project_project_id_doc_doc_id",
		func(w http.ResponseWriter, r *http.Request) {
			sendText(w, http.StatusInternalServerError,
				"DELETE-ing a doc is DEPRECATED. PATCH the doc instead.")
		})

	route("POST", "/project/{project_id}/archive", "project_project_id_archive", s.archiveAllDocs)
	route("POST", "/project/{project_id}/doc/{doc_id}/archive", "project_project_id_doc_doc_id_archive", s.archiveDoc)
	route("POST", "/project/{project_id}/unarchive", "project_project_id_unarchive", s.unArchiveAllDocs)
	route("POST", "/project/{project_id}/destroy", "project_project_id_destroy", s.destroyProject)

	route("GET", "/health_check", "health_check", s.healthCheck)
	route("GET", "/status", "status", func(w http.ResponseWriter, r *http.Request) {
		sendText(w, http.StatusOK, "docstore is alive")
	})
	route("GET", "/metrics", "metrics", func(w http.ResponseWriter, r *http.Request) {
		s.metrics.Handler().ServeHTTP(w, r)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	return s.metrics.HTTPMiddleware(s.log, mux)
}

// --- responses ----------------------------------------------------------

func sendText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func sendStatus(w http.ResponseWriter, status int) {
	sendText(w, status, http.StatusText(status))
}

func sendJSON(w http.ResponseWriter, status int, v any) {
	encoded, err := json.Marshal(v)
	if err != nil {
		sendText(w, http.StatusInternalServerError, "Oops, something went wrong")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(encoded)
}

// fail maps a domain error onto a status the way the Node error handler does.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		// The Node service logs only the request here; carrying the error text
		// too is what tells an operator which lookup actually missed.
		s.log.Warn("not found",
			slog.String("url", r.URL.RequestURI()), slog.String("err", err.Error()))
		sendStatus(w, http.StatusNotFound)
	case errors.Is(err, ErrDocModified):
		s.log.Warn("conflict: doc modified", slog.String("url", r.URL.RequestURI()))
		sendStatus(w, http.StatusConflict)
	case errors.Is(err, ErrDocVersionDecremented):
		s.log.Warn("conflict: doc version decremented", slog.String("url", r.URL.RequestURI()))
		sendStatus(w, http.StatusConflict)
	default:
		s.log.Error("request errored",
			slog.String("url", r.URL.RequestURI()), slog.String("err", err.Error()))
		sendText(w, http.StatusInternalServerError, "Oops, something went wrong")
	}
}

// --- parameters ---------------------------------------------------------

// projectParam reads and validates project_id. A malformed id fails the
// app.param check, which surfaces as a 500 rather than a 400.
func (s *Server) projectParam(w http.ResponseWriter, r *http.Request) (bson.ObjectID, bool) {
	return s.idParam(w, r, "project_id", "invalid project id")
}

func (s *Server) docParam(w http.ResponseWriter, r *http.Request) (bson.ObjectID, bool) {
	return s.idParam(w, r, "doc_id", "invalid doc id")
}

func (s *Server) idParam(w http.ResponseWriter, r *http.Request, name, message string) (bson.ObjectID, bool) {
	raw := r.PathValue(name)
	if !idPattern.MatchString(raw) {
		s.fail(w, r, errors.New(message))
		return bson.ObjectID{}, false
	}
	id, err := bson.ObjectIDFromHex(raw)
	if err != nil {
		s.fail(w, r, errors.New(message))
		return bson.ObjectID{}, false
	}
	return id, true
}

func (s *Server) projectAndDoc(w http.ResponseWriter, r *http.Request) (bson.ObjectID, bson.ObjectID, bool) {
	projectID, ok := s.projectParam(w, r)
	if !ok {
		return projectID, bson.ObjectID{}, false
	}
	docID, ok := s.docParam(w, r)
	if !ok {
		return projectID, docID, false
	}
	return projectID, docID, true
}

// --- views --------------------------------------------------------------

// buildDocView renders a doc the way _buildDocView does: the id, then only
// those attributes that are present, in a fixed order.
func buildDocView(doc *Doc) bsonjson.Document {
	view := bson.D{{Key: "_id", Value: doc.ID.Hex()}}
	if doc.Lines != nil {
		view = append(view, bson.E{Key: "lines", Value: linesValue(doc.Lines)})
	}
	if doc.Rev != nil {
		view = append(view, bson.E{Key: "rev", Value: *doc.Rev})
	}
	if doc.Version != nil {
		view = append(view, bson.E{Key: "version", Value: *doc.Version})
	}
	if doc.Ranges != nil {
		view = append(view, bson.E{Key: "ranges", Value: doc.Ranges})
	}
	if doc.Deleted != nil {
		view = append(view, bson.E{Key: "deleted", Value: *doc.Deleted})
	}
	return bsonjson.Document(view)
}

func linesValue(lines []string) bson.A {
	out := make(bson.A, 0, len(lines))
	for _, l := range lines {
		out = append(out, l)
	}
	return out
}

// buildDocsArrayView renders a list of docs, defaulting missing lines to an
// empty array for the endpoints that promise them.
func buildDocsArrayView(docs []Doc, fillLines bool) []bsonjson.Document {
	views := make([]bsonjson.Document, 0, len(docs))
	for i := range docs {
		view := buildDocView(&docs[i])
		if fillLines {
			if _, present := docGet(bson.D(view), "lines"); !present {
				view = append(view, bson.E{Key: "lines", Value: bson.A{}})
			}
		}
		views = append(views, view)
	}
	return views
}

// --- handlers -----------------------------------------------------------

func (s *Server) getDoc(w http.ResponseWriter, r *http.Request) {
	projectID, docID, ok := s.projectAndDoc(w, r)
	if !ok {
		return
	}
	doc, err := s.manager.GetFullDoc(r.Context(), projectID, docID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if doc.IsDeleted() && r.URL.Query().Get("include_deleted") != "true" {
		sendStatus(w, http.StatusNotFound)
		return
	}
	sendJSON(w, http.StatusOK, buildDocView(doc))
}

func (s *Server) peekDoc(w http.ResponseWriter, r *http.Request) {
	projectID, docID, ok := s.projectAndDoc(w, r)
	if !ok {
		return
	}
	doc, err := s.manager.PeekDoc(r.Context(), projectID, docID, bson.D{
		{Key: "deleted", Value: true},
		{Key: "inS3", Value: true},
		{Key: "lines", Value: true},
		{Key: "ranges", Value: true},
		{Key: "rev", Value: 1},
		{Key: "version", Value: true},
	}, false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	status := "active"
	if doc.IsArchived() {
		status = "archived"
	}
	w.Header().Set("x-doc-status", status)
	sendJSON(w, http.StatusOK, buildDocView(doc))
}

func (s *Server) isDocDeleted(w http.ResponseWriter, r *http.Request) {
	projectID, docID, ok := s.projectAndDoc(w, r)
	if !ok {
		return
	}
	deleted, err := s.manager.IsDocDeleted(r.Context(), projectID, docID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]bool{"deleted": deleted})
}

func (s *Server) getRawDoc(w http.ResponseWriter, r *http.Request) {
	projectID, docID, ok := s.projectAndDoc(w, r)
	if !ok {
		return
	}
	content, err := s.manager.GetDocLines(r.Context(), projectID, docID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, content)
}

func (s *Server) getAllDocs(w http.ResponseWriter, r *http.Request) {
	s.listDocs(w, r, bson.D{{Key: "lines", Value: true}, {Key: "rev", Value: true}}, true)
}

func (s *Server) getAllDocsWithRanges(w http.ResponseWriter, r *http.Request) {
	s.listDocs(w, r, bson.D{
		{Key: "lines", Value: true}, {Key: "rev", Value: true}, {Key: "ranges", Value: true},
	}, true)
}

func (s *Server) getAllRanges(w http.ResponseWriter, r *http.Request) {
	s.listDocs(w, r, bson.D{{Key: "ranges", Value: true}}, false)
}

func (s *Server) listDocs(w http.ResponseWriter, r *http.Request, projection bson.D, fillLines bool) {
	projectID, ok := s.projectParam(w, r)
	if !ok {
		return
	}
	docs, err := s.manager.GetAllNonDeletedDocs(r.Context(), projectID, projection)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	sendJSON(w, http.StatusOK, buildDocsArrayView(docs, fillLines))
}

func (s *Server) getAllDocVersions(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.projectParam(w, r)
	if !ok {
		return
	}
	docs, err := s.manager.GetAllDocVersions(r.Context(), projectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	views := make([]bsonjson.Document, 0, len(docs))
	for i := range docs {
		view := bson.D{{Key: "_id", Value: docs[i].ID.Hex()}}
		if docs[i].Version != nil {
			view = append(view, bson.E{Key: "version", Value: *docs[i].Version})
		}
		views = append(views, bsonjson.Document(view))
	}
	sendJSON(w, http.StatusOK, views)
}

func (s *Server) getAllDeletedDocs(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.projectParam(w, r)
	if !ok {
		return
	}
	docs, err := s.manager.GetAllDeletedDocs(r.Context(), projectID, bson.D{
		{Key: "name", Value: true}, {Key: "deletedAt", Value: true},
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	views := make([]bsonjson.Document, 0, len(docs))
	for i := range docs {
		view := bson.D{{Key: "_id", Value: docs[i].ID.Hex()}}
		if docs[i].Name != nil {
			view = append(view, bson.E{Key: "name", Value: *docs[i].Name})
		} else {
			view = append(view, bson.E{Key: "name", Value: nil})
		}
		if docs[i].DeletedAt != nil {
			view = append(view, bson.E{Key: "deletedAt", Value: *docs[i].DeletedAt})
		} else {
			view = append(view, bson.E{Key: "deletedAt", Value: nil})
		}
		views = append(views, bsonjson.Document(view))
	}
	sendJSON(w, http.StatusOK, views)
}

func (s *Server) getCommentThreadIds(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.projectParam(w, r)
	if !ok {
		return
	}
	threads, err := s.manager.GetCommentThreadIds(r.Context(), projectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	sendJSON(w, http.StatusOK, threads)
}

func (s *Server) getTrackedChangesUserIds(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.projectParam(w, r)
	if !ok {
		return
	}
	userIDs, err := s.manager.GetTrackedChangesUserIds(r.Context(), projectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	sendJSON(w, http.StatusOK, userIDs)
}

func (s *Server) projectHasRanges(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.projectParam(w, r)
	if !ok {
		return
	}
	hasRanges, err := s.manager.ProjectHasRanges(r.Context(), projectID,
		r.URL.Query().Get("useSecondary") == "true")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]bool{"projectHasRanges": hasRanges})
}

func (s *Server) updateDoc(w http.ResponseWriter, r *http.Request) {
	projectID, docID, ok := s.projectAndDoc(w, r)
	if !ok {
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, s.cfg.MaxJSONRequestSize+1))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if int64(len(body)) > s.cfg.MaxJSONRequestSize {
		sendText(w, http.StatusRequestEntityTooLarge, "request entity too large")
		return
	}
	doc, err := bsonjson.DecodeDocument(body)
	if err != nil {
		// An unparseable body fails before any field check, as body-parser
		// does.
		s.log.Error("no doc lines provided",
			slog.String("projectId", projectID.Hex()), slog.String("docId", docID.Hex()))
		sendStatus(w, http.StatusBadRequest)
		return
	}

	lines, ok := stringArray(doc, "lines")
	if !ok {
		s.log.Error("no doc lines provided",
			slog.String("projectId", projectID.Hex()), slog.String("docId", docID.Hex()))
		sendStatus(w, http.StatusBadRequest)
		return
	}
	version, ok := numberValue(doc, "version")
	if !ok {
		s.log.Error("no doc version provided",
			slog.String("projectId", projectID.Hex()), slog.String("docId", docID.Hex()))
		sendStatus(w, http.StatusBadRequest)
		return
	}
	ranges, present := docGet(doc, "ranges")
	if !present || ranges == nil {
		s.log.Error("no doc ranges provided",
			slog.String("projectId", projectID.Hex()), slog.String("docId", docID.Hex()))
		sendStatus(w, http.StatusBadRequest)
		return
	}

	// The limit counts UTF-16 code units, which is what String.length gives
	// the Node implementation.
	bodyLength := 0
	for _, line := range lines {
		bodyLength += len(utf16.Encode([]rune(line)))
	}
	if bodyLength > s.cfg.MaxDocLength {
		s.log.Error("document body too large",
			slog.String("projectId", projectID.Hex()),
			slog.String("docId", docID.Hex()),
			slog.Int("bodyLength", bodyLength))
		sendText(w, http.StatusRequestEntityTooLarge, "document body too large")
		return
	}

	result, err := s.manager.UpdateDoc(r.Context(), projectID, docID, lines, int64(version), ranges)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	sendJSON(w, http.StatusOK, result)
}

func (s *Server) patchDoc(w http.ResponseWriter, r *http.Request) {
	projectID, docID, ok := s.projectAndDoc(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	meta, ok := parsePatchBody(body)
	if !ok {
		// celebrate rejects the request before the controller runs.
		sendStatus(w, http.StatusBadRequest)
		return
	}
	if err := s.manager.PatchDoc(r.Context(), projectID, docID, meta); err != nil {
		s.fail(w, r, err)
		return
	}
	sendStatus(w, http.StatusNoContent)
}

// parsePatchBody applies the Joi schema on the PATCH body: only deleted, name
// and deletedAt are allowed, name and deletedAt are required when deleted is
// true, and deletedAt is coerced to a date.
func parsePatchBody(body []byte) (bson.D, bool) {
	doc, err := bsonjson.DecodeDocument(body)
	if err != nil {
		return nil, false
	}

	meta := bson.D{}
	deleted := false
	var hasName, hasDeletedAt bool

	for _, e := range doc {
		switch e.Key {
		case "deleted":
			v, ok := e.Value.(bool)
			if !ok {
				return nil, false
			}
			deleted = v
			meta = append(meta, bson.E{Key: "deleted", Value: v})
		case "name":
			v, ok := e.Value.(string)
			if !ok {
				return nil, false
			}
			hasName = true
			meta = append(meta, bson.E{Key: "name", Value: v})
		case "deletedAt":
			v, ok := e.Value.(string)
			if !ok {
				return nil, false
			}
			ts, converted := toDate(v)
			if !converted {
				return nil, false
			}
			hasDeletedAt = true
			meta = append(meta, bson.E{Key: "deletedAt", Value: ts})
		default:
			// Joi objects reject unknown keys by default.
			return nil, false
		}
	}

	if deleted && (!hasName || !hasDeletedAt) {
		return nil, false
	}
	return meta, true
}

func (s *Server) archiveAllDocs(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.projectParam(w, r)
	if !ok {
		return
	}
	if err := s.manager.ArchiveAllDocs(r.Context(), projectID); err != nil {
		s.fail(w, r, err)
		return
	}
	sendStatus(w, http.StatusNoContent)
}

func (s *Server) archiveDoc(w http.ResponseWriter, r *http.Request) {
	projectID, docID, ok := s.projectAndDoc(w, r)
	if !ok {
		return
	}
	if err := s.manager.ArchiveDoc(r.Context(), projectID, docID); err != nil {
		s.fail(w, r, err)
		return
	}
	sendStatus(w, http.StatusNoContent)
}

func (s *Server) unArchiveAllDocs(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.projectParam(w, r)
	if !ok {
		return
	}
	if err := s.manager.UnArchiveAllDocs(r.Context(), projectID); err != nil {
		if errors.Is(err, ErrDocRevValue) {
			s.log.Warn("Failed to unarchive doc", slog.String("err", err.Error()))
			sendStatus(w, http.StatusConflict)
			return
		}
		s.fail(w, r, err)
		return
	}
	sendStatus(w, http.StatusOK)
}

func (s *Server) destroyProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.projectParam(w, r)
	if !ok {
		return
	}
	if err := s.manager.DestroyProject(r.Context(), projectID); err != nil {
		s.fail(w, r, err)
		return
	}
	sendStatus(w, http.StatusNoContent)
}

// healthCheck writes a doc through the service's own HTTP API, reads it back,
// and removes it again. Going over HTTP is what makes it a check of the whole
// service rather than of Mongo alone.
func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	docID := bson.NewObjectID()
	projectID := bson.NewObjectID()
	if s.cfg.HealthCheckProject != "" {
		if id, err := bson.ObjectIDFromHex(s.cfg.HealthCheckProject); err == nil {
			projectID = id
		}
	}
	lines := []string{"smoke test - delete me", bson.NewObjectID().Hex() + bson.NewObjectID().Hex()}

	err := s.runHealthCheck(r.Context(), projectID, docID, lines)

	// The cleanup runs whatever happened, so a failed check leaves nothing
	// behind for the next one to trip over.
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Second)
	defer cancel()
	if _, delErr := s.store.docs.DeleteOne(cleanupCtx,
		bson.D{{Key: "_id", Value: docID}, {Key: "project_id", Value: projectID}}); delErr != nil {
		s.log.Error("error cleaning up health check doc", slog.String("err", delErr.Error()))
	}

	if err != nil {
		s.log.Error("error performing health check", slog.String("err", err.Error()))
		sendStatus(w, http.StatusInternalServerError)
		return
	}
	sendStatus(w, http.StatusOK)
}

func (s *Server) runHealthCheck(ctx context.Context, projectID, docID bson.ObjectID, lines []string) error {
	url := fmt.Sprintf("%s/project/%s/doc/%s", s.cfg.SelfURL, projectID.Hex(), docID.Hex())

	payload, err := json.Marshal(map[string]any{
		"lines": lines, "version": 42, "ranges": map[string]any{},
	})
	if err != nil {
		return err
	}
	if err := s.call(ctx, http.MethodPost, url, payload, nil); err != nil {
		return err
	}

	var got struct {
		Lines []string `json:"lines"`
	}
	if err := s.call(ctx, http.MethodGet, url, nil, &got); err != nil {
		return err
	}
	if !equalLines(got.Lines, lines) {
		return fmt.Errorf("health check lines not equal: got %v, want %v", got.Lines, lines)
	}
	return nil
}

func (s *Server) call(ctx context.Context, method, url string, body []byte, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("%s %s: non-2xx status code %d", method, url, res.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// --- body helpers -------------------------------------------------------

func stringArray(doc bson.D, key string) ([]string, bool) {
	raw, present := docGet(doc, key)
	if !present {
		return nil, false
	}
	arr, ok := raw.(bson.A)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		s, ok := item.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

func numberValue(doc bson.D, key string) (float64, bool) {
	raw, present := docGet(doc, key)
	if !present {
		return 0, false
	}
	return asNumber(raw)
}

var _ = strings.TrimSpace
