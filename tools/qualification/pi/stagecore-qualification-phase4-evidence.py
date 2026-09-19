#!/usr/bin/env python3
"""Bounded read-only Phase 4 evidence. No client or physical gate PASS implied."""
import argparse
import json
import os
import sqlite3
import sys
import time

CAPABILITY_COMMAND = {
    "display.message.show": "DISPLAY_MESSAGE",
    "display.countdown.show": "DISPLAY_COUNTDOWN",
    "display.alert.show": "DISPLAY_ALERT",
    "display.clear": "DISPLAY_CLEAR",
    "display.blackout": "DISPLAY_BLACKOUT",
    "display.chime.play": "DISPLAY_CHIME",
}
SOURCE_CLASSES = ("LOCAL_CAMERA", "USB_CAPTURE", "NETWORK_STREAM")


def emit(data):
    print(json.dumps(data, sort_keys=True, separators=(",", ":")))


def stop(message, code=1):
    emit({"status": "BLOCKED" if code == 3 else "FAIL", "detail": message})
    raise SystemExit(code)


def bounded(value, name):
    if not isinstance(value, str) or not value.strip() or len(value) > 256:
        stop("invalid " + name)
    if any(ord(ch) < 32 for ch in value):
        stop("invalid " + name)
    return value.strip()


def callboard_cue(conn, data):
    project = bounded(data.get("project_id"), "project_id")
    device = bounded(data.get("device_id"), "device_id")
    snapshot = bounded(data.get("runtime_snapshot_id"), "runtime_snapshot_id")
    name = bounded(data.get("cue_name"), "cue_name")
    row = conn.execute(
        """SELECT d.project_id,d.device_kind,d.enabled,d.protocol_version,
                  r.connection_state,r.readiness,r.last_seen_at_us
           FROM stage_devices d LEFT JOIN stage_device_runtime_state r ON r.device_id=d.device_id
           WHERE d.device_id=?""", (device,),
    ).fetchone()
    now = int(time.time() * 1000000)
    if (row is None or row[:4] != (project, "STAGE_DISPLAY", 1, "stagecore.device/1")
            or row[4:6] != ("ONLINE", "READY") or row[6] is None
            or not 0 <= now - row[6] <= 15000000):
        stop("real Stage Display target is not freshly qualified", 3)
    snap = conn.execute(
        "SELECT revision_id,status FROM runtime_snapshots WHERE project_id=? AND runtime_snapshot_id=?",
        (project, snapshot),
    ).fetchone()
    if snap is None or snap[1] != "PUBLISHED":
        stop("pinned Published Runtime Snapshot unavailable", 3)
    cues = conn.execute(
        """SELECT cue_id,cue_type,enabled FROM cues
           WHERE revision_id=? AND name=?""", (snap[0], name),
    ).fetchall()
    if len(cues) != 1 or cues[0][2] != 1:
        stop("unique enabled Callboard Cue is unavailable", 3)
    cue_id = cues[0][0]
    actions = conn.execute(
        """SELECT a.action_id,a.capability_key,a.enabled,p.logical_type,p.project_config_json
           FROM actions a LEFT JOIN project_device_aliases p
             ON p.project_id=? AND p.logical_name=a.target_ref
           WHERE a.cue_id=? ORDER BY a.order_index,a.action_id""",
        (project, cue_id),
    ).fetchall()
    if not actions:
        stop("qualification Cue has no display actions", 3)
    expected = {}
    for action_id, capability, enabled, kind, raw_alias in actions:
        if enabled != 1 or capability not in CAPABILITY_COMMAND or kind != "stage_device":
            stop("Cue contains non-display/disabled or non-canonical action")
        try:
            alias = json.loads(raw_alias or "{}")
        except ValueError:
            stop("invalid Cue Stage Device alias")
        if alias.get("device_id") != device or alias.get("capability_key") != capability:
            stop("Cue action is not bound to the selected real display")
        expected[action_id] = CAPABILITY_COMMAND[capability]
    executed = conn.execute(
        """SELECT ce.cue_execution_id,ce.correlation_id,ce.result,ce.session_id
           FROM cue_executions ce JOIN sessions s ON s.session_id=ce.session_id
           WHERE ce.cue_id=? AND s.project_id=? AND s.runtime_snapshot_id=?
           ORDER BY ce.started_at_us DESC LIMIT 1""",
        (cue_id, project, snapshot),
    ).fetchone()
    if executed is None:
        stop("published display Cue has not been executed", 3)
    if executed[2] != "COMPLETED" or not executed[1]:
        stop("latest display Cue execution is not complete")
    action_results = conn.execute(
        """SELECT action_execution_id,action_id,result FROM action_executions
           WHERE cue_execution_id=?""", (executed[0],),
    ).fetchall()
    actual = {a[1] for a in action_results}
    if len(action_results) != len(expected) or actual != set(expected) or any(a[2] != "COMPLETED" for a in action_results):
        stop("display Cue action executions did not complete exactly")
    commands = conn.execute(
        """SELECT command_id,command_type,status,causation_id,runtime_snapshot_id
           FROM stage_device_commands
           WHERE project_id=? AND device_id=? AND correlation_id=?""",
        (project, device, executed[1]),
    ).fetchall()
    causes = {a[0]: expected[a[1]] for a in action_results}
    actual_causes = {}
    for command_id, command_type, status, causation, cmd_snapshot in commands:
        if causation not in causes or command_type != causes[causation] or status != "COMPLETED" or cmd_snapshot != snapshot:
            stop("display command correlation/scope or completion mismatch")
        if causation in actual_causes:
            stop("duplicate display command causation in Cue execution")
        actual_causes[causation] = command_id
    if set(actual_causes) != set(causes):
        stop("a completed display Cue action lacks its exact Stage Device command")
    emit({
        "status": "PASS", "mode": "callboard-cue", "project_id": project, "device_id": device,
        "runtime_snapshot_id": snapshot, "cue_id": cue_id, "cue_execution_id": executed[0],
        "session_id": executed[3], "action_count": len(expected),
        "command_ids": sorted(actual_causes.values()),
        "go_authority": False,
        "limitations": "Read-only command correlation only; actual rendered result still needs human observation.",
    })


def source_coverage(conn, data):
    project = bounded(data.get("project_id"), "project_id")
    if conn.execute("SELECT 1 FROM projects WHERE project_id=?", (project,)).fetchone() is None:
        stop("project unavailable", 3)
    rows = conn.execute(
        """SELECT source_id,source_class,readiness,desired_enabled,execution_device_id
           FROM live_video_sources WHERE project_id=? ORDER BY source_class,source_id""",
        (project,),
    ).fetchall()
    configured = {r[1] for r in rows}
    if not configured.issubset(SOURCE_CLASSES):
        stop("unknown source class")
    emit({
        "status": "PASS", "mode": "source-coverage", "project_id": project,
        "configured_classes": sorted(configured),
        "unconfigured_classes": sorted(set(SOURCE_CLASSES) - configured),
        "sources": [
            {"source_id": r[0], "source_class": r[1], "readiness": r[2],
             "desired_enabled": r[3] == 1, "execution_device_id": r[4] or ""}
            for r in rows
        ],
        "limitations": "Configuration is not physical availability or a source open/render PASS.",
    })


def network_null_metrics(conn, data):
    project = bounded(data.get("project_id"), "project_id")
    if conn.execute("SELECT 1 FROM projects WHERE project_id=?", (project,)).fetchone() is None:
        stop("project unavailable", 3)
    rows = conn.execute(
        """SELECT o.target_kind,o.target_id,o.latency_ms,o.jitter_ms FROM network_observations o
           WHERE (
             o.target_kind IN ('HUB','COMPANION')
             OR (o.target_kind='STAGE_DEVICE' AND o.target_id IN (
                 SELECT device_id FROM stage_devices WHERE project_id=?))
             OR (o.target_kind='LIVE_SOURCE' AND o.target_id IN (
                 SELECT source_id FROM live_video_sources WHERE project_id=?))
           )
           AND o.observation_id=(
               SELECT n.observation_id FROM network_observations n
               WHERE n.target_kind=o.target_kind AND n.target_id=o.target_id
               ORDER BY n.observed_at_us DESC,n.observation_id DESC LIMIT 1
           ) ORDER BY o.target_kind,o.target_id""",
        (project, project),
    ).fetchall()
    if not rows:
        stop("no real network observation exists", 3)
    unverified = [r[0] + ":" + r[1] for r in rows if r[2] is not None or r[3] is not None]
    if unverified:
        emit({"status": "BLOCKED", "mode": "network-null-metrics", "project_id": project,
              "unverified_targets": unverified,
              "detail": "numeric latency/jitter needs independent measurement provenance and Operator UI verification"})
        raise SystemExit(3)
    emit({"status": "PASS", "mode": "network-null-metrics", "project_id": project,
          "not_measured_target_count": len(rows), "numeric_metrics_present": False,
          "limitations": "Null persistence is proven; Operator Cockpit display/classification still needs independent verification."})


def main():
    parser = argparse.ArgumentParser(description="Read-only Phase 4 qualification evidence")
    parser.add_argument("--db", default="/var/lib/stagecore/data/db/stagecore.sqlite3")
    args = parser.parse_args()
    raw = sys.stdin.buffer.read(32769)
    if len(raw) > 32768:
        stop("request too large")
    try:
        data = json.loads(raw)
    except (ValueError, UnicodeDecodeError):
        stop("invalid request")
    if not isinstance(data, dict):
        stop("request must be an object")
    mode = data.get("mode")
    fields = {
        "callboard-cue": {"mode", "project_id", "device_id", "runtime_snapshot_id", "cue_name"},
        "source-coverage": {"mode", "project_id"},
        "network-null-metrics": {"mode", "project_id"},
    }
    if mode not in fields or set(data) != fields[mode]:
        stop("unsupported mode or extra request fields")
    db = os.path.abspath(args.db)
    if not os.path.isfile(db):
        stop("database unavailable", 3)
    try:
        conn = sqlite3.connect("file:" + db + "?mode=ro", uri=True)
        try:
            {
                "callboard-cue": callboard_cue,
                "source-coverage": source_coverage,
                "network-null-metrics": network_null_metrics,
            }[mode](conn, data)
        finally:
            conn.close()
    except sqlite3.Error as exc:
        stop("Phase 4 evidence query unavailable: " + str(exc), 3)


if __name__ == "__main__":
    main()
