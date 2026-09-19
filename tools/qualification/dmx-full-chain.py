#!/usr/bin/env python3
import argparse
import json
from pathlib import Path

DEPENDENCIES = [f"Q-DMX-{i:02d}" for i in range(1, 22)]


def main():
    parser = argparse.ArgumentParser(description="Evaluate Q-DMX-22 full-chain prerequisite truth")
    parser.add_argument("--state", required=True)
    parser.add_argument("--out", required=True)
    args = parser.parse_args()

    state = json.loads(Path(args.state).read_text(encoding="utf-8"))
    gates = state.get("gates", {})
    pins = state.get("pins", {})
    required_pins = ("stagecore_sha", "lighting_firmware_sha", "hardware_baseline_id")
    missing_pins = [key for key in required_pins if not str(pins.get(key) or "").strip()]

    rows = []
    failed = []
    incomplete = []
    for gate_id in DEPENDENCIES:
        gate = gates.get(gate_id)
        status = (gate or {}).get("status", "PENDING")
        rows.append({
            "gate_id": gate_id,
            "status": status,
            "method": (gate or {}).get("method", ""),
            "evidence": list((gate or {}).get("evidence", [])),
            "updated_at": (gate or {}).get("updated_at"),
        })
        if status == "FAIL":
            failed.append(gate_id)
        elif status not in {"PASS", "N/A"}:
            incomplete.append(gate_id)

    if failed:
        status = "FAIL"
        detail = "full-chain prerequisite gate(s) failed: " + ",".join(failed)
        code = 1
    elif missing_pins or incomplete:
        status = "BLOCKED"
        pieces = []
        if missing_pins:
            pieces.append("missing pins=" + ",".join(missing_pins))
        if incomplete:
            pieces.append("incomplete gates=" + ",".join(incomplete))
        detail = "; ".join(pieces)
        code = 3
    else:
        status = "PASS"
        detail = "all Q-DMX-01..21 prerequisite gates are terminal PASS/N/A on the pinned StageCore/firmware/hardware baseline"
        code = 0

    evidence = {
        "schema_version": 1,
        "gate_id": "Q-DMX-22",
        "status": status,
        "detail": detail,
        "pins": {key: pins.get(key, "") for key in required_pins},
        "dependencies": rows,
        "dependency_count": len(rows),
        "failed": failed,
        "incomplete": incomplete,
        "missing_pins": missing_pins,
    }
    Path(args.out).parent.mkdir(parents=True, exist_ok=True)
    Path(args.out).write_text(json.dumps(evidence, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(args.out)
    raise SystemExit(code)


if __name__ == "__main__":
    main()
