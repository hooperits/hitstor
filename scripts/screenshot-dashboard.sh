#!/usr/bin/env bash
# Capture dashboard page screenshots (login + authenticated pages).
#
# Local preview (starts server automatically):
#   ./scripts/screenshot-dashboard.sh
#
# Running server:
#   BASE_URL=https://<server>:8421 PASSWORD=<dashboard-password> TARGET=remote ./scripts/screenshot-dashboard.sh --no-start
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASE_URL="${BASE_URL:-http://127.0.0.1:19421}"
TARGET="${TARGET:-local}"
PASSWORD="${PASSWORD:-preview}"
START_SERVER=1
PREVIEW_PID=""

for arg in "$@"; do
  case "$arg" in
    --no-start) START_SERVER=0 ;;
  esac
done

cleanup() {
  if [[ -n "$PREVIEW_PID" ]] && kill -0 "$PREVIEW_PID" 2>/dev/null; then
    kill "$PREVIEW_PID" 2>/dev/null || true
    wait "$PREVIEW_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

if [[ "$START_SERVER" -eq 1 && "$BASE_URL" == "http://127.0.0.1:19421" ]]; then
  PORT=19421 go run "$ROOT/scripts/login-preview" >/tmp/hitstor-preview.log 2>&1 &
  PREVIEW_PID=$!
  for _ in $(seq 1 30); do
    if curl -sf "http://127.0.0.1:19421/login" >/dev/null 2>&1; then
      break
    fi
    sleep 0.5
  done
fi

RUNNER_DIR="$ROOT/scripts/.screenshot-runner"
if [[ ! -d "$RUNNER_DIR/node_modules/playwright" ]]; then
  echo "Installing playwright in $RUNNER_DIR ..."
  (cd "$RUNNER_DIR" && npm install --silent)
fi

BASE_URL="$BASE_URL" PASSWORD="$PASSWORD" TARGET="$TARGET" \
  node "$RUNNER_DIR/capture.mjs"