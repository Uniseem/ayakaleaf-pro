package projecthistory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/histmodel"
)

// The service's own API. Almost all of it is the editor asking what a project
// used to look like, and one endpoint -- the flush -- is what everything else
// calls to make sure the answer is current before asking.
//
// Errors carry status codes rather than messages the caller has to read: a
// project that is not there is a 404, a version that does not exist is a 404, a
// resync that is already running is a 409, and everything else is a 500 with
// the reason logged rather than returned.

// Service is the whole of project-history.
type Service struct {
	redis      *RedisStore
	store      *Store
	web        *WebAPI
	history    *HistoryStore
	locks      *LockManager
	snapshots  *Snapshots
	sync       *SyncManager
	processor  *Processor
	translator *ChunkTranslator
	diffs      *DiffManager
	summarizer *Summarizer
	retries    *RetryManager
	flusher    *FlushManager
	logger     *slog.Logger

	// healthCheckProjectID is the project the health check flushes, which is
	// how the check exercises the whole path rather than only the process.
	healthCheckProjectID string
}

// ServiceConfig is everything the service needs to be built.
type ServiceConfig struct {
	Redis                *RedisStore
	Store                *Store
	Web                  *WebAPI
	History              *HistoryStore
	Locks                *LockManager
	Logger               *slog.Logger
	Metrics              ProcessorMetrics
	ShortHistoryQueues   []string
	HealthCheckProjectID string
}

// NewService wires the parts together.
func NewService(config ServiceConfig) *Service {
	snapshots := NewSnapshots(config.History, config.Web)
	sync := NewSyncManager(config.Store, config.Redis, config.Web,
		config.History, snapshots, config.Locks)
	processor := NewProcessor(config.Redis, config.Store, config.Web,
		config.History, sync, config.Locks, snapshots, config.Metrics)
	translator := NewChunkTranslator(config.History, config.Web)

	return &Service{
		redis: config.Redis, store: config.Store, web: config.Web,
		history: config.History, locks: config.Locks, snapshots: snapshots,
		sync: sync, processor: processor, translator: translator,
		diffs:      NewDiffManager(config.History, config.Web, processor, translator),
		summarizer: NewSummarizer(config.History, config.Web, processor, translator, config.Store),
		retries:    NewRetryManager(config.Store, config.Redis, config.Web, sync, processor),
		flusher:    NewFlushManager(config.Redis, config.Store, processor, config.ShortHistoryQueues),
		logger:     config.Logger,

		healthCheckProjectID: config.HealthCheckProjectID,
	}
}

// Processor is the flush loop, which the health check and the tests reach for.
func (s *Service) Processor() *Processor { return s.processor }

// Handler is the service's routes.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /project", s.initializeProject)
	mux.HandleFunc("DELETE /project/{project_id}", s.deleteProject)
	mux.HandleFunc("GET /project/{project_id}/snapshot", s.getLatestSnapshot)
	mux.HandleFunc("GET /project/{project_id}/diff", s.getDiff)
	mux.HandleFunc("GET /project/{project_id}/filetree/diff", s.getFileTreeDiff)
	mux.HandleFunc("GET /project/{project_id}/updates", s.getUpdates)
	mux.HandleFunc("GET /project/{project_id}/changes-in-chunk", s.getChangesInChunkSince)
	mux.HandleFunc("GET /project/{project_id}/version", s.latestVersion)
	mux.HandleFunc("POST /project/{project_id}/flush", s.flushProject)
	mux.HandleFunc("GET /project/{project_id}/resync-pending", s.getResyncPending)
	mux.HandleFunc("GET /project/{project_id}/debug-info", s.getDebugInfo)
	mux.HandleFunc("POST /project/{project_id}/resync", s.resyncProject)
	mux.HandleFunc("GET /project/{project_id}/dump", s.dumpProject)
	mux.HandleFunc("GET /project/{project_id}/labels", s.getLabels)
	mux.HandleFunc("POST /project/{project_id}/labels", s.createLabel)
	mux.HandleFunc("DELETE /project/{project_id}/user/{user_id}/labels/{label_id}",
		s.deleteLabelForUser)
	mux.HandleFunc("DELETE /project/{project_id}/labels/{label_id}", s.deleteLabel)
	mux.HandleFunc("POST /user/{from_user}/labels/transfer/{to_user}", s.transferLabels)
	mux.HandleFunc("GET /project/{project_id}/version/{version}/{pathname...}",
		s.getFileSnapshot)
	mux.HandleFunc("GET /project/{project_id}/ranges/version/{version}/{pathname...}",
		s.getRangesSnapshot)
	mux.HandleFunc("GET /project/{project_id}/metadata/version/{version}/{pathname...}",
		s.getFileMetadataSnapshot)
	mux.HandleFunc("GET /project/{project_id}/version/{version}", s.getProjectSnapshot)
	mux.HandleFunc("GET /project/{project_id}/paths/version/{version}", s.getPathsAtVersion)
	mux.HandleFunc("POST /project/{project_id}/force", s.forceDebugProject)
	mux.HandleFunc("GET /project/{history_id}/blob/{hash}", s.getProjectBlob)
	mux.HandleFunc("POST /project/{project_id}/clone", s.cloneProject)

	mux.HandleFunc("GET /status/failures", s.getFailures)
	mux.HandleFunc("GET /status/failures-full", s.getFailuresFull)
	mux.HandleFunc("GET /status/queue", s.getQueueCounts)
	mux.HandleFunc("POST /retry/failures", s.retryFailures)
	mux.HandleFunc("POST /flush/old", s.flushOld)
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		writeText(w, http.StatusOK, "project-history is up")
	})
	mux.HandleFunc("GET /check_lock", s.checkLock)
	mux.HandleFunc("GET /health_check", s.healthCheck)

	return mux
}

// objectIDText is what an id looks like. History ids can also be numbers, for
// projects whose history predates the current store.
var objectIDText = regexp.MustCompile(`^[0-9a-f]{24}$`)

// projectIDOf reads a project id from the path, refusing anything that is not
// one.
func projectIDOf(r *http.Request, name string) (string, bool) {
	id := r.PathValue(name)
	if objectIDText.MatchString(id) {
		return id, true
	}
	if _, err := strconv.Atoi(id); err == nil {
		return id, true
	}
	return "", false
}

// intQuery reads a number from the query string.
func intQuery(r *http.Request, name string, fallback int) (int, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return value, true
}

// boolQuery reads a flag from the query string.
func boolQuery(r *http.Request, name string) bool {
	raw := strings.ToLower(r.URL.Query().Get(name))
	return raw == "true" || raw == "1" || raw == "yes" || raw == "on"
}

// writeJSON sends a value as JSON.
//
// Written rather than streamed, because a stream ends with a newline and the
// body is compared byte for byte by things that read it.
func writeJSON(w http.ResponseWriter, status int, value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(encoded)
}

// writeText sends a plain body.
func writeText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

// fail turns an error into a status.
//
// The reason is logged rather than returned: it can name a project, a document
// and sometimes its contents, and the caller is another service that only
// needs to know whether to retry.
func (s *Service) fail(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, histmodel.ErrFileNotFound):
		status = http.StatusNotFound
	case errors.Is(err, ErrBadRequest):
		status = http.StatusBadRequest
	case errors.Is(err, ErrTooManyRequests):
		status = http.StatusTooManyRequests
	case errors.Is(err, ErrInconsistentChunk):
		status = http.StatusUnprocessableEntity
	}

	if s.logger != nil {
		attrs := []any{
			"method", r.Method, "path", r.URL.Path, "status", status,
			"err", err.Error(),
		}
		// What the other service said, when it said anything: a status code on
		// its own does not say which request it refused or why.
		var refused *StatusError
		if errors.As(err, &refused) && refused.Body != "" {
			attrs = append(attrs, "body", refused.Body)
		}
		s.logger.Error("request failed", attrs...)
	}
	w.WriteHeader(status)
}

// initializeProject makes a history for a project.
func (s *Service) initializeProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		HistoryID json.RawMessage `json:"historyId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	id, err := s.history.InitializeProject(r.Context(), jsonScalarString(body.HistoryID))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project": map[string]any{"id": id},
	})
}

// deleteProject forgets everything this service holds about a project.
func (s *Service) deleteProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDOf(r, "project_id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ctx := r.Context()

	// The record of when the queue started waiting goes before the queue
	// itself: it is what says where the queue is.
	if err := s.redis.ClearFirstOpTimestamp(ctx, projectID); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.redis.ClearCachedHistoryID(ctx, projectID); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.redis.DestroyQueue(ctx, projectID); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.store.ClearSyncState(ctx, projectID); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.store.ClearFailure(ctx, projectID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// flushProject writes a project's queued updates to the history.
func (s *Service) flushProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDOf(r, "project_id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var err error
	switch {
	case boolQuery(r, "debug"):
		// One update at a time, for stepping through a queue by hand.
		err = s.processor.ProcessSingleUpdateForProject(r.Context(), projectID)
	case boolQuery(r, "bisect"):
		err = s.processor.ProcessUpdatesForProjectUsingBisect(r.Context(),
			projectID, redisReadBatchSize)
	default:
		err = s.processor.ProcessUpdatesForProject(r.Context(), projectID)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// dumpProject shows what is on a project's queue without processing it.
func (s *Service) dumpProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDOf(r, "project_id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	batchSize, ok := intQuery(r, "count", redisReadBatchSize)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	dump, err := s.rawUpdates(r.Context(), projectID, batchSize)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dump)
}

// rawUpdatesDump is what a queue holds and what the history says, side by
// side, which is what somebody looking at a stuck project needs.
type rawUpdatesDump struct {
	ProjectID string           `json:"project_id"`
	Chunk     *ChunkResponse   `json:"chunk"`
	Updates   []*Update        `json:"updates"`
	Raw       []string         `json:"-"`
	Errors    map[string]error `json:"-"`
}

// rawUpdates reads a queue and the history's end without changing either.
func (s *Service) rawUpdates(ctx context.Context, projectID string,
	batchSize int) (*rawUpdatesDump, error) {

	batch, err := s.redis.GetRawUpdatesBatch(ctx, projectID, batchSize)
	if err != nil {
		return nil, err
	}

	updates := make([]*Update, 0, len(batch.Raw))
	for _, raw := range batch.Raw {
		var update Update
		if err := json.Unmarshal([]byte(raw), &update); err != nil {
			return nil, fmt.Errorf("failed to parse update: %w", err)
		}
		updates = append(updates, &update)
	}

	historyID, err := s.processor.historyIDFor(ctx, projectID, updates)
	if err != nil {
		return nil, err
	}
	chunk, err := s.history.GetMostRecentChunk(ctx, historyID)
	if err != nil {
		return nil, err
	}
	return &rawUpdatesDump{
		ProjectID: projectID, Chunk: chunk, Updates: updates,
	}, nil
}

// flushOld sweeps the queues that have been waiting.
func (s *Service) flushOld(w http.ResponseWriter, r *http.Request) {
	maxAge, ok1 := intQuery(r, "maxAge", 6*3600)
	queueDelay, ok2 := intQuery(r, "queueDelay", 100)
	limit, ok3 := intQuery(r, "limit", 1000)
	timeout, ok4 := intQuery(r, "timeout", 60*1000)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	options := FlushOldOptions{
		MaxAge:     time.Duration(maxAge) * time.Second,
		QueueDelay: time.Duration(queueDelay) * time.Millisecond,
		Limit:      limit,
		Timeout:    time.Duration(timeout) * time.Millisecond,
	}

	if boolQuery(r, "background") {
		// The caller is a cron job that only wants the sweep started.
		writeJSON(w, http.StatusOK,
			map[string]any{"message": "running flush in background"})
		go func() {
			ctx := context.WithoutCancel(r.Context())
			if _, err := s.flusher.FlushOldOps(ctx, options); err != nil && s.logger != nil {
				s.logger.Error("background flush failed", "err", err)
			}
		}()
		return
	}

	result, err := s.flusher.FlushOldOps(r.Context(), options)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// getDiff is what changed in one document between two versions.
func (s *Service) getDiff(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDOf(r, "project_id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	pathname := r.URL.Query().Get("pathname")
	from, ok1 := intQuery(r, "from", 0)
	to, ok2 := intQuery(r, "to", 0)
	if pathname == "" || !ok1 || !ok2 ||
		r.URL.Query().Get("from") == "" || r.URL.Query().Get("to") == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	diff, binary, err := s.diffs.GetDiff(r.Context(), projectID, pathname, from, to)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if binary {
		writeJSON(w, http.StatusOK,
			map[string]any{"diff": map[string]any{"binary": true}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"diff": diff})
}

// getFileTreeDiff is what happened to a project's files between two versions.
func (s *Service) getFileTreeDiff(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDOf(r, "project_id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	from, ok1 := intQuery(r, "from", 0)
	to, ok2 := intQuery(r, "to", 0)
	if !ok1 || !ok2 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	diff, err := s.diffs.GetFileTreeDiff(r.Context(), projectID, from, to)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"diff": diff})
}

// getUpdates lists a project's history.
func (s *Service) getUpdates(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDOf(r, "project_id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	before, ok1 := intQuery(r, "before", 0)
	minCount, ok2 := intQuery(r, "min_count", 0)
	if !ok1 || !ok2 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	updates, nextBefore, err := s.summarizer.GetSummarizedProjectUpdates(
		r.Context(), projectID, SummarizeOptions{Before: before, MinCount: minCount})
	if err != nil {
		s.fail(w, r, err)
		return
	}

	response := map[string]any{"updates": updates}
	if nextBefore > 0 {
		response["nextBeforeTimestamp"] = nextBefore
	}
	writeJSON(w, http.StatusOK, response)
}

// getResyncPending says whether a project is being resynced and whether that
// resync has got stuck.
func (s *Service) getResyncPending(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDOf(r, "project_id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	state, err := s.sync.GetResyncState(r.Context(), projectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"resyncPending": state.IsSyncOngoing(),
		"syncStuck":     state.IsSyncStuck(),
	})
}

// getDebugInfo is everything this service knows about a project's state.
func (s *Service) getDebugInfo(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDOf(r, "project_id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ctx := r.Context()

	state, err := s.sync.GetResyncState(ctx, projectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	failure, err := s.store.GetFailure(ctx, projectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	raw := state.raw()
	syncState := map[string]any{
		"resyncPending":          state.IsSyncOngoing(),
		"resyncCount":            state.ResyncCount,
		"resyncPendingSince":     state.ResyncPendingSince,
		"lastUpdated":            state.LastUpdated,
		"history":                state.History,
		"resyncProjectStructure": raw.ResyncProjectStructure,
		"resyncDocContents":      raw.ResyncDocContents,
		"hardResync":             raw.HardResync,
		"recoverCorruptedFiles":  raw.RecoverCorruptedFiles,
	}
	if len(raw.Origin) > 0 {
		syncState["origin"] = raw.Origin
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"failureRecord": failure, "syncState": syncState,
	})
}

// latestVersion is where a project's history has got to.
func (s *Service) latestVersion(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDOf(r, "project_id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ctx := r.Context()

	if err := s.processor.ProcessUpdatesForProject(ctx, projectID); err != nil {
		s.fail(w, r, err)
		return
	}
	historyID, err := s.web.GetHistoryID(ctx, projectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	recent, err := s.history.GetMostRecentVersion(ctx, projectID, historyID)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	response := map[string]any{"version": recent.Version}
	if recent.LastChange != nil {
		response["timestamp"] = recent.LastChange.Timestamp
		response["v2Authors"] = recent.LastChange.V2Authors
	}
	writeJSON(w, http.StatusOK, response)
}

// getFileSnapshot is a document's contents as they were at a version.
func (s *Service) getFileSnapshot(w http.ResponseWriter, r *http.Request) {
	projectID, version, pathname, ok := s.versionRequest(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ctx := r.Context()

	file, historyID, err := s.snapshots.FileAtVersion(ctx, projectID, version, pathname)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	if !file.Data.IsEditable() {
		// A binary file is streamed straight out of the blob it is stored in.
		body, err := s.history.GetProjectBlobStream(ctx, historyID, file.Hash())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		defer body.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = io.Copy(w, body)
		return
	}

	if err := file.LoadEager(ctx, s.history.BlobStoreFor(historyID)); err != nil {
		s.fail(w, r, err)
		return
	}
	writeText(w, http.StatusOK, file.StringData().GetContent(true))
}

// getRangesSnapshot is a document's marks as they were at a version.
func (s *Service) getRangesSnapshot(w http.ResponseWriter, r *http.Request) {
	projectID, version, pathname, ok := s.versionRequest(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ranges, err := s.snapshots.GetRangesSnapshot(r.Context(), projectID, version, pathname)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ranges)
}

// getFileMetadataSnapshot is a file's metadata as it was at a version.
func (s *Service) getFileMetadataSnapshot(w http.ResponseWriter, r *http.Request) {
	projectID, version, pathname, ok := s.versionRequest(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	metadata, err := s.snapshots.GetFileMetadataSnapshot(r.Context(), projectID,
		version, pathname)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if metadata == nil {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"metadata": metadata})
}

// versionRequest reads the project, version and path a request names.
func (s *Service) versionRequest(r *http.Request) (string, int, string, bool) {
	projectID, ok := projectIDOf(r, "project_id")
	if !ok {
		return "", 0, "", false
	}
	version, err := strconv.Atoi(r.PathValue("version"))
	if err != nil {
		return "", 0, "", false
	}
	pathname := r.PathValue("pathname")
	if pathname == "" {
		return "", 0, "", false
	}
	return projectID, version, pathname, true
}

// getLatestSnapshot is the project as it is now.
func (s *Service) getLatestSnapshot(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDOf(r, "project_id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ctx := r.Context()

	historyID, err := s.web.GetHistoryID(ctx, projectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	snapshot, version, err := s.snapshots.LatestSnapshot(ctx, projectID, historyID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"snapshot": snapshot, "version": version,
	})
}

// getChangesInChunkSince is the changes made after a version.
func (s *Service) getChangesInChunkSince(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDOf(r, "project_id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	since, valid := intQuery(r, "since", -1)
	if !valid || since < 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ctx := r.Context()

	historyID, err := s.web.GetHistoryID(ctx, projectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	latestStartVersion, changes, err := s.snapshots.ChangesInChunkSince(ctx,
		projectID, historyID, since)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if changes == nil {
		changes = []*histmodel.Change{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"latestStartVersion": latestStartVersion, "changes": changes,
	})
}

// getProjectSnapshot is the whole project at a version.
func (s *Service) getProjectSnapshot(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDOf(r, "project_id")
	version, err := strconv.Atoi(r.PathValue("version"))
	if !ok || err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	snapshot, err := s.snapshots.GetProjectSnapshot(r.Context(), projectID, version)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

// getPathsAtVersion is the files a project had at a version.
func (s *Service) getPathsAtVersion(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDOf(r, "project_id")
	version, err := strconv.Atoi(r.PathValue("version"))
	if !ok || err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	paths, err := s.snapshots.PathsAtVersion(r.Context(), projectID, version)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if paths == nil {
		paths = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"paths": paths})
}

// getProjectBlob streams a blob out of the history store.
func (s *Service) getProjectBlob(w http.ResponseWriter, r *http.Request) {
	historyID := r.PathValue("history_id")
	hash := r.PathValue("hash")
	if historyID == "" || hash == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	body, err := s.history.GetProjectBlobStream(r.Context(), historyID, hash)
	if err != nil {
		if StatusCodeOf(err) == http.StatusNotFound {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		s.fail(w, r, err)
		return
	}
	defer body.Close()

	// A blob never changes: it is named by the hash of what is in it.
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = io.Copy(w, body)
}

// resyncProject asks the editor to send a project again.
func (s *Service) resyncProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDOf(r, "project_id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var body struct {
		Force                  bool            `json:"force"`
		RecoverCorruptedFiles  bool            `json:"recoverCorruptedFiles"`
		Origin                 json.RawMessage `json:"origin"`
		HistoryRangesMigration string          `json:"historyRangesMigration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	options := StartResyncOptions{
		Origin:                 body.Origin,
		HistoryRangesMigration: body.HistoryRangesMigration,
	}

	ctx := r.Context()
	var err error
	if boolQuery(r, "force") || body.Force {
		// A project that is completely broken: the queue and the sync state go
		// and it is rebuilt from what the editor has.
		options.RecoverCorruptedFiles = boolQuery(r, "recoverCorruptedFiles") ||
			body.RecoverCorruptedFiles
		err = s.sync.StartHardResync(ctx, projectID, options)
	} else {
		err = s.sync.StartResync(ctx, projectID, options)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}

	if err := s.processor.ProcessUpdatesForProject(ctx, projectID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// forceDebugProject marks a project as one whose history may be walked past
// even when its versions are out of order.
func (s *Service) forceDebugProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDOf(r, "project_id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ctx := r.Context()

	// Set unless the request says to clear it.
	if err := s.store.SetForceDebug(ctx, projectID, !boolQuery(r, "clear")); err != nil {
		s.fail(w, r, err)
		return
	}
	failure, err := s.store.GetFailure(ctx, projectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, failure)
}

// getFailures counts the projects that are failing, by kind of failure.
func (s *Service) getFailures(w http.ResponseWriter, r *http.Request) {
	summary, err := s.store.SummariseFailures(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"failures": summary})
}

// getFailuresFull lists every failure record.
func (s *Service) getFailuresFull(w http.ResponseWriter, r *http.Request) {
	failures, err := s.store.GetFailures(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}

	listed := make([]map[string]any, 0, len(failures))
	for i := range failures {
		encoded, err := json.Marshal(failures[i])
		if err != nil {
			s.fail(w, r, err)
			return
		}
		var fields map[string]any
		if err := json.Unmarshal(encoded, &fields); err != nil {
			s.fail(w, r, err)
			return
		}
		if category := FailureCategory(&failures[i]); category != "" {
			fields["category"] = category
		}
		listed = append(listed, fields)
	}
	writeJSON(w, http.StatusOK, listed)
}

// getQueueCounts is how many projects have updates waiting.
func (s *Service) getQueueCounts(w http.ResponseWriter, r *http.Request) {
	count, err := s.redis.CountProjectsWithHistoryOps(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"queuedProjects": count})
}

// retryFailures comes back to the projects that failed.
func (s *Service) retryFailures(w http.ResponseWriter, r *http.Request) {
	failureType := r.URL.Query().Get("failureType")
	if failureType != "" && failureType != "soft" && failureType != "hard" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	timeout, ok1 := intQuery(r, "timeout", 300)
	limit, ok2 := intQuery(r, "limit", 100)
	if !ok1 || !ok2 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	options := RetryOptions{
		FailureType: failureType,
		Timeout:     time.Duration(timeout) * time.Millisecond,
		Limit:       limit,
	}

	if callbackURL := r.URL.Query().Get("callbackUrl"); callbackURL != "" {
		// The caller is a cron job that only wants the run started, and to be
		// told when it finishes.
		writeJSON(w, http.StatusOK,
			map[string]any{"retryStatus": "running retryFailures in background"})
		headers := callbackHeaders(r)
		go func() {
			ctx := context.WithoutCancel(r.Context())
			if _, err := s.retries.RetryFailures(ctx, options); err != nil {
				if s.logger != nil {
					s.logger.Error("background retry failed", "err", err)
				}
				return
			}
			s.pingCallback(ctx, callbackURL, headers)
		}()
		return
	}

	result, err := s.retries.RetryFailures(r.Context(), options)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"retryStatus": result})
}

// callbackHeaders are the headers a caller asked to have passed back to it.
func callbackHeaders(r *http.Request) map[string]string {
	headers := map[string]string{}
	for name, values := range r.Header {
		if !strings.HasPrefix(strings.ToUpper(name), "X-CALLBACK-") || len(values) == 0 {
			continue
		}
		headers[name[len("X-CALLBACK-"):]] = values[0]
	}
	return headers
}

// pingCallback tells a caller that a background run has finished. It is best
// effort: nothing depends on it arriving.
func (s *Service) pingCallback(ctx context.Context, target string,
	headers map[string]string) {

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("failed to ping callback url", "err", err)
		}
		return
	}
	_ = res.Body.Close()
}

// getLabels lists the versions somebody has named.
func (s *Service) getLabels(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDOf(r, "project_id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if !s.usesProjectHistory(w, r, projectID) {
		return
	}

	labels, err := s.store.GetLabels(r.Context(), projectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if labels == nil {
		labels = []Label{}
	}
	writeJSON(w, http.StatusOK, labels)
}

// createLabel names a version.
func (s *Service) createLabel(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDOf(r, "project_id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var body struct {
		Version        *int    `json:"version"`
		Comment        string  `json:"comment"`
		CreatedAt      string  `json:"created_at"`
		ValidateExists *bool   `json:"validate_exists"`
		UserID         *string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if body.Version == nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if !s.usesProjectHistory(w, r, projectID) {
		return
	}

	userID := ""
	if body.UserID != nil {
		userID = *body.UserID
	}
	ctx := r.Context()

	// The version is checked against the history unless the caller says not
	// to: a label on a version that does not exist would be a name for
	// nothing.
	if body.ValidateExists == nil || *body.ValidateExists {
		if err := s.validateVersionExists(ctx, projectID, *body.Version); err != nil {
			s.fail(w, r, err)
			return
		}
	}

	createdAt := time.Now()
	if body.CreatedAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, body.CreatedAt)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		createdAt = parsed
	}

	label, err := s.store.CreateLabel(ctx, projectID, userID, body.Comment,
		*body.Version, createdAt)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, label)
}

// validateVersionExists checks that a version is one the history has.
func (s *Service) validateVersionExists(ctx context.Context, projectID string,
	version int) error {

	if err := s.processor.ProcessUpdatesForProject(ctx, projectID); err != nil {
		return err
	}
	historyID, err := s.web.GetHistoryID(ctx, projectID)
	if err != nil {
		return err
	}
	_, err = s.history.GetChunkAtVersion(ctx, historyID, version)
	return err
}

// deleteLabel takes a name off a version.
func (s *Service) deleteLabel(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDOf(r, "project_id")
	labelID := r.PathValue("label_id")
	if !ok || !objectIDText.MatchString(labelID) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteLabel(r.Context(), projectID, labelID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteLabelForUser takes a name off a version, but only the person who put
// it there may.
func (s *Service) deleteLabelForUser(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDOf(r, "project_id")
	userID := r.PathValue("user_id")
	labelID := r.PathValue("label_id")
	if !ok || !objectIDText.MatchString(userID) || !objectIDText.MatchString(labelID) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteLabelForUser(r.Context(), projectID, userID, labelID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// transferLabels moves one person's labels to another, which is what happens
// when an account is merged.
func (s *Service) transferLabels(w http.ResponseWriter, r *http.Request) {
	fromUser := r.PathValue("from_user")
	toUser := r.PathValue("to_user")
	if !objectIDText.MatchString(fromUser) || !objectIDText.MatchString(toUser) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := s.store.TransferLabels(r.Context(), fromUser, toUser); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// usesProjectHistory reports whether a project has a history, answering the
// request itself when it does not.
func (s *Service) usesProjectHistory(w http.ResponseWriter, r *http.Request,
	projectID string) bool {

	historyID, err := s.web.GetHistoryID(r.Context(), projectID)
	if err != nil {
		s.fail(w, r, err)
		return false
	}
	if historyID == "" {
		w.WriteHeader(http.StatusConflict)
		return false
	}
	return true
}

// cloneProject copies one project's history onto another.
func (s *Service) cloneProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDOf(r, "project_id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var body struct {
		TargetProjectID string `json:"targetProjectId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
		body.TargetProjectID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ctx := r.Context()

	// Best effort: a project whose queue could not be flushed is copied
	// without the updates still on it, and will need a resync. Failing the
	// whole copy for that would be worse.
	if err := s.processor.ProcessUpdatesForProject(ctx, projectID); err != nil && s.logger != nil {
		s.logger.Warn("failed to flush during history clone",
			"projectId", projectID, "err", err)
	}

	sourceHistoryID, err := s.web.GetHistoryID(ctx, projectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	targetHistoryID, err := s.web.GetHistoryID(ctx, body.TargetProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	if err := s.history.CloneProject(ctx, sourceHistoryID, targetHistoryID); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.store.CloneLabels(ctx, projectID, body.TargetProjectID); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.sync.CloneResyncState(ctx, projectID, body.TargetProjectID); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.store.CloneFailure(ctx, projectID, body.TargetProjectID); err != nil {
		s.fail(w, r, err)
		return
	}
	writeText(w, http.StatusOK, "done")
}

// checkLock takes and releases the health check lock, which is what says Redis
// is reachable and the lock scripts still work.
func (s *Service) checkLock(w http.ResponseWriter, r *http.Request) {
	if err := s.locks.HealthCheck(r.Context()); err != nil {
		if s.logger != nil {
			s.logger.Error("error performing lock check", "err", err)
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// healthCheck flushes a project and reads its history, which exercises Redis,
// Mongo, web and the history store rather than only this process.
func (s *Service) healthCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := s.locks.HealthCheck(ctx); err != nil {
		if s.logger != nil {
			s.logger.Error("error performing health check", "err", err)
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if s.healthCheckProjectID == "" {
		// No project has been named to check against, so the lock is all that
		// can be checked.
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := s.processor.ProcessUpdatesForProject(ctx, s.healthCheckProjectID); err != nil {
		if s.logger != nil {
			s.logger.Error("error flushing for health check", "err", err)
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if _, _, err := s.summarizer.GetSummarizedProjectUpdates(ctx,
		s.healthCheckProjectID, SummarizeOptions{}); err != nil {

		if s.logger != nil {
			s.logger.Error("error getting updates for health check", "err", err)
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
