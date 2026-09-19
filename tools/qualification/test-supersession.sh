#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HELPER="$ROOT/tools/qualification/pi/stagecore-qualification-supersession.py"
VALIDATOR="$ROOT/tools/qualification/validate-supersession-evidence.py"
tmp="$(mktemp -d)"
server_pid=""
trap '[[ -n "$server_pid" ]] && kill "$server_pid" 2>/dev/null || true; rm -rf "$tmp"' EXIT
db="$tmp/stagecore.sqlite3"
port_file="$tmp/port"

python3 - "$db" "$port_file" <<'PY' &
import json, sqlite3, sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

db, port_file = sys.argv[1], sys.argv[2]
conn=sqlite3.connect(db)
conn.execute("""CREATE TABLE stage_device_commands (
 command_id TEXT PRIMARY KEY, device_id TEXT NOT NULL, command_type TEXT NOT NULL,
 status TEXT NOT NULL, issued_at_us INTEGER, completed_at_us INTEGER
)""")
conn.execute("""CREATE TABLE stage_device_runtime_state (
 device_id TEXT PRIMARY KEY, observed_state_json TEXT
)""")
conn.execute("INSERT INTO stage_device_runtime_state VALUES (?,?)", ("lighting-01", json.dumps({
  "current_levels":{"warm":0},"active_fade":None,
  "last_accepted_command_id":"","last_applied_command_id":""
})))
conn.commit(); conn.close()

class H(BaseHTTPRequestHandler):
    counter=0
    fade_id=None
    def log_message(self,*args): pass
    def reply(self,code,value,cookie=False):
        raw=json.dumps(value).encode()
        self.send_response(code)
        self.send_header("Content-Type","application/json")
        if cookie: self.send_header("Set-Cookie","stagecore_session=test; Path=/; HttpOnly")
        self.send_header("Content-Length",str(len(raw)))
        self.end_headers(); self.wfile.write(raw)
    def do_POST(self):
        length=int(self.headers.get("Content-Length","0"))
        body=json.loads(self.rfile.read(length) or b"{}")
        if self.path == "/api/v1/auth/login":
            assert body == {"username":"owner","password":"secret"}
            return self.reply(200,{"csrf_token":"csrf-test"},True)
        if self.path == "/api/v1/auth/logout":
            assert self.headers.get("X-StageCore-CSRF") == "csrf-test"
            return self.reply(204,{})
        assert self.headers.get("X-StageCore-CSRF") == "csrf-test"
        assert self.path == "/api/v1/stage-devices/lighting-01/commands"
        H.counter += 1
        cid=f"cmd-{H.counter}"
        c=sqlite3.connect(db)
        c.execute("PRAGMA busy_timeout=2000")
        ctype=body["command_type"]
        if H.counter == 1:
            assert ctype == "LIGHTING_CHANNELS_SET"
            c.execute("INSERT INTO stage_device_commands VALUES (?,?,?,?,?,?)",
              (cid,"lighting-01",ctype,"COMPLETED",1_000_000,1_050_000))
            obs={"current_levels":{"warm":body["payload"]["channels"]["warm"]},"active_fade":None,
                 "last_accepted_command_id":cid,"last_applied_command_id":cid}
        elif H.counter == 2:
            assert ctype == "LIGHTING_CHANNELS_FADE"
            H.fade_id=cid
            c.execute("INSERT INTO stage_device_commands VALUES (?,?,?,?,?,?)",
              (cid,"lighting-01",ctype,"ACCEPTED",2_000_000,None))
            obs={"current_levels":{"warm":20},"active_fade":{"command_id":cid,"targets":{"warm":80}},
                 "last_accepted_command_id":cid,"last_applied_command_id":"cmd-1"}
        elif H.counter == 3:
            assert ctype == "LIGHTING_CHANNELS_SET"
            c.execute("UPDATE stage_device_commands SET status='CANCELLED', completed_at_us=? WHERE command_id=?",
              (2_500_000,H.fade_id))
            c.execute("INSERT INTO stage_device_commands VALUES (?,?,?,?,?,?)",
              (cid,"lighting-01",ctype,"COMPLETED",2_500_100,2_550_000))
            obs={"current_levels":{"warm":35},"active_fade":None,
                 "last_accepted_command_id":cid,"last_applied_command_id":cid}
        else:
            raise AssertionError("unexpected command count")
        c.execute("UPDATE stage_device_runtime_state SET observed_state_json=? WHERE device_id='lighting-01'",
          (json.dumps(obs),))
        c.commit(); c.close()
        return self.reply(200,{"envelope":{"command_id":cid}})

server=ThreadingHTTPServer(("127.0.0.1",0),H)
open(port_file,"w").write(str(server.server_address[1]))
server.serve_forever()
PY
server_pid=$!
for _ in $(seq 1 50); do [[ -s "$port_file" ]] && break; sleep 0.05; done
port="$(cat "$port_file")"

printf '%s\n' '{"username":"owner","password":"secret","device_id":"lighting-01","channel_key":"warm","start_level":20,"target_level":80,"replacement_level":35,"fade_ms":5000,"activation_timeout_ms":2000}' | \
  python3 "$HELPER" --hub-url "http://127.0.0.1:$port" --db "$db" >"$tmp/evidence.json"

python3 "$VALIDATOR" --input "$tmp/evidence.json" --device-id lighting-01 >"$tmp/validation.json"
python3 - "$tmp/evidence.json" <<'PY'
import json, sys
e=json.load(open(sys.argv[1], encoding="utf-8"))
assert e["status"] == "PASS"
assert e["active_fade_seen"] is True
assert e["superseded"]["status"] == "CANCELLED"
assert e["replacement"]["status"] == "COMPLETED"
assert e["final_observation"]["active_fade_command_id"] is None
assert e["final_observation"]["last_applied_command_id"] == e["replacement"]["command_id"]
PY

echo "qualification supersession self-test PASS"
