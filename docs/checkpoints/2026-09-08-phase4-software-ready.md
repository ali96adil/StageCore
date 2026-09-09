# Phase 4 Software-Ready Checkpoint

**Created:** 2026-09-08  
**Updated:** 2026-09-09  
**Campaign:** Issue #138 / PR #139  
**Final software code checkpoint:** `7f2d84172e96b6579dfaa218237bf5586a80533c`  
**Core CI:** run #746 — PASS

## Status

Phase 4 promoted software scope is **SOFTWARE READY / PHYSICAL QUALIFICATION PENDING**.

This checkpoint is not a Phase 4 completion claim. No Phase 4 feature that requires real device/product evidence is COMPLETE until the cumulative Raspberry Pi / Stage LAN qualification in Issue #138 passes.

The StageCore Feature Backlog intentionally keeps F-003, F-006, F-007 and F-022 unchecked until that real acceptance path passes. `docs/FEATURE_STATUS.md` is the operational ledger that distinguishes SOFTWARE READY from COMPLETE.

## Implemented software scope

### F-003 — Android Tablet Player integration

- Versioned Stage Device protocol: `stagecore.device/1`.
- Stable Stage Device identity, project/profile metadata, capabilities, connection/readiness and last-seen state.
- Secure authenticated runtime WebSocket on the proven pairing/runtime-session authority.
- Runtime-session revocation terminates an otherwise idle Stage Device socket.
- Reconnect does not replay pending/expired commands.
- Typed tablet commands use canonical `contracts.CommandEnvelope` identity/correlation/expiry/idempotency semantics.
- Guided Operator Stage Devices workspace supports Select Media / Prepare / Play / Pause / Stop / Blackout without requiring raw OSC or manual IP management.
- Broad Tablet Control supports All / Group / Location / Device targeting through the server-side Stage Device batch endpoint rather than UI-side command loops.
- Per-device batch results remain individually attributable under one correlation ID.
- Bilingual Arabic/English and RTL-safe Phase 4 Operator assets are embedded in the Hub binary.

The external Android APK remains the tablet-side playback/render client. If its source is outside this repository delivery boundary, this checkpoint qualifies the StageCore protocol/server/simulator boundary only; real APK behavior remains part of physical/product qualification.

### F-006 — Stage Display / Actor Callboard

- Shared F-003 Stage Device identity and runtime channel; no second display registry or protocol.
- Typed message/countdown/alert/clear/blackout/chime capability vocabulary.
- Guided Callboard targeting supports All / Group / Location / Device.
- Message presets cover common operator intents while retaining editable text.
- Countdown input supports a bounded relative duration and optional absolute target time; the Hub canonicalizes countdowns to absolute `target_at` semantics.
- Alert payloads are semantic rather than renderer-specific: role/category, intensity percentage, steady/pulse/flash motion, bounded duration and optional chime identifier.
- Invalid alert/countdown payloads fail closed; optional chime behavior remains capability-aware.
- Broad sends require target confirmation where appropriate and return per-device results.
- Safe runtime controls remain separate from structural configuration and GO authority.
- Cue/timeline actions can target `logical_type: stage_device` through the existing Cue Engine capability registry.
- Cue-derived Stage Device execution uses deterministic action identity and bounded timeout semantics.
- Stage Device accepted/terminal command state is recorded in canonical `event_records` alongside cue/action events.
- End-to-end deterministic coverage proves `cue.go -> action -> capability registry -> stage_device target -> typed command -> device result -> action/cue completion`.

### F-007 — Live Video / Camera Inputs

- Generic live-source model for `LOCAL_CAMERA`, `USB_CAPTURE`, and `NETWORK_STREAM` source classes.
- Hub owns source configuration/readiness/routing intent; heavy video frames/capture/rendering remain outside the Hub critical path.
- Typed video-source open/close/select/route/inspect commands use the Stage Device command/runtime boundary.
- Required source / render-node readiness participates in SHOW Preflight using the canonical DeviceExperience repository.
- Guided bilingual Operator Live Video workspace provides source status plus open/close/test controls.

Real source capture/render evidence remains pending cumulative Phase 4 qualification; unavailable real source classes must be recorded N/A rather than falsely claimed.

### F-022 — Stage Network Cockpit

- Canonical bounded network observations cover Stage Devices, Companions, live-source/endpoints and measured transport signals.
- READY/WARNING/ADVISORY/BLOCKER classification preserves null/not-measured latency semantics.
- Deterministic evidence can classify stale observations, unreachable targets, revoked identities, identity conflicts and elevated measured latency/jitter without fabricating unmeasured values.
- Stale-state and bounded-retention behavior are deterministic.
- Network/device readiness participates in Preflight without creating a second network truth source.
- Guided bilingual/RTL Network Cockpit workspace is present.

### Cross-cutting Phase 4 operator work

- Phase 4 assets use the established localization ownership contract and Arabic dictionary entries.
- Phase 4 UI uses semantic appearance/status tokens and participates in the existing Operator shell rather than creating a separate app.
- Normal device/callboard/video/network workflows do not require raw JSON/OSC/IP editing.
- Phase 4 runtime controls remain permission-gated: OPERATOR may execute authorized runtime Stage Device commands; VIEWER is denied; structural configuration remains separately authorized and obeys F-012 SHOW mutation lock.

### Issue #137 — Operator Draft discard/revert UX

Software acceptance is implemented:

- OWNER-only recovery action.
- F-012 SHOW structural-mutation lock enforced server-side.
- UI confirmation identifies the Draft and validated parent.
- Discard restores the validated parent as current and marks the abandoned Draft `SUPERSEDED` rather than deleting history.
- Immutable published Runtime Snapshot content remains unchanged.
- Success/rejection/failure audit records use `security_audit_records`.
- A new Draft can be created cleanly after discard.
- Parentless initial Draft fails safely because no validated parent exists to restore.

Issue #137 may be closed after this checkpoint is merged to `main` and exact-head CI remains green.

## Canonical Stage Device execution and observability

Phase 4 does not create a parallel cue engine or event log.

The immutable Runtime Snapshot resolves a normal project alias whose `logical_type` is `stage_device`. The existing `capability.Registry` dispatches that target to the Stage Device forwarder. The forwarder maps the capability key to a typed Stage Device command, preserves project/session/runtime-snapshot/correlation/causation identity, and uses the action execution ID as the deterministic idempotency boundary.

Command lifecycle is recorded in canonical `event_records`:

- `stage_device.command.accepted`
- `stage_device.command.completed`
- `stage_device.command.failed`
- `stage_device.command.timed_out`
- `stage_device.command.cancelled`
- `stage_device.command.rejected`

Duplicate terminal completion does not emit a second terminal event. Cue/action history therefore remains one canonical Flight Recorder trace.

## Timeout / reconnect safety

- Cue-derived Stage Device commands have one deadline governing both Command Envelope expiry and Hub wait semantics.
- Explicit action timeout wins when shorter than a parent context deadline.
- With no explicit timeout/deadline, the Stage Device execution boundary is bounded by a 5-second default rather than waiting indefinitely.
- Deadline/ticker races classify deterministically as timeout/cancel instead of leaking a database lookup error.
- Reconnect does not re-send a pending command merely because the device came back online.
- Revocation terminates the secure runtime socket even when the client is otherwise idle.
- A Go 1.27 scheduler-ordering failure exposed a test-only assumption between terminal command visibility and asynchronous Flight Recorder append. The acceptance test now synchronizes on the canonical event count; production runtime behavior was not changed.

## Final software CI evidence

Core CI #746 on `7f2d84172e96b6579dfaa218237bf5586a80533c` passed:

- Go 1.26 module lock
- Go 1.26 `go test ./...`
- Go 1.26 `go vet ./...`
- Go 1.26 race tests
- Linux ARM64 CGo-free product builds
- Go 1.27 module lock
- Go 1.27 tests
- Go 1.27 vet

Acceptance coverage on the final software code checkpoint includes:

- Location-based server-side Stage Device batch expansion with per-device results;
- broad Tablet All / Group / Location / Device UI targeting;
- Callboard presets, absolute countdown target semantics, semantic alerts and optional chime identifiers;
- OPERATOR runtime authorization, VIEWER denial and structural SHOW-lock enforcement;
- Phase 4 Operator asset assertions for the promoted broad targeting/control surfaces.

Earlier CI #740 exposed only the test synchronization race described above; the narrow test-only fix then passed on both supported Go versions. Earlier CI #714 intentionally exposed an incomplete E2E expected event list because F-028 correctly emits `cue.timing_observed` before `cue.started`; that expectation was corrected without changing established timing behavior.

## Physical qualification still required

No intermediate Phase 4 slice has been deployed to the Raspberry Pi.

After PR #139 is merged and one exact `main` SHA is frozen, Issue #138 requires one cumulative campaign covering:

- supported exact candidate deployment to the Raspberry Pi;
- installed revision/hash/database/service/Doctor/readiness checks;
- real Android tablet registration/reconnect and local-media controls when a compatible client is available;
- real Callboard message/countdown/visual-alert behavior and cue-driven invocation; optional chime only when the real client advertises/supports it;
- at least one available real live-video source class;
- Network Cockpit disconnect/reconnect truthfulness;
- regression of discovery, SHOW lock, profiles, extension trust, timing, capsule metadata, F-025 external execution, persistence/security/history, and representative REHEARSAL/SHOW operation.

Only after that gate passes may fully qualified Phase 4 feature rows move to COMPLETE and the Phase 4 tracker close PASS.
