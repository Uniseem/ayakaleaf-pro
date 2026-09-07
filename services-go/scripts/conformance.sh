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
#   scripts/conformance.sh all
#
# Requires a reachable MongoDB. Point at it with MONGO_CONNECTION_STRING, or
# MONGO_HOST for the default mongodb://$MONGO_HOST/sharelatex.

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
GO_DIR="$PWD"
REPO_ROOT="$(cd .. && pwd)"
BIN_DIR="$GO_DIR/bin"

export MONGO_HOST="${MONGO_HOST:-127.0.0.1}"
export NODE_ENV="${NODE_ENV:-test}"
# The services' Makefiles set RETRIES; running the mocha script directly means
# setting it here, or --retries= is passed with no value and mocha rejects it.
export RETRIES="${RETRIES:-0}"
# vitest starts a watch loop on an interactive terminal; the notifications
# suite must run once and exit.
export CI="${CI:-true}"

SERVICE_PID=""

cleanup() {
  if [[ -n "$SERVICE_PID" ]] && kill -0 "$SERVICE_PID" 2>/dev/null; then
    kill "$SERVICE_PID" 2>/dev/null || true
    wait "$SERVICE_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

wait_for_status() {
  local port=$1 name=$2
  for _ in $(seq 1 120); do
    if curl -fsS "http://127.0.0.1:${port}/status" >/dev/null 2>&1; then
      return 0
    fi
    if ! kill -0 "$SERVICE_PID" 2>/dev/null; then
      echo "error: the Go $name service exited during startup" >&2
      return 1
    fi
    sleep 0.25
  done
  echo "error: the Go $name service did not become ready on port $port" >&2
  return 1
}

start_service() {
  local name=$1 port=$2
  echo "--- starting the Go $name service on port $port"
  "$BIN_DIR/$name" &
  SERVICE_PID=$!
  wait_for_status "$port" "$name"
}

stop_service() {
  cleanup
  SERVICE_PID=""
}

run_chat() {
  start_service chat 3010
  echo "--- running services/chat acceptance suite against it"
  (
    cd "$REPO_ROOT/services/chat"
    CHAT_EXTERNAL=true yarn run test:acceptance:_run
  )
  stop_service
}

run_notifications() {
  start_service notifications 3042
  echo "--- running services/notifications acceptance suite against it"
  (
    cd "$REPO_ROOT/services/notifications"
    NOTIFICATIONS_EXTERNAL=true yarn run test:acceptance:_run
  )
  stop_service
}

echo "--- building the Go services"
go build -o "$BIN_DIR/" ./cmd/...

case "${1:-all}" in
  chat) run_chat ;;
  notifications) run_notifications ;;
  all)
    run_chat
    run_notifications
    ;;
  *)
    echo "usage: $0 [chat|notifications|all]" >&2
    exit 2
    ;;
esac

echo "--- conformance passed"
