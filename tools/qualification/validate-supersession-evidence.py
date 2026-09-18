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
    parser = argparse.ArgumentParser(description="Validate StageCore fade supersession evidence")
    parser.add_argument("--input", required=True)
    parser.add_argument("--device-id", required=True)
    args = parser.parse_args()
    try:
        data = json.load(open(args.input, encoding="utf-8"))
    except Exception as exc:
        fail(f"invalid supersession evidence: {exc}")
    if not isinstance(data, dict):
        fail("supersession evidence must be an object")
    walk(data)
    if data.get("operation") != "LIGHTING_FADE_SUPERSESSION":
        fail("operation mismatch")
    if data.get("device_id") != args.device_id:
        fail("device mismatch")
    if data.get("status") != "PASS":
        fail(f"supersession did not pass: {data.get('status')}")
    if data.get("active_fade_seen") is not True:
        fail("original fade was never observed active")
    if data.get("final_observation_seen") is not True:
        fail("replacement observation was not confirmed")
    first = data.get("superseded") or {}
    second = data.get("replacement") or {}
    first_id = str(first.get("command_id") or "")
    second_id = str(second.get("command_id") or "")
    if not first_id or not second_id or first_id == second_id:
        fail("supersession command IDs are missing or not distinct")
    if first.get("status") != "CANCELLED":
        fail(f"superseded fade status is {first.get('status')}, expected CANCELLED")
    if second.get("status") != "COMPLETED":
        fail(f"replacement status is {second.get('status')}, expected COMPLETED")
    final = data.get("final_observation") or {}
    if final.get("last_accepted_command_id") != second_id:
        fail("final last_accepted_command_id does not match replacement")
    if final.get("last_applied_command_id") != second_id:
        fail("final last_applied_command_id does not match replacement")
    if final.get("active_fade_command_id") == first_id:
        fail("superseded fade remained active")
    print(json.dumps({
        "status": "PASS",
        "superseded_command_id": first_id,
        "replacement_command_id": second_id,
    }, sort_keys=True))


if __name__ == "__main__":
    main()
