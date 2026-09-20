#!/usr/bin/env python3
"""Bounded, outbound-only GitHub issue control: status only (first slice).

No remote shell, physical commands, update, restart, or full campaign execution.
"""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request

REQUEST_TITLE = "StageCore Qualification Request v1"
ALLOWED_KEYS = {"version", "action", "candidate_sha"}
SHA_RE = re.compile(r"^[0-9a-f]{40}$")
REPO_RE = re.compile(r"^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$")
CONTROL_MARKER = "<!-- stagecore-qualification-control-v1 issue={} -->"
HUB_BINARY = Path("/opt/stagecore/bin/stagecore-hub")
HEALTH_URL = "http://127.0.0.1:7840/health/ready"


def parse_request(issue, owner, pinned_sha):
    """Reject other authors, PRs, prose, extra JSON keys and unpinned commands."""
    if issue.get("title") != REQUEST_TITLE or "pull_request" in issue:
        return None
    if issue.get("state") != "open" or issue.get("user", {}).get("login") != owner:
        return None
    if issue.get("author_association") != "OWNER":
        return None
    body = issue.get("body", "")
    if not isinstance(body, str) or len(body) > 1024:
        return None
    try:
        request = json.loads(body)
    except (ValueError, TypeError):
        return None
    if not isinstance(request, dict) or set(request) != ALLOWED_KEYS:
        return None
    if request["version"] != 1 or request["action"] not in ("status", "report", "verify-campaign") :
        return None
    if not isinstance(request["candidate_sha"], str) or not SHA_RE.fullmatch(request["candidate_sha"]):
        return None
    if request["candidate_sha"] != pinned_sha:
        return None
    return request


def load_config(path):
    data = json.loads(Path(path).read_text(encoding="utf-8"))
    expected = {"repo", "owner_login", "pinned_sha", "expected_hub_sha256", "token_file", "state_dir"}
    if set(data) != expected:
        raise ValueError("control configuration has missing or unexpected keys")
    if not REPO_RE.fullmatch(data["repo"]):
        raise ValueError("invalid repository")
    if not re.fullmatch(r"[A-Za-z0-9-]{1,39}", data["owner_login"]):
        raise ValueError("invalid allowed issue author")
    if not SHA_RE.fullmatch(data["pinned_sha"]):
        raise ValueError("pinned_sha must be a full lowercase Git SHA")
    if not re.fullmatch(r"[0-9a-f]{64}", data["expected_hub_sha256"]):
        raise ValueError("expected_hub_sha256 must be a SHA-256 hex digest")
    if not Path(data["token_file"]).is_absolute() or not Path(data["state_dir"]).is_absolute():
        raise ValueError("credential and state paths must be absolute")
    return data


def read_token(path):
    path = Path(path)
    if path.is_symlink():
        raise ValueError("token file may not be a symlink")
    st = path.stat()
    if not path.is_file() or st.st_mode & 0o077:
        raise ValueError("token file must be a private regular file with mode 0600 or stricter")
    token = path.read_text(encoding="utf-8").strip()
    if not token or "\n" in token or "\r" in token:
        raise ValueError("invalid token file")
    return token


class GitHub:
    def __init__(self, repo, token):
        self.root = "https://api.github.com/repos/" + repo
        self.token = token

    def call(self, path, method="GET", body=None):
        if not path.startswith("/") or path.startswith("//"):
            raise ValueError("unsafe GitHub API path")
        url = self.root + path
        headers = {
            "Authorization": "Bearer " + self.token,
            "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": "2022-11-28",
            "User-Agent": "stagecore-qualification-control/1",
        }
        raw = None if body is None else json.dumps(body, separators=(",", ":")).encode("utf-8")
        if raw is not None:
            headers["Content-Type"] = "application/json"
        req = urllib.request.Request(url, data=raw, headers=headers, method=method)
        with urllib.request.urlopen(req, timeout=12) as response:
            payload = response.read(1024 * 1024 + 1)
        if len(payload) > 1024 * 1024:
            raise ValueError("oversized GitHub API response")
        return json.loads(payload)

    def open_issues(self):
        return self.call("/issues?state=open&sort=created&direction=desc&per_page=100")

    def existing_result(self, number, marker):
        comments = self.call("/issues/{}/comments?per_page=100".format(number))
        return any(marker in c.get("body", "") for c in comments)

    def post_result(self, number, message):
        self.call("/issues/{}/comments".format(number), "POST", {"body": message})

    def close(self, number):
        self.call("/issues/{}".format(number), "PATCH", {"state": "closed"})


def sha256_file(path):
    h = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()


def read_status(config):
    try:
        service = subprocess.run(
            ["/usr/bin/systemctl", "is-active", "stagecore-hub.service"],
            capture_output=True, text=True, timeout=5, check=False,
        ).stdout.strip()[:24]
    except (OSError, subprocess.TimeoutExpired):
        service = "unknown"
    try:
        with urllib.request.urlopen(HEALTH_URL, timeout=5) as response:
            health = json.loads(response.read(16384))
        ready = health.get("status") == "READY"
        storage = health.get("storage_state")
    except (OSError, ValueError, urllib.error.URLError):
        ready, storage = False, "UNAVAILABLE"
    try:
        digest = sha256_file(HUB_BINARY)
    except OSError:
        digest = None
    return {
        "candidate_sha": config["pinned_sha"],
        "installed_binary_matches": digest == config["expected_hub_sha256"],
        "hub_service": service,
        "hub_ready": ready,
        "storage_state": storage if storage in ("HEALTHY", "DEGRADED", "UNAVAILABLE") else "UNKNOWN",
        "qualification": "STATUS_ONLY_NOT_PHYSICAL_PASS",
    }


def read_campaign_report(config):
    """Read only a sanitized imported summary, never the private raw evidence."""
    path = Path(config["state_dir"]) / "campaign-summary.json"
    if path.is_symlink() or not path.is_file() or path.stat().st_size > 65536:
        return {"qualification": "IMPORT_REQUIRED_NOT_LIVE_PI_EVIDENCE",
                "candidate_sha": config["pinned_sha"]}
    try:
        snapshot = json.loads(path.read_text(encoding="utf-8"))
        expected = {"format", "campaign_id", "candidate_sha", "manifest_sha256",
                    "total", "completed", "remaining", "counts", "groups", "provenance"}
        if set(snapshot) != expected:
            raise ValueError("unrecognized summary schema")
        if snapshot["format"] != "stagecore-imported-campaign-summary-v1" or (
                snapshot["provenance"] != "MAC_IMPORTED_SNAPSHOT_NOT_LIVE_PI_EVIDENCE"):
            raise ValueError("untrusted summary provenance")
        if snapshot["candidate_sha"] != config["pinned_sha"]:
            raise ValueError("summary candidate mismatch")
        if not re.fullmatch(r"[0-9a-f]{64}", snapshot["manifest_sha256"]):
            raise ValueError("invalid manifest digest")
        if not isinstance(snapshot["campaign_id"], str) or len(snapshot["campaign_id"]) > 128:
            raise ValueError("invalid campaign ID")
        counts = snapshot["counts"]
        keys = {"PASS", "FAIL", "BLOCKED", "PENDING", "N/A"}
        if not isinstance(counts, dict) or set(counts) != keys:
            raise ValueError("invalid gate result counts")
        if any(type(x) is not int or x < 0 for x in counts.values()):
            raise ValueError("invalid gate count")
        total = snapshot["total"]
        completed = snapshot["completed"]
        remaining = snapshot["remaining"]
        if (type(total) is not int or total <= 0 or total > 1000 or
                type(completed) is not int or type(remaining) is not int or
                completed != counts["PASS"] + counts["N/A"] or
                sum(counts.values()) != total or remaining != total - completed):
            raise ValueError("inconsistent gate counts")
        groups = snapshot["groups"]
        if not isinstance(groups, list) or len(groups) > 30:
            raise ValueError("invalid groups")
        for group in groups:
            if not isinstance(group, dict) or set(group) != {"id", "total", "completed", "counts"}:
                raise ValueError("invalid group schema")
            if not isinstance(group["id"], str) or not re.fullmatch(r"[A-Za-z0-9_-]{1,64}", group["id"]):
                raise ValueError("invalid group ID")
            if set(group["counts"]) != keys or any(type(v) is not int or v < 0 for v in group["counts"].values()):
                raise ValueError("invalid group counts")
            if group["total"] != sum(group["counts"].values()) or (
                    group["completed"] != group["counts"]["PASS"] + group["counts"]["N/A"]):
                raise ValueError("inconsistent group counts")
        if sum(g["total"] for g in groups) != total:
            raise ValueError("group totals mismatch")
        return snapshot
    except (OSError, ValueError, TypeError, KeyError):
        return {"qualification": "INVALID_IMPORTED_SUMMARY_NOT_PHYSICAL_PASS",
                "candidate_sha": config["pinned_sha"]}


def atomic_journal(directory, number, result):
    path = directory / ("issue-{}.json".format(number))
    temp = directory / (".issue-{}.tmp".format(number))
    with temp.open("w", encoding="utf-8") as out:
        json.dump(result, out, separators=(",", ":"), sort_keys=True)
        out.write("\n")
        out.flush()
        os.fsync(out.fileno())
    os.chmod(temp, 0o600)
    os.replace(temp, path)


def poll_once(config, github):
    state = Path(config["state_dir"])
    state.mkdir(mode=0o700, parents=True, exist_ok=True)
    if state.stat().st_mode & 0o077:
        raise ValueError("state directory is not private")
    with (state / ".lock").open("a+") as lock:
        fcntl.flock(lock.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        for issue in github.open_issues():
            request = parse_request(issue, config["owner_login"], config["pinned_sha"])
            if request is None:
                continue
            number = issue["number"]
            if not isinstance(number, int) or number <= 0:
                continue
            marker = CONTROL_MARKER.format(number)
            # Reconcile a prior successful POST even if the close request failed.
            if github.existing_result(number, marker):
                github.close(number)
                continue
            journal = state / ("issue-{}.json".format(number))
            if journal.exists():
                result = json.loads(journal.read_text(encoding="utf-8"))
            else:
                if request["action"] == "status":
                    result = read_status(config)
                elif request["action"] == "report":
                    result = read_campaign_report(config)
                else:
                    from verify_campaign import verify_live_campaign
                    result = verify_live_campaign(config)
                atomic_journal(state, number, result)
            fence = chr(96) * 3
            body = marker + "\nStageCore read-only " + request["action"] + " (not a physical qualification PASS):\n\n" + (
                fence + "json\n" + json.dumps(result, sort_keys=True, indent=2) + "\n" + fence
            )
            github.post_result(number, body)
            github.close(number)
            print("Completed status request #{}".format(number))


def main():
    parser = argparse.ArgumentParser(description="StageCore private GitHub outbound status agent")
    parser.add_argument("--config", required=True, help="absolute path to private JSON config")
    args = parser.parse_args()
    config = load_config(args.config)
    github = GitHub(config["repo"], read_token(config["token_file"]))
    poll_once(config, github)


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print("control agent: " + str(exc), file=sys.stderr)
        raise SystemExit(1)
