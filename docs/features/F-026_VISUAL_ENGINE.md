# F-026 — StageCore Visual Engine

Status: Phase 6 Slice A COMPLETE; Slice B native macOS playback foundation in progress — software qualification only.

Slice A software freeze: `7e8d224ad96fb9d7669d26e07965db4a8659c77a` (PR #177), with exact-main Core CI #893 and Companion Core CI #192 PASS.

Physical renderer/GPU/display/capture/projector qualification remains deferred under GitHub Issue #148.

## Goal

Provide an optional first-party theatre visual engine under StageCore control without turning the Hub into a renderer or replacing external visual tools such as VDMX, TouchDesigner or QLab.

## Architectural boundary

The Hub owns show authority, immutable Runtime Snapshot state, Cue/Action execution identity, Routing, Preflight, Media/Vault truth and observability.

A suitable Companion/render node owns local rendering and native media/GPU APIs. The Hub never transports decoded frames and does not execute heavy visual processing.

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
- optional `content_mode`: `FIT`, `FILL` or `CROP` (default `FIT`)

Arbitrary Hub or render-node filesystem paths are not part of the command contract. The render node resolves only content already present in the existing content-addressed Companion media cache, and re-verifies the object SHA-256 before returning it to the renderer.

Content-mode semantics in Slice B are deterministic:

- `FIT`: preserve aspect ratio and letterbox/pillarbox as needed;
- `FILL`: stretch to the complete render bounds;
- `CROP`: preserve aspect ratio and crop overflow to fill the bounds.

### Layer state

The local state authority covers:

- preloaded / playing / paused / stopped;
- non-negative playback position;
- loop enabled/disabled;
- opacity in `[0, 1]`;
- translation `x/y`;
- positive `scale_x/scale_y`;
- rotation in degrees;
- content mode;
- global blackout.

State is committed only after the backing renderer operation succeeds. Renderer failures therefore cannot leave the deterministic Visual Engine state claiming an operation that did not complete locally.

## Slice B native macOS renderer

Slice B adds a macOS-only `NativeVisualRenderer` behind the generic `VisualRenderer` boundary:

- `NSImage` / Core Animation layers for still images;
- `AVURLAsset`, `AVPlayer` and `AVPlayerLayer` for video;
- preload, play, pause, stop and seek;
- explicit video loop state;
- blackout, opacity and basic transform updates;
- `FIT` / `FILL` / `CROP` geometry;
- deterministic native-renderer errors;
- renderer shutdown on Companion runtime termination/failure.

The renderer owns a deterministic local render surface, but Slice B intentionally does **not** create projector/display windows or claim a named physical output. Window lifecycle, multiple named outputs and projector mapping belong to Slice C.

### Optional production registration

Native visual capabilities are advertised by `CompanionBootstrap` only when `nativeVisualEngineEnabled == true` in Companion configuration. Missing/legacy configuration remains disabled by default.

When enabled on macOS, bootstrap constructs one existing `MediaCacheSynchronizer`, reuses it as both the normal media synchronizer and the Visual Engine media resolver, constructs the native renderer, and registers the canonical `visual.*` executors through the existing Companion capability path.

A Companion that has not explicitly enabled the native Visual Engine does not advertise visual playback capability.

## Safety rules

- Missing or hash-invalid managed media fails before layer state is created.
- Native renderer failure leaves deterministic Visual Engine state unchanged for that command.
- Commands for non-preloaded layers fail closed.
- Unsupported parameters and content modes fail closed.
- Snapshot/role/readiness/duplicate checks remain owned by the existing `CompanionSession` execution guard.
- No command may contain an arbitrary file path.
- Reconnect/restart never implies visual command replay.
- Renderer shutdown is local to the Companion and does not grant recovery/replay authority.

## Software qualification

Slice B CI includes:

- Linux Swift package build/tests for platform-independent Visual Engine contracts;
- macOS package/executable build;
- macOS-native renderer acceptance using generated local image/video fixtures;
- existing >=2 GiB interrupted media-resume acceptance;
- existing real macOS Companion replacement acceptance;
- Core CI contract/race/ARM64 regression gates.

These are software tests only. They do not qualify a GPU, display, projector, capture interface or physical output path.

## Next slices

Slice C adds ordered layers, transitions, output-window/multi-display management, richer transform/crop/masks and projector mapping/four-corner perspective.

Later slices add F-007 live sources, Preflight/readiness, Digital Twin/Flight Recorder/Show Capsule integration and the bilingual Operator workflow. Advanced VJ/compositing/generative features remain outside the initial theatre-reliability scope.
