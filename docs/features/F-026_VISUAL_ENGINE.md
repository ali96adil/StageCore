# F-026 — StageCore Visual Engine

Status: Phase 6 Slice A in progress — software qualification only.

Physical renderer/GPU/display/capture/projector qualification remains deferred under GitHub Issue #148.

## Goal

Provide an optional first-party theatre visual engine under StageCore control without turning the Hub into a renderer or replacing external visual tools such as VDMX, TouchDesigner or QLab.

## Architectural boundary

The Hub owns show authority, immutable Runtime Snapshot state, Cue/Action execution identity, Routing, Preflight, Media/Vault truth and observability.

A suitable Companion/render node owns local rendering and GPU APIs. The Hub never transports decoded frames and does not execute heavy visual processing.

Visual commands use the existing `machine_role` Companion forwarding boundary. They therefore inherit:

- authenticated Companion sessions;
- exact Machine Role assignment;
- immutable Runtime Snapshot matching;
- required-media readiness;
- Execution ID duplicate protection;
- bounded command timeout;
- reconnect behavior that never authorizes command replay.

F-024 SIMULATION remains a separate no-real-output path through the Digital Twin.

## F-026 contract version 1

The initial capability vocabulary is:

- `visual.preload`
- `visual.play`
- `visual.pause`
- `visual.stop`
- `visual.seek`
- `visual.loop`
- `visual.blackout`
- `visual.layer.opacity`
- `visual.layer.transform`
- `visual.state.inspect`

Every command carries `contract_version: 1`. Unknown fields and unsupported contract versions fail closed.

### Managed media identity

`visual.preload` references:

- `layer_id`
- `content_version_id`
- canonical lowercase SHA-256 `content_hash`

Arbitrary Hub or render-node filesystem paths are not part of the command contract. The render node may resolve only content already verified through the existing content-addressed Companion media cache.

### Layer state

Slice A defines deterministic local state for:

- preloaded / playing / paused / stopped;
- non-negative playback position;
- loop enabled/disabled;
- opacity in `[0, 1]`;
- translation `x/y`;
- positive `scale_x/scale_y`;
- rotation in degrees;
- global blackout.

The Slice A executor reports `ACCEPTED`, not verified physical output. It is a contract/state foundation and is intentionally **not registered by the production Companion bootstrap** until Slice B supplies a real renderer. Advertising a playback capability without a renderer would create false readiness.

## Slice A safety rules

- Missing managed media fails before layer state is created.
- Commands for non-preloaded layers fail closed.
- Unsupported parameters fail closed.
- Snapshot/role/readiness/duplicate checks remain owned by the existing `CompanionSession` execution guard.
- No command may contain an arbitrary file path.
- The state foundation has no GPU/display side effects.

## Next slices

Slice B attaches a native macOS renderer and the existing `MediaCacheSynchronizer`, then production Companion capability advertisement may be enabled.

Later slices add layers/transitions/output mapping, F-007 live sources, Preflight/Simulation/Show Capsule integration and Operator workflow. Advanced VJ/compositing/generative features remain outside the initial theatre-reliability scope.
