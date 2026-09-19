#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
S="$ROOT/tools/qualification/qualification-state.py"
M="$ROOT/tools/qualification/qualification-milestone.py"
CAMPAIGN="$ROOT/tools/qualification/campaign.sh"
MANIFEST="$ROOT/tools/qualification/manifest.json"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
state="$tmp/state.json"
python3 "$S" init --state "$state" --manifest "$MANIFEST" --stagecore-sha test >/dev/null
set +e
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" qnet-ack disconnect "too early" >/dev/null 2>&1
early=$?
set -e
[[ "$early" -eq 3 ]]
python3 "$M" record --state "$state" --manifest "$MANIFEST" --gate Q-NET-02 --key network.pre --status PASS --actor self-test --evidence "$tmp/pre.json" --note connected >/dev/null
set +e
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" qnet-ack reconnect "too early" >/dev/null 2>&1
reorder=$?
set -e
[[ "$reorder" -eq 3 ]]
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" qnet-ack disconnect "device only disconnected" >/dev/null
set +e
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" qnet-ack disconnect "duplicate" >/dev/null 2>&1
duplicate=$?
set -e
[[ "$duplicate" -eq 3 ]]
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" qnet-ack reconnect "device recovered" >/dev/null
[[ "$(python3 "$M" get --state "$state" --gate Q-NET-02 --key reconnect.action)" == "PASS" ]]
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" qnet-status >"$tmp/status.txt"
grep -F 'Q-NET-02::reconnect.action=PASS' "$tmp/status.txt" >/dev/null
set +e
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" qcall06-ack disconnect "too early" >/dev/null 2>&1
early=$?
set -e
[[ "$early" -eq 3 ]]
python3 "$M" record --state "$state" --manifest "$MANIFEST" --gate Q-CALL-06 --key reconnect.pre --status PASS --actor self-test --evidence "$tmp/callboard-pre.json" --note expired >/dev/null
set +e
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" qcall06-ack reconnect "too early" >/dev/null 2>&1
reorder=$?
set -e
[[ "$reorder" -eq 3 ]]
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" qcall06-ack disconnect "display-only network isolated" >/dev/null
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" qcall06-ack reconnect "display recovered" >/dev/null
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" qcall06-status >"$tmp/callboard-status.txt"
grep -F 'Q-CALL-06::reconnect.action=PASS' "$tmp/callboard-status.txt" >/dev/null
echo "qualification Network and Callboard workflow self-test PASS"
