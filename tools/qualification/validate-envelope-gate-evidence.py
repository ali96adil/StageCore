#!/usr/bin/env python3
import argparse
import json
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


def close(a, b):
    return abs(float(a) - float(b)) <= 0.01


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", required=True)
    parser.add_argument("--mode", choices=("duplicate", "expired", "invalid-value"), required=True)
    args = parser.parse_args()
    data = json.load(open(args.input, encoding="utf-8"))
    if not isinstance(data, dict):
        fail("evidence must be an object")
    walk(data)
    if data.get("status") != "PASS" or data.get("mode") != args.mode:
        fail("envelope gate did not pass")
    if not str(data.get("command_id") or "").startswith("qualification-"):
        fail("qualification command id missing")

    if args.mode == "duplicate":
        if "ACCEPTED" not in data.get("first_statuses", []):
            fail("first fade acceptance missing")
        if data.get("duplicate_statuses") != ["COMPLETED"]:
            fail("duplicate was not terminal-cache only")
        if data.get("first_payload") != data.get("duplicate_payload"):
            fail("duplicate result payload mismatch")
    elif args.mode == "expired":
        if data.get("terminal_status") != "TIMED_OUT":
            fail("expired command was not TIMED_OUT")
        if data.get("error_code") != "DEVICE_COMMAND_EXPIRED":
            fail("expired command error code mismatch")
        if not close(data.get("expected_level"), data.get("observed_level")):
            fail("expired command affected output")
    else:
        baseline = data.get("baseline_level")
        if data.get("invalid_value_status") != "REJECTED" or data.get("invalid_value_error_code") != "DEVICE_COMMAND_INVALID":
            fail("out-of-range value did not fail closed")
        if data.get("unknown_channel_status") != "REJECTED" or data.get("unknown_channel_error_code") != "CHANNEL_LEVEL_INVALID":
            fail("unknown channel did not fail closed")
        if not close(baseline, data.get("after_invalid_level")):
            fail("out-of-range rejection changed output")
        if not close(baseline, data.get("after_unknown_level")):
            fail("unknown-channel rejection changed output")
        if not close(baseline, data.get("restored_level")):
            fail("qualification output was not restored")
        before = str(data.get("configuration_hash_before") or "")
        after = str(data.get("configuration_hash_after") or "")
        if not before or before != after:
            fail("configuration hash changed during invalid-value gate")
        minimum = float(data.get("minimum_level"))
        maximum = float(data.get("maximum_level"))
        if minimum < 0 or maximum > 100 or minimum > maximum:
            fail("invalid configuration bounds in evidence")
        clamp = data.get("clamp") or {}
        if clamp.get("performed") is True:
            if not close(clamp.get("expected_level"), clamp.get("observed_level")):
                fail("configured-bound clamp evidence mismatch")
        elif clamp.get("reason") != "full_range_channel":
            fail("invalid clamp evidence")

    print(json.dumps({"status":"PASS","mode":args.mode,"command_id":data.get("command_id")}, sort_keys=True))


if __name__ == "__main__":
    main()
