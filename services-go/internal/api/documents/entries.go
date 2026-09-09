package documents

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/history"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/users"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Making and unmaking the things in a project.
//
// These live here rather than with the rest of the project handlers because
// creating a document is three writes -- the text, the tree, and the history --
// and the service that knows where text goes is this one. A handler that only
// knew about the tree would leave a project pointing at a document that was
// never written, and a history that never heard about it.

// starter is what a new project's first file says.
//
// Enough that pressing compile produces a page rather than an error, and
// little enough that nobody has to delete much before starting.
const starter = "\\documentclass{article}\n" +
	"\n" +
	"\\title{%s}\n" +
	"\\author{}\n" +
	"\\date{\\today}\n" +
	"\n" +
	"\\begin{document}\n" +
	"\\maketitle\n" +
	"\n" +
	"\\end{document}\n"

// SeedNewProject makes a project usable.
//
// Two things have to happen before anybody can do anything with a new project:
// it needs a history, because every change is recorded against one, and it
// needs a file, because an empty project has nothing to open and nothing to
// compile.
func (s *Service) SeedNewProject(ctx context.Context, project *projects.Project, ownerID bson.ObjectID) error {
	if err := s.PrepareNewProject(ctx, project); err != nil {
		return err
	}
	folderID, ok := project.RootFolderID()
	if !ok {
		return errors.New("the project has no root folder")
	}
	lines := strings.Split(strings.Replace(starter, "%s", project.Name, 1), "\n")
	_, err := s.addDoc(ctx, project, folderID, "main.tex", lines, ownerID, "create")
	return err
}

// PrepareNewProject gives a project a history and nothing else.
//
// What a project needs before anything can be written to it. Used on its own
// by anything that brings its own files -- an import, a copy -- where the
// first file this would otherwise make would only have to be deleted again.
func (s *Service) PrepareNewProject(ctx context.Context, project *projects.Project) error {
	historyID, err := s.history.InitialiseProject(ctx, project.ID.Hex())
	if err != nil {
		return err
	}
	if err := s.projects.SetHistoryID(ctx, project.ID, historyID); err != nil {
		return err
	}
	project.Overleaf = &projects.Overleaf{History: projects.History{ID: historyID}}
	return nil
}

// Create makes a document.
func (s *Service) Create(w http.ResponseWriter, r *http.Request) error {
	project, user, err := s.writable(r)
	if err != nil {
		return err
	}
	var in struct {
		Name     string `json:"name"`
		FolderID string `json:"folderId,omitempty"`
		Content  string `json:"content,omitempty"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	name := strings.TrimSpace(in.Name)
	folder, folderID, err := s.folderFor(project, in.FolderID)
	if err != nil {
		return err
	}
	if err := projects.ValidName(name); err != nil {
		return apierr.BadRequest.WithField("name").WithMessage(capitalise(err.Error()) + ".")
	}
	if folder.Taken(name) {
		return apierr.Conflict.WithField("name").
			WithMessage("There is already something called that here.")
	}

	lines := strings.Split(strings.ReplaceAll(in.Content, "\r\n", "\n"), "\n")
	entry, err := s.addDoc(r.Context(), project, folderID, name, lines, user.ID, "editor")
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.JSON(w, http.StatusCreated, map[string]any{"file": entry})
}

// CreateFolder makes a folder.
func (s *Service) CreateFolder(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.writable(r)
	if err != nil {
		return err
	}
	var in struct {
		Name     string `json:"name"`
		FolderID string `json:"folderId,omitempty"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	name := strings.TrimSpace(in.Name)
	folder, folderID, err := s.folderFor(project, in.FolderID)
	if err != nil {
		return err
	}
	if err := projects.ValidName(name); err != nil {
		return apierr.BadRequest.WithField("name").WithMessage(capitalise(err.Error()) + ".")
	}
	if folder.Taken(name) {
		return apierr.Conflict.WithField("name").
			WithMessage("There is already something called that here.")
	}

	// A folder is not recorded in the history: the history is of files, and an
	// empty folder holds none. It starts existing there when something is put
	// in it.
	created := projects.Folder{ID: bson.NewObjectID(), Name: name}
	if _, err := s.projects.AddFolder(r.Context(), project, folderID, created); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.JSON(w, http.StatusCreated, map[string]any{
		"file": projects.Entry{
			ID: created.ID, Name: name, Kind: projects.EntryFolder, Parent: folderID,
			Path: pathIn(project, folderID, name),
		},
	})
}

// Rename changes what something is called.
func (s *Service) Rename(w http.ResponseWriter, r *http.Request) error {
	project, user, err := s.writable(r)
	if err != nil {
		return err
	}
	entry, err := entryIn(project, r.PathValue("entryId"))
	if err != nil {
		return err
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	name := strings.TrimSpace(in.Name)
	if err := projects.ValidName(name); err != nil {
		return apierr.BadRequest.WithField("name").WithMessage(capitalise(err.Error()) + ".")
	}
	folder, ok := project.FolderByID(entry.Parent)
	if ok && !strings.EqualFold(entry.Name, name) && folder.Taken(name) {
		return apierr.Conflict.WithField("name").
			WithMessage("There is already something called that here.")
	}

	version, err := s.projects.RenameEntry(r.Context(), project, entry.ID, name)
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	// A folder's name is part of the path of everything inside it, so the
	// history hears about each of those and not about the folder.
	s.report(r.Context(), project, user.ID, version, "editor", renamesFor(project, entry, name))
	return httpapi.NoContent(w)
}

// Delete removes something from the project.
func (s *Service) Delete(w http.ResponseWriter, r *http.Request) error {
	project, user, err := s.writable(r)
	if err != nil {
		return err
	}
	entry, err := entryIn(project, r.PathValue("entryId"))
	if err != nil {
		return err
	}
	if err := s.removeEntry(r.Context(), project, entry, user.ID, "editor"); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.NoContent(w)
}

// SetRootDoc chooses the document a compile starts from.
func (s *Service) SetRootDoc(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.writable(r)
	if err != nil {
		return err
	}
	var in struct {
		DocID string `json:"docId"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	entry, err := entryIn(project, in.DocID)
	if err != nil {
		return err
	}
	if entry.Kind != projects.EntryDoc {
		return apierr.BadRequest.WithField("docId").
			WithMessage("Only a document can be the one that gets compiled.")
	}
	if err := s.projects.SetRootDoc(r.Context(), project.ID, entry.ID); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.NoContent(w)
}

// --- what other services build on ------------------------------------------

// UpsertDoc writes a text file at a path, whatever was there before.
//
// Used by anything that applies a whole file rather than an edit: a git push,
// an upload, a template. It returns the project as it now is, because every
// change to the tree moves what is after it and a stale copy would write to
// the wrong place.
func (s *Service) UpsertDoc(
	ctx context.Context,
	project *projects.Project,
	path string,
	lines []string,
	userID bson.ObjectID,
	source string,
) (*projects.Project, error) {
	if existing, ok := project.FindPath(path); ok {
		if existing.Kind == projects.EntryDoc {
			// The text goes through document-updater rather than to storage,
			// so anybody with the file open sees it change instead of quietly
			// working on a copy that is about to be overwritten.
			if err := s.client.SetContent(ctx, project.ID, existing.ID, userID, lines, source); err != nil {
				return nil, err
			}
			return project, nil
		}
		// Something else is there. It has to go before this can be written.
		if err := s.removeEntry(ctx, project, existing, userID, source); err != nil {
			return nil, err
		}
		var err error
		if project, err = s.reread(ctx, project, userID); err != nil {
			return nil, err
		}
	}

	project, folderID, err := s.EnsureFolders(ctx, project, parentOf(path), userID, source)
	if err != nil {
		return nil, err
	}
	if _, err := s.addDoc(ctx, project, folderID, nameOf(path), lines, userID, source); err != nil {
		return nil, err
	}
	return s.reread(ctx, project, userID)
}

// UpsertFile writes a binary file at a path.
func (s *Service) UpsertFile(
	ctx context.Context,
	project *projects.Project,
	path string,
	content []byte,
	userID bson.ObjectID,
	source string,
) (*projects.Project, error) {
	historyID := project.HistoryID()
	if historyID == "" {
		return nil, errors.New("the project has no history to store a file in")
	}
	// The bytes go into the blob store under their own hash before the tree
	// mentions them, so the tree never points at something that is not there.
	hash := history.BlobHash(content)
	if err := s.history.UploadBlob(ctx, historyID, hash, content); err != nil {
		return nil, err
	}
	url := s.history.BlobURL(historyID, hash)

	if existing, ok := project.FindPath(path); ok {
		if existing.Kind == projects.EntryFile {
			version, err := s.projects.SetFileHash(ctx, project, existing.ID, hash, int64(len(content)))
			if err != nil {
				return nil, err
			}
			// Recorded as a new file at the same path: the history stores what
			// a path held at a version, and the bytes are what changed.
			s.report(ctx, project, userID, version, source, []StructureUpdate{
				AddedFile(existing.ID, "/"+path, url, hash),
			})
			return s.reread(ctx, project, userID)
		}
		if err := s.removeEntry(ctx, project, existing, userID, source); err != nil {
			return nil, err
		}
		var err error
		if project, err = s.reread(ctx, project, userID); err != nil {
			return nil, err
		}
	}

	project, folderID, err := s.EnsureFolders(ctx, project, parentOf(path), userID, source)
	if err != nil {
		return nil, err
	}
	file := projects.FileRef{
		ID: bson.NewObjectID(), Name: nameOf(path), Hash: hash,
		Size: int64(len(content)),
	}
	version, err := s.projects.AddFile(ctx, project, folderID, file)
	if err != nil {
		return nil, err
	}
	s.report(ctx, project, userID, version, source, []StructureUpdate{
		AddedFile(file.ID, "/"+path, url, hash),
	})
	return s.reread(ctx, project, userID)
}

// DeletePath removes whatever is at a path.
func (s *Service) DeletePath(
	ctx context.Context,
	project *projects.Project,
	path string,
	userID bson.ObjectID,
	source string,
) (*projects.Project, error) {
	entry, ok := project.FindPath(path)
	if !ok {
		return project, nil
	}
	if err := s.removeEntry(ctx, project, entry, userID, source); err != nil {
		return nil, err
	}
	return s.reread(ctx, project, userID)
}

// EnsureFolders makes the folders a path needs, and answers with the one the
// file itself goes in.
func (s *Service) EnsureFolders(
	ctx context.Context,
	project *projects.Project,
	dir string,
	userID bson.ObjectID,
	source string,
) (*projects.Project, bson.ObjectID, error) {
	folderID, ok := project.RootFolderID()
	if !ok {
		return nil, bson.ObjectID{}, errors.New("the project has no root folder")
	}
	if dir == "" {
		return project, folderID, nil
	}

	walked := ""
	for _, name := range strings.Split(dir, "/") {
		if name == "" {
			continue
		}
		walked = joinPath(walked, name)
		if existing, ok := project.FindPath(walked); ok {
			if existing.Kind != projects.EntryFolder {
				return nil, bson.ObjectID{}, errors.New("a file is in the way of " + walked)
			}
			folderID = existing.ID
			continue
		}
		created := projects.Folder{ID: bson.NewObjectID(), Name: name}
		if _, err := s.projects.AddFolder(ctx, project, folderID, created); err != nil {
			return nil, bson.ObjectID{}, err
		}
		fresh, err := s.reread(ctx, project, userID)
		if err != nil {
			return nil, bson.ObjectID{}, err
		}
		project = fresh
		folderID = created.ID
	}
	return project, folderID, nil
}

// --- the shared middles ----------------------------------------------------

// addDoc writes a document's text, puts it in the tree, and tells the history.
func (s *Service) addDoc(
	ctx context.Context,
	project *projects.Project,
	folderID bson.ObjectID,
	name string,
	lines []string,
	userID bson.ObjectID,
	source string,
) (projects.Entry, error) {
	docID := bson.NewObjectID()
	if err := s.storage.Create(ctx, project.ID, docID, lines); err != nil {
		return projects.Entry{}, err
	}
	version, err := s.projects.AddDoc(ctx, project, folderID, projects.DocRef{ID: docID, Name: name})
	if err != nil {
		return projects.Entry{}, err
	}
	path := pathIn(project, folderID, name)
	s.report(ctx, project, userID, version, source, []StructureUpdate{
		AddedDoc(docID, "/"+path, lines),
	})
	return projects.Entry{
		ID: docID, Name: name, Path: path, Kind: projects.EntryDoc, Parent: folderID,
	}, nil
}

// removeEntry takes something out of the tree and tells the history.
func (s *Service) removeEntry(
	ctx context.Context,
	project *projects.Project,
	entry projects.Entry,
	userID bson.ObjectID,
	source string,
) error {
	// Worked out before the change, because afterwards the folder's contents
	// are no longer in the tree to be found.
	gone := removals(project, entry)

	version, err := s.projects.RemoveEntry(ctx, project, entry.ID)
	if err != nil {
		return err
	}
	if entry.Kind == projects.EntryDoc {
		// The tree is what a person sees, so it is updated first, and this
		// failing does not fail the request: what is left behind is a row
		// nothing points at.
		_ = s.storage.Delete(ctx, project.ID, entry.ID, entry.Name)
	}
	s.report(ctx, project, userID, version, source, gone)
	return nil
}

// report tells document-updater what changed, and does not fail the caller if
// it cannot.
//
// The change has already happened: refusing it now would leave the person
// looking at a file that exists with an error saying it does not. What is lost
// is a history entry, which a resync puts back.
func (s *Service) report(
	ctx context.Context,
	project *projects.Project,
	userID bson.ObjectID,
	version int64,
	source string,
	updates []StructureUpdate,
) {
	historyID := project.HistoryID()
	if historyID == "" || len(updates) == 0 {
		return
	}
	_ = s.client.ReportStructure(ctx, project.ID, historyID, userID, version, source, updates)
}

// reread reads the project again, because every change to the tree moves what
// comes after it.
func (s *Service) reread(ctx context.Context, project *projects.Project, userID bson.ObjectID) (*projects.Project, error) {
	fresh, _, err := s.projects.Get(ctx, project.ID, userID)
	if err != nil {
		return nil, err
	}
	return fresh, nil
}

// --- the small pieces ------------------------------------------------------

// removals is what disappears when an entry does: itself, and for a folder
// everything inside it.
func removals(project *projects.Project, entry projects.Entry) []StructureUpdate {
	if entry.Kind == projects.EntryDoc {
		return []StructureUpdate{RenamedDoc(entry.ID, "/"+entry.Path, "")}
	}
	if entry.Kind == projects.EntryFile {
		return []StructureUpdate{RenamedFile(entry.ID, "/"+entry.Path, "")}
	}
	inside := entry.Path + "/"
	updates := []StructureUpdate{}
	for _, child := range project.Entries() {
		if !strings.HasPrefix(child.Path, inside) {
			continue
		}
		switch child.Kind {
		case projects.EntryDoc:
			updates = append(updates, RenamedDoc(child.ID, "/"+child.Path, ""))
		case projects.EntryFile:
			updates = append(updates, RenamedFile(child.ID, "/"+child.Path, ""))
		}
	}
	return updates
}

// renamesFor is what moves when something is renamed: itself, or for a folder
// everything inside it.
func renamesFor(project *projects.Project, entry projects.Entry, name string) []StructureUpdate {
	moved := joinPath(parentOf(entry.Path), name)
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

// LooksLikeText says whether a file's bytes should be kept as a document.
//
// A document is text that can be edited, versioned line by line and compiled;
// anything else is stored as bytes. Getting this wrong in one direction shows
// somebody a screenful of nonsense, and in the other loses an image, so it
// asks the content and not only the name.
func LooksLikeText(name string, content []byte) bool {
	switch strings.ToLower(extensionOf(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".tif", ".tiff", ".ico",
		".webp", ".pdf", ".eps", ".ps", ".zip", ".gz", ".tar", ".7z",
		".ttf", ".otf", ".woff", ".woff2", ".eot",
		".mp3", ".mp4", ".mov", ".avi", ".webm", ".ogg",
		".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".odt":
		return false
	}
	if bytes.IndexByte(content, 0) >= 0 {
		return false
	}
	return utf8.Valid(content)
}

func extensionOf(name string) string {
	if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
		return name[dot:]
	}
	return ""
}

func parentOf(path string) string {
	if slash := strings.LastIndexByte(path, '/'); slash >= 0 {
		return path[:slash]
	}
	return ""
}

func nameOf(path string) string {
	if slash := strings.LastIndexByte(path, '/'); slash >= 0 {
		return path[slash+1:]
	}
	return path
}

func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "/" + name
}

// writable is the check the handlers above start with: the project exists,
// this person may see it, and they may change it.
func (s *Service) writable(r *http.Request) (*projects.Project, *users.User, error) {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return nil, nil, err
	}
	id, err := bson.ObjectIDFromHex(r.PathValue("id"))
	if err != nil {
		return nil, nil, apierr.NotFound
	}
	project, access, err := s.projects.Get(r.Context(), id, user.ID)
	if errors.Is(err, projects.ErrNotFound) {
		return nil, nil, apierr.NotFound
	}
	if err != nil {
		return nil, nil, apierr.Internal.WithCause(err)
	}
	if !access.CanWrite() {
		return nil, nil, apierr.Forbidden.WithMessage("You have read-only access to this project.")
	}
	return project, user, nil
}

// folderFor reads the folder a new thing goes into, defaulting to the root.
func (s *Service) folderFor(project *projects.Project, raw string) (projects.Folder, bson.ObjectID, error) {
	if strings.TrimSpace(raw) == "" {
		id, ok := project.RootFolderID()
		if !ok {
			return projects.Folder{}, bson.ObjectID{}, apierr.Internal.
				WithMessage("This project has no root folder.")
		}
		folder, _ := project.FolderByID(id)
		return folder, id, nil
	}
	id, err := bson.ObjectIDFromHex(raw)
	if err != nil {
		return projects.Folder{}, bson.ObjectID{}, apierr.BadRequest.WithField("folderId").
			WithMessage("That is not a folder in this project.")
	}
	folder, ok := project.FolderByID(id)
	if !ok {
		return projects.Folder{}, bson.ObjectID{}, apierr.BadRequest.WithField("folderId").
			WithMessage("That is not a folder in this project.")
	}
	return folder, id, nil
}

// entryIn reads an id and finds what it names in the project.
func entryIn(project *projects.Project, raw string) (projects.Entry, error) {
	id, err := bson.ObjectIDFromHex(raw)
	if err != nil {
		return projects.Entry{}, apierr.NotFound
	}
	entry, ok := project.Find(id)
	if !ok {
		return projects.Entry{}, apierr.NotFound
	}
	return entry, nil
}

// pathIn is where something new will be, for the answer.
func pathIn(project *projects.Project, folderID bson.ObjectID, name string) string {
	if root, ok := project.RootFolderID(); ok && root == folderID {
		return name
	}
	for _, entry := range project.Entries() {
		if entry.ID == folderID {
			return entry.Path + "/" + name
		}
	}
	return name
}

func capitalise(text string) string {
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

// readable reads the project for a request that only looks at it.
//
// The same as writable without the write check: someone with read-only access
// may open a file, and refusing that would make a shared project unreadable.
func (s *Service) readable(r *http.Request) (*projects.Project, *users.User, error) {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return nil, nil, err
	}
	id, err := bson.ObjectIDFromHex(r.PathValue("id"))
	if err != nil {
		return nil, nil, apierr.NotFound
	}
	project, _, err := s.projects.Get(r.Context(), id, user.ID)
	if errors.Is(err, projects.ErrNotFound) {
		return nil, nil, apierr.NotFound
	}
	if err != nil {
		return nil, nil, apierr.Internal.WithCause(err)
	}
	return project, user, nil
}
