#!/usr/bin/env python3
import argparse
import datetime as dt
import fcntl
import json
import os
import tempfile
from pathlib import Path

MAPPINGS = [
    ("Q-TAB-06", "physical.observation", ["prepare.command", "play.command"], "Tablet visibly prepared and played the expected main media."),
    ("Q-TAB-07", "physical.observation", ["pause.command", "stop.command"], "Tablet visibly paused and stopped main playback as expected."),
    ("Q-DMX-03", "physical.observation", ["set.command"], "Selected DMX channel visibly reached the commanded test level."),
    ("Q-DMX-05", "physical.observation", ["fade.command"], "Selected DMX channel visibly completed the requested fade smoothly."),
    ("Q-DMX-10", "physical.observation", ["blackout.command"], "Lighting output visibly reached immediate blackout."),
]


def now():
    return dt.datetime.now(dt.timezone.utc).isoformat()


def atomic_write(path, value):
    path = Path(path)
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


def lock_state(path):
    lock = open(str(path) + ".lock", "a+", encoding="utf-8")
    fcntl.flock(lock.fileno(), fcntl.LOCK_EX)
    return lock


def milestone_status(gate, key):
    return (gate.get("milestones", {}).get(key) or {}).get("status", "PENDING")


def eligible(state):
    rows = []
    for gate_id, obs_key, prereqs, description in MAPPINGS:
        gate = state.get("gates", {}).get(gate_id)
        if not gate:
            continue
        if all(milestone_status(gate, key) == "PASS" for key in prereqs):
            if milestone_status(gate, obs_key) != "PASS":
                rows.append((gate_id, obs_key, description))
    return rows


def cmd_pending(args):
    state = json.load(open(args.state, encoding="utf-8"))
    rows = eligible(state)
    if args.json:
        print(json.dumps([
            {"gate_id": gate, "milestone": key, "description": desc}
            for gate, key, desc in rows
        ], sort_keys=True))
        return
    if not rows:
        print("No pending physical confirmations.")
        return
    print("Pending physical confirmations:")
    for gate, _, desc in rows:
        print(f"- {gate}: {desc}")


def cmd_confirm(args):
    if not args.note.strip():
        raise SystemExit("--note is required")
    state_path = Path(args.state)
    state_path.parent.mkdir(parents=True, exist_ok=True)
    with lock_state(state_path):
        state = json.load(open(state_path, encoding="utf-8"))
        rows = eligible(state)
        if not rows:
            print("No eligible pending physical confirmations.")
            return
        timestamp = now()
        for gate_id, obs_key, _ in rows:
            gate = state["gates"][gate_id]
            milestones = gate.setdefault("milestones", {})
            previous = milestones.get(obs_key)
            history = list((previous or {}).get("history", []))
            if previous:
                history.append({
                    "status": previous.get("status"),
                    "updated_at": previous.get("updated_at"),
                    "actor": previous.get("actor"),
                    "evidence": previous.get("evidence", []),
                    "note": previous.get("note", ""),
                })
            milestones[obs_key] = {
                "status": args.status,
                "updated_at": timestamp,
                "actor": "manual-physical-confirmation",
                "evidence": ["physical-observation"],
                "note": args.note.strip(),
                "history": history,
            }
            if args.status == "PASS":
                gate["history"].append({
                    "status": gate.get("status"),
                    "updated_at": gate.get("updated_at"),
                    "actor": gate.get("actor"),
                    "evidence": gate.get("evidence", []),
                    "note": gate.get("note", ""),
                })
                gate["status"] = "PASS"
                gate["updated_at"] = timestamp
                gate["actor"] = "manual-physical-confirmation"
                gate["evidence"] = ["physical-observation"]
                gate["note"] = args.note.strip()
            else:
                gate["history"].append({
                    "status": gate.get("status"),
                    "updated_at": gate.get("updated_at"),
                    "actor": gate.get("actor"),
                    "evidence": gate.get("evidence", []),
                    "note": gate.get("note", ""),
                })
                gate["status"] = "FAIL"
                gate["updated_at"] = timestamp
                gate["actor"] = "manual-physical-confirmation"
                gate["evidence"] = ["physical-observation"]
                gate["note"] = args.note.strip()
        state["updated_at"] = timestamp
        atomic_write(state_path, state)
    print(f"Recorded {args.status} for {len(rows)} physical confirmation(s).")


def main():
    parser = argparse.ArgumentParser(description="Aggregated StageCore physical confirmation manager")
    sub = parser.add_subparsers(dest="command", required=True)
    pending = sub.add_parser("pending")
    pending.add_argument("--state", required=True)
    pending.add_argument("--json", action="store_true")
    pending.set_defaults(func=cmd_pending)
    confirm = sub.add_parser("confirm-all")
    confirm.add_argument("--state", required=True)
    confirm.add_argument("--status", choices=("PASS", "FAIL"), required=True)
    confirm.add_argument("--note", required=True)
    confirm.set_defaults(func=cmd_confirm)
    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
