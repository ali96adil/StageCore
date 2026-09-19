#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
PROBE="$ROOT/tools/qualification/pi/stagecore-qualification-phase4-inventory.py"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
db="$tmp/stagecore.sqlite3"
python3 - "$db" <<'PY'
import sqlite3,sys,time
now=int(time.time()*1000000);c=sqlite3.connect(sys.argv[1])
c.executescript("""
CREATE TABLE projects(project_id TEXT PRIMARY KEY);
CREATE TABLE stage_devices(device_id TEXT PRIMARY KEY,project_id TEXT,device_kind TEXT,display_name TEXT,capabilities_json TEXT,enabled INTEGER,protocol_version TEXT);
CREATE TABLE stage_device_runtime_state(device_id TEXT PRIMARY KEY,connection_state TEXT,readiness TEXT,last_seen_at_us INTEGER);
CREATE TABLE live_video_sources(source_id TEXT PRIMARY KEY,project_id TEXT,name TEXT,source_class TEXT,execution_device_id TEXT,execution_machine_role_id TEXT,required INTEGER,desired_enabled INTEGER,readiness TEXT,last_observed_at_us INTEGER,endpoint_ref TEXT,config_json TEXT);
CREATE TABLE network_observations(observation_id TEXT PRIMARY KEY,target_kind TEXT,target_id TEXT,observed_at_us INTEGER,reachability TEXT,transport_state TEXT,latency_ms REAL,jitter_ms REAL,error_code TEXT,address TEXT,details_json TEXT);
""")
c.execute("INSERT INTO projects VALUES ('project-1')")
c.execute("INSERT INTO stage_devices VALUES (?,?,?,?,?,?,?)",("display-1","project-1","STAGE_DISPLAY","Crew display",'["display.message.show","display.chime.play"]',1,"stagecore.device/1"))
c.execute("INSERT INTO stage_device_runtime_state VALUES (?,?,?,?)",("display-1","ONLINE","READY",now))
c.execute("INSERT INTO live_video_sources VALUES (?,?,?,?,?,?,?,?,?,?,?,?)",("source-1","project-1","Camera","NETWORK_STREAM",None,"render-1",1,1,"READY",now,"rtsp://user:password@private","{\"secret\":\"unexposed\"}"))
c.execute("INSERT INTO network_observations VALUES (?,?,?,?,?,?,?,?,?,?,?)",("obs-1","STAGE_DEVICE","display-1",now,"REACHABLE","WEBSOCKET_CONNECTED",None,None,"","192.168.3.10","{\"token\":\"unexposed\"}"))
c.commit();c.close()
PY
printf '%s\n' '{"project_id":"project-1"}' | python3 "$PROBE" --db "$db" >"$tmp/inventory.json"
python3 - "$tmp/inventory.json" <<'PY'
import json,sys
raw=open(sys.argv[1],encoding="utf-8").read()
for secret in ("password","unexposed","192.168.3.10"): assert secret not in raw,secret
data=json.loads(raw)
assert data["status"]=="PASS"
assert data["availability"]["eligible_display_count"]==1
assert data["availability"]["eligible_chime_display_count"]==1
assert data["availability"]["configured_source_classes"]==["NETWORK_STREAM"]
assert data["network_targets"][0]["metric_provenance"]=="NOT_MEASURED"
PY
python3 - "$db" <<'PY'
import sqlite3,sys
c=sqlite3.connect(sys.argv[1])
c.execute("UPDATE network_observations SET latency_ms=18.2,jitter_ms=2.1 WHERE observation_id='obs-1'")
c.commit();c.close()
PY
printf '%s\n' '{"project_id":"project-1"}' | python3 "$PROBE" --db "$db" >"$tmp/metric.json"
python3 - "$tmp/metric.json" <<'PY'
import json,sys
assert json.load(open(sys.argv[1]))["network_targets"][0]["metric_provenance"]=="UNVERIFIED"
PY
set +e
printf '%s\n' '{"project_id":"missing"}' | python3 "$PROBE" --db "$db" >"$tmp/missing.json"
rc=$?
set -e
[[ "$rc" -eq 3 ]]
echo "qualification Phase 4 inventory self-test PASS"
