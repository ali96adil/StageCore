"""Offline regression tests for cross-group Pi preflight without production writes."""
import hashlib
import json
from pathlib import Path
import sqlite3
import sys
import tempfile
import unittest
from unittest import mock

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import batch_preflight as batch
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


class PreflightTest(unittest.TestCase):
    def test_redacted_aggregates_and_published_manifest_count(self):
        with tempfile.TemporaryDirectory() as d:
            db = Path(d) / "db.sqlite3"
            conn = sqlite3.connect(db)
            conn.executescript("""
                CREATE TABLE projects(project_id TEXT);
                CREATE TABLE runtime_snapshots(status TEXT, manifest_json TEXT);
                CREATE TABLE stage_devices(device_kind TEXT, enabled INTEGER);
                CREATE TABLE live_video_sources(source_id TEXT);
                CREATE TABLE network_observations(observation_id TEXT);
            """)
            conn.execute("INSERT INTO projects VALUES (?)", ("secret-project",))
            conn.execute("INSERT INTO runtime_snapshots VALUES (?,?)",
                         ("PUBLISHED", json.dumps({"lighting_nodes": [
                             {"device_id": "secret-device"}, {"device_id": "second-device"}]})))
            conn.execute("INSERT INTO runtime_snapshots VALUES (?,?)", ("DRAFT", "{}"))
            conn.execute("INSERT INTO stage_devices VALUES (?,?)", ("GENERIC", 1))
            conn.execute("INSERT INTO stage_devices VALUES (?,?)", ("UNLISTED_PRIVATE_KIND", 0))
            conn.commit()
            conn.close()
            info = batch.database_inventory(db)
            self.assertEqual(info["availability"], "READONLY")
            self.assertEqual(info["row_counts"]["projects"], 1)
            self.assertEqual(info["published_snapshots"]["PUBLISHED"], 1)
            self.assertEqual(info["published_lighting_bindings"], 2)
            self.assertEqual(info["device_kinds"]["GENERIC"]["enabled"], 1)
            self.assertEqual(info["device_kinds"]["OTHER"]["disabled"], 1)
            self.assertNotIn("secret-project", json.dumps(info))
            self.assertNotIn("secret-device", json.dumps(info))
            self.assertNotIn("UNLISTED_PRIVATE_KIND", json.dumps(info))
            with mock.patch.object(batch, "file_hash", return_value=(
                 "34681e3ce7595d4caf63f1a306f496f4172f398065a2fe7885bc2bacc6ce7bfe")), \
                 mock.patch.object(batch, "safe_service_status", return_value="active"), \
                 mock.patch.object(batch, "safe_health", return_value={"status": "READY", "storage": "HEALTHY"}):
                summary = batch.collect(MANIFEST, RAW, campaign(), None, None,
                                        db=db, hub=Path(d) / "absent")
            self.assertEqual(summary["campaign_counts"]["PENDING"], 79)
            self.assertTrue(summary["candidate_binary_sha256_matches"])
            self.assertFalse(summary["campaign_modified"])
            self.assertFalse(summary["physical_actions_executed"])
            self.assertEqual(summary["independent_gate_evidence"]["Q-SYS-07"],
                             "INSTALLED_HEALTH_ONLY_RESTART_AND_NO_REPLAY_UNVERIFIED")

    def test_invalid_manifest_pin_and_db_do_not_qualify(self):
        with tempfile.TemporaryDirectory() as d:
            db = Path(d) / "missing.sqlite3"
            self.assertEqual(batch.database_inventory(db)["availability"], "UNAVAILABLE")
            with self.assertRaisesRegex(ValueError, "digest mismatch"):
                batch.collect(MANIFEST, RAW + b" ", campaign(), None, None, db=db)
            wrong = campaign()
            wrong["pins"]["stagecore_sha"] = "a" * 40
            with self.assertRaisesRegex(ValueError, "candidate mismatch"):
                batch.collect(MANIFEST, RAW, wrong, None, None, db=db)

    def test_unparseable_published_manifest_reported_not_accepted(self):
        with tempfile.TemporaryDirectory() as d:
            db = Path(d) / "db.sqlite3"
            conn = sqlite3.connect(db)
            conn.execute("CREATE TABLE runtime_snapshots(status TEXT, manifest_json TEXT)")
            conn.execute("INSERT INTO runtime_snapshots VALUES (?,?)", ("PUBLISHED", "{bad-json"))
            conn.commit()
            conn.close()
            summary = batch.database_inventory(db)
            self.assertEqual(summary["invalid_published_manifest_count"], 1)
            self.assertEqual(summary["published_lighting_bindings"], 0)


if __name__ == "__main__":
    unittest.main()
