# F-026 Slice D2b — LiveSource Renderer / Preflight Integration

Status: implementation and CI qualification in progress. Software only.

Physical capture/GPU/display/projector qualification remains deferred under GitHub Issue #148.

## Entry checkpoint

D2b starts from the verified D2a software checkpoint:

- exact `main`: `1f4ddf51db1c3b53cd93aa8310afc589587d0c3c`;
- Core CI #952: module lock, tests, vet, race tests and Linux ARM64 CGo-free product builds PASS;
- Companion Core CI #249: CompanionCore build/tests and real macOS Companion/native-renderer/media acceptance PASS.

D2a owns native capture/network resources and deterministic `video.source.*` control state. D2b completes the production renderer and Preflight integration without changing the F-007 wire vocabulary.

## Native renderer integration

When native Visual Engine support is enabled, the Companion now creates one `NativeVisualRenderer` instance and shares it with both:

- the managed-media `VisualEngine`; and
- the native `LiveSourceEngine` runtime.

Live camera/USB/network frames remain local to the Companion. The Hub sends descriptors and routing intent only; it never carries frame payloads.

A live route attaches a native presentation layer as a child of the existing named-output container. Therefore the existing output surface remains authoritative for:

- output identity;
- blackout visibility;
- output dimensions;
- four-corner projection mapping / keystone transform.

D2b does not create a second projection surface and does not convert live inputs into managed-media objects.

## Layer authority

Visual layer identity is global across named outputs.

- A managed-media layer ID cannot be claimed by a live source.
- A live layer ID cannot be stolen by a different live source.
- Re-routing the same source/layer ID moves that presentation layer to the new named output rather than creating duplicate layer authority.
- Unknown outputs fail closed before control state is committed.
- Closing a source and runtime/renderer shutdown detach live presentation layers.

A successful `video.source.route` is therefore renderer-backed authority, not merely retained intent.

## Diagnostics

`video.source.inspect` remains contract version 1 and receives additive runtime diagnostics when native runtime is active:

- `native_runtime`;
- `native_open`;
- `native_kind` when open;
- `native_renderer_surface`;
- `native_route_count`;
- `native_routes[]` containing `layer_id`, `output_id`, and `renderer_attached`.

These fields distinguish logical source state from actual native renderer attachment without exposing or transporting frame data.

## Hub Preflight integration

D1 already implemented Machine Role LiveSource Preflight authority. D2b wires that authority into the production Hub:

`devicepreflight.WithCompanionAuthority(application.Store)`

Required LiveSources placed on a Companion Machine Role are therefore evaluated against canonical role assignment, Companion trust/readiness/heartbeat and required `video.source.*` capabilities before SHOW authority is granted.

This does not infer execution from connectivity and does not create replay authority.

## Safety invariants

- Reconnect/restart never replays prior `open`, `select`, or `route` commands.
- Runtime state is committed only after native operations succeed.
- Renderer attachment failures do not create false route state.
- Named-output/layer ownership conflicts fail closed.
- Frames never transit the Hub.
- Native LiveSource capabilities remain unavailable when native Visual Engine support is disabled.
- Physical capture/render/display success is not claimed from CI-only qualification.

## D2b acceptance gate

D2b may be marked software complete only when exact-head and exact-main CI prove:

1. native LiveSource presentation layers attach to configured named outputs;
2. output/layer identity collisions fail closed;
3. route failure leaves prior logical route truth intact;
4. a successful re-route moves one global layer identity rather than duplicating it;
5. close/shutdown remove native renderer attachment;
6. `video.source.inspect` reports renderer attachment truth;
7. production Companion bootstrap shares one native renderer across managed media and LiveSource runtime;
8. production Hub Preflight receives Companion Machine Role authority;
9. existing Companion native renderer/media/replacement acceptance remains green;
10. Core CI module lock/tests/vet/race/ARM64 builds remain green.

Only after these gates pass may Slice D be frozen and Phase 6 advance to Slice E simulation/observability/portability work.
