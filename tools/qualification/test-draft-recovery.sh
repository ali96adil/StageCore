#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
EVIDENCE="$ROOT/tools/qualification/pi/stagecore-qualification-draft-evidence.py"
HTTP="$ROOT/tools/qualification/pi/stagecore-qualification-draft-http.py"
tmp="$(mktemp -d)"
server_pid=""
trap '[[ -n "$server_pid" ]] && kill "$server_pid" 2>/dev/null || true; rm -rf "$tmp"' EXIT
db="$tmp/stagecore.sqlite3"

python3 - "$db" <<'PY'
import json,sqlite3,sys,time
db=sys.argv[1]; now=int(time.time()*1_000_000)
c=sqlite3.connect(db)
c.executescript("""
CREATE TABLE projects(project_id TEXT PRIMARY KEY,current_revision_id TEXT,lifecycle_state TEXT);
CREATE TABLE project_revisions(revision_id TEXT PRIMARY KEY,project_id TEXT,revision_number INTEGER,status TEXT,parent_revision_id TEXT,created_at_us INTEGER,created_by TEXT,change_note TEXT);
CREATE TABLE runtime_snapshots(runtime_snapshot_id TEXT PRIMARY KEY,project_id TEXT,revision_id TEXT,snapshot_version INTEGER,created_at_us INTEGER,created_by TEXT,content_hash TEXT,manifest_json TEXT,status TEXT);
CREATE TABLE sessions(session_id TEXT PRIMARY KEY,project_id TEXT,runtime_snapshot_id TEXT,session_type TEXT,status TEXT,started_at_us INTEGER);
CREATE TABLE security_audit_records(audit_id TEXT,event_type TEXT,occurred_at_us INTEGER,actor_username TEXT,resource_id TEXT,result TEXT,reason TEXT,metadata_json TEXT);
""")
p="project-1"; parent="rev-parent"; draft="rev-draft"; snap="snap-1"
c.execute("INSERT INTO projects VALUES (?,?,?)",(p,draft,"ACTIVE"))
c.execute("INSERT INTO project_revisions VALUES (?,?,?,?,?,?,?,?)",(parent,p,1,"VALIDATED",None,now-3_000_000,"owner",""))
c.execute("INSERT INTO project_revisions VALUES (?,?,?,?,?,?,?,?)",(draft,p,2,"DRAFT",parent,now-2_000_000,"owner",""))
c.execute("INSERT INTO runtime_snapshots VALUES (?,?,?,?,?,?,?,?,?)",(snap,p,parent,1,now-2_500_000,"owner","h"*64,'{"schema_version":5}',"PUBLISHED"))
c.commit(); c.close()
PY

printf '%s\n' '{"mode":"baseline","project_id":"project-1"}' | python3 "$EVIDENCE" --db "$db" >"$tmp/baseline.json"
grep -F '"status":"PASS"' "$tmp/baseline.json" >/dev/null

python3 - "$db" <<'PY'
import sqlite3,sys,time
c=sqlite3.connect(sys.argv[1]); now=int(time.time()*1_000_000)
c.execute("INSERT INTO sessions VALUES (?,?,?,?,?,?)",("show-1","project-1","snap-1","SHOW","ACTIVE",now))
c.commit(); c.close()
PY
python3 - "$tmp/baseline.json" <<'PY' | python3 "$EVIDENCE" --db "$db" >"$tmp/show.json"
import json,sys
print(json.dumps({"mode":"show-active","project_id":"project-1","baseline":json.load(open(sys.argv[1]))}))
PY
grep -F '"session_id":"show-1"' "$tmp/show.json" >/dev/null

python3 - "$db" "$tmp/baseline.json" <<'PY'
import json,sqlite3,sys,time
base=json.load(open(sys.argv[2])); c=sqlite3.connect(sys.argv[1]); now=int(time.time()*1_000_000)
c.execute("UPDATE sessions SET status='COMPLETED' WHERE session_id='show-1'")
c.execute("UPDATE projects SET current_revision_id=? WHERE project_id='project-1'",(base["parent"]["revision_id"],))
c.execute("UPDATE project_revisions SET status='SUPERSEDED' WHERE revision_id=?",(base["draft"]["revision_id"],))
c.execute("INSERT INTO security_audit_records VALUES (?,?,?,?,?,?,?,?)",("audit-1","project.draft.discard",now,"owner","project-1","SUCCESS","qualified",json.dumps({"discarded":True,"restored_revision_id":base["parent"]["revision_id"]})))
c.commit(); c.close()
PY
python3 - "$tmp/baseline.json" <<'PY' | python3 "$EVIDENCE" --db "$db" >"$tmp/post.json"
import json,sys
print(json.dumps({"mode":"post","project_id":"project-1","baseline":json.load(open(sys.argv[1]))}))
PY
grep -F '"draft_status":"SUPERSEDED"' "$tmp/post.json" >/dev/null
grep -F '"audit_id":"audit-1"' "$tmp/post.json" >/dev/null

python3 - "$tmp/port" <<'PY' &
import json,sys
from http.server import BaseHTTPRequestHandler,HTTPServer
class H(BaseHTTPRequestHandler):
    def log_message(self,*a): pass
    def do_POST(self):
        n=int(self.headers.get("Content-Length","0")); self.rfile.read(n)
        body={"csrf_token":"csrf-test"}
        raw=json.dumps(body).encode(); self.send_response(200); self.send_header("Content-Type","application/json"); self.send_header("Content-Length",str(len(raw))); self.end_headers(); self.wfile.write(raw)
    def do_DELETE(self):
        n=int(self.headers.get("Content-Length","0")); self.rfile.read(n)
        mode=self.headers.get("X-Test-Mode","")
        # mode is selected by server port below via path query not headers; alternate by project.
        code=403 if "owner-only" in self.path else 423
        err="OWNER_REQUIRED" if code==403 else "SHOW_CONFIGURATION_LOCKED"
        raw=json.dumps({"error_code":err}).encode(); self.send_response(code); self.send_header("Content-Type","application/json"); self.send_header("Content-Length",str(len(raw))); self.end_headers(); self.wfile.write(raw)
srv=HTTPServer(("127.0.0.1",0),H)
open(sys.argv[1],"w").write(str(srv.server_port))
srv.serve_forever()
PY
server_pid=$!
for _ in $(seq 1 50); do [[ -s "$tmp/port" ]] && break; sleep .05; done
port="$(cat "$tmp/port")"

# Separate tiny servers are easier to make deterministic for the exact expected status.
kill "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true; server_pid=""
python3 - "$tmp/port" <<'PY' &
import json,sys
from http.server import BaseHTTPRequestHandler,HTTPServer
class H(BaseHTTPRequestHandler):
    def log_message(self,*a): pass
    def _send(self,code,obj):
        raw=json.dumps(obj).encode(); self.send_response(code); self.send_header("Content-Type","application/json"); self.send_header("Content-Length",str(len(raw))); self.end_headers(); self.wfile.write(raw)
    def do_POST(self):
        n=int(self.headers.get("Content-Length","0")); self.rfile.read(n)
        self._send(200,{"csrf_token":"csrf-test"})
    def do_DELETE(self):
        n=int(self.headers.get("Content-Length","0")); self.rfile.read(n)
        self._send(403,{"error_code":"OWNER_REQUIRED"})
srv=HTTPServer(("127.0.0.1",0),H); open(sys.argv[1],"w").write(str(srv.server_port)); srv.serve_forever()
PY
server_pid=$!; for _ in $(seq 1 50); do [[ -s "$tmp/port" ]] && break; sleep .05; done; port="$(cat "$tmp/port")"
printf '%s\n' '{"username":"tech","password":"secret","project_id":"project-1"}' | python3 "$HTTP" --mode owner-only --hub-url "http://127.0.0.1:$port" >"$tmp/http1.json"
grep -F '"error_code":"OWNER_REQUIRED"' "$tmp/http1.json" >/dev/null
kill "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true; server_pid=""; rm -f "$tmp/port"

python3 - "$tmp/port" <<'PY' &
import json,sys
from http.server import BaseHTTPRequestHandler,HTTPServer
class H(BaseHTTPRequestHandler):
    def log_message(self,*a): pass
    def _send(self,code,obj):
        raw=json.dumps(obj).encode(); self.send_response(code); self.send_header("Content-Type","application/json"); self.send_header("Content-Length",str(len(raw))); self.end_headers(); self.wfile.write(raw)
    def do_POST(self):
        n=int(self.headers.get("Content-Length","0")); self.rfile.read(n); self._send(200,{"csrf_token":"csrf-test"})
    def do_DELETE(self):
        n=int(self.headers.get("Content-Length","0")); self.rfile.read(n); self._send(423,{"error_code":"SHOW_CONFIGURATION_LOCKED"})
srv=HTTPServer(("127.0.0.1",0),H); open(sys.argv[1],"w").write(str(srv.server_port)); srv.serve_forever()
PY
server_pid=$!; for _ in $(seq 1 50); do [[ -s "$tmp/port" ]] && break; sleep .05; done; port="$(cat "$tmp/port")"
printf '%s\n' '{"username":"owner","password":"secret","project_id":"project-1"}' | python3 "$HTTP" --mode show-lock --hub-url "http://127.0.0.1:$port" >"$tmp/http2.json"
grep -F '"error_code":"SHOW_CONFIGURATION_LOCKED"' "$tmp/http2.json" >/dev/null

echo "qualification Draft recovery evidence self-test PASS"
