"""Bounded Pi-native physical READ-ONLY probe for three manifest gates.

No interactive shell, Hub restart/update, device commands, fault injection,
manual confirmations, Pi sudo, network probing or privileged DB access.
A separate root-owned timer exports a private canonical device snapshot.
"""
import datetime as dt
import json
from pathlib import Path
import re
import shutil
import subprocess
import sys
import time

import agent
import import_campaign
import verify_campaign
import export_summary

ROOT = Path("/opt/stagecore-qualification-control")
STATE_TOOL = ROOT / "qualification-state.py"
MILESTONE_TOOL = ROOT / "qualification-milestone.py"
ASSERT_TOOL = ROOT / "assert-device-probe.py"
MANIFEST = ROOT / "manifest.json"
SNAPSHOT = Path("/run/stagecore-qualification-probe/current.json")
TARGETS = ROOT / "targets.json"
GATES = ("Q-TAB-04", "Q-TAB-05", "Q-DMX-20")

# Only explicitly authoritative semantic assertions can be classified FAIL.
# An unexpected probe/script/process failure remains BLOCKED, not a product FAIL.
KNOWN_FAIL = (
    "device disabled", "protocol mismatch", "device kind mismatch",
    "missing capabilities", "observed project mismatch", "observed snapshot mismatch",
    "lighting observation schema mismatch", "lighting DMX is not healthy",
    "lighting brownout warning active", "lighting authority=", "lighting configuration hash missing",
)


def targets():
    if not TARGETS.exists():
        return {"project_id": "", "runtime_snapshot_id": "", "tablet_id": "", "lighting_id": ""}
    if TARGETS.is_symlink() or TARGETS.stat().st_size > 4096:
        raise ValueError("unsafe target configuration")
    data = json.loads(TARGETS.read_text(encoding="utf-8"))
    keys = {"project_id", "runtime_snapshot_id", "tablet_id", "lighting_id"}
    if not isinstance(data, dict) or set(data) != keys:
        raise ValueError("invalid target keys")
    for key, value in data.items():
        if not isinstance(value, str) or len(value) > 128 or (
                value and not re.fullmatch(r"[A-Za-z0-9._:-]{1,128}", value)):
            raise ValueError("invalid target " + key)
    return data


def command(argv, timeout=10):
    return subprocess.run(argv, capture_output=True, text=True,
                          timeout=timeout, check=False)


def classify(result):
    if result.returncode == 0:
        return "PASS"
    if result.returncode == 3:
        return "BLOCKED"
    if result.returncode == 1 and any(token in result.stdout for token in KNOWN_FAIL):
        return "FAIL"
    return "BLOCKED"


def run_assert(probe_path, kind, check, target):
    args = [sys.executable, str(ASSERT_TOOL), "--input", str(probe_path),
            "--kind", kind, "--check", check, "--max-age-seconds", "20",
            "--expect-online", "unknown"]
    if target["project_id"]:
        args.extend(["--project-id", target["project_id"]])
    key = "tablet_id" if kind == "tablet" else "lighting_id"
    if target[key]:
        args.extend(["--device-id", target[key]])
    if check == "scope":
        args.extend(["--runtime-snapshot-id", target["runtime_snapshot_id"]])
    return command(args, timeout=10)


def record(state_file, gate_id, status, evidence, note, milestone=""):
    argv = [sys.executable, str(MILESTONE_TOOL if milestone else STATE_TOOL),
            "record", "--state", str(state_file), "--manifest", str(MANIFEST),
            "--gate", gate_id, "--status", status, "--actor", "pi-readonly-probe",
            "--evidence", str(evidence), "--note", note]
    if milestone:
        argv.extend(["--key", milestone])
    result = command(argv, timeout=10)
    if result.returncode != 0:
        raise RuntimeError("campaign update failed for " + gate_id)


def run_readonly(config, issue_number, *, manifest=MANIFEST, probe=SNAPSHOT):
    if type(issue_number) is not int or issue_number <= 0:
        raise ValueError("invalid issue number")
    guarded = verify_campaign.verify_live_campaign(config, manifest_path=manifest)
    if guarded["qualification"] != "CANONICAL_CAMPAIGN_IMPORTED_READONLY_PREFLIGHT":
        return guarded
    if not guarded["installed_binary_matches"] or guarded["hub_service"] != "active" or not guarded["hub_ready"]:
        return {"qualification": "PI_HUB_NOT_READY_NO_MUTATION",
                "candidate_sha": config["pinned_sha"]}
    try:
        if probe.is_symlink() or not probe.is_file():
            raise FileNotFoundError("canonical probe unavailable")
        probe_info = probe.stat()
        if time.time() - probe_info.st_mtime > 150:
            return {"qualification": "PROBE_SNAPSHOT_MISSING_OR_STALE_NO_MUTATION",
                    "candidate_sha": config["pinned_sha"]}
        if probe_info.st_size > 1024 * 1024:
            return {"qualification": "PROBE_TOO_LARGE_NO_MUTATION",
                    "candidate_sha": config["pinned_sha"]}
        probe_bytes = probe.read_bytes()
    except (OSError, ValueError):
        return {"qualification": "PROBE_UNREADABLE_NO_MUTATION",
                "candidate_sha": config["pinned_sha"]}
    try:
        settings = targets()
    except (OSError, ValueError, TypeError, KeyError):
        return {"qualification": "INVALID_TARGET_CONFIGURATION_NO_MUTATION",
                "candidate_sha": config["pinned_sha"]}

    state_dir = Path(config["state_dir"])
    campaign = state_dir / "campaign.json"
    try:
        snapshot = json.loads(probe_bytes)
    except (ValueError, TypeError):
        return {"qualification": "INVALID_PROBE_SNAPSHOT_NO_MUTATION",
                "candidate_sha": config["pinned_sha"]}
    if snapshot.get("schema_version") != 1 or not isinstance(snapshot.get("devices"), list):
        return {"qualification": "INVALID_PROBE_SNAPSHOT_NO_MUTATION",
                "candidate_sha": config["pinned_sha"]}
    evidence_dir = state_dir / "evidence"
    evidence_dir.mkdir(mode=0o700, exist_ok=True)
    if evidence_dir.stat().st_mode & 0o077:
        raise ValueError("evidence directory is not private")
    path = evidence_dir / ("issue-" + str(issue_number) + "-devices.json")
    if path.exists():
        # A repeated request must not switch its evidence to a newer probe.
        if path.is_symlink():
            raise ValueError("unsafe previous evidence")
    else:
        with path.open("xb") as out:
            out.write(probe_bytes)
        path.chmod(0o600)
    outcomes = {}
    existing = json.loads(campaign.read_text(encoding="utf-8"))["gates"]

    def already_recorded_this_request(gate):
        item = existing[gate]
        return (item.get("actor") == "pi-readonly-probe" and
                str(path) in item.get("evidence", []) and
                item.get("status") in ("PASS", "FAIL", "BLOCKED"))

    def apply(gate, status, message):
        if existing[gate]["status"] in ("PASS", "N/A") or already_recorded_this_request(gate):
            outcomes[gate] = existing[gate]["status"]
            return
        record(campaign, gate, status, path, message)
        outcomes[gate] = status

    if existing["Q-TAB-04"]["status"] in ("PASS", "N/A") or already_recorded_this_request("Q-TAB-04"):
        outcomes["Q-TAB-04"] = existing["Q-TAB-04"]["status"]
    else:
        tablet_result = run_assert(path, "tablet", "readiness", settings)
        tablet_status = classify(tablet_result)
        apply("Q-TAB-04", tablet_status, "Pi canonical read-only tablet readiness")

    if existing["Q-TAB-05"]["status"] in ("PASS", "N/A") or already_recorded_this_request("Q-TAB-05"):
        outcomes["Q-TAB-05"] = existing["Q-TAB-05"]["status"]
    elif outcomes["Q-TAB-04"] != "PASS" or not settings["project_id"] or not settings["runtime_snapshot_id"]:
        apply("Q-TAB-05", "BLOCKED", "Exact operator-confirmed project and snapshot plus READY Tablet required")
    else:
        scope_result = run_assert(path, "tablet", "scope", settings)
        apply("Q-TAB-05", classify(scope_result), "Pi canonical read-only Tablet scope assertion")

    if existing["Q-DMX-20"]["status"] in ("PASS", "N/A") or already_recorded_this_request("Q-DMX-20"):
        outcomes["Q-DMX-20"] = existing["Q-DMX-20"]["status"]
    else:
        lighting_result = run_assert(path, "lighting", "observation", settings)
        lighting_status = classify(lighting_result)
        old_milestone = (existing["Q-DMX-20"].get("milestones") or {}).get("observation.readiness", {})
        if old_milestone.get("status") != "PASS" and not (
                old_milestone.get("actor") == "pi-readonly-probe" and
                str(path) in old_milestone.get("evidence", [])):
            record(campaign, "Q-DMX-20", lighting_status, path,
                   "Pi read-only canonical lighting readiness/observation",
                   milestone="observation.readiness")
        if lighting_status == "FAIL":
            apply("Q-DMX-20", "FAIL", "Canonical lighting profile/health mismatch")
        else:
            apply("Q-DMX-20", "BLOCKED",
                  "Authoritative published lighting configuration and physical prerequisites not yet verified")
    # No previous PASS/N/A is overwritten, and no manual/physical gate can PASS.
    report = export_summary.export_summary(campaign, manifest)
    report["provenance"] = "PI_READONLY_PROBE_WITH_IMPORTED_BASELINE"
    export_summary.save(report, state_dir / "campaign-summary.json")
    return {
        "qualification": "PI_READONLY_PROBE_EXECUTED_NOT_FULL_QUALIFICATION",
        "candidate_sha": config["pinned_sha"],
        "campaign_id": guarded["campaign_id"],
        "processed_gates": outcomes,
        "total": report["total"],
        "completed": report["completed"],
        "counts": report["counts"],
        "manual_physical_actions": "NOT_EXECUTED",
        "evidence": "PRIVATE_PI_LOCAL_ONLY",
    }
