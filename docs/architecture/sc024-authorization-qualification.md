# SC-024 authorization qualification

[Related component qualification matrix](related-component-qualification-matrix.md)
records the reviewed TabletPlayer, ESP32 lighting/camera/laser, Mac Companion,
StageAudio, VDMX, Ableton, and QLC+ evidence and its hardware blockers.

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

## SC-024 continuation · 2026-11-13 · runtime response fences

Three asynchronous result-delivery regressions were reproduced and fixed
with fake outputs only:

- USB-DMX manual output now rechecks the exact Owner session and current
  output gates after finishing its command record and before returning a
  result. If the Owner session ends after the final zero frame, the response
  suppresses the output result, reports that output may already have been
  sent, and prohibits retry. `TestUSBManualRouteDoesNotReturnSuccessAfterOwnerRevocation`
  first observed HTTP 200 after logout and now passes.
- Qualified Runtime GO and JUMP now recheck the exact active Session intent
  after cue processing. Revoking `rehearsal.control` while retaining
  `project.view` no longer returns the accepted cue execution result;
  `TestQualifiedDelayedCueStopsDispatchAfterShowGrantRevocation` observed the
  old HTTP 200 result and now verifies the committed-effect/non-retryable
  denial. Project ACL remains qualification-only and is not enabled in the
  production assembler.
- Project and Session Emergency Blackout routes now recheck the browser
  mutation authority before returning the safety result. The regression
  `TestEmergencyBlackoutSuppressesResultAfterOwnerRevocation` confirms that a
  latched blackout remains engaged while the revoked caller receives no result
  record and must not retry. The clear-path revocation regression now checks
  the same non-retryable committed-effect response and confirms that the GO
  latch remains set.

The three focused regressions and their adjacent Runtime/USB-DMX tests pass
locally. Full Core/Companion/Camera Relay CI for these commits is pending the
grouped branch push. No SC-024 code has been installed on the Pi; its isolated
systemd Update/Rollback qualification remains at the previously qualified
HEAD. Project ACL activation remains blocked by the outstanding endpoint
matrix, compatibility, and deployment-cutover work above.

## SC-024 continuation · 2026-11-13 · read and committed-result fences

Four additional response-boundary gaps were reproduced with red-first
regressions and fixed on `integration/stagecore-rc`:

- Extension readiness and runtime status now revalidate the exact live browser
  session and current read permission immediately before delivery. Revoked
  callers receive `403`, `Cache-Control: no-store`, and no installation or
  assessment data.
- Offline extension bundle import and trusted catalog sync revalidate the
  exact Owner session, CSRF token, `PluginManage` permission and dispatch gate
  after audit. A revoked caller receives only a non-retryable response that
  reports the package import may already have committed.
- Show Capsule export and materialization recheck session, role, CSRF and the
  scoped Project grant after the service returns. A committed operation is
  not undone; the response suppresses capsule/project details and marks the
  effect uncertain and non-retryable.
- Execution-environment capture rechecks committed mutation authority after
  Vault publication and manifest update. Its regression confirms the object
  and manifest stay committed when the Owner session ends before response
  delivery; the response contains no object hash and forbids blind retry.

The tests exercise temporary SQLite/Vault roots and fake Capsule services;
no production Hub, live StageAudio, DMX, OSC, MIDI, Tablet or Laser endpoint is
used. Focused `internal/httpapi` Race tests pass for the four groups. Their
initial red runs observed HTTP 200/201 and disclosed results after the session
was revoked. The grouped branch push and exact-HEAD CI are pending.

The route registration digest and Migration 53 remain unchanged. Production
Assembler still uses legacy authorization and its partial-activation source
guard remains active. This batch closes response-delivery findings only; it
does not establish complete endpoint-to-permission coverage or atomic ACL
cutover. **SC-024 remains not software-qualified.** Next audit the complete
route-to-policy matrix, remaining Companion/Vault ticket compatibility and
async maintenance/device paths, then run isolated Production-like assembly
and installed-copy migration/impact-preview/recovery tests. Hardware
qualification and Production ACL activation remain separate pending steps.

**Resume at:** `integration/stagecore-rc` after the grouped push for this
batch; preserve Production ACL disabled and all stable service/data evidence.

## SC-024 continuation · 2026-11-13 · Companion and environment-operation response fences

Two further response-boundary regressions were reproduced with temporary
SQLite fixtures:

- Companion capture upload previously returned HTTP 201 and the Vault hash
  when a Companion runtime session was revoked by the same commit that marked
  its upload ticket `COMPLETED`. It now revalidates the exact runtime session
  and the issuing browser session/current project grants before response
  delivery. Revocation returns a no-store, non-retryable committed-effect
  denial; the already completed ticket and Vault object remain intact.
  `TestCompanionCaptureUploadSuppressesResultAfterRuntimeSessionRevocation`
  and `TestCompanionCaptureUploadSuppressesResultAfterIssuerGrantRevocation`
  cover Companion-session and browser-issuer revocation respectively.
- An execution-environment operation could finish through its Companion
  runtime, then return `SESSION_INVALID` without indicating that OPEN,
  reconnect, capture, or restore may already have reached VDMX. Its final
  boundary now rechecks the exact browser session, CSRF, Runtime Control role,
  and qualification Project grants. A lost authority suppresses operation
  details and returns `effect_may_have_been_committed: true` and
  `retryable: false`. Revocation before runtime dispatch remains a plain
  denial. `TestExecutionEnvironmentOperationRechecksRevokedSessionAtBothBoundaries`
  was red before the change and passes under Race afterward.

The Companion capture upload regressions and adjacent replay, assignment
revocation, issuer-grant revocation, and SHOW admission tests pass under Race.
The focused operation/session revocation test also passes under Race. The
related TabletPlayer, ESP32 DMX Lighting, Camera, and StageLaser PR heads and
their current checks are recorded in
[`related-component-qualification-matrix.md`](related-component-qualification-matrix.md).
Those check results remain software evidence; the StageLaser independent
beam-off interlock and deployed tablet/camera/DMX behavior still require
physical qualification.

Production Assembler, Migration 53, stable services, and Project ACL state
were not changed. The prior four-commit group is on remote at
`9a8368123f92b8911783d30d49ea04a8144c0f43`; the grant-revocation and
Migration 54 compatibility tests below are local pending the next grouped
push. On the exact pushed HEAD, Companion Core and Camera Relay CI pass; Core
Go 1.27 and Go 1.26 Test/browser and Vet pass, while Go 1.26 Race and Linux
ARM64 are in progress.
The one full local `go test -race ./internal/httpapi -count=1` attempt timed
out at its 10-minute limit inside the existing unified ACL role-matrix test
while Argon2 password hashing was running; it reported no assertion failure.
It was not repeated. The changed Companion upload and environment-operation
regressions passed their focused Race runs. SC-024 remains
**not software-qualified**: remaining work includes the complete Hub
endpoint-to-policy/delivery matrix, isolated Production-like assembly and
installed-copy update/impact-preview/recovery cycle, and legacy Companion
compatibility qualification.

Follow-up local regressions extend this qualification evidence:

- `TestQualifiedEnvironmentOperationSuppressesResultAfterGrantRevocation`
  first reproduced a `404 PROJECT_NOT_FOUND` after VDMX returned, with no
  committed-effect or retry fence. The final authority check now suppresses
  the private result, returns `effect_may_have_been_committed: true` and
  `retryable: false`, and leaves `project.view` intact after
  `rehearsal.control` is withdrawn. It passes under Race.
- `TestCompanionUploadBrowserIssuerMigrationPreservesLegacyActiveTicket`
  migrates a fixture containing an active version-53 capture ticket through
  Migration 54, then proves its empty legacy issuer defaults, authorization,
  and completion are preserved. It passes under Race.
- Rehearsal start previously returned HTTP 201 with an active Session ID when
  `rehearsal.control` was removed by a SQLite trigger at the session commit,
  while `project.view` remained. The route now revalidates the exact browser
  session, CSRF, role, route scope and the mode-specific grant before delivery.
  If the start committed, a revoked caller receives no Session or command
  result, with both committed-effect field spellings and `retryable: false`;
  the active rehearsal remains recorded because the Hub must not pretend it
  was rolled back. `TestRehearsalStartSuppressesResultAfterGrantRevocationAtCommit`
  fails against the old 201 behavior and passes under Race. The fixture starts
  only an in-process REHEARSAL and sends no device command.

The new runtime-start regression is local on top of the two earlier tests;
local HEAD is `773557b`, four commits ahead of remote `9a8368123f92b8911783d30d49ea04a8144c0f43`.
The exact pushed-HEAD Core workflow is still in Race with ARM64 pending; do not
push another commit group until that run finishes, because the workflow's PR
concurrency setting cancels its in-progress predecessor on the next push.

**Resume at:** local `integration/stagecore-rc` HEAD `773557b` (four commits
ahead of remote `9a8368123f92b8911783d30d49ea04a8144c0f43`). Check the exact
pushed-HEAD CI, finish the remaining endpoint and installed-copy software
audit, add one related small commit, then push the group to PR #534. Keep
Production ACL disabled.

## SC-024 continuation · 2026-11-13 · STOP response fences

An Owner session revocation during the SHOW stop-safety boundary previously
returned the runtime's rejected command result directly. A mocked safety hook
can stand in for a STOP that may already have reached an external output, so
the response must not invite a blind retry. The red-first regression
`TestShowExitSuppressesResultAfterSessionRevocationDuringSafety` reproduced
that response and now verifies the browser result is suppressed, the response
is non-retryable with `effect_may_be_committed: true`, and the SHOW remains
active because authorization ended before the lifecycle transaction commit.

The same final live-authority fence is now applied after normal Session STOP,
forced Session STOP, and Cue STOP. The grant-revoked Operator regression
`TestShowExitRechecksGrantAfterStopSafety` now checks the committed-effect
contract. Mocked outputs and temporary SQLite are used; no physical output is
sent. Focused Runtime/GO/JUMP/STOP/Blackout Race tests and the full
`internal/httpapi` package pass locally. Core, Companion, and Camera Relay CI
for pushed HEAD `78cb30c06f031ebff86a8e21f5fd45e54659497b` is in progress;
do not push the local STOP fence before those runs complete. The previous
five-commit group is on the remote branch. Local code HEAD is
`9c45ffdf2d5eda6b7b0ffaa4c413f1b736fbd9c1`.

A separate async read review found that the Tablet command status route scoped
the command to its URL Project but delivered the loaded record without a final
grant/session check. `TestTabletCommandReadSuppressesDataAfterProjectGrantRevocation`
reproduced HTTP 200 after `project.view` was withdrawn between the DB read and
response. The route now performs `allowLiveProjectRead` before delivery; the
regression returns a redacted 404. The focused Tablet response tests pass under
Race, and the Tablet HTTP tests pass normally. Its private hook only coordinates
the test boundary and is not exposed by the public option.

The Machine Role list also exposed the global Companion inventory to a
project-scoped Technician. A two-Project regression first observed a Companion
assigned to the foreign Project in the response. Non-Owner lists now include
only unassigned Companions and Companions assigned within the requested
Project; Owner retains the global inventory. The route's qualification policy
now requires both `project.view` and `device.configure`, and the response
rechecks live session and grants after loading roles and Companion metadata.
The cross-Project, grant-revocation, Owner-inventory and existing Machine Role
tests pass normally and under focused Race.

These changes close the reviewed STOP and Tablet command-read delivery
boundaries, plus the Machine Role cross-Project and response-delivery findings.
The overall route-to-policy audit and disposable Production-like installed-copy
migration/update/rollback assembly remain open. Production Assembler,
Migration 53, stable services and Project ACL activation are unchanged.
**SC-024 remains not software-qualified.**

**Resume at:** the current local `integration/stagecore-rc` tip, five commits
ahead of remote `78cb30c06f031ebff86a8e21f5fd45e54659497b` after this batch.
Wait for exact-HEAD CI on the remote base before the grouped push. Continue the
endpoint-level async delivery audit and isolated assembly qualification; keep
Project ACL disabled.
