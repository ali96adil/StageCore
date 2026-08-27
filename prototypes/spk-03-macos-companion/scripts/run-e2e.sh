#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IDENTITY_DIR="${TMPDIR:-/tmp}/stagecore-spk03"
HUB_LOG="${TMPDIR:-/tmp}/stagecore-spk03-hub.log"
COMPANION_LOG="${TMPDIR:-/tmp}/stagecore-spk03-companion.log"

rm -rf "$IDENTITY_DIR"
rm -f "$HUB_LOG" "$COMPANION_LOG"

(
  cd "$ROOT/hub-sim"
  go run . >"$HUB_LOG" 2>&1
) &
HUB_PID=$!
trap 'kill "$HUB_PID" 2>/dev/null || true' EXIT

for _ in $(seq 1 50); do
  if grep -q "listening on" "$HUB_LOG" 2>/dev/null; then
    break
  fi
  sleep 0.1
done

(
  cd "$ROOT/companion"
  swift run stagecore-companion-cli --identity "$IDENTITY_DIR/companion-id" >"$COMPANION_LOG" 2>&1
)

wait "$HUB_PID"
trap - EXIT

cat "$HUB_LOG"
cat "$COMPANION_LOG"

grep -q "SPK-03 PASS" "$HUB_LOG"
grep -q "TEST_COMPLETE" "$COMPANION_LOG"
