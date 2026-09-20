"""Offline tests for the bounded first-slice private GitHub control agent."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("stagecore_pi_control", HERE / "agent.py")
agent = importlib.util.module_from_spec(spec)
spec.loader.exec_module(agent)
SHA = "809d1f4ce7a5824c27ecf82d78cd779cd28f6e6a"
DIGEST = "3" * 64


def issue(**overrides):
    record = {
        "number": 20,
        "title": agent.REQUEST_TITLE,
        "state": "open",
        "user": {"login": "ali96adil"},
        "author_association": "OWNER",
        "body": json.dumps({"version": 1, "action": "status", "candidate_sha": SHA}),
    }
    record.update(overrides)
    return record


def config(folder):
    return {
        "repo": "ali96adil/StageCore", "owner_login": "ali96adil",
        "pinned_sha": SHA, "expected_hub_sha256": DIGEST,
        "token_file": str(folder / "token"), "state_dir": str(folder / "state"),
    }


class RequestTest(unittest.TestCase):
    def test_only_owner_exact_typed_status(self):
        self.assertIsNotNone(agent.parse_request(issue(), "ali96adil", SHA))
        bad = [
            {"user": {"login": "untrusted"}},
            {"author_association": "COLLABORATOR"},
            {"state": "closed"},
            {"title": "Shell execution please"},
            {"pull_request": {}},
            {"body": "not json"},
            {"body": json.dumps({"version": 1, "action": "run", "candidate_sha": SHA})},
            {"body": json.dumps({"version": 1, "action": "status", "candidate_sha": "a" * 40})},
            {"body": json.dumps({"version": 1, "action": "status", "candidate_sha": SHA, "cmd": "rm -rf /"})},
            {"body": json.dumps([{"version": 1, "action": "status", "candidate_sha": SHA}])},
        ]
        for mutation in bad:
            with self.subTest(mutation=mutation):
                self.assertIsNone(agent.parse_request(issue(**mutation), "ali96adil", SHA))

    def test_private_token_enforced(self):
        with tempfile.TemporaryDirectory() as folder:
            p = Path(folder) / "token"
            p.write_text("test-token\n", encoding="utf-8")
            p.chmod(0o644)
            with self.assertRaises(ValueError):
                agent.read_token(p)
            p.chmod(0o600)
            self.assertEqual(agent.read_token(p), "test-token")
            p2 = Path(folder) / "link"
            p2.symlink_to(p)
            with self.assertRaises(ValueError):
                agent.read_token(p2)


class FakeGitHub:
    def __init__(self):
        self.issues = [issue()]
        self.comments = []
        self.closed = []
        self.fail_close = False

    def open_issues(self):
        return [i for i in self.issues if i["number"] not in self.closed]

    def existing_result(self, number, marker):
        return any(n == number and marker in body for n, body in self.comments)

    def post_result(self, number, body):
        self.comments.append((number, body))

    def close(self, number):
        if self.fail_close:
            raise OSError("simulated temporary remote failure")
        self.closed.append(number)


class PollTest(unittest.TestCase):
    def test_status_is_durable_and_remote_comment_idempotent(self):
        with tempfile.TemporaryDirectory() as folder:
            c = config(Path(folder))
            api = FakeGitHub()
            stub = {
                "candidate_sha": SHA,
                "hub_service": "active",
                "hub_ready": True,
                "installed_binary_matches": True,
                "storage_state": "HEALTHY",
                "qualification": "STATUS_ONLY_NOT_PHYSICAL_PASS",
            }
            with mock.patch.object(agent, "read_status", return_value=stub) as read:
                api.fail_close = True
                with self.assertRaises(OSError):
                    agent.poll_once(c, api)
                self.assertTrue((Path(c["state_dir"]) / "issue-20.json").exists())
                self.assertEqual(len(api.comments), 1)
                api.fail_close = False
                agent.poll_once(c, api)
                self.assertEqual(len(api.comments), 1)
                self.assertEqual(api.closed, [20])
                read.assert_called_once()

    def test_untrusted_issues_are_not_processed(self):
        with tempfile.TemporaryDirectory() as folder:
            api = FakeGitHub()
            api.issues = [issue(user={"login": "other"})]
            with mock.patch.object(agent, "read_status") as read:
                agent.poll_once(config(Path(folder)), api)
                read.assert_not_called()
                self.assertEqual(api.comments, [])
                self.assertEqual(api.closed, [])


if __name__ == "__main__":
    unittest.main()
