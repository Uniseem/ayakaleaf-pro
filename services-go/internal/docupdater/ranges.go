package docupdater

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/rangestracker"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/textot"
)

const (
	// A document with more markers than this is refused: past these numbers the
	// editor cannot render them usefully and every edit costs more to apply
	// than the edit itself.
	maxComments = 500
	maxChanges  = 2000
)

// ErrTooManyRanges is returned for a document that has accumulated more tracked
// changes or comments than the limits above.
var ErrTooManyRanges = errors.New("too many comments or tracked changes")

// rangesResult is what applying an update does to a document's markers.
type rangesResult struct {
	// NewRanges is what should be stored. It carries only the fields that have
	// anything in them: most documents have neither, and writing empty lists
	// for all of them costs more than it says.
	NewRanges json.RawMessage
	// Collapsed reports that markers were emptied or lost, which is when a
	// snapshot of the previous content is worth taking.
	Collapsed bool
	// HistoryOps are the operations to send to project-history.
	HistoryOps textot.Op
	// RemovedChangeIDs are the tracked changes this update did away with. An
	// edit that removes one is a rejection, and the author is told.
	RemovedChangeIDs []string
}

// applyUpdateToRanges moves a document's tracked changes and comments to
// account for an update, and reports what happened to them.
//
// It is the Go port of RangesManager.applyUpdate for the case where history
// ranges support is off, which is every document in a server-ce deployment.
func applyUpdateToRanges(
	ranges json.RawMessage, ops textot.Op, newDocLines []string,
	userID, trackChangesSeed string,
) (*rangesResult, error) {
	changes, comments, err := decodeRanges(ranges)
	if err != nil {
		return nil, err
	}

	tracker := rangestracker.New(changes, comments)
	emptyBefore, totalBefore := countEmptyRanges(tracker)

	// An update carries the id seed to use when it is recording tracked
	// changes; without one it is an ordinary edit and the markers only move.
	tracker.TrackChanges = trackChangesSeed != ""
	if trackChangesSeed != "" {
		tracker.SetIDSeed(trackChangesSeed)
	}

	// A fresh metadata map per op, matching the Node service: applyOp stamps a
	// timestamp into whatever it is given, and a shared map would give every
	// marker the first op's time.
	newMetadata := func() rangestracker.Metadata {
		metadata := rangestracker.Metadata{}
		if userID != "" {
			metadata["user_id"], _ = json.Marshal(userID)
		}
		return metadata
	}

	historyOps := textot.Op{}
	for _, op := range ops {
		// Without history ranges support, only real edits reach history: a
		// comment changes no text and history has nothing to record.
		if op.Kind == textot.Insert || op.Kind == textot.Delete {
			historyOps = append(historyOps, op)
		}
		if err := tracker.ApplyOp(op, newMetadata()); err != nil {
			return nil, err
		}
	}

	if len(tracker.Changes) > maxChanges || len(tracker.Comments) > maxComments {
		return nil, ErrTooManyRanges
	}

	// The markers and the document must still agree. This is the check that
	// catches a mistake in the arithmetic before it is stored.
	if err := tracker.Validate(textot.JoinLines(newDocLines)); err != nil {
		return nil, fmt.Errorf("error validating ranges: %w", err)
	}

	emptyAfter, totalAfter := countEmptyRanges(tracker)
	// Either more markers are empty than were, or several disappeared at once.
	// The "+1" allows for the ordinary case of one being consumed by an edit.
	collapsed := emptyAfter > emptyBefore || totalAfter+1 < totalBefore

	encoded, err := encodeRanges(tracker)
	if err != nil {
		return nil, err
	}

	removed := tracker.ChangeDirtyState().Removed
	removedIDs := make([]string, 0, len(removed))
	for id := range removed {
		removedIDs = append(removedIDs, id)
	}

	return &rangesResult{
		NewRanges: encoded, Collapsed: collapsed,
		HistoryOps: historyOps, RemovedChangeIDs: removedIDs,
	}, nil
}

// decodeRanges reads the stored markers.
func decodeRanges(ranges json.RawMessage) ([]*rangestracker.Change, []*rangestracker.Comment, error) {
	if len(ranges) == 0 {
		return nil, nil, nil
	}
	var stored rangestracker.Ranges
	if err := json.Unmarshal(ranges, &stored); err != nil {
		return nil, nil, fmt.Errorf("malformed ranges: %w", err)
	}
	return stored.Changes, stored.Comments, nil
}

// encodeRanges renders the markers for storage, leaving out what is empty.
func encodeRanges(tracker *rangestracker.Tracker) (json.RawMessage, error) {
	stored := rangestracker.Ranges{}
	if len(tracker.Changes) > 0 {
		stored.Changes = tracker.Changes
	}
	if len(tracker.Comments) > 0 {
		stored.Comments = tracker.Comments
	}
	return json.Marshal(stored)
}

// countEmptyRanges counts the markers, and how many of them cover no text.
func countEmptyRanges(tracker *rangestracker.Tracker) (empty, total int) {
	for _, comment := range tracker.Comments {
		total++
		if comment.Op.Len() == 0 {
			empty++
		}
	}
	for _, change := range tracker.Changes {
		total++
		if change.Op.Kind == textot.Insert && change.Op.Len() == 0 {
			empty++
		}
	}
	return empty, total
}
