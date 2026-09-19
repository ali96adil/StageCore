#!/usr/bin/env python3
import argparse
import json
import subprocess
import sys
from pathlib import Path

GATE_ID = "Q-DMX-21"
CHECKS = [
    ("documented", "path.documented", "Actual MAX485/DMX wiring path is documented with module/decoder terminal labels before energizing."),
    ("logic-voltage", "logic_voltage.verified", "ESP32/MAX485 logic-voltage compatibility is physically verified; no unverified 5 V output is connected to an ESP32 input."),
    ("de-re", "de_re.verified", "MAX485 DE and /RE direction-control wiring is documented and physically verified for the intended transmit-only DMX path."),
    ("polarity", "dmx_polarity.verified", "RS-485/DMX polarity is physically traced end-to-end using actual module and decoder terminal labels; vendor A/B naming is not assumed."),
    ("common", "common_reference.verified", "DMX signal common/reference path is intentionally wired and physically verified."),
    ("termination", "termination.verified", "Bus-end termination is documented and physically verified for the actual topology before energizing."),
]
ALIASES = {alias: (key, desc) for alias, key, desc in CHECKS}


def fail(message, code=1):
    print(message, file=sys.stderr)
    raise SystemExit(code)


def load_state(path):
    try:
        return json.loads(Path(path).read_text(encoding="utf-8"))
    except FileNotFoundError:
        fail(f"qualification campaign state does not exist: {path}", 66)


def latest_invalidation_at(gate):
    values = [
        str(item.get("invalidated_at") or "")
        for item in gate.get("history", [])
        if item.get("invalidated_at")
    ]
    if gate.get("actor") == "repin" and gate.get("updated_at"):
        values.append(str(gate.get("updated_at")))
    return max(values) if values else ""


def milestone_status(state, key):
    gate = state.get("gates", {}).get(GATE_ID) or {}
    item = (gate.get("milestones", {}).get(key) or {})
    status = item.get("status") or "PENDING"
    # A deliberate repin/invalidation is a durable epoch. Parent status may
    # later move from PENDING to BLOCKED as fresh checks arrive, so the epoch
    # must come from gate history rather than only the current actor.
    invalidated_at = latest_invalidation_at(gate)
    milestone_updated = str(item.get("updated_at") or "")
    if invalidated_at and (not milestone_updated or milestone_updated <= invalidated_at):
        return "PENDING"
    return status


def gate_status(state):
    gate = state.get("gates", {}).get(GATE_ID)
    if gate is None:
        fail(f"{GATE_ID} is missing from qualification campaign state")
    return gate.get("status", "PENDING")


def run_tool(path, *args):
    result = subprocess.run([sys.executable, str(path), *args])
    if result.returncode != 0:
        raise SystemExit(result.returncode)


def cmd_status(args):
    state = load_state(args.state)
    rows = []
    for alias, key, description in CHECKS:
        item = ((state.get("gates", {}).get(GATE_ID) or {}).get("milestones", {}).get(key) or {})
        rows.append({
            "check": alias,
            "milestone": key,
            "status": milestone_status(state, key),
            "note": item.get("note", ""),
            "description": description,
        })
    payload = {"gate_id": GATE_ID, "status": gate_status(state), "checks": rows}
    if args.json:
        print(json.dumps(payload, sort_keys=True))
        return
    print(f"{GATE_ID}\t{payload['status']}")
    for row in rows:
        print(f"{row['check']}\t{row['status']}\t{row['description']}")


def cmd_ack(args):
    if args.check not in ALIASES:
        fail("check must be one of: " + ", ".join(alias for alias, _, _ in CHECKS), 64)
    if not args.note.strip():
        fail("--note is required and must document the physical observation", 64)

    state = load_state(args.state)
    if gate_status(state) == "PASS":
        fail(f"{GATE_ID} is already PASS; invalidate/repin the affected gate before replacing accepted electrical evidence", 3)

    key, description = ALIASES[args.check]
    root = Path(__file__).resolve().parent
    milestone_tool = root / "qualification-milestone.py"
    state_tool = root / "qualification-state.py"

    run_tool(
        milestone_tool, "record",
        "--state", args.state,
        "--manifest", args.manifest,
        "--gate", GATE_ID,
        "--key", key,
        "--status", args.status,
        "--actor", "manual-electrical-verification",
        "--evidence", "manual-electrical-verification",
        "--note", args.note.strip(),
    )

    state = load_state(args.state)
    statuses = {alias: milestone_status(state, milestone_key) for alias, milestone_key, _ in CHECKS}
    if any(value == "FAIL" for value in statuses.values()):
        target = "FAIL"
        parent_note = "Pre-energization electrical verification has a failed check; do not energize the DMX/24 V output path."
    elif all(value == "PASS" for value in statuses.values()):
        target = "PASS"
        parent_note = "All Q-DMX-21 MAX485/DMX pre-energization checks were explicitly documented and physically verified."
    else:
        target = "BLOCKED"
        passed = sum(value == "PASS" for value in statuses.values())
        parent_note = f"Pre-energization electrical verification incomplete: {passed}/{len(CHECKS)} required checks PASS."

    current = gate_status(state)
    if current != target:
        run_tool(
            state_tool, "record",
            "--state", args.state,
            "--manifest", args.manifest,
            "--gate", GATE_ID,
            "--status", target,
            "--actor", "manual-electrical-verification",
            "--evidence", "manual-electrical-verification",
            "--note", parent_note,
        )

    print(f"{GATE_ID}::{key}\t{args.status}\t{description}")
    final_state = load_state(args.state)
    print(f"{GATE_ID}\t{gate_status(final_state)}")


def main():
    parser = argparse.ArgumentParser(description="Q-DMX-21 manual MAX485/DMX pre-energization evidence workflow")
    sub = parser.add_subparsers(dest="command", required=True)

    status = sub.add_parser("status")
    status.add_argument("--state", required=True)
    status.add_argument("--json", action="store_true")
    status.set_defaults(func=cmd_status)

    ack = sub.add_parser("ack")
    ack.add_argument("--state", required=True)
    ack.add_argument("--manifest", required=True)
    ack.add_argument("--check", required=True, choices=[alias for alias, _, _ in CHECKS])
    ack.add_argument("--status", required=True, choices=("PASS", "FAIL"))
    ack.add_argument("--note", required=True)
    ack.set_defaults(func=cmd_ack)

    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
