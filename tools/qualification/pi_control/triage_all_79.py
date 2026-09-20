#!/usr/bin/env python3
"""Read-only, fail-closed triage of ALL 79 canonical StageCore qualification gates.

This is a complete 79-gate INVENTORY and prerequisite assessment, not the
execution of physical or potentially disruptive gates. No DB writes, networking,
GitHub token, device command, Hub service change, or campaign mutation.
"""
import argparse
from collections import Counter
import hashlib
import json
from pathlib import Path
import sys
import time

EXPECTED_HUB_SHA = "809d1f4ce7a5824c27ecf82d78cd779cd28f6e6a"
EXPECTED_MANIFEST_SHA256 = "0aaa67702940cf85f97d99a2b45df7e5330238b36643393950eb0b08a1836dad"
RESULTS = ("PASS", "FAIL", "BLOCKED", "PENDING", "N/A")
METHODS = ("EVIDENCE", "AUTO", "AUTO_PHYSICAL", "MANUAL")
TABLET = "stagecore.tablet-player"
DMX = "stagecore.esp32-dmx-lighting-node"


def read_json(path, limit):
    path = Path(path)
    if path.is_symlink() or not path.is_file() or path.stat().st_size > limit:
        raise ValueError("missing, unsafe, or oversized input: " + path.name)
    return json.loads(path.read_text(encoding="utf-8"))


def summarize(manifest, raw_manifest, campaign, probe=None, targets=None, now=None):
    now = time.time() if now is None else now
    if hashlib.sha256(raw_manifest).hexdigest() != EXPECTED_MANIFEST_SHA256:
        raise ValueError("canonical 79-gate manifest digest mismatch")
    if (campaign.get("schema_version") != 1 or
            campaign.get("manifest_sha256") != EXPECTED_MANIFEST_SHA256 or
            campaign.get("pins", {}).get("stagecore_sha") != EXPECTED_HUB_SHA):
        raise ValueError("existing campaign/candidate mismatch; do not repin or reset")
    if not isinstance(campaign.get("campaign_id"), str):
        raise ValueError("missing campaign identity")
    groups = manifest.get("groups")
    if not isinstance(groups, list):
        raise ValueError("invalid group inventory")
    definitions = {}
    by_group = []
    for group in groups:
        definitions_in_group = group["gates"]
        for item in definitions_in_group:
            if item["id"] in definitions or item["method"] not in METHODS:
                raise ValueError("duplicate gate or unknown method")
            definitions[item["id"]] = item
        by_group.append((group["id"], definitions_in_group))
    gates = campaign.get("gates")
    if len(definitions) != 79 or not isinstance(gates, dict) or set(gates) != set(definitions):
        raise ValueError("canonical gate inventory does not contain exactly the same 79 gates")
    counts = Counter()
    device_facts = {"probe": "MISSING_OR_INVALID", "tablet_registered": 0,
                    "tablet_online_ready": 0, "lighting_registered": 0,
                    "lighting_online": 0, "lighting_failsafe": 0,
                    "lighting_configuration_hash_present": 0,
                    "lighting_dmx_healthy": 0, "lighting_readiness_blocker": 0,
                    "fresh_observations": 0}
    if isinstance(probe, dict) and probe.get("schema_version") == 1 and isinstance(probe.get("devices"), list):
        device_facts["probe"] = "CANONICAL_SNAPSHOT"
        for device in probe["devices"]:
            if not isinstance(device, dict):
                raise ValueError("invalid snapshot device")
            profile = device.get("profile_id")
            runtime = device.get("runtime") or {}
            if not isinstance(runtime, dict):
                raise ValueError("invalid device runtime")
            last = runtime.get("last_seen_at_us")
            fresh = type(last) is int and 0 <= now - last / 1000000 <= 20
            if fresh:
                device_facts["fresh_observations"] += 1
            if profile == TABLET:
                device_facts["tablet_registered"] += 1
                if (device.get("enabled") is True and
                        runtime.get("connection_state") == "ONLINE" and
                        runtime.get("readiness") == "READY" and fresh):
                    device_facts["tablet_online_ready"] += 1
            elif profile == DMX:
                device_facts["lighting_registered"] += 1
                device_facts["lighting_online"] += int(runtime.get("connection_state") == "ONLINE" and fresh)
                device_facts["lighting_readiness_blocker"] += int(runtime.get("readiness") == "BLOCKER")
                observed = runtime.get("observed") or {}
                if not isinstance(observed, dict):
                    raise ValueError("invalid lighting observation")
                device_facts["lighting_failsafe"] += int(observed.get("authority") == "FAILSAFE")
                device_facts["lighting_dmx_healthy"] += int(observed.get("dmx_healthy") is True)
                device_facts["lighting_configuration_hash_present"] += int(bool(observed.get("configuration_hash")))
    target_keys = ("project_id", "runtime_snapshot_id", "tablet_id", "lighting_id")
    if targets is not None and (not isinstance(targets, dict) or set(targets) != set(target_keys)):
        raise ValueError("invalid target schema")
    configured_targets = {key: bool((targets or {}).get(key)) for key in target_keys}

    rows, summary_groups = [], []
    for group_id, gate_defs in by_group:
        gc = Counter()
        for item in gate_defs:
            gid = item["id"]
            saved = gates[gid]
            status = saved.get("status")
            if status not in RESULTS or saved.get("method") != item["method"]:
                raise ValueError("gate status or method mismatch for " + gid)
            method = item["method"]
            counts[status] += 1
            gc[status] += 1
            if status in ("PASS", "N/A"):
                action = "PRESERVE_RECORDED_RESULT"
            elif gid.startswith("Q-TAB-") and device_facts["tablet_registered"] == 0:
                action = "TABLET_REGISTRATION_REQUIRED"
            elif gid in ("Q-TAB-04", "Q-TAB-05") and device_facts["tablet_online_ready"] == 0:
                action = "TABLET_ONLINE_READY_REQUIRED"
            elif gid == "Q-TAB-05" and not all(configured_targets[k] for k in ("project_id", "runtime_snapshot_id")):
                action = "TRUSTED_PROJECT_AND_SNAPSHOT_PINS_REQUIRED"
            elif gid == "Q-DMX-20" and (
                    device_facts["lighting_registered"] == 0 or
                    device_facts["lighting_online"] == 0 or
                    device_facts["lighting_failsafe"] or
                    not device_facts["lighting_configuration_hash_present"]):
                action = "LIGHTING_READINESS_AND_PUBLISHED_CONFIG_REQUIRED"
            elif method == "EVIDENCE":
                action = "PINNED_RELEASE_AND_CI_EVIDENCE_REVIEW"
            elif method == "MANUAL":
                action = "OPERATOR_PHYSICAL_CONFIRMATION_REQUIRED"
            elif method == "AUTO_PHYSICAL":
                action = "REVIEWED_ARMED_PHYSICAL_TEST_REQUIRED"
            else:
                action = "REVIEWED_AUTO_GATE_HANDLER_REQUIRED"
            rows.append({"gate": gid, "group": group_id, "method": method,
                         "status": status, "next": action})
        summary_groups.append({"id": group_id, "total": len(gate_defs),
                               "counts": {k: gc[k] for k in RESULTS}})
    result = {
        "mode": "READONLY_FULL_INVENTORY_NOT_PHYSICAL_QUALIFICATION",
        "candidate_sha": EXPECTED_HUB_SHA,
        "campaign_id": campaign["campaign_id"],
        "total": len(rows),
        "completed": counts["PASS"] + counts["N/A"],
        "counts": {k: counts[k] for k in RESULTS},
        "groups": summary_groups,
        "device_facts": device_facts,
        "targets_configured": configured_targets,
        "next_action_counts": dict(sorted(Counter(row["next"] for row in rows).items())),
        "gates": rows,
        "disruptive_actions_executed": False,
        "campaign_modified": False,
    }
    return result


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--manifest", default="/opt/stagecore-qualification-control/manifest.json")
    p.add_argument("--campaign", default="/var/lib/stagecore-control/campaign.json")
    p.add_argument("--probe", default="/run/stagecore-qualification-probe/current.json")
    p.add_argument("--targets", default="/opt/stagecore-qualification-control/targets.json")
    p.add_argument("--json", action="store_true", help="print sanitized JSON, including all 79 gate statuses")
    args = p.parse_args()
    raw = Path(args.manifest).read_bytes()
    manifest = json.loads(raw)
    campaign = read_json(args.campaign, 10 * 1024 * 1024)
    probe = read_json(args.probe, 1024 * 1024) if Path(args.probe).exists() else None
    targets = read_json(args.targets, 4096) if Path(args.targets).exists() else None
    summary = summarize(manifest, raw, campaign, probe=probe, targets=targets)
    if args.json:
        print(json.dumps(summary, indent=2, sort_keys=True))
        return
    print("MODE:", summary["mode"])
    print("CAMPAIGN:", summary["campaign_id"], "COMPLETED:", str(summary["completed"]) + "/79")
    print("COUNTS:", " ".join(k + "=" + str(v) for k, v in summary["counts"].items()))
    for group in summary["groups"]:
        print("GROUP", group["id"], " ".join(k+"="+str(v) for k,v in group["counts"].items()))
    print("FACTS:", json.dumps(summary["device_facts"], sort_keys=True))
    print("TARGETS_CONFIGURED:", json.dumps(summary["targets_configured"], sort_keys=True))
    print("===== 79 GATE TRIAGE =====")
    for row in summary["gates"]:
        print(row["gate"], row["status"], row["method"], row["next"], sep="\t")
    print("ACTION_CLUSTERS:", json.dumps(summary["next_action_counts"], sort_keys=True))
    print("READONLY_TRIAGE_COMPLETE: no stage output, fault injection, credential use or campaign writes")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, TypeError, KeyError) as exc:
        print("TRIAGE_STOP: " + str(exc), file=sys.stderr)
        raise SystemExit(3)
