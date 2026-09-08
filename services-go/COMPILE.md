# Can compiling be made faster?

The port made the services around the compile about twice as fast and a fifth
the size. It did nothing for the compile itself, which is the thing a person
actually waits for. So: where does that time go, and is any of it reachable?

Measured on the live deployment with `scripts/compile-timing.sh`,
`scripts/compile-filesystem.sh` and `scripts/compile-latexmk-overhead.sh`.
Every number below is reported by clsi itself or timed around the process it
spawns; none of it is inferred.

## Where the time goes

clsi reports its own breakdown in every compile response. Milliseconds, five
runs per size, the first of each being a cold compile with no `.aux` yet.

| Document | Pass | End to end | Writing files | latexmk | Collecting output |
| --- | --- | ---: | ---: | ---: | ---: |
| 1 kB | first | 435 | 2 | 431 | 2 |
| 1 kB | after | 238–255 | 2 | 234–250 | 2–3 |
| 22 kB | first | 644 | 2 | 640 | 2 |
| 22 kB | after | 279–322 | 1–2 | 276–317 | 2–3 |
| 111 kB | first | 1094 | 1 | 1090 | 3 |
| 111 kB | after | 438–508 | 2–4 | 433–502 | 2–4 |

**clsi's own work is 4 to 7 milliseconds, and it does not grow with the
document.** Everything else is the process it spawns.

So the answer to the question as asked — can rewriting clsi make compiling
faster — is no. There is at most 7 ms of it in a 250–1100 ms compile, and
making that zero would be under 2%.

The first compile of a project runs two or three passes and later ones run
one, which is why the first is roughly twice the rest. That mechanism already
works: clsi keeps the compile directory between compiles, so the `.aux` and
`.toc` survive and LaTeX converges immediately.

## It is not the disk

A compile is mostly file I/O from LaTeX's point of view: reading style files,
writing `.aux`, `.log` and `.pdf`. In a container that goes through the overlay
filesystem, so the obvious guess is that putting the compile directory in
memory would help.

The same document, `pdflatex` run directly, nothing else changed:

| Compile directory | Median |
| --- | ---: |
| container overlay, as deployed | 212 ms |
| memory (`/dev/shm`) | 208 ms |

Within noise. The page cache is already absorbing it, and the compile is
CPU-bound in LaTeX rather than waiting on the disk. This is worth knowing
mainly because it rules out the change everybody suggests first.

## It is partly latexmk

clsi does not run `pdflatex`. It runs `latexmk`, which works out how many
passes are needed, tracks which files the document depends on, and runs
`bibtex` and friends when they are needed. That decision is worth having. But
`latexmk` is a Perl program that starts, reads its dependency database, stats
everything in it, and writes it back, on every compile.

The same 111 kB document in a warm directory:

| | Median |
| --- | ---: |
| `pdflatex`, one pass | 218 ms |
| `latexmk`, document changed | 440 ms |
| `latexmk`, document unchanged | 428 ms |
| `latexmk`, typesetting replaced by `/bin/true` | 130 ms |

clsi reports one pass for these compiles, so the typesetting in the 440 ms is
the same 218 ms. **The other 222 ms is latexmk deciding.** More than half the
time of an ordinary recompile is spent working out what to do rather than
doing it.

The last row is the same decision with the typesetting stubbed out; it is
lower than 222 ms because with nothing written, latexmk skips some of its
post-processing. Take it as a floor rather than the figure.

The third row is worth its own line: **428 ms to conclude that nothing needs
doing at all.**

## So: yes, but not where you would look

Rewriting clsi buys nothing. Moving the compile directory buys nothing. What
is actually there is that half of a warm recompile is `latexmk` overhead, and
that is reachable — but only carefully.

The reason to keep `latexmk` is the cases it exists for: a bibliography needs
`bibtex` and then two more passes; a changed `\label` needs a second pass to
settle the cross-references; an index or a glossary needs its own tool in
between. Deciding that wrongly does not make a slow PDF, it makes a wrong one,
with `??` where the references should be — and it would be wrong quietly.

What that suggests is a fast path rather than a replacement: when the compile
directory is warm, the only thing that changed is the root document, and the
`.fdb_latexmk` shows no bibliography, index or glossary in play, run `pdflatex`
once directly and check the log for the reruns it asks for. Anything else — a
first compile, a changed dependency, any of the auxiliary tools — falls back to
`latexmk` unchanged. That is the common case for somebody typing, it is about
half the wall time, and the fallback means a mistake in the fast path costs a
slower compile rather than a wrong one.

That is a change to clsi's logic. It has nothing to do with which language
clsi is written in, and it can be made without porting it.

## What was not measured

- One machine, one document shape, no concurrent compiles. A document with
  TikZ, a large bibliography or many included files spends its time
  differently.
- `SANDBOXED_COMPILES` is off here. With it on, each compile starts a
  container, which is its own several hundred milliseconds.
- `ENABLE_PDF_CACHING` is off here, so the PDF splitting never ran. That is
  the one part of clsi with real CPU work in it, and it is unmeasured.
