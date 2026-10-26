#!/usr/bin/env bash
# Capture login page screenshots for visual QA.
# Usage:
#   ./scripts/screenshot-login.sh
#   BASE_URL=https://<server>:8421 CHROMIUM_PATH=/path/to/chromium ./scripts/screenshot-login.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/tmp/screenshots"
BASE_URL="${BASE_URL:-http://127.0.0.1:19421}"
CHROMIUM="${CHROMIUM_PATH:-chromium}"
EXTRA_FLAGS=()
if [[ "${BASE_URL}" == https://* ]]; then
  EXTRA_FLAGS+=(--ignore-certificate-errors)
fi

mkdir -p "$OUT"

"$CHROMIUM" --headless=new --disable-gpu --no-sandbox "${EXTRA_FLAGS[@]}" \
  --window-size=1280,800 \
  --screenshot="$OUT/login-desktop.png" \
  "${BASE_URL%/}/login" >/dev/null 2>&1

"$CHROMIUM" --headless=new --disable-gpu --no-sandbox "${EXTRA_FLAGS[@]}" \
  --window-size=390,844 \
  --screenshot="$OUT/login-mobile.png" \
  "${BASE_URL%/}/login" >/dev/null 2>&1

echo "desktop: $OUT/login-desktop.png"
echo "mobile:  $OUT/login-mobile.png"