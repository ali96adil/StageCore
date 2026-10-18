# SC-024 Phase 5A — HTTP route registration drift gate and atomic Project ACL cutover

**2026-10-18 · Draft only · enforced by CI only · no runtime ACL change · no Pi deployment**

## Why this phase is necessary

The active Hub currently authorizes most browser endpoints using global
`userauth.Authorize(role, permission)`. Merely filtering the Projects list
or adding a safe Project Files endpoint does not protect direct IDs for Cues,
Snapshots, Sessions, devices or downloadable content. The security stack in
#518→#520→#521→#522→#523→#524→#525→#526→#527→#528 contains
**qualification-only or opt-in** helpers, not a system-wide cutover.

Source inspection on the stacked branch verified these specific current
registrations; the table below is a **risk triage**, not evidence that
the legacy endpoints are already Project-scoped:

| Source/family | Current credential boundary | Proposed authority at atomic cutover | Critical qualification |
|---|---|---|---|
| `operator_projects.go` list, direct get, dashboard | Browser/global `project.read` | `project.view`, Owner all | ProjectID SQL truth, filter before serialization, 404 foreign/unknown |
| `operator_projects.go` create/duplicate | Browser/global `project.edit` | Owner create, explicit source duplicate/edit and new Project identity | Clone MUST NOT copy user grants/device authority |
| `operator_cues_publish.go` cues, revision/publish | Browser/global read/edit/publish | `project.view`, `cue.edit`, `snapshot.publish` | Cue→Revision→Project, immutable publish/SHOW fence |
| `operator_runtime.go` status/start/stop/GO/JUMP | Browser/global read/runtime.control | Distinct `rehearsal.control`, `show.enter_exit`, `show.go` | Session/Snapshot Project ownership and dispatch-time check, **emergency STOP/blackout separate from GO** |
| `operator_stage_devices.go` inventory, pairing, assignments, commands, live sources | Browser/global mixed role guards | Owner global inventory; `device.configure`/lighting manual and runtime authority only to assigned Project | V2 ACTIVE epoch, approved assignment, published snapshot, safe-state audits; distinguish unassigned/decommission |
| `operator_stage_device_firmware.go` firmware mutation | Global `device.firmware.manage` | Owner/maintenance privileged global plus per-device authority, NOT generic Project access | Trust/provenance, physical maintenance, SHOW fence |
| `operator_show_capsules.go` export, list, import plan, materialize | Global `project.read`, `project.edit`, `backup.restore` | Scoped `capsule.export` for local exports; **Owner-only import/restore review and explicit restore** | Raw imported Capsule has no local Project membership, checksum/trust/path bounds, identity collision and SHOW gate |
| `server.go` Vault raw hash download | Companion StageCoreSession | Device/runtime manifest & assignment authority, NOT a browser `project.view` check | SHA-256 shared by Projects and cannot carry access control on its own |
| `server.go` software download | Separate software repository service | Software distribution and ownership rules, not Project media grants by default | Binary signature/provenance, token scoped download |
| `user_auth.go` SSE | Authenticated browser global `project.read` | Current session events may be global; future Project events require owner-Project binding and live subscription revoke | Last-Event-ID replay, names, counts, envelope leakage |
| `stage_device_runtime.go`, `stage_device_firmware.go` | Device/service identity (not browser) | Preserve device cryptographic identity, scoped transport and active assignment | Never force browser cookies/Project grants onto devices |

**Additional route families still require handler-by-handler tests:**
configuration and linked cues, assistant proposals, execution environments,
plugins/extensions, lighting/tablet/laser controls, note/memory history,
simulations, timecode, timing, HA, StageAudio (stacked #506–#508),
software/firmware downloads, backup operations, setup/bootstrap, QLC+/USB
DMX integration and all dynamically registered module/extension routes.

## New CI gate

`internal/httpapi/route_acl_registration_guard_test.go` parses all Go
source files in `internal/httpapi` except tests with Go's AST parser.
It captures every `HandleFunc`/`Handle` registration expression (including
method/path literals and **dynamic argument expressions**), sorts their
source+pattern identities, and compares the resulting SHA-256 to a reviewed
source baseline. CI FAILS when registrations are added, removed or changed
until the reviewer explicitly updates both this inventory and its digest.
The failure log prints the exact registration identities for review.

Limitations: a registration digest is **not** an authenticated policy proof,
does not inspect handler code, and does not cover dynamically registered
routes constructed outside `internal/httpapi` (including plugin/extension
runtimes). It needs supplemental AST/HTTP tests for handler reachability,
credential class, nested resource IDs and mutation dispatch safety.
Registration changes that don't alter method/path or source file can still
change authorization behavior; code review remains mandatory.

A second test blocks the three new opt-in scoped Files/Capsule route Options
from being wired by the production `cmd/stagecore-hub` or `internal/app`
assembler before the security cutover. This is a **negative integration
guard**, not a replacement for full route-policy enforcement.

## Atomic, fail-closed release design (NOT implemented here)

1. **Preflight data**: Choose the canonical single migration #53 (#518,
   not alternate #517/#519). Inventory existing users, explicit grants,
   active sessions and cross-Project links. Preview precise access changes
   to Owner; no implicit global Operator grants.
2. **Coverage gate**: All browser, module, direct-ID, media, device and
   event routes must be classified by principal and authoritative Project
   resolver. Non-Project global Owner/security/bootstrap routes must be
   *deliberately* exempt, not accidentally exempt. Test unknown and foreign
   object IDs, leaked metadata, revoked grants and guessed Vault hashes.
3. **Release unit**: Produce a single reviewed Hub build with a single
   immutable-at-startup Project ACL mode. Refuse partial configuration;
   never switch only some web pages. Keep legacy mode explicitly designated
   and OFF-by-default migration mode separate. No hot toggle during SHOW.
4. **Physical safety**: Block rollout during active SHOW or REHEARSAL,
   never revoke emergency STOP/blackout due to a lost normal GO grant,
   define technician recovery and offline Pi/Companion fallback with the
   operator. Grant changes during SHOW must be fenced atomically.
5. **Rollback**: Backup SQLite/Vault and store checksum/old binary before
   rollout. Abort to known previous version if gate validation fails; do
   not delete existing grant tables on rollback. Restore schema carefully
   because migration numbers cannot be blindly rolled back in a live Hub.
6. **Validation**: Full Go matrix, race, ARM64 packages, security route
   probes, Show Capsule/Owner restore dry run, revocation-in-flight
   tests, non-SHOW Pi 4 and real Tablet/Lighting/StageLaser/Companion
   rehearsals. **Only after signed-off, attended test** permit production
   switch. The already-tested Show binary stays pinned meanwhile.

## Current sign-off

- Route inventory automated in CI: **candidate gate**, not complete ACL.
- Browser Project scope in active Hub: **NOT ENFORCED**.
- Owner grant data and opt-in Project Files/Capsule routes: **DRAFT**.
- Imported Capsule mutation: **existing behavior unchanged**.
- Atomic feature switch / rollback / all-handler coverage: **NOT BUILT**.
- StageCore Pi: **untouched**.

Maintain `docs/integration/post-show-pr-stacks-2026-10-18.md` as the
dependency/source-of-truth for eventual PR consolidation. Never merge all
stacked Drafts individually into `main` without a fresh integration review.
