#!/usr/bin/env bash
#
# Measures the deployment as it is: how much memory each service holds and how
# long the operations the editor actually makes take.
#
# It is meant to be run twice -- once with the Go services selected and once
# with the Node ones -- against the same container, the same database and the
# same project, so that the two runs differ in nothing but which
# implementation is listening. Everything is timed from inside the container,
# so what is measured is the service rather than the network to it.
#
#   ./benchmark.sh go > /tmp/go.txt
#   ./benchmark.sh node > /tmp/node.txt
#
# The label is only written into the output; it does not select anything.

set -uo pipefail

LABEL=${1:-current}
BASE=http://127.0.0.1:8080
EMAIL=${OVERLEAF_EMAIL:-admin@test.local}
PASSWORD=${OVERLEAF_PASSWORD:-'TestPassw0rd!2026'}
CONTAINER=${OVERLEAF_CONTAINER:-overleaf-pro}
MONGO_CONTAINER=${OVERLEAF_MONGO_CONTAINER:-deploy-mongo-live-1}
# Enough iterations that a single slow one does not decide the answer, few
# enough that a run finishes in a couple of minutes.
ITERATIONS=${ITERATIONS:-40}
WARMUP=${WARMUP:-5}

incontainer() { docker exec "$CONTAINER" sh -c "$1"; }
inmongo() {
  docker exec "$MONGO_CONTAINER" mongosh sharelatex --quiet --eval "$1" 2>/dev/null |
    tr -d '\r'
}

echo "# benchmark: $LABEL"
echo "# $(date -u +%Y-%m-%dT%H:%M:%SZ), $ITERATIONS iterations after $WARMUP warmups"
echo

echo "## which implementation is listening"
docker exec "$CONTAINER" sh -c '
for s in chat notifications docstore filestore linked-url-proxy real-time \
         document-updater project-history; do
  if pgrep -f "/overleaf/bin/go/$s" >/dev/null 2>&1; then
    printf "%-20s go\n" "$s"
  elif pgrep -f "services/$s/app" >/dev/null 2>&1; then
    printf "%-20s node\n" "$s"
  else
    printf "%-20s -\n" "$s"
  fi
done'
echo

# ---------------------------------------------------------------- memory ---
#
# Resident set size, which is what the machine actually has to find. It is read
# after the workload below has run, so a service that allocates on its first
# request is not measured before it has had one.

measure_memory() {
  echo "## memory (resident, MB)"
  docker exec "$CONTAINER" sh -c "ps -eo rss,args --no-headers" > /tmp/bench-ps.txt
  python3 - <<'PY'
services = {}
for line in open('/tmp/bench-ps.txt'):
    parts = line.split(None, 1)
    if len(parts) != 2:
        continue
    rss, cmd = int(parts[0]), parts[1].strip()
    if '/overleaf/bin/go/' in cmd:
        name = cmd.split('/overleaf/bin/go/')[1].split()[0]
        services.setdefault(name, 0)
        services[name] += rss
    elif '/overleaf/services/' in cmd and 'node' in cmd.split()[0]:
        name = cmd.split('/overleaf/services/')[1].split('/')[0]
        services.setdefault(name, 0)
        services[name] += rss

total = 0
for name in sorted(services):
    print('%-20s %8.1f' % (name, services[name] / 1024))
    total += services[name]
print('%-20s %8.1f' % ('TOTAL', total / 1024))
PY
  echo
}

# --------------------------------------------------------------- timings ---
#
# Each operation is run the same number of times and every duration kept, so
# the spread can be reported rather than an average that hides it. curl is
# given the whole request to time, from inside the container, because that is
# where the caller of these services actually is.

TIMES_DIR=$(mktemp -d)

time_op() {
  local name=$1 command=$2 i
  for ((i = 0; i < WARMUP; i++)); do
    incontainer "$command" >/dev/null 2>&1
  done
  : > "$TIMES_DIR/$name"
  for ((i = 0; i < ITERATIONS; i++)); do
    # curl writes the duration with no newline after it, so one is added
    # here rather than escaping one into the format string.
    incontainer "$command" 2>/dev/null >> "$TIMES_DIR/$name"
    echo >> "$TIMES_DIR/$name"
  done
}

report_times() {
  echo "## request time (ms)"
  printf '%-34s %8s %8s %8s\n' "operation" "p50" "p95" "max"
  python3 - "$TIMES_DIR" <<'PY'
import os, sys

directory = sys.argv[1]
for name in sorted(os.listdir(directory)):
    values = []
    for line in open(os.path.join(directory, name)):
        line = line.strip()
        if not line:
            continue
        try:
            # curl reports seconds; the editor's timings are in milliseconds.
            values.append(float(line) * 1000)
        except ValueError:
            pass
    if not values:
        print('%-34s %8s %8s %8s' % (name, '-', '-', '-'))
        continue
    values.sort()
    def at(fraction):
        return values[min(len(values) - 1, int(len(values) * fraction))]
    print('%-34s %8.1f %8.1f %8.1f' % (name, at(0.5), at(0.95), values[-1]))
PY
  echo
}

# ------------------------------------------------------------------ setup ---

JAR=$(mktemp)
csrf=$(curl -s -c "$JAR" "$BASE/login" |
  grep -o 'name="ol-csrfToken" content="[^"]*' | head -1 | cut -d'"' -f4)
curl -s -b "$JAR" -c "$JAR" -X POST "$BASE/login" \
  -H 'Content-Type: application/json' -H "x-csrf-token: $csrf" \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}" >/dev/null

csrf=$(curl -s -b "$JAR" -c "$JAR" "$BASE/project" |
  grep -o 'name="ol-csrfToken" content="[^"]*' | head -1 | cut -d'"' -f4)
project=$(curl -s -b "$JAR" -c "$JAR" -X POST "$BASE/project/new" \
  -H 'Content-Type: application/json' -H "x-csrf-token: $csrf" \
  -d "{\"projectName\":\"benchmark-$LABEL-$$\",\"template\":\"example\"}")
project_id=$(echo "$project" | grep -o '"project_id":"[^"]*' | cut -d'"' -f4)
if [[ -z "$project_id" ]]; then
  echo "could not create a project: $project" >&2
  exit 1
fi

doc_id=$(inmongo "var p = db.projects.findOne({_id: ObjectId('$project_id')}); print(p.rootFolder[0].docs[0]._id.toString())")
user_id=$(inmongo "print(db.users.findOne({email: '$EMAIL'})._id.toString())")
echo "# project $project_id, doc $doc_id"
echo

W='-s -o /dev/null -w %{time_total}'

# docstore: reading a document out of the database, which is where every
# editing session starts.
time_op "docstore.get-doc" \
  "curl $W http://127.0.0.1:3016/project/$project_id/doc/$doc_id"
time_op "docstore.get-all-docs" \
  "curl $W http://127.0.0.1:3016/project/$project_id/doc"

# document-updater: the path every keystroke takes. The first GET loads the
# document into Redis; the rest are served from it.
time_op "docupdater.get-doc" \
  "curl $W http://127.0.0.1:3003/project/$project_id/doc/$doc_id"
time_op "docupdater.get-project-docs" \
  "curl $W 'http://127.0.0.1:3003/project/$project_id/doc?state=&exclude='"
time_op "docupdater.set-doc" \
  "curl $W -X POST http://127.0.0.1:3003/project/$project_id/doc/$doc_id \
    -H 'Content-Type: application/json' \
    -d '{\"lines\":[\"benchmark\",\"\\\\documentclass{article}\",\"\\\\begin{document}\",\"x\",\"\\\\end{document}\"],\"source\":\"bench\",\"user_id\":\"$user_id\",\"undoing\":false}'"
time_op "docupdater.flush" \
  "curl $W -X POST http://127.0.0.1:3003/project/$project_id/flush"

# project-history: draining the queue, and the three things the history view
# asks for.
time_op "history.flush" \
  "curl $W -X POST http://127.0.0.1:3054/project/$project_id/flush"
time_op "history.version" \
  "curl $W http://127.0.0.1:3054/project/$project_id/version"
time_op "history.updates" \
  "curl $W 'http://127.0.0.1:3054/project/$project_id/updates?min_count=10'"
time_op "history.diff" \
  "curl $W 'http://127.0.0.1:3054/project/$project_id/diff?pathname=main.tex&from=0&to=4'"

# chat and notifications: small services, but they are on the page load path.
time_op "chat.send" \
  "curl $W -X POST http://127.0.0.1:3010/project/$project_id/messages \
    -H 'Content-Type: application/json' \
    -d '{\"user_id\":\"$user_id\",\"content\":\"benchmark\"}'"
time_op "chat.list" \
  "curl $W http://127.0.0.1:3010/project/$project_id/messages"
time_op "notifications.list" \
  "curl $W http://127.0.0.1:3042/user/$user_id/notifications"

# filestore: a file written and read back, which is what an upload and a
# preview do.
incontainer "head -c 65536 /dev/urandom > /tmp/bench-file.bin"
file_id=$(printf '%s' "$(date +%s%N)" | md5sum | cut -c1-24)
time_op "filestore.upload" \
  "curl $W -X POST --data-binary @/tmp/bench-file.bin \
    http://127.0.0.1:3009/project/$project_id/file/$file_id"
time_op "filestore.download" \
  "curl $W http://127.0.0.1:3009/project/$project_id/file/$file_id"

# The page a person actually opens, which goes through web and everything
# behind it.
time_op "web.project-page" \
  "curl $W -b /dev/null http://127.0.0.1:80/login"

report_times
measure_memory

rm -rf "$TIMES_DIR" "$JAR"
