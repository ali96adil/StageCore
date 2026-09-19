#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
STATE_TOOL="$ROOT/tools/qualification/qualification-state.py"
MILESTONE="$ROOT/tools/qualification/qualification-milestone.py"
CAMPAIGN="$ROOT/tools/qualification/campaign.sh"
MANIFEST="$ROOT/tools/qualification/manifest.json"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
state="$tmp/campaign.json"
python3 "$STATE_TOOL" init --state "$state" --manifest "$MANIFEST" --stagecore-sha test >/dev/null
cat >"$tmp/coverage.json" <<'JSON'
{"status":"PASS","mode":"source-coverage","project_id":"project-1",
 "configured_classes":["NETWORK_STREAM"],"unconfigured_classes":["LOCAL_CAMERA","USB_CAPTURE"],
 "sources":[{"source_id":"live-1","source_class":"NETWORK_STREAM","readiness":"READY",
             "desired_enabled":true,"execution_device_id":"renderer-1"}]}
JSON
python3 "$MILESTONE" record --state "$state" --manifest "$MANIFEST" --gate Q-LIVE-04 --key class.coverage \
  --status PASS --actor self-test --evidence "$tmp/coverage.json" --note "read-only real source coverage" >/dev/null
set +e
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" qlive04-ack LOCAL_CAMERA N/A "absent" >/dev/null 2>&1
unqualified_rc=$?
set -e
[[ "$unqualified_rc" -eq 3 ]]
python3 "$STATE_TOOL" record --state "$state" --manifest "$MANIFEST" --gate Q-LIVE-01 \
  --status PASS --actor self-test --evidence physical-observation --note "real source qualified" >/dev/null

set +e
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" qlive04-ack NETWORK_STREAM N/A "wrong N/A" >/dev/null 2>&1
configured_na_rc=$?
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" qlive04-ack USB_CAPTURE PASS "not configured" >/dev/null 2>&1
absent_pass_rc=$?
set -e
[[ "$configured_na_rc" -ne 0 && "$absent_pass_rc" -ne 0 ]]

STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" qlive04-ack LOCAL_CAMERA N/A \
  "No local camera hardware exists on the pinned campaign" >/dev/null
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" qlive04-ack USB_CAPTURE N/A \
  "No USB capture hardware is connected to the pinned campaign" >/dev/null
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-LIVE-04)" == "PENDING" ]]
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" qlive04-ack NETWORK_STREAM PASS \
  "Network stream was rendered on the real designated Companion client" >/dev/null
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-LIVE-04)" == "PASS" ]]
[[ "$(python3 "$MILESTONE" get --state "$state" --gate Q-LIVE-04 --key class.LOCAL_CAMERA)" == "N/A" ]]
[[ "$(python3 "$MILESTONE" get --state "$state" --gate Q-LIVE-04 --key class.USB_CAPTURE)" == "N/A" ]]
[[ "$(python3 "$MILESTONE" get --state "$state" --gate Q-LIVE-04 --key class.NETWORK_STREAM)" == "PASS" ]]
set +e
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" qlive04-ack LOCAL_CAMERA N/A "duplicate" >/dev/null 2>&1
locked_rc=$?
set -e
[[ "$locked_rc" -eq 3 ]]
echo "qualification explicit live source-class workflow self-test PASS"
