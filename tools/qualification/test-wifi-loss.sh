#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TOOL="$ROOT/tools/qualification/wifi-loss.py"
STATE_TOOL="$ROOT/tools/qualification/qualification-state.py"
MILESTONE_TOOL="$ROOT/tools/qualification/qualification-milestone.py"
MANIFEST="$ROOT/tools/qualification/manifest.json"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

make_probe() {
  local path="$1" generated="$2" last_seen="$3" uptime="$4" readiness="$5" level="$6"
  cat >"$path" <<EOF
{"schema_version":1,"generated_at":"$generated","devices":[{"device_id":"lighting-01","project_id":"project-1","profile_id":"stagecore.esp32-dmx-lighting-node","client_version":"0.2.0-dev","runtime":{"connection_state":"ONLINE","readiness":"$readiness","last_seen_at_us":$last_seen,"observed":{"schema_version":1,"firmware_version":"0.2.0-dev","uptime_seconds":$uptime,"reset_reason":"BROWNOUT","current_levels":{"warm":$level,"cold":0},"active_fade":null,"dmx_healthy":true,"configuration_hash":"cfg-hash-1","brownout_warning":true,"authority":"STAGECORE"}}}]}
EOF
}

make_probe "$tmp/pre-probe.json" "2026-09-18T12:10:00+00:00" 1789733399000000 490 WARNING 35
make_probe "$tmp/post-probe.json" "2026-09-18T12:10:40+00:00" 1789733439000000 530 WARNING 0
make_probe "$tmp/rebooted-post-probe.json" "2026-09-18T12:11:40+00:00" 1789733499000000 10 READY 0
make_probe "$tmp/nonzero-post-probe.json" "2026-09-18T12:12:40+00:00" 1789733559000000 650 WARNING 15

python3 "$TOOL" capture --probe "$tmp/pre-probe.json" --device-id lighting-01 --project-id project-1   --require-nonzero-channel warm --out "$tmp/pre.json" >/dev/null
python3 "$TOOL" capture --probe "$tmp/post-probe.json" --device-id lighting-01 --project-id project-1   --out "$tmp/post.json" >/dev/null
python3 "$TOOL" verify --pre "$tmp/pre.json" --post "$tmp/post.json"   --disconnect-at "2026-09-18T12:10:05+00:00" --reconnect-at "2026-09-18T12:10:35+00:00"   --out "$tmp/verify.json" >/dev/null

python3 - "$tmp/verify.json" <<'PY'
import json, sys
e=json.load(open(sys.argv[1], encoding="utf-8"))
assert e["status"] == "PASS"
assert e["expected_physical_policy"] == "brief hold, then fade to blackout"
assert e["no_reboot_observed"] is True
assert e["outage_ack_seconds"] == 30.0
assert all(abs(v) <= 0.01 for v in e["current_levels"].values())
PY

python3 "$TOOL" capture --probe "$tmp/rebooted-post-probe.json" --device-id lighting-01 --project-id project-1   --out "$tmp/rebooted.json" >/dev/null
set +e
python3 "$TOOL" verify --pre "$tmp/pre.json" --post "$tmp/rebooted.json"   --disconnect-at "2026-09-18T12:11:05+00:00" --reconnect-at "2026-09-18T12:11:35+00:00" >/dev/null 2>&1
reboot_rc=$?
set -e
[[ "$reboot_rc" -ne 0 ]]

python3 "$TOOL" capture --probe "$tmp/nonzero-post-probe.json" --device-id lighting-01 --project-id project-1   --out "$tmp/nonzero.json" >/dev/null
set +e
python3 "$TOOL" verify --pre "$tmp/pre.json" --post "$tmp/nonzero.json"   --disconnect-at "2026-09-18T12:12:05+00:00" --reconnect-at "2026-09-18T12:12:35+00:00" >/dev/null 2>&1
nonzero_rc=$?
set -e
[[ "$nonzero_rc" -ne 0 ]]

state="$tmp/campaign.json"
python3 "$STATE_TOOL" init --state "$state" --manifest "$MANIFEST" --stagecore-sha test >/dev/null
python3 "$MILESTONE_TOOL" record --state "$state" --manifest "$MANIFEST"   --gate Q-DMX-16 --key wifi_loss.pre --status PASS --actor self-test --evidence "$tmp/pre.json" --note prepared >/dev/null
STAGECORE_QUALIFICATION_STATE="$state" "$ROOT/tools/qualification/campaign.sh" q16-ack disconnect "ESP32 Wi-Fi isolated" >/dev/null
[[ "$(python3 "$MILESTONE_TOOL" get --state "$state" --gate Q-DMX-16 --key wifi_loss.disconnect_action)" == "PASS" ]]

STAGECORE_QUALIFICATION_STATE="$state" "$ROOT/tools/qualification/campaign.sh" q16-ack reconnect "ESP32 Wi-Fi restored" >/dev/null
[[ "$(python3 "$MILESTONE_TOOL" get --state "$state" --gate Q-DMX-16 --key wifi_loss.reconnect_action)" == "PASS" ]]

set +e
STAGECORE_QUALIFICATION_STATE="$state" "$ROOT/tools/qualification/campaign.sh" q16-ack reconnect "duplicate reconnect" >/dev/null 2>&1
duplicate_rc=$?
set -e
[[ "$duplicate_rc" -ne 0 ]]

echo "qualification Wi-Fi-loss self-test PASS"
