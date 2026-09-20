#!/usr/bin/env python3
"""Import a canonical, exact-SHA qualification campaign without rewriting its history.

Only the private campaign JSON moves here. Source run evidence stays on Mac
until a separately reviewed integrity-checked transfer is implemented.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import tempfile

SHA = "809d1f4ce7a5824c27ecf82d78cd779cd28f6e6a"
MANIFEST_SHA = "0aaa67702940cf85f97d99a2b45df7e5330238b36643393950eb0b08a1836dad"
STATUS = {"PENDING", "PASS", "FAIL", "BLOCKED", "N/A"}


def verify_campaign(state, manifest_data, manifest_raw):
    if hashlib.sha256(manifest_raw).hexdigest() != MANIFEST_SHA:
        raise ValueError("wrong canonical manifest")
    if state.get("schema_version") != 1 or state.get("manifest_sha256") != MANIFEST_SHA:
        raise ValueError("campaign schema/manifest mismatch")
    if state.get("pins", {}).get("stagecore_sha") != SHA:
        raise ValueError("campaign does not match the deployed Hub candidate")
    if not isinstance(state.get("campaign_id"), str) or not re.fullmatch(r"[0-9TZ:+.\\-]{1,128}", state["campaign_id"]):
        raise ValueError("missing/invalid campaign identity")
    manifest_gates = {g["id"]: g for group in manifest_data.get("groups", [])
                      for g in group.get("gates", [])}
    if len(manifest_gates) != 79 or set(state.get("gates", {})) != set(manifest_gates):
        raise ValueError("campaign gate inventory differs from canonical 79-gate manifest")
    completed = 0
    for gate_id, gate in state["gates"].items():
        if not isinstance(gate, dict) or gate.get("status") not in STATUS:
            raise ValueError("invalid gate status")
        if gate.get("method") != manifest_gates[gate_id]["method"]:
            raise ValueError("gate method mismatch")
        if gate["status"] in ("PASS", "N/A"):
            completed += 1
        for milestone in (gate.get("milestones") or {}).values():
            if not isinstance(milestone, dict) or milestone.get("status") not in STATUS:
                raise ValueError("malformed milestone")
    return {"campaign_id": state["campaign_id"], "completed": completed, "total": 79}


def immutable_import(state_path, manifest_path, target):
    src, manifest, dst = Path(state_path), Path(manifest_path), Path(target)
    if not src.is_file() or src.is_symlink() or src.stat().st_size > 10 * 1024 * 1024:
        raise ValueError("unsafe or oversized campaign input")
    if not manifest.is_file() or manifest.is_symlink() or manifest.stat().st_size > 256 * 1024:
        raise ValueError("unsafe manifest input")
    state_raw, manifest_raw = src.read_bytes(), manifest.read_bytes()
    state, manifest_data = json.loads(state_raw), json.loads(manifest_raw)
    summary = verify_campaign(state, manifest_data, manifest_raw)
    if dst.is_symlink():
        raise ValueError("campaign destination must not be a symlink")
    if dst.exists():
        if dst.read_bytes() != state_raw:
            raise ValueError("existing campaign differs; preserve it, do not overwrite/repin")
        return summary, "EXISTING_IDENTICAL"
    dst.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    if dst.parent.stat().st_mode & 0o077:
        raise ValueError("campaign directory is not private")
    fd, tmp = tempfile.mkstemp(prefix=".campaign-import-", dir=str(dst.parent))
    try:
        with os.fdopen(fd, "wb") as f:
            f.write(state_raw)
            f.flush()
            os.fsync(f.fileno())
        os.chmod(tmp, 0o600)
        # Exclusive create prevents overwriting a concurrently created campaign.
        os.link(tmp, dst)
    finally:
        if os.path.exists(tmp):
            os.unlink(tmp)
    return summary, "IMPORTED"


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--state", required=True)
    p.add_argument("--manifest", required=True)
    p.add_argument("--output", required=True)
    args = p.parse_args()
    result, action = immutable_import(args.state, args.manifest, args.output)
    print(action + ": canonical campaign " + result["campaign_id"] +
          " " + str(result["completed"]) + "/" + str(result["total"]))
    print("NOTE: source evidence remains on Mac; no physical gates were executed.")


if __name__ == "__main__":
    main()
