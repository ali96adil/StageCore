#!/usr/bin/env python3
import argparse
import json

PROFILES = {
    "tablet": "stagecore.tablet-player",
    "lighting": "stagecore.esp32-dmx-lighting-node",
}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", required=True)
    parser.add_argument("--kind", choices=sorted(PROFILES), required=True)
    parser.add_argument("--device-id", default="")
    parser.add_argument("--project-id", default="")
    args = parser.parse_args()

    data = json.load(open(args.input, encoding="utf-8"))
    devices = [d for d in data.get("devices", []) if d.get("profile_id") == PROFILES[args.kind]]
    if args.project_id:
        devices = [d for d in devices if d.get("project_id") == args.project_id]
    if args.device_id:
        devices = [d for d in devices if d.get("device_id") == args.device_id]
    if len(devices) != 1:
        raise SystemExit(3)
    device = devices[0]
    print(str(device.get("device_id", "")) + "\t" + str(device.get("project_id", "")))


if __name__ == "__main__":
    main()
