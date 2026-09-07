#!/usr/bin/env bash
#
# Run a Node service's own acceptance suite against the Go port of that
# service. This is the check that decides whether a swap is safe: the tests,
# the HTTP client and every assertion are the ones that ship with the Node
# service -- only the process listening on the port changes.
#
# Usage:
#   scripts/conformance.sh chat
#   scripts/conformance.sh notifications
#   scripts/conformance.sh all              # one after another, shared database
#   scripts/conformance.sh --parallel       # all at once, isolated databases
#
# Requires a reachable MongoDB. Point at it with MONGO_CONNECTION_STRING, or
# MONGO_HOST for the default mongodb://$MONGO_HOST/sharelatex.

set -uo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
GO_DIR="$PWD"
REPO_ROOT="$(cd .. && pwd)"
BIN_DIR="$GO_DIR/bin"
LOG_DIR="$GO_DIR/logs"

export MONGO_HOST="${MONGO_HOST:-127.0.0.1}"
export NODE_ENV="${NODE_ENV:-test}"
# The services' Makefiles set RETRIES; running the mocha script directly means
# setting it here, or --retries= is passed with no value and mocha rejects it.
export RETRIES="${RETRIES:-0}"
# vitest starts a watch loop on an interactive terminal; the notifications
# suite must run once and exit.
export CI="${CI:-true}"

# name:port:directory:external-flag:test-files
#
# test-files is optional. When set, mocha runs exactly those files instead of
# the service's own script. docstore needs it: three of its acceptance files
# mutate the running service's Settings object at runtime -- see
# ArchiveDocsTests.js:210 -- which only works when the service runs inside the
# test process. Those tests fail against any external implementation, the Node
# service included: started as its own process, Node scores the same 75/22 the
# Go port does.
SERVICES=(
  "chat:3010:services/chat:CHAT_EXTERNAL:"
  "notifications:3042:services/notifications:NOTIFICATIONS_EXTERNAL:"
  "docstore:3016:services/docstore:DOCSTORE_EXTERNAL:test/acceptance/js/GettingDocsTests.js test/acceptance/js/GettingAllDocsTests.js test/acceptance/js/UpdatingDocsTests.js test/acceptance/js/HealthCheckerTest.js"
)

spec_for() {
  local want=$1 spec
  for spec in "${SERVICES[@]}"; do
    if [[ "${spec%%:*}" == "$want" ]]; then
      echo "$spec"
      return 0
    fi
  done
  return 1
}

# run_one <name> <port> <dir> <external-var> [database]
#
# Starts the Go binary, waits for it to answer /status, runs the Node
# acceptance suite against it, then stops it again.
run_one() {
  local name=$1 port=$2 dir=$3 external_var=$4 test_files=$5 database=${6:-}

  if [[ -n "$database" ]]; then
    # An isolated database lets services run concurrently without their
    # migrations racing on the shared migrations collection.
    export MONGO_CONNECTION_STRING="mongodb://${MONGO_HOST}/${database}?directConnection=true"
  fi

  echo "--- starting the Go $name service on port $port"
  "$BIN_DIR/$name" > "$LOG_DIR/$name.service.log" 2>&1 &
  local pid=$!

  local ready=1 i
  for i in $(seq 1 120); do
    if curl -fsS "http://127.0.0.1:${port}/status" >/dev/null 2>&1; then
      ready=0
      break
    fi
    if ! kill -0 "$pid" 2>/dev/null; then
      echo "error: the Go $name service exited during startup" >&2
      sed -e 's/^/    /' "$LOG_DIR/$name.service.log" >&2
      return 1
    fi
    sleep 0.25
  done
  if [[ $ready -ne 0 ]]; then
    echo "error: the Go $name service did not become ready on port $port" >&2
    kill "$pid" 2>/dev/null
    return 1
  fi

  echo "--- running $dir acceptance suite against it"
  local rc=0
  if [[ -n "$test_files" ]]; then
    # shellcheck disable=SC2086
    ( cd "$REPO_ROOT/$dir"       && env "$external_var=true" PATH="$REPO_ROOT/node_modules/.bin:$PATH"          mocha --timeout 15000 --exit --retries="$RETRIES" $test_files ) || rc=$?
  else
    ( cd "$REPO_ROOT/$dir" && env "$external_var=true" yarn run test:acceptance:_run ) || rc=$?
  fi

  kill "$pid" 2>/dev/null
  wait "$pid" 2>/dev/null
  return $rc
}

run_named() {
  local name=$1 database=${2:-}
  local spec
  spec=$(spec_for "$name") || { echo "unknown service: $name" >&2; return 2; }
  local n port dir external_var test_files
  IFS=: read -r n port dir external_var test_files <<< "$spec"
  run_one "$n" "$port" "$dir" "$external_var" "$test_files" "$database"
}

mkdir -p "$LOG_DIR"

echo "--- building the Go services"
go build -o "$BIN_DIR/" ./cmd/... || exit 1

case "${1:-all}" in
  --parallel)
    echo "--- running every service concurrently, each against its own database"
    pids=()
    names=()
    for spec in "${SERVICES[@]}"; do
      name="${spec%%:*}"
      # A subshell per service, so each gets its own MONGO_CONNECTION_STRING.
      ( run_named "$name" "conformance_${name//-/_}" ) > "$LOG_DIR/$name.log" 2>&1 &
      pids+=("$!")
      names+=("$name")
    done

    status=0
    for i in "${!pids[@]}"; do
      rc=0
      wait "${pids[$i]}" || rc=$?
      name="${names[$i]}"
      echo
      echo "======== ${name} (exit ${rc}) ========"
      cat "$LOG_DIR/$name.log"
      if [[ $rc -ne 0 ]]; then
        status=1
      fi
    done
    if [[ $status -ne 0 ]]; then
      echo
      echo "--- conformance FAILED"
      exit 1
    fi
    ;;
  all)
    for spec in "${SERVICES[@]}"; do
      run_named "${spec%%:*}" || { echo "--- conformance FAILED"; exit 1; }
    done
    ;;
  *)
    run_named "$1" || { echo "--- conformance FAILED"; exit 1; }
    ;;
esac

echo
echo "--- conformance passed"
