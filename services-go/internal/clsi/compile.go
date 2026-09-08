package clsi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A compile.

// Resource is one file a compile is given.
type Resource struct {
	Path string `json:"path"`
	// Content is the text of a file the editor holds.
	Content *string `json:"content,omitempty"`
	// URL is where to fetch a file the editor does not: an image, a PDF.
	URL      string `json:"url,omitempty"`
	Modified string `json:"modified,omitempty"`
}

// Request is what a caller asks for.
type Request struct {
	Compile struct {
		Options struct {
			Compiler         string   `json:"compiler"`
			Timeout          int      `json:"timeout"`
			ImageName        string   `json:"imageName"`
			Draft            bool     `json:"draft"`
			StopOnFirstError bool     `json:"stopOnFirstError"`
			SyncType         string   `json:"syncType"`
			Check            string   `json:"check"`
			Flags            []string `json:"flags"`
		} `json:"options"`
		RootResourcePath string     `json:"rootResourcePath"`
		Resources        []Resource `json:"resources"`
	} `json:"compile"`
}

// OutputFile is one thing a compile produced.
type OutputFile struct {
	Path  string `json:"path"`
	Type  string `json:"type,omitempty"`
	Build string `json:"build"`
	Size  int64  `json:"size,omitempty"`
	URL   string `json:"url,omitempty"`
}

// Result is what a compile answers with.
type Result struct {
	Status      string         `json:"status"`
	Error       string         `json:"error,omitempty"`
	OutputFiles []OutputFile   `json:"outputFiles"`
	Stats       map[string]any `json:"stats,omitempty"`
	Timings     map[string]any `json:"timings,omitempty"`
	BuildID     string         `json:"buildId,omitempty"`
}

// Service compiles projects.
type Service struct {
	log     *slog.Logger
	options Options
	runner  runner
	http    *http.Client

	// One compile at a time per project. Two would share a directory and read
	// each other's auxiliary files, which LaTeX does read.
	locks sync.Map
	// running is what can be stopped, by project.
	running sync.Map
}

// New builds it.
func New(log *slog.Logger, options Options) *Service {
	service := &Service{
		log:     log,
		options: options,
		http:    &http.Client{Timeout: 5 * time.Minute},
	}
	if options.Sandboxed {
		service.runner = newDockerRunner(options)
	} else {
		service.runner = newLocalRunner(options)
	}
	return service
}

// Compile runs one.
func (s *Service) Compile(ctx context.Context, projectID, userID string, request *Request) (*Result, error) {
	if !validID(projectID) || (userID != "" && !validID(userID)) {
		return nil, fmt.Errorf("%w: that is not a project", ErrBadRequest)
	}
	unlock := s.lock(compileName(projectID, userID))
	defer unlock()

	options := request.Compile.Options
	compiler := options.Compiler
	if compiler == "" {
		compiler = "pdflatex"
	}
	if _, known := compilerFlags[compiler]; !known {
		return nil, fmt.Errorf("%w: %q is not a compiler this runs", ErrBadRequest, compiler)
	}
	image, err := s.imageFor(options.ImageName)
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(options.Timeout) * time.Second
	if timeout <= 0 || timeout > s.options.MaxTimeout {
		timeout = s.options.MaxTimeout
	}
	root := request.Compile.RootResourcePath
	if root == "" {
		root = "main.tex"
	}
	if err := safePath(root); err != nil {
		return nil, err
	}

	compileDir := s.compileDir(projectID, userID)
	if err := s.writeResources(ctx, compileDir, request.Compile.Resources,
		options.SyncType != "incremental"); err != nil {
		return nil, err
	}
	if options.Draft {
		if err := s.draftMode(compileDir, root); err != nil {
			return nil, err
		}
	}

	started := time.Now()
	run, err := s.runner.Run(ctx, runRequest{
		ProjectID:  projectID,
		UserID:     userID,
		CompileDir: compileDir,
		Command: latexCommand(root, compiler, options.StopOnFirstError,
			options.Flags),
		Image:   image,
		Timeout: timeout,
	}, s.watch(compileName(projectID, userID)))
	elapsed := time.Since(started)

	status := "success"
	message := ""
	switch {
	case run != nil && run.TimedOut:
		status = "timedout"
	case err != nil:
		status = "failure"
		message = err.Error()
	case run != nil && run.ExitCode != 0:
		// latexmk answers non-zero for a document with errors in it, which is
		// a compile that ran and a PDF that may still exist. The log says
		// which, and the caller reads it.
		status = "failure"
	}

	buildID, files, err := s.collectOutputs(projectID, userID, compileDir)
	if err != nil {
		return nil, err
	}
	if status == "failure" && hasPDF(files) {
		// It produced a document despite the errors, which is what LaTeX does
		// most of the time. Saying it failed would hide the pages.
		status = "success"
	}
	if len(files) == 0 && status == "success" {
		status = "failure"
		message = "the compile produced nothing"
	}

	return &Result{
		Status:      status,
		Error:       message,
		OutputFiles: files,
		BuildID:     buildID,
		Timings:     map[string]any{"compile": elapsed.Milliseconds()},
	}, nil
}

// Stop ends a compile that is still running.
func (s *Service) Stop(projectID, userID string) {
	if value, ok := s.running.Load(compileName(projectID, userID)); ok {
		if stop, ok := value.(context.CancelFunc); ok {
			stop()
		}
	}
}

// Clear forgets a project: its files and everything it produced.
func (s *Service) Clear(projectID, userID string) error {
	if !validID(projectID) {
		return fmt.Errorf("%w: that is not a project", ErrBadRequest)
	}
	unlock := s.lock(compileName(projectID, userID))
	defer unlock()

	if err := os.RemoveAll(s.compileDir(projectID, userID)); err != nil {
		return err
	}
	return os.RemoveAll(s.outputDir(projectID, userID))
}

// --- the four steps --------------------------------------------------------

// writeResources puts the project's files in the compile directory.
//
// A full compile clears out what was there first; an incremental one leaves
// it, which is what makes a second compile of the same project fast -- latexmk
// reads the auxiliary files it wrote last time.
func (s *Service) writeResources(ctx context.Context, compileDir string, resources []Resource, full bool) error {
	if full {
		if err := os.RemoveAll(compileDir); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(compileDir, 0o755); err != nil {
		return err
	}

	keep := map[string]bool{}
	for _, resource := range resources {
		if err := safePath(resource.Path); err != nil {
			return err
		}
		keep[filepath.ToSlash(resource.Path)] = true

		if resource.Content != nil {
			if err := writeInside(compileDir, resource.Path, []byte(*resource.Content)); err != nil {
				return err
			}
			continue
		}
		if resource.URL == "" {
			continue
		}
		if err := s.fetch(ctx, compileDir, resource); err != nil {
			return err
		}
	}
	if !full {
		return s.removeUnwanted(compileDir, keep)
	}
	return nil
}

// fetch downloads a file the editor does not hold.
//
// Skipped when the file is already there and no newer: a project's images do
// not change between compiles, and fetching them every time is most of what a
// slow compile is doing.
func (s *Service) fetch(ctx context.Context, compileDir string, resource Resource) error {
	target := filepath.Join(compileDir, filepath.FromSlash(resource.Path))
	if info, err := os.Stat(target); err == nil && resource.Modified != "" {
		if modified, err := strconv.ParseInt(resource.Modified, 10, 64); err == nil {
			if !info.ModTime().Before(time.UnixMilli(modified)) {
				return nil
			}
		}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, resource.URL, nil)
	if err != nil {
		return err
	}
	response, err := s.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 300 {
		// One file that cannot be fetched should not fail the compile: the
		// document may not even use it, and LaTeX says so much better than
		// this can.
		s.log.Warn("a file could not be fetched for a compile",
			slog.String("path", resource.Path), slog.String("status", response.Status))
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	file, err := os.Create(target)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	_, err = io.Copy(file, io.LimitReader(response.Body, 2<<30))
	return err
}

// removeUnwanted deletes files the project no longer has.
//
// Only the ones a compile could have been given: everything latexmk wrote is
// left alone, because that is what makes the next compile incremental.
func (s *Service) removeUnwanted(compileDir string, keep map[string]bool) error {
	return filepath.Walk(compileDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(compileDir, path)
		if err != nil {
			return nil
		}
		slashed := filepath.ToSlash(relative)
		if keep[slashed] || generated[strings.ToLower(filepath.Ext(slashed))] {
			return nil
		}
		if strings.HasPrefix(slashed, buildsDir+"/") {
			return nil
		}
		return os.Remove(path)
	})
}

// generated is what latexmk writes, which is not the project's and is not
// deleted between incremental compiles.
var generated = map[string]bool{
	".aux": true, ".fdb_latexmk": true, ".fls": true, ".log": true,
	".out": true, ".pdf": true, ".synctex": true, ".gz": true, ".toc": true,
	".lof": true, ".lot": true, ".bbl": true, ".blg": true, ".nav": true,
	".snm": true, ".vrb": true, ".idx": true, ".ind": true, ".ilg": true,
	".dvi": true, ".xdv": true, ".run": true,
}

// draftMode makes the document compile without its images, which is faster and
// is what somebody checking whether it builds at all wants.
func (s *Service) draftMode(compileDir, root string) error {
	path := filepath.Join(compileDir, filepath.FromSlash(root))
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	text := string(content)
	// The same substitution the service this replaces makes: the class is
	// asked for draft mode, which every standard class understands.
	replaced := documentClass.ReplaceAllString(text, "\\documentclass[$1,draft]{")
	if replaced == text {
		replaced = documentClassPlain.ReplaceAllString(text, "\\documentclass[draft]{")
	}
	if replaced == text {
		return nil
	}
	return os.WriteFile(path, []byte(replaced), 0o644)
}

// collectOutputs copies what a compile produced into a build directory.
//
// A directory per build, because the address of a PDF has the build in it: a
// reader who is part way through one is not interrupted by the next compile
// replacing the file underneath them.
func (s *Service) collectOutputs(projectID, userID, compileDir string) (string, []OutputFile, error) {
	buildID, err := newBuildID()
	if err != nil {
		return "", nil, err
	}
	target := filepath.Join(s.outputDir(projectID, userID), buildsDir, buildID)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return "", nil, err
	}

	var files []OutputFile
	entries, err := os.ReadDir(compileDir)
	if err != nil {
		return "", nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !wanted[strings.ToLower(filepath.Ext(name))] {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if err := copyFile(filepath.Join(compileDir, name), filepath.Join(target, name)); err != nil {
			continue
		}
		files = append(files, OutputFile{
			Path:  name,
			Type:  strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), "."),
			Build: buildID,
			Size:  info.Size(),
			URL:   outputURL(projectID, userID, buildID, name),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	s.pruneBuilds(projectID, userID)
	return buildID, files, nil
}

// wanted is what is worth keeping out of a compile directory: the document,
// and the files somebody reads when it went wrong.
var wanted = map[string]bool{
	".pdf": true, ".log": true, ".blg": true, ".synctex": true, ".gz": true,
	".dvi": true, ".xdv": true, ".stdout": true, ".stderr": true,
}

// pruneBuilds keeps the last few and removes the rest. Old builds are only
// useful to somebody still reading one, and they are a whole PDF each.
func (s *Service) pruneBuilds(projectID, userID string) {
	const keep = 3
	root := filepath.Join(s.outputDir(projectID, userID), buildsDir)
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) <= keep {
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries[:len(entries)-keep] {
		_ = os.RemoveAll(filepath.Join(root, entry.Name()))
	}
}

// --- the small pieces ------------------------------------------------------

func (s *Service) imageFor(requested string) (string, error) {
	if requested == "" {
		return s.options.Image, nil
	}
	if len(s.options.AllowedImages) == 0 {
		return requested, nil
	}
	for _, allowed := range s.options.AllowedImages {
		if allowed == requested {
			return requested, nil
		}
	}
	return "", fmt.Errorf("%w: %q is not an image this site offers", ErrBadRequest, requested)
}

func (s *Service) lock(name string) func() {
	value, _ := s.locks.LoadOrStore(name, &sync.Mutex{})
	mutex := value.(*sync.Mutex)
	mutex.Lock()
	return mutex.Unlock
}

// watch records how to stop a running compile, and forgets it afterwards.
func (s *Service) watch(name string) func(context.CancelFunc) func() {
	return func(stop context.CancelFunc) func() {
		s.running.Store(name, stop)
		return func() { s.running.Delete(name) }
	}
}

// compilerFlags is which engine latexmk should use.
var compilerFlags = map[string]string{
	"pdflatex": "-pdf",
	"latex":    "-pdfdvi",
	"xelatex":  "-xelatex",
	"lualatex": "-lualatex",
}

// latexCommand is what gets run. $COMPILE_DIR is replaced by whichever runner
// takes it, because the path inside a compile container is not the path here.
func latexCommand(root, compiler string, stopOnFirstError bool, flags []string) []string {
	command := []string{
		"latexmk", "-cd", "-jobname=output",
		"-auxdir=$COMPILE_DIR", "-outdir=$COMPILE_DIR",
		"-synctex=1", "-interaction=batchmode", "-time",
	}
	if stopOnFirstError {
		command = append(command, "-halt-on-error")
	} else {
		// Every pass, errors and all: a document with a mistake in it usually
		// still produces the pages somebody wanted to look at.
		command = append(command, "-f")
	}
	command = append(command, flags...)
	command = append(command, compilerFlags[compiler])
	// A file the editor generates the .tex from is compiled as the .tex.
	root = markupExtension.ReplaceAllString(root, ".tex")
	command = append(command, "$COMPILE_DIR/"+root)
	return command
}

func newBuildID() (string, error) {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	// Time first, so that sorting build directories by name sorts them by age,
	// which is what keeping the last few relies on.
	return strconv.FormatInt(time.Now().UnixMilli(), 16) + "-" + hex.EncodeToString(raw), nil
}

func outputURL(projectID, userID, buildID, name string) string {
	if userID == "" {
		return fmt.Sprintf("/project/%s/build/%s/output/%s", projectID, buildID, name)
	}
	return fmt.Sprintf("/project/%s/user/%s/build/%s/output/%s", projectID, userID, buildID, name)
}

func hasPDF(files []OutputFile) bool {
	for _, file := range files {
		if file.Path == "output.pdf" && file.Size > 0 {
			return true
		}
	}
	return false
}

func copyFile(from, to string) error {
	source, err := os.Open(from)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	target, err := os.Create(to)
	if err != nil {
		return err
	}
	defer func() { _ = target.Close() }()
	_, err = io.Copy(target, source)
	return err
}

// answerJSON is how the HTTP layer replies. Here so both the compile and the
// smaller endpoints use one shape.
func answerJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
