#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STATE_TOOL="$ROOT/tools/qualification/qualification-state.py"
MILESTONE="$ROOT/tools/qualification/qualification-milestone.py"
CAMPAIGN="$ROOT/tools/qualification/campaign.sh"
MANIFEST="$ROOT/tools/qualification/manifest.json"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
state="$tmp/campaign.json"

python3 "$STATE_TOOL" init --state "$state" --manifest "$MANIFEST" --stagecore-sha test >/dev/null

python3 "$MILESTONE" record --state "$state" --manifest "$MANIFEST" \
  --gate Q-TAB-11 --key cue_builder.canonical --status PASS --actor self-test \
  --evidence canonical.json --note canonical >/dev/null
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" q11-ack \
  "graphical builder used normal fields only; no raw JSON, capability key or target_ref was exposed" >/dev/null
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-TAB-11)" == "PASS" ]]
[[ "$(python3 "$MILESTONE" get --state "$state" --gate Q-TAB-11 --key ui.observation)" == "PASS" ]]

python3 "$MILESTONE" record --state "$state" --manifest "$MANIFEST" \
  --gate Q-TAB-14 --key reconnect.pre --status PASS --actor self-test \
  --evidence reconnect.pre.json --note baseline >/dev/null

set +e
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" q14-ack reconnect "too early" >/dev/null 2>&1
early_rc=$?
set -e
[[ "$early_rc" -eq 3 ]]

STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" q14-ack disconnect \
  "tablet Wi-Fi was disconnected while app stayed powered" >/dev/null
[[ "$(python3 "$MILESTONE" get --state "$state" --gate Q-TAB-14 --key disconnect.action)" == "PASS" ]]

set +e
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" q14-ack disconnect "duplicate" >/dev/null 2>&1
duplicate_rc=$?
set -e
[[ "$duplicate_rc" -eq 3 ]]

STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" q14-ack reconnect \
  "tablet Wi-Fi restored and authenticated Stage Device channel returned" >/dev/null
[[ "$(python3 "$MILESTONE" get --state "$state" --gate Q-TAB-14 --key reconnect.action)" == "PASS" ]]

echo "qualification Tablet workflow self-test PASS"
