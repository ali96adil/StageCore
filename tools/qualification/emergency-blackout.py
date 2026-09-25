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


def finite(value, label):
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
    devices = [d for d in data.get("devices", []) if d.get("profile_id") == LIGHTING_PROFILE]
    if device_id:
        devices = [d for d in devices if d.get("device_id") == device_id]
    if project_id:
        devices = [d for d in devices if d.get("project_id") == project_id]
    if not devices:
        fail("lighting target not present in probe", 3)
    if len(devices) != 1:
        fail("lighting target is ambiguous", 3)
    return devices[0]


def normalized_levels(observed, label):
    levels = observed.get("current_levels")
    if not isinstance(levels, dict) or not levels:
        fail(f"{label} current_levels missing", 3)
    out = {}
    for key, value in levels.items():
        out[str(key)] = finite(value, f"{label} current_levels.{key}")
    return out


def snapshot_from_probe(data, device, require_nonzero_channel=""):
    runtime = device.get("runtime")
    if not isinstance(runtime, dict):
        fail("lighting runtime observation missing", 3)
    observed = runtime.get("observed")
    if not isinstance(observed, dict):
        fail("lighting observed state missing", 3)
    required = (
        "uptime_seconds", "reset_reason", "current_levels", "dmx_healthy",
        "configuration_hash", "brownout_warning", "authority",
    )
    missing = [key for key in required if key not in observed]
    if missing:
        fail("lighting observation missing: " + ",".join(missing), 3)
    levels = normalized_levels(observed, "snapshot")
    if require_nonzero_channel:
        if require_nonzero_channel not in levels:
            fail("qualification channel absent from current_levels", 3)
        if abs(levels[require_nonzero_channel]) <= 0.01:
            fail("qualification channel is not visibly nonzero", 3)
    uptime = finite(observed.get("uptime_seconds"), "uptime_seconds")
    if uptime < 0:
        fail("uptime_seconds must be non-negative")
    return {
        "schema_version": 1,
        "probe_generated_at": data.get("generated_at"),
        "device_id": device.get("device_id"),
        "project_id": device.get("project_id"),
        "client_version": device.get("client_version"),
        "connection_state": runtime.get("connection_state"),
        "readiness": runtime.get("readiness"),
        "last_seen_at_us": runtime.get("last_seen_at_us"),
        "observed": dict(observed, current_levels=levels),
    }


def cmd_capture(args):
    data = json.load(open(args.probe, encoding="utf-8"))
    if data.get("schema_version") != 1:
        fail("unsupported device probe schema")
    device = select_lighting(data, args.device_id, args.project_id)
    value = snapshot_from_probe(data, device, args.require_nonzero_channel)
    atomic_write(args.out, value)
    print(args.out)


def validate_safe_snapshot(label, value, pre, recovery_time, prior_command_id):
    if value.get("schema_version") != 1:
        fail(f"unsupported {label} snapshot schema")
    for key in ("device_id", "project_id", "client_version"):
        if value.get(key) != pre.get(key):
            fail(f"{label} {key} changed")

    generated = parse_time(value.get("probe_generated_at"), label)
    if generated < recovery_time - dt.timedelta(seconds=5):
        fail(f"{label} probe predates Hub recovery")

    if value.get("connection_state") != "ONLINE":
        fail(f"{label} connection_state={value.get('connection_state')}")
    if value.get("readiness") not in {"READY", "WARNING"}:
        fail(f"{label} readiness={value.get('readiness')}")
    last_seen_us = value.get("last_seen_at_us")
    if not isinstance(last_seen_us, int):
        fail(f"{label} last_seen_at_us missing")
    last_seen = dt.datetime.fromtimestamp(last_seen_us / 1_000_000, tz=dt.timezone.utc)
    age = (generated - last_seen).total_seconds()
    if age < -5 or age > 20:
        fail(f"{label} observation stale age_seconds={age:.1f}")

    observed = value.get("observed") or {}
    pre_observed = pre.get("observed") or {}
    if observed.get("dmx_healthy") is not True:
        fail(f"{label} DMX is not healthy")
    if observed.get("authority") != "STAGECORE":
        fail(f"{label} authority={observed.get('authority')}")
    pre_hash = str(pre_observed.get("configuration_hash") or "")
    post_hash = str(observed.get("configuration_hash") or "")
    if not pre_hash or pre_hash != post_hash:
        fail(f"{label} configuration hash changed or is missing")
    pre_uptime = finite(pre_observed.get("uptime_seconds"), "pre uptime_seconds")
    post_uptime = finite(observed.get("uptime_seconds"), f"{label} uptime_seconds")
    if post_uptime + 1 < pre_uptime:
        fail(f"{label} ESP32 rebooted while only Hub was unavailable")
    if observed.get("reset_reason") != pre_observed.get("reset_reason"):
        fail(f"{label} reset_reason changed")
    levels = normalized_levels(observed, label)
    nonzero = {key: value for key, value in levels.items() if abs(value) > 0.01}
    if nonzero:
        fail(f"{label} output is not blackout: " + json.dumps(nonzero, sort_keys=True))
    if observed.get("active_fade") not in (None, {}, False):
        fail(f"{label} active fade is present")
    if prior_command_id:
        if observed.get("last_accepted_command_id") == prior_command_id:
            fail(f"{label} old StageCore command was accepted again")
        if observed.get("last_applied_command_id") == prior_command_id:
            fail(f"{label} old StageCore command was applied again")
    return generated, levels


def cmd_verify(args):
    pre = json.load(open(args.pre, encoding="utf-8"))
    post = json.load(open(args.post, encoding="utf-8"))
    stable = json.load(open(args.stable, encoding="utf-8"))
    if pre.get("schema_version") != 1:
        fail("unsupported pre snapshot schema")
    pre_time = parse_time(pre.get("probe_generated_at"), "pre")
    unavailable_time = parse_time(args.hub_unavailable_at, "hub-unavailable")
    blackout_time = parse_time(args.blackout_at, "blackout")
    recovery_time = parse_time(args.recovery_at, "recovery")
    if unavailable_time < pre_time - dt.timedelta(seconds=5):
        fail("Hub-unavailable evidence predates prepared baseline")
    if blackout_time < unavailable_time:
        fail("local blackout acknowledgement predates Hub unavailability")
    if recovery_time < blackout_time:
        fail("Hub recovery predates local blackout acknowledgement")

    pre_observed = pre.get("observed") or {}
    pre_levels = normalized_levels(pre_observed, "pre")
    if not any(abs(value) > 0.01 for value in pre_levels.values()):
        fail("prepared baseline is not visibly nonzero", 3)
    prior_command_id = str(pre_observed.get("last_applied_command_id") or "")

    post_time, post_levels = validate_safe_snapshot(
        "post", post, pre, recovery_time, prior_command_id
    )
    stable_time, stable_levels = validate_safe_snapshot(
        "stable", stable, pre, recovery_time, prior_command_id
    )
    if stable_time < post_time:
        fail("stable probe predates post probe")

    result = {
        "status": "PASS",
        "event": "LOCAL_EMERGENCY_BLACKOUT_WITH_HUB_UNAVAILABLE",
        "device_id": post.get("device_id"),
        "project_id": post.get("project_id"),
        "client_version": post.get("client_version"),
        "configuration_hash": str((post.get("observed") or {}).get("configuration_hash") or ""),
        "pre_probe_generated_at": pre.get("probe_generated_at"),
        "hub_unavailable_at": unavailable_time.isoformat(),
        "local_blackout_acknowledged_at": blackout_time.isoformat(),
        "hub_recovery_at": recovery_time.isoformat(),
        "post_probe_generated_at": post.get("probe_generated_at"),
        "stable_probe_generated_at": stable.get("probe_generated_at"),
        "no_esp_reboot": True,
        "no_stale_replay": True,
        "post_levels": post_levels,
        "stable_levels": stable_levels,
        "post_readiness": post.get("readiness"),
        "post_authority": (post.get("observed") or {}).get("authority"),
        "post_dmx_healthy": (post.get("observed") or {}).get("dmx_healthy"),
    }
    if args.out:
        atomic_write(args.out, result)
        print(args.out)
    else:
        print(json.dumps(result, sort_keys=True))


def main():
    parser = argparse.ArgumentParser(description="StageCore Q-DMX-19 local emergency blackout evidence")
    sub = parser.add_subparsers(dest="command", required=True)

    capture = sub.add_parser("capture")
    capture.add_argument("--probe", required=True)
    capture.add_argument("--device-id", default="")
    capture.add_argument("--project-id", default="")
    capture.add_argument("--require-nonzero-channel", default="")
    capture.add_argument("--out", required=True)
    capture.set_defaults(func=cmd_capture)

    verify = sub.add_parser("verify")
    verify.add_argument("--pre", required=True)
    verify.add_argument("--post", required=True)
    verify.add_argument("--stable", required=True)
    verify.add_argument("--hub-unavailable-at", required=True)
    verify.add_argument("--blackout-at", required=True)
    verify.add_argument("--recovery-at", required=True)
    verify.add_argument("--out", default="")
    verify.set_defaults(func=cmd_verify)

    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
