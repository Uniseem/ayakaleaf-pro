package documents

import (
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
)

// Searching the text of a project.

// Limits on what one search will do.
//
// A project can hold thousands of documents and a search for "e" matches every
// line of all of them. These caps mean a useless search costs a bounded amount
// rather than the whole server, and the client is told the result was cut.
const (
	maxSearchMatchesPerFile = 100
	maxSearchMatchesTotal   = 1000
	maxSearchQueryBytes     = 200
)

type searchMatch struct {
	Line   int    `json:"line"`
	Text   string `json:"text"`
	Column int    `json:"column"`
}

type searchHit struct {
	ID      string        `json:"id"`
	Path    string        `json:"path"`
	Matches []searchMatch `json:"matches"`
}

// Search looks for text in every document in a project.
//
// A scan rather than an index. An index would be faster and is what a hosted
// service needs, but it is another store to keep in step with the documents,
// and for a project of the size one person writes, reading them is fast
// enough that the difference does not show. The caps below are what keeps
// "fast enough" true for a project that is not that size.
func (s *Service) Search(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.readable(r)
	if err != nil {
		return err
	}

	query := r.URL.Query().Get("q")
	if strings.TrimSpace(query) == "" {
		return httpapi.JSON(w, http.StatusOK, map[string]any{"hits": []searchHit{}})
	}
	if len(query) > maxSearchQueryBytes {
		return apierr.BadRequest.WithField("q").WithMessage("That search is too long.")
	}

	caseSensitive, _ := strconv.ParseBool(r.URL.Query().Get("case"))
	wholeWord, _ := strconv.ParseBool(r.URL.Query().Get("word"))

	needle := query
	if !caseSensitive {
		needle = strings.ToLower(needle)
	}

	hits := []searchHit{}
	total := 0

	for _, entry := range project.Entries() {
		if entry.Kind != projects.EntryDoc {
			continue
		}
		if total >= maxSearchMatchesTotal {
			break
		}

		// Through document-updater rather than straight out of docstore,
		// because it is the one that knows what the text is right now --
		// including edits somebody else has made and not yet had written back.
		// A search that cannot see those is a search that lies.
		doc, err := s.client.Get(r.Context(), project.ID, entry.ID)
		if err != nil {
			// A document that cannot be read is not a reason to fail the whole
			// search; it is one file's worth of results missing.
			continue
		}
		lines := doc.Lines

		matches := []searchMatch{}
		for number, line := range lines {
			if len(matches) >= maxSearchMatchesPerFile || total >= maxSearchMatchesTotal {
				break
			}
			haystack := line
			if !caseSensitive {
				haystack = strings.ToLower(haystack)
			}
			for column := 0; ; {
				at := strings.Index(haystack[column:], needle)
				if at == -1 {
					break
				}
				at += column
				column = at + len(needle)
				if wholeWord && !isWholeWord(haystack, at, len(needle)) {
					continue
				}
				matches = append(matches, searchMatch{
					Line:   number,
					Text:   truncateLine(line),
					Column: at,
				})
				total++
				break // one match per line is enough to find the line
			}
		}

		if len(matches) > 0 {
			hits = append(hits, searchHit{
				ID:      entry.ID.Hex(),
				Path:    entry.Path,
				Matches: matches,
			})
		}
	}

	return httpapi.JSON(w, http.StatusOK, map[string]any{
		"hits":      hits,
		"truncated": total >= maxSearchMatchesTotal,
	})
}

// isWholeWord says whether a match sits on word boundaries on both sides.
func isWholeWord(haystack string, at, length int) bool {
	before := at == 0 || !isWordRune(rune(haystack[at-1]))
	end := at + length
	after := end >= len(haystack) || !isWordRune(rune(haystack[end]))
	return before && after
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// truncateLine keeps a very long line from arriving whole.
//
// A generated file can be one line of a hundred thousand characters, and the
// result is only ever shown as one row of context.
func truncateLine(line string) string {
	const limit = 300
	if len(line) <= limit {
		return line
	}
	return line[:limit] + "..."
}
