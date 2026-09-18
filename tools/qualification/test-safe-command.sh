#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HELPER="$ROOT/tools/qualification/pi/stagecore-qualification-command.py"
tmp="$(mktemp -d)"
server_pid=""
trap '[[ -n "$server_pid" ]] && kill "$server_pid" 2>/dev/null || true; rm -rf "$tmp"' EXIT
db="$tmp/stagecore.sqlite3"
port_file="$tmp/port"

python3 - "$db" "$port_file" <<'PY' &
import json, sqlite3, sys, threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

db, port_file = sys.argv[1], sys.argv[2]
conn=sqlite3.connect(db)
conn.execute("""CREATE TABLE stage_device_commands (
 command_id TEXT PRIMARY KEY, command_type TEXT, status TEXT, result_json TEXT, completed_at_us INTEGER
)""")
conn.commit(); conn.close()

class H(BaseHTTPRequestHandler):
    counter=0
    def log_message(self, *args): pass
    def reply(self, code, value, cookie=False):
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
        H.counter += 1
        command_type = body["command_type"]
        command_id=f"cmd-{H.counter}"
        if "/tablet-controller/commands" in self.path:
            assert command_type in {"TABLET_PREPARE","TABLET_PLAY","TABLET_PAUSE","TABLET_STOP"}
            response={"results":[{"device_id":"tablet-01","command":{"envelope":{"command_id":command_id}}}]}
        else:
            assert command_type in {"LIGHTING_STATE_READ","LIGHTING_CONFIG_READ","LIGHTING_CHANNELS_SET","LIGHTING_CHANNELS_FADE","LIGHTING_BLACKOUT"}
            response={"envelope":{"command_id":command_id}}
        c=sqlite3.connect(db)
        c.execute("INSERT INTO stage_device_commands VALUES (?,?,?,?,?)",
          (command_id,command_type,"COMPLETED",json.dumps({"status":"COMPLETED","payload":{"ok":True},"session_token":"must-redact"}),123))
        c.commit(); c.close()
        return self.reply(200,response)

server=ThreadingHTTPServer(("127.0.0.1",0),H)
open(port_file,"w").write(str(server.server_address[1]))
server.serve_forever()
PY
server_pid=$!

for _ in $(seq 1 50); do [[ -s "$port_file" ]] && break; sleep 0.05; done
port="$(cat "$port_file")"
base="http://127.0.0.1:$port"

run_one() {
  local command="$1" device="$2" project="$3" payload="$4" out="$5"
  printf '{"username":"owner","password":"secret","project_id":"%s","device_id":"%s","command_type":"%s","payload":%s}\n'     "$project" "$device" "$command" "$payload" |     python3 "$HELPER" --hub-url "$base" --db "$db" --timeout-seconds 2 >"$out"
  python3 - "$out" "$command" <<'PY'
import json, sys
r=json.load(open(sys.argv[1], encoding="utf-8"))
assert r["status"] == "COMPLETED"
assert r["qualification_command"] == sys.argv[2]
PY
}

run_one TABLET_PREPARE tablet-01 project-1 '{"media_number":1}' "$tmp/tablet.json"
run_one LIGHTING_STATE_READ lighting-01 project-1 '{}' "$tmp/state.json"
run_one LIGHTING_CONFIG_READ lighting-01 project-1 '{}' "$tmp/config.json"
grep -F '"session_token": "[REDACTED]"' "$tmp/config.json" >/dev/null

printf '{"username":"owner","password":"secret","project_id":"project-1","device_id":"tablet-01","command_type":"TABLET_PLAY","payload":{"media_number":1}}\n' | \
  python3 "$HELPER" --allow-physical --hub-url "$base" --db "$db" --timeout-seconds 2 >"$tmp/play.json"
printf '{"username":"owner","password":"secret","project_id":"project-1","device_id":"lighting-01","command_type":"LIGHTING_CHANNELS_SET","payload":{"channels":{"warm":20}}}\n' | \
  python3 "$HELPER" --allow-physical --hub-url "$base" --db "$db" --timeout-seconds 2 >"$tmp/set.json"

set +e
printf '{"username":"owner","password":"secret","project_id":"project-1","device_id":"lighting-01","command_type":"LIGHTING_CHANNELS_SET","payload":{}}\n' |   python3 "$HELPER" --hub-url "$base" --db "$db" --timeout-seconds 1 >/dev/null
rc=$?
set -e
[[ "$rc" -ne 0 ]]

echo "qualification safe-command self-test PASS"
