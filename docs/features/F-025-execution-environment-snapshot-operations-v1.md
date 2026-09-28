# F-025 — Execution Environment Snapshot & Adapter Operations v1

**Status:** Phase 3 bounded foundation  
**Feature ID:** F-025  
**Builds on:** Execution Environment Manifest v1, authenticated Companion runtime authority, Machine Roles, immutable Runtime Snapshots, StageCore Vault, F-012 SHOW configuration lock

## Goal

This slice extends the existing F-025 Execution Environment Manifest with two engine-neutral capabilities:

1. a truthful, canonical **Execution Environment Snapshot** that records reconstruction aids without claiming unsupported third-party project portability; and
2. bounded, typed **adapter operations** for `OPEN`, `RECONNECT`, `CAPTURE_SNAPSHOT`, and `RESTORE_OBSERVABLE_STATE` over the existing authenticated Companion execution channel.

It does not add arbitrary command or shell execution and does not add application-specific state to Core.

## Adapter operation authority

The single Companion capability is:

`execution.environment.operation`

An operation is accepted only when StageCore can resolve all of the following existing authority:

- persisted Execution Environment Manifest;
- manifest-bound Machine Role in the same Project;
- active trusted Companion assignment;
- Machine Role required Runtime Snapshot;
- Companion `READY` on that exact Runtime Snapshot;
- required Companion configuration hash when configured.

The Hub sends the operation through the existing authenticated `execution.request` / `execution.result` WebSocket flow. No second transport or parallel Machine Role model is introduced.

### Typed operations

v1 supports only:

- `OPEN`
- `RECONNECT`
- `CAPTURE_SNAPSHOT`
- `RESTORE_OBSERVABLE_STATE`

The operation payload carries the canonical manifest, its `adapter_key`, and the source manifest SHA-256. Adapter providers receive typed input; Core never sends an arbitrary shell command.

For `RESTORE_OBSERVABLE_STATE`, the browser/operator request still supplies only the operation kind and identity. The Hub selects the **latest validated snapshot for that exact Execution Environment Manifest**, binds that snapshot content identity into operation idempotency, and forwards its canonical snapshot through the authenticated Companion channel. The operator cannot inject an arbitrary snapshot or OSC payload. Inline restore parameters are bounded below the Companion runtime message ceiling; oversized state fails explicitly rather than being truncated.

Adapters are explicitly registered. Missing adapters or unsupported operations return truthful `UNSUPPORTED` results. There is no generic fallback that fabricates success.

## Operation identity and idempotency

`operation_id` is stable execution identity.

StageCore binds it to:

- environment manifest identity;
- operation kind;
- manifest content hash;
- Machine Role;
- required Runtime Snapshot;
- assigned Companion.

Repeating the same operation identity reuses the existing Companion execution result and does not replay the external operation. Reusing the same `operation_id` for different authority or parameters is rejected with `ENVIRONMENT_OPERATION_ID_CONFLICT`.

Disconnects, timeouts, cancellation, authentication loss, Runtime Snapshot mismatch, and unsupported adapters preserve the existing Companion terminal-result semantics. They are never converted into false success.

## Execution Environment Snapshot v1

A snapshot is bound to one canonical source manifest by:

- `environment_key`;
- `adapter_key`;
- `source_manifest_sha256`.

It declares one overall capture state:

- `COMPLETE`
- `PARTIAL`
- `UNSUPPORTED`

`PARTIAL` and `UNSUPPORTED` are first-class truthful outcomes. A snapshot must not imply that StageCore can reproduce a full external application project when the application does not expose that state through legitimate supported surfaces.

### Snapshot items

Each item has stable identity plus:

- item kind;
- provenance;
- capture status;
- portability class;
- optional locator or content identity according to portability;
- bounded operator/adapter notes.

Supported item kinds cover generic reconstruction aids such as project exports, templates, presets, configuration, resources, control namespaces/state, external-extension inventory, output notes, and operator reference material.

### Provenance

v1 provenance includes legitimate sources such as:

- application export;
- filesystem resource;
- adapter observation;
- OSCQuery;
- operator reference.

Provenance records where the information came from; it does not grant new authority to read or execute that source.

## Portability truth

Snapshot items use exactly one portability class.

### `CONTENT_BOUND`

The bytes are captured and identified by SHA-256 plus exact size. Persistence verifies the same hash and size already exist in the existing StageCore `vault_objects` store before accepting the snapshot.

F-025 does not create another blob store.

### `REFERENCE_ONLY`

The snapshot records a locator but must not claim content hash or captured bytes. This is useful for licensed, machine-owned, or otherwise non-portable resources.

### `DESCRIPTIVE_ONLY`

The item records reconstruction guidance or observed state only. It may not claim bytes or a locator.

## Canonical identity

Snapshot normalization:

1. validates schema and bounded fields;
2. normalizes SHA-256 case;
3. validates truthful capture/portability combinations;
4. sorts items by stable key;
5. encodes through StageCore canonical JSON;
6. hashes canonical bytes with SHA-256.

Stored snapshot JSON is identity-bearing. Reads fail closed if canonical bytes, redundant source identity, or stored content hash are tampered with.

## Persistence and revision safety

Snapshots are revision configuration and belong to an existing Execution Environment Manifest.

Persistence enforces:

- source manifest/revision consistency;
- deterministic content identity;
- duplicate-content conflict handling;
- DRAFT-only create/delete mutation;
- F-012 project-wide SHOW-lock protection, including database triggers;
- read access during SHOW;
- revision-fork lineage.

When a validated revision is forked into a successor DRAFT, environment manifests and their snapshots are cloned to the new environment records with preserved canonical content identity and new audit creation metadata.

## Observable-state restore

Observable restore is deliberately narrower than project/workspace restore.

The current VDMX adapter may restore only OSCQuery-published controls that pass a fresh live safety comparison. Before the first write it:

1. validates snapshot Environment/Adapter/source-manifest identity;
2. extracts exactly one observed OSCQuery namespace from the Hub-selected snapshot;
3. fetches the current loopback OSCQuery namespace and current `HOST_INFO`;
4. requires the live OSC UDP port from `HOST_INFO`;
5. classifies captured vs live state by exact OSC path, type, write access, value shape, and current range;
6. permits only continuous floating-point (`f`/`d`) state by default;
7. treats integer, boolean, string, button, trigger, and event-like controls as unsafe and does not replay them;
8. performs a second identical live read immediately before the first OSC write, aborting on any race;
9. sends only bounded differing stateful controls; and
10. re-reads OSCQuery after transport send and requires every written path to report the captured value before returning success.

A transport-level UDP send is never called application-state proof. Missing/incompatible/read-only captured state fails before any write. A post-write verification failure requires fresh inspection before retry. The operation has no rollback claim and is therefore operator-confirmed rather than an automatic reconnect action.

This does **not** restore unpublished VDMX layers, FX graphs, media-bin internals, plugins, licenses, or unsaved project structure. It does not replay StageCore GO/button controls.

## Assisted rebuild plan

Manifest + latest Snapshot can produce deterministic reconstruction guidance containing:

- declared portable content;
- reference-only dependencies;
- manual installation or binding work;
- missing/unsupported capture surfaces;
- declared launch/open target.

The rebuild plan is guidance, not destination readiness. It always states that a fresh execution-environment inspection is required before StageCore may treat the destination as ready.

No installer execution or license bypass is implemented by this plan. The separate observable-state restore operation above is limited to verified OSCQuery-published state and does not turn rebuild guidance into a complete project restore.

## Companion implementation boundary

The macOS Companion provides an engine-neutral operation executor with an explicit adapter-provider registry.

The framework itself is advertised as `execution.environment.operation`. Application adapters must be registered separately and may use only legitimate supported application surfaces.

The generic framework deliberately returns `ENVIRONMENT_ADAPTER_UNSUPPORTED` when no provider exists. Actual VDMX/QLab/Ableton/TouchDesigner behavior remains adapter work, not Core special-casing.

## Verification

This slice is covered by:

- canonical snapshot normalization/hash tests;
- snapshot portability and truthfulness tests;
- Store persistence, Vault verification, tamper, DRAFT and SHOW-lock tests;
- revision-fork snapshot lineage tests;
- deterministic rebuild-plan tests;
- Swift adapter-provider routing and unsupported-result tests;
- authenticated Companion WebSocket Execution Environment operation E2E tests covering successful typed snapshot capture, cached duplicate identity, conflicting `operation_id`, result identity mismatch, and unsupported adapter behavior;
- existing Companion channel disconnect/timeout/revocation and execution replay tests on the same runtime authority;
- Core Test/Vet/Race and Linux ARM64 CGo-free build gates;
- CompanionCore and real macOS Companion replacement/media acceptance gates.

## F-019 relationship

F-019 Show Capsule may consume the canonical F-025 Manifest and Snapshot identities later.

A Capsule may carry or resolve `CONTENT_BOUND` Vault objects according to later package policy. `REFERENCE_ONLY` and `DESCRIPTIVE_ONLY` items must remain explicit limitations and must never be presented as embedded backup data.

This slice does **not** implement Capsule export, import, restore, signing, or transport.

## Deferred

- real VDMX operation/capture provider;
- real QLab/Ableton/TouchDesigner providers;
- complete destination project/workspace restore and application-install workflows;
- license or entitlement handling beyond truthful manual requirements;
- F-019 Show Capsule packaging/restore;
- automatic readiness declaration without a fresh inspection.
