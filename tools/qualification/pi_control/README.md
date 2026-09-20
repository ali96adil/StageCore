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

## Operator provisioning (not yet executed)

Install from a reviewed/approved version of this slice, **not by blindly
running unreviewed PR code on the Pi**:

1. Create the system account `stagecore-control` (system, no interactive
   shell), and private state directory `/var/lib/stagecore-control` owned by
   that account with mode `0700`.
2. Install `agent.py` into
   `/opt/stagecore-qualification-control/agent.py` owned by root and not
   writable by the agent.
3. Create root-managed `/etc/stagecore-qualification-control/config.json`,
   readable by the dedicated account, with the exact keys shown below.
   Create its `token` as a regular, non-symlink file owned by the dedicated
   account with mode `0600`. Provision the PAT **locally** through an approved
   protected prompt/file mechanism.
4. Install the supplied service and timer under `/etc/systemd/system/`;
   review them before `systemctl daemon-reload` and
   `systemctl enable --now stagecore-qualification-control.timer`.
5. Verify one `systemctl start stagecore-qualification-control.service`
   status run, then verify service logs are free of sensitive data.
   Do not enable the timer until the manual service check passes.

Example configuration for the exact qualified candidate (substitute a new
reviewed candidate and independently checked local binary hash before any
repin; **the agent refuses mismatches**):

```json
{
  "repo": "ali96adil/StageCore",
  "owner_login": "ali96adil",
  "pinned_sha": "809d1f4ce7a5824c27ecf82d78cd779cd28f6e6a",
  "expected_hub_sha256": "34681e3ce7595d4caf63f1a306f496f4172f398065a2fe7885bc2bacc6ce7bfe",
  "token_file": "/etc/stagecore-qualification-control/token",
  "state_dir": "/var/lib/stagecore-control"
}
```

## Create a status request from connected GitHub

An authorized actor creates a **private** GitHub issue with the exact title
above and this **exact JSON body**, not a Markdown fenced block:

```json
{"version":1,"action":"status","candidate_sha":"809d1f4ce7a5824c27ecf82d78cd779cd28f6e6a"}
```

After the timer polls and processes the issue, read its result comment through
the connected GitHub tool. Creating the issue **before the Pi agent is
installed** only creates an inert issue; it does not run a command.

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
