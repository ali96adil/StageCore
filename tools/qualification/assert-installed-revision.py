#!/usr/bin/env python3
"""Fail closed if the installed Pi Hub does not match the pinned runner HEAD."""
import argparse
import re
from pathlib import Path

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", required=True)
    parser.add_argument("--expected", required=True)
    args = parser.parse_args()
    expected = args.expected.strip().lower()
    if re.fullmatch(r"[0-9a-f]{40}", expected) is None:
        parser.error("the repository HEAD must be a full 40-character Git SHA")
    text = Path(args.input).read_text(encoding="utf-8")
    matches = re.findall(r"\bvcs\.revision=([0-9a-fA-F]{40})\b", text)
    matches = {m.lower() for m in matches}
    if len(matches) != 1:
        print("BLOCKED: installed Hub VCS revision missing/ambiguous; verify exact release provenance")
        return 3
    installed = next(iter(matches))
    if installed != expected:
        print("BLOCKED: installed Hub revision differs from qualification candidate; deploy the pinned release before physical tests")
        print("expected=" + expected)
        print("installed=" + installed)
        return 3
    if re.search(r"\bvcs\.modified=true\b", text):
        print("BLOCKED: installed Hub build reports local source modifications")
        return 3
    print("PASS: installed Hub matches qualification candidate " + installed)
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
