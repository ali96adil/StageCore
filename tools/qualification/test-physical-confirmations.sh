#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STATE_TOOL="$ROOT/tools/qualification/qualification-state.py"
MILESTONE_TOOL="$ROOT/tools/qualification/qualification-milestone.py"
CONFIRM="$ROOT/tools/qualification/physical-confirmations.py"
MANIFEST="$ROOT/tools/qualification/manifest.json"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
state="$tmp/campaign.json"

python3 "$STATE_TOOL" init --state "$state" --manifest "$MANIFEST" --stagecore-sha test >/dev/null
for spec in \
  "Q-TAB-06 prepare.command" "Q-TAB-06 play.command" \
  "Q-TAB-07 pause.command" "Q-TAB-07 stop.command" \
  "Q-TAB-08 overlay_play.command" "Q-TAB-08 overlay_clear.command" \
  "Q-TAB-09 live_show.command" "Q-TAB-09 live_hide.command" \
  "Q-TAB-10 blackout.command" "Q-TAB-10 blackout_clear.command" \
  "Q-TAB-12 published.execution" \
  "Q-TAB-13 missing_media.command" \
  "Q-TAB-14 reconnect.post" \
  "Q-TAB-16 group_play.command" \
  "Q-CALL-01 message.command" "Q-CALL-02 countdown.command" \
  "Q-CALL-03 alert.command" "Q-CALL-03 clear.command" "Q-CALL-04 chime.command" \
  "Q-DMX-03 set.command" "Q-DMX-04 multi_set.command" \
  "Q-DMX-05 fade.command" "Q-DMX-06 multi_fade.command" \
  "Q-DMX-07 timing.measurement" \
  "Q-DMX-08 precondition_set.command" "Q-DMX-08 long_fade.command" \
  "Q-DMX-09 supersession.sequence" \
  "Q-DMX-10 blackout.command" \
  "Q-DMX-11 precondition_set.command" "Q-DMX-11 timed_blackout.command" \
  "Q-DMX-14 invalid_value.sequence" \
  "Q-DMX-15 power_cycle.post" "Q-DMX-15 brownout.post" \
  "Q-DMX-16 wifi_loss.post" \
  "Q-DMX-17 restart.sequence" \
  "Q-DMX-18 local_web.action" "Q-DMX-18 stress.auto" \
  "Q-DMX-19 local_blackout.action" "Q-DMX-19 emergency.post" \
  "Q-DMX-22 regression.prereqs"; do
  set -- $spec
  python3 "$MILESTONE_TOOL" record --state "$state" --manifest "$MANIFEST" \
    --gate "$1" --key "$2" --status PASS --actor self-test --evidence test.json --note ok >/dev/null
done

python3 "$CONFIRM" pending --state "$state" --json >"$tmp/pending.json"
python3 - "$tmp/pending.json" <<'PY'
import json, sys
rows=json.load(open(sys.argv[1], encoding="utf-8"))
ids={r["gate_id"] for r in rows}
for gate in (
    "Q-TAB-06","Q-TAB-07","Q-TAB-08","Q-TAB-09","Q-TAB-10","Q-TAB-12","Q-TAB-13","Q-TAB-14","Q-TAB-16","Q-CALL-01","Q-CALL-02","Q-CALL-03","Q-CALL-04",
    "Q-DMX-03","Q-DMX-04","Q-DMX-05","Q-DMX-06","Q-DMX-07","Q-DMX-08",
    "Q-DMX-09","Q-DMX-10","Q-DMX-11","Q-DMX-14","Q-DMX-15","Q-DMX-16",
    "Q-DMX-17","Q-DMX-18","Q-DMX-19","Q-DMX-22",
):
    assert gate in ids, gate
PY

python3 "$CONFIRM" confirm-one --state "$state" --gate Q-DMX-06 --status FAIL --note "sync observation failed" >/dev/null
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-DMX-06)" == "FAIL" ]]

python3 "$CONFIRM" confirm-all --state "$state" --status PASS --note "operator observed all remaining listed outputs" >/dev/null
for gate in Q-TAB-06 Q-TAB-07 Q-TAB-08 Q-TAB-09 Q-TAB-10 Q-TAB-12 Q-TAB-13 Q-TAB-14 Q-TAB-16 Q-CALL-01 Q-CALL-02 Q-CALL-03 Q-CALL-04 Q-DMX-03 Q-DMX-04 Q-DMX-05 Q-DMX-07 Q-DMX-08 Q-DMX-09 Q-DMX-10 Q-DMX-11 Q-DMX-14 Q-DMX-15 Q-DMX-16 Q-DMX-17 Q-DMX-18 Q-DMX-19 Q-DMX-22; do
  [[ "$(python3 "$STATE_TOOL" get --state "$state" --gate "$gate")" == "PASS" ]]
done
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-DMX-06)" == "FAIL" ]]
echo "qualification physical confirmation self-test PASS"
