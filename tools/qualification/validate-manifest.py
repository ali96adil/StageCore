#!/usr/bin/env python3
import argparse
import json
import sys

ALLOWED_METHODS = {"AUTO", "AUTO_PHYSICAL", "MANUAL", "EVIDENCE"}
ALLOWED_ISSUES = {138, 148, 195, 221}

def fail(message):
    print("qualification manifest invalid:", message, file=sys.stderr)
    raise SystemExit(1)

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--manifest", default="tools/qualification/manifest.json")
    parser.add_argument("--summary", action="store_true")
    args = parser.parse_args()

    with open(args.manifest, "r", encoding="utf-8") as fh:
        data = json.load(fh)

    if data.get("schema_version") != 1:
        fail("schema_version must be 1")
    rules = data.get("rules") or {}
    if set(rules.get("methods") or []) != ALLOWED_METHODS:
        fail("rules.methods must match the supported method set")
    if set(rules.get("source_issues") or []) != ALLOWED_ISSUES:
        fail("rules.source_issues must match authoritative qualification trackers")

    groups = data.get("groups")
    if not isinstance(groups, list) or not groups:
        fail("groups must be a non-empty list")

    seen_groups=set()
    seen_gates=set()
    counts={m:0 for m in sorted(ALLOWED_METHODS)}
    source_counts={i:0 for i in sorted(ALLOWED_ISSUES)}
    for group in groups:
        gid=group.get("id")
        if not gid or gid in seen_groups:
            fail(f"invalid or duplicate group id {gid!r}")
        seen_groups.add(gid)
        issue=group.get("source_issue")
        if issue not in ALLOWED_ISSUES:
            fail(f"group {gid} has unsupported source issue {issue!r}")
        gates=group.get("gates")
        if not isinstance(gates,list) or not gates:
            fail(f"group {gid} has no gates")
        for gate in gates:
            gate_id=gate.get("id")
            if not isinstance(gate_id,str) or not gate_id.startswith("Q-") or gate_id in seen_gates:
                fail(f"invalid or duplicate gate id {gate_id!r}")
            seen_gates.add(gate_id)
            method=gate.get("method")
            if method not in ALLOWED_METHODS:
                fail(f"gate {gate_id} has unsupported method {method!r}")
            if not str(gate.get("acceptance") or "").strip():
                fail(f"gate {gate_id} has empty acceptance")
            if not str(gate.get("source_section") or "").strip():
                fail(f"gate {gate_id} has empty source_section")
            if not isinstance(gate.get("na_allowed"), bool):
                fail(f"gate {gate_id} na_allowed must be boolean")
            counts[method]+=1
            source_counts[issue]+=1

    if args.summary:
        print(json.dumps({
            "schema_version":1,
            "groups":len(groups),
            "gates":len(seen_gates),
            "methods":counts,
            "source_issues":source_counts,
        },sort_keys=True))
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
