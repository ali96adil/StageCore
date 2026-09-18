#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PUBLISHED="$ROOT/tools/qualification/pi/stagecore-qualification-published-lighting.py"
VALIDATOR_DIR="./tools/qualification/lighting-config-truth"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
db="$tmp/stagecore.sqlite3"
snapshot="22222222-2222-2222-2222-222222222222"
project="11111111-1111-1111-1111-111111111111"
device="lighting-01"

python3 - "$db" "$snapshot" "$project" "$device" "$tmp/hash" <<'PY'
import hashlib, json, sqlite3, sys
db,snapshot,project,device,hashfile=sys.argv[1:]
config={
 "schema_version":1,
 "channels":[
  {"channel_key":"warm","channel_number":1,"display_name":"Warm","kind":"WARM_WHITE","minimum_level":0,"maximum_level":80,"inverted":False,"enabled":True},
  {"channel_key":"cold","channel_number":2,"display_name":"Cold","kind":"COLD_WHITE","physical_zone":"front","minimum_level":0,"maximum_level":100,"inverted":False,"enabled":True},
 ]
}
canonical=json.dumps(config,separators=(",",":"),ensure_ascii=False).replace("<","\\u003c").replace(">","\\u003e").replace("&","\\u0026")
expected=hashlib.sha256(canonical.encode()).hexdigest()
open(hashfile,"w").write(expected)
manifest={
 "schema_version":5,"project_id":project,"revision_id":"33333333-3333-3333-3333-333333333333",
 "revision_number":1,"cues":[],
 "lighting_nodes":[{"device_id":device,"profile_id":"stagecore.esp32-dmx-lighting-node","configuration":config,"aliases":{"front_warm":"warm"}}]
}
conn=sqlite3.connect(db)
conn.execute("""CREATE TABLE runtime_snapshots (
 runtime_snapshot_id TEXT PRIMARY KEY, project_id TEXT, revision_id TEXT,
 snapshot_version INTEGER, content_hash TEXT, manifest_json TEXT, status TEXT
)""")
conn.execute("INSERT INTO runtime_snapshots VALUES (?,?,?,?,?,?,?)",
 (snapshot,project,manifest["revision_id"],1,"a"*64,json.dumps(manifest,separators=(",",":")),"PUBLISHED"))
conn.commit(); conn.close()
PY
hash="$(cat "$tmp/hash")"

printf '{"device_id":"%s","project_id":"%s","runtime_snapshot_id":"%s"}\n' "$device" "$project" "$snapshot" |   python3 "$PUBLISHED" --db "$db" >"$tmp/published.json"

python3 - "$tmp/probe.json" "$device" "$project" "$hash" <<'PY'
import json,sys,time
path,device,project,h=sys.argv[1:]
data={"schema_version":1,"devices":[{"device_id":device,"project_id":project,
"profile_id":"stagecore.esp32-dmx-lighting-node","runtime":{"connection_state":"ONLINE","readiness":"READY",
"last_seen_at_us":int(time.time()*1_000_000),"observed":{"schema_version":1,"firmware_version":"test",
"current_levels":{"warm":0,"cold":0},"dmx_healthy":True,"configuration_hash":h,
"brownout_warning":False,"authority":"STAGECORE"}}}]}
json.dump(data,open(path,"w"))
PY

python3 - "$tmp/state.json" "$device" "$hash" <<'PY'
import json,sys
path,device,h=sys.argv[1:]
payload={"schema_version":1,"firmware_version":"test","current_levels":{"warm":0,"cold":0},
"dmx_healthy":True,"configuration_hash":h,"brownout_warning":False,"authority":"STAGECORE"}
json.dump({"status":"COMPLETED","device_id":device,"qualification_command":"LIGHTING_STATE_READ",
"result":{"status":"COMPLETED","payload":payload}},open(path,"w"))
PY

python3 - "$tmp/config.json" "$tmp/published.json" "$device" <<'PY'
import json,sys
path,published,device=sys.argv[1:]
p=json.load(open(published))
json.dump({"status":"COMPLETED","device_id":device,"qualification_command":"LIGHTING_CONFIG_READ",
"result":{"status":"COMPLETED","payload":p["configuration"]}},open(path,"w"))
PY

go run "$VALIDATOR_DIR"   --published "$tmp/published.json" --probe "$tmp/probe.json"   --state-command "$tmp/state.json" --config-command "$tmp/config.json"   --device-id "$device" --project-id "$project" --runtime-snapshot-id "$snapshot"   >"$tmp/truth.json"
python3 - "$tmp/truth.json" "$hash" <<'PY'
import json,sys
data=json.load(open(sys.argv[1]))
assert data["status"]=="PASS"
assert data["expected_configuration_hash"]==sys.argv[2]
assert data["config_read_matches_published"] is True
PY

python3 - "$tmp/probe.json" "$tmp/bad-probe.json" <<'PY'
import json,sys
data=json.load(open(sys.argv[1]))
data["devices"][0]["runtime"]["observed"]["configuration_hash"]="0"*64
json.dump(data,open(sys.argv[2],"w"))
PY
set +e
go run "$VALIDATOR_DIR"   --published "$tmp/published.json" --probe "$tmp/bad-probe.json"   --state-command "$tmp/state.json" --config-command "$tmp/config.json"   --device-id "$device" --project-id "$project" --runtime-snapshot-id "$snapshot"   >/dev/null 2>&1
bad_hash_rc=$?
set -e
[[ "$bad_hash_rc" -ne 0 ]]

python3 - "$tmp/config.json" "$tmp/bad-config.json" <<'PY'
import json,sys
data=json.load(open(sys.argv[1]))
data["result"]["payload"]["channels"][0]["maximum_level"]=79
json.dump(data,open(sys.argv[2],"w"))
PY
set +e
go run "$VALIDATOR_DIR"   --published "$tmp/published.json" --probe "$tmp/probe.json"   --state-command "$tmp/state.json" --config-command "$tmp/bad-config.json"   --device-id "$device" --project-id "$project" --runtime-snapshot-id "$snapshot"   >/dev/null 2>&1
bad_config_rc=$?
set -e
[[ "$bad_config_rc" -ne 0 ]]

set +e
printf '{"device_id":"%s","project_id":"%s","runtime_snapshot_id":"44444444-4444-4444-4444-444444444444"}\n' "$device" "$project" |   python3 "$PUBLISHED" --db "$db" >/dev/null 2>&1
missing_snapshot_rc=$?
set -e
[[ "$missing_snapshot_rc" -eq 3 ]]

echo "qualification lighting-config truth self-test PASS"
