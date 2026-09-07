package docupdater

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// Server is the HTTP surface of document-updater.
type Server struct {
	docs    *DocumentManager
	project *ProjectManager
	redis   *RedisStore
	log     *slog.Logger
}

// NewServer builds the HTTP server.
func NewServer(docs *DocumentManager, project *ProjectManager, redis *RedisStore, log *slog.Logger) *Server {
	return &Server{docs: docs, project: project, redis: redis, log: log}
}

// docResponse is what a GET of a document returns.
type docResponse struct {
	ID       string            `json:"id"`
	Lines    []string          `json:"lines"`
	Version  int64             `json:"version"`
	Ops      []json.RawMessage `json:"ops"`
	Ranges   json.RawMessage   `json:"ranges"`
	Pathname string            `json:"pathname"`
	// TTLInS tells the client how long operations are kept, so it can decide
	// between catching up and reloading.
	TTLInS int    `json:"ttlInS"`
	Type   string `json:"type"`
}

// peekResponse is the smaller answer for a peek, which does not load the
// document.
type peekResponse struct {
	ID      string   `json:"id"`
	Lines   []string `json:"lines"`
	Version int64    `json:"version"`
}

// Handler builds the routes.
func (s *Server) Handler(monitor func(http.Handler) http.Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("document updater is alive"))
	})
	mux.HandleFunc("GET /health_check", s.healthCheck)
	mux.HandleFunc("GET /health_check/redis", s.healthCheck)

	mux.HandleFunc("GET /project/{project_id}/doc/{doc_id}", s.getDoc)
	mux.HandleFunc("GET /project/{project_id}/doc/{doc_id}/peek", s.peekDoc)
	mux.HandleFunc("GET /project/{project_id}/ranges", s.getProjectRanges)
	mux.HandleFunc("GET /project/{project_id}/last_updated_at", s.getProjectLastUpdatedAt)
	mux.HandleFunc("POST /project/{project_id}/clearState", s.clearProjectState)

	if monitor != nil {
		return monitor(mux)
	}
	return mux
}

func (s *Server) getDoc(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")
	docID := r.PathValue("doc_id")

	fromVersion := int64(-1)
	if raw := r.URL.Query().Get("fromVersion"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			// parseInt of nonsense is NaN in the Node service, which then
			// asks Redis for a NaN range and fails there instead. Refusing it
			// here says the same thing sooner.
			http.Error(w, "fromVersion must be a number", http.StatusBadRequest)
			return
		}
		fromVersion = parsed
	}

	result, err := s.docs.GetDocAndRecentOpsWithLock(r.Context(), projectID, docID, fromVersion)
	if err != nil {
		s.writeError(w, r, err, "getDoc")
		return
	}
	if !result.Loaded() {
		s.writeError(w, r, ErrNotFound, "getDoc")
		return
	}
	// This port implements the ShareJS text type only. A history-ot document
	// reaching here would be served as though its content were lines, which it
	// is not.
	if result.Type() != TypeShareJSTextOT {
		s.writeError(w, r, &OTTypeMismatchError{Got: result.Type(), Want: TypeShareJSTextOT}, "getDoc")
		return
	}

	if r.URL.Query().Get("historyRanges") == "true" {
		// The history view needs the document as history stores it, with
		// tracked deletions put back in. That conversion belongs to
		// RangesManager and is not ported yet.
		http.Error(w, "historyRanges is not supported by this implementation",
			http.StatusNotImplemented)
		return
	}

	writeJSON(w, docResponse{
		ID:       docID,
		Lines:    result.Lines,
		Version:  result.Version,
		Ops:      result.Ops,
		Ranges:   result.Ranges,
		Pathname: result.Pathname,
		TTLInS:   int(docOpsTTL / time.Second),
		Type:     result.Type(),
	})
}

func (s *Server) peekDoc(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")
	docID := r.PathValue("doc_id")

	doc, err := s.docs.PeekDoc(r.Context(), projectID, docID)
	if err != nil {
		s.writeError(w, r, err, "peekDoc")
		return
	}
	if doc.Type() != TypeShareJSTextOT {
		s.writeError(w, r, &OTTypeMismatchError{Got: doc.Type(), Want: TypeShareJSTextOT}, "peekDoc")
		return
	}
	writeJSON(w, peekResponse{ID: docID, Lines: doc.Lines, Version: doc.Version})
}

func (s *Server) getProjectRanges(w http.ResponseWriter, r *http.Request) {
	docs, err := s.project.GetProjectRanges(r.Context(), r.PathValue("project_id"))
	if err != nil {
		s.writeError(w, r, err, "getProjectRanges")
		return
	}
	writeJSON(w, map[string]any{"docs": docs})
}

func (s *Server) getProjectLastUpdatedAt(w http.ResponseWriter, r *http.Request) {
	timestamps, err := s.project.GetProjectDocsTimestamps(r.Context(), r.PathValue("project_id"))
	if err != nil {
		s.writeError(w, r, err, "getProjectLastUpdatedAt")
		return
	}

	// Documents flushed between listing them and reading their timestamps come
	// back empty; the newest of what is left is the project's.
	var newest int64
	for _, ts := range timestamps {
		if ts == "" {
			continue
		}
		parsed, err := strconv.ParseInt(ts, 10, 64)
		if err != nil {
			continue
		}
		if parsed > newest {
			newest = parsed
		}
	}
	if newest == 0 {
		writeJSON(w, map[string]any{})
		return
	}
	writeJSON(w, map[string]any{"lastUpdatedAt": newest})
}

func (s *Server) clearProjectState(w http.ResponseWriter, r *http.Request) {
	if err := s.project.ClearProjectState(r.Context(), r.PathValue("project_id")); err != nil {
		s.writeError(w, r, err, "clearProjectState")
		return
	}
	// 200 with a body of "OK", which is what res.sendStatus(200) produces. web
	// does not read it, but the two services answering differently is the kind
	// of difference that turns up later as a puzzling log line.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.redis.redis.Ping(ctx).Err(); err != nil {
		s.log.Error("failed redis health check", slog.String("err", err.Error()))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// writeError maps an error to the status the Node service answers with.
func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error, method string) {
	var otErr *OTTypeMismatchError
	var opRange *OpRangeNotAvailable

	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "document not found", http.StatusNotFound)
	case errors.As(err, &opRange):
		// The client asked to resume from a version whose operations are gone.
		// The body carries the oldest version it could ask for instead, which
		// is how it decides between catching up and reloading.
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"firstVersionInRedis": opRange.FirstVersionInRedis,
			"version":             opRange.Version,
			"ttlInS":              opRange.TTLInS,
		})
	case errors.As(err, &otErr):
		http.Error(w, otErr.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, ErrFileTooLarge):
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
	case errors.Is(err, ErrProjectStateChanged):
		w.WriteHeader(http.StatusConflict)
	default:
		s.log.Error("error in "+method, slog.String("err", err.Error()),
			slog.String("path", r.URL.Path))
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
