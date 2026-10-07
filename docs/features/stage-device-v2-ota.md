# Stage Device v2 controlled firmware update

Status: manifest contract, Hub-local qualified artifact registry/download, and Operator maintenance manifest issuance are implemented. No device maintenance delivery/result lifecycle or ESP32 OTA write path is implemented yet.

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

The Hub stores device firmware artifacts below its DataRoot in a dedicated immutable registry. Registration is programmatic in this slice; no browser/operator upload endpoint is exposed yet.

Registry acceptance requires:
- UUID artifact ID and device ID
- exact target profile and firmware version
- exact source revision
- `QUALIFIED` status
- expected byte size and lowercase SHA-256 matching the bytes actually written
- immutable artifact IDs

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

## StageLaser policy

Before StageLaser V1 can accept an OTA update, later slices must additionally prove:

- controller is DISARMED
- logical state is stable known OFF
- no pulse is in progress
- no local Flash is active
- no resync is required
- rollback is required
- the update is not delivered through Cue/show authority

UNKNOWN is never converted to OFF by a blind pulse merely to make an update possible.

## Remaining slices

1. Qualified artifact registration/promotion workflow.
2. Stage Device v2 maintenance request/result lifecycle.
3. ESP32-C3 streamed download, SHA-256/size verification and OTA write.
4. Physical update, failed-boot rollback and recovery qualification.

Anti-rollback eFuse changes are out of scope for the first StageLaser update implementation.
