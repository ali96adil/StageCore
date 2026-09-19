#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HELPER="$ROOT/tools/qualification/pi/stagecore-qualification-hub-restart.py"
VALIDATOR="$ROOT/tools/qualification/validate-hub-restart-evidence.py"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

python3 -m py_compile "$HELPER" "$VALIDATOR"

cat >"$tmp/pass.json" <<'EOF'
{
  "status":"PASS",
  "operation":"HUB_RESTART_NO_REPLAY",
  "device_id":"lighting-01",
  "project_id":"project-1",
  "precondition_command_id":"cmd-pre",
  "interrupted_command_id":"cmd-fade",
  "active_before_restart":true,
  "command_status_before_restart":"ACCEPTED",
  "command_status_after_restart":"FAILED",
  "command_terminal_error_code":"DEVICE_EXECUTION_INTERRUPTED",
  "command_row_count":1,
  "hub_ready_after_restart":true,
  "reconnected":true,
  "same_firmware":true,
  "same_configuration_hash":true,
  "no_esp_reboot":true,
  "no_stale_replay":true,
  "safe_blackout_after_reconnect":true,
  "post_levels":{"warm":0,"cold":0},
  "stable_levels_after_hold":{"warm":0,"cold":0},
  "post_readiness":"READY",
  "post_authority":"STAGECORE",
  "post_dmx_healthy":true
}
EOF
python3 "$VALIDATOR" --input "$tmp/pass.json" --device-id lighting-01 >/dev/null

python3 - "$tmp/pass.json" "$tmp/replay.json" <<'PY'
import json, sys
data=json.load(open(sys.argv[1], encoding="utf-8"))
data["no_stale_replay"]=False
data["stable_levels_after_hold"]["warm"]=25
json.dump(data, open(sys.argv[2],"w",encoding="utf-8"))
PY
set +e
python3 "$VALIDATOR" --input "$tmp/replay.json" --device-id lighting-01 >/dev/null 2>&1
replay_rc=$?
set -e
[[ "$replay_rc" -ne 0 ]]

python3 - "$tmp/pass.json" "$tmp/ambiguous.json" <<'PY'
import json, sys
data=json.load(open(sys.argv[1], encoding="utf-8"))
data["command_status_after_restart"]="ACCEPTED"
json.dump(data, open(sys.argv[2],"w",encoding="utf-8"))
PY
set +e
python3 "$VALIDATOR" --input "$tmp/ambiguous.json" --device-id lighting-01 >/dev/null 2>&1
ambiguous_rc=$?
set -e
[[ "$ambiguous_rc" -ne 0 ]]

echo "qualification Hub-restart evidence self-test PASS"
