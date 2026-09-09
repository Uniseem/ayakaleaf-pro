package documents

import (
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Moving, uploading, and reading back what was uploaded.

// MaxUploadBytes is the largest file that may be put into a project.
//
// Held here rather than left to the proxy so that the answer is this API's
// error shape and not nginx's HTML page, which a fetch cannot read.
const MaxUploadBytes = 50 << 20

// Move puts something in a different folder.
func (s *Service) Move(w http.ResponseWriter, r *http.Request) error {
	project, user, err := s.writable(r)
	if err != nil {
		return err
	}
	entry, err := entryIn(project, r.PathValue("entryId"))
	if err != nil {
		return err
	}
	var in struct {
		FolderID *string `json:"folderId"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}

	// No folder means the root, which is what dropping onto empty space in
	// the tree means.
	var folderID bson.ObjectID
	if in.FolderID == nil || *in.FolderID == "" {
		id, ok := project.RootFolderID()
		if !ok {
			return apierr.Internal.WithMessage("That project has no root folder.")
		}
		folderID = id
	} else {
		id, parseErr := bson.ObjectIDFromHex(*in.FolderID)
		if parseErr != nil {
			return apierr.BadRequest.WithField("folderId").
				WithMessage("That is not a folder.")
		}
		folderID = id
	}

	if entry.Parent == folderID {
		return apierr.BadRequest.WithMessage("That is already where it is.")
	}
	// Where it will end up, worked out before the move so that the history can
	// be told what changed in terms of paths.
	targetPath, ok := project.FolderPathName(folderID)
	if !ok {
		return apierr.BadRequest.WithField("folderId").
			WithMessage("There is no such folder.")
	}

	version, err := s.projects.MoveEntry(r.Context(), project, entry.ID, folderID)
	switch {
	case err == projects.ErrNameTaken:
		return apierr.Conflict.WithField("name").
			WithMessage("There is already something called that there.")
	case err == projects.ErrNotFound:
		return apierr.BadRequest.WithMessage("That cannot be moved there.")
	case err != nil:
		return apierr.Internal.WithCause(err)
	}

	s.report(r.Context(), project, user.ID, version, "editor",
		movesFor(project, entry, targetPath))
	return httpapi.NoContent(w)
}

// movesFor describes a move to the history as the path changes it causes.
//
// The history has no idea of a move: it stores what path held what, so moving
// a folder of forty files is forty renames. The same shape as renamesFor, for
// the same reason.
func movesFor(project *projects.Project, entry projects.Entry, targetPath string) []StructureUpdate {
	moved := joinPath(targetPath, entry.Name)
	switch entry.Kind {
	case projects.EntryDoc:
		return []StructureUpdate{RenamedDoc(entry.ID, "/"+entry.Path, "/"+moved)}
	case projects.EntryFile:
		return []StructureUpdate{RenamedFile(entry.ID, "/"+entry.Path, "/"+moved)}
	}

	inside := entry.Path + "/"
	updates := []StructureUpdate{}
	for _, child := range project.Entries() {
		if !strings.HasPrefix(child.Path, inside) {
			continue
		}
		to := moved + child.Path[len(entry.Path):]
		switch child.Kind {
		case projects.EntryDoc:
			updates = append(updates, RenamedDoc(child.ID, "/"+child.Path, "/"+to))
		case projects.EntryFile:
			updates = append(updates, RenamedFile(child.ID, "/"+child.Path, "/"+to))
		}
	}
	return updates
}

// Upload puts a file into a project.
//
// The one endpoint in this API that takes something other than JSON, because
// the alternative is base64 in a JSON body, which costs a third more bytes and
// has to be held in memory twice.
func (s *Service) Upload(w http.ResponseWriter, r *http.Request) error {
	project, user, err := s.writable(r)
	if err != nil {
		return err
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		return apierr.BadRequest.WithCause(err).
			WithMessage(fmt.Sprintf("That upload could not be read, or is larger than %d MB.", MaxUploadBytes>>20))
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	file, header, err := r.FormFile("file")
	if err != nil {
		return apierr.BadRequest.WithField("file").WithMessage("A file is required.")
	}
	defer func() { _ = file.Close() }()

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" && header != nil {
		name = header.Filename
	}
	// A browser may send a path; only the last part of it is the name, and a
	// name from somewhere else is not trusted to be one.
	name = path.Base(filepath(name))
	if err := projects.ValidName(name); err != nil {
		return apierr.BadRequest.WithField("name").
			WithMessage(capitalise(err.Error()) + ".")
	}

	folderPath := ""
	if raw := strings.TrimSpace(r.FormValue("folderId")); raw != "" {
		id, parseErr := bson.ObjectIDFromHex(raw)
		if parseErr != nil {
			return apierr.BadRequest.WithField("folderId").
				WithMessage("That is not a folder.")
		}
		named, ok := project.FolderPathName(id)
		if !ok {
			return apierr.BadRequest.WithField("folderId").
				WithMessage("There is no such folder.")
		}
		folderPath = named
	}

	content, err := io.ReadAll(file)
	if err != nil {
		return apierr.BadRequest.WithCause(err).
			WithMessage("That file could not be read.")
	}

	target := joinPath(folderPath, name)
	updated, err := s.UpsertFile(r.Context(), project, target, content, user.ID, "upload")
	if err != nil {
		return apierr.Internal.WithCause(err)
	}

	entry, ok := updated.FindPath(target)
	if !ok {
		return apierr.Internal.WithMessage("That file was stored but cannot be found.")
	}
	return httpapi.JSON(w, http.StatusCreated, map[string]any{"file": entry})
}

// ReadFile serves the bytes of a binary file in a project.
//
// Proxied rather than redirected to the blob store: a redirect would send the
// browser somewhere that does not check who is asking, and these files are as
// private as the project they are in.
func (s *Service) ReadFile(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.readable(r)
	if err != nil {
		return err
	}
	entry, err := entryIn(project, r.PathValue("fileId"))
	if err != nil {
		return err
	}
	if entry.Kind != projects.EntryFile || entry.Hash == "" {
		return apierr.NotFound.WithMessage("That is not a file with contents.")
	}
	historyID := project.HistoryID()
	if historyID == "" {
		return apierr.NotFound.WithMessage("That project has no stored files.")
	}

	body, err := s.history.ReadBlob(r.Context(), historyID, entry.Hash)
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	defer func() { _ = body.Close() }()

	// What a browser may render in place, and what it must only download.
	//
	// The distinction is a security one, not a convenience one. Anything
	// served inline from this origin runs with this origin's cookies, so an
	// uploaded .html -- or .svg, which is a document that may carry script --
	// would be a way to run script as whoever opened it. Those are sent as
	// bytes to save instead. The cost is that the file viewer cannot show an
	// SVG in place; that is the right way round.
	kind, inline := servableType(entry.Name)
	w.Header().Set("Content-Type", kind)
	// Addressed by content hash, so it can be cached until the address changes.
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	disposition := "attachment"
	if inline {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", disposition+"; filename*=UTF-8''"+escapeName(entry.Name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Belt and braces: even if something does end up rendered, it renders with
	// no origin of its own and no script.
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; img-src 'self' data:; style-src 'unsafe-inline'")
	if entry.Size > 0 {
		w.Header().Set("Content-Length", fmt.Sprint(entry.Size))
	}
	_, err = io.Copy(w, body)
	return err
}

// inlineTypes are the types a browser may be asked to render in place.
//
// Raster images and PDF only. Every one of them is a format a browser renders
// without a scripting context, which is the property that matters here.
var inlineTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".avif": "image/avif",
	".bmp":  "image/bmp",
	".ico":  "image/vnd.microsoft.icon",
	".pdf":  "application/pdf",
}

// servableType says what to call a file and whether it may be shown in place.
func servableType(name string) (string, bool) {
	extension := strings.ToLower(path.Ext(name))
	if kind, ok := inlineTypes[extension]; ok {
		return kind, true
	}
	// Everything else is bytes. Not the type the extension suggests: naming it
	// text/html is most of what makes serving it dangerous.
	return "application/octet-stream", false
}

// filepath takes the last segment of a name that may have arrived with
// separators of either kind in it.
func filepath(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	return name
}

// escapeName makes a filename safe to put in a header.
func escapeName(name string) string {
	var out strings.Builder
	for _, r := range name {
		if r > 32 && r < 127 && !strings.ContainsRune("\"\\;,", r) {
			out.WriteRune(r)
			continue
		}
		for _, b := range []byte(string(r)) {
			out.WriteString(fmt.Sprintf("%%%02X", b))
		}
	}
	return out.String()
}
