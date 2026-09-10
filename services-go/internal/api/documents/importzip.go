package documents

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
)

// Making a project out of a zip file.
//
// This is how somebody arrives with work that already exists -- a paper from a
// journal's template, a thesis from a colleague, a project exported from
// somewhere else. It is the same shape as a copy: a new project, prepared and
// then filled in, and torn down again if any part of it fails, because a
// project missing half its files is worse than no project at all. It looks
// like it worked.

// What one zip may contain.
//
// The compressed size is bounded by the request; these bound what it expands
// to, which is the number that matters. A few hundred megabytes of text from a
// few kilobytes of zip is an old trick, and the count matters as much as the
// size: every entry is a round trip to the blob store and to Mongo.
const (
	maxImportBytes   = 300 << 20
	maxImportEntries = 2000
)

// Text this large is stored as a file rather than as a document.
//
// The editor holds a document in memory and sends every keystroke through the
// operation log. Past a point that stops being an editor and starts being a
// way to lock up a browser, and the file is more useful as something to
// download than as something to open.
const maxDocBytes = 2 << 20

// The extensions the editor can open, from the original's own list.
//
// A name rather than a guess at the content: a .tex file of pure ASCII and a
// .png of pure ASCII are not the same thing, and the person who made the zip
// said which is which by naming them.
var textExtensions = map[string]bool{
	".tex": true, ".latex": true, ".sty": true, ".cls": true, ".bst": true,
	".bib": true, ".bibtex": true, ".txt": true, ".tikz": true, ".mtx": true,
	".rtex": true, ".md": true, ".asy": true, ".lbx": true, ".bbx": true,
	".cbx": true, ".m": true, ".lco": true, ".dtx": true, ".ins": true,
	".ist": true, ".def": true, ".clo": true, ".ldf": true, ".rmd": true,
	".qmd": true, ".lua": true, ".py": true, ".gv": true, ".mf": true,
	".yml": true, ".yaml": true, ".lhs": true, ".lean": true, ".lean4": true,
	".hs": true, ".mk": true, ".xmpdata": true, ".cfg": true, ".rnw": true,
	".ltx": true, ".inc": true,
}

// Files with no extension that are still text, for the same reason.
var textFilenames = map[string]bool{
	"latexmkrc": true, ".latexmkrc": true,
	"makefile": true, "gnumakefile": true,
}

// Directories that are somebody's tooling rather than their project.
var skippedDirectories = map[string]bool{
	"__macosx": true, ".git": true, ".texpadtmp": true, ".r": true,
	".svn": true, ".hg": true,
}

// What LaTeX writes while compiling, which the compiler will write again.
//
// Importing them is not merely untidy: a stale .aux or .bbl is read in
// preference to being regenerated, so a zip made mid-compile can produce a
// document that will not build until somebody works out which invisible file
// is wrong.
var skippedExtensions = map[string]bool{
	".dvi": true, ".aux": true, ".log": true, ".toc": true, ".out": true,
	".pdfsync": true, ".synctex": true, ".fdb_latexmk": true, ".fls": true,
	".nlo": true, ".ind": true, ".glo": true, ".gls": true, ".glg": true,
	".bbl": true, ".blg": true, ".swp": true, ".gz": true,
}

// The line that says a file is the one to compile, and the title in it.
var (
	documentClassLine = regexp.MustCompile(`(?m)^\s*\\documentclass\b`)
	titleInBraces     = regexp.MustCompile(`\\[tT]itle\*?\s*\{([^}]+)\}`)
	titleInBrackets   = regexp.MustCompile(`\\[tT]itle\s*\[([^\]]+)\]`)
)

// ImportZip makes a project out of an uploaded zip file.
func (s *Service) ImportZip(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		return apierr.BadRequest.WithCause(err).WithMessage(fmt.Sprintf(
			"That upload could not be read, or is larger than %d MB.",
			maxImportBytes>>20))
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	upload, header, err := r.FormFile("file")
	if err != nil {
		return apierr.BadRequest.WithField("file").WithMessage("A file is required.")
	}
	defer func() { _ = upload.Close() }()

	archive, err := io.ReadAll(upload)
	if err != nil {
		return apierr.BadRequest.WithCause(err).WithMessage("That file could not be read.")
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return apierr.BadRequest.WithField("file").
			WithMessage("That is not a zip file.")
	}

	contents, err := readArchive(reader)
	if err != nil {
		return err
	}
	if len(contents) == 0 {
		return apierr.BadRequest.WithField("file").
			WithMessage("There is nothing in that zip file that can go in a project.")
	}

	// A zip made by right-clicking a folder holds one folder holding
	// everything. Keeping it would put the whole project one level down, and
	// every path in it would be wrong by that level.
	contents = withoutCommonRoot(contents)

	root, title := rootDocumentIn(contents)

	// The name asked for, then the document's own title, then the file's.
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = title
	}
	if name == "" && header != nil {
		name = strings.TrimSuffix(path.Base(header.Filename), ".zip")
	}
	if name == "" {
		name = "Imported project"
	}
	if err := projects.ValidName(name); err != nil {
		return apierr.BadRequest.WithField("name").
			WithMessage(capitalise(err.Error()) + ".")
	}

	project, err := s.projects.Create(r.Context(), user.ID, name, "")
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	// From here a failure takes the half-made project with it.
	undo := func() { _ = s.projects.Delete(r.Context(), project.ID) }

	if err := s.PrepareNewProject(r.Context(), project); err != nil {
		undo()
		return apierr.Internal.WithCause(err)
	}
	filled, err := s.reread(r.Context(), project, user.ID)
	if err != nil {
		undo()
		return apierr.Internal.WithCause(err)
	}

	for _, entry := range contents {
		var next *projects.Project
		var writeErr error
		if entry.text {
			lines := strings.Split(normaliseNewlines(string(entry.content)), "\n")
			next, writeErr = s.UpsertDoc(
				r.Context(), filled, entry.path, lines, user.ID, "upload")
		} else {
			next, writeErr = s.UpsertFile(
				r.Context(), filled, entry.path, entry.content, user.ID, "upload")
		}
		if writeErr != nil {
			undo()
			return apierr.Internal.WithCause(writeErr).WithMessage(
				fmt.Sprintf("%q could not be stored, so the project was not made.", entry.path))
		}
		filled = next
	}

	if root != "" {
		if entry, ok := filled.FindPath(root); ok {
			if err := s.projects.SetRootDoc(r.Context(), filled.ID, entry.ID); err != nil {
				undo()
				return apierr.Internal.WithCause(err)
			}
		}
	}

	return httpapi.JSON(w, http.StatusCreated, map[string]any{
		"project": map[string]any{
			"id":          filled.ID.Hex(),
			"name":        filled.Name,
			"ownerId":     user.ID.Hex(),
			"access":      "owner",
			"lastUpdated": filled.LastUpdated,
			"archived":    false,
			"trashed":     false,
		},
	})
}

// importEntry is one file on its way into a project.
type importEntry struct {
	path    string
	content []byte
	text    bool
}

// readArchive reads what the zip holds, refusing what will not fit.
func readArchive(reader *zip.Reader) ([]importEntry, error) {
	var entries []importEntry
	var total int64

	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		name, ok := safeArchivePath(file.Name)
		if !ok {
			continue
		}
		// Checked before reading, from what the zip claims. A zip can lie
		// about this, which is why the running total is checked as well.
		total += int64(file.UncompressedSize64)
		if total > maxImportBytes {
			return nil, apierr.BadRequest.WithField("file").WithMessage(fmt.Sprintf(
				"That zip file expands to more than %d MB.", maxImportBytes>>20))
		}
		if len(entries) >= maxImportEntries {
			return nil, apierr.BadRequest.WithField("file").WithMessage(fmt.Sprintf(
				"That zip file holds more than %d files.", maxImportEntries))
		}

		opened, err := file.Open()
		if err != nil {
			return nil, apierr.BadRequest.WithCause(err).
				WithMessage(fmt.Sprintf("%q could not be read out of that zip file.", name))
		}
		content, err := io.ReadAll(io.LimitReader(opened, maxImportBytes))
		_ = opened.Close()
		if err != nil {
			return nil, apierr.BadRequest.WithCause(err).
				WithMessage(fmt.Sprintf("%q could not be read out of that zip file.", name))
		}

		entries = append(entries, importEntry{
			path:    name,
			content: content,
			text:    isText(name, content),
		})
	}
	return entries, nil
}

// safeArchivePath is the path an entry may be written to, if any.
//
// A name in a zip is whatever the program that wrote it chose to put there,
// including "../../etc/passwd" and "C:\Windows". Anything that is not a plain
// relative path inside the project is refused rather than corrected, because
// there is no reading of those under which they are what somebody meant.
func safeArchivePath(name string) (string, bool) {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimPrefix(name, "./")
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\x00") {
		return "", false
	}
	// Volume names, which Windows zips carry and POSIX paths never do.
	if len(name) > 1 && name[1] == ':' {
		return "", false
	}

	cleaned := path.Clean(name)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", false
	}

	segments := strings.Split(cleaned, "/")
	for index, segment := range segments {
		if segment == "" {
			return "", false
		}
		last := index == len(segments)-1
		lower := strings.ToLower(segment)
		if skippedDirectories[lower] {
			return "", false
		}
		// Hidden files are tooling, apart from the one that is configuration.
		if strings.HasPrefix(segment, ".") && !textFilenames[lower] {
			return "", false
		}
		if last {
			if skippedExtensions[strings.ToLower(path.Ext(segment))] {
				return "", false
			}
			if projects.ValidName(segment) != nil {
				return "", false
			}
		} else if projects.ValidName(segment) != nil {
			return "", false
		}
	}
	return cleaned, true
}

// withoutCommonRoot lifts a project out of the single folder holding it.
func withoutCommonRoot(entries []importEntry) []importEntry {
	root := ""
	for _, entry := range entries {
		slash := strings.IndexByte(entry.path, '/')
		if slash < 0 {
			// Something is at the top already, so there is no single folder.
			return entries
		}
		first := entry.path[:slash]
		if root == "" {
			root = first
		} else if root != first {
			return entries
		}
	}
	if root == "" {
		return entries
	}
	lifted := make([]importEntry, 0, len(entries))
	for _, entry := range entries {
		entry.path = strings.TrimPrefix(entry.path, root+"/")
		lifted = append(lifted, entry)
	}
	return lifted
}

// isText decides whether something can be opened in the editor.
//
// The name decides what it is meant to be and the content decides whether it
// can be: the history stores a document as text, so anything that is not
// valid UTF-8, or holds a NUL, is a file however it is named.
func isText(name string, content []byte) bool {
	base := strings.ToLower(path.Base(name))
	if !textExtensions[strings.ToLower(path.Ext(name))] && !textFilenames[base] {
		return false
	}
	if len(content) > maxDocBytes {
		return false
	}
	if bytes.IndexByte(content, 0) >= 0 {
		return false
	}
	return utf8.Valid(content)
}

// rootDocumentIn picks the file to compile, and reads its title.
//
// The first one that declares a document class, shallowest first, which is
// where a paper's main file almost always is. Getting it wrong costs one
// click in the file tree; not choosing at all leaves a project that will not
// compile until somebody works out that it needs to be told.
func rootDocumentIn(entries []importEntry) (root, title string) {
	best := -1
	for index, entry := range entries {
		if !entry.text || !strings.EqualFold(path.Ext(entry.path), ".tex") {
			continue
		}
		if !documentClassLine.Match(entry.content) {
			continue
		}
		if best < 0 || fewerFolders(entry.path, entries[best].path) {
			best = index
		}
	}
	if best < 0 {
		return "", ""
	}
	return entries[best].path, titleIn(string(entries[best].content))
}

func fewerFolders(a, b string) bool {
	depthA, depthB := strings.Count(a, "/"), strings.Count(b, "/")
	if depthA != depthB {
		return depthA < depthB
	}
	return a < b
}

// titleIn reads what the document calls itself.
func titleIn(content string) string {
	if len(content) > 30000 {
		content = content[:30000]
	}
	for _, pattern := range []*regexp.Regexp{titleInBraces, titleInBrackets} {
		if found := pattern.FindStringSubmatch(content); found != nil {
			title := strings.TrimSpace(found[1])
			// A title is a line of LaTeX, not a name: it can carry commands,
			// comments and line breaks, none of which belong in a project's
			// name. If cleaning it leaves nothing usable, the name is left to
			// the file's own.
			title = strings.NewReplacer("\\\\", " ", "\n", " ", "\r", " ").Replace(title)
			title = strings.Join(strings.Fields(title), " ")
			if title != "" && projects.ValidName(title) == nil {
				return title
			}
		}
	}
	return ""
}

// normaliseNewlines makes one kind of line ending out of three.
func normaliseNewlines(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}
