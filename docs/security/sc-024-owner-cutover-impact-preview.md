# SC-024 Phase 5B — Owner Project authorization cutover impact preview

**2026-10-18 · INTEGRATION DRAFT #530 · PREVIEW ONLY; NOT ENFORCED.**

The new `projectaccess.Service.PreviewProjectCutover(ctx, actingOwnerID)`
produces a stable read-only, SQLite-consistent comparison between the
**legacy global browser ROLE eligibility** and the **prepared per-Project
membership grants** that would apply after a future complete cutover.

An opt-in (unwired) `WithOwnerProjectCutoverPreview(auth, access)` registers:

`GET /api/v1/security/project-cutover/preview`

It requires an authenticated secure browser session, live enabled Owner
validation in the DB, and global `user.manage` Owner authority.
Non-Owners (including OPERATOR/TECHNICIAN/VIEWER) are rejected with 403,
anonymous users with 401, insecure non-loopback HTTP with 426. The API
sends `Cache-Control: no-store`. The unconfigured Hub returns 404.

## Meaning of the numbers

For each Owner-visible account, the report includes:

- Account ID, username, current role and enabled state.
- Number of Projects presently *role-eligible to view* under global
  `project.read`, compared to the planned number with live grants for
  `project.view`, and exact Project IDs/names for view loss.
- Number of Projects presently *role-eligible to execute GO* under global
  `runtime.control`, compared to Project `project.view` + `show.go`
  grants in the prepared policy, and exact Project IDs/names for GO
  eligibility loss.
- Owner retains complete access. Disabled accounts have zero current or
  prepared effective access. VIEWER is never eligible for GO; a stale or
  malicious stored grant must not override the role ceiling.
- Prepared independent safety coverage: number of Projects with
  `show.stop` and `show.blackout.engage` grants, and an explicit
  `go_without_independent_safety` Project list for users whose prepared
  GO access has no matching STOP or blackout engagement capability. These
  trigger the `GO_WITHOUT_INDEPENDENT_STOP_AND_BLACKOUT_GRANTS` blocker,
  never automatic safety escalation.
- Per-user and Project rows are consistently read from a **single SQLite
  read-only transaction**. Reads are bounded to 128 users, 128 Projects,
  and 4,096 user-Project combinations. A larger instance fails with
  `PROJECT_CUTOVER_PAGINATION_REQUIRED`; it never silently truncates.

**Warning:** `legacy_go_role_eligible_projects` measures the current
role gate only, not active Session/Snapshot/physical GO readiness.
`prepared_go_granted_projects` measures the future `show.go`
policy in a database, *not* what the Hub is enforcing today. There
is no promise that prepared GO automatically grants JUMP, STOP,
Emergency Blackout, Force Stop Session, REHEARSAL enter/exit or
Companion/Device transport authority.

## Why cutover is permanently BLOCKED in this phase

Every successful response is explicitly
`read_only=true, enforced=false, ready_for_cutover=false`,
`status=BLOCKED_SECURITY_CUTOVER_INCOMPLETE`.

The report always contains blockers for:

- All Project direct-ID, media, Vault, Stage Device and module routes
  must be secured at a **single** cutover point.
- Emergency STOP/blackout must have independent authority; do not let
  revoking ordinary `show.go` strand a live operator's safety controls.
- Show Capsules, imported restore, events/SSE, Companion/device download
  boundaries must be covered by the same threat model.
- Owner reviewed grant impact, durable backups, rollback path and
  attended non-SHOW physical validation.

An active SHOW or REHEARSAL session appends
`ACTIVE_OPERATIONAL_SESSION_BLOCKS_CUTOVER` and identifies the current
session type. This is an additional blocker, not a request to stop it.

## GO/STOP route authority review

**Current** `internal/httpapi/operator_runtime.go` uses global
`PermissionRuntimeControl` for GO, JUMP, `/runtime/stop`,
`/runtime/stop-session`, project/session blackout and `/runtime/start`.
SHOW start/stop-session additionally checks global
`PermissionShowEnterExit`. `/runtime/stop-session` accepts a
dangerous **Force** path with explicit
`FORCE_EXIT_WITHOUT_BLACKOUT` confirmation; this is **not**
equivalent to normal emergency STOP.

**Future** policy must distinguish at least:

- `show.go` (normal GO/JUMP, subject to live dispatch-time authority)
- `show.enter_exit` and `rehearsal.control` (session lifecycle,
  with active role checks and operational fencing)
- A separate narrowly defined emergency STOP/Blackout authority that
  can still operate on authenticated emergency controls despite loss of
  normal GO permission; accidental CLEAR/force-stop must remain restricted
- Hub-to-Companion/Stage Device runtime session capabilities, not
  browser cookies on device transports

This phase **does not** modify any of those runtime routes, permission
checks, driver actions or STOP/BLACKOUT behavior. It only gives the Owner
a truthful dry-run impact preview, to inform a future human-approved
atomic and rollbackable switch.

## Tests and deployment boundary

Unit and HTTP tests cover strict Owner-only review, anonymous/TLS denial,
legacy Operator loss across two Projects, isolated Project A grants,
Viewer's read-only ceiling, Owner override, immediate grant revocation,
disabled accounts, and non-mutation of grants. The new opt-in Option is
listed in the production assembly drift guard to prevent premature
installation.

**No changes to production Hub, main, Pi or active theater devices.**
CI success of this preview alone must NEVER be used to authorize a
production per-Project cutover.
