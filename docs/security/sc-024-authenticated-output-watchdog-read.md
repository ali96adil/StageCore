# SC-024 Phase 5F — Authenticated Hub evidence → read-only Operator output view

**2026-10-18 · PR #530 Draft integration; NOT wired into production Hub.**

The prior passive `outputwatch.Monitor` remains separate and cannot authenticate network peers. Phase 5F adds `outputwatch.CurrentHubEvidence` which reads the **existing** Hub device repository and current authenticated V2 socket; it never requests a fresh probe, reconnects devices, modifies assignments, dispatches cues, or replays old commands.

The reader requires a device's Hub-owned `ACTIVE` assignment with exact ProjectID, positive Epoch, and an immutable RuntimeSnapshotID. It verifies V2 enabled device identity and compares the current authenticated DeviceChannel scope against the assignment's Project, Epoch, and Snapshot. Foreign, unassigned, BLOCKED, disabled and legacy devices are not listed as another Project's outputs. A socket that has not completed current assignment/snapshot validation is **AWAITING_FRESH_REPORT**, not READY.

A fresh (5 second) same-generation software report can be summarized as software-only. Reports older than that, in the future due to clock skew, from another socket, or without expected scope never count as fresh. In the current firmware, the opt-in read-only V2 Lighting probe is mostly restricted to BLOCKED/unassigned devices, **so ACTIVE devices will normally show AWAITING_FRESH_REPORT** until a reviewed current-scope firmware reporting mechanism exists. No heartbeat is invented, and the software report cannot prove physical blackout. There is deliberately no `software acknowledged command` claim because the observed probe and generic device liveness data cannot be bound to a specific output command.

## Opt-in Project API

`WithScopedOutputWatchRead(auth, projectaccess, CurrentHubEvidence)` registers:

`GET /api/v1/projects/{project_id}/stage-devices/{device_id}/output-watch`

Only a secure, authenticated browser session with current `project.view` permission can access the selected Project; the authorization is checked before and after the read. Foreign and unknown devices return indistinguishable 404. Successful reads are no-store and return `read_only:true`, `automated_replay:false`, `physical_status:UNKNOWN`, with `commands_enabled:false` and `physical_output_verified:false`. No endpoint dispatches a command or proves actual DMX, light, audio, tablet or laser state.

**The Hub assembler does not install this Option.** Its automatic-install regression guard prevents connecting it to production while all-route ACL cutover is unfinished.

## Operator Stage Devices UI

In Phase4 Stage Devices, an ACTIVE V2 device card can display a bilingual **Read output watchdog / اقرأ حالة مراقبة المخرجات** control. Clicking it issues a single GET for that exact selected Project and Device. Client code validates the read-only/unknown/false flags before displaying an observation. It never sends GO/STOP, runs a timer, retries commands or silently clears a blackout. If the opted-in API is missing or the Project grant was revoked, a neutral UNKNOWN/unavailable message appears. Stale asynchronous responses from a changed Project/page are discarded.

Current endpoint and panel should be treated as **preview only**. There is no certification of physical output or device firmware failsafe, and no enrollment of Companions, audio endpoints or legacy Tablets in this partial V2 view. A future read-only adapter can connect authenticated heartbeat sequences to the passive Monitor, with fresh generation/epoch checks and event drop detection, before claiming a full device Watchdog.

## Tests

- HubEvidence: foreign Projects, wrong Snapshot, stale/old generation, time skew, current socket without output report, software zero not physical zero, nonzero software levels.
- HTTP: Owner/Operator per-Project grant, other Project, anonymous browser, insecure HTTP, revocation, unknown/unassigned device, no opt-in by default, and rejection of an incorrect reader scope.
- UI contract: bilingual labels, escaped status, read-only envelope check, exact Project ID and device ID, and absence of command-send/replay code.

**No main merge, no Pi deployment, no Stage Device firmware flash, no physical show changes.**
