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
for spec in   "Q-TAB-06 prepare.command" "Q-TAB-06 play.command"   "Q-DMX-03 set.command"   "Q-DMX-04 multi_set.command"   "Q-DMX-05 fade.command"   "Q-DMX-06 multi_fade.command"   "Q-DMX-07 timing.measurement"   "Q-DMX-08 precondition_set.command" "Q-DMX-08 long_fade.command"   "Q-DMX-10 blackout.command"   "Q-DMX-11 precondition_set.command" "Q-DMX-11 timed_blackout.command"; do
  set -- $spec
  python3 "$MILESTONE_TOOL" record --state "$state" --manifest "$MANIFEST"     --gate "$1" --key "$2" --status PASS --actor self-test --evidence test.json --note ok >/dev/null
done

python3 "$CONFIRM" pending --state "$state" --json >"$tmp/pending.json"
python3 - "$tmp/pending.json" <<'PY'
import json, sys
rows=json.load(open(sys.argv[1], encoding="utf-8"))
ids={r["gate_id"] for r in rows}
for gate in ("Q-TAB-06","Q-DMX-03","Q-DMX-04","Q-DMX-05","Q-DMX-06","Q-DMX-07","Q-DMX-08","Q-DMX-10","Q-DMX-11"):
    assert gate in ids
assert "Q-TAB-07" not in ids
PY

python3 "$CONFIRM" confirm-one --state "$state" --gate Q-DMX-06 --status FAIL --note "sync observation failed" >/dev/null
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-DMX-06)" == "FAIL" ]]

python3 "$CONFIRM" confirm-all --state "$state" --status PASS --note "operator observed all remaining listed outputs" >/dev/null
for gate in Q-TAB-06 Q-DMX-03 Q-DMX-04 Q-DMX-05 Q-DMX-07 Q-DMX-08 Q-DMX-10 Q-DMX-11; do
  [[ "$(python3 "$STATE_TOOL" get --state "$state" --gate "$gate")" == "PASS" ]]
done
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-DMX-06)" == "FAIL" ]]
echo "qualification physical confirmation self-test PASS"
