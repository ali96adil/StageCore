# F-026 Slice C1 — Layers, Named Outputs and Projection Mapping

Status: software implementation candidate. Physical renderer/projector qualification remains deferred under GitHub Issue #148.

## Scope

This slice extends the StageCore Visual Engine without moving rendering into the Hub and without turning StageCore into a general VJ/compositing application.

Delivered software contracts:

- deterministic ordered layers through bounded `z_index` values;
- explicit layer-to-output assignment;
- multiple named render outputs with deterministic dimensions;
- normalized four-corner output mapping suitable for keystone/perspective correction;
- native macOS Core Animation output containers and perspective transforms;
- deterministic state inspection for configured outputs, mappings, layer output assignment, and layer order;
- fail-closed validation for unknown outputs, invalid dimensions, unsafe z-order values, malformed/degenerate projection quads, and renderer failures.

## Canonical capabilities

Slice C1 adds:

- `visual.layer.order`
- `visual.layer.output`
- `visual.output.configure`
- `visual.output.mapping`

`visual.preload` also accepts optional `output_id` and `z_index`. The existing `main` output remains the default so Slice A/B commands stay compatible.

## Output model

Every output has:

- `output_id` — stable logical output name;
- `width` / `height` — deterministic logical render dimensions, each bounded to 1...16384;
- `mapping` — four normalized corners in perimeter order.

The identity mapping is:

- top-left `(0, 0)`;
- top-right `(1, 0)`;
- bottom-right `(1, 1)`;
- bottom-left `(0, 1)`.

Projection coordinates are intentionally bounded to `-4...4` so reasonable off-canvas keystone adjustment is possible without accepting unbounded numeric transforms. Degenerate and self-crossing quads fail closed.

## Layer ordering

`z_index` is bounded to `-4096...4096`. State inspection sorts deterministically by:

1. `output_id`;
2. `z_index`;
3. `layer_id`.

The macOS renderer mirrors the logical order through Core Animation `zPosition`.

## Renderer authority

The renderer operation must succeed before Visual Engine state is committed. Therefore:

- a failed output configuration does not create logical output state;
- assignment to an unknown output does not mutate a layer;
- a rejected mapping does not replace the last verified mapping;
- missing/invalid output state never falls through to physical rendering.

## Compatibility and safety

- Hub remains coordinator only; no video/image frames pass through Hub.
- Managed media still comes only from the verified Companion media cache.
- Existing `execution_id` idempotency and Companion session/runtime-snapshot binding remain unchanged.
- Reconnect/restart does not authorize replay of visual commands.
- SIMULATION remains governed by F-024 and must never reach this physical renderer path.
- The existing default `main` output preserves Slice A/B single-output behavior.

## Explicit non-goals of C1

C1 does not yet complete all of Slice C. The following remain for C2:

- CUT / FADE / CROSSFADE transition commands;
- deterministic crop rectangles beyond FIT/FILL/CROP content mode;
- simple masks/effects;
- physical display enumeration/selection and real projector calibration qualification.

The last item is also physically gated by #148 even after software configuration exists.

## Software acceptance

C1 is acceptable only when exact-head and exact-main Core + Companion CI prove:

- Go command validation accepts the canonical output/mapping/order shapes and rejects malformed/unsafe forms;
- Visual Engine commits output/layer state only after renderer success;
- named-output preload, re-ordering, and re-assignment remain deterministic;
- native macOS renderer exposes named-output state, applies four-corner transforms, and fails closed for missing outputs/degenerate mappings;
- existing Slice A/B playback tests continue to pass;
- no Raspberry Pi/GPU/display/projector physical qualification is claimed.

Refs #171
Refs #176
Refs #148
