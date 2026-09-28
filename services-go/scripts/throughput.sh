#!/usr/bin/env bash
#
# How much a service gets through when several editors are talking to it at
# once.
#
# One request at a time says how fast a service answers; it says nothing about
# what happens when twenty people are editing, which is the case the memory
# and the concurrency model actually decide. This runs a fixed number of
# clients against one endpoint for a fixed time and counts what completed.
#
#   ./throughput.sh go <project-id> <doc-id>
#
# The label is only written into the output; it does not select anything.

set -uo pipefail

LABEL=${1:-current}
PROJECT=${2:-}
DOC=${3:-}
CONTAINER=${OVERLEAF_CONTAINER:-overleaf-pro}
CONCURRENCY=${CONCURRENCY:-16}
DURATION=${DURATION:-10}

if [[ -z "$PROJECT" || -z "$DOC" ]]; then
  echo "usage: throughput.sh <label> <project-id> <doc-id>" >&2
  exit 2
fi

# The loop runs inside the container, so what is measured is the service and
# not the pipe to it. Each client counts what it completed and the counts are
# added up at the end.
WORKER='
  url=$1
  seconds=$2
  clients=$3
  end=$(( $(date +%s) + seconds ))
  i=0
  while [ "$i" -lt "$clients" ]; do
    (
      n=0
      while [ "$(date +%s)" -lt "$end" ]; do
        if curl -s -o /dev/null --max-time 30 "$url"; then
          n=$((n + 1))
        fi
      done
      echo "$n"
    ) &
    i=$((i + 1))
  done
  wait
'

run() {
  local name=$1 url=$2 total
  total=$(docker exec "$CONTAINER" sh -c "$WORKER" -- "$url" "$DURATION" "$CONCURRENCY" |
    awk '{ sum += $1 } END { print sum + 0 }')
  printf '%-28s %8d req  %8.1f req/s\n' \
    "$name" "$total" "$(awk -v t="$total" -v d="$DURATION" 'BEGIN { print t / d }')"
}

echo "## throughput: $LABEL, $CONCURRENCY concurrent clients, ${DURATION}s each"
run "docupdater.get-doc" "http://127.0.0.1:3003/project/$PROJECT/doc/$DOC"
run "docstore.get-doc" "http://127.0.0.1:3016/project/$PROJECT/doc/$DOC"
run "history.version" "http://127.0.0.1:3054/project/$PROJECT/version"
run "chat.list" "http://127.0.0.1:3010/project/$PROJECT/messages"
