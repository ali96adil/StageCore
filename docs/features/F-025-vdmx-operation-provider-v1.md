# F-025 — VDMX Operation Provider v1

**Status:** Phase 3 application-adapter slice  
**Feature ID:** F-025  
**Depends on:** Execution Environment Manifest v1, authenticated Companion runtime authority, typed execution-environment operations, Machine Roles, Runtime Snapshots, Operator RBAC

## Goal

Provide the first real application-specific execution-environment operation provider without moving VDMX-specific state into StageCore Core.

The provider translates the existing engine-neutral F-025 Manifest into bounded, legitimate macOS actions. Core continues to know only the generic adapter identity, launch contract, operation kind, operation identity, and generic snapshot document.

## Supported operations

### `OPEN`

`stagecore.adapter.vdmx` supports `OPEN` when all of the following are true:

- the persisted Manifest identifies `stagecore.adapter.vdmx` and application key `vdmx`;
- the assigned Companion is trusted and READY on the required Runtime Snapshot under the existing F-025 runtime authority;
- a VDMX application bundle is found at a safe known candidate location;
- the Manifest declares a launch target as either:
  - `ASSET`, resolving to the referenced asset locator; or
  - `LOCATOR`, resolving directly to the launch locator;
- the locator is a local absolute path or `file://` URL;
- the target exists and does not resolve through a symlink to a different canonical path.

The Companion asks macOS to open the declared target specifically with the discovered VDMX application through the supported `NSWorkspace` application-opening surface.

The browser never chooses an executable, capability, shell command, or arbitrary operation parameters.

Explicit failures include:

- `VDMX_MANIFEST_INVALID`
- `VDMX_APPLICATION_NOT_FOUND`
- `VDMX_LAUNCH_TARGET_UNAVAILABLE`
- `VDMX_OPEN_FAILED`

## `RECONNECT`

VDMX `RECONNECT` is intentionally **unsupported** in v1.

StageCore does not simulate reconnect success and does not introduce AppleScript, JXA, private APIs, synthetic UI events, or arbitrary shell execution to approximate it. A later provider version may add reconnect only after a legitimate supported VDMX control surface is identified and bounded.

## `CAPTURE_SNAPSHOT`

VDMX supports `CAPTURE_SNAPSHOT` as a truthful generic **PARTIAL** Execution Environment Snapshot.

v1 records only facts the provider can safely establish without claiming complete VDMX project-state export:

- whether the VDMX application bundle is safely present;
- whether the declared launch target is safely present in place;
- the launch target as `REFERENCE_ONLY` metadata when inspectable;
- an explicit `UNSUPPORTED` item for complete VDMX internal workspace / plug-in / FX / published-control state capture.

The snapshot is bound exactly to:

- `environment_key`;
- `adapter_key`;
- source Manifest SHA-256.

The provider never claims new `CONTENT_BOUND` bytes. Existing StageCore Vault capture remains the authority for managed content bytes.

A `PARTIAL` snapshot is reconstruction guidance, not a claim that StageCore can reproduce every internal VDMX state bit.

## Operator runtime endpoint

The existing Execution Environments UI extends its current environment card with bounded runtime controls.

Endpoint:

`POST /api/v1/projects/{project_id}/revisions/{revision_id}/execution-environments/{execution_environment_id}/operations`

Authorization:

- existing `PermissionRuntimeControl`;
- normal browser session, same-origin and CSRF requirements remain unchanged.

Accepted request fields are only:

```json
{
  "operation_id": "stable-operation-id",
  "kind": "OPEN",
  "timeout_ms": 10000
}
```

Allowed kinds remain the generic F-025 enum. The VDMX UI exposes only `OPEN` and `CAPTURE_SNAPSHOT` in this slice.

The endpoint rejects unknown JSON fields. Therefore a browser cannot inject:

- `capability`;
- `parameters`;
- `command`;
- Companion identity;
- Machine Role identity;
- Runtime Snapshot identity;
- arbitrary executable/application paths.

The Hub resolves those authorities from persisted StageCore state and dispatches only through `RuntimeChannel.OperateExecutionEnvironment`.

## SHOW and persistence semantics

`OPEN` and the read-only metadata collection performed by `CAPTURE_SNAPSHOT` are runtime operations and do not mutate the persisted Manifest.

This slice does **not** automatically persist the returned snapshot. Any future persisted snapshot mutation remains subject to existing revision DRAFT rules and the F-012 SHOW configuration lock.

## Security boundaries

v1 explicitly does not add:

- shell/command execution;
- AppleScript/JXA automation;
- private or undocumented VDMX APIs;
- browser-selected executable paths;
- VDMX-specific top-level Core schema;
- a second Companion transport;
- a second Machine Role or Runtime Snapshot model;
- a second blob store;
- F-019 Show Capsule packaging/restore.

## Verification contract

The slice must retain:

- cross-platform Swift provider tests through an injected opener;
- real macOS compilation of the production `NSWorkspace` path;
- safe ASSET and LOCATOR launch tests;
- missing application / missing target / symlink / open failure tests;
- exact PARTIAL snapshot identity and truthfulness tests;
- truthful `RECONNECT` unsupported behavior;
- Operator API authentication, RBAC, scope, bounded JSON and result tests;
- Operator Web contract tests forbidding generic execution authority;
- existing real macOS Companion replacement and media-resume acceptance;
- Core module lock, Test, Vet, Race and Linux ARM64 CGo-free product builds.
