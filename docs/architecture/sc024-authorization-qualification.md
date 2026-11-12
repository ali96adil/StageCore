# SC-024 authorization qualification

[Route inventory](sc024-http-route-inventory.tsv) contains Hub registration expressions with the 18 indirect patterns resolved to their
actual paths, plus the three standalone camera relay routes. GET registrations
also serve HEAD through Go's ServeMux. Patterns are registration records,
including options used only by qualification tests. This is a coverage
inventory, not proof that a route uses the prospective policy.

| Surface | Qualified behavior | Remaining qualification before cutover |
| --- | --- | --- |
| Projects and Show Capsule | Explicit alternate assembly qualifies list, detail, dashboard, rename, preview, export and Owner-only creation, duplication and global import. | The qualification route registry additionally checks cue/draft/publish, project configuration, execution environments and runtime intent before service entry. Qualified browser GO and Jump recheck authorization at each action dispatch. Atomic coupling to the adapter and remaining mutations still need review. |
| Project Files and browser Vault | Scoped paginated listing and version-bound attachment downloads in the browser. Shared bytes do not grant a foreign project version. Downloads recheck both grant and session before and after each bounded read. | Activation and remaining execution-environment media and ticket flows. |
| Companion Vault | `WithVault` always installs its backing media catalog for Companion read authorization; a missing scope fails closed. Roles with a published Runtime Snapshot can read only that role's registered required media; pre-Snapshot roles retain compatibility for media registered in their assigned Project only. Assignment and Runtime Session are rechecked before and after every bounded read, including range transfers. | Capture upload qualification checks the current device ticket before and after each bounded read. Cached capture receipts also match Companion connection, environment, Machine Role, Snapshot and operation. New browser-issued tickets also fence issuance, transfer, completion, and receipt replay to the issuing browser session. Qualification requests recheck the project runtime and media grants. Existing tickets retain their original scope. Hardware/deployment compatibility qualification remains open. |
| SSE | The current feed emits a session-ready event and keepalive, with exact-session and user fencing and bounded writes. It has no project payload. | Any future project event publisher must resolve project ownership and check delivery grants before writing. |
| Device | Staged device metadata reads require current audited V2 assignment and live project.view. Hub-wide inventory diagnostics are Owner-only in that assembly. | Qualification reserves raw commands, transfers, setup and firmware mutations for Owner, and checks project configuration grants on mapped device edits. Qualified Cue dispatch rechecks the originating grant at the Stage Device socket write. Qualified Owner-only direct commands now also carry live Owner/session authority to the device write boundary. Firmware and setup writes plus Tablet/Lighting/Laser assignment preparation also check live authority at the socket. Assignment commits recheck the originating browser session. Already-sent work and remaining asynchronous paths need qualification. |
| Plugin and extensions | Qualification-only Owner boundary for the 28 Hub-global extension and plugin administration registrations. | Qualified Cue dispatch rechecks the originating grant before the OSC plugin pipe write. Runtime probes check authority at process start and after handshake. Enable and Disable check before durable intent changes. Install/uninstall and permission writes also check live authority before changing durable state. |
| Audio | Optional StageAudio project, Vault version and SHOW fence. Live grant/session checks before commands, at the Unix socket write, and after sidecar replies. Scoped LOAD and PLAY also fence the exact rehearsal at write time. | Unbound legacy audio is Owner-only in qualification. Hardware-side revocation during a device operation remains open. |
| Camera relay | Optional standalone bearer authorization separates viewing from flash control. Active viewers are disconnected on credential rotation, and protected LAN listeners require TLS. | The opt-in Hub browser viewer checks project.view and media.read on the fixed configured project and revalidates the exact browser session and grants during streaming. Tablet delivery and deployment verification remain open. |

The source guard rejects wiring the alternate Project/Capsule/Files options
into the production Hub before the coordinated ACL cutover. Migration 53 is
unchanged, and the legacy assembly continues to serve previously created shows.


Preflight rejects an explicit foreign Snapshot and checks live authorization after evaluation. In qualification mode a project viewer without media.read receives readiness status without media hashes or item checks.

Every registered project-ID browser pattern in the inventory now has an
explicit qualification policy, including legacy memory, simulation,
preflight, visual and tablet routes. The opt-in router rejects new project-ID
patterns without a policy. Legacy registrations for project listing, device
metadata and unbound StageAudio inherit the qualification context. This
coverage includes the live grant check at the Companion, Stage Device, and OSC plugin Cue send boundaries. Other asynchronous dispatch paths still need qualification.

## SC-023 cutover and rollback rehearsal

The current release candidate uses the legacy production assembly. The
qualified route registry and the Owner cutover impact preview are exercised
through tests. The preview deliberately reports `ready_for_cutover: false`
and does not grant access or activate enforcement.

### Before a future cutover

1. Record the approved current bundle checksum, installed revision, managed
   configuration paths and authoritative Data Root. Retain the previous
   release bundle and its checksums beside the candidate.
2. Run the [F-010 update dry run](../features/F-010-safe-update-rollback-foundation.md)
   against a disposable copy of the installation. Review Doctor, the managed
   paths, and the candidate bundle verification. A dry run creates no
   rollback snapshot.
3. On the disposable database, review the Owner impact preview for each
   active operator. Confirm that Project visibility, GO, independent STOP
   and Blackout grants match the intended assignments. Include an account
   with revoked access and an unrelated Project in the rehearsal.
4. Verify a rollback snapshot can be created and restored on the disposable
   installation. Record the snapshot path, verification outcome, expected
   previous revision, and Doctor result after restoration. Keep any copied
   operator identities, grants and session records private.
5. Require an idle Hub, a designated rollback operator, a reviewed production
   assembly change, and separately qualified device safety behavior before
   scheduling activation.

### Rollback decision

Treat an inaccessible intended Project, unexpected cross-Project access,
loss of independent STOP or Blackout authority, a blocked Doctor result,
or a migration/health failure as a cutover stop condition. The F-010 updater
automatically restores the verified cold snapshot if install or postflight
fails. If a later authorization check fails after a successful update, use
the reviewed recovery procedure for the verified pre-cutover snapshot. Do
not substitute an older executable on top of newer database state or
hand-edit permission rows to simulate rollback.

After recovery, verify the recorded previous revision, Doctor, restored
Project visibility and the legacy authorization behavior in the disposable
installation. Record the triggering failure and the resulting state. The
existing source guard stays in place until an explicit coordinated cutover.

## SC-024 continuation · 2026-11-12

Starting at `63fb413d`, the integration branch adds isolated regressions for
the asynchronous authorization boundary and a single test Router composed
from the existing staged ACL options. `TestRuntimeRevocationAfterCommandAcceptancePreventsSocketWrite`
observes a durably accepted Device command, revokes authority before the
socket write, and requires a terminal non-retryable failure with no WebSocket
command. `TestQualifiedDelayedCueStopsDispatchAfterShowGrantRevocation`
allows the action already across the simulated adapter boundary to finish,
then proves a delayed action is not dispatched after the live project grant
is revoked. These tests make no claim that external work already sent can be
retracted.

`TestSC024UnifiedScopedACLAssemblyRoleAndProjectMatrix` composes the staged
Project list, Cue, Runtime, Project Files/download, Capsule, StageAudio,
Companion Vault/capture, project grant administration, Owner import review,
and device metadata/controller options in one temporary-database Router. It
checks Owner, Operator and Viewer behavior, cross-Project denial, Companion
hash isolation, and Owner-only boundaries. Existing qualification tests also
cover closed/wrong Project Sessions, foreign Snapshots, live STOP and
Blackout grants, missing ACL fail-closed behavior, and the production
partial-activation source guard.

The isolated updater rollback regression now changes Project grants and
removes a published Snapshot in the candidate database, then verifies that
rollback restores the original Project visibility, GO, STOP, Blackout,
Snapshot manifest/hash, SQLite integrity and foreign-key state. The
Migration 53 compatibility and read-only Owner impact-preview tests pass on
temporary databases; the preview remains `ready_for_cutover: false`. This is
not a complete disposable installed-bundle dry-run/impact-preview/restore
rehearsal, which remains outstanding. No production assembler or Migration
53 change was made. No real STOP/Blackout or device output was sent.

**Decision: Atomic Project ACL cutover is not software-qualified.** The
current production assembly remains legacy and the partial-activation source
guard remains active. Remaining blockers are the full endpoint-to-permission
and delivery-boundary reconciliation for all asynchronous adapters and
maintenance paths; compatibility qualification for existing Companion
deployments and tickets; a disposable installed-copy migration, impact
preview and recovery rehearsal; and independent device-side STOP/Blackout,
StageAudio and already-in-flight output qualification. The test Router is a
combined API qualification fixture, not a booted copy of the complete Hub
production assembler.

## SC-024 continuation · 2026-11-12 · dispatch response revocation

Additional delivery-boundary regressions were reproduced and fixed in the
integration branch:

- Lighting commissioning `identify` and `apply-published-config` now recheck
  the live dispatch authority after transport acceptance and before returning
  command details. A command already accepted by the transport is reported as
  possibly sent and non-retryable; the Hub does not claim it can retract it.
- Tablet Controller command batches now recheck authority after dispatch and
  suppress all accumulated command IDs/results if the Owner session ends.
- Tablet live-flash now rechecks authority after the Relay call and response
  read, before returning Relay state or error details. Ambiguous outcomes are
  marked non-retryable. The Relay is mocked in the regression test.
- Stage Device command batches now recheck authority after the dispatch loop
  and suppress accepted command results when the Owner session ends mid-batch.

Regression coverage includes
`TestLightingCommissioningSuppressesSuccessAfterOwnerRevocationPostDispatch`,
`TestTabletControllerSuppressesAcceptedResultsAfterOwnerRevocation`,
`TestTabletLiveFlashSuppressesRelayResultAfterOwnerRevocation`, and
`TestStageDeviceBatchSuppressesAcceptedResultsAfterOwnerRevocation`. The
focused `internal/httpapi` Race tests passed locally. The last code checkpoint,
`221289307e29e67a79ee0a2c229f7e77e0b0b97e`, passed Core CI including Go 1.26
Race and Linux ARM64, Companion Core including the macOS acceptance job on its
second attempt, and Camera Relay CI. The first Companion attempt was cancelled
before its macOS job started; the retry completed successfully.

Production Assembler and Migration 53 remain unchanged. Production ACL remains
disabled. **Atomic ACL cutover is still not software-qualified.** The remaining
software work is a complete endpoint-to-permission and final-delivery audit of
the remaining maintenance, Companion ticket/upload, Capsule, Vault/media,
plugin/extension and asynchronous adapter paths; a disposable installed-copy
Migration, impact-preview, update dry-run, restore/rollback and health cycle;
and old Companion/session/ticket compatibility qualification. Hardware-only
qualification still includes real Companion deployment compatibility,
device-side STOP/Blackout behavior, StageAudio revocation during a physical
operation, and already-in-flight external effects. No hardware was used in
these software tests.

**Resume at:** remote branch `integration/stagecore-rc`, code checkpoint
`221289307e29e67a79ee0a2c229f7e77e0b0b97e`, PR #534. Start with the next
uncovered asynchronous endpoint group and the disposable installed-copy
recovery rehearsal; do not change Production Assembler or enable ACL.

## SC-024 continuation · 2026-11-12 · final response fences

The next dispatch-response review added final live-authority checks to the
single Stage Device command and StageLaser visual-resync routes. If authority
ends after transport acceptance, the handler suppresses command identifiers
and results and returns a non-retryable response that says the external effect
may already have been sent. Regression tests
`TestStageDeviceCommandSuppressesAcceptedResultAfterOwnerRevocation` and
`TestStageLaserResyncSuppressesAcceptedResultAfterOwnerRevocation` were first
observed failing against the old behavior and pass with the fix. Focused
`internal/httpapi` Race tests pass locally.

The same review checked Companion capture upload fencing. Existing regression
tests exercise assignment and issuing-project-grant revocation during request
body reads, cancelled ticket state, and absence of stored Vault bytes; the
ticket/receipt code also has replay and terminal-state coverage. These checks
are simulated with temporary databases and do not qualify old deployed
Companion binaries on hardware.

Latest code checkpoint `b0993a2608361a909b0fc6f202809e86cf72ae9f` is pushed
to `integration/stagecore-rc`. Core CI run `43234033413` passed Go 1.27,
Go 1.26 tests, Browser Cue Delivery, Vet, Go 1.26 Race, and Linux ARM64
product builds. Companion Core run `43234033619` and Camera Relay run
`43234033375` passed. Local isolated rollback/Snapshot Race tests in
`internal/update` and `internal/projectaccess` also passed on temporary
databases.

One remaining code-level boundary is the HTTP response after the transactional
Stage Device software-transfer, StageLaser-assignment, and lighting-activation
operations return. Their runtime transactions reauthorize before committing,
but the HTTP handlers have no deterministic post-commit revocation regression
seam; they can still return the operation record if authority ends in the
small interval before response delivery. Qualifying this requires extracting
an internal runtime-operation interface or equivalent test seam, then adding
red-first tests and suppressing the record after a failed final check. This
must preserve the fact that a committed assignment is not rolled back and is
not safe to retry blindly.

The updater tests exercise dry-run non-mutation and automatic rollback against
temporary SQLite state, including project grants and Snapshot hashes. A full
disposable installed-bundle run with the real Linux service manager, Hub
restart and health checks has not been performed on this macOS host. The
Migration/impact-preview/restore rehearsal as an installed copy remains open.
Production Assembler and Migration 53 remain unchanged; Production ACL remains
disabled.

**Decision: SC-024 is not software-qualified.** Remaining software blockers
are the post-commit operation-response fence above, the full endpoint-to-
permission reconciliation for still-unreviewed plugin/extension and
maintenance flows, and installed-copy migration/update/recovery/health
qualification. Hardware-only blockers include deployed legacy Companion
compatibility, real device-side STOP/Blackout and StageAudio behavior, and
physical confirmation of in-flight output semantics. No hardware or real
output was used.

**Resume at:** remote `integration/stagecore-rc` HEAD
`b0993a2608361a909b0fc6f202809e86cf72ae9f`, Draft PR #534. Start with
post-commit fencing for Stage Device transfer/assignment/lighting activation;
add a failing regression before code changes. Keep Production ACL disabled
and the partial-activation source guard intact.

## SC-024 continuation · 2026-11-12 · committed-operation responses

The post-commit audit reproduced successful response leakage after the Owner
session ended for StageLaser assignment, software-only Stage Device transfer,
and Lighting activation. Each regression simulates a completed operation,
revokes the browser session before handler delivery, and previously observed
HTTP 202 with the operation record. The handlers now perform one final
session/role/CSRF check before returning the record. On revocation they return
403 with `effect_may_have_been_committed: true`, `retryable: false`, and no
operation record. They do not attempt to undo a committed assignment or
reactivate/replay device work.

New coverage:
`TestStageLaserAssignmentSuppressesCommittedRecordAfterOwnerRevocation` and
`TestStageDeviceOperationsSuppressCommittedRecordsAfterOwnerRevocation`
(software transfer and Lighting activation subtests). The focused tests passed
under `go test -race`; the complete local `internal/httpapi` Race package also
passed (`592.637s`).

The code commits are `e58539c56ac0563db1d463537d66855865a8bec6` and
`2797b284452e5f844cfd0e7005a498c6aa7ca9f9`, both pushed to the same branch.
Core CI run `43240030135` passed Go 1.27, Go 1.26 tests, Browser Cue
Delivery, Vet, Go 1.26 Race, and Linux ARM64 product builds. Camera Relay run
`43240030290` passed. Companion run `43240030394` attempt 2 passed Swift
build/tests; its real macOS acceptance job is still queued, so Companion CI
was not yet complete at that code checkpoint. The follow-up exact-head checks
on documentation checkpoint `f9e98b8341143a301c3e7003e466e012e50b59fe` all
passed: Core run `43245218960` including Race and Linux ARM64, Companion run
`43245222086` including the real macOS replacement/media acceptance job, and
Camera Relay run `43245218842`. Production Assembler and Migration 53 are
unchanged. Production ACL remains disabled.

**Decision remains: SC-024 is not software-qualified.** The broader audit is
incomplete for remaining maintenance/extension paths and multi-device
Snapshot synchronization responses. A disposable installed-copy update,
restore and Linux Hub service health cycle is also outstanding. Hardware
qualification remains separate. Continue on the existing remote
`integration/stagecore-rc` branch from documentation checkpoint
`f9e98b8341143a301c3e7003e466e012e50b59fe` (code checkpoint
`2797b284452e5f844cfd0e7005a498c6aa7ca9f9`). All exact-head Core, Companion
and Camera Relay CI runs listed above passed. Keep Production ACL disabled and
the partial-activation source guard intact.

## SC-024 continuation · extension maintenance response fence

The extension maintenance audit found that update and repair responses could
contain the completed operation record after the Owner session was revoked
inside the supervisor call. The handlers now revalidate the same live browser
session, CSRF token and `PluginManage` role before returning success or error
details. The update-plan read also revalidates its session and `ProjectRead`
permission before delivery. Update/repair audit records are written before
the final response fence; a revoked caller receives only a non-retryable 403
that says the operation may have applied, and must refresh installation
status. No completed mutation is rolled back or automatically retried.

Red-first HTTP regressions
`TestExtensionMaintenanceSuppressesCommittedUpdateAfterOwnerRevocation` and
`TestExtensionMaintenanceSuppressesCommittedRepairAfterOwnerRevocation`
passed under focused Race testing alongside the existing update/rollback/
repair RBAC test. Remaining qualification still includes the other
extension lifecycle routes, the multi-device Snapshot sync delivery path,
the full maintenance/plugin endpoint matrix, and isolated installed-copy
recovery/Hub health. This fence does not qualify extension package changes
under an active real stage installation.

The multi-device Snapshot synchronization response now has a final live
session/role/CSRF check after its device loop and before returning accumulated
results. It reports whether this request may already have committed a device
assignment and marks the response non-retryable. The regression
`TestStageDeviceSnapshotSyncSuppressesResultsAfterOwnerRevocation` was first
observed returning HTTP 200 with the Snapshot ID/results after logout; it now
passes under Race and verifies response suppression when no device operation
was performed. Assignment/transfer/activation post-commit fences from the
prior checkpoint remain in place. Production ACL remains disabled.

The extension runtime lifecycle audit then found that a completed uninstall
could return `payload_removed` and the installation identity after the Owner
session ended during work. The new regression confirmed the uninstall had
committed before the response and observed that result leak. Uninstall,
enable, and disable now revalidate the exact browser session, CSRF token,
`PluginManage` role, and dispatch gate before returning either results or
operation details. Revocation produces a generic non-retryable 403 that
reports the effect may have committed; it never repeats or undoes runtime or
filesystem work. The regression
`TestOperatorExtensionUninstallSuppressesCommittedResultAfterOwnerRevocation`
and the uninstall RBAC/CSRF test pass under Race. Production ACL remains
disabled.

The extension runtime-probe response had the same late-delivery gap: an
Owner could lose the session after a probe process had started and stopped,
then still receive its installation and execution details. A deterministic
regression observed that HTTP 200 response after logout. Runtime-probe
success and error delivery now recheck the exact session, CSRF, `PluginManage`
permission, dispatch gate, and qualification Owner gate. A revoked response
contains no probe result and states that probing may have run, with retries
disallowed. The regression
`TestExtensionRuntimeProbeSuppressesResultAfterOwnerRevocation` passes under
Race; the full `internal/httpapi` package passes. Production ACL remains
disabled.

Activation Staging also returned installation, permission and execution
authorization details without a final session check. The regression
`TestExtensionActivationStagingSuppressesResultAfterOwnerRevocation`
reproduced the HTTP 200 leak after Owner logout. Success and error delivery
now revalidate the same session and `PluginManage` authority; revoked callers
get only a non-retryable response because temporary staging work may have
occurred. The regression passes under Race together with the uninstall and
Runtime Probe regressions, and the full `internal/httpapi` package passes.
Production ACL remains disabled.

Firmware Artifact upload also returned its newly committed Artifact metadata
if the browser session ended after publication but before delivery. A
test-only callback reproduced HTTP 201 and metadata after logout, while
confirming the Artifact remained published. The route now performs a final
exact-session/CSRF/role check after publication and audit; revoked callers
receive no Artifact details and a non-retryable response that says publication
may have committed. The new regression passes under Race alongside firmware
send/authority and upload integrity tests; the full `internal/httpapi`
package passes. Production ACL remains disabled.

Firmware update issuance received the same final-response fence. Its regression
confirms that a revoked Owner receives no Manifest or update record after the
`ISSUED` record commits; the response reports the uncertain commit and forbids
automatic retry. Artifact upload, Manifest issuance and post-transport send
revocation regressions pass under Race, and `internal/httpapi` passes in full.
Any update already marked `SENT` remains recorded truthfully; this work never
attempts to retract or replay a firmware operation.

The follow-up Firmware read audit found two response-delivery gaps. Artifact
listing and update-status reads validated `DeviceFirmwareManage` only when the
request entered the handler, so a session revoked during the registry/SQLite
read could still receive the response. The red-first regressions
`TestFirmwareArtifactListSuppressesDataAfterOwnerRevocation` and
`TestFirmwareUpdateStatusSuppressesDataAfterOwnerRevocation` each reproduced
HTTP 200 with the data after logout. Both handlers now revalidate the same
live session and permission immediately before delivery and suppress the
payload after revocation. These reads have no external side effect, so the
response is a plain 403 and does not claim a commit or invite replay of a
device operation. The two regressions plus Firmware upload, manifest issuance
and post-transport send revocation checks pass under Race; the full
`internal/httpapi` package passes locally. Production ACL remains disabled.

The disposable update rollback rehearsal now runs the Owner Impact Preview and
an update dry-run against the same temporary installation fixture before
simulating candidate installation failure. It verifies the preview remains
read-only and blocked, the Operator's project visibility plus GO/STOP/Blackout
grants are intact, the dry-run leaves the prior bundle, database grants and
published Snapshot untouched, and automatic rollback restores SQLite
integrity, Snapshot identity/hash and the exact baseline grants. The focused
Race run passed. The installer and service manager remain mocked in this
test, so it does not qualify a real Linux service restart or Hub health check.
