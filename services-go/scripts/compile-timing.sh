#!/usr/bin/env bash
#
# Where the time in a LaTeX compile actually goes.
#
# clsi reports its own breakdown in the compile response, so nothing here is
# inferred: `sync` is writing the project to disk, `compile` is latexmk,
# `output` is collecting what it produced, `compute-pdf-caching` is splitting
# the PDF into ranges for incremental download, and `compileE2E` is the whole
# request.
#
# It is run against documents of three sizes, because the question is not how
# long a compile takes but which part of it grows.

set -uo pipefail

CONTAINER=${OVERLEAF_CONTAINER:-overleaf-pro}
RUNS=${RUNS:-5}
SIZES=${SIZES:-"1 20 100"}
# clsi will not take a project id with punctuation in it.
STAMP=$(printf %x $$)

# writeBody <sections> <path>
#
# A document with a table of contents, so latexmk has a reason to run more
# than one pass, and enough text that the PDF is worth splitting.
writeBody() {
  python3 - "$1" "$2" <<'PY'
import json, sys

sections = int(sys.argv[1])
path = sys.argv[2]
paragraph = ('The quick brown fox jumps over the lazy dog. ' * 12) + '\n\n'

body = []
for i in range(sections):
    body.append('\\section{Section %d}' % i)
    body.append(paragraph)
    body.append('\\subsection{Detail %d}' % i)
    body.append(paragraph)

document = (
    '\\documentclass{article}\n'
    '\\usepackage{amsmath}\n'
    '\\begin{document}\n'
    '\\tableofcontents\n'
    + '\n'.join(body)
    + '\n\\end{document}\n'
)

with open(path, 'w') as out:
    json.dump({'compile': {
        'options': {
            'compiler': 'pdflatex',
            'timeout': 60,
            'enablePdfCaching': True,
        },
        'rootResourcePath': 'main.tex',
        'resources': [{'path': 'main.tex', 'content': document}],
    }}, out)
PY
}

report() {
  python3 - <<'PY'
import json, sys

try:
    payload = json.load(sys.stdin)['compile']
except Exception as error:
    print('  could not read the response: %s' % error)
    raise SystemExit

timings = payload.get('timings', {})
stats = payload.get('stats', {})
print(
    '  e2e %-6s = sync %-5s + latexmk %-6s + output %-5s'
    '   (pdf-caching %-5s, %s passes, pdf %s kB)'
    % (
        timings.get('compileE2E'),
        timings.get('sync'),
        timings.get('compile'),
        timings.get('output'),
        timings.get('compute-pdf-caching'),
        stats.get('latex-runs'),
        round(stats.get('pdf-size', 0) / 1024),
    )
)
PY
}

echo "# compile timing, $RUNS runs per size, milliseconds"
echo

for sections in $SIZES; do
  writeBody "$sections" /tmp/compile-body.json
  docker cp /tmp/compile-body.json "$CONTAINER":/tmp/compile-body.json >/dev/null
  bytes=$(wc -c < /tmp/compile-body.json)
  echo "## $sections sections, $((bytes / 1024)) kB of LaTeX"

  # A project id of its own per size, so the first run is a cold compile and
  # the rest reuse the compile directory -- which is what a person editing
  # gets after their first compile.
  project="bench${STAMP}x${sections}"
  for run in $(seq 1 "$RUNS"); do
    docker exec "$CONTAINER" sh -c "
      curl -s -X POST -H 'Content-Type: application/json' \
        --data @/tmp/compile-body.json \
        http://127.0.0.1:3013/project/$project/compile" | report
  done
  echo
done
