#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HELPER="$ROOT/tools/qualification/pi/stagecore-qualification-envelope-gates.py"
VALIDATOR="$ROOT/tools/qualification/validate-envelope-gate-evidence.py"
INSTALLER="$ROOT/tools/qualification/pi/install-stagecore-qualification-helper.sh"
ROOT_HELPER="$ROOT/tools/qualification/pi/stagecore-qualification-helper"
DMX_STABILITY="$ROOT/tools/qualification/pi/stagecore-qualification-dmx-stability.py"
tmp="$(mktemp -d)"
server_pid=""
trap '[[ -n "$server_pid" ]] && kill "$server_pid" 2>/dev/null || true; rm -rf "$tmp"' EXIT
sock="$tmp/qualification.sock"

runtime_socket="/run/stagecore-qualification/qualification-envelope.sock"
grep -F "Environment=STAGECORE_QUALIFICATION_SOCKET=$runtime_socket" "$INSTALLER" >/dev/null
grep -F 'RuntimeDirectory=stagecore-qualification' "$INSTALLER" >/dev/null
grep -F 'RuntimeDirectoryMode=0700' "$INSTALLER" >/dev/null
grep -F "$runtime_socket" "$ROOT_HELPER" >/dev/null
grep -F "$runtime_socket" "$HELPER" >/dev/null
grep -F "$runtime_socket" "$DMX_STABILITY" >/dev/null
if grep -F '/var/lib/stagecore/qualification-envelope.sock' \
  "$INSTALLER" "$ROOT_HELPER" "$HELPER" "$DMX_STABILITY" >/dev/null; then
  echo "qualification socket must not be placed under the read-only /var/lib/stagecore root" >&2
  exit 1
fi

python3 - "$sock" <<'PY' &
import json, os, socketserver, sys
from http.server import BaseHTTPRequestHandler

path=sys.argv[1]
try: os.unlink(path)
except FileNotFoundError: pass
seen={}
current={"warm":20.0}
config={
    "schema_version":1,
    "channels":[{
        "channel_key":"warm","channel_number":1,"display_name":"Warm",
        "kind":"WARM_WHITE","minimum_level":10,"maximum_level":70,
        "inverted":False,"enabled":True
    }]
}

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
        elif ctype == "TABLET_PREPARE":
            if cmd.get("project_id") != "project-1":
                value={"command_id":cid,"statuses":["REJECTED"],"status":"REJECTED","error":{"error_code":"PROJECT_MISMATCH","category":"SCOPE","message":"project mismatch","retryable":False}}
            elif cmd.get("runtime_snapshot_id") != "snapshot-1":
                value={"command_id":cid,"statuses":["REJECTED"],"status":"REJECTED","error":{"error_code":"SNAPSHOT_MISMATCH","category":"SCOPE","message":"snapshot mismatch","retryable":False}}
            else:
                value={"command_id":cid,"statuses":["COMPLETED"],"status":"COMPLETED","payload":{"prepared":True}}
        elif ctype == "LIGHTING_CONFIG_READ":
            value={"command_id":cid,"statuses":["COMPLETED"],"status":"COMPLETED","payload":config}
        elif ctype == "LIGHTING_CHANNELS_SET":
            deadline=cmd.get("deadline_at","")
            issued=cmd.get("issued_at","")
            if "exp-" in cid and deadline < issued:
                raise AssertionError("test helper produced invalid timestamp ordering")
            if "exp-" in cid and not cid.endswith("-pre"):
                value={"command_id":cid,"statuses":["TIMED_OUT"],"status":"TIMED_OUT","error":{"error_code":"DEVICE_COMMAND_EXPIRED","category":"TIMING","message":"expired","retryable":False}}
            else:
                channels=payload.get("channels") or {}
                if any((not isinstance(v,(int,float)) or isinstance(v,bool) or v < 0 or v > 100) for v in channels.values()):
                    value={"command_id":cid,"statuses":["REJECTED"],"status":"REJECTED","error":{"error_code":"DEVICE_COMMAND_INVALID","category":"VALIDATION","message":"invalid","retryable":False}}
                elif any(k != "warm" for k in channels):
                    value={"command_id":cid,"statuses":["REJECTED"],"status":"REJECTED","error":{"error_code":"CHANNEL_LEVEL_INVALID","category":"VALIDATION","message":"unknown","retryable":False}}
                else:
                    normalized={k:max(10.0,min(70.0,float(v))) for k,v in channels.items()}
                    current.update(normalized)
                    value={"command_id":cid,"statuses":["COMPLETED"],"status":"COMPLETED","payload":{"levels":dict(normalized)}}
        elif ctype == "LIGHTING_CHANNELS_FADE":
            normalized={k:max(10.0,min(70.0,float(v))) for k,v in payload["channels"].items()}
            current.update(normalized)
            value={"command_id":cid,"statuses":["ACCEPTED","COMPLETED"],"status":"COMPLETED","payload":{"levels":dict(normalized)}}
        elif ctype == "LIGHTING_STATE_READ":
            value={"command_id":cid,"statuses":["COMPLETED"],"status":"COMPLETED","payload":{"current_levels":dict(current),"configuration_hash":"fake-config-hash"}}
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

printf '{%s,"mode":"invalid-value"}\n' "$request_base" |   STAGECORE_QUALIFICATION_SOCKET="$sock" python3 "$HELPER" >"$tmp/invalid-value.json"
python3 "$VALIDATOR" --input "$tmp/invalid-value.json" --mode invalid-value >/dev/null
python3 - "$tmp/invalid-value.json" <<'PY'
import json, sys
e=json.load(open(sys.argv[1], encoding="utf-8"))
assert e["invalid_value_error_code"] == "DEVICE_COMMAND_INVALID"
assert e["unknown_channel_error_code"] == "CHANNEL_LEVEL_INVALID"
assert e["clamp"]["performed"] is True
assert abs(e["clamp"]["expected_level"] - 70.0) <= 0.01
assert abs(e["restored_level"] - 20.0) <= 0.01
assert e["configuration_hash_before"] == e["configuration_hash_after"] == "fake-config-hash"
PY

printf '%s\n' '{"mode":"tablet-scope","device_id":"tablet-01","project_id":"project-1","runtime_snapshot_id":"snapshot-1","tablet_manifest_id":"manifest-1","media_number":1}' | \
  STAGECORE_QUALIFICATION_SOCKET="$sock" python3 "$HELPER" >"$tmp/tablet-scope.json"
grep -F '"project_mismatch_error_code": "PROJECT_MISMATCH"' "$tmp/tablet-scope.json" >/dev/null
grep -F '"snapshot_mismatch_error_code": "SNAPSHOT_MISMATCH"' "$tmp/tablet-scope.json" >/dev/null

echo "qualification duplicate/expiry/invalid-value/tablet-scope envelope self-test PASS"
