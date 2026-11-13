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
| Projects and Show Capsule | Explicit alternate assembly qualifies list, detail, dashboard, rename, preview, export and Owner-only creation, duplication and global import. The Owner cutover impact endpoint is read-only and excluded from production assembly. | The qualification route registry additionally checks cue/draft/publish, project configuration, execution environments and runtime intent before service entry. Qualified browser GO and Jump recheck authorization at each action dispatch. Atomic coupling to the adapter and remaining mutations still need review. |
| Project Files and browser Vault | Scoped paginated listing and version-bound attachment downloads in the browser. Shared bytes do not grant a foreign project version. File listing and availability recheck the live session and `media.read` grant at response delivery; downloads recheck both before and after each bounded read. | Activation and remaining execution-environment media and ticket flows. |
| Companion Vault | `WithVault` always installs its backing media catalog for Companion read authorization; a missing scope fails closed. Roles with a published Runtime Snapshot can read only that role's registered required media; pre-Snapshot roles retain compatibility for media registered in their assigned Project only. Assignment and Runtime Session are rechecked before and after every bounded read, including range transfers. | Capture upload qualification checks the current device ticket before and after each bounded read. Cached capture receipts also match Companion connection, environment, Machine Role, Snapshot and operation. New browser-issued tickets also fence issuance, transfer, completion, and receipt replay to the issuing browser session. Qualification requests recheck the project runtime and media grants. Existing tickets retain their original scope. Hardware/deployment compatibility qualification remains open. |
| SSE | The current feed emits a session-ready event and keepalive, with exact-session and user fencing and bounded writes. It has no project payload. | Any future project event publisher must resolve project ownership and check delivery grants before writing. |
| Device | Staged device metadata reads require current audited V2 assignment and live project.view. Hub-wide inventory diagnostics are Owner-only in that assembly. Owner-only inventory, unassigned-device, transfer-status, StageLaser visual-check and Network Cockpit responses now recheck the live session and applicable Owner/pairing permission immediately before delivery. Setup AP maintenance rechecks authority after device ACKs and failures; if the effect may already have occurred, details are withheld with committed-effect flags and `retryable: false`. Firmware send responses also recheck authority after errors; ambiguous transport writes preserve the same non-retryable effect boundary. All five browser Firmware maintenance routes send `Cache-Control: no-store`. | Qualification reserves raw commands, transfers, setup and firmware mutations for Owner, and checks project configuration grants on mapped device edits. Qualified Cue dispatch rechecks the originating grant at the Stage Device socket write. Qualified Owner-only direct commands also carry live Owner/session authority to the device write boundary. Firmware and setup writes plus Tablet/Lighting/Laser assignment preparation also check live authority at the socket. Assignment commits recheck the originating browser session. Already-sent work and remaining asynchronous paths need qualification. |
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

## SC-024 continuation · 2026-11-13 · read-response revocation fences

Four local commits after pushed HEAD `cb5cfb2c61898893c48ac9fcce40bdee995ad755`
close additional authorization gaps on asynchronous and project-scoped read
responses:

- Timing Intelligence now rechecks the live browser session and project grant
  before returning a report-generation error.
- Project Stage Device listing rechecks the live session before returning
  either device data or scoped-list errors. A revoked Owner previously received
  the private device record; a revoked Operator previously received HTTP 200
  with an empty list.
- Live video-source listing rechecks before probing an eligible Relay and
  before returning source details or read errors. The regression fixture uses
  an RTSP URL that is not probed; no camera, relay, or stage device was called.
- Show Capsule preview rechecks the session and both project grants before
  returning a manifest-build error.

The red-first regressions are
`TestTimingIntelligenceDoesNotDeliverReportErrorAfterSessionRevocation`,
`TestProjectStageDeviceListSuppressesResultsAfterSessionRevocation`,
`TestProjectLiveVideoSourcesSuppressesResultsAfterSessionRevocation`, and
`TestShowCapsulePreviewSuppressesErrorsAfterSessionRevocation`. They reproduced
HTTP 503 error disclosure or HTTP 200 result delivery after logout, then passed
with the final authority check. `go test ./internal/httpapi -count=1`, focused
`internal/httpapi` Race tests for all four delivery boundaries,
`go vet ./internal/httpapi ./internal/devicechannel`, and `git diff --check`
pass locally. The current exact pushed-base Core workflow `43459674814` has
passed Go 1.27 and Go 1.26 tests, Browser Cue Delivery and Vet; Race is still
running and ARM64 is pending. Companion run `43459674704` and Camera Relay run
`43459674681` pass at that base. The four local commits are not covered by
remote CI until they are pushed.

No Production Assembler or Migration 53 change was made; the source guard and
disabled production ACL remain in place. **SC-024 is not software-qualified.**
The full route-to-policy reconciliation, remaining asynchronous and maintenance
delivery boundaries, deployed Companion/ticket compatibility, and disposable
installed-copy migration/update/impact-preview/restore rehearsal remain open.
Independent device STOP/Blackout, StageAudio revocation during physical work,
and in-flight hardware effects remain hardware qualification. No device output
or production data was used.

**Resume at:** local `integration/stagecore-rc` HEAD `d70e703` plus this
documentation checkpoint; remote remains `cb5cfb2c61898893c48ac9fcce40bdee995ad755`.
Finish exact-base Core Race/ARM64 CI, then push the tested local group normally
to the existing branch and Draft PR #534. Keep Production ACL disabled and do
not install the new code on Pi before its complete CI passes.

## SC-024 continuation · 2026-11-13 · capsule error delivery fences

Three local commits after pushed HEAD
`45677c187f45a61ffd3edd2ee37a4e742fbb23fe` extend the Capsule delivery audit:

- Import Plan errors now recheck the Owner session before returning capsule
  verification details.
- Export errors revalidate the exact session, role, CSRF token, and Project
  grants before returning a store error. The production exporter stages to a
  temporary directory and checks authority before its atomic promotion.
- Materialize errors revalidate before returning details. If authority ended
  while Restore was running, the response withholds details and reports
  `effect_may_have_been_committed: true`, `retryable: false` so a caller cannot
  blindly repeat a potentially committed restore.

Red-first tests
`TestShowCapsulePlanSuppressesErrorsAfterSessionRevocation`,
`TestShowCapsuleExportSuppressesErrorsAfterSessionRevocation`, and
`TestShowCapsuleMaterializeSuppressesErrorsAfterSessionRevocation` reproduced
HTTP 422/503 error disclosure after logout. They now pass with the expected
authorization boundary. The full `internal/httpapi` package passes locally
(`53.539s`); focused Race tests cover the current three Capsule changes. The
full Race package passed on the preceding code checkpoint in `795.012s` with
an expanded 20-minute Go test timeout. The current unpushed code will receive
exact-head Race CI after push. `go vet` and `git diff --check` also pass. The
exact pushed-base Core workflow `43466920955` has Go 1.27 and normal
Go 1.26 checks complete, with Race running and Linux ARM64 pending. Companion
`43466920915` and Camera Relay `43466920848` pass on that base. These three
local commits are not yet covered by remote CI.

Production Assembler, Migration 53, the source guard, and real show data remain
unchanged. **SC-024 is not software-qualified.** Remaining work includes the
full route-to-policy and asynchronous delivery reconciliation, existing
Companion/ticket compatibility, and a disposable installed-copy Migration,
impact-preview, Update Dry-run, Restore/Rollback and Health rehearsal. Physical
device STOP/Blackout, StageAudio and already-in-flight effects still need
hardware qualification; no real output was sent.

**Resume at:** local `integration/stagecore-rc` HEAD `cdab5e2`; remote
`45677c187f45a61ffd3edd2ee37a4e742fbb23fe`, PR #534. Finish exact-base Core
Race/ARM64 CI, then push the grouped local commits normally. Do not enable
Production ACL or install unqualified code on Pi.

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

## SC-024 continuation · 2026-11-13 · simulation and editor response fences

Three additional result-delivery gaps were reproduced with local regressions
and fixed on the existing integration branch:

- Every Simulation mutation (start, GO, STOP, start confirmation, checkpoint
  capture/restore, fault configuration/clear, target state and twin reset) now
  rechecks the active browser session and `rehearsal.control` before returning
  a success or service error. Status-derived errors and “no active simulation”
  responses also recheck the live project grant. The new start regression first
  observed HTTP 201 with the committed Session and command result after grant
  withdrawal; it now returns only a forbidden, non-retryable response that
  truthfully says the Session may have committed.
- Simulation input injection and Timing Intelligence mutations recheck live
  authority before delivering execution/selection results. Red-first tests
  revoked the Owner session after a simulated input completed and after timing
  selection returned; both old responses exposed committed results. Simulation
  injection still uses the simulation-safe executor and the physical probe
  remains at zero calls.
- Cue Action Preset create/delete responses and built-in/imported Show Template
  materialization responses now fence the exact session after the service
  returns. The preset and materialization regressions observed leaked records
  before the fix. Committed operations remain committed when the caller loses
  authority; callers receive no record and must inspect current state before
  retrying. Existing Cue Editor tests still preserve not-found behavior when a
  project grant is revoked before the write transaction commits.

Focused regressions passed normally and under Race. The complete
`internal/httpapi` package passed normally after the Simulation change; the
focused Simulation, input, Timing Intelligence, Cue Editor/Preset and Template
Race suites passed. The exact pushed base `a7b57d85aa5fa627fcae49752afc9cb1d050b21d`
has Companion Core and Camera Relay CI green. Core Go 1.27 is green; Core Go
1.26 Race is still running and ARM64 remains queued. These local commits are
not yet on the remote and have not been installed on the Pi.

Production Assembler, Migration 53, stable services/data and Project ACL
activation are unchanged. This response-fencing batch does not close the
complete endpoint-to-policy audit, legacy Companion compatibility, or the
installed-copy Production-like migration/update/impact-preview/recovery
qualification. **SC-024 remains not software-qualified.** Hardware qualification
and Production Activation remain separate.

**Resume at:** wait for exact-head Core CI on remote `a7b57d85aa5fa627fcae49752afc9cb1d050b21d`; then run the final focused/source-guard checks and push the tested local commit group to PR #534 without force. Continue route-to-policy reconciliation; keep Production ACL disabled.

## SC-024 continuation · 2026-11-13 · cue editor and visual mode fences

The follow-up asynchronous-delivery review reproduced four more cases where a
successful project mutation returned its record after the Owner session was
revoked at the response boundary:

- Cue create/replace/duplicate/delete and Snapshot publish now perform a final
  live session, role, and project-scope recheck before returning a record or
  validation output.
- Cue Editor Recovery PUT/DELETE suppress their private generation or
  recovery result after session revocation. A committed recovery remains in the
  database; the caller receives `effect_may_be_committed: true` and
  `retryable: false`.
- Cue reorder rechecks `cue.edit` before returning the reordered revision and
  list. The committed order is preserved while the revoked caller receives no
  cue data.
- Visual Engine mode changes recheck `project.edit` before returning the new
  mode/revision. The committed mode remains truthful in storage if the Owner
  session ends after the write.

Red-first regressions observed HTTP 200/201 with cue, recovery, order and
visual-mode data for the old handlers. The new tests pass normally and under
focused Race. Simulation, input injection, Timing Intelligence, Cue Presets
and Show Template suites remain green after the earlier batch. No output was
sent to hardware.

The code is local on `integration/stagecore-rc` after remote checkpoint
`76f929c67b66723f75199ec15d2d35c24f793057`; four small commits are pending
the next grouped push. Exact-head Core CI for `76f929c` (including Go 1.26
Race and Linux ARM64) is in progress; Companion and Camera Relay CI passed.
Run the full `internal/httpapi` package plus source guard, then wait for that
Core run before pushing. These changes have not been installed on the Pi.

Production Assembler, Migration 53, stable services/data and Project ACL
activation remain unchanged. The complete route-to-policy reconciliation,
legacy Companion compatibility and installed-copy Production-like recovery
cycle remain open. **SC-024 is still not software-qualified.**

**Resume at:** local `integration/stagecore-rc` (four commits ahead of remote
`76f929c67b66723f75199ec15d2d35c24f793057`). Check exact-head Core CI, run the
full HTTP API/source guard, append the grouped evidence, and push to PR #534
without force. Continue the remaining async-maintenance route audit; keep
Production ACL disabled.

## SC-024 continuation · 2026-11-13 · extension and security administration delivery

The follow-up Owner-result audit reproduced disclosure from the extension
library after session revocation. `GET /api/v1/extensions`, package detail,
and registration now recheck the live session and permission before releasing
the result, including service-error responses. If registration committed
before revocation, the registered package remains in the library while the
HTTP result is withheld with `effect_may_have_been_committed: true` and
`retryable: false`. Red-first regression cases observed the old HTTP 200/201
results for list, detail, and register; the focused tests now pass.

The global Security Operations routes now buffer their responses until the
same session and route permission are checked again. This covers audit and
secret reads, plugin permission reads/writes, user administration, and
Companion pairing/revocation. Mutations whose caller loses authority return a
non-retryable uncertain-effect response without exposing the completed
record. Plugin permission storage commits before its runtime refresh; when
refresh fails, the response now identifies the persisted effect and disables
blind retry. Red-first tests reproduced both a leaked plugin-grant result
after Owner logout and a refresh failure response that hid the stored grant.
The route source guard now recognizes the shared authenticated wrapper and
asserts that its final authorization check remains before buffered delivery.
The route-pattern inventory digest did not change.

`go test ./internal/httpapi -count=1` passes after these changes
(`46.698s`). Focused Race tests for extension library, security operations,
and route guards pass. The `88432f6` remote checkpoint has Companion Core and
native replacement/media acceptance PASS (run `43428647182`), Camera Relay
unit/Race/Vet/Linux ARM64 trial build PASS (run `43428647193`), and Core Go
1.27 PASS. Its Go 1.26 Race was still running and Core ARM64 was queued in run
`43428647236` when this entry was recorded. The new local commits have not
been installed on the Pi.

### Read-only sudo audit

`sudo -n -l` shows the password-required `(ALL : ALL) ALL` rule, the fixed RC
qualification helper, direct passwordless `systemctl start|stop|restart
stagecore-hub`, and fixed helper operations including `physical-command`.
That helper invokes the qualification command with `--allow-physical`; its
bounded allowlist includes real Tablet and lighting commands. No systemd unit
references the helper, and the RC update wrapper explicitly targets only
`stagecore-rc-qualification.service`. The staged
`/home/ali96adil/90-stagecore-codex.sudoers.new` draft contains only the fixed
RC helper rule. The installed sudoers files are root-owned mode `0440` and
could not be read without interactive sudo; no sudoers or service change was
made. Keep the direct production-service and physical-command grants pending
owner review, because they provide manual recovery and real-output authority.

Production Assembler, Migration 53, stable services/data and Project ACL
activation remain unchanged. Route-to-policy reconciliation, old Companion
deployment/ticket compatibility, and the Production-like ACL migration,
impact-preview and restore rehearsal still need completion. **SC-024 remains
not software-qualified.** Hardware qualification and Production Activation
remain separate.

**Resume at:** local `integration/stagecore-rc` HEAD
`fa4444b7a77c1eecaa5d6f50224db88a343df122`, three commits ahead of remote
`88432f63edc71f115f5fc1c3a5cc79b1c2c54853`. Finish the remaining endpoint
delivery audit, update this checkpoint after the grouped push, and wait for
the exact-head Core Go 1.26 Race and ARM64 checks before pushing. Keep
Production ACL disabled and make no sudoers or hardware changes.

## SC-024 continuation · 2026-11-13 · project delivery and Owner impact route

The latest endpoint review added final-delivery fences for project list,
project detail/dashboard, create/duplicate/rename, configuration drafts, and
Owner project-grant updates. Each result is withheld if the exact browser
session or required live project authority ends before delivery. A committed
write is not described as rolled back: response uncertainty includes both
legacy `effect_may_be_committed` and canonical
`effect_may_have_been_committed`, with `retryable: false`. Project metadata
responses set `Cache-Control: no-store`.

The delayed runtime regression now covers both GO and JUMP after revoking the
project grant between actions. The first mock action may already have crossed
its external boundary; the remaining action is not dispatched, and the result
retains the truthful uncertain-effect state. STOP, emergency blackout, and
Owner grant update regressions also assert the canonical uncertainty key.
These tests use mocks and do not send real output.

Added `GET /api/v1/project-access/cutover-preview` as a read-only Owner route
available to explicit qualification assemblies. It returns `no-store`,
rechecks the live Owner session and `user.manage` permission before response,
and never claims cutover readiness. Owner, Operator, Viewer, grant immutability,
session revocation, and route registration are covered in route and
production-shaped assembly tests. It is deliberately not wired into the Hub
Production Assembler. The reviewed HTTP inventory now contains 283
registrations, 18 indirect patterns, and digest
`f75ee5e8b2fc7fcf5ed62120ef65c903614d190e1ead792da69f49d80b5e2621`.

The complete `internal/httpapi` package passed after the route was added to
the production-shaped composition (`49.505s`); `internal/projectaccess` also
passed (`4.363s`). `internal/update` passed (`1.243s`), and `go vet
./internal/httpapi` passed. The Owner preview, ACL role matrix, route inventory
and production non-activation guards passed, as did the focused Race set for
project response delivery and GO/JUMP/Blackout. The full HTTP API Race package
passed locally with `-timeout 45m` in `747.352s` (12m27s), with no race report.

At this checkpoint local HEAD is `8d269dd0390ca1b9749c89ab9eb63287d1397c4f`,
nine commits ahead of remote `88432f63edc71f115f5fc1c3a5cc79b1c2c54853`;
the qualification route and test are still uncommitted. Remote Core run
`43428647236` is for the remote base: Go 1.27 passed, Go 1.26 Race failed at
its 20-minute test timeout while migrating the HTTP API test database, and
Linux ARM64 was skipped. A prior attempt on the same commit timed out in a
different capture-upload test while also opening a fresh test database. The
race threshold needs correction before the next Core run. Companion Core run
`43428647182` and Camera Relay run `43428647193` passed. These results do not
cover the local commits. No code was installed on the Pi.

### Sudo audit checkpoint

Read-only `sudo -n -l` on `stagecore-pi` confirms a password-required
`(ALL : ALL) ALL` grant plus passwordless direct `systemctl start|stop|restart
production Hub`, the fixed RC qualification command, and the qualification
helper's enumerated verbs including `physical-command`. The production Hub
and Camera Relay are active; the isolated RC qualification service is
inactive. The inspected RC wrapper pins its service and managed roots to the
separate RC environment and passes fixed command arguments.

The sudoers directory contains StageCore-specific drop-ins, but their
contents could not be read: the passwordless command allowlist does not
include file reads, and `sudo -n cat` correctly failed without prompting.
Mapping each listed command to its exact source line is therefore not yet
proven. The only matching systemd unit is the RC qualification unit; no
production service depends on its wrapper as `ExecStart`. No sudoers or
service change was made. The direct production-service rule may be needed for
manual recovery, and `physical-command` grants real-output authority; keep
both pending owner review.

Production Assembler, Migration 53, stable services/data and Project ACL
activation remain unchanged. The end-to-end route-to-policy audit and full
disposable Production-like migration/update/preview/restore qualification
still need closure. Software tests verify established Companion sessions
remain valid past credential-token expiry until revoked, and Migration 54
preserves an issuerless active upload ticket across upgrade. Compatibility
with already-installed Companion deployments still needs operational
qualification. **SC-024 remains not software-qualified.** Hardware
Qualification and Production Activation are still pending.

**Resume at:** branch `integration/stagecore-rc` local and remote
`056c2bfc99c66943a11cd01946756fb4f84babb9`. The ten-commit grouped push is
complete. Exact-HEAD Core run `43440138563` passed Go 1.26 Test, Browser Cue
Delivery, Vet, Race, and Linux ARM64 CGo-free product builds; Go 1.27 also
passed. Companion run `43440138457` and Camera Relay run `43440138599` passed.
The Project Files and StageAudio fixes below are after this checkpoint and
will require their own full CI run. The previous Core run timed out twice at
`-timeout 20m`; the full HTTP API Race package passed locally in `747.352s`,
and the full Race job now has a finite `45m` timeout. Keep Project ACL disabled
and do not modify Pi sudoers or production services.

## SC-024 continuation · 2026-11-13 · Project Files response authority

A red-first response-boundary test reproduced two leaks in the opt-in scoped
Project Files routes. `GET /files` returned HTTP 200 and workspace metadata
after `media.read` was revoked after the database read; `/files/availability`
returned HTTP 204 after its initial grant check was revoked. Both handlers now
revalidate the exact browser session and live `media.read` grant immediately
before delivery. Regressions cover grant removal and session revocation on
both endpoints; revoked callers receive no workspace or availability result.
The tests pass normally and under Race.

The complete `internal/httpapi` package passed after the grant-revocation fix
(`49.918s`), `go vet ./internal/httpapi` passed, and `git diff --check` passed.
The existing full package Race run on the pushed code passed locally in
`747.352s`; the grant and session response-boundary regressions pass under
focused Race. The route inventory, no-partial-activation guard, and unified
role/cross-project matrix tests passed again.
No Production Assembler, Migration 53, live service, or Pi data changed.

This Project Files fix and its session-revocation regression are local and not
yet committed or included in CI. Remote remains
`056c2bfc99c66943a11cd01946756fb4f84babb9`; exact-HEAD Core run `43440138563`
passed Race and Linux ARM64. Companion Core passed run `43440138457`, and
Camera Relay passed run `43440138599`. Include the local fixes in the next
grouped push and wait for exact-head CI before any Pi install.

## SC-024 continuation · 2026-11-13 · StageAudio offline status delivery

A red-first test reproduced a scoped read leak in the optional, unconfigured
StageAudio path. After reading the active operational session type, the
handler returned `active_session: SHOW` without checking whether `media.read`
had been revoked since request authorization. The offline-status path now
rechecks the live browser session and exact scoped grant immediately before
returning the status. The regression starts an isolated mock SHOW, revokes
the Operator's grant at the response boundary, and verifies the session state
is withheld. It passed normally and under focused Race. No sidecar or device
was contacted.

The Project Files and StageAudio fixes remain local and are not covered by the
passing base-head CI runs. The updated `internal/httpapi` package and focused
Race checks are being rerun before grouping and pushing both security fixes.

## SC-024 continuation · 2026-11-13 · Owner device-read delivery

The read-only Stage Device review found that several Owner-gated inventory and
diagnostic handlers authorized at request entry but wrote their assembled
results without checking whether the browser session still existed. A
red-first test reproduced the leak in `GET /api/v1/stage-devices/inventory`:
after logging out at the response boundary, the handler still returned device
identity, assignment, runtime state, and capability data.

The Owner read paths now revalidate the exact session, Owner boundary, and
route permission immediately before writing their results: device inventory,
unassigned-device listing, transfer status, StageLaser visual-check history,
and Network Cockpit. The table-driven regression revokes the Owner session at
each route's response boundary and requires HTTP 401 with no route payload.
The Stage Device owner-read matrix passes normally and under focused Race. It
uses local SQLite fixtures and does not contact hardware.

This device-read fix is local after branch HEAD `f67ecf7c389e0ec322a04944f7d3396a44ca75a6` and is not included in that checkpoint's CI. Core run `43444479730` has since passed Go 1.26 Race and Linux ARM64, as well as the regular tests and vet; Companion run `43444479734` and Camera Relay run `43444479816` passed. The Owner device-read change is included in the next grouped push.

## SC-024 continuation · 2026-11-13 · Setup AP ACK delivery

A mock-only red-first route test reproduced a stale success response for
`POST /api/v1/stage-devices/{device_id}/setup-ap-password`: after a simulated
device ACK and Owner session revocation, the old handler returned HTTP 200
with `maintenance_state: APPLIED`. The handler now rechecks the same live
dispatch authority before returning any ACK result. If authority ended after
the mock external effect, the response withholds ACK details and carries both
committed-effect flags with `retryable: false`.

The Stage Device runtime also distinguishes revocation before socket write
from uncertain outcomes after a write: transport-write failure, loss of the
ACK path, deadline, or authority loss after send are marked as potentially
committed and non-retryable. An explicit device rejection remains a distinct
device response. A second red-first regression reproduced disclosure of a
private device rejection detail when Owner logout occurred before the error
response. The handler now rechecks live authority before reflecting either
success or failure details; revoked callers receive a generic denial, while
uncertain committed effects retain both committed-effect flags and
`retryable: false`. Regression coverage uses a mocked configure callback and
does not connect to or command a physical device.

After the fix, full `go test ./internal/httpapi -count=1` and
`go test ./internal/devicechannel -count=1` passed. Both Setup AP response
revocation tests passed under focused Race; `go vet ./internal/httpapi
./internal/devicechannel` and `git diff --check` passed. Core run `43444479730`
passed on the prior `f67ecf7` checkpoint, including Go 1.26 Race and Linux
ARM64; exact-head CI for the new grouped push remains required.

## SC-024 continuation · 2026-11-13 · firmware send error delivery

A mock-only red-first test reproduced a firmware-send error response containing
private transport details after the Owner session had been revoked following
the send boundary. The route now rechecks live dispatch authority before
returning any send error. The device-channel runtime marks non-authorization
socket-write failures as uncertain because bytes may have reached the device;
that path reports `effect_may_be_committed`,
`effect_may_have_been_committed`, and the legacy `effect_may_have_been_sent`
field with `retryable: false`. If Owner authority ended, the route returns a
generic denial and retains those uncertainty fields without exposing the
transport detail. A successful post-send revocation response now also carries
the canonical committed-effect fields. Test senders stand in for firmware
transport and no physical firmware action is sent.

The focused firmware send response tests pass normally and under Race. The
full `internal/httpapi` and `internal/devicechannel` packages pass locally;
`go vet` for both and `git diff --check` pass. Exact-head CI for the prior
push was pending when this section was recorded; the later result is captured
in the qualification checkpoint below. This firmware change remains local
until the queued Companion check is resolved.

## SC-024 continuation · 2026-11-13 · qualification checkpoint

Remote `integration/stagecore-rc` HEAD `8470b197e356fd686d0836bd61a112da18ff513b`
passed Core CI run `43449132042`, including Go 1.26 Race and Linux ARM64.
Camera Relay run `43449132159` passed. Companion Core run `43449132092`
passed its Swift build/tests and, on the failed-job retry, the native macOS
replacement/media, renderer, large-media resume, and real Companion
acceptance checks. Its first queued attempt was canceled before a runner
started; the single failed-job retry passed.

Before recording this CI result, local HEAD was
`77847edcdaa308de111b22fc88bde6d2b8865b9d`, four commits ahead of the remote.
The local branch now has five commits ahead of the remote, including this
documentation checkpoint. The group contains the firmware uncertain-send
fence, regression matrix, and refreshed component evidence. The code tests pass:
full `internal/httpapi` and `internal/devicechannel`, focused HTTP API Race,
full `devicechannel` Race, `go vet` for both packages, and `git diff --check`.
The red-first firmware-send test first returned HTTP 409 with private details
after Owner logout; the fixed route returns a generic 403 with non-retryable
uncertain-effect fields. No Pi installation, production service, physical
output, or firmware was changed.

Production Project ACL remains disabled. The broader route-to-policy audit,
installed-copy migration/update/impact-preview/restore rehearsal, and
deployed legacy Companion compatibility remain open; hardware STOP/Blackout,
StageAudio operation revocation and independent laser-safe-off acceptance
also remain pending. **SC-024 is not software-qualified.**

At this checkpoint the five-commit group was subsequently pushed to
`integration/stagecore-rc` as `dc516dd9d797af01d12e5fdf5558f1f3bcd342a7`.
The current local/remote status and exact-head CI are recorded below. Keep
Production ACL disabled.

## SC-024 continuation · 2026-11-13 · firmware response cache control

A red-first test found that the Owner-gated Firmware artifact list returned
HTTP 403 after response-boundary revocation without `Cache-Control: no-store`.
The same cache gap applied to Firmware upload, manifest issue, update status,
and send responses. Each of the five routes now marks all of its responses
no-store before processing the request. Regressions assert the header on
upload, list, manifest, status, successful-send revocation, and uncertain-send
error paths.

The focused route tests and their Race variants passed. Full
`go test ./internal/httpapi -count=1`, `go vet ./internal/httpapi
./internal/devicechannel`, and `git diff --check` passed. Code commit
`06130bf` is local after remote `dc516dd`; Core run `43453654206` has passed
Go 1.27 tests/vet and is running Go 1.26 Race; Companion run `43453654028`
has passed Swift tests and is running its native macOS acceptance; Camera
Relay run `43453654320` passed. This cache-control change is not on those
remote CI runs.

## SC-024 continuation · 2026-11-13 · browser cache and device metadata fences

A red-first middleware regression found that authenticated browser APIs did
not set `Cache-Control: no-store` consistently: unauthenticated `401`, denied
role `403`, and successful private `200` responses all lacked the header. The
browser-session middleware now sets `no-store` before transport, origin,
session, CSRF, and role checks. Firmware maintenance routes retain explicit
no-store headers and route tests for their success, revocation, and uncertain
send responses.

A second red-first regression found that
`GET /api/v1/stage-devices/{device_id}` and its `/assignment` route returned
HTTP 200 with device/assignment metadata after Owner logout at the response
boundary. Both routes now revalidate the exact live session, current role,
required global pairing/read permission, and current Project binding/grant
immediately before delivery. Tests assert that both response payloads are
withheld after session revocation. They use temporary SQLite and do not
contact a device.

Local evidence for the current four-commit group after remote
`dc516dd9d797af01d12e5fdf5558f1f3bcd342a7`:

- `06130bf` adds no-store to all five Firmware maintenance routes;
  `22b3d99` records those response guards.
- `94d31c8` applies a default no-store policy to authenticated browser APIs.
- `ca7b37b` fences Stage Device detail and assignment responses after logout.
- The red-first regressions failed with cacheable responses and leaked
  metadata before their fixes. Focused tests and Race passed after the fixes;
  full `go test ./internal/httpapi -count=1` passed (`50.931s`), as did
  `go vet ./internal/httpapi ./internal/devicechannel` and `git diff --check`.

The preceding remote checkpoint `dc516dd9d797af01d12e5fdf5558f1f3bcd342a7`
has complete Core CI PASS in run `43453654206`, including Race and Linux
ARM64. Companion Core run `43453654028` and Camera Relay run `43453654320`
also passed. These runs do not include the four local commits; the grouped
push and exact-head CI remain pending.

Linked PRs remain open and unchanged: TabletPlayer #36 at
`91613da1c8e0455a501c7be4b6df8ac1ce73a6bc` (Android run `42719885664`
PASS); ESP32 DMX Lighting #39 at
`3ad774988b2f78eb385f1046f7668a7f29049ab1` (Firmware CI `42759555665`
PASS); ESP32 Camera #20 at
`455b066c6520e1c8a576d26dd2afd4b377652dde` (firmware build
`42761096513` PASS); and ESP32 StageLaser #56 at
`b14a5789132549f2724f77b342bbe4c6b0fc880c` (Firmware CI `42765890415`
PASS) plus #57 at `c4ff10b37dc2fbfd3ebe31e55f0f8d4869d3dc54` (Firmware CI
`38054411190` PASS). These are software checks; board-specific SAFE_OFF,
network recovery, installation, and physical output remain unqualified.

Read-only Pi verification still shows `stagecore-hub.service` and
`stagecore-camera-relay.service` active and `stagecore-rc-qualification.service`
inactive. No service, sudo policy, data, firmware, or physical output changed.
`sudo -n -l` lists a password-required `(ALL : ALL) ALL`, passwordless fixed
RC helper, direct passwordless start/stop/restart of the production Hub, and
fixed helper verbs including `physical-command`. `sudo -n cat` could not read
root-owned sudoers files without a password; no permission removal was made
because the production-service rule may be needed for recovery and the helper
can command physical outputs. Keep those grants for owner review.

Production Assembler and Migration 53 remain unchanged; Production ACL stays
disabled. The complete route-to-policy reconciliation, production-shaped
installed-copy migration/update/impact-preview/restore rehearsal, and old
Companion deployment compatibility remain open. Hardware STOP/Blackout,
StageAudio operation revocation, and independent laser-safe-off remain
pending. **SC-024 is not software-qualified.**

## Current resume snapshot · 2026-11-13

The last pushed StageCore commit is
`45677c187f45a61ffd3edd2ee37a4e742fbb23fe` on
`integration/stagecore-rc` (Draft PR #534). The next local code group is
`0a29378`, `e2fbb29`, and `cdab5e2`, followed by documentation checkpoints
`da98097` and this resume update. The full `internal/httpapi` test and focused
Race tests for the current changes pass; the full Race run passed on the
preceding code checkpoint and will run in CI for the new exact head. `go vet`
and diff checks pass. Core CI
`43466920955` on the pushed base has Go 1.27, Go 1.26 tests, Browser Cue
Delivery and Vet passed; Race is in progress and ARM64 is pending. Companion
`43466920915` and Camera Relay `43466920848` pass on the same base. Push the
local group normally only after those Core jobs finish successfully; then
wait for CI on the new exact head. Keep the Production Assembler and ACL
source guard unchanged until all Software Qualification blockers close.

## SC-024 continuation · 2026-11-13 · lighting authoring grant fences

Red-first SQLite-backed regressions found that Lighting configuration save
and delete passed raw request contexts into writes. Revoking `device.configure`
after route entry could still persist the binding change. Both store writes
now use `capability.ExecAuthorized` and the route's live Project mutation
context, so the permission check runs inside the write transaction. The routes
also recheck before delivery: if revocation follows a successful commit, the
binding remains intact and the caller receives both committed-effect fields
with `retryable: false`. Save-before-write, delete-before-write and
post-commit response tests pass under Race.

The same test-first audit found that Lighting, Tablet and StageLaser Cue
authoring could create project target aliases using an unguarded request
context. Red regressions demonstrated a target alias persisted after
`cue.edit` was revoked. Each route now supplies a scoped mutation context to
the transactional alias write, suppresses the result if current authority
ends, and returns the existing committed-effect denial if revocation occurs
after an alias has committed. The three focused regressions and their
existing authoring tests pass normally and under Race. No physical command or
device connection was used.

Full local verification for this batch: `go test ./internal/httpapi -count=1`
passed (`54.623s`); `go test ./internal/store -count=1` passed (`5.262s`);
`go vet ./internal/httpapi ./internal/store` and `git diff --check` passed.
The focused Race tests for Lighting config save/delete, Tablet list and all
three Cue alias cases passed. The full HTTP API Race package passed at the
previous pushed code checkpoint; exact-head full Race CI remains required.

## SC-024 continuation · 2026-11-13 · partial project-edit fences

Red-first PUT and DELETE regressions now cover revocation after `EnsureProjectDraft`
forks a validated revision but before the Lighting binding transaction. The
binding write rolls back; because the draft fork itself has committed, the
route withholds revision/binding details and returns the established 403
committed-effect response with `retryable: false`. The ordinary no-fork
revocation path keeps its previous 404 response. The complete HTTP API suite
and focused Race tests pass after preserving those API semantics.

Assistant Cue proposal application had the same split transaction: a validated
revision fork could survive a session withdrawal before cue creation. The
red-first test proved the fork remained while the cue transaction rolled back.
The route now reports the committed fork without revealing the item result or
inviting retry. Assistant single and batch apply also honor an Operator's live
`ProjectEdit` grant at their operation-specific RBAC checks; both paths now
have scoped Operator integration coverage. Visual Engine mode has equivalent
fork/revoke coverage and committed-effect handling. No device or physical
output was used.

The Pi was only inspected read-only in this continuation: production Hub and
Camera Relay remained active, and `stagecore-rc-qualification.service` remained
inactive. `sudo -n -l` showed password-required general `(ALL:ALL) ALL`, plus
passwordless fixed RC qualification and qualification-helper commands. The
helper allowlist includes production Hub stop/restart and `physical-command`.
Root-owned sudoers files could not be read without a password, and `sudo -n id`
confirmed no non-interactive general ticket. No sudoers file or service
was changed; source-file attribution and actual external callers still need
owner review.

## Current resume snapshot · 2026-11-13 · streaming and cue fork fences

Latest pushed StageCore HEAD is `f648ed277afdebd8ce4825c88e9618665e70d800`
on `integration/stagecore-rc`; Draft PR #534 remains open. Its exact-head
Companion CI `43489543293` and Camera Relay CI `43489543145` passed. Core CI
`43489543455` passed Go 1.26/1.27, Browser Cue Delivery, Vet, Race, and Linux
ARM64. All three workflows passed on the pushed base.

The prior five-commit push and four follow-up commits are covered by exact-head
Core, Companion and Camera Relay checks. Three newer code commits are local:
`5237278` checks browser session and Project grants before every Camera Relay
frame; `0e4e681` and `5ed785c` report a committed validated-revision fork if
Cue reorder/create is denied after session revocation. Red-first stream test
observed a JPEG frame after access removal; the frame-boundary check now stops
the stream before it is written. Red-first Cue tests observed plain 403
responses after a Draft fork; the corrected responses include committed
effect fields, `retryable: false`, and no Cue details. No real camera or stage
device was used.

After these three commits, the complete HTTP API suite passed in `57.358s`.
Camera viewer Race tests passed (`7.610s`), Cue authoring and reorder focused
Race tests passed (`6.162s`), and `go vet ./internal/httpapi` plus
`git diff --check` passed. The local code HEAD before this resume checkpoint is
`5ed785c42d346dc3fdd90e531144c04b4ec4b9e0`; it is three commits ahead of the
pushed head. The checkpoint and matrix are committed in `98db49e`; this resume
note adds the read-only sudo rule-source observation. The five local commits
after `f648ed2` are ready for normal push; require full exact-head CI before
Pi use.

Read-only Pi check: `stagecore-hub.service` and `stagecore-camera-relay.service`
are active; `stagecore-rc-qualification.service` is inactive. `sudo -n -l`
shows password-required unrestricted sudo plus passwordless fixed RC
qualification, direct `stagecore-hub` start/stop/restart, and helper commands
including `service-stop` and `physical-command`. No sudoers rule or service
was changed;
the directory lists `90-stagecore-codex`, `stagecore-hub`, and
`stagecore-qualification` as root-owned mode-0440 files, but their contents
remain unreadable without interactive sudo. The dedicated RC wrapper is not a
production service dependency. Source-to-rule attribution and external caller
usage still need owner review before restricting production service recovery
or physical-command authority.

Keep the Production Assembler, Migration 53 and ACL source guard unchanged;
Production ACL remains disabled. **SC-024 is not software-qualified.** Full
route-to-policy reconciliation, remaining asynchronous/maintenance delivery
paths, installed-copy migration/update/impact-preview/restore qualification,
and deployed legacy Companion/ticket compatibility remain open. Hardware
STOP/Blackout, StageAudio revocation and independent laser-safe-off remain
pending.

## SC-024 continuation · 2026-11-13 · failure-response and rollback fences

Two red-first regressions found that scoped authorization was rechecked before
successful response delivery but not before some failure responses:

- Owner Project cutover preview returned `PROJECT_ACCESS_PREVIEW_UNAVAILABLE`
  after the Owner session was revoked while preview generation failed. The
  handler now rechecks the exact Owner session and permission before returning
  either an error or the report.
- Scoped StageAudio media listing returned `STAGEAUDIO_MEDIA_UNAVAILABLE`
  when `media.read` was withdrawn immediately before file lookup. The handler
  now rechecks the browser session and Project grant before reporting lookup
  errors or returning media choices.

The regression tests first observed HTTP 503 on the revoked requests; the
fixed routes return a scoped denial and withhold the preview/media results.
No StageAudio sidecar, camera, or stage device was contacted.

Migration 53 now has an additional temporary-SQLite rollback test. It creates
a prospective Project grant, rolls back to migration 52, verifies only the
new membership/grant tables are removed, checks the legacy Project, Cue, and
published Snapshot identity/bytes, then reapplies migration 53 and confirms
that it creates no implicit grants.

Local evidence at code HEAD `e95115d`:

- `go test ./internal/httpapi -count=1 -timeout=15m` PASS (`57.927s`).
- Focused `internal/httpapi` Race tests for cutover preview and StageAudio
  media/status revocation PASS (`10.391s`).
- `go vet ./internal/httpapi` and `git diff --check` PASS.
- `go test ./internal/db -count=1` PASS (`0.903s`), including the Migration
  53 rollback-impact test.

The two authorization fixes and migration test are three local commits after
pushed `be13f86aaefb34fbe56102dff9612a71514cdfee`. At this checkpoint,
Companion Core and Camera Relay CI on `be13f86` pass; Core CI run
`43493613268` has Go 1.27 tests and vet passing, with Go 1.26 Race still in
progress and Linux ARM64 builds pending. Do not install or push this local
group until the exact pushed-head CI completes; after normal grouped push,
wait for exact-head CI before any Pi installation.

Read-only Pi check still reports `stagecore-hub.service` and
`stagecore-camera-relay.service` active and `stagecore-rc-qualification.service`
inactive. No Pi files, services, sudo rules, firmware, or physical outputs
changed. Sudo rules remain unaudited at the file-content/source-attribution
level because root-owned files cannot be read with the active limited
passwordless grants; the password-required unrestricted rule, direct Hub
start/stop/restart grant, and helper `physical-command` remain unchanged for
owner review.

Production Assembler, Migration 53, and its partial-activation source guard
remain unchanged; Project ACL is not active in production. SC-024 remains
**not software-qualified**: complete route-to-policy reconciliation and
production-shaped integration, remaining failure/async delivery fences, and
old deployed Companion compatibility evidence still need completion. Hardware
STOP/Blackout, StageAudio behavior during physical operation, laser independent
safe-off, and Production Activation remain pending.

**Resume at:** local `integration/stagecore-rc` HEAD `e95115d`; remote remains
`be13f86` while exact-head Core Race/ARM64 CI finishes. Draft PR #534 remains
open. Continue the route/async authorization audit, preserve production ACL
and the source guard, then push a normal 3–5 commit group and await its full CI.

## SC-024 continuation · 2026-11-14 · Pi 4 authorization performance baseline

The integration branch advanced normally from `be13f86` through eight small
commits to code HEAD `3a35f0a63ed0f4947be0a271ac6c8b9035298395`. Draft PR #534
remains open. Core CI `43504230469` passed Go 1.26 and 1.27 tests, Browser Cue
Delivery, Vet, Race and Linux ARM64 product builds. Companion Core
`43504230628` and Camera Relay `43504230748` passed at the same exact head.
The local and remote branch heads matched after the push. The supplied
historical count of 440 commits is stale; `git rev-list --count HEAD` reports
1441 reachable commits.

### ACL dispatch cost before any optimization

`BenchmarkProjectRuntimeAuthorization` exercises the live browser `ValidateCSRF`
and `CheckRuntimeIntent` SQLite-backed authorization boundary. GO, JUMP, STOP,
Rehearsal GO, Emergency Blackout and a four-action compound cue are named
scenarios. The compound scenario repeats session and Project grant checks for
each action. The benchmark does not execute the rest of the HTTP cue route or
call a device/adapter; action correctness and revocation are covered separately
by simulator/mock integration tests. Each result below is the median of three
3000-iteration runs with one Go CPU.

| Scenario | Raspberry Pi 4 mean | Pi p95 | Pi p99 | M3 mean | M3 p95 | M3 p99 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| SHOW GO authorization boundary | 0.916 ms | 1.086 ms | 1.490 ms | 0.102 ms | 0.116 ms | 0.137 ms |
| SHOW JUMP authorization boundary | 0.921 ms | 1.139 ms | 1.651 ms | 0.103 ms | 0.121 ms | 0.141 ms |
| SHOW STOP authorization boundary | 0.911 ms | 1.073 ms | 1.440 ms | 0.103 ms | 0.123 ms | 0.139 ms |
| REHEARSAL GO authorization boundary | 0.916 ms | 1.134 ms | 1.450 ms | 0.104 ms | 0.125 ms | 0.144 ms |
| Emergency Blackout authorization boundary | 0.906 ms | 1.069 ms | 1.445 ms | 0.104 ms | 0.123 ms | 0.137 ms |
| Compound cue, four action checks | 3.643 ms | 4.237–4.611 ms | 5.446–6.134 ms | 0.416 ms | 0.466–0.489 ms | 0.512–0.539 ms |

The ARM64 benchmark test binary was built from `3a35f0a` with
`GOOS=linux GOARCH=arm64 CGO_ENABLED=0`; its SHA256 matched before and after
transfer (`39c652c8c7c871cbc122033ad988c399692ca965be6a4cf90826fdfe2ac4a950`).
It used only `testing.B.TempDir()` SQLite databases, reached Migration 58, and
had no network, service, Vault, show-data or physical-output access. It ran
with `GOMAXPROCS=1`, `nice 19` and idle `ionice`.

The 75.499-second test process used 71.505 seconds user CPU and 3.986 seconds
system CPU; peak RSS was 118,972 KiB (sampled peak 118,840 KiB). These are
benchmark-process figures, not Hub memory measurements. Pi MemAvailable was
7,390,640 KiB before and 7,436,464 KiB after. `/tmp` had 3,921,395,712 bytes
free before and after the run while the temporary test binary was present.
After removing that binary, `/tmp` returned to 156 MiB used (4%). Hub PID 1062
and Camera Relay PID 1121 retained their prior start timestamps and remained
active; current `NRestarts` is 0. `stagecore-rc-qualification.service` remained inactive.
No service was stopped or restarted. The current SHOW/Rehearsal state was not
read, so no claim is made about whether a live session was active during this
isolated low-priority benchmark.

The measured authorization cost is about 1.1 ms p95 per action and at most
6.2 ms p99 for the four-action authorization component. The existing
performance plan's simple-route evaluation target is p95 <= 20 ms. This is
enough headroom to retain the immediate live SQLite revocation checks; no
permission cache or other runtime optimization was added. There is no
post-optimization measurement because no optimization was justified by this
baseline. Full HTTP cue latency and Pi soak/burst tests remain separate work.
The session-bound Emergency Blackout intent was measured; the sessionless
Project blackout intent was not measured separately. systemd did not expose
Hub/Camera `MemoryCurrent`, so reported peak RSS is for the benchmark process.

### Regression and remaining qualification

The full `internal/httpapi` suite passed at `7add54d` before the final firmware
response fence. At final code HEAD, focused Firmware tests passed normally and
under Race, as did focused GO/JUMP/compound/STOP/Blackout Race tests, the full
`internal/projectaccess` Race package, `go vet ./internal/httpapi`, the SC-024
route/source-guard tests, and `git diff --check`. A red-first firmware-upload
test reproduced a stale Owner session receiving `actual_sha256` on a hash
failure; the handler now rechecks before that error response. No firmware was
sent.

Migration 53 rollback-impact tests and Owner cutover-preview revocation tests
remain green. Production Assembler, Migration 53, the partial-activation
source guard and Production ACL state are unchanged; Project ACL remains
disabled. **SC-024 is not software-qualified.** Remaining software gates are
the final endpoint-to-policy and asynchronous delivery reconciliation, a
complete production-like ACL migration/preview/restore rehearsal, compatibility
evidence for already-deployed Companion/tickets, and a simple Owner-facing
Project permission UX. The current Project Access API is preparation-only and
the web app has no Project grant management page. Hardware STOP/Blackout,
StageAudio during a physical operation, independent laser safe-off, and
Production Activation remain pending.

The component matrix now records linked PR heads and checks re-verified on
2026-11-14. TabletPlayer #36, DMX Lighting #39, ESP32 Camera #20, and StageLaser
#56/#57 remain open; their latest checks pass. CI does not replace their
hardware/deployment qualifications.

No sudoers change was made. The previously observed password-required broad
sudo rule, direct production Hub systemctl grant and qualification-helper
`physical-command` grant remain subject to root-file/source and caller audit;
no authority was exercised during this session.

**Resume at:** code head `3a35f0a63ed0f4947be0a271ac6c8b9035298395` on
`integration/stagecore-rc`; Draft PR #534; SC-024 issue #511 remains open.
Core, Companion and Camera Relay CI pass at this code head. Continue with the
remaining endpoint/async ACL audit and a focused Owner-facing permissions UX
proposal without wiring Project ACL into Production. The Pi performance
baseline supports retaining live checks. Preserve the verified Pi backup,
Production services and disabled ACL. The latest local checkpoint update is a
documentation-only commit after this code head; group it with the next safe
3–5 commit push. Prompt for the next session: **«كمل»**.
