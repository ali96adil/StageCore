#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HELPER="$ROOT/tools/qualification/pi/stagecore-qualification-tablet-evidence.py"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
db="$tmp/stagecore.sqlite3"

python3 - "$db" <<'PY'
import json, sqlite3, sys, time
db=sys.argv[1]; now=int(time.time()*1_000_000)
c=sqlite3.connect(db)
c.executescript("""
CREATE TABLE projects(project_id TEXT PRIMARY KEY,current_revision_id TEXT);
CREATE TABLE cues(cue_id TEXT PRIMARY KEY,revision_id TEXT,display_label TEXT,name TEXT,cue_type TEXT,enabled INTEGER);
CREATE TABLE actions(action_id TEXT PRIMARY KEY,cue_id TEXT,order_index INTEGER,execution_mode TEXT,target_ref TEXT,capability_key TEXT,parameters_json TEXT,priority_class TEXT,enabled INTEGER);
CREATE TABLE project_device_aliases(project_id TEXT,logical_name TEXT,logical_type TEXT,project_config_json TEXT);
CREATE TABLE stage_devices(device_id TEXT PRIMARY KEY,project_id TEXT,profile_id TEXT,device_kind TEXT,client_version TEXT);
CREATE TABLE stage_device_runtime_state(device_id TEXT PRIMARY KEY,connection_state TEXT,readiness TEXT,last_seen_at_us INTEGER,observed_state_json TEXT);
CREATE TABLE runtime_snapshots(runtime_snapshot_id TEXT PRIMARY KEY,project_id TEXT,revision_id TEXT,status TEXT);
CREATE TABLE sessions(session_id TEXT PRIMARY KEY,project_id TEXT,runtime_snapshot_id TEXT);
CREATE TABLE cue_executions(cue_execution_id TEXT PRIMARY KEY,session_id TEXT,cue_id TEXT,correlation_id TEXT,started_at_us INTEGER,completed_at_us INTEGER,result TEXT);
CREATE TABLE action_executions(action_execution_id TEXT PRIMARY KEY,cue_execution_id TEXT,action_id TEXT,result TEXT,error_code TEXT);
CREATE TABLE stage_device_commands(command_id TEXT PRIMARY KEY,device_id TEXT,project_id TEXT,command_type TEXT,runtime_snapshot_id TEXT,correlation_id TEXT,causation_id TEXT,status TEXT,issued_at_us INTEGER);
CREATE TABLE network_observations(target_kind TEXT,target_id TEXT,observed_at_us INTEGER,reachability TEXT,transport_state TEXT);
""")
project="project-1"; rev="rev-1"; cue="cue-1"; action="action-1"; device="tablet-01"; snapshot="snapshot-1"
c.execute("INSERT INTO projects VALUES (?,?)",(project,rev))
c.execute("INSERT INTO cues VALUES (?,?,?,?,?,1)",(cue,rev,"QT","Qualification Tablet Scene","TABLET_SCENE"))
c.execute("INSERT INTO actions VALUES (?,?,?,?,?,?,?,?,1)",(action,cue,0,"PARALLEL_BARRIER","tablet.tablet-01.play","tablet.media.play",'{"media_number":1}',"P1"))
c.execute("INSERT INTO project_device_aliases VALUES (?,?,?,?)",(project,"tablet.tablet-01.play","stage_device",json.dumps({"device_id":device,"capability_key":"tablet.media.play"})))
c.execute("INSERT INTO stage_devices VALUES (?,?,?,?,?)",(device,project,"stagecore.tablet-player","TABLET_PLAYER","1.0.0-rc3"))
c.execute("INSERT INTO stage_device_runtime_state VALUES (?,?,?,?,?)",(device,"ONLINE","READY",now,json.dumps({"project_id":project,"runtime_snapshot_id":snapshot,"tablet_manifest_id":"manifest-1"})))
c.execute("INSERT INTO runtime_snapshots VALUES (?,?,?,?)",(snapshot,project,rev,"PUBLISHED"))
c.execute("INSERT INTO sessions VALUES (?,?,?)",("session-1",project,snapshot))
c.execute("INSERT INTO cue_executions VALUES (?,?,?,?,?,?,?)",("exec-1","session-1",cue,"corr-1",now-2_000_000,now-1_000_000,"COMPLETED"))
c.execute("INSERT INTO action_executions VALUES (?,?,?,?,?)",("aexec-1","exec-1",action,"COMPLETED",None))
c.execute("INSERT INTO stage_device_commands VALUES (?,?,?,?,?,?,?,?,?)",("cmd-1",device,project,"TABLET_PLAY",snapshot,"corr-1","aexec-1","COMPLETED",now-1_500_000))
c.execute("INSERT INTO network_observations VALUES (?,?,?,?,?)",("STAGE_DEVICE",device,now-900_000,"UNREACHABLE","WEBSOCKET_DISCONNECTED"))
c.execute("INSERT INTO network_observations VALUES (?,?,?,?,?)",("STAGE_DEVICE",device,now-400_000,"REACHABLE","WEBSOCKET_CONNECTED"))
c.commit(); c.close()
PY

printf '%s\n' '{"mode":"cue-canonical","project_id":"project-1","device_id":"tablet-01","cue_name":"Qualification Tablet Scene"}' | \
  python3 "$HELPER" --db "$db" >"$tmp/canonical.json"
grep -F '"status":"PASS"' "$tmp/canonical.json" >/dev/null

printf '%s\n' '{"mode":"cue-execution","project_id":"project-1","device_id":"tablet-01","cue_name":"Qualification Tablet Scene","runtime_snapshot_id":"snapshot-1"}' | \
  python3 "$HELPER" --db "$db" >"$tmp/execution.json"
grep -F '"command_type":"TABLET_PLAY"' "$tmp/execution.json" >/dev/null

printf '%s\n' '{"mode":"reconnect-pre","project_id":"project-1","device_id":"tablet-01","runtime_snapshot_id":"snapshot-1"}' | \
  python3 "$HELPER" --db "$db" >"$tmp/pre.json"
baseline="$(python3 - "$tmp/pre.json" <<'PY'
import json,sys
print(json.load(open(sys.argv[1]))["baseline_issued_at_us"])
PY
)"
now_iso="$(python3 - <<'PY'
import datetime
print(datetime.datetime.now(datetime.timezone.utc).isoformat())
PY
)"
baseline_captured_at="$(python3 - "$tmp/pre.json" <<'PY'
import json,sys
print(json.load(open(sys.argv[1]))["captured_at"])
PY
)"
printf '{"mode":"reconnect-post","project_id":"project-1","device_id":"tablet-01","runtime_snapshot_id":"snapshot-1","baseline_issued_at_us":%s,"baseline_captured_at":"%s","disconnect_at":"%s","reconnect_at":"%s"}\n' "$baseline" "$baseline_captured_at" "$now_iso" "$now_iso" | \
  python3 "$HELPER" --db "$db" >"$tmp/post.json"
grep -F '"commands_after_baseline":0' "$tmp/post.json" >/dev/null

echo "qualification Tablet remaining-evidence self-test PASS"
