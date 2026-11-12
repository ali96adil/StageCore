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
