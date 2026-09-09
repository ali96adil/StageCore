# F-003 — Android Tablet Player Integration

## Status

Phase 4 implementation specification. This document defines the StageCore-owned integration boundary; it does not replace the existing Android theatre-player application.

## Product goal

Make Android theatre tablets native StageCore devices without forcing an operator to manage IP addresses or raw OSC. The Android APK remains the tablet-side playback client. StageCore owns identity, secure registration, capability negotiation, command identity/expiry, readiness, grouping, observability and operator workflow.

OSC remains an optional compatibility/fallback surface for older client builds.

## Existing client contract to preserve

The external theatre-player client used before F-003 has these known operational expectations:

- local/offline video files on the tablet;
- device identity suitable for per-tablet targeting;
- scene/media selection;
- prepare/preload followed by explicit live playback;
- play, pause, stop and blackout behavior;
- per-device orientation where supported;
- brightness and touch-lock controls where supported;
- online/offline indication;
- an existing OSC control surface that may remain available as a compatibility adapter.

The StageCore protocol must not encode the client’s current storage path, Android UI structure or implementation language as Core assumptions. File naming and storage details are client capabilities/configuration, not Hub architecture.

## Roles and authority

### Hub

Owns:

- stable Stage Device identity and operator-readable name;
- secure pairing/registration authority;
- project/device bindings and groups;
- command envelopes and expiry;
- desired command intent;
- canonical result/history records;
- health/readiness classification;
- Show Mode policy.

The Hub does not decode or render tablet video.

### Android client

Owns:

- local media discovery/opening;
- decode/render/playback;
- device-local orientation/brightness/touch-lock execution;
- truthful capability advertisement;
- acknowledgements/results for commands it actually executes.

The client may continue standalone/offline playback when StageCore is unavailable. Reconnecting to StageCore must never replay expired transient commands.

## Secure identity

F-003 reuses the existing StageCore secure device pairing/session authority. The current persistence/API terminology uses `Companion` for this identity boundary; Android tablets may use that proven trust path while the Stage Device layer presents a product-neutral device identity to operators. A future database terminology migration must not change the wire identity contract.

Requirements:

- TLS or loopback-only bootstrap rules remain fail-closed;
- first pairing requires explicit authorized approval;
- remembered identity survives reconnect;
- revocation invalidates the runtime session;
- device runtime identity must match the authenticated identity;
- a reconnect never grants a second logical identity silently.

## Protocol versions

Initial protocol: `stagecore.device/1`.

A runtime hello advertises:

- `device_id`;
- display name;
- platform / architecture;
- client version;
- protocol version;
- device roles (`TABLET_PLAYER`, optionally `STAGE_DISPLAY`, `RENDER_NODE`);
- capability keys;
- readiness;
- optional current media/display state.

Unknown protocol major versions fail closed. Unknown capabilities are retained as opaque advertised capabilities but do not become executable until StageCore understands them.

## Capability keys

Initial F-003 capability vocabulary:

- `tablet.media.prepare`
- `tablet.media.play`
- `tablet.media.pause`
- `tablet.media.stop`
- `tablet.media.blackout`
- `tablet.media.select`
- `tablet.orientation.set`
- `tablet.brightness.set`
- `tablet.touch_lock.set`

A client must not advertise a capability it cannot execute truthfully.

## Commands

Minimum typed commands:

- `TABLET_PREPARE`
- `TABLET_PLAY`
- `TABLET_PAUSE`
- `TABLET_STOP`
- `TABLET_BLACKOUT`
- `TABLET_SELECT_MEDIA`

Optional commands are permitted only when capability negotiation confirms support.

Each meaningful command uses the canonical `contracts.CommandEnvelope` and therefore carries stable command identity, schema version, issue time, optional deadline, project/runtime identity where applicable, issuer, correlation/causation IDs, priority, idempotency key and payload.

### Expiry

- `PLAY`, transient display alerts and cue-derived actions should normally have explicit deadlines.
- A client must reject an already-expired command.
- A Hub reconnect must not re-send an expired command simply because the device returned online.

### Duplicate delivery

`command_id` is the primary execution identity. Repeated delivery of the same command ID returns the already-known terminal result or remains idempotently pending; it must not execute twice.

## Media addressing

StageCore sends a logical media selection descriptor, not an Android filesystem path. Initial payload may include:

- logical media key / scene key;
- expected file name when the show convention requires one;
- optional content hash/size when StageCore has a Vault/media manifest reference;
- optional start position.

The client resolves the descriptor against its local media library and reports `READY`, `MISSING`, `INCOMPATIBLE` or another explicit result.

## Groups

Device grouping is StageCore-owned metadata. A group send expands to individual device commands with individual command IDs/results under one correlation ID. One failed tablet must not hide successful/failed state of the others.

## SHOW behavior

Runtime tablet controls are operator actions and may remain available in SHOW for users with `runtime.control`. Structural changes—pairing policy, profile materialization, group definitions or protected configuration—follow F-012 Show Configuration Lock rules.

## Operator UX

Normal workflow:

1. Device is discovered/pairs securely.
2. Operator sees a human-readable tablet card.
3. StageCore shows online state, capabilities, media/readiness and group membership.
4. Operator selects a device/group and chooses Prepare/Play/Pause/Stop/Blackout or a media scene.
5. The UI shows per-device acknowledgement/result truthfully.

Raw IP/OSC JSON is not required for the normal path.

All new UI strings require Arabic (`ar-IQ`) and English variants, RTL-safe layout and semantic status tokens.

## Offline compatibility

The APK must remain capable of local playback without a Hub. StageCore integration is additive. Legacy OSC may remain enabled by an explicit compatibility setting/profile; it is not the default StageCore operator workflow.

## Acceptance

Software acceptance requires deterministic coverage for:

- protocol/version validation;
- capability truthfulness;
- registration/reconnect/revocation;
- command expiry and duplicate delivery;
- prepare/play/pause/stop/blackout/select-media result mapping;
- authorization and SHOW policy;
- grouping/per-device results;
- bilingual/RTL operator contract.

Physical acceptance is deferred to the cumulative Phase 4 gate in Issue #138 and must use at least one real Android tablet. If the external APK source is not in the StageCore repository, StageCore may complete and CI-qualify its server/client protocol and simulator first; real APK integration remains part of the physical/product gate and must never be falsely claimed from a simulator.