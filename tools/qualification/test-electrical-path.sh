#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STATE_TOOL="$ROOT/tools/qualification/qualification-state.py"
MILESTONE_TOOL="$ROOT/tools/qualification/qualification-milestone.py"
ELECTRICAL_TOOL="$ROOT/tools/qualification/electrical-path.py"
CAMPAIGN="$ROOT/tools/qualification/campaign.sh"
MANIFEST="$ROOT/tools/qualification/manifest.json"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
state="$tmp/campaign.json"

python3 "$STATE_TOOL" init --state "$state" --manifest "$MANIFEST" \
  --stagecore-sha stagecore-a --lighting-firmware-sha fw-a \
  --hardware-baseline-id bench-a >/dev/null

python3 "$ELECTRICAL_TOOL" status --state "$state" --json >"$tmp/status.json"
python3 - "$tmp/status.json" <<'PY'
import json, sys
data=json.load(open(sys.argv[1], encoding="utf-8"))
assert data["gate_id"] == "Q-DMX-21"
assert data["status"] == "PENDING"
assert len(data["checks"]) == 6
assert all(row["status"] == "PENDING" for row in data["checks"])
PY

ack() {
  local check="$1" status="$2" note="$3"
  STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" q21-ack "$check" "$status" "$note" >/dev/null
}

ack documented PASS "Documented ESP32 TX -> MAX485 DI, DE//RE direction control, module A/B terminals and decoder D+/D-/COM labels."
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-DMX-21)" == "BLOCKED" ]]

ack logic-voltage PASS "Measured/verified intended logic path; no 5 V MAX485 receiver output is connected to an ESP32 input."
ack de-re PASS "Verified DE and /RE direction-control wiring for transmit-only DMX."
ack polarity FAIL "Continuity check found module/decoder differential polarity labels crossed; do not energize."
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-DMX-21)" == "FAIL" ]]

ack polarity PASS "Corrected and physically traced module differential pair to decoder D+/D- using actual terminal labels."
ack common PASS "Verified intentional DMX common/reference between transceiver and decoder."
ack termination PASS "Verified one appropriate 120 ohm end termination at the far end of this single-drop bus."
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-DMX-21)" == "PASS" ]]

python3 "$MILESTONE_TOOL" get --state "$state" --gate Q-DMX-21 --key dmx_polarity.verified --json >"$tmp/polarity.json"
python3 - "$tmp/polarity.json" <<'PY'
import json, sys
item=json.load(open(sys.argv[1], encoding="utf-8"))
assert item["status"] == "PASS"
assert item["history"] and item["history"][-1]["status"] == "FAIL"
PY

STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" q21-status >"$tmp/campaign-status"
grep -F $'Q-DMX-21\tPASS' "$tmp/campaign-status" >/dev/null

set +e
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" q21-ack termination PASS "duplicate evidence" >/dev/null 2>&1
locked_rc=$?
set -e
[[ "$locked_rc" -eq 3 ]]

python3 "$STATE_TOOL" repin --state "$state" --manifest "$MANIFEST" \
  --pin hardware_baseline_id --value bench-b --reason "rewired DMX electrical path" \
  --invalidate Q-DMX-21 >/dev/null
python3 "$ELECTRICAL_TOOL" status --state "$state" --json >"$tmp/repinned.json"
python3 - "$tmp/repinned.json" <<'PY'
import json, sys
data=json.load(open(sys.argv[1], encoding="utf-8"))
assert data["status"] == "PENDING"
assert all(row["status"] == "PENDING" for row in data["checks"])
PY
ack documented PASS "Re-documented the rewired physical path after hardware baseline change."
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-DMX-21)" == "BLOCKED" ]]
python3 "$ELECTRICAL_TOOL" status --state "$state" --json >"$tmp/repinned-partial.json"
python3 - "$tmp/repinned-partial.json" <<'PY'
import json, sys
data=json.load(open(sys.argv[1], encoding="utf-8"))
statuses={row["check"]:row["status"] for row in data["checks"]}
assert statuses["documented"] == "PASS"
assert all(statuses[name] == "PENDING" for name in ("logic-voltage","de-re","polarity","common","termination"))
PY

echo "qualification Q-DMX-21 electrical-path self-test PASS"
