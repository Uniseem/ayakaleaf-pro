#!/usr/bin/env bash
#
# What latexmk costs on top of the typesetting it does.
#
# clsi does not run pdflatex; it runs latexmk, which decides how many passes
# are needed, tracks which files the document depends on, and runs bibtex and
# friends when they are needed. That decision is worth having -- but it is a
# Perl program that starts, reads its database, scans, and writes it back on
# every compile, and none of that is typesetting.
#
# This runs the same document three ways in the same warm directory: pdflatex
# once, latexmk as clsi invokes it, and latexmk told there is nothing to do.
# The difference between the first two is what the decision costs.

set -uo pipefail

CONTAINER=${OVERLEAF_CONTAINER:-overleaf-pro}
RUNS=${RUNS:-6}
SECTIONS=${SECTIONS:-100}

python3 - "$SECTIONS" > /tmp/lm-main.tex <<'PY'
import sys

sections = int(sys.argv[1])
paragraph = ('The quick brown fox jumps over the lazy dog. ' * 12) + '\n\n'

body = []
for i in range(sections):
    body.append('\\section{Section %d}' % i)
    body.append(paragraph)
    body.append('\\subsection{Detail %d}' % i)
    body.append(paragraph)

print(
    '\\documentclass{article}\n'
    '\\usepackage{amsmath}\n'
    '\\begin{document}\n'
    '\\tableofcontents\n'
    + '\n'.join(body)
    + '\n\\end{document}'
)
PY

docker cp /tmp/lm-main.tex "$CONTAINER":/tmp/lm-main.tex >/dev/null

cat > /tmp/lm-runner.sh <<'RUNNER'
#!/bin/bash
# usage: lm-runner.sh <runs>
set -u
runs=$1
dir=/overleaf/services/clsi/compiles/latexmk-benchmark
rm -rf "$dir"
mkdir -p "$dir"
cp /tmp/lm-main.tex "$dir/main.tex"
cd "$dir" || exit 1

timed() {
  local start end
  start=$(date +%s%N)
  "$@" >/dev/null 2>&1
  end=$(date +%s%N)
  echo $(( (end - start) / 1000000 ))
}

# Warm the directory: after this there is an .aux, a .toc and an .fdb_latexmk,
# which is the state a project is in for every compile after the first.
pdflatex -interaction=batchmode -halt-on-error main.tex >/dev/null 2>&1
pdflatex -interaction=batchmode -halt-on-error main.tex >/dev/null 2>&1
latexmk -pdf -f -interaction=batchmode main.tex >/dev/null 2>&1

echo "pdflatex"
for i in $(seq 1 "$runs"); do
  timed pdflatex -interaction=batchmode -halt-on-error main.tex
done

echo "latexmk"
for i in $(seq 1 "$runs"); do
  # Touched so latexmk has a reason to typeset, which is the ordinary case:
  # somebody changed the document and asked for a compile.
  touch main.tex
  timed latexmk -pdf -f -interaction=batchmode -synctex=1 main.tex
done

echo "latexmk-nothing-to-do"
for i in $(seq 1 "$runs"); do
  timed latexmk -pdf -f -interaction=batchmode -synctex=1 main.tex
done
RUNNER
docker cp /tmp/lm-runner.sh "$CONTAINER":/tmp/lm-runner.sh >/dev/null
docker exec "$CONTAINER" chmod +x /tmp/lm-runner.sh

echo "# a $SECTIONS-section document in a warm directory, $RUNS runs each"
echo
docker exec "$CONTAINER" /tmp/lm-runner.sh "$RUNS" | python3 -c '
import sys

current = None
groups = {}
order = []
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    if not line.isdigit():
        current = line
        groups[current] = []
        order.append(current)
        continue
    groups[current].append(int(line))

for name in order:
    values = sorted(groups[name])
    if not values:
        continue
    median = values[len(values) // 2]
    print("%-24s median %5d ms   %s" % (name, median, values))
'
