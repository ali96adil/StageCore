"""Read-only guard for the imported canonical campaign on the Raspberry Pi."""
import hashlib
import json
from pathlib import Path
import re
import import_campaign

MANIFEST_FILE = Path("/opt/stagecore-qualification-control/manifest.json")


def verify_live_campaign(config, *, manifest_path=MANIFEST_FILE):
    """No writes, devices, sudo, or operator credential access."""
    state_dir = Path(config["state_dir"])
    campaign_path = state_dir / "campaign.json"
    if campaign_path.is_symlink() or not campaign_path.is_file():
        return {"qualification": "CANONICAL_CAMPAIGN_IMPORT_REQUIRED",
                "candidate_sha": config["pinned_sha"]}
    try:
        if campaign_path.stat().st_size > 10 * 1024 * 1024:
            raise ValueError("oversized campaign")
        if manifest_path.is_symlink() or not manifest_path.is_file():
            raise ValueError("canonical manifest missing")
        raw = manifest_path.read_bytes()
        state = json.loads(campaign_path.read_text(encoding="utf-8"))
        manifest = json.loads(raw)
        values = import_campaign.verify_campaign(state, manifest, raw)
        if config["pinned_sha"] != import_campaign.SHA:
            raise ValueError("agent pin does not match canonical campaign pin")
        # Read-only, pinned binary + liveness checks. An imported campaign is
        # never considered physical PASS on the strength of these checks.
        import agent
        hub = agent.read_status(config)
        counts = {x: 0 for x in ("PASS", "FAIL", "BLOCKED", "PENDING", "N/A")}
        source_references = 0
        for gate in state["gates"].values():
            counts[gate["status"]] += 1
            source_references += len(gate.get("evidence", []))
            for milestone in (gate.get("milestones") or {}).values():
                source_references += len(milestone.get("evidence", []))
        if not isinstance(state.get("created_at"), str) or len(state["created_at"]) > 80:
            raise ValueError("missing campaign timestamp")
        return {
            "qualification": "CANONICAL_CAMPAIGN_IMPORTED_READONLY_PREFLIGHT",
            "candidate_sha": config["pinned_sha"],
            "campaign_id": values["campaign_id"],
            "total": values["total"],
            "completed": values["completed"],
            "counts": counts,
            "manifest_sha256": hashlib.sha256(raw).hexdigest(),
            "installed_binary_matches": hub["installed_binary_matches"],
            "hub_service": hub["hub_service"],
            "hub_ready": hub["hub_ready"],
            "source_evidence_references": source_references,
            "evidence_location": "ORIGINAL_MAC_NOT_COPIED",
            "physical_gate_execution": "NOT_STARTED",
        }
    except (OSError, ValueError, TypeError, KeyError, AttributeError):
        return {"qualification": "INVALID_CANONICAL_CAMPAIGN_NO_QUALIFICATION",
                "candidate_sha": config["pinned_sha"]}
