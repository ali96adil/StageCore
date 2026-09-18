#!/usr/bin/env python3
import argparse
import datetime as dt
import json
import math
import os
import tempfile
from pathlib import Path

LIGHTING_PROFILE = "stagecore.esp32-dmx-lighting-node"


def fail(message, code=1):
    print(message)
    raise SystemExit(code)


def parse_time(value, label):
    value = str(value or "").strip()
    if not value:
        fail(f"{label} timestamp missing")
    try:
        return dt.datetime.fromisoformat(value.replace("Z", "+00:00")).astimezone(dt.timezone.utc)
    except ValueError:
        fail(f"{label} timestamp invalid")


def finite_number(value, label):
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        fail(f"{label} must be numeric")
    value = float(value)
    if not math.isfinite(value):
        fail(f"{label} must be finite")
    return value


def atomic_write(path, value):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, temp_path = tempfile.mkstemp(prefix=path.name + ".", dir=str(path.parent))
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as fh:
            json.dump(value, fh, indent=2, sort_keys=True)
            fh.write("\n")
            fh.flush()
            os.fsync(fh.fileno())
        os.chmod(temp_path, 0o600)
        os.replace(temp_path, path)
    finally:
        if os.path.exists(temp_path):
            os.unlink(temp_path)


def select_lighting(data, device_id="", project_id=""):
    devices = [
        d for d in data.get("devices", [])
        if d.get("profile_id") == LIGHTING_PROFILE
    ]
    if device_id:
        devices = [d for d in devices if d.get("device_id") == device_id]
    if project_id:
        devices = [d for d in devices if d.get("project_id") == project_id]
    if not devices:
        fail("lighting target not present in probe", 3)
    if len(devices) != 1:
        fail("lighting target is ambiguous", 3)
    return devices[0]


def snapshot_from_probe(data, device):
    runtime = device.get("runtime")
    if not isinstance(runtime, dict):
        fail("lighting runtime observation missing", 3)
    observed = runtime.get("observed")
    if not isinstance(observed, dict):
        fail("lighting observed state missing", 3)
    required = (
        "uptime_seconds", "reset_reason", "current_levels",
        "dmx_healthy", "configuration_hash", "brownout_warning", "authority",
    )
    missing = [key for key in required if key not in observed]
    if missing:
        fail("lighting observation missing: " + ",".join(missing), 3)
    uptime = finite_number(observed.get("uptime_seconds"), "uptime_seconds")
    if uptime < 0:
        fail("uptime_seconds must be non-negative")
    if not isinstance(observed.get("current_levels"), dict) or not observed["current_levels"]:
        fail("current_levels missing or empty", 3)
    return {
        "schema_version": 1,
        "probe_generated_at": data.get("generated_at"),
        "device_id": device.get("device_id"),
        "project_id": device.get("project_id"),
        "client_version": device.get("client_version"),
        "connection_state": runtime.get("connection_state"),
        "readiness": runtime.get("readiness"),
        "last_seen_at_us": runtime.get("last_seen_at_us"),
        "observed": observed,
    }


def cmd_capture(args):
    data = json.load(open(args.probe, encoding="utf-8"))
    if data.get("schema_version") != 1:
        fail("unsupported device probe schema")
    device = select_lighting(data, args.device_id, args.project_id)
    snapshot = snapshot_from_probe(data, device)
    atomic_write(args.out, snapshot)
    print(args.out)


def normalized_levels(observed):
    levels = observed.get("current_levels")
    if not isinstance(levels, dict) or not levels:
        fail("post current_levels missing")
    result = {}
    for key, value in levels.items():
        result[str(key)] = finite_number(value, f"current_levels.{key}")
    return result


def verify(event, pre, post, action_at):
    if event not in {"power-cycle", "brownout"}:
        fail("unsupported power event")
    if pre.get("schema_version") != 1 or post.get("schema_version") != 1:
        fail("unsupported power-event snapshot schema")
    if pre.get("device_id") != post.get("device_id"):
        fail("device identity changed across power event")
    if pre.get("project_id") != post.get("project_id"):
        fail("project identity changed across power event")
    if pre.get("client_version") != post.get("client_version"):
        fail("firmware client version changed across power event")

    pre_time = parse_time(pre.get("probe_generated_at"), "pre")
    post_time = parse_time(post.get("probe_generated_at"), "post")
    action_time = parse_time(action_at, "action")
    if post_time < pre_time:
        fail("post probe predates pre probe")
    if action_time < pre_time - dt.timedelta(seconds=5):
        fail("manual action acknowledgement predates prepared baseline")
    if action_time > post_time + dt.timedelta(seconds=5):
        fail("post probe predates manual action acknowledgement")

    post_observed = post.get("observed") or {}
    pre_observed = pre.get("observed") or {}

    uptime = finite_number(post_observed.get("uptime_seconds"), "post uptime_seconds")
    if uptime < 0:
        fail("post uptime_seconds must be non-negative")
    boot_time = post_time - dt.timedelta(seconds=uptime)
    if boot_time < pre_time - dt.timedelta(seconds=5):
        fail("post observation does not prove a reboot after the prepared baseline")
    if boot_time > action_time + dt.timedelta(seconds=5):
        fail("observed reboot occurred after the action was acknowledged")

    if post.get("connection_state") != "ONLINE":
        fail(f"post connection_state={post.get('connection_state')}")
    last_seen_us = post.get("last_seen_at_us")
    if not isinstance(last_seen_us, int):
        fail("post last_seen_at_us missing")
    last_seen = dt.datetime.fromtimestamp(last_seen_us / 1_000_000, tz=dt.timezone.utc)
    age = (post_time - last_seen).total_seconds()
    if age < -5 or age > 20:
        fail(f"post observation stale age_seconds={age:.1f}")

    expected_reason = "POWERON" if event == "power-cycle" else "BROWNOUT"
    expected_readiness = "READY" if event == "power-cycle" else "WARNING"
    expected_brownout = event == "brownout"

    if post_observed.get("reset_reason") != expected_reason:
        fail(f"reset_reason={post_observed.get('reset_reason')} expected={expected_reason}")
    if post_observed.get("brownout_warning") is not expected_brownout:
        fail(f"brownout_warning mismatch for {event}")
    if post.get("readiness") != expected_readiness:
        fail(f"post readiness={post.get('readiness')} expected={expected_readiness}")
    if post_observed.get("dmx_healthy") is not True:
        fail("post DMX is not healthy")
    if post_observed.get("authority") != "STAGECORE":
        fail(f"post authority={post_observed.get('authority')}")

    pre_hash = str(pre_observed.get("configuration_hash") or "")
    post_hash = str(post_observed.get("configuration_hash") or "")
    if not pre_hash or pre_hash != post_hash:
        fail("configuration hash changed or is missing across power event")

    levels = normalized_levels(post_observed)
    nonzero = {key: value for key, value in levels.items() if abs(value) > 0.01}
    if nonzero:
        fail("post-reboot logical output is not safe blackout: " + json.dumps(nonzero, sort_keys=True))
    if post_observed.get("active_fade") not in (None, {}, False):
        fail("post-reboot active fade is present")

    return {
        "status": "PASS",
        "event": event,
        "device_id": post.get("device_id"),
        "project_id": post.get("project_id"),
        "client_version": post.get("client_version"),
        "configuration_hash": post_hash,
        "pre_probe_generated_at": pre.get("probe_generated_at"),
        "manual_action_at": action_time.isoformat(),
        "post_probe_generated_at": post.get("probe_generated_at"),
        "derived_boot_at": boot_time.isoformat(),
        "reset_reason": post_observed.get("reset_reason"),
        "brownout_warning": post_observed.get("brownout_warning"),
        "readiness": post.get("readiness"),
        "authority": post_observed.get("authority"),
        "dmx_healthy": post_observed.get("dmx_healthy"),
        "current_levels": levels,
        "active_fade": post_observed.get("active_fade"),
    }


def cmd_verify(args):
    pre = json.load(open(args.pre, encoding="utf-8"))
    post = json.load(open(args.post, encoding="utf-8"))
    result = verify(args.event, pre, post, args.action_at)
    if args.out:
        atomic_write(args.out, result)
        print(args.out)
    else:
        print(json.dumps(result, sort_keys=True))


def main():
    parser = argparse.ArgumentParser(description="StageCore Q-DMX-15 power-event evidence")
    sub = parser.add_subparsers(dest="command", required=True)

    capture = sub.add_parser("capture")
    capture.add_argument("--probe", required=True)
    capture.add_argument("--device-id", default="")
    capture.add_argument("--project-id", default="")
    capture.add_argument("--out", required=True)
    capture.set_defaults(func=cmd_capture)

    verify_cmd = sub.add_parser("verify")
    verify_cmd.add_argument("--event", required=True, choices=("power-cycle", "brownout"))
    verify_cmd.add_argument("--pre", required=True)
    verify_cmd.add_argument("--post", required=True)
    verify_cmd.add_argument("--action-at", required=True)
    verify_cmd.add_argument("--out", default="")
    verify_cmd.set_defaults(func=cmd_verify)

    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
