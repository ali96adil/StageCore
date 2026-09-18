#!/usr/bin/env python3
import datetime as dt
import http.client
import json
import math
import os
import socket
import sys
import uuid

SOCKET_PATH = os.environ.get(
    "STAGECORE_QUALIFICATION_SOCKET",
    "/var/lib/stagecore/qualification-envelope.sock",
)


class UnixHTTPConnection(http.client.HTTPConnection):
    def __init__(self, path):
        super().__init__("localhost", timeout=145)
        self.path = path

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(self.timeout)
        self.sock.connect(self.path)


def fail(message, code=1):
    print(json.dumps({"status": "FAIL" if code == 1 else "BLOCKED", "detail": message}, sort_keys=True))
    raise SystemExit(code)


def read_request():
    raw = sys.stdin.buffer.read(32769)
    if len(raw) > 32768:
        fail("envelope-gate request too large")
    try:
        value = json.loads(raw.decode("utf-8"))
    except Exception:
        fail("invalid envelope-gate request")
    if not isinstance(value, dict):
        fail("envelope-gate request must be an object")
    return value


def text(value, name, maximum=256):
    value = str(value or "").strip()
    if not value or len(value) > maximum or any(ord(ch) < 32 for ch in value):
        fail(f"invalid {name}")
    return value


def level(value, name):
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        fail(f"{name} must be numeric")
    value = float(value)
    if not math.isfinite(value) or value < 0 or value > 100:
        fail(f"{name} must be 0..100")
    return value


def rfc3339(value):
    return value.astimezone(dt.timezone.utc).isoformat().replace("+00:00", "Z")


def command(command_id, command_type, project_id, payload, issued_at, deadline_at):
    return {
        "command_id": command_id,
        "command_type": command_type,
        "schema_version": 1,
        "issued_at": rfc3339(issued_at),
        "deadline_at": rfc3339(deadline_at),
        "project_id": project_id,
        "runtime_snapshot_id": "",
        "issuer": "qualification:physical-runner",
        "correlation_id": command_id,
        "causation_id": "",
        "priority": "P2",
        "idempotency_key": "",
        "payload": payload,
    }


def exchange(device_id, envelope):
    if not os.path.exists(SOCKET_PATH):
        fail("qualification envelope socket is unavailable", 3)
    body = json.dumps({"device_id": device_id, "command": envelope}, separators=(",", ":"))
    conn = UnixHTTPConnection(SOCKET_PATH)
    try:
        conn.request("POST", "/v1/device-envelope", body=body, headers={"Content-Type": "application/json"})
        response = conn.getresponse()
        raw = response.read(1 << 20)
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
    return value


def expect_completed(value, label):
    if value.get("status") != "COMPLETED":
        fail(f"{label} status={value.get('status')}")


def duplicate_gate(device_id, project_id, channel_key, start_level, target_level, fade_ms):
    if start_level == target_level:
        fail("duplicate gate requires distinct start/target levels", 3)
    now = dt.datetime.now(dt.timezone.utc)
    prefix = "qualification-dup-" + uuid.uuid4().hex[:16]
    pre = command(
        prefix + "-pre", "LIGHTING_CHANNELS_SET", project_id,
        {"channels": {channel_key: start_level}}, now, now + dt.timedelta(seconds=15),
    )
    pre_result = exchange(device_id, pre)
    expect_completed(pre_result, "duplicate precondition")

    now = dt.datetime.now(dt.timezone.utc)
    duplicate = command(
        prefix, "LIGHTING_CHANNELS_FADE", project_id,
        {"channels": {channel_key: target_level}, "fade_ms": fade_ms},
        now, now + dt.timedelta(milliseconds=fade_ms + 15000),
    )
    first = exchange(device_id, duplicate)
    expect_completed(first, "first duplicate command")
    second = exchange(device_id, duplicate)
    expect_completed(second, "second duplicate command")

    if "ACCEPTED" not in first.get("statuses", []):
        fail("first fade never reported ACCEPTED")
    if second.get("statuses") != ["COMPLETED"]:
        fail(f"duplicate was not served as cached terminal result: {second.get('statuses')}")
    if first.get("payload") != second.get("payload"):
        fail("duplicate cached result payload differs from first terminal result")

    return {
        "status": "PASS",
        "mode": "duplicate",
        "command_id": prefix,
        "first_statuses": first.get("statuses", []),
        "duplicate_statuses": second.get("statuses", []),
        "first_payload": first.get("payload"),
        "duplicate_payload": second.get("payload"),
    }


def expired_gate(device_id, project_id, channel_key, expected_level, rejected_level):
    if expected_level == rejected_level:
        fail("expired gate requires distinct expected/rejected levels", 3)
    prefix = "qualification-exp-" + uuid.uuid4().hex[:16]
    now = dt.datetime.now(dt.timezone.utc)
    pre = command(
        prefix + "-pre", "LIGHTING_CHANNELS_SET", project_id,
        {"channels": {channel_key: expected_level}}, now, now + dt.timedelta(seconds=15),
    )
    pre_result = exchange(device_id, pre)
    expect_completed(pre_result, "expired precondition")

    now = dt.datetime.now(dt.timezone.utc)
    expired = command(
        prefix, "LIGHTING_CHANNELS_SET", project_id,
        {"channels": {channel_key: rejected_level}},
        now - dt.timedelta(seconds=10), now - dt.timedelta(seconds=5),
    )
    rejected = exchange(device_id, expired)
    if rejected.get("status") != "TIMED_OUT" or rejected.get("statuses") != ["TIMED_OUT"]:
        fail(f"expired command result={rejected.get('status')} statuses={rejected.get('statuses')}")
    error = rejected.get("error") or {}
    if error.get("error_code") != "DEVICE_COMMAND_EXPIRED":
        fail(f"expired command error_code={error.get('error_code')}")

    now = dt.datetime.now(dt.timezone.utc)
    state_cmd = command(
        prefix + "-state", "LIGHTING_STATE_READ", project_id, {},
        now, now + dt.timedelta(seconds=15),
    )
    state_result = exchange(device_id, state_cmd)
    expect_completed(state_result, "expired state read")
    payload = state_result.get("payload") or {}
    levels = payload.get("current_levels") if isinstance(payload, dict) else None
    current = levels.get(channel_key) if isinstance(levels, dict) else None
    if isinstance(current, bool) or not isinstance(current, (int, float)) or abs(float(current) - expected_level) > 0.01:
        fail(f"expired command affected output: current={current} expected={expected_level}")

    return {
        "status": "PASS",
        "mode": "expired",
        "command_id": prefix,
        "terminal_status": rejected.get("status"),
        "error_code": error.get("error_code"),
        "expected_level": expected_level,
        "observed_level": current,
    }


def main():
    data = read_request()
    mode = text(data.get("mode"), "mode", 32)
    device_id = text(data.get("device_id"), "device_id")
    project_id = text(data.get("project_id"), "project_id")
    channel_key = text(data.get("channel_key"), "channel_key", 64)
    first = level(data.get("start_level"), "start_level")
    second = level(data.get("target_level"), "target_level")
    fade_ms = data.get("fade_ms")
    if isinstance(fade_ms, bool) or not isinstance(fade_ms, int) or fade_ms < 1000 or fade_ms > 120000:
        fail("fade_ms must be 1000..120000")

    if mode == "duplicate":
        result = duplicate_gate(device_id, project_id, channel_key, first, second, fade_ms)
    elif mode == "expired":
        result = expired_gate(device_id, project_id, channel_key, second, first)
    else:
        fail("unsupported envelope-gate mode")
    print(json.dumps(result, sort_keys=True))


if __name__ == "__main__":
    main()
