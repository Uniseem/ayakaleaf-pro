package docupdater

import (
	"encoding/json"
	"sort"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/rangestracker"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/textot"
)

// Accepting a tracked change does not move the text the editor shows, so with
// history ranges support off there is nothing to record. With it on there is:
// the history holds the tracked deletions, and accepting one is what finally
// takes it out there. Accepting a tracked insert is recorded too, as the moment
// it stopped being marked.
//
// This is the port of RangesManager.getHistoryUpdatesForAcceptedChanges.

// historyUpdatesForAcceptedChanges builds the entries to queue when tracked
// changes are accepted.
func historyUpdatesForAcceptedChanges(docID string, acceptedIDs []string,
	changes []*rangestracker.Change, lines []string, pathname,
	projectHistoryID string) ([]json.RawMessage, error) {

	accepted := make(map[string]bool, len(acceptedIDs))
	for _, id := range acceptedIDs {
		accepted[id] = true
	}

	// In order of position, and where two sit at the same one, the deletion
	// first: it is the one that shifts the positions after it.
	sorted := append([]*rangestracker.Change(nil), changes...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Op.Pos != sorted[j].Op.Pos {
			return sorted[i].Op.Pos < sorted[j].Op.Pos
		}
		return sorted[i].Op.Kind == textot.Delete && sorted[j].Op.Kind == textot.Insert
	})

	docLength := docLength(lines)
	// The history text is longer by every tracked deletion still in it.
	historyDocLength := docLength
	for _, change := range sorted {
		if change.Op.Kind == textot.Delete {
			historyDocLength += change.Op.Len()
		}
	}

	var entries []json.RawMessage
	// A tracked deletion that is not being accepted stays in the history text,
	// so everything after it is that much further along there.
	var unacceptedDeletes int

	for _, change := range sorted {
		op := map[string]any{}
		switch {
		case change.Op.Kind == textot.Delete && accepted[change.ID]:
			op["p"] = change.Op.Pos
			op["d"] = change.Op.Text.String()

		case change.Op.Kind == textot.Delete:
			unacceptedDeletes += change.Op.Len()
			continue

		case change.Op.Kind == textot.Insert && accepted[change.ID]:
			op["p"] = change.Op.Pos
			// A retain that clears the tracking: the text stays, the mark goes.
			op["r"] = change.Op.Text.String()
			op["tracking"] = map[string]any{"type": "none"}

		default:
			continue
		}
		if unacceptedDeletes > 0 {
			op["hpos"] = change.Op.Pos + unacceptedDeletes
		}

		meta := map[string]any{}
		for key, value := range change.Metadata {
			meta[key] = value
		}
		meta["ts"] = nowMillis()
		meta["doc_length"] = docLength
		meta["pathname"] = pathname
		if historyDocLength != docLength {
			meta["history_doc_length"] = historyDocLength
		}

		entry := map[string]any{"doc": docID, "op": []any{op}, "meta": meta}
		if projectHistoryID != "" {
			entry["projectHistoryId"] = projectHistoryID
		}

		encoded, err := json.Marshal(entry)
		if err != nil {
			return nil, err
		}
		entries = append(entries, encoded)

		if change.Op.Kind == textot.Delete {
			// Accepted, so it is gone from the history text too now.
			historyDocLength -= change.Op.Len()
		}
	}
	return entries, nil
}
