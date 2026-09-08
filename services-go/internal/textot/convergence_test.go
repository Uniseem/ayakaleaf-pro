package textot

import (
	"flag"
	"math/rand/v2"
	"testing"
)

var convergenceCases = flag.Int("convergence-cases", 200000,
	"how many random scenarios the convergence test runs")

// alphabet is deliberately mixed. One-byte, two-byte and three-byte UTF-8
// characters all occupy a single UTF-16 code unit; the emoji occupies two.
// A port that counted bytes passes on the first group and fails on the rest.
var alphabet = []rune{
	'a', 'b', 'c', ' ', '\n',
	'é', 'ß', // two bytes in UTF-8, one code unit
	'中', '日', // three bytes in UTF-8, one code unit
	'\U0001F600', // four bytes in UTF-8, two code units
}

func randomText(rng *rand.Rand, maxLen int) Text {
	n := 1 + rng.IntN(maxLen)
	runes := make([]rune, n)
	for i := range runes {
		runes[i] = alphabet[rng.IntN(len(alphabet))]
	}
	return T(string(runes))
}

// randomOp builds an operation that is valid against snapshot, and returns the
// document it produces.
//
// The components are generated against a running document because that is how
// an operation is defined: each component's position assumes the ones before it
// have already been applied.
func randomOp(rng *rand.Rand, snapshot Text) (Op, Text) {
	op := Op{}
	doc := snapshot

	for n := 1 + rng.IntN(3); n > 0; n-- {
		roll := rng.IntN(100)
		switch {
		case doc.Len() == 0 || roll < 45:
			pos := rng.IntN(doc.Len() + 1)
			text := randomText(rng, 4)
			op = append(op, Component{Kind: Insert, Text: text, Pos: pos})
			doc = doc.Inject(pos, text)

		case roll < 85:
			pos := rng.IntN(doc.Len())
			length := 1 + rng.IntN(min(doc.Len()-pos, 5))
			op = append(op, Component{Kind: Delete, Text: doc.Slice(pos, pos+length), Pos: pos})
			doc = Concat(doc.Slice(0, pos), doc.From(pos+length))

		default:
			// A comment leaves the document unchanged, so doc does not move.
			pos := rng.IntN(doc.Len())
			length := 1 + rng.IntN(min(doc.Len()-pos, 5))
			op = append(op, Component{
				Kind: Comment, Text: doc.Slice(pos, pos+length), Pos: pos,
				Thread: "t" + string(rune('0'+rng.IntN(3))),
			})
		}
	}
	return op, doc
}

// Convergence is the property the whole editor rests on: two people editing at
// once must end up with the same document, whichever order their edits arrive
// in.
//
//	apply(apply(S, B), transform(A, B, left))
//	  ==
//	apply(apply(S, A), transform(B, A, right))
//
// A transform that gets this wrong does not crash. It leaves two collaborators
// looking at different text, and the next edit either is rejected or writes the
// divergence into the stored document.
func TestConvergence(t *testing.T) {
	// A fixed seed keeps a failure reproducible; the subtest name carries the
	// case number so a failing scenario can be re-run on its own.
	rng := rand.New(rand.NewPCG(1, 2))

	for i := 0; i < *convergenceCases; i++ {
		snapshot := randomText(rng, 12)
		opA, docA := randomOp(rng, snapshot)
		opB, docB := randomOp(rng, snapshot)

		transformedA, err := Transform(opA.Clone(), opB, Left)
		if err != nil {
			t.Fatalf("case %d: transform(A, B, left): %v\n  S=%q\n  A=%s\n  B=%s",
				i, err, snapshot.String(), mustJSON(t, opA), mustJSON(t, opB))
		}
		transformedB, err := Transform(opB.Clone(), opA, Right)
		if err != nil {
			t.Fatalf("case %d: transform(B, A, right): %v\n  S=%q\n  A=%s\n  B=%s",
				i, err, snapshot.String(), mustJSON(t, opA), mustJSON(t, opB))
		}

		viaB, err := Apply(docB, transformedA)
		if err != nil {
			t.Fatalf("case %d: applying A' after B: %v\n  S=%q\n  A=%s\n  B=%s\n  A'=%s",
				i, err, snapshot.String(), mustJSON(t, opA), mustJSON(t, opB),
				mustJSON(t, transformedA))
		}
		viaA, err := Apply(docA, transformedB)
		if err != nil {
			t.Fatalf("case %d: applying B' after A: %v\n  S=%q\n  A=%s\n  B=%s\n  B'=%s",
				i, err, snapshot.String(), mustJSON(t, opA), mustJSON(t, opB),
				mustJSON(t, transformedB))
		}

		if !viaA.Equal(viaB) {
			t.Fatalf("case %d: the two orders diverged\n"+
				"  S  = %q\n  A  = %s\n  B  = %s\n  A' = %s\n  B' = %s\n"+
				"  B then A' = %q\n  A then B' = %q",
				i, snapshot.String(), mustJSON(t, opA), mustJSON(t, opB),
				mustJSON(t, transformedA), mustJSON(t, transformedB),
				viaB.String(), viaA.String())
		}
	}
}

// Transform must not alter the operations it is given: document-updater
// transforms the same incoming update against each historical op in turn, and
// reuses the originals for the ranges and history paths.
func TestTransformDoesNotMutateItsInputs(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))

	for i := 0; i < 5000; i++ {
		snapshot := randomText(rng, 12)
		opA, _ := randomOp(rng, snapshot)
		opB, _ := randomOp(rng, snapshot)
		beforeA, beforeB := mustJSON(t, opA), mustJSON(t, opB)

		if _, err := Transform(opA, opB, Left); err != nil {
			continue
		}
		if got := mustJSON(t, opA); got != beforeA {
			t.Fatalf("case %d: transform modified the op being transformed\n  was %s\n  now %s",
				i, beforeA, got)
		}
		if got := mustJSON(t, opB); got != beforeB {
			t.Fatalf("case %d: transform modified the other op\n  was %s\n  now %s",
				i, beforeB, got)
		}
	}
}

// Applying an operation and then its inverse must return the original text.
func TestInvertRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6))

	for i := 0; i < 20000; i++ {
		snapshot := randomText(rng, 12)
		op, after := randomOp(rng, snapshot)

		// Invert is only defined for inserts and deletes.
		hasComment := false
		for _, c := range op {
			if c.Kind == Comment {
				hasComment = true
			}
		}
		if hasComment {
			continue
		}

		back, err := Apply(after, Invert(op))
		if err != nil {
			t.Fatalf("case %d: applying the inverse: %v\n  S=%q\n  op=%s",
				i, err, snapshot.String(), mustJSON(t, op))
		}
		if !back.Equal(snapshot) {
			t.Fatalf("case %d: round trip gave %q, want %q\n  op=%s",
				i, back.String(), snapshot.String(), mustJSON(t, op))
		}
	}
}

func mustJSON(t *testing.T, op Op) string {
	t.Helper()
	return opJSON(t, op)
}
