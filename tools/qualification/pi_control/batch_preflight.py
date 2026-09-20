#!/usr/bin/env python3
"""Read-only, redacted cross-group Pi preflight for the current 79-gate campaign.

Do not treat this snapshot as physical gate PASS. No SQLite writes, GitHub
authentication, device commands, output changes, restarts, or credential reads.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import sqlite3
import subprocess
import sys
import urllib.request
import urllib.error

import triage_all_79 as triage

DB = Path("/var/lib/stagecore/data/db/stagecore.sqlite3")
HUB = Path("/opt/stagecore/bin/stagecore-hub")
HEALTH = "http://127.0.0.1:7840/health/ready"
INTERESTING_TABLES = (
    "projects", "runtime_snapshots", "stage_devices", "live_video_sources",
    "network_observations", "sessions", "runtime_snapshots", "cue_executions",
)
DEVICE_KIND_ALLOWLIST = ("TABLET_PLAYER", "STAGE_DISPLAY", "GENERIC", "LIGHTING_NODE")
SNAPSHOT_STATES = ("PUBLISHED", "DRAFT", "SUPERSEDED", "INVALIDATED")


def file_hash(path):
    if path.is_symlink() or not path.is_file():
        return None
    hash_ = hashlib.sha256()
    with path.open("rb") as stream:
        for piece in iter(lambda: stream.read(65536), b""):
            hash_.update(piece)
    return hash_.hexdigest()


def safe_service_status():
    try:
        out = subprocess.run(
            ["/usr/bin/systemctl", "is-active", "stagecore-hub.service"],
            stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True,
            timeout=5, check=False)
        status = out.stdout.strip()
        return status if status in ("active", "inactive", "failed", "activating", "deactivating") else "UNKNOWN"
    except (OSError, subprocess.TimeoutExpired):
        return "UNKNOWN"


def safe_health():
    try:
        with urllib.request.urlopen(HEALTH, timeout=4) as response:
            data = json.loads(response.read(16384))
        status, storage = data.get("status"), data.get("storage_state")
        return {"status": status if status in ("READY", "NOT_READY", "DEGRADED") else "UNKNOWN",
                "storage": storage if storage in ("HEALTHY", "DEGRADED", "UNAVAILABLE") else "UNKNOWN"}
    except (OSError, ValueError, urllib.error.URLError):
        return {"status": "UNAVAILABLE", "storage": "UNAVAILABLE"}


def database_inventory(path):
    if path.is_symlink() or not path.is_file():
        return {"availability": "UNAVAILABLE"}
    conn = sqlite3.connect("file:" + str(path) + "?mode=ro", uri=True, timeout=3)
    try:
        conn.execute("PRAGMA query_only = ON")
        names = {row[0] for row in conn.execute(
            "SELECT name FROM sqlite_master WHERE type='table'")}
        counts = {}
        for name in dict.fromkeys(INTERESTING_TABLES):
            if name in names:
                counts[name] = int(conn.execute('SELECT COUNT(*) FROM "' + name + '"').fetchone()[0])
            else:
                counts[name] = None
        published = {"PUBLISHED": 0, "OTHER": 0}
        lighting_bindings = 0
        manifest_invalid = 0
        if "runtime_snapshots" in names:
            columns = {row[1] for row in conn.execute('PRAGMA table_info("runtime_snapshots")')}
            if {"status", "manifest_json"} <= columns:
                for status, raw in conn.execute('SELECT status,manifest_json FROM "runtime_snapshots"'):
                    if status == "PUBLISHED":
                        published["PUBLISHED"] += 1
                        try:
                            manifest = json.loads(raw)
                            if not isinstance(manifest, dict):
                                raise ValueError("not an object")
                            lighting = manifest.get("lighting_nodes") or []
                            if not isinstance(lighting, list):
                                raise ValueError("bad lighting binding")
                            lighting_bindings += len(lighting)
                        except (ValueError, TypeError):
                            manifest_invalid += 1
                    else:
                        published["OTHER"] += 1
        enabled_by_kind = {}
        if "stage_devices" in names:
            columns = {row[1] for row in conn.execute('PRAGMA table_info("stage_devices")')}
            if {"device_kind", "enabled"} <= columns:
                for kind, enabled, amount in conn.execute(
                        'SELECT device_kind, enabled, COUNT(*) FROM "stage_devices" GROUP BY device_kind,enabled'):
                    label = kind if kind in DEVICE_KIND_ALLOWLIST else "OTHER"
                    group = enabled_by_kind.setdefault(label, {"enabled": 0, "disabled": 0})
                    group["enabled" if enabled == 1 else "disabled"] += int(amount)
        return {"availability": "READONLY", "row_counts": counts,
                "published_snapshots": published,
                "published_lighting_bindings": lighting_bindings,
                "invalid_published_manifest_count": manifest_invalid,
                "device_kinds": enabled_by_kind,
                "integrity_check": "NOT_EXECUTED",
                "rollback_restore_test": "NOT_EXECUTED"}
    finally:
        conn.close()


def collect(manifest, manifest_raw, campaign, probe, targets, *, db=DB, hub=HUB):
    triage_summary = triage.summarize(manifest, manifest_raw, campaign, probe, targets)
    return {"mode": "BATCH_PREFLIGHT_READONLY_NOT_QUALIFICATION",
            "candidate_sha": triage.EXPECTED_HUB_SHA,
            "campaign_id": triage_summary["campaign_id"],
            "campaign_counts": triage_summary["counts"],
            "candidate_binary_sha256_matches": file_hash(hub) == "34681e3ce7595d4caf63f1a306f496f4172f398065a2fe7885bc2bacc6ce7bfe",
            "hub_service": safe_service_status(),
            "hub_health": safe_health(),
            "database": database_inventory(db),
            "device_facts": triage_summary["device_facts"],
            "targets_configured": triage_summary["targets_configured"],
            "independent_gate_evidence": {
                "Q-SYS-01": "FINAL_MAIN_CANDIDATE_FREEZE_NOT_VERIFIED",
                "Q-SYS-02": "GITHUB_EXACT_HEAD_CI_SEPARATELY_VERIFIED_NOT_FINAL_GATE_PASS",
                "Q-SYS-03": "CLIENT_BUILD_CI_AVAILABLE_INSTALLED_BUILD_NOT_VERIFIED",
                "Q-SYS-04": "OFFLINE_MEDIA_BUILD_ARTIFACT_NOT_VERIFIED",
                "Q-SYS-05": "BACKUP_AND_ROLLBACK_BASELINE_NOT_VERIFIED",
                "Q-SYS-06": "CONTROLLED_FINAL_UPDATE_NOT_VERIFIED",
                "Q-SYS-07": "INSTALLED_HEALTH_ONLY_RESTART_AND_NO_REPLAY_UNVERIFIED",
                "Q-TAB-01": "PHYSICAL_APK_INSTALLATION_NOT_VERIFIED",
                "Q-TAB-02": "EXACT_INSTALLED_APK_HASH_NOT_VERIFIED",
                "Q-DMX-20": "PUBLISHED_BINDING_AND_REAL_APPLIED_HASH_MUST_MATCH",
            },
            "campaign_modified": False, "physical_actions_executed": False}


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--manifest", default="/opt/stagecore-qualification-control/manifest.json")
    p.add_argument("--campaign", default="/var/lib/stagecore-control/campaign.json")
    p.add_argument("--probe", default="/run/stagecore-qualification-probe/current.json")
    p.add_argument("--targets", default="/opt/stagecore-qualification-control/targets.json")
    p.add_argument("--db", default=str(DB))
    p.add_argument("--hub", default=str(HUB))
    args = p.parse_args()
    raw = Path(args.manifest).read_bytes()
    manifest = json.loads(raw)
    campaign = triage.read_json(args.campaign, 10 * 1024 * 1024)
    probe = triage.read_json(args.probe, 1024 * 1024) if Path(args.probe).is_file() else None
    targets = triage.read_json(args.targets, 4096) if Path(args.targets).is_file() else None
    output = collect(manifest, raw, campaign, probe, targets, db=Path(args.db), hub=Path(args.hub))
    print(json.dumps(output, indent=2, sort_keys=True))
    print("BATCH_PREFLIGHT_COMPLETE: no Hub/device mutation and no physical gate PASS")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, sqlite3.Error, TypeError, KeyError) as exc:
        print("BATCH_PREFLIGHT_STOP: " + type(exc).__name__, file=sys.stderr)
        raise SystemExit(3)
