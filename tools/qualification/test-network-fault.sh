#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
HELPER="$ROOT/tools/qualification/pi/stagecore-qualification-network-fault.py"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
db="$tmp/stagecore.sqlite3"
python3 - "$db" <<'PY'
import sqlite3,sys,time
now=int(time.time()*1000000)
c=sqlite3.connect(sys.argv[1])
c.executescript("""
CREATE TABLE stage_devices(device_id TEXT PRIMARY KEY,project_id TEXT,device_kind TEXT,enabled INTEGER);
CREATE TABLE stage_device_runtime_state(device_id TEXT PRIMARY KEY,connection_state TEXT,readiness TEXT,last_seen_at_us INTEGER);
CREATE TABLE network_observations(observation_id TEXT PRIMARY KEY,target_kind TEXT,target_id TEXT,observed_at_us INTEGER,reachability TEXT,transport_state TEXT,error_code TEXT);
""")
c.execute("INSERT INTO stage_devices VALUES ('tablet-01','project-1','TABLET_PLAYER',1)")
c.execute("INSERT INTO stage_device_runtime_state VALUES ('tablet-01','ONLINE','READY',?)",(now,))
c.execute("INSERT INTO network_observations VALUES ('base-1','STAGE_DEVICE','tablet-01',?,'REACHABLE','WEBSOCKET_CONNECTED','')",(now,))
c.commit();c.close()
PY
printf '%s\n' '{"mode":"pre","project_id":"project-1","device_id":"tablet-01"}' | python3 "$HELPER" --db "$db" >"$tmp/pre.json"
python3 - "$tmp/pre.json" <<'PY'
import json,sys
d=json.load(open(sys.argv[1]));assert d["status"]=="PASS" and d["last_observation_id"]=="base-1"
PY
python3 - "$db" "$tmp/pre.json" <<'PY'
import json,sqlite3,sys,time
b=json.load(open(sys.argv[2]));time.sleep(.02)
c=sqlite3.connect(sys.argv[1])
n=int(time.time()*1000000)
c.execute("INSERT INTO network_observations VALUES ('off-1','STAGE_DEVICE','tablet-01',?,'UNREACHABLE','WEBSOCKET_DISCONNECTED','')",(n-1000,))
c.execute("INSERT INTO network_observations VALUES ('on-1','STAGE_DEVICE','tablet-01',?,'REACHABLE','WEBSOCKET_CONNECTED','')",(n,))
c.execute("UPDATE stage_device_runtime_state SET last_seen_at_us=? WHERE device_id='tablet-01'",(n,))
c.commit();c.close()
PY
python3 - "$tmp/pre.json" <<'PY' | python3 "$HELPER" --db "$db" >"$tmp/post.json"
import json,sys
print(json.dumps({"mode":"post","project_id":"project-1","device_id":"tablet-01","baseline":json.load(open(sys.argv[1]))}))
PY
python3 - "$tmp/post.json" <<'PY'
import json,sys
d=json.load(open(sys.argv[1]))
assert d["status"]=="PASS" and d["disconnect"]["reason_code"]=="TARGET_UNREACHABLE"
assert d["reconnect"]["observation_id"]=="on-1" and d["operator_cockpit_verified"] is False
PY
python3 - "$db" <<'PY'
import sqlite3,sys
c=sqlite3.connect(sys.argv[1]);c.execute("DELETE FROM network_observations WHERE observation_id='off-1'");c.commit();c.close()
PY
set +e
python3 - "$tmp/pre.json" <<'PY' | python3 "$HELPER" --db "$db" >"$tmp/blocked.json"
import json,sys
print(json.dumps({"mode":"post","project_id":"project-1","device_id":"tablet-01","baseline":json.load(open(sys.argv[1]))}))
PY
rc=$?
set -e
[[ "$rc" -eq 3 ]]
grep -F '"status":"BLOCKED"' "$tmp/blocked.json" >/dev/null
set +e
printf '%s\n' '{"mode":"pre","project_id":"project-2","device_id":"tablet-01"}' | python3 "$HELPER" --db "$db" >"$tmp/wrong.json"
wrong_rc=$?
set -e
[[ "$wrong_rc" -eq 3 ]]
echo "qualification Network fault evidence self-test PASS"
