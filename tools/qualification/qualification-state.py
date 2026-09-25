#!/usr/bin/env python3
import argparse
import copy
import datetime as dt
import fcntl
import hashlib
import json
import os
import tempfile
from pathlib import Path

ALLOWED_RESULTS = {"PENDING", "PASS", "FAIL", "BLOCKED", "N/A"}
PHYSICAL_METHODS = {"AUTO_PHYSICAL", "MANUAL"}

def now():
    return dt.datetime.now(dt.timezone.utc).isoformat()

def load_json(path):
    with open(path, "r", encoding="utf-8") as fh:
        return json.load(fh)

def manifest_info(path):
    raw = Path(path).read_bytes()
    data = json.loads(raw)
    gates = {}
    for group in data.get("groups", []):
        for gate in group.get("gates", []):
            item = dict(gate)
            item["group_id"] = group.get("id")
            item["source_issue"] = group.get("source_issue")
            gates[item["id"]] = item
    return hashlib.sha256(raw).hexdigest(), gates

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

def locked_state(path):
    lock_path = str(path) + ".lock"
    Path(lock_path).parent.mkdir(parents=True, exist_ok=True)
    lock = open(lock_path, "a+", encoding="utf-8")
    fcntl.flock(lock.fileno(), fcntl.LOCK_EX)
    return lock

def fresh_gate(gate):
    return {
        "status": "PENDING",
        "method": gate["method"],
        "source_issue": gate["source_issue"],
        "acceptance": gate["acceptance"],
        "updated_at": None,
        "actor": None,
        "evidence": [],
        "note": "",
        "history": [],
    }

def pins_from_args(args):
    return {
        "stagecore_sha": (args.stagecore_sha or "").strip(),
        "tablet_build_sha": (args.tablet_build_sha or "").strip(),
        "tablet_apk_sha256": (args.tablet_apk_sha256 or "").strip(),
        "lighting_firmware_sha": (args.lighting_firmware_sha or "").strip(),
        "hardware_baseline_id": (args.hardware_baseline_id or "").strip(),
    }

def cmd_init(args):
    manifest_hash, gates = manifest_info(args.manifest)
    state_path = Path(args.state)
    with locked_state(state_path):
        if state_path.exists():
            state = load_json(state_path)
            if state.get("schema_version") != 1:
                raise SystemExit("unsupported campaign state schema")
            if state.get("manifest_sha256") != manifest_hash:
                raise SystemExit("qualification manifest changed; start/repin deliberately before reusing evidence")
            supplied = pins_from_args(args)
            existing = state.get("pins", {})
            conflicts = {
                key: (existing.get(key, ""), value)
                for key, value in supplied.items()
                if value and existing.get(key, "") and value != existing.get(key, "")
            }
            if conflicts:
                details = ", ".join(f"{k}:{old}->{new}" for k, (old, new) in conflicts.items())
                raise SystemExit("qualification baseline mismatch; use repin with explicit invalidation: " + details)
            changed = False
            for key, value in supplied.items():
                if value and not existing.get(key):
                    existing[key] = value
                    changed = True
            state["pins"] = existing
            for gate_id, gate in gates.items():
                if gate_id not in state["gates"]:
                    state["gates"][gate_id] = fresh_gate(gate)
                    changed = True
            if changed:
                state["updated_at"] = now()
                atomic_write(state_path, state)
        else:
            timestamp = now()
            state = {
                "schema_version": 1,
                "campaign_id": timestamp.replace(":", "").replace("+00:00", "Z"),
                "manifest_sha256": manifest_hash,
                "created_at": timestamp,
                "updated_at": timestamp,
                "pins": pins_from_args(args),
                "pin_history": [],
                "gates": {gate_id: fresh_gate(gate) for gate_id, gate in gates.items()},
            }
            atomic_write(state_path, state)
    print(args.state)

def cmd_get(args):
    state = load_json(args.state)
    gate = state.get("gates", {}).get(args.gate)
    if gate is None:
        raise SystemExit(f"unknown gate {args.gate}")
    if args.field == "status":
        print(gate.get("status", "PENDING"))
    else:
        print(json.dumps(gate, sort_keys=True))

def cmd_record(args):
    if args.status not in ALLOWED_RESULTS - {"PENDING"}:
        raise SystemExit("record status must be PASS, FAIL, BLOCKED, or N/A")
    manifest_hash, manifest_gates = manifest_info(args.manifest)
    gate_def = manifest_gates.get(args.gate)
    if gate_def is None:
        raise SystemExit(f"unknown manifest gate {args.gate}")
    if args.status == "N/A" and not gate_def.get("na_allowed", False):
        raise SystemExit(f"{args.gate} does not allow N/A")
    if args.status == "N/A" and not (args.note or "").strip():
        raise SystemExit("N/A requires a reason in --note")
    if args.status == "PASS" and gate_def.get("method") in PHYSICAL_METHODS:
        if not args.evidence and not (args.note or "").strip():
            raise SystemExit(f"{args.gate} physical/manual PASS requires evidence or an observation note")

    state_path = Path(args.state)
    with locked_state(state_path):
        state = load_json(state_path)
        if state.get("manifest_sha256") != manifest_hash:
            raise SystemExit("manifest/state mismatch")
        gate = state["gates"].get(args.gate)
        if gate is None:
            raise SystemExit(f"gate missing from campaign state: {args.gate}")
        previous = copy.deepcopy(gate)
        if previous.get("status") != "PENDING":
            gate.setdefault("history", []).append({
                "status": previous.get("status"),
                "updated_at": previous.get("updated_at"),
                "actor": previous.get("actor"),
                "evidence": previous.get("evidence", []),
                "note": previous.get("note", ""),
            })
        gate.update({
            "status": args.status,
            "updated_at": now(),
            "actor": args.actor,
            "evidence": list(args.evidence or []),
            "note": args.note or "",
        })
        state["updated_at"] = gate["updated_at"]
        atomic_write(state_path, state)
    print(f"{args.gate}\t{args.status}")

def cmd_status(args):
    state = load_json(args.state)
    counts = {status: 0 for status in ["PENDING", "PASS", "FAIL", "BLOCKED", "N/A"]}
    rows = []
    for gate_id, gate in sorted(state.get("gates", {}).items()):
        status = gate.get("status", "PENDING")
        counts[status] = counts.get(status, 0) + 1
        if args.all or status != "PASS":
            rows.append((gate_id, status, gate.get("method", ""), gate.get("acceptance", "")))
    if args.json:
        print(json.dumps({
            "campaign_id": state.get("campaign_id"),
            "pins": state.get("pins"),
            "counts": counts,
            "gates": [
                {"gate_id": gid, "status": status, "method": method, "acceptance": acceptance}
                for gid, status, method, acceptance in rows
            ],
        }, sort_keys=True))
        return
    print("Campaign:", state.get("campaign_id"))
    print("Counts:", " ".join(f"{k}={v}" for k, v in counts.items()))
    for row in rows:
        print("\t".join(row))

def cmd_repin(args):
    manifest_hash, manifest_gates = manifest_info(args.manifest)
    if args.pin not in {"stagecore_sha", "tablet_build_sha", "tablet_apk_sha256", "lighting_firmware_sha", "hardware_baseline_id"}:
        raise SystemExit("unsupported pin")
    if not args.value.strip():
        raise SystemExit("repin value is required")
    if not args.invalidate:
        raise SystemExit("repin requires at least one explicit --invalidate gate")
    for gate_id in args.invalidate:
        if gate_id not in manifest_gates:
            raise SystemExit(f"unknown invalidation gate {gate_id}")
    if not args.reason.strip():
        raise SystemExit("repin requires --reason")

    state_path = Path(args.state)
    with locked_state(state_path):
        state = load_json(state_path)
        if state.get("manifest_sha256") != manifest_hash:
            raise SystemExit("manifest/state mismatch")
        old = state.get("pins", {}).get(args.pin, "")
        timestamp = now()
        state.setdefault("pin_history", []).append({
            "pin": args.pin,
            "old": old,
            "new": args.value.strip(),
            "reason": args.reason.strip(),
            "at": timestamp,
            "invalidated_gates": list(args.invalidate),
        })
        state.setdefault("pins", {})[args.pin] = args.value.strip()
        for gate_id in args.invalidate:
            gate = state["gates"][gate_id]
            gate.setdefault("history", []).append({
                "status": gate.get("status"),
                "updated_at": gate.get("updated_at"),
                "actor": gate.get("actor"),
                "evidence": gate.get("evidence", []),
                "note": gate.get("note", ""),
                "invalidated_reason": args.reason.strip(),
                "invalidated_at": timestamp,
            })
            gate.update({
                "status": "PENDING",
                "updated_at": timestamp,
                "actor": "repin",
                "evidence": [],
                "note": "Invalidated after baseline change: " + args.reason.strip(),
            })
        state["updated_at"] = timestamp
        atomic_write(state_path, state)
    print(f"{args.pin}\t{old}\t{args.value.strip()}")

def parser():
    p = argparse.ArgumentParser(description="Durable StageCore physical qualification campaign state")
    sub = p.add_subparsers(dest="command", required=True)

    init = sub.add_parser("init")
    init.add_argument("--state", required=True)
    init.add_argument("--manifest", required=True)
    init.add_argument("--stagecore-sha", default="")
    init.add_argument("--tablet-build-sha", default="")
    init.add_argument("--tablet-apk-sha256", default="")
    init.add_argument("--lighting-firmware-sha", default="")
    init.add_argument("--hardware-baseline-id", default="")
    init.set_defaults(func=cmd_init)

    get = sub.add_parser("get")
    get.add_argument("--state", required=True)
    get.add_argument("--gate", required=True)
    get.add_argument("--field", choices=("status", "json"), default="status")
    get.set_defaults(func=cmd_get)

    record = sub.add_parser("record")
    record.add_argument("--state", required=True)
    record.add_argument("--manifest", required=True)
    record.add_argument("--gate", required=True)
    record.add_argument("--status", required=True, choices=("PASS", "FAIL", "BLOCKED", "N/A"))
    record.add_argument("--actor", default="runner")
    record.add_argument("--evidence", action="append", default=[])
    record.add_argument("--note", default="")
    record.set_defaults(func=cmd_record)

    status = sub.add_parser("status")
    status.add_argument("--state", required=True)
    status.add_argument("--json", action="store_true")
    status.add_argument("--all", action="store_true")
    status.set_defaults(func=cmd_status)

    repin = sub.add_parser("repin")
    repin.add_argument("--state", required=True)
    repin.add_argument("--manifest", required=True)
    repin.add_argument("--pin", required=True)
    repin.add_argument("--value", required=True)
    repin.add_argument("--invalidate", action="append", default=[])
    repin.add_argument("--reason", required=True)
    repin.set_defaults(func=cmd_repin)
    return p

def main():
    args = parser().parse_args()
    args.func(args)

if __name__ == "__main__":
    main()
