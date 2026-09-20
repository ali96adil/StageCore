"""No-device tests: manifest-verified summary export, pinned import, and request flow."""
import hashlib
import importlib.util
import json
import shutil
from pathlib import Path
import tempfile
import unittest
from unittest import mock

HERE = Path(__file__).resolve().parent


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


agent = load("campaign_agent", HERE / "agent.py")
exporter = load("campaign_exporter", HERE / "export_summary.py")
SHA = "809d1f4ce7a5824c27ecf82d78cd779cd28f6e6a"


def make_state(folder):
    manifest = {
        "schema_version": 1,
        "groups": [{"id": "system", "title": "System", "source_issue": 148,
                    "gates": [{"id": "Q-SYS-01", "method": "EVIDENCE", "acceptance": "pin"}]}],
    }
    raw = json.dumps(manifest).encode()
    state = {
        "manifest_sha256": hashlib.sha256(raw).hexdigest(),
        "campaign_id": "example-campaign",
        "pins": {"stagecore_sha": SHA},
        "gates": {"Q-SYS-01": {
            "method": "EVIDENCE", "status": "BLOCKED", "milestones": {},
            "note": "do not upload SECRET_PASSWORD=should-not-leak",
            "evidence": ["/private/raw/test.json"],
        }},
    }
    mf, st = folder / "manifest.json", folder / "campaign.json"
    mf.write_bytes(raw)
    st.write_text(json.dumps(state), encoding="utf-8")
    return st, mf


def config(folder):
    return {
        "pinned_sha": SHA,
        "state_dir": str(folder),
    }


class SummaryTest(unittest.TestCase):
    def test_export_contains_only_allowlisted_summary_and_no_raw_details(self):
        with tempfile.TemporaryDirectory() as root:
            folder = Path(root)
            state, manifest = make_state(folder)
            snapshot = exporter.export_summary(state, manifest)
            text = json.dumps(snapshot)
            self.assertNotIn("should-not-leak", text)
            self.assertNotIn("/private/raw/", text)
            self.assertEqual(snapshot["counts"]["BLOCKED"], 1)
            self.assertEqual(snapshot["total"], 1)
            self.assertEqual(snapshot["completed"], 0)
            exporter.save(snapshot, folder / "campaign-summary.json")
            self.assertEqual(agent.read_campaign_report(config(folder)), snapshot)

    def test_deployed_report_module_next_to_exporter(self):
        with tempfile.TemporaryDirectory() as root:
            folder = Path(root)
            shutil.copy2(HERE / "export_summary.py", folder / "export_summary.py")
            shutil.copy2(HERE.parent / "qualification-report.py",
                         folder / "qualification-report.py")
            deployed = load("deployed_summary_fixture", folder / "export_summary.py")
            self.assertEqual(deployed.REPORT_MODULE, folder / "qualification-report.py")
            state, manifest = make_state(folder)
            self.assertEqual(deployed.export_summary(state, manifest)["total"], 1)

    def test_wrong_manifest_rejected_before_export(self):
        with tempfile.TemporaryDirectory() as root:
            folder = Path(root)
            state, manifest = make_state(folder)
            manifest.write_text(manifest.read_text(encoding="utf-8") + " ", encoding="utf-8")
            with self.assertRaises(ValueError):
                exporter.export_summary(state, manifest)

    def test_bad_snapshot_not_reported(self):
        with tempfile.TemporaryDirectory() as root:
            folder = Path(root)
            state, manifest = make_state(folder)
            snapshot = exporter.export_summary(state, manifest)
            path = folder / "campaign-summary.json"
            snapshot["candidate_sha"] = "f" * 40
            exporter.save(snapshot, path)
            self.assertEqual(agent.read_campaign_report(config(folder))["qualification"],
                             "INVALID_IMPORTED_SUMMARY_NOT_PHYSICAL_PASS")
            snapshot["candidate_sha"] = SHA
            snapshot["counts"]["PASS"] = 1
            exporter.save(snapshot, path)
            self.assertEqual(agent.read_campaign_report(config(folder))["qualification"],
                             "INVALID_IMPORTED_SUMMARY_NOT_PHYSICAL_PASS")
            path.unlink()
            self.assertEqual(agent.read_campaign_report(config(folder))["qualification"],
                             "IMPORT_REQUIRED_NOT_LIVE_PI_EVIDENCE")

    def test_owner_only_typed_report(self):
        request = {
            "number": 31, "title": agent.REQUEST_TITLE,
            "state": "open", "user": {"login": "ali96adil"},
            "author_association": "OWNER",
            "body": json.dumps({"version": 1, "action": "report", "candidate_sha": SHA}),
        }
        self.assertIsNotNone(agent.parse_request(request, "ali96adil", SHA))
        request["body"] = json.dumps({"version": 1, "action": "run", "candidate_sha": SHA})
        self.assertIsNone(agent.parse_request(request, "ali96adil", SHA))

    def test_agent_replies_once_with_imported_summary(self):
        with tempfile.TemporaryDirectory() as root:
            folder = Path(root)
            state, manifest = make_state(folder)
            exporter.save(exporter.export_summary(state, manifest), folder / "campaign-summary.json")
            class FakeGitHub:
                closed = False
                comments = []
                def open_issues(self):
                    return [] if self.closed else [{
                        "number": 42, "title": agent.REQUEST_TITLE, "state": "open",
                        "user": {"login": "ali96adil"}, "author_association": "OWNER",
                        "body": json.dumps({"version": 1, "action": "report", "candidate_sha": SHA}),
                    }]
                def existing_result(self, number, marker):
                    return any(marker in body for body in self.comments)
                def post_result(self, number, body):
                    self.comments.append(body)
                def close(self, number):
                    self.closed = True
            github = FakeGitHub()
            cfg = dict(config(folder), owner_login="ali96adil")
            with mock.patch.object(agent, "read_status", side_effect=AssertionError("must not run status")):
                agent.poll_once(cfg, github)
                self.assertEqual(len(github.comments), 1)
                self.assertIn("MAC_IMPORTED_SNAPSHOT_NOT_LIVE_PI_EVIDENCE", github.comments[0])
                self.assertNotIn("should-not-leak", github.comments[0])
                agent.poll_once(cfg, github)
                self.assertEqual(len(github.comments), 1)


if __name__ == "__main__":
    unittest.main()
