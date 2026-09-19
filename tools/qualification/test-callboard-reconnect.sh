#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
H="$ROOT/tools/qualification/pi/stagecore-qualification-callboard-reconnect.py"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
db="$tmp/db.sqlite"
python3 - "$db" <<'PY'
import sqlite3,sys,time
now=int(time.time()*1000000)
c=sqlite3.connect(sys.argv[1])
c.executescript("""
CREATE TABLE stage_devices(device_id TEXT PRIMARY KEY,project_id TEXT,device_kind TEXT,enabled INTEGER,protocol_version TEXT);
CREATE TABLE stage_device_runtime_state(device_id TEXT PRIMARY KEY,connection_state TEXT,readiness TEXT,last_seen_at_us INTEGER);
CREATE TABLE stage_display_state(device_id TEXT PRIMARY KEY,mode TEXT,command_id TEXT,expires_at_us INTEGER);
CREATE TABLE stage_device_commands(command_id TEXT PRIMARY KEY,project_id TEXT,device_id TEXT,command_type TEXT,status TEXT,completed_at_us INTEGER,payload_json TEXT,issued_at_us INTEGER);
CREATE TABLE network_observations(observation_id TEXT PRIMARY KEY,target_kind TEXT,target_id TEXT,observed_at_us INTEGER,reachability TEXT,transport_state TEXT,error_code TEXT);
""")
c.execute("INSERT INTO stage_devices VALUES('display-1','project-1','STAGE_DISPLAY',1,'stagecore.device/1')")
c.execute("INSERT INTO stage_device_runtime_state VALUES('display-1','ONLINE','READY',?)",(now,))
c.execute("INSERT INTO stage_display_state VALUES('display-1','IDLE','clear-1',NULL)")
c.execute("INSERT INTO stage_device_commands VALUES('alert-1','project-1','display-1','DISPLAY_ALERT','COMPLETED',?,'{\"duration_seconds\":3}',?)",(now-7000000,now-7000000))
c.execute("INSERT INTO stage_device_commands VALUES('clear-1','project-1','display-1','DISPLAY_CLEAR','COMPLETED',?,'{}',?)",(now-3000000,now-3000000))
c.execute("INSERT INTO network_observations VALUES('base-1','STAGE_DEVICE','display-1',?,'REACHABLE','WEBSOCKET_CONNECTED','')",(now,))
c.commit();c.close()
PY
printf '%s\n' '{"mode":"pre","project_id":"project-1","device_id":"display-1"}' | python3 "$H" --db "$db" >"$tmp/pre.json"
python3 - "$tmp/pre.json" <<'PY'
import json,sys
v=json.load(open(sys.argv[1]));assert v["status"]=="PASS" and v["alert_command_id"]=="alert-1" and v["physical_observation"] is False
PY
python3 - "$db" <<'PY'
import sqlite3,sys,time
c=sqlite3.connect(sys.argv[1]);now=int(time.time()*1000000)
c.execute("INSERT INTO network_observations VALUES('down','STAGE_DEVICE','display-1',?,'UNREACHABLE','WEBSOCKET_DISCONNECTED','')",(now,))
c.execute("INSERT INTO network_observations VALUES('up','STAGE_DEVICE','display-1',?,'REACHABLE','WEBSOCKET_CONNECTED','')",(now+100,))
c.execute("UPDATE stage_device_runtime_state SET last_seen_at_us=? WHERE device_id='display-1'",(now,))
c.commit();c.close()
PY
python3 - "$tmp/pre.json" <<'PY' | python3 "$H" --db "$db" >"$tmp/post.json"
import json,sys
print(json.dumps({"mode":"post","project_id":"project-1","device_id":"display-1","baseline":json.load(open(sys.argv[1]))}))
PY
python3 - "$tmp/post.json" <<'PY'
import json,sys
v=json.load(open(sys.argv[1]));assert v["status"]=="PASS" and v["reconnect_observation_id"]=="up" and v["physical_observation"] is False
PY
python3 - "$db" <<'PY'
import sqlite3,sys
c=sqlite3.connect(sys.argv[1])
c.execute("INSERT INTO stage_device_commands VALUES('replay','project-1','display-1','DISPLAY_ALERT','COMPLETED',0,'{}',0)")
c.commit();c.close()
PY
set +e
python3 - "$tmp/pre.json" <<'PY' | python3 "$H" --db "$db" >"$tmp/replay.json"
import json,sys
print(json.dumps({"mode":"post","project_id":"project-1","device_id":"display-1","baseline":json.load(open(sys.argv[1]))}))
PY
rc=$?
set -e
[[ "$rc" -eq 1 ]]
grep -q '"status":"FAIL"' "$tmp/replay.json"
set +e
printf '%s\n' '{"mode":"pre","project_id":"wrong","device_id":"display-1"}' | python3 "$H" --db "$db" >"$tmp/wrong.json"
rc=$?
set -e
[[ "$rc" -eq 3 ]]
echo "qualification Callboard reconnect evidence self-test PASS"
