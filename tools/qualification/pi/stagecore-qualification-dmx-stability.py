#!/usr/bin/env python3
import argparse
import datetime as dt
import http.client
import json
import math
import os
import socket
import sqlite3
import sys
import time
import uuid

SOCKET_PATH = os.environ.get("STAGECORE_QUALIFICATION_SOCKET", "/var/lib/stagecore/qualification-envelope.sock")
DB_PATH = os.environ.get("STAGECORE_DB", "/var/lib/stagecore/data/db/stagecore.sqlite3")


class UnixHTTPConnection(http.client.HTTPConnection):
    def __init__(self, path):
        super().__init__("localhost", timeout=20)
        self.path = path

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(self.timeout)
        self.sock.connect(self.path)


def fail(message, code=1):
    print(json.dumps({"status": "BLOCKED" if code == 3 else "FAIL", "detail": message}, sort_keys=True))
    raise SystemExit(code)


def read_request():
    raw = sys.stdin.buffer.read(32769)
    if len(raw) > 32768:
        fail("DMX-stability request too large")
    try:
        value = json.loads(raw.decode("utf-8"))
    except Exception:
        fail("invalid DMX-stability request")
    if not isinstance(value, dict):
        fail("DMX-stability request must be an object")
    return value


def bounded_text(value, name, maximum=256):
    value = str(value or "").strip()
    if not value or len(value) > maximum or any(ord(ch) < 32 for ch in value):
        fail(f"invalid {name}")
    return value


def finite_level(value, name):
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        fail(f"{name} must be numeric")
    value = float(value)
    if not math.isfinite(value) or value < 0 or value > 100:
        fail(f"{name} must be within 0..100")
    return value


def rfc3339(value):
    return value.astimezone(dt.timezone.utc).isoformat().replace("+00:00", "Z")


def envelope(command_id, command_type, project_id):
    now = dt.datetime.now(dt.timezone.utc)
    return {
        "command_id": command_id,
        "command_type": command_type,
        "schema_version": 1,
        "issued_at": rfc3339(now),
        "deadline_at": rfc3339(now + dt.timedelta(seconds=15)),
        "project_id": project_id,
        "runtime_snapshot_id": "",
        "issuer": "qualification:physical-runner",
        "correlation_id": command_id,
        "causation_id": "",
        "priority": "P2",
        "idempotency_key": "",
        "payload": {},
    }


def exchange(device_id, command):
    if not os.path.exists(SOCKET_PATH):
        fail("qualification envelope socket is unavailable", 3)
    body = json.dumps({"device_id": device_id, "command": command}, separators=(",", ":"))
    conn = UnixHTTPConnection(SOCKET_PATH)
    try:
        started = time.monotonic()
        conn.request("POST", "/v1/device-envelope", body=body, headers={"Content-Type": "application/json"})
        response = conn.getresponse()
        raw = response.read(1 << 20)
        latency_ms = (time.monotonic() - started) * 1000.0
    except (OSError, http.client.HTTPException) as exc:
        fail(f"qualification envelope socket error: {exc}", 3)
    finally:
        conn.close()
    if response.status != 200:
        fail(f"qualification envelope HTTP {response.status}", 3 if response.status == 503 else 1)
    try:
        value = json.loads(raw.decode("utf-8"))
    except Exception:
        fail("invalid qualification envelope response")
    return value, latency_ms


def open_db():
    return sqlite3.connect("file:" + os.path.abspath(DB_PATH) + "?mode=ro", uri=True)


def safe_json(raw):
    try:
        value = json.loads(raw or "{}")
        return value if isinstance(value, dict) else {}
    except json.JSONDecodeError:
        return {}


def snapshot(device_id):
    conn = open_db()
    try:
        row = conn.execute(
            """
            SELECT COALESCE(d.project_id,''), d.client_version,
                   r.connection_state, r.readiness, r.last_seen_at_us,
                   r.observed_state_json
            FROM stage_devices d
            LEFT JOIN stage_device_runtime_state r ON r.device_id=d.device_id
            WHERE d.device_id=?
            """,
            (device_id,),
        ).fetchone()
    finally:
        conn.close()
    if row is None:
        fail("lighting device is absent from StageCore runtime state", 3)
    return {
        "project_id": row[0],
        "client_version": row[1],
        "connection_state": row[2],
        "readiness": row[3],
        "last_seen_at_us": row[4],
        "observed": safe_json(row[5]),
    }


def normalized_levels(observed):
    levels = observed.get("current_levels")
    if not isinstance(levels, dict) or not levels:
        fail("current_levels missing from observation", 3)
    result = {}
    for key, value in levels.items():
        if isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(float(value)):
            fail(f"invalid current level for {key}")
        result[str(key)] = float(value)
    return result


def validate_snapshot(value, *, project_id, channel_key, expected_level, baseline=None):
    observed = value.get("observed") or {}
    if value.get("project_id") != project_id:
        fail("project identity changed during DMX stability window")
    if value.get("connection_state") != "ONLINE":
        fail(f"device connection_state={value.get('connection_state')}")
    if value.get("readiness") not in {"READY", "WARNING"}:
        fail(f"device readiness={value.get('readiness')}")
    if observed.get("dmx_healthy") is not True:
        fail("DMX health became false during stability window")
    if observed.get("authority") != "STAGECORE":
        fail(f"authority changed to {observed.get('authority')}")
    config_hash = str(observed.get("configuration_hash") or "")
    if not config_hash:
        fail("configuration_hash missing")
    levels = normalized_levels(observed)
    if channel_key not in levels:
        fail("qualification channel missing from current_levels")
    if abs(levels[channel_key] - expected_level) > 0.01:
        fail(f"qualification level changed: actual={levels[channel_key]} expected={expected_level}")
    if observed.get("active_fade") not in (None, {}, False):
        fail("active fade appeared during fixed-level stability window")
    uptime = observed.get("uptime_seconds")
    if isinstance(uptime, bool) or not isinstance(uptime, (int, float)) or not math.isfinite(float(uptime)):
        fail("uptime_seconds missing or invalid")
    reset_reason = str(observed.get("reset_reason") or "")
    if not reset_reason:
        fail("reset_reason missing")
    if baseline is not None:
        base_observed = baseline.get("observed") or {}
        if value.get("client_version") != baseline.get("client_version"):
            fail("firmware client version changed during stability window")
        if config_hash != str(base_observed.get("configuration_hash") or ""):
            fail("configuration hash changed during stability window")
        base_uptime = base_observed.get("uptime_seconds")
        if float(uptime) + 1 < float(base_uptime):
            fail("ESP32 rebooted during stability window")
        if reset_reason != str(base_observed.get("reset_reason") or ""):
            fail("reset_reason changed during stability window")
    return {
        "client_version": value.get("client_version"),
        "readiness": value.get("readiness"),
        "last_seen_at_us": value.get("last_seen_at_us"),
        "configuration_hash": config_hash,
        "uptime_seconds": float(uptime),
        "reset_reason": reset_reason,
        "levels": levels,
        "dmx_healthy": True,
        "authority": "STAGECORE",
    }


def completed(result, command_type):
    if result.get("status") != "COMPLETED":
        fail(f"{command_type} status={result.get('status')}")
    statuses = result.get("statuses")
    if not isinstance(statuses, list) or statuses[-1:] != ["COMPLETED"]:
        fail(f"{command_type} lifecycle is not terminal COMPLETED")


def main():
    global DB_PATH
    parser = argparse.ArgumentParser(description="Bounded real-node network/DMX stability stress helper")
    parser.add_argument("--db", default=DB_PATH)
    args = parser.parse_args()
    DB_PATH = args.db

    data = read_request()
    device_id = bounded_text(data.get("device_id"), "device_id")
    project_id = bounded_text(data.get("project_id"), "project_id")
    channel_key = bounded_text(data.get("channel_key"), "channel_key", 64)
    expected_level = finite_level(data.get("expected_level"), "expected_level")
    duration_seconds = data.get("duration_seconds")
    interval_ms = data.get("interval_ms")
    if isinstance(duration_seconds, bool) or not isinstance(duration_seconds, int) or duration_seconds < 1 or duration_seconds > 300:
        fail("duration_seconds must be within 1..300")
    if isinstance(interval_ms, bool) or not isinstance(interval_ms, int) or interval_ms < 100 or interval_ms > 5000:
        fail("interval_ms must be within 100..5000")

    before_raw = snapshot(device_id)
    before = validate_snapshot(before_raw, project_id=project_id, channel_key=channel_key, expected_level=expected_level)

    started_wall = dt.datetime.now(dt.timezone.utc)
    started = time.monotonic()
    deadline = started + duration_seconds
    samples = []
    state_reads = 0
    config_reads = 0
    while time.monotonic() < deadline:
        index = len(samples)
        command_type = "LIGHTING_STATE_READ" if index % 2 == 0 else "LIGHTING_CONFIG_READ"
        command_id = "qualification-stability-" + uuid.uuid4().hex[:20]
        result, latency_ms = exchange(device_id, envelope(command_id, command_type, project_id))
        completed(result, command_type)
        if command_type == "LIGHTING_STATE_READ":
            state_reads += 1
            payload = result.get("payload") or {}
            levels = payload.get("current_levels") if isinstance(payload, dict) else None
            current = levels.get(channel_key) if isinstance(levels, dict) else None
            if isinstance(current, bool) or not isinstance(current, (int, float)):
                fail("state-read payload missing qualification channel")
            if abs(float(current) - expected_level) > 0.01:
                fail("state-read payload reports changed qualification level")
        else:
            config_reads += 1

        live = validate_snapshot(
            snapshot(device_id),
            project_id=project_id,
            channel_key=channel_key,
            expected_level=expected_level,
            baseline=before_raw,
        )
        samples.append({
            "index": index,
            "command_type": command_type,
            "latency_ms": round(latency_ms, 3),
            "readiness": live["readiness"],
            "uptime_seconds": live["uptime_seconds"],
        })
        remaining = deadline - time.monotonic()
        if remaining > 0:
            time.sleep(min(interval_ms / 1000.0, remaining))

    after_raw = snapshot(device_id)
    after = validate_snapshot(after_raw, project_id=project_id, channel_key=channel_key, expected_level=expected_level, baseline=before_raw)
    if not samples:
        fail("stability window produced no samples")
    latencies = [item["latency_ms"] for item in samples]
    result = {
        "status": "PASS",
        "operation": "DMX_STABILITY_NETWORK_STRESS",
        "device_id": device_id,
        "project_id": project_id,
        "channel_key": channel_key,
        "expected_level": expected_level,
        "duration_seconds_requested": duration_seconds,
        "duration_seconds_observed": round(time.monotonic() - started, 3),
        "interval_ms": interval_ms,
        "sample_count": len(samples),
        "state_read_count": state_reads,
        "config_read_count": config_reads,
        "command_failures": 0,
        "max_command_latency_ms": max(latencies),
        "average_command_latency_ms": round(sum(latencies) / len(latencies), 3),
        "started_at": started_wall.isoformat(),
        "completed_at": dt.datetime.now(dt.timezone.utc).isoformat(),
        "before": before,
        "after": after,
        "samples": samples,
    }
    print(json.dumps(result, sort_keys=True))


if __name__ == "__main__":
    main()
