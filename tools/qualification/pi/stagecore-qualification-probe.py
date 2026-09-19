#!/usr/bin/env python3
import argparse
import datetime as dt
import json
import os
import sqlite3
import sys

TABLET_PROFILE = "stagecore.tablet-player"
LIGHTING_PROFILE = "stagecore.esp32-dmx-lighting-node"
ALLOWED_PROFILES = (TABLET_PROFILE, LIGHTING_PROFILE)

def safe_json(raw):
    try:
        value = json.loads(raw or "{}")
        return value if isinstance(value, dict) else {}
    except json.JSONDecodeError:
        return {}

def filtered_observation(profile_id, raw):
    value = safe_json(raw)
    if profile_id == TABLET_PROFILE:
        keys = {
            "project_id", "runtime_snapshot_id", "tablet_manifest_id",
            "battery_percent", "charging", "brightness", "orientation",
            "app_mode", "show_lock_enabled", "storage_permission_state",
            "media_scan", "last_error", "main", "overlay", "live", "blackout",
        }
    else:
        keys = {
            "schema_version", "firmware_version", "uptime_seconds", "reset_reason",
            "wifi_rssi_dbm", "current_levels", "active_fade",
            "last_accepted_command_id", "last_applied_command_id",
            "dmx_healthy", "configuration_hash", "brownout_warning", "authority",
        }
    return {k: value[k] for k in keys if k in value}

def latest_command(conn, device_id):
    row = conn.execute(
        """
        SELECT command_id, command_type, status, issued_at_us, completed_at_us
        FROM stage_device_commands
        WHERE device_id = ?
        ORDER BY issued_at_us DESC
        LIMIT 1
        """,
        (device_id,),
    ).fetchone()
    if row is None:
        return None
    return {
        "command_id": row[0],
        "command_type": row[1],
        "status": row[2],
        "issued_at_us": row[3],
        "completed_at_us": row[4],
    }

def main():
    parser = argparse.ArgumentParser(description="Read-only StageCore physical qualification device probe")
    parser.add_argument(
        "--db",
        default="/var/lib/stagecore/data/db/stagecore.sqlite3",
        help="StageCore SQLite database path",
    )
    args = parser.parse_args()

    db_path = os.path.abspath(args.db)
    if not os.path.isfile(db_path):
        print(f"qualification probe database not found: {db_path}", file=sys.stderr)
        return 66

    uri = "file:" + db_path + "?mode=ro"
    conn = sqlite3.connect(uri, uri=True)
    try:
        rows = conn.execute(
            """
            SELECT d.device_id, COALESCE(d.project_id, ''), COALESCE(d.profile_id, ''),
                   d.device_kind, d.display_name, d.platform, d.architecture,
                   d.client_version, d.protocol_version, d.capabilities_json,
                   d.enabled,
                   r.connection_state, r.readiness, r.last_seen_at_us,
                   r.observed_state_json
            FROM stage_devices d
            LEFT JOIN stage_device_runtime_state r ON r.device_id = d.device_id
            WHERE d.profile_id IN (?, ?)
            ORDER BY d.profile_id, d.display_name COLLATE NOCASE, d.device_id
            """,
            ALLOWED_PROFILES,
        ).fetchall()

        devices = []
        for row in rows:
            try:
                capabilities = json.loads(row[9] or "[]")
                if not isinstance(capabilities, list):
                    capabilities = []
            except json.JSONDecodeError:
                capabilities = []
            runtime = None
            if row[11] is not None:
                runtime = {
                    "connection_state": row[11],
                    "readiness": row[12],
                    "last_seen_at_us": row[13],
                    "observed": filtered_observation(row[2], row[14]),
                }
            devices.append({
                "device_id": row[0],
                "project_id": row[1],
                "profile_id": row[2],
                "device_kind": row[3],
                "display_name": row[4],
                "platform": row[5],
                "architecture": row[6],
                "client_version": row[7],
                "protocol_version": row[8],
                "capabilities": capabilities,
                "enabled": bool(row[10]),
                "runtime": runtime,
                "latest_command": latest_command(conn, row[0]),
            })

        payload = {
            "schema_version": 1,
            "generated_at": dt.datetime.now(dt.timezone.utc).isoformat(),
            "database": os.path.basename(db_path),
            "devices": devices,
        }
        json.dump(payload, sys.stdout, sort_keys=True, separators=(",", ":"))
        sys.stdout.write("\n")
        return 0
    finally:
        conn.close()

if __name__ == "__main__":
    raise SystemExit(main())
