#!/usr/bin/env python3
"""Read-only redacted Phase 4 inventory: capture PASS is not a physical gate PASS."""
import argparse
import json
import math
import os
import sqlite3
import sys
import time

CAPS = {"display.message.show", "display.countdown.show", "display.alert.show",
        "display.clear", "display.blackout", "display.chime.play"}
CLASSES = {"LOCAL_CAMERA", "USB_CAPTURE", "NETWORK_STREAM"}


def emit(value):
    print(json.dumps(value, sort_keys=True, separators=(",", ":")))


def stop(message, code=3):
    emit({"status": "BLOCKED" if code == 3 else "FAIL", "detail": message})
    raise SystemExit(code)


def main():
    parser = argparse.ArgumentParser(description="Read-only Phase 4 device evidence")
    parser.add_argument("--db", default="/var/lib/stagecore/data/db/stagecore.sqlite3")
    args = parser.parse_args()
    raw = sys.stdin.buffer.read(4097)
    if len(raw) > 4096:
        stop("request too large", 1)
    try:
        data = json.loads(raw)
    except (ValueError, UnicodeDecodeError):
        stop("invalid request", 1)
    if not isinstance(data, dict) or set(data) != {"project_id"}:
        stop("only project_id is accepted", 1)
    project = data["project_id"]
    if not isinstance(project, str) or not project.strip() or len(project) > 256:
        stop("invalid project_id", 1)
    project = project.strip()
    db = os.path.abspath(args.db)
    if not os.path.isfile(db):
        stop("database unavailable")
    try:
        conn = sqlite3.connect("file:" + db + "?mode=ro", uri=True)
        try:
            if conn.execute("SELECT 1 FROM projects WHERE project_id=?", (project,)).fetchone() is None:
                stop("project unavailable")
            now = int(time.time() * 1000000)
            displays = []
            for row in conn.execute(
                """SELECT d.device_id,d.display_name,d.capabilities_json,d.enabled,d.protocol_version,
                          r.connection_state,r.readiness,r.last_seen_at_us
                   FROM stage_devices d LEFT JOIN stage_device_runtime_state r ON r.device_id=d.device_id
                   WHERE d.project_id=? AND d.device_kind='STAGE_DISPLAY' ORDER BY d.device_id""",
                (project,),
            ):
                try:
                    caps = json.loads(row[2] or "[]")
                    if not isinstance(caps, list):
                        raise ValueError()
                except (ValueError, TypeError):
                    stop("invalid display capabilities", 1)
                displays.append({
                    "device_id": row[0], "display_name": row[1],
                    "capabilities": sorted({c for c in caps if isinstance(c, str) and c in CAPS}),
                    "enabled": row[3] == 1, "protocol_version": row[4],
                    "connection_state": row[5] or "UNKNOWN", "readiness": row[6] or "UNKNOWN",
                    "fresh": row[7] is not None and 0 <= now - row[7] <= 15000000,
                })
            sources = []
            for row in conn.execute(
                """SELECT source_id,name,source_class,execution_device_id,execution_machine_role_id,required,desired_enabled,
                          readiness,last_observed_at_us
                   FROM live_video_sources WHERE project_id=? ORDER BY source_id""",
                (project,),
            ):
                if row[2] not in CLASSES:
                    stop("invalid live source class", 1)
                sources.append({
                    "source_id": row[0], "name": row[1], "source_class": row[2],
                    "execution_device_id": row[3] or "", "execution_machine_role_id": row[4] or "",
                    "required": row[5] == 1, "desired_enabled": row[6] == 1,
                    "readiness": row[7],
                    "fresh": row[8] is not None and 0 <= now - row[8] <= 15000000,
                })
            targets = {}
            for row in conn.execute(
                """SELECT target_kind,target_id,observed_at_us,reachability,transport_state,
                          latency_ms,jitter_ms,error_code
                   FROM network_observations ORDER BY observed_at_us DESC,observation_id DESC"""
            ):
                kind, target, at, reach, transport, latency, jitter, error = row
                if (kind, target) in targets:
                    continue
                for number in (latency, jitter):
                    if number is not None and (not math.isfinite(number) or number < 0):
                        stop("invalid network metric", 1)
                targets[(kind, target)] = {
                    "target_kind": kind, "target_id": target, "observed_at_us": at,
                    "reachability": reach, "transport_state": transport,
                    "error_code": error or "", "stale": not (0 <= now - at <= 15000000),
                    "latency_ms": latency, "jitter_ms": jitter,
                    "metric_provenance": "UNVERIFIED" if latency is not None or jitter is not None else "NOT_MEASURED",
                }
            eligible = [d for d in displays if d["enabled"] and d["fresh"] and
                        d["connection_state"] == "ONLINE" and d["readiness"] == "READY" and
                        d["protocol_version"] == "stagecore.device/1"]
            emit({
                "status": "PASS", "mode": "phase4-inventory", "project_id": project,
                "generated_at_us": now, "displays": displays, "sources": sources,
                "network_targets": list(targets.values()),
                "availability": {
                    "eligible_display_count": len(eligible),
                    "eligible_chime_display_count": sum("display.chime.play" in d["capabilities"] for d in eligible),
                    "configured_source_classes": sorted({s["source_class"] for s in sources}),
                },
                "limitations": [
                    "Inventory does not prove physical rendering/source behavior.",
                    "Numeric network metrics require independent measurement provenance.",
                    "Endpoint URLs, addresses, source configs, raw observations and credentials are excluded.",
                ],
            })
        finally:
            conn.close()
    except sqlite3.Error as exc:
        stop("read-only query failed: " + str(exc))


if __name__ == "__main__":
    main()
