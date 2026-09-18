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
for spec in "Q-TAB-06 prepare.command" "Q-TAB-06 play.command" "Q-DMX-03 set.command"; do
  set -- $spec
  python3 "$MILESTONE_TOOL" record --state "$state" --manifest "$MANIFEST" --gate "$1" --key "$2" --status PASS --actor self-test --evidence test.json --note ok >/dev/null
done
python3 "$CONFIRM" pending --state "$state" --json >"$tmp/pending.json"
python3 - "$tmp/pending.json" <<'PY'
import json, sys
rows=json.load(open(sys.argv[1], encoding="utf-8"))
ids={r["gate_id"] for r in rows}
assert "Q-TAB-06" in ids
assert "Q-DMX-03" in ids
assert "Q-TAB-07" not in ids
PY
python3 "$CONFIRM" confirm-all --state "$state" --status PASS --note "operator observed all listed outputs" >/dev/null
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-TAB-06)" == "PASS" ]]
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-DMX-03)" == "PASS" ]]
echo "qualification physical confirmation self-test PASS"
