#!/usr/bin/env python3
import argparse
import hashlib
import json
import os
import sqlite3
import sys

PROFILE_ID = "stagecore.esp32-dmx-lighting-node"


def emit(value):
    print(json.dumps(value, sort_keys=True, separators=(",", ":")))


def fail(message, code=1):
    emit({"status": "BLOCKED" if code == 3 else "FAIL", "detail": message})
    raise SystemExit(code)


def read_request():
    raw = sys.stdin.buffer.read(16385)
    if len(raw) > 16384:
        fail("published-lighting request too large")
    try:
        value = json.loads(raw.decode("utf-8"))
    except Exception:
        fail("invalid published-lighting request")
    if not isinstance(value, dict):
        fail("published-lighting request must be an object")
    allowed = {"device_id", "project_id", "runtime_snapshot_id"}
    if set(value) - allowed:
        fail("published-lighting request contains unsupported fields")
    return value


def bounded_text(value, name, maximum=256):
    value = str(value or "").strip()
    if not value or len(value) > maximum or any(ord(ch) < 32 for ch in value):
        fail(f"invalid {name}")
    return value


def main():
    parser = argparse.ArgumentParser(description="Read exact Published Runtime Snapshot lighting configuration")
    parser.add_argument("--db", default="/var/lib/stagecore/data/db/stagecore.sqlite3")
    args = parser.parse_args()

    data = read_request()
    device_id = bounded_text(data.get("device_id"), "device_id")
    project_id = bounded_text(data.get("project_id"), "project_id")
    snapshot_id = bounded_text(data.get("runtime_snapshot_id"), "runtime_snapshot_id")

    db_path = os.path.abspath(args.db)
    if not os.path.isfile(db_path):
        fail("StageCore database unavailable", 3)

    try:
        conn = sqlite3.connect("file:" + db_path + "?mode=ro", uri=True)
    except sqlite3.Error as exc:
        fail("StageCore database open failed: " + str(exc), 3)
    try:
        row = conn.execute(
            """
            SELECT project_id, revision_id, snapshot_version, content_hash,
                   manifest_json, status
            FROM runtime_snapshots
            WHERE runtime_snapshot_id = ?
            """,
            (snapshot_id,),
        ).fetchone()
    except sqlite3.Error as exc:
        fail("runtime snapshot query failed: " + str(exc), 3)
    finally:
        conn.close()

    if row is None:
        fail("exact runtime snapshot is not installed", 3)
    if str(row[0] or "").strip() != project_id:
        fail("runtime snapshot project does not match qualification project")
    if row[5] != "PUBLISHED":
        fail("exact runtime snapshot is not PUBLISHED", 3)
    content_hash = str(row[3] or "").strip().lower()
    if len(content_hash) != 64 or any(ch not in "0123456789abcdef" for ch in content_hash):
        fail("runtime snapshot content hash is invalid")

    manifest_raw = str(row[4] or "")
    actual_content_hash = hashlib.sha256(manifest_raw.encode("utf-8")).hexdigest()
    if actual_content_hash != content_hash:
        fail("runtime snapshot manifest content hash mismatch")
    try:
        manifest = json.loads(manifest_raw)
    except Exception:
        fail("runtime snapshot manifest is invalid JSON")
    if not isinstance(manifest, dict):
        fail("runtime snapshot manifest must be an object")
    schema_version = manifest.get("schema_version")
    if isinstance(schema_version, bool) or not isinstance(schema_version, int) or schema_version < 5:
        fail("runtime snapshot manifest does not contain frozen lighting bindings", 3)
    if str(manifest.get("project_id") or "").strip() != project_id:
        fail("runtime snapshot manifest project mismatch")

    matches = []
    for item in manifest.get("lighting_nodes") or []:
        if isinstance(item, dict) and str(item.get("device_id") or "").strip() == device_id:
            matches.append(item)
    if len(matches) != 1:
        fail("exact runtime snapshot does not contain one lighting binding for the device")
    binding = matches[0]
    if str(binding.get("profile_id") or "").strip() != PROFILE_ID:
        fail("runtime snapshot lighting profile mismatch")
    configuration = binding.get("configuration")
    if not isinstance(configuration, dict):
        fail("runtime snapshot lighting configuration missing")

    emit({
        "status": "PASS",
        "runtime_snapshot_id": snapshot_id,
        "project_id": project_id,
        "revision_id": str(row[1] or ""),
        "snapshot_version": row[2],
        "snapshot_status": row[5],
        "snapshot_content_hash": content_hash,
        "manifest_schema_version": schema_version,
        "device_id": device_id,
        "profile_id": PROFILE_ID,
        "configuration": configuration,
    })
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
