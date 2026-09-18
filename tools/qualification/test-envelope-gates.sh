#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HELPER="$ROOT/tools/qualification/pi/stagecore-qualification-envelope-gates.py"
VALIDATOR="$ROOT/tools/qualification/validate-envelope-gate-evidence.py"
tmp="$(mktemp -d)"
server_pid=""
trap '[[ -n "$server_pid" ]] && kill "$server_pid" 2>/dev/null || true; rm -rf "$tmp"' EXIT
sock="$tmp/qualification.sock"

python3 - "$sock" <<'PY' &
import json, os, socketserver, sys
from http.server import BaseHTTPRequestHandler

path=sys.argv[1]
try: os.unlink(path)
except FileNotFoundError: pass
seen={}
current={"warm":20.0}

class Server(socketserver.UnixStreamServer):
    allow_reuse_address=True

class H(BaseHTTPRequestHandler):
    def log_message(self,*args): pass
    def do_POST(self):
        n=int(self.headers.get("Content-Length","0"))
        body=json.loads(self.rfile.read(n))
        cmd=body["command"]
        cid=cmd["command_id"]
        ctype=cmd["command_type"]
        payload=cmd["payload"]
        if cid in seen:
            prior=seen[cid]
            value=dict(prior)
            value["statuses"]=[prior["status"]]
        elif ctype == "LIGHTING_CHANNELS_SET":
            deadline=cmd.get("deadline_at","")
            issued=cmd.get("issued_at","")
            if "exp-" in cid and deadline < issued:
                raise AssertionError("test helper produced invalid timestamp ordering")
            if "exp-" in cid and not cid.endswith("-pre"):
                value={"command_id":cid,"statuses":["TIMED_OUT"],"status":"TIMED_OUT","error":{"error_code":"DEVICE_COMMAND_EXPIRED","category":"TIMING","message":"expired","retryable":False}}
            else:
                current.update({k:float(v) for k,v in payload["channels"].items()})
                value={"command_id":cid,"statuses":["COMPLETED"],"status":"COMPLETED","payload":{"levels":dict(current)}}
        elif ctype == "LIGHTING_CHANNELS_FADE":
            current.update({k:float(v) for k,v in payload["channels"].items()})
            value={"command_id":cid,"statuses":["ACCEPTED","COMPLETED"],"status":"COMPLETED","payload":{"levels":dict(current)}}
        elif ctype == "LIGHTING_STATE_READ":
            value={"command_id":cid,"statuses":["COMPLETED"],"status":"COMPLETED","payload":{"current_levels":dict(current)}}
        else:
            raise AssertionError(ctype)
        seen[cid]=value
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

request_base='"device_id":"lighting-01","project_id":"project-1","channel_key":"warm","start_level":20,"target_level":80,"fade_ms":1500'
printf '{%s,"mode":"duplicate"}\n' "$request_base" |   STAGECORE_QUALIFICATION_SOCKET="$sock" python3 "$HELPER" >"$tmp/duplicate.json"
python3 "$VALIDATOR" --input "$tmp/duplicate.json" --mode duplicate >/dev/null

printf '{%s,"mode":"expired"}\n' "$request_base" |   STAGECORE_QUALIFICATION_SOCKET="$sock" python3 "$HELPER" >"$tmp/expired.json"
python3 "$VALIDATOR" --input "$tmp/expired.json" --mode expired >/dev/null

echo "qualification duplicate/expiry envelope self-test PASS"
