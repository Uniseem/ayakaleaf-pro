package historystore

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/histmodel"
)

// The history store's HTTP surface.
//
// Small and old: these are the addresses project-history has called since
// before this rewrite, and they are kept as they were because the caller is
// not being rewritten to match. Everything is behind one shared password on a
// loopback port -- this service holds every project's contents and has no idea
// who anybody is, so it is never exposed.

// Server serves the history store.
type Server struct {
	store    *Store
	log      *slog.Logger
	user     string
	password string
	// maxBlob is the largest file this will take in one request.
	maxBlob int64
}

// NewServer builds one.
func NewServer(store *Store, log *slog.Logger, user, password string, maxBlob int64) *Server {
	return &Server{store: store, log: log, user: user, password: password, maxBlob: maxBlob}
}

// Handler is the whole API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/projects", s.initialise)
	mux.HandleFunc("GET /api/projects/{id}/latest/history", s.latestChunk)
	mux.HandleFunc("GET /api/projects/{id}/latest/history/raw", s.latestVersion)
	mux.HandleFunc("GET /api/projects/{id}/versions/{version}/history", s.chunkAtVersion)
	mux.HandleFunc("GET /api/projects/{id}/versions/{version}/content", s.contentAtVersion)
	mux.HandleFunc("POST /api/projects/{id}/legacy_changes", s.appendChanges)
	mux.HandleFunc("POST /api/projects/{id}/changes", s.appendChanges)
	mux.HandleFunc("GET /api/projects/{id}/blobs/{hash}", s.getBlob)
	mux.HandleFunc("HEAD /api/projects/{id}/blobs/{hash}", s.headBlob)
	mux.HandleFunc("PUT /api/projects/{id}/blobs/{hash}", s.putBlob)
	mux.HandleFunc("POST /api/projects/{id}/clone", s.clone)

	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "history-v1 is alive\n")
	})
	mux.HandleFunc("GET /health_check", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return s.authenticated(mux)
}

func (s *Server) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		user, password, ok := r.BasicAuth()
		if !ok ||
			subtle.ConstantTimeCompare([]byte(user), []byte(s.user)) != 1 ||
			subtle.ConstantTimeCompare([]byte(password), []byte(s.password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="history"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// initialise gives a project a history.
func (s *Server) initialise(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProjectID json.RawMessage `json:"projectId"`
	}
	if r.ContentLength > 0 {
		// The body is sometimes the literal true, which is the caller saying
		// "any id will do". It has never done that here: an id is always sent.
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body)
	}
	historyID := scalar(body.ProjectID)
	if historyID == "" {
		s.refuse(w, r, ErrBadRequest)
		return
	}
	if err := s.store.InitialiseProject(r.Context(), historyID); err != nil {
		s.refuse(w, r, err)
		return
	}
	answer(w, http.StatusOK, map[string]any{"projectId": historyID})
}

// latestChunk is the end of a project's history.
func (s *Server) latestChunk(w http.ResponseWriter, r *http.Request) {
	chunk, err := s.store.LatestChunk(r.Context(), r.PathValue("id"))
	if err != nil {
		s.refuse(w, r, err)
		return
	}
	answer(w, http.StatusOK, map[string]any{"chunk": chunk})
}

// latestVersion is how far the history goes, without the changes.
func (s *Server) latestVersion(w http.ResponseWriter, r *http.Request) {
	version, err := s.store.LatestVersion(r.Context(), r.PathValue("id"))
	if err != nil {
		s.refuse(w, r, err)
		return
	}
	answer(w, http.StatusOK, version)
}

// chunkAtVersion is the run of history a version falls in.
func (s *Server) chunkAtVersion(w http.ResponseWriter, r *http.Request) {
	version, err := strconv.Atoi(r.PathValue("version"))
	if err != nil {
		s.refuse(w, r, ErrBadRequest)
		return
	}
	chunk, err := s.store.ChunkAtVersion(r.Context(), r.PathValue("id"), version)
	if err != nil {
		s.refuse(w, r, err)
		return
	}
	answer(w, http.StatusOK, map[string]any{"chunk": chunk})
}

// contentAtVersion is the project as it was, files and all.
func (s *Server) contentAtVersion(w http.ResponseWriter, r *http.Request) {
	version, err := strconv.Atoi(r.PathValue("version"))
	if err != nil {
		s.refuse(w, r, ErrBadRequest)
		return
	}
	chunk, err := s.store.ChunkAtVersion(r.Context(), r.PathValue("id"), version)
	if err != nil {
		s.refuse(w, r, err)
		return
	}
	snapshot, err := chunk.GetSnapshotAt(version)
	if err != nil {
		s.refuse(w, r, err)
		return
	}
	answer(w, http.StatusOK, snapshot)
}

// appendChanges adds to the end of a project's history.
func (s *Server) appendChanges(w http.ResponseWriter, r *http.Request) {
	endVersion, err := strconv.Atoi(r.URL.Query().Get("end_version"))
	if err != nil {
		s.refuse(w, r, ErrBadRequest)
		return
	}
	var changes []*histmodel.Change
	if err := json.NewDecoder(io.LimitReader(r.Body, 256<<20)).Decode(&changes); err != nil {
		s.refuse(w, r, ErrBadRequest)
		return
	}
	err = s.store.AppendChanges(r.Context(), r.PathValue("id"), changes, endVersion)
	if err != nil {
		s.refuse(w, r, err)
		return
	}
	// An empty answer means the changes were taken and no resync is needed,
	// which is what the caller reads as success.
	w.WriteHeader(http.StatusCreated)
}

// getBlob hands over a file's bytes.
func (s *Server) getBlob(w http.ResponseWriter, r *http.Request) {
	body, blob, err := s.store.ReadBlob(r.Context(), r.PathValue("id"), r.PathValue("hash"))
	if err != nil {
		s.refuse(w, r, err)
		return
	}
	defer func() { _ = body.Close() }()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(blob.ByteLength, 10))
	// A blob never changes: it is named by the hash of what is in it.
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
}

// headBlob says whether a blob is there and how big it is.
func (s *Server) headBlob(w http.ResponseWriter, r *http.Request) {
	blob, err := s.store.FindBlob(r.Context(), r.PathValue("id"), r.PathValue("hash"))
	if err != nil {
		s.refuse(w, r, err)
		return
	}
	w.Header().Set("Content-Length", strconv.FormatInt(blob.ByteLength, 10))
	w.WriteHeader(http.StatusOK)
}

// putBlob stores a file's bytes under their own hash.
func (s *Server) putBlob(w http.ResponseWriter, r *http.Request) {
	content, err := io.ReadAll(io.LimitReader(r.Body, s.maxBlob+1))
	if err != nil {
		s.refuse(w, r, err)
		return
	}
	if int64(len(content)) > s.maxBlob {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}

	// Whether the history should hold this as text decides what a later reader
	// can do with it: a text blob can be part of a snapshot and diffed, a
	// binary one can only be fetched.
	blob, err := s.store.PutBlob(r.Context(), r.PathValue("id"), r.PathValue("hash"),
		content, isText(content))
	if err != nil {
		s.refuse(w, r, err)
		return
	}
	answer(w, http.StatusCreated, blob)
}

// clone gives one project another's history.
func (s *Server) clone(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NewProjectID json.RawMessage `json:"newProjectId"`
	}
	if r.ContentLength > 0 {
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body)
	}
	target := scalar(body.NewProjectID)
	if target == "" {
		s.refuse(w, r, ErrBadRequest)
		return
	}
	if err := s.store.CopyHistory(r.Context(), r.PathValue("id"), target); err != nil {
		s.refuse(w, r, err)
		return
	}
	answer(w, http.StatusOK, map[string]any{"projectId": target})
}

// refuse turns a failure into the status the caller acts on.
//
// The conflict matters most: it is not an error but a race, and the caller
// answers it by reading the history again and retrying.
func (s *Server) refuse(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		w.WriteHeader(http.StatusNotFound)
	case errors.Is(err, ErrConflict):
		w.WriteHeader(http.StatusConflict)
	case errors.Is(err, ErrBadRequest):
		w.WriteHeader(http.StatusBadRequest)
	default:
		if s.log != nil {
			s.log.Error("a history request failed",
				slog.String("path", r.URL.Path), slog.Any("err", err))
		}
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func answer(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// scalar reads an id that may arrive as a string or as a number.
func scalar(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var number float64
	if json.Unmarshal(raw, &number) == nil {
		return strconv.FormatInt(int64(number), 10)
	}
	return ""
}

// isText says whether the history should hold these bytes as text.
func isText(content []byte) bool {
	if len(content) > 2<<20 {
		// Beyond a certain size the history does not hold text either: a
		// snapshot has to fit in memory to be worked with.
		return false
	}
	for _, b := range content {
		if b == 0 {
			return false
		}
	}
	return validUTF8(content)
}
