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


def snapshot_from_probe(data, device, require_nonzero_channel=""):
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
    levels = observed.get("current_levels")
    if not isinstance(levels, dict) or not levels:
        fail("current_levels missing or empty", 3)

    normalized = {}
    for key, value in levels.items():
        normalized[str(key)] = finite_number(value, f"current_levels.{key}")

    if require_nonzero_channel:
        if require_nonzero_channel not in normalized:
            fail("qualification channel is absent from current_levels", 3)
        if abs(normalized[require_nonzero_channel]) <= 0.01:
            fail("qualification channel is not visibly nonzero before Wi-Fi loss", 3)

    return {
        "schema_version": 1,
        "probe_generated_at": data.get("generated_at"),
        "device_id": device.get("device_id"),
        "project_id": device.get("project_id"),
        "client_version": device.get("client_version"),
        "connection_state": runtime.get("connection_state"),
        "readiness": runtime.get("readiness"),
        "last_seen_at_us": runtime.get("last_seen_at_us"),
        "observed": dict(observed, current_levels=normalized),
    }


def cmd_capture(args):
    data = json.load(open(args.probe, encoding="utf-8"))
    if data.get("schema_version") != 1:
        fail("unsupported device probe schema")
    device = select_lighting(data, args.device_id, args.project_id)
    snapshot = snapshot_from_probe(data, device, args.require_nonzero_channel)
    atomic_write(args.out, snapshot)
    print(args.out)


def verify(pre, post, disconnect_at, reconnect_at):
    if pre.get("schema_version") != 1 or post.get("schema_version") != 1:
        fail("unsupported Wi-Fi-loss snapshot schema")
    for key in ("device_id", "project_id", "client_version"):
        if pre.get(key) != post.get(key):
            fail(f"{key} changed across Wi-Fi loss")

    pre_time = parse_time(pre.get("probe_generated_at"), "pre")
    post_time = parse_time(post.get("probe_generated_at"), "post")
    disconnect_time = parse_time(disconnect_at, "disconnect")
    reconnect_time = parse_time(reconnect_at, "reconnect")

    if disconnect_time < pre_time - dt.timedelta(seconds=5):
        fail("disconnect acknowledgement predates prepared baseline")
    if reconnect_time < disconnect_time:
        fail("reconnect acknowledgement predates disconnect acknowledgement")
    if post_time < reconnect_time - dt.timedelta(seconds=5):
        fail("post probe predates reconnect acknowledgement")

    pre_observed = pre.get("observed") or {}
    post_observed = post.get("observed") or {}
    pre_uptime = finite_number(pre_observed.get("uptime_seconds"), "pre uptime_seconds")
    post_uptime = finite_number(post_observed.get("uptime_seconds"), "post uptime_seconds")
    if post_uptime + 1 < pre_uptime:
        fail("device rebooted during Wi-Fi-loss test; this gate requires network loss without power reset")
    if post_observed.get("reset_reason") != pre_observed.get("reset_reason"):
        fail("reset_reason changed during Wi-Fi-loss test")
    if post_observed.get("brownout_warning") is not pre_observed.get("brownout_warning"):
        fail("brownout warning changed during Wi-Fi-loss test")

    if post.get("connection_state") != "ONLINE":
        fail(f"post connection_state={post.get('connection_state')}")
    if post.get("readiness") not in {"READY", "WARNING"}:
        fail(f"post readiness={post.get('readiness')}")
    if post_observed.get("dmx_healthy") is not True:
        fail("post DMX is not healthy")
    if post_observed.get("authority") != "STAGECORE":
        fail(f"post authority={post_observed.get('authority')}")

    last_seen_us = post.get("last_seen_at_us")
    if not isinstance(last_seen_us, int):
        fail("post last_seen_at_us missing")
    last_seen = dt.datetime.fromtimestamp(last_seen_us / 1_000_000, tz=dt.timezone.utc)
    age = (post_time - last_seen).total_seconds()
    if age < -5 or age > 20:
        fail(f"post observation stale age_seconds={age:.1f}")

    pre_hash = str(pre_observed.get("configuration_hash") or "")
    post_hash = str(post_observed.get("configuration_hash") or "")
    if not pre_hash or pre_hash != post_hash:
        fail("configuration hash changed or is missing across Wi-Fi loss")

    levels = post_observed.get("current_levels")
    if not isinstance(levels, dict) or not levels:
        fail("post current_levels missing")
    normalized = {}
    for key, value in levels.items():
        normalized[str(key)] = finite_number(value, f"post current_levels.{key}")
    nonzero = {key: value for key, value in normalized.items() if abs(value) > 0.01}
    if nonzero:
        fail("post-reconnect output is not safe blackout: " + json.dumps(nonzero, sort_keys=True))
    if post_observed.get("active_fade") not in (None, {}, False):
        fail("post-reconnect active fade is present")

    return {
        "status": "PASS",
        "event": "wifi-loss",
        "expected_physical_policy": "brief hold, then fade to blackout",
        "device_id": post.get("device_id"),
        "project_id": post.get("project_id"),
        "client_version": post.get("client_version"),
        "configuration_hash": post_hash,
        "pre_probe_generated_at": pre.get("probe_generated_at"),
        "disconnect_action_at": disconnect_time.isoformat(),
        "reconnect_action_at": reconnect_time.isoformat(),
        "post_probe_generated_at": post.get("probe_generated_at"),
        "outage_ack_seconds": (reconnect_time - disconnect_time).total_seconds(),
        "reset_reason": post_observed.get("reset_reason"),
        "brownout_warning": post_observed.get("brownout_warning"),
        "readiness": post.get("readiness"),
        "authority": post_observed.get("authority"),
        "dmx_healthy": post_observed.get("dmx_healthy"),
        "current_levels": normalized,
        "active_fade": post_observed.get("active_fade"),
        "no_reboot_observed": True,
    }


def cmd_verify(args):
    pre = json.load(open(args.pre, encoding="utf-8"))
    post = json.load(open(args.post, encoding="utf-8"))
    result = verify(pre, post, args.disconnect_at, args.reconnect_at)
    if args.out:
        atomic_write(args.out, result)
        print(args.out)
    else:
        print(json.dumps(result, sort_keys=True))


def main():
    parser = argparse.ArgumentParser(description="StageCore Q-DMX-16 Wi-Fi-loss evidence")
    sub = parser.add_subparsers(dest="command", required=True)

    capture = sub.add_parser("capture")
    capture.add_argument("--probe", required=True)
    capture.add_argument("--device-id", default="")
    capture.add_argument("--project-id", default="")
    capture.add_argument("--require-nonzero-channel", default="")
    capture.add_argument("--out", required=True)
    capture.set_defaults(func=cmd_capture)

    verify_cmd = sub.add_parser("verify")
    verify_cmd.add_argument("--pre", required=True)
    verify_cmd.add_argument("--post", required=True)
    verify_cmd.add_argument("--disconnect-at", required=True)
    verify_cmd.add_argument("--reconnect-at", required=True)
    verify_cmd.add_argument("--out", default="")
    verify_cmd.set_defaults(func=cmd_verify)

    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
