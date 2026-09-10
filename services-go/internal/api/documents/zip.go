package documents

// Downloading a whole project.
//
// The zip is written straight to the response rather than built in memory: a
// project is a few hundred kilobytes of text and however many megabytes of
// figures somebody put in it, and holding all of that per request is the kind
// of thing that works until two people do it at once.
//
// The consequence is that a failure part way through cannot be reported. The
// status line and the headers are long gone by then, so the only honest thing
// left is to stop writing, which the browser sees as a truncated download. It
// is logged here so the cause is not a mystery.

import (
	"archive/zip"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
)

// DownloadZip answers with the project's files as a zip.
func (s *Service) DownloadZip(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.readable(r)
	if err != nil {
		return err
	}

	// Everything is decided before the first byte, because after that there is
	// no way to say no.
	historyID := project.HistoryID()
	entries := project.Entries()

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition",
		"attachment; filename*=UTF-8''"+escapeName(zipName(project.Name)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Built on the fly, so there is nothing to cache and no length to promise.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	archive := zip.NewWriter(w)
	defer func() { _ = archive.Close() }()

	for _, entry := range entries {
		switch entry.Kind {
		case projects.EntryFolder:
			// An empty folder is part of what somebody made, and a zip only
			// implies the folders its files are in, so it is written out.
			if _, err := archive.Create(entry.Path + "/"); err != nil {
				s.logZipFailure(r, project.ID.Hex(), entry.Path, err)
				return nil
			}

		case projects.EntryDoc:
			// Through document-updater, for the same reason search goes that
			// way: it is the one that knows what the text is right now,
			// including edits nobody has written back yet.
			doc, err := s.client.Get(r.Context(), project.ID, entry.ID)
			if err != nil {
				// One unreadable document is not a reason to abandon the
				// download; it is one file missing from it.
				s.logZipFailure(r, project.ID.Hex(), entry.Path, err)
				continue
			}
			file, err := archive.Create(entry.Path)
			if err != nil {
				s.logZipFailure(r, project.ID.Hex(), entry.Path, err)
				return nil
			}
			if _, err := io.WriteString(file, strings.Join(doc.Lines, "\n")); err != nil {
				s.logZipFailure(r, project.ID.Hex(), entry.Path, err)
				return nil
			}

		case projects.EntryFile:
			if entry.Hash == "" || historyID == "" {
				continue
			}
			body, err := s.history.ReadBlob(r.Context(), historyID, entry.Hash)
			if err != nil {
				s.logZipFailure(r, project.ID.Hex(), entry.Path, err)
				continue
			}
			file, err := archive.Create(entry.Path)
			if err != nil {
				_ = body.Close()
				s.logZipFailure(r, project.ID.Hex(), entry.Path, err)
				return nil
			}
			_, copyErr := io.Copy(file, body)
			_ = body.Close()
			if copyErr != nil {
				s.logZipFailure(r, project.ID.Hex(), entry.Path, copyErr)
				return nil
			}
		}
	}

	return nil
}

// logZipFailure records what went wrong once the answer is already going out.
func (s *Service) logZipFailure(r *http.Request, projectID, path string, err error) {
	slog.ErrorContext(r.Context(), "could not add a file to the project zip",
		slog.String("project", projectID),
		slog.String("path", path),
		slog.String("err", err.Error()))
}

// zipName is what the downloaded file is called.
//
// The project's name with the separators taken out, because a name with a
// slash in it would be a suggestion to the browser about where to put the
// file.
func zipName(name string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', 0:
			return '-'
		}
		return r
	}, strings.TrimSpace(name))

	if cleaned == "" {
		cleaned = "project"
	}
	return cleaned + ".zip"
}
