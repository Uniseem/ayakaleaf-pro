#!/usr/bin/env bash
#
# What web costs on the pages a person actually opens.
#
# The services behind it were measured separately; this is the one in front,
# and the question it answers is whether the time a page takes is web's or
# somebody else's.

set -uo pipefail

BASE=${OVERLEAF_BASE:-http://127.0.0.1:8080}
EMAIL=${OVERLEAF_EMAIL:-admin@test.local}
PASSWORD=${OVERLEAF_PASSWORD:-'TestPassw0rd!2026'}
MONGO_CONTAINER=${OVERLEAF_MONGO_CONTAINER:-mongo}
RUNS=${RUNS:-20}

JAR=$(mktemp)
trap 'rm -f "$JAR"' EXIT

csrf=$(curl -s -c "$JAR" "$BASE/login" |
  grep -o 'name="ol-csrfToken" content="[^"]*' | head -1 | cut -d'"' -f4)
login=$(curl -s -b "$JAR" -c "$JAR" -X POST "$BASE/login" \
  -H 'Content-Type: application/json' -H "x-csrf-token: $csrf" \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")
if [[ "$login" != *Redirecting* ]]; then
  echo "could not sign in: ${login:0:200}" >&2
  exit 1
fi

project=$(docker exec "$MONGO_CONTAINER" mongosh sharelatex --quiet \
  --eval 'print(db.projects.findOne({}, {_id: 1})._id.toString())' 2>/dev/null | tr -d '\r')

echo "# $RUNS runs each, after 3 warmups"
echo

timeit() {
  local name=$1 url=$2 i out
  out=$(mktemp)
  for ((i = 0; i < 3; i++)); do curl -s -b "$JAR" -o /dev/null "$url"; done
  for ((i = 0; i < RUNS; i++)); do
    curl -s -b "$JAR" -o /dev/null \
      -w '%{time_total} %{size_download} %{http_code}\n' "$url" >> "$out"
  done
  python3 - "$name" "$out" <<'PY'
import sys

name, path = sys.argv[1], sys.argv[2]
rows = [line.split() for line in open(path) if line.strip()]
times = sorted(float(row[0]) * 1000 for row in rows)
size = int(rows[0][1])
status = rows[0][2]
print('%-30s %s  p50 %7.1f ms   p95 %7.1f ms   %6.1f kB' % (
    name, status, times[len(times) // 2],
    times[min(len(times) - 1, int(len(times) * 0.95))], size / 1024))
PY
  rm -f "$out"
}

timeit "GET /login" "$BASE/login"
timeit "GET /project  (project list)" "$BASE/project"
timeit "GET /project/:id  (editor)" "$BASE/project/$project"
timeit "GET /socket.io  (handshake)" "$BASE/socket.io/socket.io.js"
