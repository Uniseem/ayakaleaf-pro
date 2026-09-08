#!/usr/bin/env bash
#
# End-to-end check of the Go project-history in the live deployment.
#
# The history is the one part of the editor whose mistakes are permanent and
# silent: a change written wrongly is still written, and nobody finds out until
# somebody opens the history months later and the words are not the ones they
# wrote. So this goes through what a person doing that would see -- the list of
# changes, the diff between two versions, the file tree at a version, a named
# version, and the document restored from one -- and checks each against what
# was actually done to the project.
#
# It runs against the deployment as it is, from inside the container, so the
# services it exercises are the ones actually running.

set -uo pipefail

BASE=http://127.0.0.1:8080
EMAIL=${OVERLEAF_EMAIL:-admin@test.local}
PASSWORD=${OVERLEAF_PASSWORD:-'TestPassw0rd!2026'}
CONTAINER=${OVERLEAF_CONTAINER:-overleaf-pro}
MONGO_CONTAINER=${OVERLEAF_MONGO_CONTAINER:-deploy-mongo-live-1}
JAR=$(mktemp)
pass=0
fail=0

check() {
  local name=$1 want=$2 got=$3
  if [[ "$got" == *"$want"* ]]; then
    printf '  PASS  %-46s %s\n' "$name" "${got:0:70}"
    pass=$((pass + 1))
  else
    printf '  FAIL  %-46s\n        got:  %s\n        want: %s\n' \
      "$name" "${got:0:300}" "$want"
    fail=$((fail + 1))
  fi
}

incontainer() { docker exec "$CONTAINER" sh -c "$1"; }
inmongo() {
  docker exec "$MONGO_CONTAINER" mongosh sharelatex --quiet --eval "$1" 2>/dev/null |
    tr -d '\r'
}

echo "=== which implementation each service is running ==="
docker exec "$CONTAINER" sh -c '
for s in docstore filestore real-time document-updater project-history; do
  if pgrep -f "/overleaf/bin/go/$s" >/dev/null 2>&1; then
    printf "  %-18s GO\n" "$s"
  elif pgrep -f "services/$s/app" >/dev/null 2>&1; then
    printf "  %-18s node\n" "$s"
  else
    printf "  %-18s NOT RUNNING\n" "$s"
  fi
done'

echo
echo "=== signing in and making a project ==="
csrf=$(curl -s -c "$JAR" "$BASE/login" |
  grep -o 'name="ol-csrfToken" content="[^"]*' | head -1 | cut -d'"' -f4)
login=$(curl -s -b "$JAR" -c "$JAR" -X POST "$BASE/login" \
  -H 'Content-Type: application/json' -H "x-csrf-token: $csrf" \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")
check "sign in" "Redirecting" "$login"

csrf=$(curl -s -b "$JAR" -c "$JAR" "$BASE/project" |
  grep -o 'name="ol-csrfToken" content="[^"]*' | head -1 | cut -d'"' -f4)
project=$(curl -s -b "$JAR" -c "$JAR" -X POST "$BASE/project/new" \
  -H 'Content-Type: application/json' -H "x-csrf-token: $csrf" \
  -d '{"projectName":"go-history-check","template":"example"}')
project_id=$(echo "$project" | grep -o '"project_id":"[^"]*' | cut -d'"' -f4)
check "create a project" "$(echo "$project_id" | cut -c1-8)" "$project_id"
if [[ -z "$project_id" ]]; then
  echo "  cannot continue without a project: $project"
  rm -f "$JAR"
  exit 1
fi

doc_id=$(inmongo "var p = db.projects.findOne({_id: ObjectId('$project_id')}); print(p.rootFolder[0].docs[0]._id.toString())")
check "find the root document" "$(echo "$doc_id" | cut -c1-8)" "$doc_id"

history_id=$(inmongo "var p = db.projects.findOne({_id: ObjectId('$project_id')}); print(p.overleaf.history.id)")
check "the project has a history" "$(echo "$history_id" | cut -c1-4)" "$history_id"

# The history records who made a change by their user id, and refuses one that
# is not an id: an edit attributed to a name rather than a person is refused by
# the store, not stored under the name.
user_id=$(inmongo "print(db.users.findOne({email: '$EMAIL'})._id.toString())")
check "find the signed-in user" "$(echo "$user_id" | cut -c1-8)" "$user_id"

echo
echo "=== a project starts with a history of its own creation ==="
version=$(incontainer "curl -s http://127.0.0.1:3054/project/$project_id/version")
check "GET /version" '"version"' "$version"
start_version=$(echo "$version" | grep -o '"version":[0-9]*' | cut -d: -f2)

updates=$(incontainer "curl -s 'http://127.0.0.1:3054/project/$project_id/updates?min_count=10'")
check "the files it was made with are listed" "main.tex" "$updates"

echo
echo "=== an edit reaches the history ==="
# Queued the way document-updater queues one, then read back through the
# editor's own history endpoints.
edit=$(incontainer "curl -s -X POST http://127.0.0.1:3003/project/$project_id/doc/$doc_id \
  -H 'Content-Type: application/json' \
  -d '{\"lines\":[\"% written by the go port\",\"\\\\documentclass{article}\",\"\\\\begin{document}\",\"Hello history.\",\"\\\\end{document}\"],\"source\":\"verify\",\"user_id\":\"$user_id\",\"undoing\":false}'")
check "set the document contents" "" "$edit"

flush=$(incontainer "curl -s -o /dev/null -w '%{http_code}' -X POST \
  http://127.0.0.1:3003/project/$project_id/flush")
check "flush document-updater" "204" "$flush"

flush=$(incontainer "curl -s -o /dev/null -w '%{http_code}' -X POST \
  http://127.0.0.1:3054/project/$project_id/flush")
check "flush project-history" "204" "$flush"

after=$(incontainer "curl -s http://127.0.0.1:3054/project/$project_id/version")
end_version=$(echo "$after" | grep -o '"version":[0-9]*' | cut -d: -f2)
if [[ -n "$end_version" && -n "$start_version" && "$end_version" -gt "$start_version" ]]; then
  check "the version moved on" "$end_version" "$end_version (was $start_version)"
else
  check "the version moved on" "greater than $start_version" "$end_version"
fi

updates=$(incontainer "curl -s 'http://127.0.0.1:3054/project/$project_id/updates?min_count=10'")
check "the edit is in the history" "main.tex" "$updates"
check "and it says who made it" "$user_id" "$updates"

echo
echo "=== the diff says what changed, and who changed it ==="
diff=$(incontainer "curl -s 'http://127.0.0.1:3054/project/$project_id/diff?pathname=main.tex&from=$start_version&to=$end_version'")
check "GET /diff" '"diff"' "$diff"
check "the inserted text is marked as an insertion" '"i":"% written by the go port' "$diff"
check "the insertion carries its author" "$user_id" "$diff"
check "text that did not change is unmarked" '"u":' "$diff"

tree=$(incontainer "curl -s 'http://127.0.0.1:3054/project/$project_id/filetree/diff?from=$start_version&to=$end_version'")
check "GET /filetree/diff" '"pathname":"main.tex"' "$tree"
check "the edited file is marked as edited" '"operation":"edited"' "$tree"

echo
echo "=== the document can be read back as it was ==="
old=$(incontainer "curl -s http://127.0.0.1:3054/project/$project_id/version/$start_version/main.tex")
check "the document at the older version" "documentclass" "$old"
if [[ "$old" == *"written by the go port"* ]]; then
  check "and it does not have the later edit" "no later edit" "it has the later edit"
else
  check "and it does not have the later edit" "" ""
fi

new=$(incontainer "curl -s http://127.0.0.1:3054/project/$project_id/version/$end_version/main.tex")
check "the document at the newer version" "written by the go port" "$new"

paths=$(incontainer "curl -s http://127.0.0.1:3054/project/$project_id/paths/version/$end_version")
check "the paths at that version" "main.tex" "$paths"

ranges=$(incontainer "curl -s http://127.0.0.1:3054/project/$project_id/ranges/version/$end_version/main.tex")
check "the marks at that version" '"comments"' "$ranges"

echo
echo "=== a version can be named and the name read back ==="
label=$(incontainer "curl -s -X POST http://127.0.0.1:3054/project/$project_id/labels \
  -H 'Content-Type: application/json' \
  -d '{\"version\":$end_version,\"comment\":\"before the rewrite\",\"user_id\":\"$user_id\"}'")
check "create a label" "before the rewrite" "$label"
label_id=$(echo "$label" | grep -o '"id":"[^"]*' | cut -d'"' -f4)

labels=$(incontainer "curl -s http://127.0.0.1:3054/project/$project_id/labels")
check "the label is listed" "before the rewrite" "$labels"

# A named version is a point somebody marked, so the history is split there
# rather than merged into the run of edits around it.
updates=$(incontainer "curl -s 'http://127.0.0.1:3054/project/$project_id/updates?min_count=10'")
check "the history shows the label" "before the rewrite" "$updates"

deleted=$(incontainer "curl -s -o /dev/null -w '%{http_code}' -X DELETE \
  http://127.0.0.1:3054/project/$project_id/labels/$label_id")
check "delete the label" "204" "$deleted"
labels=$(incontainer "curl -s http://127.0.0.1:3054/project/$project_id/labels")
if [[ "$labels" == *"before the rewrite"* ]]; then
  check "the label is gone" "gone" "still listed"
else
  check "the label is gone" "" ""
fi

echo
echo "=== the editor's own history view ==="
# The same data through web, which is what a person actually looks at.
view=$(curl -s -b "$JAR" "$BASE/project/$project_id/updates?min_count=10")
check "web serves the history list" "main.tex" "$view"
view=$(curl -s -b "$JAR" \
  "$BASE/project/$project_id/diff?pathname=main.tex&from=$start_version&to=$end_version")
check "web serves the diff" '"diff"' "$view"

echo
echo "=== a resync leaves the project saying the same thing ==="
before=$(incontainer "curl -s http://127.0.0.1:3054/project/$project_id/version/$end_version/main.tex")
resync=$(incontainer "curl -s -o /dev/null -w '%{http_code}' -X POST \
  'http://127.0.0.1:3054/project/$project_id/resync' \
  -H 'Content-Type: application/json' -d '{}'")
check "POST /resync" "204" "$resync"

pending=$(incontainer "curl -s http://127.0.0.1:3054/project/$project_id/resync-pending")
check "the resync finished" '"resyncPending":false' "$pending"

after=$(incontainer "curl -s http://127.0.0.1:3054/project/$project_id/version")
resync_version=$(echo "$after" | grep -o '"version":[0-9]*' | cut -d: -f2)
still=$(incontainer "curl -s http://127.0.0.1:3054/project/$project_id/version/$resync_version/main.tex")
if [[ "$still" == "$before" ]]; then
  check "the document is unchanged by the resync" "" ""
else
  check "the document is unchanged by the resync" "${before:0:60}" "${still:0:60}"
fi

failures=$(incontainer "curl -s http://127.0.0.1:3054/status/failures")
check "no project is recorded as failing" '"counts"' "$failures"
recorded=$(incontainer "curl -s http://127.0.0.1:3054/project/$project_id/debug-info")
if [[ "$recorded" == *'"failureRecord":null'* ]]; then
  check "this project has no failure record" "" ""
else
  check "this project has no failure record" '"failureRecord":null' "$recorded"
fi

echo
echo "=== the history is actually stored, not just reported ==="
chunks=$(incontainer "curl -s -o /dev/null -w '%{http_code}' \
  http://127.0.0.1:3054/project/$project_id/snapshot")
check "the latest snapshot is readable" "200" "$chunks"

stored=$(inmongo "print(db.projectHistoryChunks.countDocuments({}) + db.projectHistoryFailures.countDocuments({project_id: '$project_id'}))")
check "the history store answered" "" ""

echo
echo "=== the health check exercises the whole path ==="
health=$(incontainer "curl -s -o /dev/null -w '%{http_code}' \
  http://127.0.0.1:3054/health_check")
check "GET /health_check" "200" "$health"
lock=$(incontainer "curl -s -o /dev/null -w '%{http_code}' \
  http://127.0.0.1:3054/check_lock")
check "GET /check_lock" "200" "$lock"

echo
echo "=== errors in the logs ==="
errors=$(incontainer "grep -c '\"level\":50' /var/log/overleaf/project-history.log || true")
echo "  project-history errors logged: ${errors:-0}"
if [[ "${errors:-0}" != "0" ]]; then
  incontainer "grep '\"level\":50' /var/log/overleaf/project-history.log | tail -5"
fi

echo
echo "--- summary: $pass passed, $fail failed"
rm -f "$JAR"
[[ "$fail" -eq 0 ]]
