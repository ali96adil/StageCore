"""Regression tests for non-mutating full campaign triage."""
import hashlib
import json
from pathlib import Path
import sys
import unittest

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import triage_all_79 as triage

RAW = (HERE.parent / "manifest.json").read_bytes()
MANIFEST = json.loads(RAW)


def campaign():
    return {
        "schema_version": 1,
        "manifest_sha256": hashlib.sha256(RAW).hexdigest(),
        "campaign_id": "2026-09-20T120557.017453+0000",
        "pins": {"stagecore_sha": triage.EXPECTED_HUB_SHA},
        "gates": {g["id"]: {"status": "PENDING", "method": g["method"]}
                  for group in MANIFEST["groups"] for g in group["gates"]},
    }


class TriageTests(unittest.TestCase):
    def test_all_79_gates_and_no_inferred_pass(self):
        state = campaign()
        for gate in ("Q-TAB-04", "Q-TAB-05", "Q-DMX-20", "Q-SYS-04"):
            state["gates"][gate]["status"] = "BLOCKED"
        probe = {"schema_version": 1, "devices": [{
            "profile_id": triage.DMX, "enabled": True,
            "runtime": {"connection_state": "ONLINE", "readiness": "BLOCKER",
                        "last_seen_at_us": 100_000_000,
                        "observed": {"authority": "FAILSAFE", "dmx_healthy": True}},
        }]}
        result = triage.summarize(MANIFEST, RAW, state, probe=probe, now=102)
        self.assertEqual(len(result["gates"]), 79)
        self.assertEqual(result["counts"]["BLOCKED"], 4)
        self.assertEqual(result["completed"], 0)
        self.assertFalse(result["campaign_modified"])
        self.assertFalse(result["disruptive_actions_executed"])
        self.assertEqual(result["device_facts"]["tablet_registered"], 0)
        self.assertEqual(result["device_facts"]["lighting_online"], 1)
        actions = {row["gate"]: row["next"] for row in result["gates"]}
        self.assertEqual(actions["Q-TAB-01"], "EXACT_APK_INSTALLATION_PHYSICAL_EVIDENCE_REQUIRED")
        self.assertEqual(actions["Q-TAB-02"], "EXACT_RC3_APK_BUILD_IDENTITY_EVIDENCE_REQUIRED")
        self.assertEqual(actions["Q-TAB-03"], "SECURE_TABLET_PAIRING_REQUIRED")
        self.assertEqual(actions["Q-TAB-04"], "TABLET_REGISTRATION_REQUIRED")
        self.assertEqual(actions["Q-DMX-20"], "LIGHTING_READINESS_AND_PUBLISHED_CONFIG_REQUIRED")
        self.assertEqual(sum(group["total"] for group in result["groups"]), 79)

    def test_identity_mismatch_fails_closed(self):
        state = campaign()
        with self.assertRaisesRegex(ValueError, "digest mismatch"):
            triage.summarize(MANIFEST, RAW + b" ", state)
        state["pins"]["stagecore_sha"] = "a" * 40
        with self.assertRaisesRegex(ValueError, "candidate mismatch"):
            triage.summarize(MANIFEST, RAW, state)

    def test_prior_results_not_reclassified(self):
        state = campaign()
        state["gates"]["Q-SYS-04"]["status"] = "PASS"
        result = triage.summarize(MANIFEST, RAW, state)
        row = next(row for row in result["gates"] if row["gate"] == "Q-SYS-04")
        self.assertEqual(row["status"], "PASS")
        self.assertEqual(row["next"], "PRESERVE_RECORDED_RESULT")
        self.assertEqual(state["gates"]["Q-SYS-04"]["status"], "PASS")


if __name__ == "__main__":
    unittest.main()
