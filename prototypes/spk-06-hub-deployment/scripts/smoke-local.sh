#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"
PORT="${STAGECORE_SMOKE_PORT:-33210}"
cleanup() {
  if [[ -n "${PID:-}" ]]; then kill "$PID" 2>/dev/null || true; wait "$PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT
export STAGECORE_BIND="127.0.0.1:$PORT"
export STAGECORE_DATA_ROOT="$TMP/data"
export STAGECORE_VAULT_ROOT="$TMP/ssd/vault"
export STAGECORE_INSTANCE_NAME="SPK-06 Smoke Hub"
BIN="$TMP/stagecore-hub"
CGO_ENABLED=0 go build -o "$BIN" "$ROOT/cmd/stagecore-hub"
start() {
  "$BIN" >"$TMP/hub.log" 2>&1 & PID=$!
  for _ in $(seq 1 50); do
    if curl -fsS "http://127.0.0.1:$PORT/health/ready" >"$TMP/ready.json"; then return; fi
    sleep 0.1
  done
  cat "$TMP/hub.log"; return 1
}
start
ID1="$(sed -n 's/.*"hub_id":"\([^"]*\)".*/\1/p' "$TMP/ready.json")"
curl -fsS "http://127.0.0.1:$PORT/runtime/ping" >/dev/null
kill "$PID"; wait "$PID" || true; PID=""
start
ID2="$(sed -n 's/.*"hub_id":"\([^"]*\)".*/\1/p' "$TMP/ready.json")"
[[ -n "$ID1" && "$ID1" == "$ID2" ]]
[[ -d "$TMP/ssd/vault/objects/sha256" ]]
printf 'SPK-06 smoke PASS hub_id=%s\n' "$ID2"
