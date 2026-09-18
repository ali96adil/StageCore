#!/usr/bin/env python3
import argparse
import json
import math
import re
import sys

SENSITIVE = re.compile(r"(password|secret|token|cookie|authorization|csrf|api[_-]?key)", re.I)


def fail(message):
    print(message, file=sys.stderr)
    raise SystemExit(1)


def walk(value, path="$"):
    if isinstance(value, dict):
        for key, item in value.items():
            if SENSITIVE.search(str(key)) and item != "[REDACTED]":
                fail(f"unredacted sensitive field at {path}.{key}")
            walk(item, f"{path}.{key}")
    elif isinstance(value, list):
        for index, item in enumerate(value):
            walk(item, f"{path}[{index}]")


def finite(value, name):
    if isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(float(value)):
        fail(f"{name} is not finite")
    return float(value)


def main():
    parser = argparse.ArgumentParser(description="Validate Q-DMX-18 DMX stability evidence")
    parser.add_argument("--input", required=True)
    parser.add_argument("--device-id", required=True)
    parser.add_argument("--expected-level", required=True, type=float)
    parser.add_argument("--min-duration-seconds", required=True, type=float)
    args = parser.parse_args()

    data = json.load(open(args.input, encoding="utf-8"))
    if not isinstance(data, dict):
        fail("DMX stability evidence must be an object")
    walk(data)
    if data.get("status") != "PASS" or data.get("operation") != "DMX_STABILITY_NETWORK_STRESS":
        fail("DMX stability evidence did not pass")
    if data.get("device_id") != args.device_id:
        fail("DMX stability device mismatch")
    if data.get("command_failures") != 0:
        fail("network stress recorded command failures")
    if int(data.get("sample_count") or 0) < 2:
        fail("insufficient network stress samples")
    if int(data.get("state_read_count") or 0) < 1 or int(data.get("config_read_count") or 0) < 1:
        fail("state/config network activity was not both exercised")
    duration = finite(data.get("duration_seconds_observed"), "duration_seconds_observed")
    if duration + 0.05 < args.min_duration_seconds:
        fail("network stress window shorter than configured duration")
    if abs(finite(data.get("expected_level"), "expected_level") - args.expected_level) > 0.01:
        fail("expected level mismatch")

    before = data.get("before") or {}
    after = data.get("after") or {}
    if before.get("configuration_hash") != after.get("configuration_hash") or not before.get("configuration_hash"):
        fail("configuration hash changed")
    if before.get("client_version") != after.get("client_version"):
        fail("firmware changed")
    if before.get("reset_reason") != after.get("reset_reason"):
        fail("reset reason changed")
    if finite(after.get("uptime_seconds"), "after.uptime_seconds") + 1 < finite(before.get("uptime_seconds"), "before.uptime_seconds"):
        fail("ESP32 rebooted during stability window")
    for label, state in (("before", before), ("after", after)):
        if state.get("dmx_healthy") is not True:
            fail(f"{label} DMX health is not true")
        if state.get("authority") != "STAGECORE":
            fail(f"{label} authority is not STAGECORE")
        levels = state.get("levels")
        if not isinstance(levels, dict) or data.get("channel_key") not in levels:
            fail(f"{label} qualification level missing")
        if abs(finite(levels[data["channel_key"]], f"{label}.level") - args.expected_level) > 0.01:
            fail(f"{label} qualification level changed")
    print(json.dumps({
        "status": "PASS",
        "sample_count": data.get("sample_count"),
        "duration_seconds": duration,
        "max_command_latency_ms": data.get("max_command_latency_ms"),
    }, sort_keys=True))


if __name__ == "__main__":
    main()
