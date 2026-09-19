#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SELECT="$ROOT/tools/qualification/select-callboard.py"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cat >"$tmp/inventory.json" <<'JSON'
{"status":"PASS","project_id":"project-1","displays":[
{"device_id":"display-1","enabled":true,"fresh":true,"connection_state":"ONLINE","readiness":"READY","protocol_version":"stagecore.device/1","capabilities":["display.message.show","display.countdown.show","display.alert.show","display.clear"]},
{"device_id":"stale","enabled":true,"fresh":false,"connection_state":"ONLINE","readiness":"READY","protocol_version":"stagecore.device/1","capabilities":["display.message.show","display.countdown.show","display.alert.show","display.clear","display.chime.play"]}
]}
JSON
python3 "$SELECT" --input "$tmp/inventory.json" >"$tmp/selected"
grep -F $'display-1\tproject-1\t0' "$tmp/selected" >/dev/null
set +e
python3 "$SELECT" --input "$tmp/inventory.json" --device-id stale >/dev/null 2>&1
stale_rc=$?
python3 "$SELECT" --input "$tmp/inventory.json" --device-id unknown >/dev/null 2>&1
missing_rc=$?
set -e
[[ "$stale_rc" -eq 3 && "$missing_rc" -eq 3 ]]
python3 - "$tmp/inventory.json" <<'PY'
import json,sys
path=sys.argv[1]; data=json.load(open(path))
data["displays"][1]["fresh"]=True
json.dump(data,open(path,"w"))
PY
set +e
python3 "$SELECT" --input "$tmp/inventory.json" >/dev/null 2>&1
ambiguous_rc=$?
set -e
[[ "$ambiguous_rc" -eq 3 ]]
python3 "$SELECT" --input "$tmp/inventory.json" --device-id stale >"$tmp/chime"
grep -F $'stale\tproject-1\t1' "$tmp/chime" >/dev/null
echo "qualification Callboard target selection self-test PASS"
