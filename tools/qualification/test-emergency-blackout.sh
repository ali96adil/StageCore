#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TOOL="$ROOT/tools/qualification/emergency-blackout.py"
STATE_TOOL="$ROOT/tools/qualification/qualification-state.py"
MILESTONE_TOOL="$ROOT/tools/qualification/qualification-milestone.py"
MANIFEST="$ROOT/tools/qualification/manifest.json"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

make_probe() {
  local path="$1" generated="$2" last_seen="$3" uptime="$4" level="$5" last_applied="$6"
  cat >"$path" <<EOF
{"schema_version":1,"generated_at":"$generated","devices":[{"device_id":"lighting-01","project_id":"project-1","profile_id":"stagecore.esp32-dmx-lighting-node","client_version":"0.2.0-dev","runtime":{"connection_state":"ONLINE","readiness":"READY","last_seen_at_us":$last_seen,"observed":{"schema_version":1,"firmware_version":"0.2.0-dev","uptime_seconds":$uptime,"reset_reason":"POWERON","current_levels":{"warm":$level,"cold":0},"active_fade":null,"last_applied_command_id":"$last_applied","dmx_healthy":true,"configuration_hash":"cfg-hash-1","brownout_warning":false,"authority":"STAGECORE"}}}]}
EOF
}

make_probe "$tmp/pre-probe.json" "2026-09-18T13:00:00+00:00" 1789736399000000 1000 35 cmd-pre
make_probe "$tmp/post-probe.json" "2026-09-18T13:01:00+00:00" 1789736459000000 1060 0 ""
make_probe "$tmp/stable-probe.json" "2026-09-18T13:01:03+00:00" 1789736462000000 1063 0 ""
make_probe "$tmp/unsafe-probe.json" "2026-09-18T13:02:00+00:00" 1789736519000000 1120 12 cmd-pre

python3 "$TOOL" capture --probe "$tmp/pre-probe.json" --device-id lighting-01 --project-id project-1   --require-nonzero-channel warm --out "$tmp/pre.json" >/dev/null
python3 "$TOOL" capture --probe "$tmp/post-probe.json" --device-id lighting-01 --project-id project-1 --out "$tmp/post.json" >/dev/null
python3 "$TOOL" capture --probe "$tmp/stable-probe.json" --device-id lighting-01 --project-id project-1 --out "$tmp/stable.json" >/dev/null
python3 "$TOOL" verify --pre "$tmp/pre.json" --post "$tmp/post.json" --stable "$tmp/stable.json"   --hub-unavailable-at "2026-09-18T13:00:10+00:00"   --blackout-at "2026-09-18T13:00:25+00:00"   --recovery-at "2026-09-18T13:00:45+00:00"   --out "$tmp/verify.json" >/dev/null

python3 - "$tmp/verify.json" <<'PY'
import json, sys
data=json.load(open(sys.argv[1],encoding="utf-8"))
assert data["status"] == "PASS"
assert data["no_esp_reboot"] is True
assert data["no_stale_replay"] is True
assert all(abs(v) <= 0.01 for v in data["stable_levels"].values())
PY

python3 "$TOOL" capture --probe "$tmp/unsafe-probe.json" --device-id lighting-01 --project-id project-1 --out "$tmp/unsafe.json" >/dev/null
set +e
python3 "$TOOL" verify --pre "$tmp/pre.json" --post "$tmp/unsafe.json" --stable "$tmp/unsafe.json"   --hub-unavailable-at "2026-09-18T13:00:10+00:00"   --blackout-at "2026-09-18T13:00:25+00:00"   --recovery-at "2026-09-18T13:00:45+00:00" >/dev/null 2>&1
unsafe_rc=$?
set -e
[[ "$unsafe_rc" -ne 0 ]]

state="$tmp/campaign.json"
python3 "$STATE_TOOL" init --state "$state" --manifest "$MANIFEST" --stagecore-sha test >/dev/null
python3 "$MILESTONE_TOOL" record --state "$state" --manifest "$MANIFEST"   --gate Q-DMX-19 --key emergency.pre --status PASS --actor self-test --evidence "$tmp/pre.json" --note prepared >/dev/null

set +e
STAGECORE_QUALIFICATION_STATE="$state" "$ROOT/tools/qualification/campaign.sh" q19-ack "too early" >/dev/null 2>&1
early_rc=$?
set -e
[[ "$early_rc" -ne 0 ]]

python3 "$MILESTONE_TOOL" record --state "$state" --manifest "$MANIFEST"   --gate Q-DMX-19 --key hub_unavailable.action --status PASS --actor self-test --evidence test --note stopped >/dev/null
STAGECORE_QUALIFICATION_STATE="$state" "$ROOT/tools/qualification/campaign.sh" q19-ack "local emergency blackout pressed while Hub unavailable" >/dev/null
[[ "$(python3 "$MILESTONE_TOOL" get --state "$state" --gate Q-DMX-19 --key local_blackout.action)" == "PASS" ]]

set +e
STAGECORE_QUALIFICATION_STATE="$state" "$ROOT/tools/qualification/campaign.sh" q19-ack "duplicate" >/dev/null 2>&1
duplicate_rc=$?
set -e
[[ "$duplicate_rc" -ne 0 ]]

echo "qualification local-emergency-blackout self-test PASS"
