#!/usr/bin/env python3
import argparse
import datetime as dt
import fcntl
import json
import os
import tempfile
from pathlib import Path

ALLOWED = {"PASS", "FAIL", "BLOCKED"}


def now():
    return dt.datetime.now(dt.timezone.utc).isoformat()


def manifest_gates(path):
    data = json.loads(Path(path).read_text(encoding="utf-8"))
    out = set()
    for group in data.get("groups", []):
        for gate in group.get("gates", []):
            out.add(gate["id"])
    return out


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


def with_lock(path):
    lock_path = str(path) + ".lock"
    Path(lock_path).parent.mkdir(parents=True, exist_ok=True)
    lock = open(lock_path, "a+", encoding="utf-8")
    fcntl.flock(lock.fileno(), fcntl.LOCK_EX)
    return lock


def get_milestone(state, gate_id, key):
    gate = state.get("gates", {}).get(gate_id)
    if gate is None:
        raise SystemExit(f"unknown gate {gate_id}")
    return gate.setdefault("milestones", {}).get(key)


def cmd_get(args):
    state = json.loads(Path(args.state).read_text(encoding="utf-8"))
    milestone = get_milestone(state, args.gate, args.key)
    if milestone is None:
        print("PENDING")
    elif args.json:
        print(json.dumps(milestone, sort_keys=True))
    else:
        print(milestone.get("status", "PENDING"))


def cmd_record(args):
    if args.status not in ALLOWED:
        raise SystemExit("milestone status must be PASS, FAIL, or BLOCKED")
    gates = manifest_gates(args.manifest)
    if args.gate not in gates:
        raise SystemExit(f"unknown manifest gate {args.gate}")
    if not args.key.strip():
        raise SystemExit("milestone key is required")

    state_path = Path(args.state)
    with with_lock(state_path):
        state = json.loads(state_path.read_text(encoding="utf-8"))
        gate = state.get("gates", {}).get(args.gate)
        if gate is None:
            raise SystemExit(f"gate missing from campaign state: {args.gate}")
        milestones = gate.setdefault("milestones", {})
        previous = milestones.get(args.key)
        history = []
        if previous:
            history = list(previous.get("history", []))
            history.append({
                "status": previous.get("status"),
                "updated_at": previous.get("updated_at"),
                "actor": previous.get("actor"),
                "evidence": previous.get("evidence", []),
                "note": previous.get("note", ""),
            })
        timestamp = now()
        milestones[args.key] = {
            "status": args.status,
            "updated_at": timestamp,
            "actor": args.actor,
            "evidence": list(args.evidence or []),
            "note": args.note or "",
            "history": history,
        }
        state["updated_at"] = timestamp
        atomic_write(state_path, state)
    print(f"{args.gate}::{args.key}\t{args.status}")


def main():
    parser = argparse.ArgumentParser(description="StageCore qualification gate milestone state")
    sub = parser.add_subparsers(dest="command", required=True)

    get = sub.add_parser("get")
    get.add_argument("--state", required=True)
    get.add_argument("--gate", required=True)
    get.add_argument("--key", required=True)
    get.add_argument("--json", action="store_true")
    get.set_defaults(func=cmd_get)

    record = sub.add_parser("record")
    record.add_argument("--state", required=True)
    record.add_argument("--manifest", required=True)
    record.add_argument("--gate", required=True)
    record.add_argument("--key", required=True)
    record.add_argument("--status", required=True, choices=sorted(ALLOWED))
    record.add_argument("--actor", default="runner")
    record.add_argument("--evidence", action="append", default=[])
    record.add_argument("--note", default="")
    record.set_defaults(func=cmd_record)

    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
