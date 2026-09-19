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
                fail(f"unredacted sensitive evidence field at {path}.{key}")
            walk(item, f"{path}.{key}")
    elif isinstance(value, list):
        for index, item in enumerate(value):
            walk(item, f"{path}[{index}]")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", required=True)
    parser.add_argument("--command", required=True)
    parser.add_argument("--device-id", required=True)
    args = parser.parse_args()

    try:
        data = json.load(open(args.input, encoding="utf-8"))
    except Exception as exc:
        fail(f"invalid JSON evidence: {exc}")

    if not isinstance(data, dict):
        fail("command evidence must be an object")
    walk(data)
    if data.get("status") != "COMPLETED":
        fail(f"terminal status is not COMPLETED: {data.get('status')}")
    if data.get("qualification_command") != args.command:
        fail("qualification command mismatch")
    if data.get("command_type") not in (None, args.command):
        fail("persisted command type mismatch")
    if data.get("device_id") != args.device_id:
        fail("device id mismatch")
    command_id = str(data.get("command_id") or "").strip()
    if not command_id:
        fail("command id missing")
    print(json.dumps({
        "status": "PASS",
        "command_id": command_id,
        "command": args.command,
        "device_id": args.device_id,
    }, sort_keys=True))


if __name__ == "__main__":
    main()
