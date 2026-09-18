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


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", required=True)
    parser.add_argument("--mode", choices=("duplicate", "expired"), required=True)
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
    else:
        if data.get("terminal_status") != "TIMED_OUT":
            fail("expired command was not TIMED_OUT")
        if data.get("error_code") != "DEVICE_COMMAND_EXPIRED":
            fail("expired command error code mismatch")
        if abs(float(data.get("expected_level")) - float(data.get("observed_level"))) > 0.01:
            fail("expired command affected output")
    print(json.dumps({"status":"PASS","mode":args.mode,"command_id":data.get("command_id")}, sort_keys=True))


if __name__ == "__main__":
    main()
