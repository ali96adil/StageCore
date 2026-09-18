#!/usr/bin/env python3
import argparse
import datetime as dt
import json
import os
import sqlite3
import sys

TABLET_PROFILE = "stagecore.tablet-player"
TABLET_KIND = "TABLET_PLAYER"
STAGE_DEVICE_TYPE = "stage_device"


def emit(value):
    print(json.dumps(value, sort_keys=True, separators=(",", ":")))


def fail(message, code=1):
    emit({"status": "BLOCKED" if code == 3 else "FAIL", "detail": message})
    raise SystemExit(code)


def read_request():
    raw = sys.stdin.buffer.read(32769)
    if len(raw) > 32768:
        fail("tablet evidence request too large")
    try:
        value = json.loads(raw.decode("utf-8"))
    except Exception:
        fail("invalid tablet evidence request")
    if not isinstance(value, dict):
        fail("tablet evidence request must be an object")
    return value


def text(value, name, maximum=256, required=True):
    value = str(value or "").strip()
    if required and not value:
        fail(f"{name} is required")
    if len(value) > maximum or any(ord(ch) < 32 for ch in value):
        fail(f"invalid {name}")
    return value


def parse_time(value, label):
    raw = text(value, label, 128)
    try:
        return dt.datetime.fromisoformat(raw.replace("Z", "+00:00")).astimezone(dt.timezone.utc)
    except ValueError:
        fail(f"invalid {label}")


def micros(value):
    return int(value.timestamp() * 1_000_000)


def open_db(path):
    path = os.path.abspath(path)
    if not os.path.isfile(path):
        fail("StageCore database unavailable", 3)
    try:
        return sqlite3.connect("file:" + path + "?mode=ro", uri=True)
    except sqlite3.Error as exc:
        fail("StageCore database open failed: " + str(exc), 3)


def tablet_runtime(conn, device_id, project_id, snapshot_id=""):
    row = conn.execute(
        """
        SELECT d.project_id,d.profile_id,d.device_kind,d.client_version,
               r.connection_state,r.readiness,r.last_seen_at_us,r.observed_state_json
        FROM stage_devices d
        LEFT JOIN stage_device_runtime_state r ON r.device_id=d.device_id
        WHERE d.device_id=?
        """,
        (device_id,),
    ).fetchone()
    if row is None:
        fail("tablet device is not registered", 3)
    if row[0] != project_id or row[1] != TABLET_PROFILE or row[2] != TABLET_KIND:
        fail("tablet identity/project/profile mismatch")
    if row[4] != "ONLINE" or row[5] != "READY":
        fail(f"tablet runtime is not ONLINE/READY: {row[4]}/{row[5]}", 3)
    try:
        observed = json.loads(row[7] or "{}")
    except json.JSONDecodeError:
        fail("tablet observed state is invalid")
    if observed.get("project_id") != project_id:
        fail("tablet observed project mismatch")
    if snapshot_id and observed.get("runtime_snapshot_id") != snapshot_id:
        fail("tablet observed Runtime Snapshot mismatch")
    return {
        "client_version": row[3],
        "connection_state": row[4],
        "readiness": row[5],
        "last_seen_at_us": row[6],
        "observed": observed,
    }


def find_scene(conn, project_id, cue_name, revision_id=None):
    if revision_id is None:
        row = conn.execute("SELECT current_revision_id FROM projects WHERE project_id=?", (project_id,)).fetchone()
        if row is None or not row[0]:
            fail("project/current revision unavailable", 3)
        revision_id = row[0]
    rows = conn.execute(
        """
        SELECT cue_id,revision_id,display_label,name,cue_type,enabled
        FROM cues WHERE revision_id=? AND name=?
        """,
        (revision_id, cue_name),
    ).fetchall()
    if not rows:
        fail("qualification Tablet Scene not found", 3)
    if len(rows) != 1:
        fail("qualification Tablet Scene name is ambiguous")
    cue = rows[0]
    if cue[4] != "TABLET_SCENE" or cue[5] != 1:
        fail("qualification cue is not an enabled TABLET_SCENE")
    return cue


def canonical_scene(conn, project_id, device_id, cue_name, revision_id=None):
    cue = find_scene(conn, project_id, cue_name, revision_id)
    rows = conn.execute(
        """
        SELECT a.action_id,a.order_index,a.execution_mode,a.target_ref,a.capability_key,
               a.parameters_json,a.priority_class,a.enabled,
               p.logical_type,p.project_config_json
        FROM actions a
        LEFT JOIN project_device_aliases p
          ON p.project_id=? AND p.logical_name=a.target_ref
        WHERE a.cue_id=?
        ORDER BY a.order_index,a.action_id
        """,
        (project_id, cue[0]),
    ).fetchall()
    if not rows:
        fail("qualification Tablet Scene has no actions")
    actions = []
    for row in rows:
        if row[7] != 1 or not str(row[4]).startswith("tablet.media."):
            fail("Tablet Scene contains a disabled/non-tablet action")
        if row[8] != STAGE_DEVICE_TYPE:
            fail("Tablet Scene action target is not a canonical stage_device alias")
        try:
            alias = json.loads(row[9] or "{}")
            params = json.loads(row[5] or "{}")
        except json.JSONDecodeError:
            fail("Tablet Scene action/alias JSON is invalid")
        if not isinstance(params, dict) or alias.get("device_id") != device_id or alias.get("capability_key") != row[4]:
            fail("Tablet Scene alias/action canonical binding mismatch")
        actions.append({
            "action_id": row[0], "order_index": row[1], "execution_mode": row[2],
            "target_ref": row[3], "capability_key": row[4],
            "parameters": params, "priority": row[6],
        })
    return cue, actions


def mode_cue_canonical(conn, data):
    project_id = text(data.get("project_id"), "project_id")
    device_id = text(data.get("device_id"), "device_id")
    cue_name = text(data.get("cue_name"), "cue_name")
    runtime = tablet_runtime(conn, device_id, project_id)
    cue, actions = canonical_scene(conn, project_id, device_id, cue_name)
    emit({
        "status": "PASS", "mode": "cue-canonical", "project_id": project_id,
        "device_id": device_id, "client_version": runtime["client_version"],
        "cue_id": cue[0], "revision_id": cue[1], "cue_name": cue[3],
        "cue_type": cue[4], "action_count": len(actions), "actions": actions,
    })


def mode_cue_execution(conn, data):
    project_id = text(data.get("project_id"), "project_id")
    device_id = text(data.get("device_id"), "device_id")
    cue_name = text(data.get("cue_name"), "cue_name")
    snapshot_id = text(data.get("runtime_snapshot_id"), "runtime_snapshot_id")
    tablet_runtime(conn, device_id, project_id, snapshot_id)

    snap = conn.execute(
        "SELECT revision_id,status FROM runtime_snapshots WHERE runtime_snapshot_id=? AND project_id=?",
        (snapshot_id, project_id),
    ).fetchone()
    if snap is None or snap[1] != "PUBLISHED":
        fail("exact Published Runtime Snapshot unavailable", 3)
    cue, actions = canonical_scene(conn, project_id, device_id, cue_name, snap[0])

    execution = conn.execute(
        """
        SELECT ce.cue_execution_id,ce.session_id,ce.correlation_id,ce.started_at_us,
               ce.completed_at_us,ce.result
        FROM cue_executions ce
        JOIN sessions s ON s.session_id=ce.session_id
        WHERE ce.cue_id=? AND s.project_id=? AND s.runtime_snapshot_id=?
        ORDER BY ce.started_at_us DESC LIMIT 1
        """,
        (cue[0], project_id, snapshot_id),
    ).fetchone()
    if execution is None:
        fail("Published Tablet Scene has not been executed in the pinned Runtime Snapshot", 3)
    if execution[5] != "COMPLETED":
        fail("latest Published Tablet Scene execution is not COMPLETED")

    expected_action_ids = {item["action_id"] for item in actions}
    action_rows = conn.execute(
        """
        SELECT action_execution_id,action_id,result,error_code FROM action_executions
        WHERE cue_execution_id=?
        """,
        (execution[0],),
    ).fetchall()
    actual_action_ids = {row[1] for row in action_rows}
    action_execution_ids = {row[0] for row in action_rows}
    if actual_action_ids != expected_action_ids or any(row[2] != "COMPLETED" for row in action_rows):
        fail("Cue Engine action execution did not complete the canonical Tablet Scene")

    commands = conn.execute(
        """
        SELECT command_id,command_type,status,runtime_snapshot_id,correlation_id,causation_id
        FROM stage_device_commands
        WHERE device_id=? AND project_id=? AND correlation_id=?
        ORDER BY issued_at_us
        """,
        (device_id, project_id, execution[2]),
    ).fetchall()
    completed = [
        row for row in commands
        if row[2] == "COMPLETED" and row[3] == snapshot_id and str(row[1]).startswith("TABLET_")
    ]
    if not completed:
        fail("Cue Engine execution has no completed Tablet Stage Device command")
    command_causation_ids = {row[5] for row in completed}
    if command_causation_ids != action_execution_ids:
        fail("Tablet Stage Device command causation does not match the completed Cue action executions")
    emit({
        "status": "PASS", "mode": "cue-execution", "project_id": project_id,
        "device_id": device_id, "runtime_snapshot_id": snapshot_id,
        "cue_id": cue[0], "cue_execution_id": execution[0],
        "session_id": execution[1], "correlation_id": execution[2],
        "action_count": len(actions),
        "commands": [
            {"command_id": row[0], "command_type": row[1], "status": row[2], "causation_id": row[5]}
            for row in completed
        ],
    })


def mode_reconnect_pre(conn, data):
    project_id = text(data.get("project_id"), "project_id")
    device_id = text(data.get("device_id"), "device_id")
    snapshot_id = text(data.get("runtime_snapshot_id"), "runtime_snapshot_id")
    runtime = tablet_runtime(conn, device_id, project_id, snapshot_id)
    row = conn.execute(
        "SELECT COALESCE(MAX(issued_at_us),0), COUNT(*) FROM stage_device_commands WHERE device_id=?",
        (device_id,),
    ).fetchone()
    emit({
        "status": "PASS", "mode": "reconnect-pre", "project_id": project_id,
        "device_id": device_id, "runtime_snapshot_id": snapshot_id,
        "captured_at": dt.datetime.now(dt.timezone.utc).isoformat(),
        "baseline_issued_at_us": int(row[0] or 0), "baseline_command_count": int(row[1] or 0),
        "client_version": runtime["client_version"],
    })


def mode_reconnect_post(conn, data):
    project_id = text(data.get("project_id"), "project_id")
    device_id = text(data.get("device_id"), "device_id")
    snapshot_id = text(data.get("runtime_snapshot_id"), "runtime_snapshot_id")
    baseline = data.get("baseline_issued_at_us")
    if isinstance(baseline, bool) or not isinstance(baseline, int) or baseline < 0:
        fail("invalid baseline_issued_at_us")
    baseline_captured_at = parse_time(data.get("baseline_captured_at"), "baseline_captured_at")
    disconnect_at = parse_time(data.get("disconnect_at"), "disconnect_at")
    reconnect_at = parse_time(data.get("reconnect_at"), "reconnect_at")
    if disconnect_at < baseline_captured_at:
        fail("disconnect acknowledgement predates reconnect baseline")
    if reconnect_at < disconnect_at:
        fail("reconnect acknowledgement predates disconnect")
    runtime = tablet_runtime(conn, device_id, project_id, snapshot_id)

    offline = conn.execute(
        """
        SELECT COUNT(*) FROM network_observations
        WHERE target_kind='STAGE_DEVICE' AND target_id=? AND observed_at_us>=? AND observed_at_us<=?
          AND (reachability='UNREACHABLE' OR transport_state='WEBSOCKET_DISCONNECTED')
        """,
        (device_id, micros(baseline_captured_at) - 1_000_000, micros(reconnect_at) + 5_000_000),
    ).fetchone()[0]
    online = conn.execute(
        """
        SELECT COUNT(*) FROM network_observations
        WHERE target_kind='STAGE_DEVICE' AND target_id=? AND observed_at_us>=? AND observed_at_us<=?
          AND reachability='REACHABLE' AND transport_state='WEBSOCKET_CONNECTED'
        """,
        (device_id, micros(disconnect_at), micros(reconnect_at) + 5_000_000),
    ).fetchone()[0]
    if offline < 1:
        fail("no Stage Device disconnect observation found for Q-TAB-14", 3)
    if online < 1:
        fail("no Stage Device reconnect observation found for Q-TAB-14", 3)

    commands_after = conn.execute(
        "SELECT COUNT(*) FROM stage_device_commands WHERE device_id=? AND issued_at_us>?",
        (device_id, baseline),
    ).fetchone()[0]
    if commands_after != 0:
        fail("new production Tablet command appeared during the reconnect no-replay window")

    emit({
        "status": "PASS", "mode": "reconnect-post", "project_id": project_id,
        "device_id": device_id, "runtime_snapshot_id": snapshot_id,
        "disconnect_observations": offline, "reconnect_observations": online,
        "commands_after_baseline": commands_after,
        "connection_state": runtime["connection_state"], "readiness": runtime["readiness"],
        "client_version": runtime["client_version"],
    })


def main():
    parser = argparse.ArgumentParser(description="Read-only Tablet qualification evidence")
    parser.add_argument("--db", default="/var/lib/stagecore/data/db/stagecore.sqlite3")
    args = parser.parse_args()
    data = read_request()
    mode = text(data.get("mode"), "mode", 64)
    conn = open_db(args.db)
    try:
        if mode == "cue-canonical":
            mode_cue_canonical(conn, data)
        elif mode == "cue-execution":
            mode_cue_execution(conn, data)
        elif mode == "reconnect-pre":
            mode_reconnect_pre(conn, data)
        elif mode == "reconnect-post":
            mode_reconnect_post(conn, data)
        else:
            fail("unsupported tablet evidence mode")
    except sqlite3.Error as exc:
        fail("tablet evidence query failed: " + str(exc), 3)
    finally:
        conn.close()


if __name__ == "__main__":
    main()
