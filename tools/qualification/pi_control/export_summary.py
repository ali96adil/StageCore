#!/usr/bin/env python3
"""Export a non-sensitive, manifest-verified campaign summary without mutating state.

This export is a point-in-time status snapshot, NOT live Pi qualification evidence.
"""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import tempfile

# Source-tree layout differs from the root-owned Pi install directory.
# Prefer the co-installed audited report module; retain source-tree fallback.
DEPLOYED_REPORT_MODULE = Path(__file__).resolve().parent / "qualification-report.py"
SOURCE_REPORT_MODULE = Path(__file__).resolve().parent.parent / "qualification-report.py"
REPORT_MODULE = DEPLOYED_REPORT_MODULE if DEPLOYED_REPORT_MODULE.is_file() else SOURCE_REPORT_MODULE
ALLOWED = ("PASS", "FAIL", "BLOCKED", "PENDING", "N/A")


def export_summary(state_path, manifest_path):
    manifest_raw = Path(manifest_path).read_bytes()
    manifest = json.loads(manifest_raw)
    state = json.loads(Path(state_path).read_text(encoding="utf-8"))
    digest = hashlib.sha256(manifest_raw).hexdigest()
    if state.get("manifest_sha256") != digest:
        raise ValueError("manifest hash mismatch; no campaign evidence may be reused")
    pin = state.get("pins", {}).get("stagecore_sha")
    if not isinstance(pin, str) or len(pin) != 40 or any(c not in "0123456789abcdef" for c in pin):
        raise ValueError("campaign stagecore SHA is missing or malformed")
    spec = importlib.util.spec_from_file_location("stagecore_qualification_report", REPORT_MODULE)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    summary, _ = module.summarize(manifest, state)
    output = {
        "format": "stagecore-imported-campaign-summary-v1",
        "campaign_id": str(summary.get("campaign_id", ""))[:128],
        "candidate_sha": pin,
        "manifest_sha256": digest,
        "total": summary["total"],
        "completed": summary["completed"],
        "remaining": summary["remaining"],
        "counts": {status: summary["counts"][status] for status in ALLOWED},
        "groups": [{
            "id": str(group["id"])[:64],
            "total": group["total"],
            "completed": group["completed"],
            "counts": {status: group["counts"][status] for status in ALLOWED},
        } for group in summary["groups"]],
        "provenance": "MAC_IMPORTED_SNAPSHOT_NOT_LIVE_PI_EVIDENCE",
    }
    return output


def save(output, path):
    target = Path(path)
    target.parent.mkdir(parents=True, exist_ok=True)
    fd, temp = tempfile.mkstemp(prefix=".qualification-summary-", dir=str(target.parent))
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as stream:
            json.dump(output, stream, indent=2, sort_keys=True)
            stream.write("\n")
            stream.flush()
            os.fsync(stream.fileno())
        os.chmod(temp, 0o600)
        os.replace(temp, target)
    finally:
        if os.path.exists(temp):
            os.unlink(temp)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--state", required=True)
    parser.add_argument("--manifest", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    output = export_summary(args.state, args.manifest)
    save(output, args.output)
    print("Sanitized summary exported: " + str(output["completed"]) + "/" + str(output["total"]))
    print("Raw campaign and its evidence remain unchanged and local.")


if __name__ == "__main__":
    main()
