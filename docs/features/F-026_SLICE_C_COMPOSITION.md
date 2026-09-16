# F-026 Slice C2 — Transitions, Crop, Masks, and Simple Effects

Status: implementation and software qualification in progress.

Baseline: verified Slice C1 exact `main` SHA `cacac3745a39b4814da6ec0d4f1def2ec07f677c` (`F-026: add named outputs and projection mapping foundation (#179)`).

Physical GPU/display/projector qualification remains deferred under GitHub Issue #148. Nothing in this slice should be described as physically qualified until the deferred campaign runs.

## Goal

Complete the bounded composition controls deferred from Slice C1 without creating a second visual-state authority or weakening the named-output / projection-mapping model.

The existing F-026 wire contract remains additive `contract_version: 1`.

## Capabilities

Slice C2 adds four canonical visual capabilities:

- `visual.transition`
- `visual.layer.crop`
- `visual.layer.mask`
- `visual.layer.effect`

They use the same Companion capability execution path, Command Envelope semantics, execution-result handling, and `visual.state.inspect` authority as the existing Visual Engine controls.

## Transition contract

`visual.transition` accepts:

- `kind`: `CUT`, `FADE`, or `CROSSFADE`;
- `from_layer_id`;
- `to_layer_id`;
- `duration_ms`;
- explicit `target_opacity` in `[0,1]`.

Rules:

- source and target layers must be distinct;
- both layers must already be preloaded;
- both layers must target the same named output;
- `CUT` requires `duration_ms = 0`;
- `FADE` and `CROSSFADE` require `duration_ms` in `1...30000`;
- malformed, unknown, null, non-finite, or out-of-range values fail closed before visual state mutation.

### Final-state authority

After the renderer accepts a transition, Visual Engine state immediately commits the deterministic final model state:

- source opacity becomes `0`;
- target opacity becomes the explicit `target_opacity`.

Core Animation is presentation only. StageCore does not persist an in-flight transition as a new runtime authority, does not wait on an animation timer before committing final model state, and does not gain replay permission from animation state.

Presentation semantics:

- `CUT`: immediate switch;
- `CROSSFADE`: source and target opacities animate simultaneously;
- `FADE`: bounded fade-through-black, with the source reaching zero before the target rises.

A later Cue may explicitly set opacity or start another transition; that new command is authoritative in the normal execution order.

## Explicit crop rectangle

`visual.layer.crop` accepts a normalized layer-local rectangle:

- `x`, `y`, `width`, `height` are finite values;
- coordinates are normalized to `[0,1]`;
- width and height must be positive;
- the rectangle must remain fully inside the normalized layer bounds.

This explicit crop is separate from the existing media content mode (`FIT`, `FILL`, `CROP`). Content mode still controls how media fills the layer; the explicit rectangle controls which portion of the resulting layer remains visible.

## Masks

`visual.layer.mask` accepts one of:

- `NONE`
- `RECT`
- `ELLIPSE`

The current crop rectangle is the mask domain:

- `NONE` with a full crop removes the layer mask;
- `NONE` with a partial crop retains only the rectangular crop;
- `RECT` uses a rectangular path for the crop domain;
- `ELLIPSE` uses an ellipse inscribed inside the crop domain.

## Simple effects

`visual.layer.effect` supports bounded color controls:

- `brightness`: `-1...1`, neutral `0`;
- `contrast`: `0...4`, neutral `1`;
- `saturation`: `0...2`, neutral `1`.

At least one field must be present in a command. Omitted fields preserve the previous value. The all-neutral state removes the native effect filter.

The macOS native renderer uses `CIColorControls`; no arbitrary shader/plugin execution is introduced by this slice.

## Visual state inspection

`visual.state.inspect` remains the single Visual Engine state authority.

Slice C2 adds a deterministic `composition` array sorted by `layer_id`. Each preloaded layer has exactly one composition state containing:

- current crop rectangle;
- current mask kind;
- current brightness / contrast / saturation values.

Preloading/replacing an existing layer resets its composition state to the default full crop, no mask, and neutral effect.

## Native rendering order

The native macOS path keeps C1 authority ordering:

1. media content mode renders inside the layer;
2. layer crop / mask and simple color effect apply to the layer;
3. layer opacity / transform / z-order / output assignment apply;
4. the named output projection mapping / four-corner transform applies to the output surface.

Slice C2 therefore does not expand or bypass the C1 projector-mapping authority.

## Software acceptance

Slice C2 is software-acceptable only when exact-head and exact-main CI prove:

- Go contract validation recognizes the four capabilities and rejects malformed parameters;
- Visual Engine state commits crop/mask/effect deterministically;
- CUT / FADE / CROSSFADE commit deterministic final opacity state;
- invalid composition commands fail before state mutation;
- replacing a layer resets composition state;
- native macOS rendering accepts crop/mask/effect state;
- native transition presentation leaves the expected final model opacity;
- Core CI passes module lock, tests, vet, race tests, and Linux ARM64 CGo-free product builds;
- Companion Core CI passes Swift build/tests on the supported macOS runner.

## Explicit non-goals

Slice C2 does not implement:

- F-007 LiveSource integration or reconnect/readiness policy (Slice D);
- Digital Twin / no-real-output qualification or fault scenarios (Slice E);
- Operator visual authoring UI, project-native/external-engine choice, or RBAC workflow (Slice F);
- arbitrary masks, arbitrary shaders, shader scripting, blend-mode graphs, or GPU plugin APIs;
- physical GPU/display/projector/capture qualification while Issue #148 is active.
