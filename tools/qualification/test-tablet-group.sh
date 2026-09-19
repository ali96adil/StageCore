#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
EVIDENCE="$ROOT/tools/qualification/pi/stagecore-qualification-tablet-evidence.py"
GROUP="$ROOT/tools/qualification/pi/stagecore-qualification-tablet-group.py"
tmp="$(mktemp -d)"
server_pid=""
trap '[[ -n "$server_pid" ]] && kill "$server_pid" 2>/dev/null || true; rm -rf "$tmp"' EXIT
db="$tmp/stagecore.sqlite3"

python3 - "$db" <<'PY'
import json,sqlite3,sys,time
db=sys.argv[1]; now=int(time.time()*1_000_000); c=sqlite3.connect(db)
c.executescript("""
CREATE TABLE stage_devices(device_id TEXT PRIMARY KEY,project_id TEXT,profile_id TEXT,device_kind TEXT,client_version TEXT,group_name TEXT,capabilities_json TEXT,enabled INTEGER);
CREATE TABLE stage_device_runtime_state(device_id TEXT PRIMARY KEY,connection_state TEXT,readiness TEXT,last_seen_at_us INTEGER,observed_state_json TEXT);
CREATE TABLE stage_device_commands(command_id TEXT PRIMARY KEY,status TEXT,result_json TEXT);
""")
for d in ("tablet-01","tablet-02"):
 c.execute("INSERT INTO stage_devices VALUES (?,?,?,?,?,?,?,1)",(d,"project-1","stagecore.tablet-player","TABLET_PLAYER","rc3","actors",json.dumps(["tablet.media.play"])))
 c.execute("INSERT INTO stage_device_runtime_state VALUES (?,?,?,?,?)",(d,"ONLINE","READY",now,json.dumps({"project_id":"project-1","runtime_snapshot_id":"snapshot-1"})))
c.commit(); c.close()
PY
printf '%s\n' '{"mode":"group-availability","project_id":"project-1","device_id":"ignored","runtime_snapshot_id":"snapshot-1","group_name":"actors"}' | python3 "$EVIDENCE" --db "$db" >"$tmp/avail.json"
grep -F '"selection_state":"ELIGIBLE"' "$tmp/avail.json" >/dev/null

python3 - "$tmp/port" "$db" <<'PY' &
import json,sqlite3,sys
from http.server import BaseHTTPRequestHandler,HTTPServer
db=sys.argv[2]
class H(BaseHTTPRequestHandler):
 def log_message(self,*a): pass
 def sendj(self,code,obj):
  raw=json.dumps(obj).encode(); self.send_response(code); self.send_header("Content-Type","application/json"); self.send_header("Content-Length",str(len(raw))); self.end_headers(); self.wfile.write(raw)
 def do_POST(self):
  n=int(self.headers.get("Content-Length","0")); body=json.loads(self.rfile.read(n) or b"{}")
  if self.path.endswith("/auth/login"): self.sendj(200,{"csrf_token":"csrf"}); return
  if self.path.endswith("/auth/logout"): self.sendj(200,{}); return
  assert body.get("group_name")=="actors"
  assert not body.get("all") and not body.get("device_ids")
  assert body.get("command_type")=="TABLET_PLAY"
  results=[]
  c=sqlite3.connect(db)
  for i,d in enumerate(("tablet-01","tablet-02"),1):
   cid=f"cmd-{i}"; c.execute("INSERT OR REPLACE INTO stage_device_commands VALUES (?,?,?)",(cid,"COMPLETED",'{"status":"COMPLETED"}'))
   results.append({"device_id":d,"command":{"envelope":{"command_id":cid}}})
  c.commit(); c.close()
  self.sendj(200,{"correlation_id":"corr-1","results":results})
srv=HTTPServer(("127.0.0.1",0),H); open(sys.argv[1],"w").write(str(srv.server_port)); srv.serve_forever()
PY
server_pid=$!; for _ in $(seq 1 50); do [[ -s "$tmp/port" ]] && break; sleep .05; done; port="$(cat "$tmp/port")"
printf '%s\n' '{"username":"owner","password":"secret","project_id":"project-1","group_name":"actors","expected_device_ids":["tablet-01","tablet-02"],"media_number":1}' | python3 "$GROUP" --hub-url "http://127.0.0.1:$port" --db "$db" >"$tmp/group.json"
grep -F '"status": "PASS"' "$tmp/group.json" >/dev/null
grep -F '"tablet-01"' "$tmp/group.json" >/dev/null
grep -F '"tablet-02"' "$tmp/group.json" >/dev/null

echo "qualification Tablet group self-test PASS"
