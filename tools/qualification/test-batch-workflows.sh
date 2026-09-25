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
{"status":"PASS","selection_state":"INSUFFICIENT_HARDWARE","selected_group":"","device_ids":[],"eligible_device_ids":["tablet-01"],"eligible_device_count":1,"na_allowed":true}
JSON
python3 "$MILESTONE" record --state "$state" --manifest "$MANIFEST" --gate Q-TAB-16 --key group.availability --status PASS --actor self-test --evidence "$tmp/availability.json" --note insufficient >/dev/null
STAGECORE_QUALIFICATION_STATE="$state" "$CAMPAIGN" q16-na "only one physical Android tablet is available on this campaign hardware baseline" >/dev/null
[[ "$(python3 "$STATE_TOOL" get --state "$state" --gate Q-TAB-16)" == "N/A" ]]

eligible_state="$tmp/eligible-campaign.json"
python3 "$STATE_TOOL" init --state "$eligible_state" --manifest "$MANIFEST" --stagecore-sha test >/dev/null
cat >"$tmp/eligible.json" <<'JSON'
{"status":"PASS","selection_state":"ELIGIBLE","selected_group":"actors","device_ids":["tablet-01","tablet-02"],"eligible_device_ids":["tablet-01","tablet-02"],"eligible_device_count":2,"na_allowed":false}
JSON
python3 "$MILESTONE" record --state "$eligible_state" --manifest "$MANIFEST" --gate Q-TAB-16 --key group.availability --status PASS --actor self-test --evidence "$tmp/eligible.json" --note eligible >/dev/null
set +e
STAGECORE_QUALIFICATION_STATE="$eligible_state" "$CAMPAIGN" q16-na "should be refused" >/dev/null 2>&1
eligible_na_rc=$?
set -e
[[ "$eligible_na_rc" -eq 3 ]]
[[ "$(python3 "$STATE_TOOL" get --state "$eligible_state" --gate Q-TAB-16)" == "PENDING" ]]

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
