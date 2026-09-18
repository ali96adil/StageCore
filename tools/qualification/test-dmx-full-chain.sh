#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STATE_TOOL="$ROOT/tools/qualification/qualification-state.py"
AGG="$ROOT/tools/qualification/dmx-full-chain.py"
MANIFEST="$ROOT/tools/qualification/manifest.json"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
state="$tmp/campaign.json"

python3 "$STATE_TOOL" init --state "$state" --manifest "$MANIFEST" \
  --stagecore-sha stagecore-a --lighting-firmware-sha firmware-a --hardware-baseline-id bench-a >/dev/null

set +e
python3 "$AGG" --state "$state" --out "$tmp/blocked.json" >/dev/null
blocked_rc=$?
set -e
[[ "$blocked_rc" -eq 3 ]]

for i in $(seq -w 1 21); do
  python3 "$STATE_TOOL" record --state "$state" --manifest "$MANIFEST" \
    --gate "Q-DMX-$i" --status PASS --actor self-test \
    --evidence "self-test-Q-DMX-$i" --note "qualified dependency" >/dev/null
done

python3 "$AGG" --state "$state" --out "$tmp/pass.json" >/dev/null
python3 - "$tmp/pass.json" <<'PY'
import json,sys
data=json.load(open(sys.argv[1],encoding="utf-8"))
assert data["status"]=="PASS"
assert data["dependency_count"]==21
assert not data["failed"]
assert not data["incomplete"]
assert all(data["pins"].values())
PY

python3 "$STATE_TOOL" record --state "$state" --manifest "$MANIFEST" \
  --gate Q-DMX-18 --status FAIL --actor self-test --evidence fail --note "stress failed" >/dev/null
set +e
python3 "$AGG" --state "$state" --out "$tmp/fail.json" >/dev/null
fail_rc=$?
set -e
[[ "$fail_rc" -eq 1 ]]
grep -F '"Q-DMX-18"' "$tmp/fail.json" >/dev/null

echo "qualification Q-DMX-22 full-chain aggregate self-test PASS"
