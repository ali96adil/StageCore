#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TOOL="$ROOT/tools/qualification/power-event.py"
STATE_TOOL="$ROOT/tools/qualification/qualification-state.py"
MILESTONE_TOOL="$ROOT/tools/qualification/qualification-milestone.py"
MANIFEST="$ROOT/tools/qualification/manifest.json"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

make_probe() {
  local path="$1" generated="$2" last_seen="$3" uptime="$4" reason="$5" readiness="$6" brownout="$7" level="$8"
  cat >"$path" <<EOF
{"schema_version":1,"generated_at":"$generated","devices":[{"device_id":"lighting-01","project_id":"project-1","profile_id":"stagecore.esp32-dmx-lighting-node","client_version":"0.2.0-dev","runtime":{"connection_state":"ONLINE","readiness":"$readiness","last_seen_at_us":$last_seen,"observed":{"schema_version":1,"firmware_version":"0.2.0-dev","uptime_seconds":$uptime,"reset_reason":"$reason","current_levels":{"warm":$level,"cold":0},"dmx_healthy":true,"configuration_hash":"cfg-hash-1","brownout_warning":$brownout,"authority":"STAGECORE"}}}]}
EOF
}

make_probe "$tmp/pre-probe.json" "2026-09-18T12:00:00+00:00" 1789732799000000 1000 POWERON READY false 35
make_probe "$tmp/power-post-probe.json" "2026-09-18T12:01:10+00:00" 1789732869000000 10 POWERON READY false 0
make_probe "$tmp/brownout-pre-probe.json" "2026-09-18T12:01:20+00:00" 1789732879000000 20 POWERON READY false 0
make_probe "$tmp/brownout-post-probe.json" "2026-09-18T12:02:10+00:00" 1789732929000000 10 BROWNOUT WARNING true 0
make_probe "$tmp/unsafe-post-probe.json" "2026-09-18T12:03:10+00:00" 1789732989000000 10 POWERON READY false 10

python3 "$TOOL" capture --probe "$tmp/pre-probe.json" --device-id lighting-01 --project-id project-1 --out "$tmp/power-pre.json" >/dev/null
python3 "$TOOL" capture --probe "$tmp/power-post-probe.json" --device-id lighting-01 --project-id project-1 --out "$tmp/power-post.json" >/dev/null
python3 "$TOOL" verify --event power-cycle --pre "$tmp/power-pre.json" --post "$tmp/power-post.json"   --action-at "2026-09-18T12:01:05+00:00" --out "$tmp/power-verify.json" >/dev/null

python3 "$TOOL" capture --probe "$tmp/brownout-pre-probe.json" --device-id lighting-01 --project-id project-1 --out "$tmp/brownout-pre.json" >/dev/null
python3 "$TOOL" capture --probe "$tmp/brownout-post-probe.json" --device-id lighting-01 --project-id project-1 --out "$tmp/brownout-post.json" >/dev/null
python3 "$TOOL" verify --event brownout --pre "$tmp/brownout-pre.json" --post "$tmp/brownout-post.json"   --action-at "2026-09-18T12:02:05+00:00" --out "$tmp/brownout-verify.json" >/dev/null

python3 "$TOOL" capture --probe "$tmp/unsafe-post-probe.json" --device-id lighting-01 --project-id project-1 --out "$tmp/unsafe-post.json" >/dev/null
set +e
python3 "$TOOL" verify --event power-cycle --pre "$tmp/power-pre.json" --post "$tmp/unsafe-post.json"   --action-at "2026-09-18T12:03:05+00:00" >/dev/null 2>&1
unsafe_rc=$?
set -e
[[ "$unsafe_rc" -ne 0 ]]

state="$tmp/campaign.json"
python3 "$STATE_TOOL" init --state "$state" --manifest "$MANIFEST" --stagecore-sha test >/dev/null
python3 "$MILESTONE_TOOL" record --state "$state" --manifest "$MANIFEST"   --gate Q-DMX-15 --key power_cycle.pre --status PASS --actor self-test --evidence "$tmp/power-pre.json" --note prepared >/dev/null
STAGECORE_QUALIFICATION_STATE="$state" "$ROOT/tools/qualification/campaign.sh" q15-ack power-cycle "power removed and restored" >/dev/null
[[ "$(python3 "$MILESTONE_TOOL" get --state "$state" --gate Q-DMX-15 --key power_cycle.action)" == "PASS" ]]

set +e
STAGECORE_QUALIFICATION_STATE="$state" "$ROOT/tools/qualification/campaign.sh" q15-ack brownout "controlled brownout performed" >/dev/null 2>&1
brownout_early_rc=$?
set -e
[[ "$brownout_early_rc" -ne 0 ]]

echo "qualification power-event self-test PASS"
