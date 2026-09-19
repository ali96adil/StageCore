#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STATE_TOOL="$ROOT/tools/qualification/qualification-state.py"
MILESTONE="$ROOT/tools/qualification/qualification-milestone.py"
CAMPAIGN="$ROOT/tools/qualification/campaign.sh"
MANIFEST="$ROOT/tools/qualification/manifest.json"
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
state="$tmp/campaign.json"
python3 "$STATE_TOOL" init --state "$state" --manifest "$MANIFEST" --stagecore-sha test >/dev/null

cat >"$tmp/availability.json" <<'JSON'
{"status":"PASS","selection_state":"INSUFFICIENT","selected_group":"","device_ids":[]}
JSON
python3 "$MILESTONE" record --state "$state" --manifest "$MANIFEST" --gate Q-TAB-16 --key group.availability --status PASS --actor self-test --evidence "$tmp/availability.json" --note insufficient >/dev/null
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" q16-na "only one physical Android tablet is available on this campaign hardware baseline" >/dev/null
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-TAB-16)" == "N/A" ]]

cat >"$tmp/baseline.json" <<'JSON'
{"project_id":"project-1","draft":{"revision_id":"draft-1","parent_revision_id":"parent-1"},"parent":{"revision_id":"parent-1"},"published_snapshot":{"runtime_snapshot_id":"snap-1"}}
JSON
python3 "$MILESTONE" record --state "$state" --manifest "$MANIFEST" --gate Q-DRAFT-01 --key baseline.state --status PASS --actor self-test --evidence "$tmp/baseline.json" --note baseline >/dev/null
python3 "$STATE_TOOL" record --state "$state" --manifest "$MANIFEST" --gate Q-DRAFT-02 --status PASS --actor self-test --evidence owner.json --note owner >/dev/null
python3 "$STATE_TOOL" record --state "$state" --manifest "$MANIFEST" --gate Q-DRAFT-03 --status PASS --actor self-test --evidence show.json --note show >/dev/null
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" draft-ack visible "Discard Draft control visibly identified the current recovery target" >/dev/null
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-DRAFT-01)" == "PASS" ]]
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" draft-ack confirmation "confirmation dialog explicitly named the Draft before destructive approval" >/dev/null
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-DRAFT-04)" == "PASS" ]]

echo "qualification Tablet-group/Draft campaign workflow self-test PASS"
