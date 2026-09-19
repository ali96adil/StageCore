#!/usr/bin/env python3
"""Deterministic offline tests; no real Mac/stream PASS implied."""
import importlib.util
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location("mac_live", Path(__file__).with_name("live-mac-evidence.py"))
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)


def sample():
    identity = dict(zip(mod.REQUIRED, ("p", "snapshot", "sha", "source", "mac", "main", "layer")))
    rows = {}
    for i, step in enumerate(mod.STEPS, 1):
        rows[step] = dict(at_us=i, source_id="source", result="COMPLETED",
                          native_open=step not in ("baseline", "closed"),
                          renderer_attached=step in ("routed", "loss", "recovered"),
                          readiness="WARNING" if step == "loss" else "READY", note="")
    observations = dict.fromkeys(("moving_frames", "companion_local_render",
        "hub_control_only", "source_loss_detected", "moving_frames_recovered",
        "no_implicit_replay"), True)
    return dict(identity=identity, steps=rows, observations=observations)


class LiveEvidenceTests(unittest.TestCase):
    def test_complete_evidence_review_only(self):
        result = mod.check(sample())
        self.assertEqual(result["status"], "PASS")
        self.assertIn("not an independent Companion measurement", result["limitations"])

    def test_loss_must_be_reported(self):
        data = sample()
        data["steps"]["loss"]["readiness"] = "READY"
        self.assertEqual(mod.check(data)["gates"]["Q-LIVE-03"], "BLOCKED")

    def test_open_is_not_render(self):
        data = sample()
        data["steps"]["routed"]["renderer_attached"] = False
        self.assertEqual(mod.check(data)["gates"]["Q-LIVE-01"], "BLOCKED")

    def test_no_replay_is_required(self):
        data = sample()
        data["observations"]["no_implicit_replay"] = False
        self.assertEqual(mod.check(data)["gates"]["Q-LIVE-03"], "BLOCKED")

    def test_identity_and_time_are_pinned(self):
        data = sample()
        data["steps"]["loss"]["source_id"] = "another"
        with self.assertRaises(ValueError):
            mod.check(data)
        data = sample()
        data["steps"]["loss"]["at_us"] = 2
        with self.assertRaises(ValueError):
            mod.check(data)

    def test_extra_fields_rejected(self):
        data = sample()
        data["steps"]["opened"]["endpoint_ref"] = "private-url"
        with self.assertRaises(ValueError):
            mod.check(data)


if __name__ == "__main__":
    unittest.main()
