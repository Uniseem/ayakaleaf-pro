// Package clsi runs LaTeX.
//
// A compile is four steps: write the project's files into a directory, run
// latexmk over them, collect what it produced into a build directory, and hand
// back a list of what is there. The files themselves are served straight off
// the disk by nginx, so nothing here streams a PDF.
//
// Two ways to run it. On a deployment that trusts its users, latexmk runs in
// this container as an unprivileged user. On one that does not, each compile
// runs in a container of its own with only that project's directory mounted,
// which is the only way to be sure one project's macros cannot read another
// project's files.
package clsi

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	// ErrBadRequest is a compile request that could never run.
	ErrBadRequest = errors.New("bad compile request")
	// ErrTimedOut is a compile that ran too long and was stopped.
	ErrTimedOut = errors.New("the compile took too long")
	// ErrNotFound is a project with nothing compiled.
	ErrNotFound = errors.New("nothing here")
)

// Options is how this service is set up.
type Options struct {
	// CompilesDir is where a project's files are written.
	CompilesDir string
	// OutputDir is where what a compile produced is kept, in the layout nginx
	// serves from.
	OutputDir string
	// User is who latexmk runs as when compiles are not sandboxed.
	User string
	// Sandboxed runs each compile in a container of its own.
	Sandboxed bool
	// Image is the TeX Live container a sandboxed compile runs in when the
	// request does not name one.
	Image string
	// AllowedImages is what a request may ask for. A request naming anything
	// else is refused rather than run: the image name arrives from a project
	// and a deployment decides what it will start.
	AllowedImages []string
	// HostCompilesDir and HostOutputDir are the same two directories as the
	// docker daemon sees them. They differ from the paths above whenever this
	// service is itself in a container, which is always.
	HostCompilesDir string
	HostOutputDir   string
	// MaxTimeout caps what a request may ask for.
	MaxTimeout time.Duration
	// ProjectLife is how long a project's files are kept after its last
	// compile.
	ProjectLife time.Duration
	// SeccompProfile is a path on the host, for a runtime that needs one.
	SeccompProfile string
}

// compileName is the directory one project's compiles happen in.
//
// The user is part of it: two people compiling one project at the same time
// would otherwise overwrite each other's auxiliary files halfway through, and
// LaTeX reads those back.
func compileName(projectID, userID string) string {
	if userID == "" {
		return projectID
	}
	return projectID + "-" + userID
}

func (s *Service) compileDir(projectID, userID string) string {
	return filepath.Join(s.options.CompilesDir, compileName(projectID, userID))
}

func (s *Service) outputDir(projectID, userID string) string {
	return filepath.Join(s.options.OutputDir, compileName(projectID, userID))
}

// buildsDir is where finished compiles are kept, one directory per build. The
// name is the one nginx has in its config, so it is not ours to change alone.
const buildsDir = "generated-files"

// idPattern is what a project or user id may be. Both end up in a path.
var idPattern = regexp.MustCompile(`^[0-9a-zA-Z_-]+$`)

func validID(id string) bool { return id != "" && idPattern.MatchString(id) }

// safePath says whether a resource path can be written inside the compile
// directory.
//
// The one check that matters here: everything in a compile request came from
// somebody's project, and a path that climbs out of the directory writes
// wherever this process can write.
func safePath(path string) error {
	if path == "" {
		return fmt.Errorf("%w: a file with no name", ErrBadRequest)
	}
	if strings.ContainsRune(path, 0) {
		return fmt.Errorf("%w: a file name with a null in it", ErrBadRequest)
	}
	cleaned := filepath.ToSlash(filepath.Clean("/" + path))
	if cleaned != "/"+path {
		return fmt.Errorf("%w: %q is not a plain path", ErrBadRequest, path)
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".." || part == "." || part == "" {
			return fmt.Errorf("%w: %q is not a plain path", ErrBadRequest, path)
		}
	}
	return nil
}

// writeInside writes a file under a directory, refusing anything that would
// land outside it.
func writeInside(root, path string, content []byte) error {
	if err := safePath(path); err != nil {
		return err
	}
	target := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, content, 0o644)
}
