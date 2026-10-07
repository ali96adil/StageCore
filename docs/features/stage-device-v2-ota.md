# Stage Device v2 controlled firmware update

Status: the controlled OTA software path is implemented end-to-end: manifest validation, qualified artifact registration, pinned-Hub artifact download, Operator issuance/send, authenticated Stage Device v2 lifecycle, and StageLaser ESP32-C3 streamed OTA with rollback protection. Physical update/rollback/recovery qualification is still pending.

Tracked by #430 and required by StageLaser hardware tracker #429.

## Authority boundary

Firmware update is a maintenance operation. It is not a Cue, Mixed Cue action, Runtime Snapshot mutation, or ordinary show command.

The StageCore Hub remains the only update authority. A Stage Device may later download an image only from the same Hub identity and TLS certificate it already verified for its authenticated runtime. The manifest therefore carries a Hub-local artifact path instead of an arbitrary URL.

## Manifest contract

Core validates:

- schema version
- UUID update ID
- exact target device ID
- exact target profile ID
- current and target firmware versions
- exact source revision
- artifact qualification
- Hub-local canonical artifact path
- bounded artifact size
- canonical lowercase SHA-256
- issue/expiry window
- device-policy rollback requirement

Only `QUALIFIED` artifacts are eligible. CI artifacts explicitly marked `UNQUALIFIED` cannot pass this contract.

The first canonical artifact namespace is:

```text
/api/v1/stage-device-firmware/artifacts/
```

The path is intentionally relative to the pinned Hub. Scheme, host, query string, fragment, path traversal and arbitrary external URLs are rejected.

## Qualified artifact registry

The Hub stores device firmware artifacts below its DataRoot in a dedicated immutable registry. OWNER and TECHNICIAN can explicitly register an exact firmware binary as `QUALIFIED` from the Stage Devices UI. Registration is a maintenance/admin action only; it does not send, flash, reboot, or acquire show authority.

Registry acceptance requires:
- Hub-generated UUID artifact ID bound to one exact device ID
- exact target profile and firmware version
- exact 40-character lowercase source revision
- explicit `QUALIFIED` promotion by OWNER/TECHNICIAN
- expected lowercase SHA-256 supplied by the operator and recomputed from the uploaded bytes
- bounded non-zero artifact size recomputed from the upload
- canonical original filename
- immutable artifact IDs

The Operator upload is same-origin, authenticated, CSRF-protected, role-gated, size-bounded, and audited as `stage_device.firmware_artifact.qualified`. A SHA mismatch or non-`QUALIFIED` submission fails closed.

Every artifact is re-verified from disk before download. Missing, tampered or metadata-mismatched artifacts fail closed.

The download route exists only on the pinned TLS Stage Device gateway:

```text
GET /api/v1/stage-device-firmware/artifacts/{artifact_id}/firmware.bin
Authorization: StageCoreSession <token>
```

The session must still be valid and the authenticated device ID must exactly match the artifact target. Cross-device lookups intentionally return the same not-found response as missing artifacts.

This route is not installed on the Operator/browser server.

## Operator maintenance issuance

OWNER and TECHNICIAN roles have the dedicated `device.firmware.manage` permission. OPERATOR and VIEWER do not.

The Devices UI can list only verified `QUALIFIED` artifacts bound to the exact device and issue a short-lived maintenance manifest. Issuance is unavailable while any operational Session is active and requires the target device to be ONLINE + READY.

For StageLaser, issuance additionally requires the latest authenticated observation to prove:
- DISARMED
- stable OFF
- state quality TRACKED or CONFIRMED
- no resync required
- no relay pulse in progress
- no local Flash active

Issuance is audited and returns `delivery_state: NOT_SENT`. It does not send a maintenance message, write flash, reboot the device, or use Cue/show authority.

## Authenticated maintenance lifecycle

Firmware delivery stays outside ordinary `stage_device_commands`. The Operator first issues an immutable record in `ISSUED`, then performs a separate explicit send action.

Delivery uses the existing authenticated Stage Device v2 WebSocket only when the current socket advertises:

`device.maintenance.firmware-update`

The outbound message is `maintenance.firmware_update` and is bound to the exact device ID and current connection generation. The device reports lifecycle state with `maintenance.firmware_update.result`.

Normal progress is strictly ordered:

`SENT -> ACCEPTED -> DOWNLOADING -> WRITING -> VERIFYING -> REBOOTING`

The device may reject before acceptance or fail after acceptance. It cannot skip normal stages and cannot self-report `COMPLETED`. The inactive OTA slot may be written while the body is streaming; `VERIFYING` means the complete received image is checked against the manifest size/SHA-256 and image validation before it becomes bootable.

A disconnect before `REBOOTING` becomes `INTERRUPTED`. A disconnect after `REBOOTING` is expected and preserves the update. Completion is recorded only when the same authenticated device/profile reconnects on a newer connection generation and reports exactly the manifest target firmware version. If it returns on the previous version, StageCore records `ROLLBACK_OBSERVED`; any other version is a post-reboot mismatch.

Hub restart interrupts pre-reboot updates but preserves `REBOOTING` handoff so the returning authenticated device can still close the lifecycle.

StageLaser firmware PR #37 implements the device side. The OTA-capable 4 MB dual-slot candidate advertises `device.maintenance.firmware-update`; the default image does not. The device downloads only from its pinned Hub over HTTPS using the active `StageCoreSession`, streams bytes into the inactive OTA slot, verifies exact size + SHA-256 + ESP image, persists the expected update/version/source identity, reports `REBOOTING`, selects the verified boot partition, and relies on the existing rollback boot guard. A pending image is confirmed only when its compiled firmware version and source revision exactly match the persisted qualified manifest after the local safe-boot checkpoint; otherwise it rolls back.

## StageLaser policy

Before StageLaser V1 accepts an OTA update, the current software path requires:

- controller is DISARMED
- logical state is stable known OFF
- no pulse is in progress
- no local Flash is active
- no resync is required
- rollback is required
- the update is not delivered through Cue/show authority

UNKNOWN is never converted to OFF by a blind pulse merely to make an update possible.

## Remaining slice

Only physical qualification remains for StageLaser V1 OTA:

- real-board successful update from one qualified candidate to another
- failed-boot automatic rollback observation
- Hub restart / reconnect closure of `REBOOTING`
- recovery behavior on the qualified physical recovery input
- confirmation that the OTA path remains non-actuating throughout maintenance

Anti-rollback eFuse changes remain out of scope for the first StageLaser update implementation.
