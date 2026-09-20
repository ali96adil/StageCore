"""Offline regression checks for exact-SHA, immutable Pi campaign import/preflight."""
import hashlib
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import import_campaign
import verify_campaign
import agent

SHA = import_campaign.SHA


def fixtures(folder):
    gates = [{"id": "TEST-%02d" % i, "method": "AUTO", "acceptance": "read-only"}
             for i in range(79)]
    manifest = {"schema_version": 1,
                "groups": [{"id": "system", "title": "System",
                            "gates": gates, "source_issue": 148}]}
    raw = json.dumps(manifest, sort_keys=True).encode()
    mf = folder / "manifest.json"
    mf.write_bytes(raw)
    state = {
        "schema_version": 1,
        "manifest_sha256": hashlib.sha256(raw).hexdigest(),
        "pins": {"stagecore_sha": SHA},
        "campaign_id": "2026-09-20T120557Z",
        "created_at": "2026-09-20T00:00:00Z",
        "gates": {g["id"]: {
            "status": "PENDING", "method": "AUTO", "evidence": [],
        } for g in gates},
    }
    st = folder / "source.json"
    st.write_text(json.dumps(state), encoding="utf-8")
    return st, mf, state


class ImportTest(unittest.TestCase):
    def test_import_is_immutable_exact_pin_and_private(self):
        with tempfile.TemporaryDirectory() as root:
            folder = Path(root)
            st, mf, state = fixtures(folder)
            target_dir = folder / "private"
            target_dir.mkdir(mode=0o700)
            dst = target_dir / "campaign.json"
            with mock.patch.object(import_campaign, "MANIFEST_SHA", state["manifest_sha256"]):
                summary, status = import_campaign.immutable_import(st, mf, dst)
                self.assertEqual(status, "IMPORTED")
                self.assertEqual(summary["total"], 79)
                self.assertEqual(dst.stat().st_mode & 0o777, 0o600)
                self.assertEqual(import_campaign.immutable_import(st, mf, dst)[1],
                                 "EXISTING_IDENTICAL")
                state["gates"]["TEST-01"]["status"] = "PASS"
                st.write_text(json.dumps(state), encoding="utf-8")
                with self.assertRaisesRegex(ValueError, "do not overwrite"):
                    import_campaign.immutable_import(st, mf, dst)
                self.assertEqual(json.loads(dst.read_text())["gates"]["TEST-01"]["status"],
                                 "PENDING")
                dst.unlink()
                state["pins"]["stagecore_sha"] = "f" * 40
                st.write_text(json.dumps(state), encoding="utf-8")
                with self.assertRaisesRegex(ValueError, "deployed"):
                    import_campaign.immutable_import(st, mf, dst)

    def test_bad_manifest_milestone_and_symlink_fail(self):
        with tempfile.TemporaryDirectory() as root:
            folder = Path(root)
            st, mf, state = fixtures(folder)
            with self.assertRaises(ValueError):
                import_campaign.immutable_import(st, mf, folder / "missing" / "campaign.json")
            with mock.patch.object(import_campaign, "MANIFEST_SHA", state["manifest_sha256"]):
                state["gates"]["TEST-02"]["milestones"] = {"readiness": {"status": "UNKNOWN"}}
                st.write_text(json.dumps(state), encoding="utf-8")
                with self.assertRaisesRegex(ValueError, "milestone"):
                    import_campaign.immutable_import(st, mf, folder / "campaign.json")
                st.unlink()
                st.symlink_to(mf)
                with self.assertRaisesRegex(ValueError, "unsafe"):
                    import_campaign.immutable_import(st, mf, folder / "campaign.json")

    def test_pi_campaign_preflight_does_not_mark_gates_pass(self):
        with tempfile.TemporaryDirectory() as root:
            folder = Path(root)
            st, mf, state = fixtures(folder)
            state_dir = folder / "private"
            state_dir.mkdir(mode=0o700)
            cfg = {
                "state_dir": str(state_dir), "pinned_sha": SHA,
                "expected_hub_sha256": "3" * 64,
            }
            self.assertEqual(verify_campaign.verify_live_campaign(cfg)["qualification"],
                             "CANONICAL_CAMPAIGN_IMPORT_REQUIRED")
            with mock.patch.object(import_campaign, "MANIFEST_SHA", state["manifest_sha256"]):
                import_campaign.immutable_import(st, mf, state_dir / "campaign.json")
                with mock.patch.object(agent, "read_status", return_value={
                    "installed_binary_matches": True, "hub_service": "active", "hub_ready": True,
                }):
                    preflight = verify_campaign.verify_live_campaign(cfg, manifest_path=mf)
                self.assertEqual(preflight["qualification"],
                                 "CANONICAL_CAMPAIGN_IMPORTED_READONLY_PREFLIGHT")
                self.assertEqual(preflight["completed"], 0)
                self.assertEqual(preflight["physical_gate_execution"], "NOT_STARTED")
                self.assertEqual(preflight["evidence_location"], "ORIGINAL_MAC_NOT_COPIED")
                state_dir.joinpath("campaign.json").write_text("{}", encoding="utf-8")
                self.assertEqual(verify_campaign.verify_live_campaign(cfg, manifest_path=mf)
                                 ["qualification"], "INVALID_CANONICAL_CAMPAIGN_NO_QUALIFICATION")

    def test_only_owner_exact_verify_campaign_request(self):
        body = json.dumps({"version": 1, "action": "verify-campaign",
                           "candidate_sha": SHA})
        issue = {"number": 300, "title": agent.REQUEST_TITLE, "state": "open",
                 "user": {"login": "ali96adil"}, "author_association": "OWNER", "body": body}
        self.assertIsNotNone(agent.parse_request(issue, "ali96adil", SHA))
        issue["body"] = body.replace(SHA, "a" * 40)
        self.assertIsNone(agent.parse_request(issue, "ali96adil", SHA))


if __name__ == "__main__":
    unittest.main()
