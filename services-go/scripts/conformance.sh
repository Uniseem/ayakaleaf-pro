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
#   scripts/conformance.sh --parallel       # the independent ones at once
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

# name:port:directory:external-flag:test-files:excluded-test
#
# test-files is optional. When set, mocha runs exactly those files instead of
# the service's own script. docstore needs it: three of its acceptance files
# mutate the running service's Settings object at runtime -- see
# ArchiveDocsTests.js:210 -- which only works when the service runs inside the
# test process. Those tests fail against any external implementation, the Node
# service included: started as its own process, Node scores the same 75/22 the
# Go port does.
#
# real-time uses it for a duller reason: the whole suite is runnable, but
# "yarn run" re-resolves the workspace first, and the lockfile in this fork is
# not always in step with package.json. Calling mocha directly skips a
# resolution failure that says nothing about the port.
#
# excluded-test is a regex of test titles to skip. real-time has one that
# asserts on the test process's own logger stub, so it can only pass when the
# service shares that process -- Node started externally fails it too. Skipping
# it is what makes a red run mean something.
#
# project-history excludes eight, and none of them is about the port. Two push
# a project id onto Settings.shortHistoryQueues, one assigns
# settings.history.healthCheck, and one turns Settings.apis.filestore off:
# those reach into the running service and can only work when it is the test
# process. Three read a sinon spy on that process's own logger. The last needs
# port 80 for a callback URL, which is taken on most machines. Node started as
# its own process fails exactly these eight and passes the other 119.
#
# document-updater excludes two things. history-ot is a second OT type with its
# own storage format that this port does not implement; it is refused outright
# by the Go service rather than half-handled, and is named in
# services-go/README.md as what is not ported. The two "accepting multiple
# changes" tests spy on a function inside the service and read its return
# values, so they can only pass when the service shares the test process --
# Node started externally fails them the same way.
SERVICES=(
  "chat:3010:services/chat:CHAT_EXTERNAL:"
  "notifications:3042:services/notifications:NOTIFICATIONS_EXTERNAL:"
  "docstore:3016:services/docstore:DOCSTORE_EXTERNAL:test/acceptance/js/GettingDocsTests.js test/acceptance/js/GettingAllDocsTests.js test/acceptance/js/UpdatingDocsTests.js test/acceptance/js/HealthCheckerTest.js"
  "filestore:3009:services/filestore:FILESTORE_EXTERNAL:test/acceptance/js/FilestoreApiTests.js"
  "real-time:3026:services/real-time:REALTIME_EXTERNAL:--recursive test/acceptance/js:should trigger a low level message only"
  "document-updater:3003:services/document-updater:DOCUPDATER_EXTERNAL:--recursive test/acceptance/js:history-ot|accepting multiple changes"
  "project-history:3054:services/project-history:PROJECT_HISTORY_EXTERNAL:--loader=esmock --recursive test/acceptance/js:short queue|health check|retries in the background|record error when checking blob fails|record error when blob is missing|remove and re-add the file during hard resync|reject partial resync on docs"
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

# clear_mongo <script>
#
# Runs a snippet against the test database. There is not always a client on
# the host -- the database is usually a container -- so this tries the host
# first and then the container that publishes the port, and says so when it
# cannot: a run that quietly kept the last one's records is a run whose
# results are about the wrong thing.
clear_mongo() {
  local script=$1
  local uri="${MONGO_CONNECTION_STRING:-mongodb://${MONGO_HOST}/sharelatex}"

  local client
  for client in mongosh mongo; do
    if command -v "$client" >/dev/null 2>&1; then
      "$client" "$uri" --quiet --eval "$script" >/dev/null 2>&1 && return 0
    fi
  done

  local container
  container=$(docker ps --format '{{.Names}} {{.Ports}}' 2>/dev/null |
    awk '/:27017->/ {print $1; exit}')
  if [[ -n "$container" ]]; then
    docker exec "$container" mongosh "mongodb://127.0.0.1/sharelatex"       --quiet --eval "$script" >/dev/null 2>&1 && return 0
  fi

  echo "warning: could not clear mongo; this run will be judging what the"     "last one left behind" >&2
  return 1
}

# run_one <name> <port> <dir> <external-var> [database]
#
# Starts the Go binary, waits for it to answer /status, runs the Node
# acceptance suite against it, then stops it again.
run_one() {
  local name=$1 port=$2 dir=$3 external_var=$4 test_files=$5 exclude=$6 database=${7:-}

  if [[ "$name" == "real-time" ]]; then
    # The suite signs its session cookies with the three secrets in
    # services/real-time/config/settings.test.cjs and checks that all three are
    # accepted, so the Go service has to be given exactly those.
    export SESSION_SECRET=static-secret-for-tests
    export SESSION_SECRET_UPCOMING=static-secret-upcoming-for-tests
    export SESSION_SECRET_FALLBACK=static-secret-fallback-for-tests
    # Unlike the other ports, this one needs Redis: sessions, presence and the
    # pub/sub fan-out all live there.
    export REDIS_HOST="${REDIS_HOST:-127.0.0.1}"
  fi

  if [[ "$name" == "document-updater" ]]; then
    # The documents, the locks and the queue real-time pushes edits onto all
    # live in Redis.
    export REDIS_HOST="${REDIS_HOST:-127.0.0.1}"
  fi

  if [[ "$name" == "project-history" ]]; then
    export REDIS_HOST="${REDIS_HOST:-127.0.0.1}"
    # The queue document-updater fills, the lock, and the remembered history
    # ids are in Redis; the failure records, labels and sync state are in
    # Mongo. Both are read back by the tests as a baseline, and a queue left
    # behind by an earlier run is a project this one will try to flush -- so a
    # run has to start from nothing or it is judging what the last one left.
    redis-cli -h "$REDIS_HOST" --scan --pattern 'ProjectHistory:*' 2>/dev/null |
      xargs -r redis-cli -h "$REDIS_HOST" del >/dev/null 2>&1 || true
    clear_mongo '
      db.projectHistoryFailures.deleteMany({});
      db.projectHistoryLabels.deleteMany({});
      db.projectHistorySyncState.deleteMany({});
    '
  fi

  if [[ "$name" == "filestore" ]]; then
    # The filesystem backend stores objects under these directories; a run of
    # its own keeps concurrent services from sharing state.
    local root="${TMPDIR:-/tmp}/conformance-filestore-$$"
    export BACKEND="${BACKEND:-fs}"
    export TEMPLATE_FILES_BUCKET_NAME="$root/template"
    export OVERLEAF_EDITOR_BLOBS_BUCKET="$root/global-blobs"
    export OVERLEAF_EDITOR_PROJECT_BLOBS_BUCKET="$root/project-blobs"
    rm -rf "$root"
    mkdir -p "$TEMPLATE_FILES_BUCKET_NAME" "$OVERLEAF_EDITOR_BLOBS_BUCKET"              "$OVERLEAF_EDITOR_PROJECT_BLOBS_BUCKET"
  fi

  if [[ -n "$database" ]]; then
    # An isolated database lets services run concurrently without their
    # migrations racing on the shared migrations collection.
    export MONGO_CONNECTION_STRING="mongodb://${MONGO_HOST}/${database}?directConnection=true"
  fi

  # A service left behind by an earlier run would answer the readiness check
  # below, and the suite would then test that stale process -- against its
  # configuration, not this one's. Refuse to start rather than report a result
  # about the wrong thing.
  if curl -fsS "http://127.0.0.1:${port}/status" >/dev/null 2>&1; then
    echo "error: something is already listening on port $port; stop it first" >&2
    return 1
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
    local mocha_args=()
    read -ra mocha_args <<< "$test_files"
    if [[ -n "$exclude" ]]; then
      # A regex, so a whole category can be left out rather than one test.
      mocha_args+=(--grep "$exclude" --invert)
    fi
    ( cd "$REPO_ROOT/$dir"       && env "$external_var=true" PATH="$REPO_ROOT/node_modules/.bin:$PATH"          mocha --timeout 15000 --exit --retries="$RETRIES" "${mocha_args[@]}" ) || rc=$?
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
  local n port dir external_var test_files exclude
  IFS=: read -r n port dir external_var test_files exclude <<< "$spec"
  run_one "$n" "$port" "$dir" "$external_var" "$test_files" "$exclude" "$database"
}

mkdir -p "$LOG_DIR"

echo "--- building the Go services"
go build -o "$BIN_DIR/" ./cmd/... || exit 1

case "${1:-all}" in
  --parallel)
    echo "--- running the independent services concurrently, each against its"
    echo "    own database, then the ones that share ports in turn"
    status=0

    # Three of these cannot run at the same time as each other, and it is the
    # suites rather than the services that collide: real-time's own tests
    # start a mock document-updater on 3003, which is the port the real Go
    # document-updater is listening on, and document-updater's tests talk to
    # project-history on 3054. Ports are not something an isolated database
    # fixes, so this chain is run one at a time.
    COUPLED=(real-time document-updater project-history)

    is_coupled() {
      local want=$1 name
      for name in "${COUPLED[@]}"; do
        [[ "$name" == "$want" ]] && return 0
      done
      return 1
    }

    pids=()
    names=()
    for spec in "${SERVICES[@]}"; do
      name="${spec%%:*}"
      is_coupled "$name" && continue
      # A subshell per service, so each gets its own MONGO_CONNECTION_STRING.
      ( run_named "$name" "conformance_${name//-/_}" ) > "$LOG_DIR/$name.log" 2>&1 &
      pids+=("$!")
      names+=("$name")
    done

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

    for name in "${COUPLED[@]}"; do
      spec_for "$name" >/dev/null || continue
      rc=0
      ( run_named "$name" "conformance_${name//-/_}" )         > "$LOG_DIR/$name.log" 2>&1 || rc=$?
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
