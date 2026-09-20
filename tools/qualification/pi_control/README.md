# Private GitHub → Raspberry Pi qualification control: first read-only slice

**Status: experimental, not installed by adding this branch/PR.** Source and tests only.
This first slice provides exactly one action: `status`. It does **not** run the
79-gate physical campaign, transfer the campaign state from the Mac, update
StageCore, restart the Hub, arm physical commands, disable authentication, or
expose a public endpoint.

## Split of responsibility

| Environment | Authority |
| --- | --- |
| GitHub-hosted Linux Core CI | `go test ./...`, vet, race, release and qualification runner self-tests on the exact commit |
| Raspberry Pi | installed binary identity, live Hub service/readiness and eventually durable physical qualification evidence |
| ChatGPT connected GitHub | create a typed private request issue; read its sanitized result and existing CI evidence |
| Operator Mac | one-time approved provisioning, emergency SSH and optional local diagnostics; its `go test ./...` is not the Linux release gate |

The Pi makes **outbound** HTTPS calls to GitHub. Neither Cloudflare Access
interactive cookies nor public WAN SSH/HTTP are used. The agent runs as an
unprivileged dedicated systemd identity, has no `sudo`, and calls only fixed
local read-only operations. GitHub issues are data, never shell scripts.

## First-slice trust boundary

Only open issues with **exact** title `StageCore Qualification Request v1`
and a JSON object containing exactly `version`, `action`, `candidate_sha`
can be processed. The GitHub issue must have `user.login` exactly equal to
the configured owner and `author_association=OWNER`, and its SHA must equal
the pinned local candidate. Supported values are `version: 1`, `action:
"status"`. All other content, PRs, comments, users, and actions are ignored.

A valid request reads `systemctl is-active`, localhost `/health/ready`, and
SHA-256 of the installed Hub binary. Results contain only allowlisted fields.
The agent journals the response locally before posting, checks for its
distinctive marker in existing issue comments before reposting, then closes
the issue. A failed GitHub comment/close is retried on the next run.

The token only needs fine-grained access to **this private repository** with
`Issues: Read and write` and repository metadata read. Never grant
administration, workflow, contents write, or other repositories. Do not enter
the token into ChatGPT, a GitHub issue, a commit, CI logs or source control.
Revoke it in GitHub to cut off the channel.

## One-time operator provisioning (NOT YET EXECUTED)

First confirm this branch's Linux CI PASS and review its status-only code.
Create a **fine-grained GitHub PAT** restricted to this private `StageCore`
repository with **Issues: Read/write** and metadata read. Do not paste it
into ChatGPT, terminal commands, issues, logs or a repository.

From the Mac, fetch the control branch and copy its four deployment files
to the Pi using the already-authorized Cloudflare SSH alias:

```bash
git -C "$HOME/StageCore" fetch origin qualification/pi-github-control-readonly
git -C "$HOME/StageCore" worktree add --detach "$HOME/StageCore-control" origin/qualification/pi-github-control-readonly
scp -r "$HOME/StageCore-control/tools/qualification/pi_control" stagecore-qualification:pi_control
```

If the worktree already exists, verify its HEAD is the reviewed CI SHA
instead of overwriting it. On an **interactive** Mac-to-Pi SSH session:

```bash
ssh -tt stagecore-qualification 'sudo bash "$HOME/pi_control/install.sh"'
```

This installer refuses a Hub binary hash other than the already qualified
`809d1f4` candidate, creates a dedicated `stagecore-control` unprivileged
system user and isolated private directories, and asks for the GitHub PAT
through a local hidden terminal prompt. It installs a systemd oneshot service
and timer, performs one status-only connectivity test, then enables polling.
If authentication fails, the timer is not enabled. It does **not** restart or
modify the StageCore Hub, grant sudo to the new agent, or touch the 79-gate
campaign state.

Check that the timer is active without printing credentials:

```bash
ssh stagecore-qualification 'systemctl is-enabled stagecore-qualification-control.timer; systemctl is-active stagecore-qualification-control.timer'
```

The private Pi configuration pins `candidate_sha` to
`809d1f4ce7a5824c27ecf82d78cd779cd28f6e6a` and expected installed
Hub SHA-256 to
`34681e3ce7595d4caf63f1a306f496f4172f398065a2fe7885bc2bacc6ce7bfe`.
Credentials are stored **only on the Pi**. Revoke the PAT in GitHub, then
disable the systemd timer, to cut off the channel. The old Cloudflare
SSH path remains available for break-glass maintenance.

## Create a status request from connected GitHub

An authorized actor creates a **private** GitHub issue with the exact title
above and this **exact JSON body**, not a Markdown fenced block:

```json
{"version":1,"action":"status","candidate_sha":"809d1f4ce7a5824c27ecf82d78cd779cd28f6e6a"}
```

After the timer polls and processes the issue, read its result comment through
the connected GitHub tool. Creating the issue **before the Pi agent is
installed** only creates an inert issue; it does not run a command.

## Second slice: pinned campaign summary (imported, not live)

This separate follow-up branch adds a read-only `report` action. It does not
migrate raw evidence or run the physical campaign. First, export the **existing**
Mac campaign with the authoritative manifest. The exporter verifies the
manifest SHA and excludes all evidence paths, notes, secrets, command payloads,
device IDs and names. It writes only aggregate counts and group counts.

After this branch passes Linux CI, the operator can fetch it into a separate
Mac worktree and deploy only the updated agent (the old timer and token remain
in place):

```bash
git -C "$HOME/StageCore" fetch origin qualification/pi-campaign-summary-control
git -C "$HOME/StageCore" worktree add --detach "$HOME/StageCore-control-report" origin/qualification/pi-campaign-summary-control
cd "$HOME/StageCore-control-report"
python3 tools/qualification/pi_control/export_summary.py \
  --state "$HOME/.local/state/stagecore/qualification-campaign.json" \
  --manifest tools/qualification/manifest.json \
  --output "$HOME/.local/state/stagecore/qualification-safe-summary.json"
scp tools/qualification/pi_control/agent.py stagecore-qualification:pi-agent-next.py
scp "$HOME/.local/state/stagecore/qualification-safe-summary.json" stagecore-qualification:pi-campaign-summary.json
ssh -tt stagecore-qualification 'sudo systemctl stop stagecore-qualification-control.timer && sudo install -o stagecore-control -g stagecore-control -m 0600 "$HOME/pi-campaign-summary.json" /var/lib/stagecore-control/campaign-summary.json && sudo install -o root -g root -m 0644 "$HOME/pi-agent-next.py" /opt/stagecore-qualification-control/agent.py && sudo systemctl start stagecore-qualification-control.timer'
```

Before issuing a request, verify locally on the Mac that the source state
remains pinned to the **installed** candidate SHA and that CI passed for the
exact control-agent code. Do not use these install commands to modify Hub,
credential files or the archived original campaign.

After the next timer tick, a trusted actor can create the private Issue title
`StageCore Qualification Request v1` with the JSON body:

```json
{"version":1,"action":"report","candidate_sha":"809d1f4ce7a5824c27ecf82d78cd779cd28f6e6a"}
```

The Pi returns `MAC_IMPORTED_SNAPSHOT_NOT_LIVE_PI_EVIDENCE` with only
manifest-validated counts. A missing or mismatched snapshot yields
`IMPORT_REQUIRED_NOT_LIVE_PI_EVIDENCE` or
`INVALID_IMPORTED_SUMMARY_NOT_PHYSICAL_PASS`, never a fabricated PASS.
This summary is an interim visibility step. The Mac still owns the raw
79-gate campaign until a subsequent explicit, integrity-checked migration.

## Next slice, deliberately not implemented here

- Safely move/import the existing zero-completed 79-gate campaign and its
  provenance from the Mac to a Pi-authoritative private directory; preserve
  archived prior campaign and raw logs.
- Adapt runner for local Pi execution with no SSH-to-self and no Go compiler;
  use separately authenticated exact-SHA GitHub CI evidence in lieu of a
  macOS `go test` preflight.
- Add typed `run-readonly`, `retry`, and `collect-sanitized-report` with
  bounded timeouts and a durable per-gate history. Do not promote stale
  milestone PASS after repin (#227), or confuse unavailable hardware with a
  product FAIL.
- Keep SHOW interlocks, any service restart or update, network fault injection,
  device outputs and physical observations outside unattended control.

Refs #226, #227, #228, #225.
