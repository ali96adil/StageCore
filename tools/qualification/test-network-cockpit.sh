#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
HELPER="$ROOT/tools/qualification/pi/stagecore-qualification-network-cockpit.py"
tmp="$(mktemp -d)"; server_pid=""
trap '[[ -n "$server_pid" ]] && kill "$server_pid" 2>/dev/null || true; rm -rf "$tmp"' EXIT
db="$tmp/stagecore.sqlite3"; port_file="$tmp/port"
python3 - "$db" "$port_file" <<'PY' &
import json,sqlite3,sys,time
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
db,port_file=sys.argv[1:];now=int(time.time()*1000000)
c=sqlite3.connect(db)
c.executescript("""
CREATE TABLE stage_devices(device_id TEXT PRIMARY KEY,project_id TEXT,enabled INTEGER);
CREATE TABLE live_video_sources(source_id TEXT PRIMARY KEY,project_id TEXT,desired_enabled INTEGER);
CREATE TABLE network_observations(observation_id TEXT PRIMARY KEY,target_kind TEXT,target_id TEXT,observed_at_us INTEGER,reachability TEXT,transport_state TEXT,latency_ms REAL,jitter_ms REAL,error_code TEXT);
""")
c.execute("INSERT INTO stage_devices VALUES (?,?,1)",("tablet-1","project-1"))
c.execute("INSERT INTO live_video_sources VALUES (?,?,1)",("source-1","project-1"))
for row in [("d","STAGE_DEVICE","tablet-1",now,"REACHABLE","WEBSOCKET_CONNECTED",None,None,""),("s","LIVE_SOURCE","source-1",now,"REACHABLE","STREAMING",None,None,""),("c","COMPANION","companion-1",now,"REACHABLE","WEBSOCKET_CONNECTED",None,None,"")]:
 c.execute("INSERT INTO network_observations VALUES (?,?,?,?,?,?,?,?,?)",row)
c.commit();c.close()
class H(BaseHTTPRequestHandler):
 def log_message(self,*a): pass
 def sendj(self,code,obj):
  raw=json.dumps(obj).encode();self.send_response(code);self.send_header("Content-Type","application/json");self.send_header("Content-Length",str(len(raw)));self.end_headers();self.wfile.write(raw)
 def do_POST(self):
  n=int(self.headers.get("Content-Length","0"));self.rfile.read(n)
  self.sendj(200,{"csrf_token":"csrf"} if self.path.endswith("/auth/login") else {})
 def do_GET(self):
  c=sqlite3.connect(db);rows=c.execute("SELECT target_kind,target_id,reachability,transport_state,latency_ms,jitter_ms,error_code FROM network_observations ORDER BY observed_at_us DESC,observation_id DESC").fetchall();c.close()
  seen=set();targets=[]
  for r in rows:
   k=(r[0],r[1])
   if k in seen: continue
   seen.add(k);targets.append({"target_kind":r[0],"target_id":r[1],"readiness":"READY","reason_code":"READY","observation":{"reachability":r[2],"transport_state":r[3],"latency_ms":r[4],"jitter_ms":r[5]}})
  self.sendj(200,{"targets":targets,"stale_after_ms":15000})
srv=ThreadingHTTPServer(("127.0.0.1",0),H);open(port_file,"w").write(str(srv.server_address[1]));srv.serve_forever()
PY
server_pid=$!; for _ in $(seq 1 50); do [[ -s "$port_file" ]] && break; sleep .05; done
base="http://127.0.0.1:$(cat "$port_file")"
printf '%s\n' '{"username":"owner","password":"secret","project_id":"project-1"}' | python3 "$HELPER" --db "$db" --hub-url "$base" >"$tmp/pass.json"
grep -F '"metric_status":"NOT_MEASURED_PRESERVED"' "$tmp/pass.json" >/dev/null
grep -F '"companion_ids":["companion-1"]' "$tmp/pass.json" >/dev/null
python3 - "$db" <<'PY'
import sqlite3,sys
c=sqlite3.connect(sys.argv[1]);c.execute("UPDATE network_observations SET latency_ms=12.5 WHERE observation_id='d'");c.commit();c.close()
PY
set +e
printf '%s\n' '{"username":"owner","password":"secret","project_id":"project-1"}' | python3 "$HELPER" --db "$db" --hub-url "$base" >"$tmp/numeric.json"
rc=$?
set -e
[[ "$rc" -eq 3 ]]
grep -F '"metric_status":"PROVENANCE_REQUIRED"' "$tmp/numeric.json" >/dev/null
echo "qualification Network Cockpit target/metric truth self-test PASS"
