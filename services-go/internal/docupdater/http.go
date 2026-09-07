package docupdater

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
	history *HistoryClient
	log     *slog.Logger

	// maxDocLength is the size a document may not exceed, in characters. A
	// write larger than this is refused before anything is loaded.
	maxDocLength int
}

// maxRequestBody bounds a request body this service parses. Nothing it accepts
// is large; the document content arrives on the update queue, not here.
const maxRequestBody = 8 << 20

// NewServer builds the HTTP server.
func NewServer(docs *DocumentManager, project *ProjectManager, redis *RedisStore,
	history *HistoryClient, maxDocLength int, log *slog.Logger) *Server {
	return &Server{
		docs: docs, project: project, redis: redis, history: history,
		maxDocLength: maxDocLength, log: log,
	}
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
	mux.HandleFunc("POST /project/{project_id}/get_and_flush_if_old", s.getProjectDocsAndFlushIfOld)
	mux.HandleFunc("GET /project/{project_id}/doc", s.getProjectDocsAndFlushIfOld)

	mux.HandleFunc("POST /project/{project_id}/doc/{doc_id}", s.setDoc)
	mux.HandleFunc("POST /project/{project_id}/doc/{doc_id}/append", s.appendToDoc)
	mux.HandleFunc("POST /project/{project_id}/doc/{doc_id}/flush", s.flushDocIfLoaded)
	mux.HandleFunc("DELETE /project/{project_id}/doc/{doc_id}", s.deleteDoc)
	mux.HandleFunc("POST /project/{project_id}/flush", s.flushProject)
	mux.HandleFunc("DELETE /project/{project_id}", s.deleteProject)
	mux.HandleFunc("DELETE /project", s.deleteMultipleProjects)
	mux.HandleFunc("GET /project/{project_id}/doc/{doc_id}/comment/{comment_id}", s.getComment)
	mux.HandleFunc("POST /project/{project_id}/doc/{doc_id}/change/{change_id}/accept", s.acceptChanges)
	mux.HandleFunc("POST /project/{project_id}/doc/{doc_id}/change/accept", s.acceptChanges)
	mux.HandleFunc("POST /project/{project_id}/doc/{doc_id}/change/reject", s.rejectChanges)
	mux.HandleFunc("POST /project/{project_id}/doc/{doc_id}/comment/{comment_id}/resolve", s.resolveComment)
	mux.HandleFunc("POST /project/{project_id}/doc/{doc_id}/comment/{comment_id}/reopen", s.reopenComment)
	mux.HandleFunc("DELETE /project/{project_id}/doc/{doc_id}/comment/{comment_id}", s.deleteComment)

	mux.HandleFunc("POST /project/{project_id}", s.updateProject)
	mux.HandleFunc("POST /project/{project_id}/history/resync", s.resyncProjectHistory)
	mux.HandleFunc("GET /flush_queued_projects", s.flushQueuedProjects)
	mux.HandleFunc("GET /total", s.total)

	mux.HandleFunc("POST /project/{project_id}/block", s.blockProject)
	mux.HandleFunc("POST /project/{project_id}/unblock", s.unblockProject)

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

// setDocBody is what a write through the API carries.
type setDocBody struct {
	Lines []string `json:"lines"`
	// Source is either a name or an object; it is passed on as it arrived.
	Source  json.RawMessage `json:"source"`
	UserID  string          `json:"user_id"`
	Undoing bool            `json:"undoing"`
}

func (s *Server) setDoc(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")
	docID := r.PathValue("doc_id")

	var body setDocBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&body); err != nil {
		writeBodyError(w, err)
		return
	}
	// Checked before the document is loaded: a document this size will be
	// refused whatever is in Redis, and loading it first would be work for
	// nothing.
	if totalSizeOfLines(body.Lines) > s.maxDocLength {
		s.log.Warn("document too large, refusing to set it",
			slog.String("project", projectID), slog.String("doc", docID),
			slog.Int("size", totalSizeOfLines(body.Lines)))
		w.WriteHeader(http.StatusNotAcceptable)
		_, _ = w.Write([]byte("Not Acceptable"))
		return
	}

	result, err := s.docs.SetDocWithLock(r.Context(), projectID, docID, body.Lines,
		SetDocOptions{
			OriginOrSource: body.Source, UserID: body.UserID,
			Undoing: body.Undoing, External: true,
		})
	if err != nil {
		s.writeError(w, r, err, "setDoc")
		return
	}
	// A write that changed nothing flushes nothing, so there is no answer from
	// web to pass on. An empty object goes back rather than nothing at all,
	// which would not be valid JSON.
	if len(result) == 0 {
		result = json.RawMessage("{}")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(result)
}

func (s *Server) appendToDoc(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")
	docID := r.PathValue("doc_id")

	var body setDocBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&body); err != nil {
		writeBodyError(w, err)
		return
	}

	result, err := s.docs.AppendToDocWithLock(r.Context(), projectID, docID, body.Lines,
		SetDocOptions{OriginOrSource: body.Source, UserID: body.UserID})
	if err != nil {
		if errors.Is(err, ErrFileTooLarge) {
			// Not the 413 a write gets: appending is refused because of what
			// the document would become, not because of what was sent.
			s.log.Warn("refusing to append to file, it would become too large",
				slog.String("project", projectID), slog.String("doc", docID))
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte("Unprocessable Entity"))
			return
		}
		s.writeError(w, r, err, "appendToDoc")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if len(result) == 0 {
		// res.json(undefined) sends an empty body in Express, which is what
		// this reproduces.
		return
	}
	_, _ = w.Write(result)
}

// changesBody is what the change and comment routes carry.
type changesBody struct {
	ChangeIDs []string `json:"change_ids"`
	UserID    string   `json:"user_id"`
}

// readChangesBody decodes the body, tolerating an empty one: the accept route
// with a change id in the path is called without one.
func readChangesBody(w http.ResponseWriter, r *http.Request) (changesBody, bool) {
	var body changesBody
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&body)
	if err != nil && !errors.Is(err, io.EOF) {
		writeBodyError(w, err)
		return body, false
	}
	return body, true
}

func (s *Server) acceptChanges(w http.ResponseWriter, r *http.Request) {
	body, ok := readChangesBody(w, r)
	if !ok {
		return
	}
	// One change in the path, or a list in the body.
	changeIDs := body.ChangeIDs
	if changeIDs == nil {
		changeIDs = []string{r.PathValue("change_id")}
	}

	authors, err := s.docs.AcceptChangesWithLock(r.Context(),
		r.PathValue("project_id"), r.PathValue("doc_id"), changeIDs)
	if err != nil {
		s.writeError(w, r, err, "acceptChanges")
		return
	}
	if authors == nil {
		authors = []string{}
	}
	writeJSON(w, map[string]any{"changeContributors": authors})
}

func (s *Server) rejectChanges(w http.ResponseWriter, r *http.Request) {
	body, ok := readChangesBody(w, r)
	if !ok {
		return
	}
	rejected, err := s.docs.RejectChangesWithLock(r.Context(),
		r.PathValue("project_id"), r.PathValue("doc_id"), body.ChangeIDs, body.UserID)
	if err != nil {
		s.writeError(w, r, err, "rejectChanges")
		return
	}
	writeJSON(w, map[string]any{"rejectedChangeIds": rejected})
}

func (s *Server) getComment(w http.ResponseWriter, r *http.Request) {
	comment, err := s.docs.GetCommentWithLock(r.Context(), r.PathValue("project_id"),
		r.PathValue("doc_id"), r.PathValue("comment_id"))
	if err != nil {
		s.writeError(w, r, err, "getComment")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(comment)
}

func (s *Server) resolveComment(w http.ResponseWriter, r *http.Request) {
	s.updateCommentState(w, r, true)
}

func (s *Server) reopenComment(w http.ResponseWriter, r *http.Request) {
	s.updateCommentState(w, r, false)
}

func (s *Server) updateCommentState(w http.ResponseWriter, r *http.Request, resolved bool) {
	body, ok := readChangesBody(w, r)
	if !ok {
		return
	}
	err := s.docs.UpdateCommentStateWithLock(r.Context(), r.PathValue("project_id"),
		r.PathValue("doc_id"), r.PathValue("comment_id"), body.UserID, resolved)
	if err != nil {
		s.writeError(w, r, err, "updateCommentState")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteComment(w http.ResponseWriter, r *http.Request) {
	if _, ok := readChangesBody(w, r); !ok {
		return
	}
	err := s.docs.DeleteCommentWithLock(r.Context(), r.PathValue("project_id"),
		r.PathValue("doc_id"), r.PathValue("comment_id"))
	if err != nil {
		s.writeError(w, r, err, "deleteComment")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) flushDocIfLoaded(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")
	docID := r.PathValue("doc_id")

	if err := s.docs.FlushDocIfLoadedWithLock(r.Context(), projectID, docID); err != nil {
		s.writeError(w, r, err, "flushDocIfLoaded")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteDoc(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")
	docID := r.PathValue("doc_id")
	ignoreFlushErrors := r.URL.Query().Get("ignore_flush_errors") == "true"

	err := s.docs.FlushAndDeleteDocWithLock(r.Context(), projectID, docID, ignoreFlushErrors)
	// The history queue is flushed either way: a document that failed to write
	// back still has operations queued, and sometimes the failure is what makes
	// the flush necessary.
	if s.history != nil {
		s.history.FlushProjectChangesAsync(r.Context(), projectID)
	}
	if err != nil {
		s.writeError(w, r, err, "deleteDoc")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) flushProject(w http.ResponseWriter, r *http.Request) {
	if err := s.project.FlushProjectWithLocks(r.Context(), r.PathValue("project_id")); err != nil {
		s.writeError(w, r, err, "flushProject")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")
	query := r.URL.Query()
	// Express treats any value as present, so "?background=" counts. The two
	// callers pass "true", but matching the check keeps a stray empty value
	// from taking the other branch here than it does there.
	background := query.Has("background") && query.Get("background") != "false"
	shutdown := query.Has("shutdown") && query.Get("shutdown") != "false"

	if background {
		if err := s.project.QueueFlushAndDeleteProject(r.Context(), projectID); err != nil {
			s.writeError(w, r, err, "deleteProject")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// real-time shutting down skips the history flush: the queue is in Redis
	// and whoever opens the project next will drain it.
	opts := FlushOptions{Background: background, SkipHistoryFlush: shutdown}
	if err := s.project.FlushAndDeleteProjectWithLocks(r.Context(), projectID, opts); err != nil {
		s.writeError(w, r, err, "deleteProject")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteMultipleProjects(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProjectIDs []string `json:"project_ids"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&body); err != nil {
		writeBodyError(w, err)
		return
	}
	for _, projectID := range body.ProjectIDs {
		if err := s.project.QueueFlushAndDeleteProject(r.Context(), projectID); err != nil {
			s.writeError(w, r, err, "deleteMultipleProjects")
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getProjectDocsAndFlushIfOld(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")
	// The caller sends the hash it believes describes the project structure.
	//
	// It also sends an "exclude" list of documents it already has. The Node
	// service parses it and then does nothing with it, so neither does this.
	stateHash := r.URL.Query().Get("state")

	docs, err := s.project.GetProjectDocsAndFlushIfOld(r.Context(), projectID, stateHash)
	if err != nil {
		s.writeError(w, r, err, "getProjectDocsAndFlushIfOld")
		return
	}
	writeJSON(w, docs)
}

func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProjectHistoryID string                   `json:"projectHistoryId"`
		UserID           string                   `json:"userId"`
		Updates          []ProjectStructureUpdate `json:"updates"`
		Version          int64                    `json:"version"`
		Source           json.RawMessage          `json:"source"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&body); err != nil {
		writeBodyError(w, err)
		return
	}

	err := s.project.UpdateProjectWithLocks(r.Context(), r.PathValue("project_id"),
		body.ProjectHistoryID, body.UserID, body.Updates, body.Version, body.Source)
	if err != nil {
		s.writeError(w, r, err, "updateProject")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) resyncProjectHistory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProjectHistoryID           string            `json:"projectHistoryId"`
		Docs                       []json.RawMessage `json:"docs"`
		Files                      []json.RawMessage `json:"files"`
		HistoryRangesMigration     string            `json:"historyRangesMigration"`
		ResyncProjectStructureOnly bool              `json:"resyncProjectStructureOnly"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&body); err != nil {
		writeBodyError(w, err)
		return
	}

	err := s.project.ResyncProjectHistory(r.Context(), r.PathValue("project_id"),
		body.ProjectHistoryID, body.Docs, body.Files, ResyncOptions{
			StructureOnly:          body.ResyncProjectStructureOnly,
			HistoryRangesMigration: body.HistoryRangesMigration,
		})
	if err != nil {
		s.writeError(w, r, err, "resyncProjectHistory")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) flushQueuedProjects(w http.ResponseWriter, r *http.Request) {
	opts := DeleteQueueOptions{
		Limit:        1000,
		Timeout:      5 * time.Minute,
		MinDeleteAge: 5 * time.Minute,
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			opts.Limit = parsed
		}
	}
	if raw := r.URL.Query().Get("min_delete_age"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			opts.MinDeleteAge = time.Duration(parsed) * time.Millisecond
		}
	}

	flushed, err := s.project.FlushAndDeleteOldProjects(r.Context(), opts)
	if err != nil {
		s.log.Error("error flushing old projects", slog.String("err", err.Error()))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	s.log.Info("flush of queued projects completed", slog.Int("flushed", flushed))
	writeJSON(w, map[string]any{"flushed": flushed})
}

// total answers with how many documents are held in Redis.
//
// The Node service has this route too, but it calls a function that is not
// there any more, so it answers with a 500. It is an operator route that
// nothing else calls, so this one is made to work rather than made to fail the
// same way.
func (s *Server) total(w http.ResponseWriter, r *http.Request) {
	count, err := s.redis.CountDocsInMemory(r.Context())
	if err != nil {
		s.writeError(w, r, err, "total")
		return
	}
	writeJSON(w, map[string]any{"total": count})
}

func (s *Server) blockProject(w http.ResponseWriter, r *http.Request) {
	blocked, err := s.redis.BlockProject(r.Context(), r.PathValue("project_id"))
	if err != nil {
		s.writeError(w, r, err, "blockProject")
		return
	}
	writeJSON(w, map[string]any{"blocked": blocked})
}

func (s *Server) unblockProject(w http.ResponseWriter, r *http.Request) {
	wasBlocked, err := s.redis.UnblockProject(r.Context(), r.PathValue("project_id"))
	if err != nil {
		s.writeError(w, r, err, "unblockProject")
		return
	}
	writeJSON(w, map[string]any{"wasBlocked": wasBlocked})
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
	case errors.Is(err, ErrDocumentValidation):
		// web answered, but with a document missing the fields that make it
		// usable. That is not this service failing, so it is not a 500.
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, ErrHistoryRangesNotSupported):
		http.Error(w, err.Error(), http.StatusNotImplemented)
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

// writeBodyError answers a request whose body could not be read.
//
// A body over the limit is reported as too large rather than as malformed,
// which is what the caller acts on: it means send less, not send it again.
func writeBodyError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		http.Error(w, "request entity too large", http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, "invalid request body", http.StatusBadRequest)
}
