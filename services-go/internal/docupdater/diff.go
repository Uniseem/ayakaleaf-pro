package docupdater

import (
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/textdiff"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/textot"
)

// diffTimeout is how long the diff is given before it settles for a coarser
// answer.
//
// The result is still correct when it runs out, only less minimal, so the cost
// of stopping is a larger operation in the history rather than a wrong
// document. Matching the Node service, which uses the same limit.
const diffTimeout = 100 * time.Millisecond

// diffAsShareJSOp works out the edit that turns one version of a document into
// another.
//
// This is what a write through the HTTP API becomes. The caller hands over the
// finished document, but everything downstream -- the editors watching it, the
// tracked changes, the history -- is expressed as operations, so the difference
// has to be recovered before the new text can be stored.
func diffAsShareJSOp(before, after []string) textot.Op {
	diffs := textdiff.Main(textot.JoinLines(before), textot.JoinLines(after),
		true, diffTimeout)
	diffs = textdiff.CleanupSemantic(diffs)

	op := textot.Op{}
	var position int
	for _, diff := range diffs {
		switch diff.Kind {
		case textdiff.Insert:
			op = append(op, textot.Component{
				Kind: textot.Insert, Pos: position, Text: textot.Text(diff.Text),
			})
			position += len(diff.Text)
		case textdiff.Delete:
			// No advance of the position: the deleted text is not in the new
			// document, so the next component starts where this one does.
			op = append(op, textot.Component{
				Kind: textot.Delete, Pos: position, Text: textot.Text(diff.Text),
			})
		case textdiff.Equal:
			position += len(diff.Text)
		}
	}
	return op
}
