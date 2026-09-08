// Package internalapi is what the other services in the deployment ask this
// one.
//
// It is not the public API and it is not on the public port: real-time asking
// whether somebody may open a project, document-updater loading a document
// nobody has open, project-history asking where a project's history is. All of
// it used to be the Node web service's, which is the only reason that service
// was still running.
//
// The shapes here are that service's, not this API's, because the callers were
// written against them and are not being rewritten to match.
package internalapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/documents"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/users"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Service answers the other services.
type Service struct {
	log        *slog.Logger
	projects   *projects.Store
	users      *users.Store
	storage    *documents.Storage
	docupdater *documents.Client
	chat       *chatClient

	user     string
	password string
}

// Options is what a Service needs.
type Options struct {
	Log        *slog.Logger
	Projects   *projects.Store
	Users      *users.Store
	Storage    *documents.Storage
	Docupdater *documents.Client
	ChatURL    string
	// User and Password are what the other services authenticate with. They
	// are on a loopback port inside one container, so this is a second lock on
	// a door that is already inside the building -- but the callers send it
	// and a service that ignored it would be one misconfiguration away from
	// answering anybody.
	User     string
	Password string
}

// New builds it.
func New(opts Options) *Service {
	return &Service{
		log:        opts.Log,
		projects:   opts.Projects,
		users:      opts.Users,
		storage:    opts.Storage,
		docupdater: opts.Docupdater,
		chat:       newChatClient(opts.ChatURL),
		user:       opts.User,
		password:   opts.Password,
	}
}

// Handler is the whole internal API.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()

	// real-time: may this person open this project, and what should the editor
	// be given.
	mux.HandleFunc("POST /project/{projectId}/join", s.join)

	// document-updater: a document as it is stored, and back again.
	mux.HandleFunc("GET /project/{projectId}/doc/{docId}", s.getDoc)
	mux.HandleFunc("POST /project/{projectId}/doc/{docId}", s.setDoc)
	mux.HandleFunc("POST /project/{projectId}/doc/{docId}/changes/reject", s.rejectChanges)

	// project-history: where a project's history is, and asking for the whole
	// project again when the two have drifted apart.
	mux.HandleFunc("GET /project/{projectId}/details", s.details)
	mux.HandleFunc("GET /internal/project/{projectId}", s.details)
	mux.HandleFunc("POST /project/{projectId}/history/resync", s.resync)

	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("api is alive\n"))
	})
	mux.HandleFunc("GET /health_check", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return s.authenticated(mux)
}

// authenticated refuses anything that did not present the shared credentials.
func (s *Service) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/status") ||
			strings.HasPrefix(r.URL.Path, "/health_check") {
			next.ServeHTTP(w, r)
			return
		}
		user, password, ok := r.BasicAuth()
		if !ok ||
			subtle.ConstantTimeCompare([]byte(user), []byte(s.user)) != 1 ||
			subtle.ConstantTimeCompare([]byte(password), []byte(s.password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="internal"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- real-time -------------------------------------------------------------

// join is the authorisation decision for a whole editing connection.
//
// real-time does not read permissions itself: it asks here once, when somebody
// opens a project, and the answer decides what they may do for as long as they
// stay connected.
func (s *Service) join(w http.ResponseWriter, r *http.Request) {
	projectID, ok := idOf(r, "projectId")
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var in struct {
		UserID string `json:"userId"`
		// AnonymousAccessToken is for somebody following a share link. Link
		// sharing is not offered yet, so a request carrying one is treated as
		// anonymous, which is a refusal.
		AnonymousAccessToken string `json:"anonymousAccessToken"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in)

	userID, err := bson.ObjectIDFromHex(in.UserID)
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	project, access, err := s.projects.Get(r.Context(), projectID, userID)
	if errors.Is(err, projects.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}

	owner := map[string]any{"_id": project.OwnerRef.Hex()}
	if person, err := s.users.ByID(r.Context(), project.OwnerRef); err == nil {
		owner["email"] = person.Email
		owner["first_name"] = person.FirstName
		owner["last_name"] = person.LastName
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"project": map[string]any{
			"_id":                         project.ID.Hex(),
			"name":                        project.Name,
			"rootDoc_id":                  hexOrNil(project.RootDocID),
			"rootFolder":                  project.RootFolder,
			"owner":                       owner,
			"compiler":                    project.Compiler,
			"imageName":                   project.ImageName,
			"spellCheckLanguage":          project.SpellCheck,
			"description":                 project.Description,
			"features":                    features(),
			"deletedByExternalDataSource": false,
			"invites":                     []any{},
			"members":                     []any{},
		},
		"privilegeLevel": string(access),
		// Restricted users are the ones who reached a project through a link
		// with less than full access. Nothing hands those out yet.
		"isRestrictedUser": false,
		"isTokenMember":    false,
		"isInvitedMember":  access != projects.AccessNone,
	})
}

// --- document-updater ------------------------------------------------------

// getDoc is a document as it is stored, with the things only this service
// knows: where the document is in the project, and which history it belongs
// to.
func (s *Service) getDoc(w http.ResponseWriter, r *http.Request) {
	projectID, ok := idOf(r, "projectId")
	docID, ok2 := idOf(r, "docId")
	if !ok || !ok2 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	project, err := s.projects.Read(r.Context(), projectID)
	if errors.Is(err, projects.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	entry, found := project.Find(docID)
	if !found || entry.Kind != projects.EntryDoc {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	stored, err := s.storage.Get(r.Context(), projectID, docID,
		r.URL.Query().Get("peek") == "true")
	if errors.Is(err, documents.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}

	if r.URL.Query().Get("plain") == "true" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(strings.Join(stored.Lines, "\n")))
		return
	}

	// Which of this document's comments have been resolved. It lives with the
	// comments, in chat, and the editor needs it at the same moment as the
	// text, so it is answered here rather than fetched separately.
	resolved := s.chat.resolvedIn(r.Context(), projectID, stored.Ranges)

	writeJSON(w, http.StatusOK, map[string]any{
		"lines":                stored.Lines,
		"version":              stored.Version,
		"ranges":               rawOrEmpty(stored.Ranges),
		"pathname":             "/" + entry.Path,
		"projectHistoryId":     project.HistoryID(),
		"projectHistoryType":   "project-history",
		"historyRangesSupport": false,
		"otMigrationStage":     0,
		"resolvedCommentIds":   resolved,
	})
}

// setDoc writes a document back to storage.
//
// Called when the last person closes a document: until then the text lives in
// document-updater, and this is where it lands.
func (s *Service) setDoc(w http.ResponseWriter, r *http.Request) {
	projectID, ok := idOf(r, "projectId")
	docID, ok2 := idOf(r, "docId")
	if !ok || !ok2 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var in struct {
		Lines         []string        `json:"lines"`
		Version       int64           `json:"version"`
		Ranges        json.RawMessage `json:"ranges"`
		LastUpdatedAt json.RawMessage `json:"lastUpdatedAt"`
		LastUpdatedBy *string         `json:"lastUpdatedBy"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<20)).Decode(&in); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	written, err := s.storage.Update(r.Context(), projectID, docID, in.Lines, in.Version, in.Ranges)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if written.Modified {
		// The project's own record of when it last changed, which is what a
		// project list is ordered by. Best effort: the text is already stored,
		// and failing the write back over a timestamp would lose it.
		by := bson.ObjectID{}
		if in.LastUpdatedBy != nil {
			if parsed, err := bson.ObjectIDFromHex(*in.LastUpdatedBy); err == nil {
				by = parsed
			}
		}
		_ = s.projects.Touch(r.Context(), projectID, by)
	}
	writeJSON(w, http.StatusOK, written)
}

// rejectChanges is where document-updater reports that tracked changes were
// rejected.
//
// Nothing here acts on it: what listened for it was the track-changes module,
// which is not part of this rewrite yet. Answering rather than refusing keeps
// document-updater from treating a rejection as a failed one.
func (s *Service) rejectChanges(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// --- project-history -------------------------------------------------------

// details is what another service needs to know about a project.
func (s *Service) details(w http.ResponseWriter, r *http.Request) {
	projectID, ok := idOf(r, "projectId")
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	project, err := s.projects.Read(r.Context(), projectID)
	if errors.Is(err, projects.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}

	answer := map[string]any{
		"name":        project.Name,
		"description": project.Description,
		"compiler":    project.Compiler,
		"features":    features(),
	}
	if historyID := project.HistoryID(); historyID != "" {
		answer["overleaf"] = map[string]any{
			"history": map[string]any{"id": historyID},
		}
	}
	writeJSON(w, http.StatusOK, answer)
}

// resync sends a project's whole file tree to the history again.
//
// Asked for when project-history finds that its copy of the tree no longer
// matches: it can tell that the two disagree but not which is right, and the
// project document is the one that is.
func (s *Service) resync(w http.ResponseWriter, r *http.Request) {
	projectID, ok := idOf(r, "projectId")
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var in struct {
		HistoryRangesMigration     string `json:"historyRangesMigration"`
		ResyncProjectStructureOnly bool   `json:"resyncProjectStructureOnly"`
	}
	if r.ContentLength > 0 {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in)
	}

	project, err := s.projects.Read(r.Context(), projectID)
	if errors.Is(err, projects.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	historyID := project.HistoryID()
	if historyID == "" {
		// No history to resync against. A 404 is what the caller reads as
		// "this project has no history", which is true.
		w.WriteHeader(http.StatusNotFound)
		return
	}

	docs := []map[string]any{}
	files := []map[string]any{}
	for _, entry := range project.Entries() {
		switch entry.Kind {
		case projects.EntryDoc:
			docs = append(docs, map[string]any{
				"doc":  entry.ID.Hex(),
				"path": "/" + entry.Path,
			})
		case projects.EntryFile:
			if entry.Hash == "" {
				// A file with no blob cannot be resynced: the history would be
				// told about something it cannot fetch. Left out rather than
				// failing the whole resync over one file.
				s.log.Warn("a file with no stored bytes was left out of a resync",
					slog.String("projectId", projectID.Hex()),
					slog.String("path", entry.Path))
				continue
			}
			files = append(files, map[string]any{
				"file":        entry.ID.Hex(),
				"path":        "/" + entry.Path,
				"_hash":       entry.Hash,
				"createdBlob": true,
			})
		}
	}

	err = s.docupdater.Resync(r.Context(), projectID, historyID, docs, files,
		in.HistoryRangesMigration, in.ResyncProjectStructureOnly)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- the small pieces ------------------------------------------------------

func (s *Service) fail(w http.ResponseWriter, r *http.Request, err error) {
	if s.log != nil {
		s.log.Error("an internal request failed",
			slog.String("path", r.URL.Path), slog.Any("err", err))
	}
	w.WriteHeader(http.StatusInternalServerError)
}

func idOf(r *http.Request, name string) (bson.ObjectID, bool) {
	id, err := bson.ObjectIDFromHex(r.PathValue(name))
	return id, err == nil
}

func hexOrNil(id bson.ObjectID) any {
	if id.IsZero() {
		return nil
	}
	return id.Hex()
}

func rawOrEmpty(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}
	return raw
}

// features is what an account is allowed. Every account is allowed everything
// here: the limits it describes belong to a hosted service with paid plans,
// and this is not one.
func features() map[string]any {
	return map[string]any{
		"collaborators":       -1,
		"versioning":          true,
		"dropbox":             true,
		"github":              true,
		"gitBridge":           true,
		"compileTimeout":      180,
		"compileGroup":        "standard",
		"templates":           true,
		"references":          true,
		"trackChanges":        true,
		"trackChangesVisible": true,
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// ReadTimeout is what the server this runs in should allow. Generous, because
// a resync reads a whole project.
func ReadTimeout() time.Duration { return 6 * time.Minute }
