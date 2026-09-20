#!/usr/bin/env python3
import argparse
import json
import time

TABLET_PROFILE = "stagecore.tablet-player"
LIGHTING_PROFILE = "stagecore.esp32-dmx-lighting-node"

TABLET_CAPS = {
    "tablet.media.prepare",
    "tablet.media.play",
    "tablet.media.pause",
    "tablet.media.stop",
    "tablet.media.blackout",
    "tablet.media.blackout.clear",
    "tablet.media.overlay.play",
    "tablet.media.overlay.clear",
    "tablet.media.live.show",
    "tablet.media.live.hide",
}
LIGHTING_CAPS = {
    "lighting.channels.set",
    "lighting.channels.fade",
    "lighting.blackout",
    "lighting.state.read",
    "lighting.identify",
    "lighting.config.read",
    "lighting.config.apply",
}


def fail(message, code=1):
    print(message)
    raise SystemExit(code)


def main():
    parser = argparse.ArgumentParser(description="Assert StageCore qualification probe readiness")
    parser.add_argument("--input", required=True)
    parser.add_argument("--kind", choices=("tablet", "lighting"), required=True)
    parser.add_argument("--device-id", default="")
    parser.add_argument("--project-id", default="")
    parser.add_argument("--runtime-snapshot-id", default="")
    parser.add_argument("--max-age-seconds", type=int, default=20)
    parser.add_argument("--check", choices=("readiness", "scope", "observation"), default="readiness")
    parser.add_argument("--expect-online", choices=("yes", "no", "unknown"), default="unknown",
                        help="yes: an explicitly powered-on device must be ONLINE/READY; no: confirmed intentionally off; unknown: no physical power evidence")
    args = parser.parse_args()

    with open(args.input, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    if data.get("schema_version") != 1:
        fail("unsupported probe schema")

    profile = TABLET_PROFILE if args.kind == "tablet" else LIGHTING_PROFILE
    required = TABLET_CAPS if args.kind == "tablet" else LIGHTING_CAPS
    candidates = [d for d in data.get("devices", []) if d.get("profile_id") == profile]
    if args.project_id:
        candidates = [d for d in candidates if d.get("project_id") == args.project_id]
    if args.device_id:
        candidates = [d for d in candidates if d.get("device_id") == args.device_id]

    if len(candidates) == 0:
        fail(f"{args.kind} target not discovered", 3)
    if len(candidates) > 1:
        ids = ",".join(sorted(str(d.get("device_id", "")) for d in candidates))
        fail(f"{args.kind} target is ambiguous; configure device id: {ids}", 3)

    device = candidates[0]
    if not device.get("enabled"):
        fail(f"{args.kind} device disabled")
    if device.get("protocol_version") != "stagecore.device/1":
        fail(f"{args.kind} protocol mismatch: {device.get('protocol_version')}")
    if args.kind == "tablet" and device.get("device_kind") != "TABLET_PLAYER":
        fail(f"tablet device kind mismatch: {device.get('device_kind')}")

    missing = sorted(required - set(device.get("capabilities") or []))
    if missing:
        fail(f"{args.kind} missing capabilities: {','.join(missing)}")

    runtime = device.get("runtime")
    # A stale Hub observation cannot establish whether the device is powered.
    # Do not classify a confirmed switched-off Tablet/ESP32 as a product defect.
    # Once the operator explicitly confirms that the device is powered on,
    # missing reachability/readiness is a genuine test failure.
    unavailable_code = 1 if args.expect_online == "yes" else 3
    power_reason = ("confirmed powered-on target" if args.expect_online == "yes" else
                    "confirmed intentionally powered off" if args.expect_online == "no" else
                    "power state not independently confirmed")
    if not runtime:
        fail(f"DEVICE_UNAVAILABLE: {args.kind} has no runtime observation; {power_reason}",
             unavailable_code)
    if runtime.get("connection_state") != "ONLINE":
        fail(f"DEVICE_UNAVAILABLE: {args.kind} connection={runtime.get('connection_state')}; {power_reason}",
             unavailable_code)
    if runtime.get("readiness") != "READY":
        fail(f"DEVICE_UNAVAILABLE: {args.kind} readiness={runtime.get('readiness')}; {power_reason}",
             unavailable_code)

    last_seen_us = runtime.get("last_seen_at_us")
    if not isinstance(last_seen_us, int):
        fail(f"DEVICE_UNAVAILABLE: {args.kind} missing last_seen; {power_reason}", unavailable_code)
    age = time.time() - (last_seen_us / 1_000_000)
    if age < -5 or age > args.max_age_seconds:
        fail(f"DEVICE_UNAVAILABLE: {args.kind} observation stale age_seconds={age:.1f}; {power_reason}",
             unavailable_code)

    observed = runtime.get("observed") or {}

    if args.kind == "tablet" and args.check == "scope":
        if not args.project_id or not args.runtime_snapshot_id:
            fail("tablet scope check requires expected project and runtime snapshot", 3)
        if observed.get("project_id") != args.project_id:
            fail(f"tablet observed project mismatch: {observed.get('project_id')}")
        if observed.get("runtime_snapshot_id") != args.runtime_snapshot_id:
            fail(f"tablet observed snapshot mismatch: {observed.get('runtime_snapshot_id')}")

    if args.kind == "lighting" and args.check in ("readiness", "observation"):
        if observed.get("schema_version") != 1:
            fail("lighting observation schema mismatch")
        if observed.get("dmx_healthy") is not True:
            fail("lighting DMX is not healthy")
        if observed.get("brownout_warning") is True:
            fail("lighting brownout warning active")
        if observed.get("authority") != "STAGECORE":
            fail(f"lighting authority={observed.get('authority')}")
        if not str(observed.get("configuration_hash") or "").strip():
            fail("lighting configuration hash missing")

    print(json.dumps({
        "status": "PASS",
        "kind": args.kind,
        "device_id": device.get("device_id"),
        "project_id": device.get("project_id"),
        "client_version": device.get("client_version"),
        "connection": runtime.get("connection_state"),
        "readiness": runtime.get("readiness"),
        "latest_command": device.get("latest_command"),
    }, sort_keys=True))


if __name__ == "__main__":
    main()
