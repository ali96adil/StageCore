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
    ("Q-TAB-08", "physical.observation", ["overlay_play.command", "overlay_clear.command"], "Tablet visibly showed and cleared the overlay while main playback continued underneath."),
    ("Q-TAB-09", "physical.observation", ["live_show.command", "live_hide.command"], "Tablet visibly showed and hid the configured live layer."),
    ("Q-TAB-10", "physical.observation", ["blackout.command", "blackout_clear.command"], "Tablet visibly blacked out and then restored the prior presentation when blackout was cleared."),
    ("Q-TAB-12", "physical.observation", ["published.execution"], "The Published qualification Tablet Scene visibly reached the real tablet through Cue Engine / Stage Device forwarding."),
    ("Q-TAB-13", "physical.observation", ["missing_media.command"], "The intentionally missing media produced a clear operator-visible failure matching MEDIA_NOT_FOUND."),
    ("Q-TAB-14", "physical.observation", ["reconnect.post"], "After the Tablet network disconnect/reconnect, the prior media command did not visibly replay or restart."),
    ("Q-TAB-16", "physical.observation", ["group_play.command"], "A single Tablet Controller group target visibly started the expected media on at least two real tablets in the same qualified group."),
    ("Q-CALL-01", "physical.observation", ["message.command"], "The selected real Stage Display visibly rendered the qualification message."),
    ("Q-CALL-02", "physical.observation", ["countdown.command"], "Real Stage Display countdown visibly advanced from one absolute target; it was not repeatedly driven by Hub commands."),
    ("Q-CALL-03", "physical.observation", ["alert.command", "clear.command"], "The selected real Stage Display visibly rendered the bounded semantic pulse alert."),
    ("Q-CALL-04", "physical.observation", ["chime.command"], "The selected real Stage Display audibly played exactly one optional qualification chime."),
    ("Q-CALL-06", "physical.observation", ["reconnect.post"], "The real display showed no replay of the expired alert/chime after its network recovered."),
    ("Q-CALL-05", "physical.observation", ["published.cue"], "A Published Callboard Cue or timeline visibly invoked the real Stage Display without autonomous GO."),
    ("Q-NET-01", "physical.observation", ["cockpit.api"], "The Operator Network Cockpit visibly showed the same real project Stage Device(s), Companion and Live Video source(s) proven by the authenticated API evidence."),
    ("Q-NET-02", "physical.observation", ["disconnect.action", "reconnect.action", "reconnect.observations"], "During real device-only network isolation, the Operator Cockpit visibly classified disconnect and recovery on the exact selected device."),
    ("Q-NET-04", "physical.observation", ["warning.observations"], "The real Stage Network Cockpit presented the observed unreachable/transport warning with a truthful actionable reason, without inventing latency or jitter."),
    ("Q-DMX-03", "physical.observation", ["set.command"], "Selected DMX channel visibly reached the commanded test level."),
    ("Q-DMX-04", "physical.observation", ["multi_set.command"], "Two configured DMX channels visibly reached their commanded levels."),
    ("Q-DMX-05", "physical.observation", ["fade.command"], "Selected DMX channel visibly completed the requested fade smoothly."),
    ("Q-DMX-06", "physical.observation", ["multi_fade.command"], "Two configured DMX channels visibly faded in synchronization."),
    ("Q-DMX-07", "physical.observation", ["timing.measurement"], "Measured real-device fade lifecycle was within the configured tolerance and the fade visibly completed normally."),
    ("Q-DMX-08", "physical.observation", ["precondition_set.command", "long_fade.command"], "Configured long fade visibly completed without premature timeout or interruption."),
    ("Q-DMX-09", "physical.observation", ["supersession.sequence"], "A visible active fade was interrupted by the newer SET and the superseded fade did not resume."),
    ("Q-DMX-10", "physical.observation", ["blackout.command"], "Lighting output visibly reached immediate blackout."),
    ("Q-DMX-11", "physical.observation", ["precondition_set.command", "timed_blackout.command"], "Lighting visibly faded to blackout over the configured timed-blackout duration."),
    ("Q-DMX-14", "physical.observation", ["invalid_value.sequence"], "Invalid out-of-range/unknown-channel requests caused no visible unsafe change; any configured-bound clamp stayed within the channel limit and output restored."),
    ("Q-DMX-15", "physical.observation", ["power_cycle.post", "brownout.post"], "Power-cycle and controlled brownout both visibly returned the lighting output to the safe blackout state during reboot/recovery."),
    ("Q-DMX-16", "physical.observation", ["wifi_loss.post"], "During ESP-only Wi-Fi loss, output visibly followed the frozen policy: brief hold, then fade to blackout; after Wi-Fi restore no stale brightness returned."),
    ("Q-DMX-17", "physical.observation", ["restart.sequence"], "A visible active fade was interrupted by the Hub restart and did not resume or replay after the ESP32 reconnected; output remained in the safe state."),
    ("Q-DMX-18", "physical.observation", ["local_web.action", "stress.auto"], "While bounded Stage Device state/config traffic and protected read-only local-web activity ran together, real DMX output showed no visible flicker/jitter/dropout and the fixed level stayed stable."),
    ("Q-DMX-19", "physical.observation", ["local_blackout.action", "emergency.post"], "With StageCore Hub confirmed unavailable, the protected local emergency control visibly forced real lighting to blackout; after Hub recovery no stale brightness returned."),
    ("Q-DMX-22", "physical.observation", ["regression.prereqs"], "Full pinned Raspberry Pi + Stage LAN + ESP32 + MAX485/DMX decoder + 24 V lighting chain completed representative control/regression with no visible unsafe behavior."),
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
            if milestone_status(gate, obs_key) not in {"PASS", "FAIL"}:
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


def apply_result(state, rows, status, note):
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
            "status": status,
            "updated_at": timestamp,
            "actor": "manual-physical-confirmation",
            "evidence": ["physical-observation"],
            "note": note,
            "history": history,
        }
        gate.setdefault("history", []).append({
            "status": gate.get("status"),
            "updated_at": gate.get("updated_at"),
            "actor": gate.get("actor"),
            "evidence": gate.get("evidence", []),
            "note": gate.get("note", ""),
        })
        gate["status"] = status
        gate["updated_at"] = timestamp
        gate["actor"] = "manual-physical-confirmation"
        gate["evidence"] = ["physical-observation"]
        gate["note"] = note
    state["updated_at"] = timestamp


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
        apply_result(state, rows, args.status, args.note.strip())
        atomic_write(state_path, state)
    print(f"Recorded {args.status} for {len(rows)} physical confirmation(s).")


def cmd_confirm_one(args):
    if not args.note.strip():
        raise SystemExit("--note is required")
    state_path = Path(args.state)
    with lock_state(state_path):
        state = json.load(open(state_path, encoding="utf-8"))
        rows = [row for row in eligible(state) if row[0] == args.gate]
        if len(rows) != 1:
            raise SystemExit(f"{args.gate} is not an eligible pending physical confirmation")
        apply_result(state, rows, args.status, args.note.strip())
        atomic_write(state_path, state)
    print(f"Recorded {args.status} for {args.gate}.")


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
    one = sub.add_parser("confirm-one")
    one.add_argument("--state", required=True)
    one.add_argument("--gate", required=True)
    one.add_argument("--status", choices=("PASS", "FAIL"), required=True)
    one.add_argument("--note", required=True)
    one.set_defaults(func=cmd_confirm_one)
    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
