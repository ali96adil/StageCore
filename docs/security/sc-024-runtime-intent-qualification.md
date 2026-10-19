# SC-024 Phase 5C — GO, STOP and emergency-blackout authority qualification

**2026-10-18 · DRAFT integration #530 · not wired into live Hub or GO dispatcher.**

## Discovery

Current HTTP `operator_runtime.go` uses a broad browser
`PermissionRuntimeControl` for GO, JUMP, StopCue, StopSession, Project
Blackout and Emergency Blackout, and a separate role check for SHOW
enter/exit. Revoking a future `show.go` grant alone MUST NOT accidentally
revoke access to an authorized safety STOP, and granting `show.go` MUST
NOT implicitly permit a force exit or clearing an emergency blackout.

## Independent prepared capabilities

Two new Project-grant names are recognized by the non-enforced
`projectaccess` service: `show.stop`, `show.blackout.engage`.
Both require `project.view` for a non-Owner. VIEWER cannot receive
actuating capabilities, while an enabled Owner has Project override.

`CheckRuntimeIntent(ctx, userID, projectID, sessionID, intent)` is a
**READ-ONLY QUALIFICATION** API which never executes or queues a Cue,
changes the DB, writes an audit, touches a device, or authorizes an
existing HTTP endpoint. It resolves the exact ACTIVE Session through
the DB against its immutable `runtime_snapshots.project_id` and the
requested ProjectID. Guessing another Project's SessionID fails closed
even when the caller can control a different Project.

| Runtime intent | SHOW prepared authority | REHEARSAL prepared authority |
|---|---|---|
| GO / JUMP | `show.go` | `rehearsal.control` |
| StopCue | `show.stop` | `rehearsal.control` |
| StopSession (normal) | `show.enter_exit` | `rehearsal.control` |
| Engage emergency Blackout | `show.blackout.engage` | `show.blackout.engage` |
| Clear emergency Blackout | Enabled Owner only | Enabled Owner only |
| Force exit WITHOUT Blackout | Enabled Owner only | Enabled Owner only |

A sessionless `BLACKOUT_ENGAGE` check may use an exact ProjectID, but
a sessionless GO/JUMP/STOP/force exit cannot pass. SHOW/REHEARSAL
SessionIDs must be ACTIVE, and SIMULATION cannot inherit physical SHOW
authority. Owner still cannot use a foreign, ended or unknown SessionID
for GO.

Engaging and clearing Blackout have different trust boundaries.
`show.blackout.engage` NEVER grants CLEAR. Normal GO alone NEVER grants
STOP or Blackout. A user whose `show.go` grant is replaced without
removing separately assigned `show.stop` and
`show.blackout.engage` retains only those safety permissions.
This is an application-level candidate, **not** the final failsafe
policy. Losing `project.view` or the entire browser session still
blocks this scoped qualification; physical emergency fallbacks and
Owner/Technician recovery need separate independently verified design.

**Crucial:** clearing an engaged blackout, dispatching a Cue, and the
unsafe `FORCE_EXIT_WITHOUT_BLACKOUT` path must NOT be driven solely by
the preliminary policy helper. A real cutover must create an atomic
live authorization/operation fence, protect explicit confirmation,
check actual Session + Snapshot + active machine assignment at dispatch
time, and preserve an independent on-site safe-state control if the
Hub/DB or authentication goes away. Network failures must fail safely.

## Tests

`internal/projectaccess/runtime_authority_test.go` creates two actual
Projects, two published Snapshots and live SHOW Sessions and probes:
unassigned OPERATOR, exact membership+intent for Project A, guessed
Project B Session ID on Project A URL, real Project B access denial,
unknown/ended Session, missing Session ID, Owner override, live GO grant
revocation with STOP/Blackout retained, GO-only without STOP,
REHEARSAL authority, explicit revoke, and VIEWER actuation denial.

Owner-only `project-cutover/preview` (also unwired) reports
`prepared_stop_granted_projects`,
`prepared_blackout_engage_granted_projects` and
`go_without_independent_safety` per user. GO grants without both
independent safety grants trigger
`GO_WITHOUT_INDEPENDENT_STOP_AND_BLACKOUT_GRANTS` in blockers.
The report **always** remains `ready_for_cutover:false`.

These tests qualify future intent checks **only**. Existing routes,
GO/STOP/Blackout dispatch, Pi4, Tablet, StageLaser, Ableton, VDMX and
DMX are untouched. No new HTTP route is installed, so the HTTP route
inventory fingerprint is unchanged.
