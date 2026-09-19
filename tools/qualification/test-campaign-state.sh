#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STATE_TOOL="$ROOT/tools/qualification/qualification-state.py"
MANIFEST="$ROOT/tools/qualification/manifest.json"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
state="$tmp/campaign.json"

python3 "$STATE_TOOL" init --state "$state" --manifest "$MANIFEST" \
  --stagecore-sha stagecore-a --tablet-build-sha tablet-a \
  --tablet-apk-sha256 apk-a --lighting-firmware-sha fw-a \
  --hardware-baseline-id bench-a

[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-TAB-04)" == "PENDING" ]]
python3 "$STATE_TOOL" record --state "$state" --manifest "$MANIFEST" \
  --gate Q-TAB-04 --status PASS --actor self-test --evidence probe.json
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-TAB-04)" == "PASS" ]]

# Re-init with identical pins is idempotent and preserves PASS.
python3 "$STATE_TOOL" init --state "$state" --manifest "$MANIFEST" \
  --stagecore-sha stagecore-a --tablet-build-sha tablet-a \
  --tablet-apk-sha256 apk-a --lighting-firmware-sha fw-a \
  --hardware-baseline-id bench-a >/dev/null
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-TAB-04)" == "PASS" ]]

# Physical/manual PASS requires evidence or an observation note.
set +e
python3 "$STATE_TOOL" record --state "$state" --manifest "$MANIFEST" \
  --gate Q-TAB-06 --status PASS --actor self-test >/dev/null 2>&1
rc=$?
set -e
[[ "$rc" -ne 0 ]]

# N/A is allowed only on gates that explicitly permit it and requires a reason.
python3 "$STATE_TOOL" record --state "$state" --manifest "$MANIFEST" \
  --gate Q-LIVE-04 --status N/A --actor self-test --note "no second source class available" >/dev/null

# A changed pin cannot silently reuse evidence.
set +e
python3 "$STATE_TOOL" init --state "$state" --manifest "$MANIFEST" --lighting-firmware-sha fw-b >/dev/null 2>&1
rc=$?
set -e
[[ "$rc" -ne 0 ]]
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-TAB-04)" == "PASS" ]]

# Deliberate repin preserves history and invalidates only explicitly named gates.
python3 "$STATE_TOOL" record --state "$state" --manifest "$MANIFEST" \
  --gate Q-DMX-08 --status FAIL --actor self-test --evidence fade.log >/dev/null
python3 "$STATE_TOOL" repin --state "$state" --manifest "$MANIFEST" \
  --pin lighting_firmware_sha --value fw-b --reason "long-fade fix" \
  --invalidate Q-DMX-08 --invalidate Q-DMX-22 >/dev/null
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-DMX-08)" == "PENDING" ]]
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-TAB-04)" == "PASS" ]]

python3 "$STATE_TOOL" status --state "$state" --json >"$tmp/status.json"
python3 - "$state" "$tmp/status.json" <<'PY'
import json, sys
state=json.load(open(sys.argv[1], encoding="utf-8"))
status=json.load(open(sys.argv[2], encoding="utf-8"))
assert status["counts"]["PASS"] >= 1
assert state["pins"]["lighting_firmware_sha"] == "fw-b"
assert state["pin_history"][-1]["invalidated_gates"] == ["Q-DMX-08","Q-DMX-22"]
assert state["gates"]["Q-DMX-08"]["history"]
PY

echo "qualification campaign state self-test PASS"
