# F-026 Slice E1 — Visual Digital Twin Qualification

Status: implementation / CI qualification in progress.

Entry checkpoint: Slice D SOFTWARE COMPLETE on exact `main` SHA `744ff9db931a23da82c878dbadb0d1a83b56ea72`.

Physical GPU/display/projector/capture qualification remains deferred under GitHub Issue #148.

## Goal

Exercise F-026 Visual Engine commands through the existing F-024 SIMULATION execution boundary without granting any physical renderer, Companion, device, OSC, HTTP, or script transport authority.

Slice E1 deliberately reuses the canonical Digital Twin rather than inventing a second visual-only simulator.

## Contract parity

The generic simulator fallback now recognizes every capability owned by `internal/visualengine` and validates its parameters with `visualengine.ValidateCommand` before returning simulated completion.

This means a Visual Engine command that would be rejected by the real Visual Engine wire contract is also rejected in SIMULATION with `SIM_VISUAL_COMMAND_INVALID`.

Visual simulation does not decode media, render frames, open displays, attach capture devices, or contact a Companion. Valid visual commands are consumed locally by the Digital Twin and recorded as simulation-only execution truth.

## Authority boundary

`simulator.SessionExecutor` remains the sole F-024 mode boundary:

- `SIMULATION` -> Digital Twin only;
- `REHEARSAL` / `SHOW` -> physical executor.

E1 does not alter this routing rule.

Acceptance proves that a Visual Engine command targeted at a Machine Role during SIMULATION:

- completes through the Digital Twin;
- never invokes the physical executor;
- updates Digital Twin target execution truth;
- records canonical `simulation.execution.*` Flight Recorder evidence under the authoritative Project / Session / Runtime Snapshot.

Invalid visual commands also fail inside the Digital Twin and never reach physical execution.

## Visual fault qualification

E1 uses the existing F-024 deterministic fault engine with capability/target selectors to qualify Visual Engine failure behavior for:

- renderer unavailable -> `VISUAL_RENDERER_UNAVAILABLE`;
- source unavailable -> `VISUAL_SOURCE_UNAVAILABLE`;
- managed media missing -> `VISUAL_MEDIA_MISSING`;
- renderer timeout -> canonical timeout result.

These scenarios remain simulation-only. They do not alter device trust, Companion readiness, Machine Role assignment, real media state, or renderer state.

## Explicit non-goals

E1 does not:

- emulate AVFoundation/AppKit frame rendering in Go;
- claim physical renderer acknowledgement;
- synthesize a second Visual Engine runtime-state model;
- move media frames through the Hub;
- change LiveSource reconnect/replay semantics;
- change Runtime Snapshot or Show Capsule schemas;
- complete Slice E portability qualification by itself;
- perform Raspberry Pi or physical display/capture qualification.

## Next Slice E work

After E1 exact-head and exact-main Core CI pass, remaining Slice E work is:

1. prove F-026 visual command/configuration and immutable managed-media requirements survive Runtime Snapshot -> Show Capsule portability without schema drift;
2. verify required portable media content inclusion/reference policy through existing Show Capsule authority;
3. close Slice E with cumulative observability/fault/portability acceptance evidence.

Refs #171
Refs #176
Refs #148
