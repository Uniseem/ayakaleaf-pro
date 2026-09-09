package documents

import (
	"io"
	"net/http"
	"strings"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
)

// Copying a project.

// maxCloneEntries bounds one copy.
//
// Every document in the source is read and written one at a time, so a project
// with ten thousand files would hold a request open for minutes. A project
// that large is not one anybody made by hand.
const maxCloneEntries = 2000

// Clone makes a copy of a project, owned by whoever asked for it.
//
// The copy is a new project with new ids, not a share and not a fork with a
// link back: nothing about it refers to the original, and deleting either
// leaves the other whole. Its history starts here, because the copy did.
func (s *Service) Clone(w http.ResponseWriter, r *http.Request) error {
	// Read access is enough to copy. Somebody who may read a project may
	// already save every file out of it; refusing the copy would only make
	// them do it by hand.
	source, user, err := s.readable(r)
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
	if name == "" {
		name = source.Name + " (copy)"
	}

	entries := source.Entries()
	if len(entries) > maxCloneEntries {
		return apierr.BadRequest.
			WithMessage("That project is too large to copy.")
	}

	created, err := s.projects.Create(r.Context(), user.ID, name, source.Compiler)
	if err != nil {
		return apierr.Internal.WithCause(err)
	}

	// From here on a failure has to take the half-made project with it: a
	// project that is missing most of its files is worse than none, because it
	// looks like it worked.
	undo := func() {
		_ = s.projects.Delete(r.Context(), created.ID)
	}

	if err := s.PrepareNewProject(r.Context(), created); err != nil {
		undo()
		return apierr.Internal.WithCause(err)
	}
	copied, err := s.reread(r.Context(), created, user.ID)
	if err != nil {
		undo()
		return apierr.Internal.WithCause(err)
	}

	// Folders before what goes in them, and shallowest first, which is the
	// order Entries walks in.
	for _, entry := range entries {
		switch entry.Kind {
		case projects.EntryFolder:
			next, _, err := s.EnsureFolders(r.Context(), copied, entry.Path, user.ID, "clone")
			if err != nil {
				undo()
				return apierr.Internal.WithCause(err)
			}
			copied = next

		case projects.EntryDoc:
			doc, err := s.client.Get(r.Context(), source.ID, entry.ID)
			if err != nil {
				undo()
				return apierr.Internal.WithCause(err).
					WithMessage("A file in that project could not be read.")
			}
			next, err := s.UpsertDoc(r.Context(), copied, entry.Path, doc.Lines, user.ID, "clone")
			if err != nil {
				undo()
				return apierr.Internal.WithCause(err)
			}
			copied = next

		case projects.EntryFile:
			content, err := s.readBlob(r, source, entry)
			if err != nil {
				undo()
				return err
			}
			next, err := s.UpsertFile(r.Context(), copied, entry.Path, content, user.ID, "clone")
			if err != nil {
				undo()
				return apierr.Internal.WithCause(err)
			}
			copied = next
		}
	}

	// The root document is chosen by path: the copy's ids are all new, so the
	// source's id means nothing here.
	if root, ok := source.Find(source.RootDocID); ok {
		if entry, found := copied.FindPath(root.Path); found {
			if err := s.projects.SetRootDoc(r.Context(), copied.ID, entry.ID); err != nil {
				undo()
				return apierr.Internal.WithCause(err)
			}
		}
	}

	return httpapi.JSON(w, http.StatusCreated, map[string]any{
		"project": map[string]any{
			"id":          copied.ID.Hex(),
			"name":        copied.Name,
			"ownerId":     user.ID.Hex(),
			"access":      "owner",
			"lastUpdated": copied.LastUpdated,
			"archived":    false,
			"trashed":     false,
		},
	})
}

// readBlob reads the bytes of a file in the project being copied.
func (s *Service) readBlob(
	r *http.Request,
	source *projects.Project,
	entry projects.Entry,
) ([]byte, error) {
	historyID := source.HistoryID()
	if historyID == "" || entry.Hash == "" {
		// A file with no stored bytes is copied as an empty one rather than
		// failing the whole copy: the alternative loses the other files too.
		return nil, nil
	}
	body, err := s.history.ReadBlob(r.Context(), historyID, entry.Hash)
	if err != nil {
		return nil, apierr.Internal.WithCause(err).
			WithMessage("A file in that project could not be read.")
	}
	defer func() { _ = body.Close() }()

	content, err := io.ReadAll(io.LimitReader(body, MaxUploadBytes))
	if err != nil {
		return nil, apierr.Internal.WithCause(err)
	}
	return content, nil
}
