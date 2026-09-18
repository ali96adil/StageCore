#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HELPER="$ROOT/tools/qualification/pi/stagecore-qualification-dmx-stability.py"
VALIDATOR="$ROOT/tools/qualification/validate-dmx-stability-evidence.py"
tmp="$(mktemp -d)"
server_pid=""
trap '[[ -n "$server_pid" ]] && kill "$server_pid" 2>/dev/null || true; rm -rf "$tmp"' EXIT
sock="$tmp/qualification.sock"
db="$tmp/stagecore.sqlite3"

python3 - "$db" "$sock" <<'PY' &
import json, os, socketserver, sqlite3, sys
from http.server import BaseHTTPRequestHandler

db, path = sys.argv[1], sys.argv[2]
try: os.unlink(path)
except FileNotFoundError: pass
conn=sqlite3.connect(db)
conn.execute("CREATE TABLE stage_devices (device_id TEXT PRIMARY KEY, project_id TEXT, client_version TEXT)")
conn.execute("""CREATE TABLE stage_device_runtime_state (
 device_id TEXT PRIMARY KEY, connection_state TEXT, readiness TEXT,
 last_seen_at_us INTEGER, observed_state_json TEXT
)""")
observed={
 "uptime_seconds":500,"reset_reason":"POWERON","current_levels":{"warm":35,"cold":0},
 "active_fade":None,"dmx_healthy":True,"configuration_hash":"cfg-1",
 "brownout_warning":False,"authority":"STAGECORE"
}
conn.execute("INSERT INTO stage_devices VALUES (?,?,?)",("lighting-01","project-1","0.2.0-dev"))
conn.execute("INSERT INTO stage_device_runtime_state VALUES (?,?,?,?,?)",
 ("lighting-01","ONLINE","READY",1789734000000000,json.dumps(observed)))
conn.commit(); conn.close()

config={"schema_version":1,"channels":[{"channel_key":"warm","channel_number":1,"display_name":"Warm","kind":"WARM_WHITE","minimum_level":0,"maximum_level":100,"inverted":False,"enabled":True}]}

class Server(socketserver.UnixStreamServer):
    allow_reuse_address=True

class H(BaseHTTPRequestHandler):
    def log_message(self,*args): pass
    def do_POST(self):
        n=int(self.headers.get("Content-Length","0"))
        body=json.loads(self.rfile.read(n))
        cmd=body["command"]
        ctype=cmd["command_type"]
        cid=cmd["command_id"]
        if ctype == "LIGHTING_STATE_READ":
            payload={"current_levels":{"warm":35,"cold":0},"dmx_healthy":True,"configuration_hash":"cfg-1","authority":"STAGECORE"}
        elif ctype == "LIGHTING_CONFIG_READ":
            payload=config
        else:
            raise AssertionError(ctype)
        value={"command_id":cid,"statuses":["COMPLETED"],"status":"COMPLETED","payload":payload}
        raw=json.dumps(value).encode()
        self.send_response(200)
        self.send_header("Content-Type","application/json")
        self.send_header("Content-Length",str(len(raw)))
        self.end_headers(); self.wfile.write(raw)

server=Server(path,H)
server.serve_forever()
PY
server_pid=$!
for _ in $(seq 1 50); do [[ -S "$sock" ]] && break; sleep 0.05; done

printf '%s\n' '{"device_id":"lighting-01","project_id":"project-1","channel_key":"warm","expected_level":35,"duration_seconds":1,"interval_ms":150}' |   STAGECORE_QUALIFICATION_SOCKET="$sock" STAGECORE_DB="$db" python3 "$HELPER" --db "$db" >"$tmp/pass.json"

python3 "$VALIDATOR" --input "$tmp/pass.json" --device-id lighting-01 --expected-level 35 --min-duration-seconds 1 >/dev/null

python3 - "$tmp/pass.json" "$tmp/bad.json" <<'PY'
import json, sys
data=json.load(open(sys.argv[1],encoding="utf-8"))
data["after"]["levels"]["warm"]=22
json.dump(data,open(sys.argv[2],"w",encoding="utf-8"))
PY
set +e
python3 "$VALIDATOR" --input "$tmp/bad.json" --device-id lighting-01 --expected-level 35 --min-duration-seconds 1 >/dev/null 2>&1
bad_rc=$?
set -e
[[ "$bad_rc" -ne 0 ]]

echo "qualification DMX-stability self-test PASS"
