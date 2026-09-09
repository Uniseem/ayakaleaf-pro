package documents

import (
	"encoding/json"
	"net/http"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
)

// Tracked changes.
//
// A tracked change is an edit that has been recorded rather than applied: the
// text carries a marker saying who wrote it and when, and somebody with
// authority over the document decides later whether it stays.
//
// The markers live with the document in document-updater, alongside the text
// they refer to, which is the only place they can be kept correct -- a
// position is only meaningful against a particular version, and every edit
// moves it.

// Changes answers with the tracked changes and comment anchors on a document.
func (s *Service) Changes(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.readable(r)
	if err != nil {
		return err
	}
	entry, err := entryIn(project, r.PathValue("docId"))
	if err != nil {
		return err
	}
	if entry.Kind != projects.EntryDoc {
		return apierr.NotFound.WithMessage("That is not a document.")
	}

	doc, err := s.client.Get(r.Context(), project.ID, entry.ID)
	if err != nil {
		return apierr.Internal.WithCause(err)
	}

	ranges := doc.Ranges
	if len(ranges) == 0 {
		ranges = json.RawMessage(`{}`)
	}
	return httpapi.JSON(w, http.StatusOK, map[string]any{"ranges": ranges})
}

// AcceptChanges marks tracked changes as accepted, so the text stands.
//
// Write access, not review access: accepting is the decision a reviewer was
// asked to inform, not one they get to make.
func (s *Service) AcceptChanges(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.writable(r)
	if err != nil {
		return err
	}
	entry, err := entryIn(project, r.PathValue("docId"))
	if err != nil {
		return err
	}
	if entry.Kind != projects.EntryDoc {
		return apierr.NotFound.WithMessage("That is not a document.")
	}

	var in struct {
		ChangeIDs []string `json:"changeIds"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	if len(in.ChangeIDs) == 0 {
		return apierr.BadRequest.WithField("changeIds").
			WithMessage("Nothing was named to accept.")
	}
	if len(in.ChangeIDs) > 1000 {
		return apierr.BadRequest.WithField("changeIds").
			WithMessage("That is more changes than one request may accept.")
	}

	if err := s.client.AcceptChanges(r.Context(), project.ID, entry.ID, in.ChangeIDs); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.NoContent(w)
}
