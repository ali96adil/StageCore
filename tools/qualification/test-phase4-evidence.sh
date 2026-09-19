#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
HELPER="$ROOT/tools/qualification/pi/stagecore-qualification-phase4-evidence.py"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
db="$tmp/stagecore.sqlite3"

python3 - "$db" <<'PY'
import json,sqlite3,sys,time
c=sqlite3.connect(sys.argv[1])
now=int(time.time()*1000000)
c.executescript("""
CREATE TABLE projects(project_id TEXT PRIMARY KEY);
CREATE TABLE stage_devices(device_id TEXT PRIMARY KEY,project_id TEXT,device_kind TEXT,enabled INTEGER,protocol_version TEXT);
CREATE TABLE stage_device_runtime_state(device_id TEXT PRIMARY KEY,connection_state TEXT,readiness TEXT,last_seen_at_us INTEGER);
CREATE TABLE runtime_snapshots(runtime_snapshot_id TEXT PRIMARY KEY,project_id TEXT,revision_id TEXT,status TEXT);
CREATE TABLE cues(cue_id TEXT PRIMARY KEY,revision_id TEXT,name TEXT,cue_type TEXT,enabled INTEGER);
CREATE TABLE actions(action_id TEXT PRIMARY KEY,cue_id TEXT,order_index INTEGER,capability_key TEXT,enabled INTEGER,target_ref TEXT);
CREATE TABLE project_device_aliases(project_id TEXT,logical_name TEXT,logical_type TEXT,project_config_json TEXT);
CREATE TABLE sessions(session_id TEXT PRIMARY KEY,project_id TEXT,runtime_snapshot_id TEXT);
CREATE TABLE cue_executions(cue_execution_id TEXT PRIMARY KEY,cue_id TEXT,session_id TEXT,correlation_id TEXT,result TEXT,started_at_us INTEGER);
CREATE TABLE action_executions(action_execution_id TEXT PRIMARY KEY,cue_execution_id TEXT,action_id TEXT,result TEXT);
CREATE TABLE stage_device_commands(command_id TEXT PRIMARY KEY,project_id TEXT,device_id TEXT,correlation_id TEXT,
 command_type TEXT,status TEXT,causation_id TEXT,runtime_snapshot_id TEXT);
CREATE TABLE live_video_sources(source_id TEXT PRIMARY KEY,project_id TEXT,source_class TEXT,readiness TEXT,desired_enabled INTEGER,execution_device_id TEXT,execution_machine_role_id TEXT);
CREATE TABLE network_observations(observation_id TEXT PRIMARY KEY,target_kind TEXT,target_id TEXT,observed_at_us INTEGER,latency_ms REAL,jitter_ms REAL);
""")
c.execute("INSERT INTO projects VALUES ('p1')")
c.execute("INSERT INTO projects VALUES ('p2')")
c.execute("INSERT INTO stage_devices VALUES (?,?,?,?,?)",("display-1","p1","STAGE_DISPLAY",1,"stagecore.device/1"))
c.execute("INSERT INTO stage_device_runtime_state VALUES (?,?,?,?)",("display-1","ONLINE","READY",now))
c.execute("INSERT INTO runtime_snapshots VALUES (?,?,?,?)",("snap-1","p1","rev-1","PUBLISHED"))
c.execute("INSERT INTO cues VALUES (?,?,?,?,?)",("cue-1","rev-1","Qualification callboard","CUE",1))
c.execute("INSERT INTO actions VALUES (?,?,?,?,?,?)",("a1","cue-1",0,"display.message.show",1,"stage.display"))
c.execute("INSERT INTO project_device_aliases VALUES (?,?,?,?)",("p1","stage.display","stage_device",json.dumps({"device_id":"display-1","capability_key":"display.message.show"})))
c.execute("INSERT INTO sessions VALUES (?,?,?)",("session-1","p1","snap-1"))
c.execute("INSERT INTO cue_executions VALUES (?,?,?,?,?,?)",("exec-1","cue-1","session-1","correlation-1","COMPLETED",now-2_000_000))
c.execute("INSERT INTO action_executions VALUES (?,?,?,?)",("action-exec-1","exec-1","a1","COMPLETED"))
c.execute("INSERT INTO stage_device_commands VALUES (?,?,?,?,?,?,?,?)",("display-cmd-1","p1","display-1","correlation-1","DISPLAY_MESSAGE","COMPLETED","action-exec-1","snap-1"))
c.execute("INSERT INTO live_video_sources VALUES (?,?,?,?,?,?,?)",("live-1","p1","NETWORK_STREAM","READY",1,None,"render-1"))
c.execute("INSERT INTO network_observations VALUES (?,?,?,?,?,?)",("network-1","STAGE_DEVICE","display-1",now,None,None))
c.execute("INSERT INTO network_observations VALUES (?,?,?,?,?,?)",("network-other","STAGE_DEVICE","other-device",now,140.0,20.0))
c.commit();c.close()
PY

cat >"$tmp/cue.json" <<'JSON'
{"mode":"callboard-cue","project_id":"p1","device_id":"display-1","runtime_snapshot_id":"snap-1","cue_name":"Qualification callboard"}
JSON
cat >"$tmp/source.json" <<'JSON'
{"mode":"source-coverage","project_id":"p1"}
JSON
cat >"$tmp/network.json" <<'JSON'
{"mode":"network-null-metrics","project_id":"p1"}
JSON
python3 "$HELPER" --db "$db" <"$tmp/cue.json" >"$tmp/cue.out"
python3 "$HELPER" --db "$db" <"$tmp/source.json" >"$tmp/source.out"
python3 "$HELPER" --db "$db" <"$tmp/network.json" >"$tmp/network.out"
python3 - "$tmp/cue.out" "$tmp/source.out" "$tmp/network.out" <<'PY'
import json,sys
cue,source,network=(json.load(open(p,encoding="utf-8")) for p in sys.argv[1:])
assert cue["status"]=="PASS" and cue["action_count"]==1 and cue["command_ids"]==["display-cmd-1"]
assert cue["go_authority"] is False
assert source["status"]=="PASS" and source["configured_classes"]==["NETWORK_STREAM"]
assert source["unconfigured_classes"]==["LOCAL_CAMERA","USB_CAPTURE"]
assert network["status"]=="PASS" and network["not_measured_target_count"]==1
PY

python3 - "$db" <<'PY'
import sqlite3,sys,time
c=sqlite3.connect(sys.argv[1]);now=int(time.time()*1_000_000)
c.execute("INSERT INTO network_observations VALUES (?,?,?,?,?,?)",("network-2","STAGE_DEVICE","display-1",now+100,20.5,1.0))
c.commit();c.close()
PY
set +e
python3 "$HELPER" --db "$db" <"$tmp/network.json" >"$tmp/network-blocked.out"
network_rc=$?
set -e
[[ "$network_rc" -eq 3 ]]
grep -F '"status":"BLOCKED"' "$tmp/network-blocked.out" >/dev/null

python3 - "$db" <<'PY'
import sqlite3,sys
c=sqlite3.connect(sys.argv[1])
c.execute("UPDATE stage_device_commands SET causation_id='wrong' WHERE command_id='display-cmd-1'")
c.commit();c.close()
PY
set +e
python3 "$HELPER" --db "$db" <"$tmp/cue.json" >"$tmp/cue-fail.out"
cue_rc=$?
set -e
[[ "$cue_rc" -eq 1 ]]
grep -F '"status":"FAIL"' "$tmp/cue-fail.out" >/dev/null
echo "qualification Phase 4 Cue/source/network evidence self-test PASS"
