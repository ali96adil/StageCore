#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
STATE_TOOL="$ROOT/tools/qualification/qualification-state.py"
MILESTONE="$ROOT/tools/qualification/qualification-milestone.py"
MANIFEST="$ROOT/tools/qualification/manifest.json"
CAMPAIGN="$ROOT/tools/qualification/campaign.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
state="$tmp/campaign.json"
python3 "$STATE_TOOL" init --state "$state" --manifest "$MANIFEST" --stagecore-sha test >/dev/null
cat >"$tmp/inventory.json" <<'JSON'
{"status":"PASS","project_id":"project-1","availability":{"eligible_display_count":1,"eligible_chime_display_count":0},"displays":[{"device_id":"display-1","enabled":true,"fresh":true,"connection_state":"ONLINE","readiness":"READY","capabilities":["display.message.show","display.countdown.show","display.alert.show","display.clear"]}]}
JSON
cat >"$tmp/message.json" <<'JSON'
{"device_id":"display-1","status":"COMPLETED","qualification_command":"DISPLAY_MESSAGE"}
JSON
python3 "$MILESTONE" record --state "$state" --manifest "$MANIFEST" --gate Q-CALL-01 --key inventory.baseline --status PASS --actor self-test --evidence "$tmp/inventory.json" --note baseline >/dev/null
python3 "$MILESTONE" record --state "$state" --manifest "$MANIFEST" --gate Q-CALL-01 --key message.command --status PASS --actor self-test --evidence "$tmp/message.json" --note command >/dev/null
set +e
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" qcall04-na "premature" >/dev/null 2>&1
early=$?
set -e
[[ "$early" -eq 3 ]]
python3 "$STATE_TOOL" record --state "$state" --manifest "$MANIFEST" --gate Q-CALL-01 --status PASS --actor self-test --evidence physical-observation --note "real message observed" >/dev/null
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" qcall04-na "selected physical display does not advertise a chime; none available on this pinned hardware" >/dev/null
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-CALL-04)" == "N/A" ]]
eligible_state="$tmp/eligible.json"
python3 "$STATE_TOOL" init --state "$eligible_state" --manifest "$MANIFEST" --stagecore-sha test >/dev/null
python3 - "$tmp/inventory.json" "$tmp/with-chime.json" <<'PY'
import json,sys
d=json.load(open(sys.argv[1]));d["availability"]["eligible_chime_display_count"]=1
json.dump(d,open(sys.argv[2],"w"))
PY
python3 "$MILESTONE" record --state "$eligible_state" --manifest "$MANIFEST" --gate Q-CALL-01 --key inventory.baseline --status PASS --actor self-test --evidence "$tmp/with-chime.json" --note eligible >/dev/null
python3 "$MILESTONE" record --state "$eligible_state" --manifest "$MANIFEST" --gate Q-CALL-01 --key message.command --status PASS --actor self-test --evidence "$tmp/message.json" --note command >/dev/null
python3 "$STATE_TOOL" record --state "$eligible_state" --manifest "$MANIFEST" --gate Q-CALL-01 --status PASS --actor self-test --evidence physical-observation --note "real message observed" >/dev/null
set +e
STAGECORE_QUALIFICATION_STATE="$eligible_state" "$CAMPAIGN" qcall04-na "not allowed" >/dev/null 2>&1
bad=$?
set -e
[[ "$bad" -eq 3 ]]
[[ "$(python3 "$STATE_TOOL" get --state "$eligible_state" --gate Q-CALL-04)" == "PENDING" ]]
echo "qualification optional Callboard chime N/A workflow self-test PASS"
