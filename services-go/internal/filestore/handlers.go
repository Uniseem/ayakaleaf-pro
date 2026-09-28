package filestore

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/obsv"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/persistor"
)

// Server is the filestore HTTP server.
type Server struct {
	handler *Handler
	stores  Stores
	log     *slog.Logger
	metrics *obsv.Metrics
	// shuttingDown makes /status report unavailable so a load balancer stops
	// sending new work before the process goes away.
	shuttingDown func() bool
}

// NewServer builds the filestore HTTP server.
func NewServer(h *Handler, stores Stores, log *slog.Logger, metrics *obsv.Metrics, shuttingDown func() bool) *Server {
	return &Server{handler: h, stores: stores, log: log, metrics: metrics, shuttingDown: shuttingDown}
}

// Handler returns the fully wired filestore router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	route := func(method, pattern, label string, h http.HandlerFunc) {
		mux.Handle(method+" "+pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h(w, obsv.WithRoutePath(r, label))
		}))
	}

	// The template routes only exist when a template store is configured,
	// matching app.js's `if (settings.filestore.stores.template_files)`.
	if s.stores.TemplateFiles != "" {
		const base = "/template/{template_id}/v/{version}/{format}"
		const label = "template_template_id_v_version_format"
		route("HEAD", base, label, s.getTemplateHead)
		route("GET", base, label, s.getTemplate)
		route("GET", base+"/{sub_type}", label+"_sub_type", s.getTemplate)
		route("POST", base, label, s.insertTemplate)
		route("DELETE", base, label, s.deleteTemplate)
	}

	route("GET", "/bucket/{bucket}/key/{key...}", "bucket_bucket_key_0", s.getBucketFile)
	route("GET", "/history/global/hash/{hash}", "history_global_hash_hash", s.getGlobalBlob)
	route("GET", "/history/project/{historyId}/hash/{hash}", "history_project_historyId_hash_hash", s.getProjectBlob)

	route("GET", "/status", "status", s.status)
	route("GET", "/health_check", "health_check", s.status)
	route("GET", "/metrics", "metrics", func(w http.ResponseWriter, r *http.Request) {
		s.metrics.Handler().ServeHTTP(w, r)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	return s.metrics.HTTPMiddleware(s.log, mux)
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	if s.shuttingDown != nil && s.shuttingDown() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "filestore sharelatex up")
}

// --- targets ------------------------------------------------------------

func (s *Server) getTemplate(w http.ResponseWriter, r *http.Request) {
	s.serve(w, r, TemplateTarget(s.stores,
		r.PathValue("template_id"), r.PathValue("version"),
		r.PathValue("format"), r.PathValue("sub_type")))
}

func (s *Server) getTemplateHead(w http.ResponseWriter, r *http.Request) {
	t := TemplateTarget(s.stores, r.PathValue("template_id"), r.PathValue("version"),
		r.PathValue("format"), "")
	s.metrics.Inc("getFileSize")
	size, err := s.handler.GetFileSize(r.Context(), t)
	if err != nil {
		s.failGet(w, r, err)
		return
	}
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.WriteHeader(http.StatusOK)
}

func (s *Server) insertTemplate(w http.ResponseWriter, r *http.Request) {
	t := TemplateTarget(s.stores, r.PathValue("template_id"), r.PathValue("version"),
		r.PathValue("format"), "")
	s.metrics.Inc("insertFile")
	if err := s.handler.InsertFile(r.Context(), t, r.Body); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	t := TemplateTarget(s.stores, r.PathValue("template_id"), r.PathValue("version"),
		r.PathValue("format"), "")
	s.metrics.Inc("deleteFile")
	if err := s.handler.DeleteFile(r.Context(), t); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getBucketFile(w http.ResponseWriter, r *http.Request) {
	s.serve(w, r, BucketTarget(r.PathValue("bucket"), r.PathValue("key")))
}

func (s *Server) getGlobalBlob(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	if !ValidHash(hash) {
		// A malformed hash is spliced straight into a storage key, so it is
		// rejected rather than looked up.
		w.WriteHeader(http.StatusNotFound)
		return
	}
	s.serve(w, r, GlobalBlobTarget(s.stores, hash))
}

func (s *Server) getProjectBlob(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	if !ValidHash(hash) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	t, err := ProjectBlobTarget(s.stores, r.PathValue("historyId"), hash)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	s.serve(w, r, t)
}

// --- serving ------------------------------------------------------------

func (s *Server) serve(w http.ResponseWriter, r *http.Request, t Target) {
	q := r.URL.Query()
	o := GetOptions{Format: q.Get("format"), Style: q.Get("style")}
	if start, end, ok := parseRange(r.Header.Get("Range")); ok {
		o.Start, o.End, o.HasRange = start, end, true
	}
	s.metrics.Inc("getFile")

	if url, err := s.handler.RedirectURL(r.Context(), t, o); err != nil {
		s.metrics.Inc("file_redirect_error")
	} else if url != "" {
		s.metrics.Inc("file_redirect")
		http.Redirect(w, r, url, http.StatusFound)
		return
	}

	body, err := s.handler.GetFile(r.Context(), t, o)
	if err != nil {
		s.failGet(w, r, err)
		return
	}
	defer func() { _ = body.Close() }()

	// cacheWarm asks only that the conversion be produced and cached; the
	// bytes are not wanted.
	if q.Get("cacheWarm") != "" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if _, err := io.Copy(w, body); err != nil {
		// The status line is already sent, so there is nothing to report but
		// the log line.
		s.log.Info("error transferring stream",
			slog.String("bucket", t.Bucket), slog.String("key", t.Key),
			slog.String("err", err.Error()))
	}
}

// failGet maps a read failure onto a status: a missing object is a 404,
// anything else a 500.
func (s *Server) failGet(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, persistor.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	s.fail(w, r, err)
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrInvalidParameters):
		status = http.StatusBadRequest
	case errors.Is(err, ErrConversionsDisabled):
		status = http.StatusUnprocessableEntity
	case errors.Is(err, ErrConversion):
		status = http.StatusUnprocessableEntity
	}
	s.log.Error("filestore request failed",
		slog.String("url", r.URL.RequestURI()), slog.String("err", err.Error()))
	w.WriteHeader(status)
}

// parseRange reads a single "bytes=start-end" range. Anything else -- multiple
// ranges, or a suffix range -- is ignored, which makes the whole object be
// served, exactly as range-parser's result is used in FileController.
func parseRange(header string) (start, end int64, ok bool) {
	const prefix = "bytes="
	if !strings.HasPrefix(header, prefix) {
		return 0, 0, false
	}
	spec := strings.TrimPrefix(header, prefix)
	if strings.Contains(spec, ",") {
		return 0, 0, false
	}
	dash := strings.IndexByte(spec, '-')
	if dash <= 0 || dash == len(spec)-1 {
		return 0, 0, false
	}
	start, err := strconv.ParseInt(spec[:dash], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	end, err = strconv.ParseInt(spec[dash+1:], 10, 64)
	if err != nil || end < start {
		return 0, 0, false
	}
	return start, end, true
}
