#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STATE_TOOL="$ROOT/tools/qualification/qualification-state.py"
MILESTONE_TOOL="$ROOT/tools/qualification/qualification-milestone.py"
MANIFEST="$ROOT/tools/qualification/manifest.json"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
state="$tmp/campaign.json"

python3 "$STATE_TOOL" init --state "$state" --manifest "$MANIFEST" --stagecore-sha test >/dev/null
[[ "$(python3 "$MILESTONE_TOOL" get --state "$state" --gate Q-TAB-06 --key prepare.command)" == "PENDING" ]]
python3 "$MILESTONE_TOOL" record --state "$state" --manifest "$MANIFEST"   --gate Q-TAB-06 --key prepare.command --status PASS --actor self-test --evidence command.json --note "prepared" >/dev/null
[[ "$(python3 "$MILESTONE_TOOL" get --state "$state" --gate Q-TAB-06 --key prepare.command)" == "PASS" ]]
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-TAB-06)" == "PENDING" ]]
python3 "$MILESTONE_TOOL" record --state "$state" --manifest "$MANIFEST"   --gate Q-TAB-06 --key prepare.command --status FAIL --actor self-test --evidence retry.json --note "retry failed" >/dev/null
python3 "$MILESTONE_TOOL" get --state "$state" --gate Q-TAB-06 --key prepare.command --json >"$tmp/milestone.json"
python3 - "$tmp/milestone.json" <<'PY'
import json, sys
m=json.load(open(sys.argv[1], encoding="utf-8"))
assert m["status"] == "FAIL"
assert m["history"] and m["history"][-1]["status"] == "PASS"
PY
echo "qualification milestone self-test PASS"
