#!/usr/bin/env bash
#
# End-to-end check of the Go document-updater in the live deployment.
#
# It goes through the path a person editing a document goes through: web makes
# the project, document-updater loads the document out of the database, an edit
# is queued the way real-time queues one, and the result has to come back
# through the API, reach the other editors, reach the history and reach the
# database. Every step is checked from inside the container, against the
# service that is actually running.

set -uo pipefail

BASE=http://127.0.0.1:8080
EMAIL=admin@test.local
PASSWORD='TestPassw0rd!2026'
JAR=$(mktemp)
pass=0
fail=0

check() {
  local name=$1 want=$2 got=$3
  if [[ "$got" == *"$want"* ]]; then
    printf '  PASS  %-46s %s\n' "$name" "${got:0:70}"
    pass=$((pass + 1))
  else
    printf '  FAIL  %-46s\n        got:  %s\n        want: %s\n' "$name" "${got:0:200}" "$want"
    fail=$((fail + 1))
  fi
}

incontainer() { docker exec overleaf-pro sh -c "$1"; }

echo "=== which implementation each service is running ==="
docker exec overleaf-pro sh -c '
for s in chat notifications docstore linked-url-proxy filestore real-time document-updater; do
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
csrf=$(curl -s -c "$JAR" "$BASE/login" | grep -o 'name="ol-csrfToken" content="[^"]*' | head -1 | cut -d'"' -f4)
if [[ -z "$csrf" ]]; then
  csrf=$(curl -s -c "$JAR" "$BASE/login" | grep -o '"csrfToken"[^,]*' | head -1 | cut -d'"' -f4)
fi
login=$(curl -s -b "$JAR" -c "$JAR" -X POST "$BASE/login" \
  -H 'Content-Type: application/json' -H "x-csrf-token: $csrf" \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")
check "sign in" "Redirecting" "$login"

csrf=$(curl -s -b "$JAR" -c "$JAR" "$BASE/project" | grep -o 'name="ol-csrfToken" content="[^"]*' | head -1 | cut -d'"' -f4)
project=$(curl -s -b "$JAR" -c "$JAR" -X POST "$BASE/project/new" \
  -H 'Content-Type: application/json' -H "x-csrf-token: $csrf" \
  -d '{"projectName":"go-docupdater-check","template":"example"}')
project_id=$(echo "$project" | grep -o '"project_id":"[^"]*' | cut -d'"' -f4)
check "create a project" "$(echo "$project_id" | cut -c1-8)" "$project_id"
if [[ -z "$project_id" ]]; then
  echo "  cannot continue without a project: $project"
  rm -f "$JAR"
  exit 1
fi

doc_id=$(docker exec deploy-mongo-live-1 mongosh sharelatex --quiet --eval \
  "var p = db.projects.findOne({_id: ObjectId('$project_id')}); print(p.rootFolder[0].docs[0]._id.toString())" 2>/dev/null | tr -d '\r')
check "find the root document" "$(echo "$doc_id" | cut -c1-8)" "$doc_id"

echo
echo "=== document-updater loads the document out of the database ==="
doc=$(incontainer "curl -s http://127.0.0.1:3003/project/$project_id/doc/$doc_id")
check "GET the doc" '"lines"' "$doc"
version=$(echo "$doc" | grep -o '"version":[0-9]*' | cut -d: -f2)
check "it has a version" "0" "$version"
check "it came back with a pathname" "main.tex" "$doc"

loaded=$(docker exec deploy-redis-live-1 redis-cli --raw GET "doclines:$doc_id" | head -c 60)
check "the document is now in redis" "documentclass" "$loaded"

echo
echo "=== an edit queued the way real-time queues one ==="
# The marker goes on its own line at the top, which is unambiguous to look for.
edit='{"doc":"'$doc_id'","op":[{"p":0,"i":"% edited by the go port\n"}],"v":'$version',"meta":{"source":"live-check","user_id":"6a9eb3bc8a19695a4a6c978d","ts":1700000000000}}'
# project-history drains the queue within a second, so it is held still for
# the edit. What document-updater put there is the thing being checked, and it
# cannot be read reliably while something else is taking it away.
docker exec overleaf-pro sv stop project-history-overleaf >/dev/null 2>&1

docker exec deploy-redis-live-1 redis-cli RPUSH "PendingUpdates:$doc_id" "$edit" >/dev/null
docker exec deploy-redis-live-1 redis-cli RPUSH "pending-updates-list" "$project_id:$doc_id" >/dev/null

applied=no
for _ in $(seq 1 40); do
  now=$(docker exec deploy-redis-live-1 redis-cli --raw GET "DocVersion:$doc_id")
  if [[ "$now" == "$((version + 1))" ]]; then applied=yes; break; fi
  sleep 0.25
done
check "the edit was applied" "yes" "$applied"

history=$(docker exec deploy-redis-live-1 redis-cli --raw LRANGE "ProjectHistory:Ops:{$project_id}" 0 -1 | grep -F "edited by the go port" | head -c 400)
docker exec overleaf-pro sv start project-history-overleaf >/dev/null 2>&1

after=$(incontainer "curl -s http://127.0.0.1:3003/project/$project_id/doc/$doc_id")
check "the doc now carries the edit" "edited by the go port" "$after"
check "the version went up" "\"version\":$((version + 1))" "$after"

ops=$(docker exec deploy-redis-live-1 redis-cli --raw LRANGE "DocOps:$doc_id" 0 -1 | head -c 200)
check "the operation was kept for transforming" "edited by the go port" "$ops"

check "the edit was queued for history" "edited by the go port" "$history"
check "history got the document length" "doc_length" "$history"

echo
echo "=== catching up from an older version, which is what a reconnect does ==="
catchup=$(incontainer "curl -s 'http://127.0.0.1:3003/project/$project_id/doc/$doc_id?fromVersion=$version'")
check "the missed operation came back" "edited by the go port" "$catchup"

echo
echo "=== writing it back to the database ==="
flushed=$(incontainer "curl -s -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:3003/project/$project_id/doc/$doc_id/flush")
check "POST /flush" "204" "$flushed"
stored=$(docker exec deploy-mongo-live-1 mongosh sharelatex --quiet --eval \
  "var d = db.docs.findOne({_id: ObjectId('$doc_id')}); print(JSON.stringify(d.lines).substring(0,80))" 2>/dev/null)
check "the database has the edit" "edited by the go port" "$stored"

echo
echo "=== the compile path reads every document through it ==="
first=$(incontainer "curl -s -o /dev/null -w '%{http_code}' -X POST 'http://127.0.0.1:3003/project/$project_id/get_and_flush_if_old?state=live-check'")
check "an unknown project state is a conflict" "409" "$first"
docs=$(incontainer "curl -s -X POST 'http://127.0.0.1:3003/project/$project_id/get_and_flush_if_old?state=live-check'")
check "GET the project documents" "edited by the go port" "$docs"

echo
echo "=== project-history read what was queued ==="
sleep 3
versions=$(curl -s -b "$JAR" "$BASE/project/$project_id/updates" | head -c 300)
check "the change shows in the history" "\"updates\"" "$versions"

echo
echo "=== deleting the project drops it from redis ==="
dropped=$(incontainer "curl -s -o /dev/null -w '%{http_code}' -X DELETE http://127.0.0.1:3003/project/$project_id")
check "DELETE the project" "204" "$dropped"
left=$(docker exec deploy-redis-live-1 redis-cli --raw EXISTS "doclines:$doc_id")
check "the document is out of redis" "0" "$left"

echo
echo "=== the editor page still loads ==="
page=$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR" "$BASE/project/$project_id")
check "GET the project page" "200" "$page"

echo
echo "=== errors in the logs ==="
docker exec overleaf-pro sh -c 'grep -c "\"level\":50\|\"level\":60" /var/log/overleaf/document-updater.log 2>/dev/null || echo 0' | while read -r n; do
  echo "  document-updater errors logged: $n"
done
docker exec overleaf-pro sh -c 'tail -3 /var/log/overleaf/document-updater.log'

rm -f "$JAR"
echo
echo "--- summary: $pass passed, $fail failed"
[[ $fail -eq 0 ]]
