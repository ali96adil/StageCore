#!/usr/bin/env python3
"""Offline, redacted Mac LiveSource evidence checker; never changes campaign state."""
import argparse
import json
import sys
from pathlib import Path

STEPS = ("baseline", "opened", "routed", "loss", "recovered", "closed")
REQUIRED = ("project_id", "runtime_snapshot_id", "stagecore_sha", "source_id",
            "companion_id", "output_id", "layer_id")
MAX_BYTES = 65536


def check(data):
    if not isinstance(data, dict) or set(data) != {"identity", "steps", "observations"}:
        raise ValueError("expected identity, steps and observations only")
    identity, steps, observations = data["identity"], data["steps"], data["observations"]
    if not isinstance(identity, dict) or set(identity) != set(REQUIRED):
        raise ValueError("identity must contain exactly the pinned IDs")
    if any(not isinstance(identity[k], str) or not identity[k].strip() or len(identity[k]) > 256
           or any(ord(ch) < 32 for ch in identity[k]) for k in REQUIRED):
        raise ValueError("invalid pinned identity")
    if not isinstance(steps, dict) or set(steps) != set(STEPS):
        raise ValueError("six exact ordered steps required")
    if not isinstance(observations, dict) or set(observations) != {
        "moving_frames", "companion_local_render", "hub_control_only",
        "source_loss_detected", "moving_frames_recovered", "no_implicit_replay",
    }:
        raise ValueError("exact physical observations required")
    if any(type(v) is not bool for v in observations.values()):
        raise ValueError("observations must be explicit booleans")
    last = -1
    for name in STEPS:
        row = steps[name]
        if not isinstance(row, dict) or set(row) != {
            "at_us", "source_id", "result", "native_open", "renderer_attached",
            "readiness", "note",
        }:
            raise ValueError(name + ": exact redacted step fields required")
        at = row["at_us"]
        if type(at) is not int or at <= last or at <= 0:
            raise ValueError(name + ": non-increasing timestamp")
        last = at
        if row["source_id"] != identity["source_id"]:
            raise ValueError(name + ": source identity mismatch")
        if row["result"] not in ("COMPLETED", "FAILED", "OBSERVED", "UNKNOWN"):
            raise ValueError(name + ": invalid result")
        if type(row["native_open"]) is not bool or type(row["renderer_attached"]) is not bool:
            raise ValueError(name + ": native fields must be booleans")
        if row["readiness"] not in ("READY", "WARNING", "ADVISORY", "BLOCKER", "UNKNOWN"):
            raise ValueError(name + ": invalid readiness")
        if not isinstance(row["note"], str) or len(row["note"]) > 512:
            raise ValueError(name + ": invalid note")
    problems = []
    if steps["opened"]["result"] != "COMPLETED" or not steps["opened"]["native_open"]:
        problems.append("open not proven")
    if (steps["routed"]["result"] != "COMPLETED" or not steps["routed"]["native_open"]
            or not steps["routed"]["renderer_attached"]):
        problems.append("native renderer route not proven")
    if not observations["moving_frames"]:
        problems.append("moving frames not observed")
    if not observations["companion_local_render"] or not observations["hub_control_only"]:
        problems.append("Companion/Hub execution boundary not observed")
    if (steps["loss"]["readiness"] not in ("WARNING", "BLOCKER")
            or not observations["source_loss_detected"]):
        problems.append("source loss/readiness not proven")
    if (steps["recovered"]["readiness"] != "READY"
            or not steps["recovered"]["native_open"]
            or not steps["recovered"]["renderer_attached"]
            or not observations["moving_frames_recovered"]
            or not observations["no_implicit_replay"]):
        problems.append("recovery/no-replay not proven")
    if steps["closed"]["result"] != "COMPLETED" or steps["closed"]["renderer_attached"]:
        problems.append("safe close not proven")
    return {"status": "PASS" if not problems else "BLOCKED",
            "mode": "mac-live-evidence-review",
            "source_id": identity["source_id"],
            "stagecore_sha": identity["stagecore_sha"],
            "gates": {
                "Q-LIVE-01": "EVIDENCE_READY" if not any(x in problems for x in (
                    "open not proven", "native renderer route not proven", "moving frames not observed")) else "BLOCKED",
                "Q-LIVE-02": "EVIDENCE_READY" if "Companion/Hub execution boundary not observed" not in problems else "BLOCKED",
                "Q-LIVE-03": "EVIDENCE_READY" if not any(x in problems for x in (
                    "source loss/readiness not proven", "recovery/no-replay not proven",
                    "safe close not proven")) else "BLOCKED",
            },
            "problems": problems,
            "limitations": "Review of operator-supplied redacted evidence only; not an independent Companion measurement, campaign milestone, or physical PASS."}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("evidence", type=Path)
    args = parser.parse_args()
    if args.evidence.stat().st_size > MAX_BYTES:
        raise SystemExit("evidence too large")
    try:
        data = json.loads(args.evidence.read_text(encoding="utf-8"))
        result = check(data)
    except (ValueError, UnicodeError, OSError) as exc:
        print(json.dumps({"status": "FAIL", "detail": str(exc)}))
        raise SystemExit(1)
    print(json.dumps(result, sort_keys=True))
    raise SystemExit(0 if result["status"] == "PASS" else 3)


if __name__ == "__main__":
    main()
