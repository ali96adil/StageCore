#!/usr/bin/env python3
import argparse
import json
import re
import sys

SENSITIVE = re.compile(r"(password|secret|token|cookie|authorization|csrf|api[_-]?key)", re.I)
NON_SUCCESS_TERMINAL = {"FAILED", "TIMED_OUT", "CANCELLED"}


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


def levels_are_blackout(value):
    return isinstance(value, dict) and bool(value) and all(
        isinstance(level, (int, float)) and not isinstance(level, bool) and abs(float(level)) <= 0.01
        for level in value.values()
    )


def main():
    parser = argparse.ArgumentParser(description="Validate Q-DMX-17 Hub-restart evidence")
    parser.add_argument("--input", required=True)
    parser.add_argument("--device-id", required=True)
    args = parser.parse_args()
    try:
        data = json.load(open(args.input, encoding="utf-8"))
    except Exception as exc:
        fail(f"invalid Hub-restart evidence: {exc}")
    if not isinstance(data, dict):
        fail("Hub-restart evidence must be an object")
    walk(data)
    if data.get("status") != "PASS" or data.get("operation") != "HUB_RESTART_NO_REPLAY":
        fail("Hub-restart evidence did not pass")
    if data.get("device_id") != args.device_id:
        fail("Hub-restart device mismatch")
    if data.get("active_before_restart") is not True:
        fail("target fade was not proven active before restart")
    if data.get("command_status_before_restart") != "ACCEPTED":
        fail("interrupted command was not ACCEPTED before restart")
    if data.get("command_status_after_restart") not in NON_SUCCESS_TERMINAL:
        fail("interrupted command was not terminalized as non-success")
    if data.get("command_row_count") != 1:
        fail("interrupted command row count changed")
    for key in (
        "hub_ready_after_restart", "reconnected", "same_firmware",
        "same_configuration_hash", "no_esp_reboot", "no_stale_replay",
        "safe_blackout_after_reconnect", "post_dmx_healthy",
    ):
        if data.get(key) is not True:
            fail(f"{key} is not true")
    if data.get("post_authority") != "STAGECORE":
        fail("post-restart authority is not STAGECORE")
    if data.get("post_readiness") not in {"READY", "WARNING"}:
        fail("post-restart readiness is not READY/WARNING")
    if not levels_are_blackout(data.get("post_levels")):
        fail("post-reconnect levels are not blackout")
    if not levels_are_blackout(data.get("stable_levels_after_hold")):
        fail("safe blackout did not remain stable after reconnect hold")
    interrupted = str(data.get("interrupted_command_id") or "")
    precondition = str(data.get("precondition_command_id") or "")
    if not interrupted or not precondition or interrupted == precondition:
        fail("command identities are missing or not distinct")
    print(json.dumps({
        "status": "PASS",
        "interrupted_command_id": interrupted,
        "terminal_status": data.get("command_status_after_restart"),
    }, sort_keys=True))


if __name__ == "__main__":
    main()
