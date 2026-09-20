"""Offline tests proving only three read-only gates may change."""
import json
from pathlib import Path
import sys
import tempfile
import time
import unittest
from unittest import mock

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import agent
import import_campaign
import pi_readonly

SHA = import_campaign.SHA
MANIFEST = HERE.parent / "manifest.json"
STATE_TOOL = HERE.parent / "qualification-state.py"
MILESTONE_TOOL = HERE.parent / "qualification-milestone.py"
ASSERT_TOOL = HERE.parent / "assert-device-probe.py"


def make_campaign(folder):
    manifest = json.loads(MANIFEST.read_text(encoding="utf-8"))
    gates = {g["id"]: {
        "status": "PENDING", "method": g["method"], "source_issue": group["source_issue"],
        "acceptance": g["acceptance"], "updated_at": None, "actor": None,
        "evidence": [], "note": "", "history": [],
    } for group in manifest["groups"] for g in group["gates"]}
    state = {
        "schema_version": 1, "manifest_sha256": import_campaign.MANIFEST_SHA,
        "pins": {"stagecore_sha": SHA}, "campaign_id": "2026-09-20T120557Z",
        "created_at": "2026-09-20T12:05:57Z",
        "gates": gates,
    }
    state_dir = folder / "private"
    state_dir.mkdir(mode=0o700)
    (state_dir / "campaign.json").write_text(json.dumps(state), encoding="utf-8")
    return state_dir


def hub_ready(_):
    return {"installed_binary_matches": True, "hub_service": "active", "hub_ready": True}


def tablet():
    from datetime import datetime, timezone
    return {
        "profile_id": "stagecore.tablet-player",
        "device_id": "tablet-test",
        "project_id": "project-1",
        "device_kind": "TABLET_PLAYER",
        "enabled": True,
        "protocol_version": "stagecore.device/1",
        "capabilities": sorted({
            "tablet.media.prepare", "tablet.media.play", "tablet.media.pause",
            "tablet.media.stop", "tablet.media.blackout", "tablet.media.blackout.clear",
            "tablet.media.overlay.play", "tablet.media.overlay.clear",
            "tablet.media.live.show", "tablet.media.live.hide",
        }),
        "runtime": {
            "connection_state": "ONLINE", "readiness": "READY",
            "last_seen_at_us": int(time.time() * 1000000),
            "observed": {"project_id": "project-1", "runtime_snapshot_id": "snapshot-1"},
        },
    }


class RunnerTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.folder = Path(self.tmp.name)
        self.state_dir = make_campaign(self.folder)
        self.cfg = {"state_dir": str(self.state_dir), "pinned_sha": SHA,
                    "expected_hub_sha256": "a" * 64}
        self.patcher = mock.patch.multiple(pi_readonly, MANIFEST=MANIFEST,
                                           STATE_TOOL=STATE_TOOL, MILESTONE_TOOL=MILESTONE_TOOL,
                                           ASSERT_TOOL=ASSERT_TOOL)
        self.patcher.start()
        self.hub_patch = mock.patch.object(agent, "read_status", side_effect=hub_ready)
        self.hub_patch.start()

    def tearDown(self):
        self.hub_patch.stop()
        self.patcher.stop()
        self.tmp.cleanup()

    def snapshot(self, devices):
        path = self.folder / "probe.json"
        path.write_text(json.dumps({"schema_version": 1, "devices": devices}),
                        encoding="utf-8")
        return path

    def test_missing_snapshot_does_not_modify_campaign(self):
        original = (self.state_dir / "campaign.json").read_bytes()
        result = pi_readonly.run_readonly(self.cfg, 300, manifest=MANIFEST,
                                          probe=self.folder / "missing.json")
        self.assertEqual(result["qualification"], "PROBE_SNAPSHOT_MISSING_OR_STALE_NO_MUTATION")
        self.assertEqual((self.state_dir / "campaign.json").read_bytes(), original)

    def test_devices_off_remain_blocked_not_failed_and_manual_unchanged(self):
        with mock.patch.object(pi_readonly, "targets", return_value={
            "project_id": "", "runtime_snapshot_id": "",
            "tablet_id": "", "lighting_id": "",
        }):
            outcome = pi_readonly.run_readonly(self.cfg, 301, manifest=MANIFEST,
                                               probe=self.snapshot([]))
        self.assertEqual(outcome["qualification"],
                         "PI_READONLY_PROBE_EXECUTED_NOT_FULL_QUALIFICATION")
        self.assertEqual(outcome["processed_gates"],
                         {"Q-TAB-04": "BLOCKED", "Q-TAB-05": "BLOCKED", "Q-DMX-20": "BLOCKED"})
        self.assertEqual(outcome["counts"]["FAIL"], 0)
        state = json.loads((self.state_dir / "campaign.json").read_text())
        self.assertEqual(state["gates"]["Q-DMX-21"]["status"], "PENDING")
        self.assertEqual(outcome["completed"], 0)

    def test_online_tablet_scope_only_when_explicitly_pinned(self):
        with mock.patch.object(pi_readonly, "targets", return_value={
            "project_id": "project-1", "runtime_snapshot_id": "snapshot-1",
            "tablet_id": "tablet-test", "lighting_id": "",
        }):
            outcome = pi_readonly.run_readonly(self.cfg, 302, manifest=MANIFEST,
                                               probe=self.snapshot([tablet()]))
        self.assertEqual(outcome["processed_gates"]["Q-TAB-04"], "PASS")
        self.assertEqual(outcome["processed_gates"]["Q-TAB-05"], "PASS")
        self.assertEqual(outcome["processed_gates"]["Q-DMX-20"], "BLOCKED")
        self.assertEqual(outcome["completed"], 2)
        self.assertEqual(outcome["manual_physical_actions"], "NOT_EXECUTED")
        with mock.patch.object(pi_readonly, "targets", return_value={
            "project_id": "project-1", "runtime_snapshot_id": "snapshot-1",
            "tablet_id": "tablet-test", "lighting_id": "",
        }):
            again = pi_readonly.run_readonly(self.cfg, 303, manifest=MANIFEST,
                                             probe=self.snapshot([]))
        self.assertEqual(again["processed_gates"]["Q-TAB-04"], "PASS")
        self.assertEqual(again["processed_gates"]["Q-TAB-05"], "PASS")
        self.assertEqual(again["counts"]["FAIL"], 0)

    def test_only_pinned_author_can_request_run_readonly(self):
        request = {"title": agent.REQUEST_TITLE, "state": "open",
                   "user": {"login": "ali96adil"}, "author_association": "OWNER",
                   "body": json.dumps({"version": 1, "action": "run-readonly",
                                       "candidate_sha": SHA})}
        self.assertIsNotNone(agent.parse_request(request, "ali96adil", SHA))
        request["user"]["login"] = "attacker"
        self.assertIsNone(agent.parse_request(request, "ali96adil", SHA))


if __name__ == "__main__":
    unittest.main()
