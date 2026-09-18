#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
db="$tmp/stagecore.sqlite3"
now_us="$(python3 - <<'PY'
import time
print(int(time.time() * 1_000_000))
PY
)"

python3 - "$db" "$now_us" <<'PY'
import json, sqlite3, sys
db, now_us = sys.argv[1], int(sys.argv[2])
conn = sqlite3.connect(db)
conn.executescript("""
CREATE TABLE stage_devices (
  device_id TEXT PRIMARY KEY, project_id TEXT, profile_id TEXT, device_kind TEXT,
  display_name TEXT, platform TEXT, architecture TEXT, client_version TEXT,
  protocol_version TEXT, capabilities_json TEXT, enabled INTEGER
);
CREATE TABLE stage_device_runtime_state (
  device_id TEXT PRIMARY KEY, connection_state TEXT, readiness TEXT,
  last_seen_at_us INTEGER, observed_state_json TEXT
);
CREATE TABLE stage_device_commands (
  command_id TEXT PRIMARY KEY, device_id TEXT, command_type TEXT, status TEXT,
  issued_at_us INTEGER, completed_at_us INTEGER
);
""")
tablet_caps = [
 "tablet.media.prepare","tablet.media.play","tablet.media.pause","tablet.media.stop",
 "tablet.media.blackout","tablet.media.blackout.clear","tablet.media.overlay.play",
 "tablet.media.overlay.clear","tablet.media.live.show","tablet.media.live.hide"
]
lighting_caps = [
 "lighting.channels.set","lighting.channels.fade","lighting.blackout",
 "lighting.state.read","lighting.identify","lighting.config.read","lighting.config.apply"
]
conn.execute("INSERT INTO stage_devices VALUES (?,?,?,?,?,?,?,?,?,?,?)",
 ("tablet-01","project-1","stagecore.tablet-player","TABLET_PLAYER","Tablet 01","android","arm64","1.0.0-rc2","stagecore.device/1",json.dumps(tablet_caps),1))
conn.execute("INSERT INTO stage_device_runtime_state VALUES (?,?,?,?,?)",
 ("tablet-01","ONLINE","READY",now_us,json.dumps({"project_id":"project-1","runtime_snapshot_id":"snap-1","battery_percent":90,"charging":True})))
conn.execute("INSERT INTO stage_device_commands VALUES (?,?,?,?,?,?)",
 ("cmd-tablet","tablet-01","TABLET_PREPARE","COMPLETED",now_us-1_000_000,now_us-900_000))
conn.execute("INSERT INTO stage_devices VALUES (?,?,?,?,?,?,?,?,?,?,?)",
 ("lighting-01","project-1","stagecore.esp32-dmx-lighting-node","GENERIC","Lighting 01","esp32","xtensa","1.0.0","stagecore.device/1",json.dumps(lighting_caps),1))
conn.execute("INSERT INTO stage_device_runtime_state VALUES (?,?,?,?,?)",
 ("lighting-01","ONLINE","READY",now_us,json.dumps({"schema_version":1,"firmware_version":"1.0.0","dmx_healthy":True,"configuration_hash":"abc123","brownout_warning":False,"authority":"STAGECORE","current_levels":{"warm_a":0}})))
conn.execute("INSERT INTO stage_device_commands VALUES (?,?,?,?,?,?)",
 ("cmd-light","lighting-01","LIGHTING_STATE_READ","COMPLETED",now_us-800_000,now_us-700_000))
conn.commit()
conn.close()
PY

probe="$tmp/probe.json"
python3 "$REPO_ROOT/tools/qualification/pi/stagecore-qualification-probe.py" --db "$db" >"$probe"
python3 "$REPO_ROOT/tools/qualification/assert-device-probe.py" --input "$probe" --kind tablet --check readiness --project-id project-1 --device-id tablet-01
python3 "$REPO_ROOT/tools/qualification/assert-device-probe.py" --input "$probe" --kind tablet --check scope --project-id project-1 --runtime-snapshot-id snap-1 --device-id tablet-01
python3 "$REPO_ROOT/tools/qualification/assert-device-probe.py" --input "$probe" --kind lighting --check observation --project-id project-1 --device-id lighting-01

python3 - "$probe" <<'PY'
import json, sys
data=json.load(open(sys.argv[1], encoding="utf-8"))
assert data["schema_version"] == 1
assert len(data["devices"]) == 2
raw=json.dumps(data)
assert "payload_json" not in raw
assert "result_json" not in raw
PY

echo "qualification device probe self-test PASS"
