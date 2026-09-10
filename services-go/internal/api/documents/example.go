package documents

import (
	"context"
	"embed"
	"errors"
	"path"
	"strings"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// The example project.
//
// A blank project is a blank page, and a blank page is where somebody who has
// not written LaTeX before gets stuck. This is the alternative: a document
// that already does the things they are about to want to do -- a figure, a
// table, an equation, a citation -- so that the way to do each of them is
// there to be copied rather than looked up.
//
// Compiled into the binary rather than read from disk. It is part of the
// program, and a program whose behaviour depends on files being deployed
// beside it has a way of failing that nothing here needs.

//go:embed exampleproject
var exampleFiles embed.FS

const exampleRoot = "exampleproject"

// SeedExampleProject fills a new project with the example.
func (s *Service) SeedExampleProject(
	ctx context.Context, project *projects.Project, ownerID bson.ObjectID,
) error {
	if err := s.PrepareNewProject(ctx, project); err != nil {
		return err
	}
	filled, err := s.reread(ctx, project, ownerID)
	if err != nil {
		return err
	}

	entries, err := exampleFiles.ReadDir(exampleRoot)
	if err != nil {
		return err
	}
	var rootDoc string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		content, err := exampleFiles.ReadFile(path.Join(exampleRoot, name))
		if err != nil {
			return err
		}
		if isText(name, content) {
			lines := strings.Split(normaliseNewlines(string(content)), "\n")
			filled, err = s.UpsertDoc(ctx, filled, name, lines, ownerID, "create")
			if name == "main.tex" {
				rootDoc = name
			}
		} else {
			filled, err = s.UpsertFile(ctx, filled, name, content, ownerID, "create")
		}
		if err != nil {
			return err
		}
	}

	if rootDoc == "" {
		return errors.New("the example project has no main document")
	}
	entry, ok := filled.FindPath(rootDoc)
	if !ok {
		return errors.New("the example project's main document was not stored")
	}
	// Written back onto the caller's project as well: it holds the one that
	// the rest of creating a project reads from.
	project.Overleaf = filled.Overleaf
	return s.projects.SetRootDoc(ctx, filled.ID, entry.ID)
}
