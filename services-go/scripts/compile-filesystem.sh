#!/usr/bin/env bash
#
# Whether where the compile directory lives makes a difference.
#
# A compile is almost entirely pdflatex, and pdflatex spends its time reading
# style files and writing .aux, .log and .pdf. In a container that all goes
# through the overlay filesystem. This runs the same document from the overlay
# and from memory, with nothing else changed, to see whether the difference is
# worth anything.
#
# It runs pdflatex directly rather than through clsi, so that what is compared
# is the filesystem and not the service.

set -uo pipefail

CONTAINER=${OVERLEAF_CONTAINER:-overleaf-pro}
RUNS=${RUNS:-6}
SECTIONS=${SECTIONS:-100}

python3 - "$SECTIONS" > /tmp/fs-main.tex <<'PY'
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

docker cp /tmp/fs-main.tex "$CONTAINER":/tmp/fs-main.tex >/dev/null

# The runner is written to a file so the quoting stays readable.
cat > /tmp/fs-runner.sh <<'RUNNER'
#!/bin/bash
# usage: fs-runner.sh <directory> <runs>
set -u
dir=$1
runs=$2
rm -rf "$dir"
mkdir -p "$dir"
cp /tmp/fs-main.tex "$dir/main.tex"
cd "$dir" || exit 1

# The first run is cold: no .aux, and the format file may not be in the page
# cache. It is reported separately rather than averaged in.
for i in $(seq 1 "$runs"); do
  start=$(date +%s%N)
  pdflatex -interaction=batchmode -halt-on-error main.tex >/dev/null 2>&1
  end=$(date +%s%N)
  echo $(( (end - start) / 1000000 ))
done
RUNNER
docker cp /tmp/fs-runner.sh "$CONTAINER":/tmp/fs-runner.sh >/dev/null
docker exec "$CONTAINER" chmod +x /tmp/fs-runner.sh

report() {
  local name=$1 dir=$2
  local times
  times=$(docker exec "$CONTAINER" /tmp/fs-runner.sh "$dir" "$RUNS")
  local first rest
  first=$(echo "$times" | head -1)
  rest=$(echo "$times" | tail -n +2 | sort -n)
  local median
  median=$(echo "$rest" | awk '{ a[NR] = $1 } END { print a[int(NR/2) + 1] }')
  printf '%-28s first %5s ms   then %s   median %s ms\n' \
    "$name" "$first" "$(echo "$rest" | tr '\n' ' ')" "$median"
}

echo "# pdflatex on a $SECTIONS-section document, $RUNS runs each"
echo "# where the compile directory lives is the only thing that differs"
echo
docker exec "$CONTAINER" sh -c 'df -h /overleaf/services/clsi/compiles /dev/shm 2>/dev/null | grep -v Filesystem'
echo
report "overlay (as deployed)" /overleaf/services/clsi/compiles/fs-benchmark
report "memory (/dev/shm)" /dev/shm/fs-benchmark
