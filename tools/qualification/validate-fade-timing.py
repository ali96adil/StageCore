#!/usr/bin/env python3
import argparse
import json
import math
import sys


def fail(message):
    print(message, file=sys.stderr)
    raise SystemExit(1)


def main():
    parser = argparse.ArgumentParser(description="Validate measured StageCore lighting fade lifecycle timing")
    parser.add_argument("--input", required=True)
    parser.add_argument("--expected-ms", required=True, type=float)
    parser.add_argument("--tolerance-ms", required=True, type=float)
    args = parser.parse_args()

    if not math.isfinite(args.expected_ms) or args.expected_ms <= 0:
        fail("expected-ms must be positive")
    if not math.isfinite(args.tolerance_ms) or args.tolerance_ms < 0:
        fail("tolerance-ms must be non-negative")

    try:
        evidence = json.load(open(args.input, encoding="utf-8"))
    except Exception as exc:
        fail(f"invalid timing evidence: {exc}")

    if evidence.get("status") != "COMPLETED":
        fail(f"command did not complete: {evidence.get('status')}")
    measured = evidence.get("lifecycle_ms")
    if isinstance(measured, bool) or not isinstance(measured, (int, float)) or not math.isfinite(float(measured)):
        fail("lifecycle_ms is missing or invalid")
    measured = float(measured)
    delta = measured - args.expected_ms
    within = abs(delta) <= args.tolerance_ms

    result = {
        "status": "PASS" if within else "FAIL",
        "command_id": evidence.get("command_id"),
        "expected_ms": args.expected_ms,
        "measured_ms": measured,
        "delta_ms": delta,
        "tolerance_ms": args.tolerance_ms,
    }
    print(json.dumps(result, sort_keys=True))
    if not within:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
