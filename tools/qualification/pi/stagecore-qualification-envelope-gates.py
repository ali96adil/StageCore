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


def expect_rejected(value, label, error_code):
    if value.get("status") != "REJECTED" or value.get("statuses") != ["REJECTED"]:
        fail(f"{label} status={value.get('status')} statuses={value.get('statuses')}")
    error = value.get("error") or {}
    if error.get("error_code") != error_code:
        fail(f"{label} error_code={error.get('error_code')}")
    return error


def numeric_level(value, label):
    if isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(float(value)):
        fail(f"{label} is not a finite numeric level")
    return float(value)


def result_level(value, channel_key, field, label):
    payload = value.get("payload") or {}
    levels = payload.get(field) if isinstance(payload, dict) else None
    current = levels.get(channel_key) if isinstance(levels, dict) else None
    return numeric_level(current, label)


def assert_level(actual, expected, label):
    if abs(float(actual) - float(expected)) > 0.01:
        fail(f"{label}: actual={actual} expected={expected}")


def read_state(device_id, project_id, command_id, channel_key):
    now = dt.datetime.now(dt.timezone.utc)
    result = exchange(device_id, command(
        command_id, "LIGHTING_STATE_READ", project_id, {},
        now, now + dt.timedelta(seconds=15),
    ))
    expect_completed(result, "state read")
    return result, result_level(result, channel_key, "current_levels", "state level")


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
    baseline = result_level(pre_result, channel_key, "levels", "expired precondition level")

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

    _, current = read_state(device_id, project_id, prefix + "-state", channel_key)
    assert_level(current, baseline, "expired command affected output")

    return {
        "status": "PASS",
        "mode": "expired",
        "command_id": prefix,
        "terminal_status": rejected.get("status"),
        "error_code": error.get("error_code"),
        "expected_level": baseline,
        "observed_level": current,
    }


def invalid_value_gate(device_id, project_id, channel_key, start_level):
    prefix = "qualification-invalid-" + uuid.uuid4().hex[:12]

    now = dt.datetime.now(dt.timezone.utc)
    config_result = exchange(device_id, command(
        prefix + "-config", "LIGHTING_CONFIG_READ", project_id, {},
        now, now + dt.timedelta(seconds=15),
    ))
    expect_completed(config_result, "configuration read")
    config = config_result.get("payload") or {}
    channels = config.get("channels") if isinstance(config, dict) else None
    if not isinstance(channels, list):
        fail("configuration read did not return channels")

    selected = None
    existing_keys = set()
    for item in channels:
        if not isinstance(item, dict):
            continue
        key = str(item.get("channel_key") or "")
        if key:
            existing_keys.add(key)
        if key == channel_key:
            selected = item
    if selected is None:
        fail("configured qualification channel is absent from device configuration", 3)

    minimum = numeric_level(selected.get("minimum_level"), "minimum_level")
    maximum = numeric_level(selected.get("maximum_level"), "maximum_level")
    if minimum < 0 or maximum > 100 or minimum > maximum:
        fail("device configuration returned invalid channel bounds")

    now = dt.datetime.now(dt.timezone.utc)
    pre_result = exchange(device_id, command(
        prefix + "-pre", "LIGHTING_CHANNELS_SET", project_id,
        {"channels": {channel_key: start_level}}, now, now + dt.timedelta(seconds=15),
    ))
    expect_completed(pre_result, "invalid-value precondition")
    baseline = result_level(pre_result, channel_key, "levels", "invalid-value baseline")
    state_before, observed_before = read_state(
        device_id, project_id, prefix + "-state-before", channel_key
    )
    assert_level(observed_before, baseline, "precondition state mismatch")
    before_payload = state_before.get("payload") or {}
    config_hash_before = str(before_payload.get("configuration_hash") or "")
    if not config_hash_before:
        fail("state read did not expose configuration_hash")

    now = dt.datetime.now(dt.timezone.utc)
    invalid_result = exchange(device_id, command(
        prefix + "-oor", "LIGHTING_CHANNELS_SET", project_id,
        {"channels": {channel_key: 101.0}}, now, now + dt.timedelta(seconds=15),
    ))
    invalid_error = expect_rejected(
        invalid_result, "out-of-range value", "DEVICE_COMMAND_INVALID"
    )
    _, after_invalid = read_state(
        device_id, project_id, prefix + "-state-oor", channel_key
    )
    assert_level(after_invalid, baseline, "out-of-range value changed output")

    missing_key = "qualification_missing_channel"
    suffix = 1
    while missing_key in existing_keys:
        missing_key = f"qualification_missing_{suffix}"
        suffix += 1
    now = dt.datetime.now(dt.timezone.utc)
    missing_result = exchange(device_id, command(
        prefix + "-missing", "LIGHTING_CHANNELS_SET", project_id,
        {"channels": {missing_key: 50.0}}, now, now + dt.timedelta(seconds=15),
    ))
    missing_error = expect_rejected(
        missing_result, "unknown channel", "CHANNEL_LEVEL_INVALID"
    )
    _, after_missing = read_state(
        device_id, project_id, prefix + "-state-missing", channel_key
    )
    assert_level(after_missing, baseline, "unknown channel changed output")

    clamp = {"performed": False, "reason": "full_range_channel"}
    if maximum < 100.0 - 0.01:
        requested = 100.0
        expected = maximum
    elif minimum > 0.01:
        requested = 0.0
        expected = minimum
    else:
        requested = None
        expected = None

    if requested is not None:
        now = dt.datetime.now(dt.timezone.utc)
        clamp_result = exchange(device_id, command(
            prefix + "-clamp", "LIGHTING_CHANNELS_SET", project_id,
            {"channels": {channel_key: requested}}, now, now + dt.timedelta(seconds=15),
        ))
        expect_completed(clamp_result, "configured-bound clamp")
        normalized = result_level(
            clamp_result, channel_key, "levels", "configured-bound normalized level"
        )
        assert_level(normalized, expected, "configured bound was not clamped")
        _, observed_clamp = read_state(
            device_id, project_id, prefix + "-state-clamp", channel_key
        )
        assert_level(observed_clamp, expected, "clamped level was not observed")
        clamp = {
            "performed": True,
            "requested_level": requested,
            "expected_level": expected,
            "observed_level": observed_clamp,
        }

    now = dt.datetime.now(dt.timezone.utc)
    restore_result = exchange(device_id, command(
        prefix + "-restore", "LIGHTING_CHANNELS_SET", project_id,
        {"channels": {channel_key: baseline}}, now, now + dt.timedelta(seconds=15),
    ))
    expect_completed(restore_result, "invalid-value restore")
    restored = result_level(restore_result, channel_key, "levels", "restored level")
    assert_level(restored, baseline, "restore result mismatch")
    final_state, final_level = read_state(
        device_id, project_id, prefix + "-state-final", channel_key
    )
    assert_level(final_level, baseline, "final state was not restored")
    final_payload = final_state.get("payload") or {}
    config_hash_after = str(final_payload.get("configuration_hash") or "")
    if config_hash_after != config_hash_before:
        fail("configuration hash changed during invalid-value/clamp gate")

    return {
        "status": "PASS",
        "mode": "invalid-value",
        "command_id": prefix,
        "channel_key": channel_key,
        "configuration_hash_before": config_hash_before,
        "configuration_hash_after": config_hash_after,
        "minimum_level": minimum,
        "maximum_level": maximum,
        "baseline_level": baseline,
        "invalid_value_status": invalid_result.get("status"),
        "invalid_value_error_code": invalid_error.get("error_code"),
        "after_invalid_level": after_invalid,
        "unknown_channel_status": missing_result.get("status"),
        "unknown_channel_error_code": missing_error.get("error_code"),
        "after_unknown_level": after_missing,
        "clamp": clamp,
        "restored_level": final_level,
    }


def main():
    data = read_request()
    mode = text(data.get("mode"), "mode", 32)
    device_id = text(data.get("device_id"), "device_id")
    project_id = text(data.get("project_id"), "project_id")
    channel_key = text(data.get("channel_key"), "channel_key", 64)
    first = level(data.get("start_level"), "start_level")
    second = level(data.get("target_level"), "target_level")

    if mode == "duplicate":
        fade_ms = data.get("fade_ms")
        if isinstance(fade_ms, bool) or not isinstance(fade_ms, int) or fade_ms < 1000 or fade_ms > 120000:
            fail("fade_ms must be 1000..120000 for duplicate mode")
        result = duplicate_gate(device_id, project_id, channel_key, first, second, fade_ms)
    elif mode == "expired":
        result = expired_gate(device_id, project_id, channel_key, second, first)
    elif mode == "invalid-value":
        result = invalid_value_gate(device_id, project_id, channel_key, first)
    else:
        fail("unsupported envelope-gate mode")
    print(json.dumps(result, sort_keys=True))


if __name__ == "__main__":
    main()
