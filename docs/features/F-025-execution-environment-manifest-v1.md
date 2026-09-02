# F-025 — Execution Environment Manifest v1

**Status:** Phase 3 foundation contract  
**Feature ID:** F-025  
**Depends on:** existing immutable Vault/content-hash model, canonical JSON, Project configuration authority, F-012 SHOW configuration lock, F-015 extension trust boundaries

## Goal

StageCore must be able to describe the external software environment a show depends on without putting VDMX-, QLab-, Ableton-, TouchDesigner-, or vendor-specific state models into Core.

Execution Environment Manifest v1 is the engine-neutral description of that requirement. It is deliberately a **description and integrity contract**, not an execution API.

A future Project persistence layer will bind one or more manifests to a Project/revision. A later engine adapter can inspect the real workstation and compare it with the manifest. F-019 can then include the same manifest in a Show Capsule rather than inventing a second portability format.

## Contract

A v1 manifest contains:

- `schema_version`: exactly `1`;
- `environment_key`: stable project-local semantic key such as `video-main` or `sound-main`;
- `name`: operator-readable environment name;
- `adapter_key`: stable StageCore adapter identity such as `stagecore.adapter.vdmx`;
- `application`: external application identity, version constraint, and supported host tuples;
- `assets`: project/session/media/preset/config/resource requirements;
- `external_extensions`: plug-ins/add-ons required by the external application, kept distinct from StageCore F-015 extensions;
- `bindings`: declared external device/input/output/display/network bindings and optional StageCore target relationship;
- optional `launch`: a declarative asset or locator that a future authorized adapter may use to open the environment.

The manifest itself grants no process, filesystem, network, Companion, or application-control authority.

## Content-bound versus reference-only assets

Every asset explicitly declares one of two policies.

### `CONTENT_BOUND`

The bytes are legitimately capturable and therefore the requirement carries:

- SHA-256 `content_hash`;
- exact `size_bytes`;
- optional `locator` describing the expected workstation location.

This identity is intentionally compatible with StageCore's existing `vault_objects` / media content-hash model. F-025 does not create a second blob store.

### `REFERENCE_ONLY`

StageCore can describe the required external item but must not claim possession of its bytes. It carries a non-secret `locator` and **must not** carry `content_hash` or `size_bytes`.

Typical reasons include application edition limitations, licensing restrictions, machine-owned resources, or state that the external application does not expose for legitimate export.

Reference-only is a truthful reduced-portability state; it is not treated as if a backup exists.

## External extension boundary

`external_extensions` describes plug-ins/add-ons used by the external application. It is not the F-015 StageCore Extension Manager inventory and does not inherit F-015 installation/runtime authority.

Future adapters may verify external extension presence/version through legitimate application or filesystem metadata, but this manifest does not install them.

## Host model

v1 supports explicit host tuples for:

- `darwin/arm64`
- `darwin/amd64`
- `linux/arm64`
- `linux/amd64`
- `windows/arm64`
- `windows/amd64`

Application version syntax is intentionally opaque and bounded. Different third-party applications use incompatible version schemes, so v1 records a truthful adapter-owned constraint string rather than pretending all applications follow SemVer.

## Canonical identity

Before hashing, StageCore:

1. validates the manifest;
2. normalizes host OS/architecture to lowercase;
3. normalizes SHA-256 digests to lowercase;
4. sorts host tuples and requirement lists by stable identity;
5. encodes through `internal/canonicaljson`;
6. computes SHA-256 over those canonical bytes.

Therefore list ordering and SHA-256 hex case cannot create a different environment identity for the same semantic manifest.

The normalizer returns a copy and does not mutate caller-owned state.

## Bounded validation

v1 rejects:

- unsupported schema versions;
- unstable keys outside lowercase `[a-z0-9._-]` or keys beginning with punctuation;
- unsupported host tuples or duplicates;
- duplicate asset, external-extension, or binding keys;
- malformed content hashes;
- negative/missing sizes for content-bound assets;
- reference-only assets that falsely claim captured bytes;
- missing reference-only locators;
- locator control characters or parent-traversal segments;
- launch targets that combine asset and locator forms;
- launch asset references that do not exist in the manifest;
- unbounded text/list sizes.

A locator is metadata only in this slice. It is not opened, resolved, fetched, or executed.

## Example — content-bound with a truthful reference-only dependency

```json
{
  "schema_version": 1,
  "environment_key": "video-main",
  "name": "Main video workstation",
  "adapter_key": "stagecore.adapter.vdmx",
  "application": {
    "key": "vdmx",
    "name": "VDMX",
    "vendor": "VIDVOX",
    "version_constraint": "8.x-tested",
    "hosts": [
      {"os": "darwin", "architecture": "arm64"}
    ]
  },
  "assets": [
    {
      "key": "workspace",
      "kind": "PROJECT_FILE",
      "name": "VDMX workspace",
      "capture_policy": "CONTENT_BOUND",
      "content_hash": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "size_bytes": 4096,
      "locator": "/Users/show/Stage.vdmx5"
    },
    {
      "key": "licensed-plugin-state",
      "kind": "RESOURCE",
      "name": "Licensed plugin state",
      "capture_policy": "REFERENCE_ONLY",
      "locator": "/Library/Application Support/Vendor/State"
    }
  ],
  "launch": {
    "kind": "ASSET",
    "asset_key": "workspace"
  }
}
```

This manifest truthfully says the workspace bytes are reproducible while the licensed resource is only described.

## Example — application state that cannot yet be captured

```json
{
  "schema_version": 1,
  "environment_key": "sound-main",
  "name": "Main sound workstation",
  "adapter_key": "stagecore.adapter.qlab",
  "application": {
    "key": "qlab",
    "name": "QLab",
    "vendor": "Figure 53",
    "version_constraint": "5.x-tested",
    "hosts": [
      {"os": "darwin", "architecture": "arm64"}
    ]
  },
  "assets": [
    {
      "key": "workspace",
      "kind": "PROJECT_FILE",
      "name": "QLab workspace",
      "capture_policy": "REFERENCE_ONLY",
      "locator": "/Users/show/Show.qlab5"
    }
  ],
  "launch": {
    "kind": "ASSET",
    "asset_key": "workspace"
  }
}
```

This does **not** claim that StageCore can recreate the workspace yet. A future adapter/readiness report must surface the portability limitation rather than silently treating the reference as a managed backup.

## SHOW and readiness semantics

This first slice has no persistence or mutation route and therefore does not change SHOW behavior.

Future read-only environment inspection may run during SHOW. Any future mutation—capture, replacement, import, restore, adapter configuration, or launch-authority configuration—must remain under the existing StageCore configuration/SHOW-lock policy.

Future readiness must map into the existing StageCore `PASS` / `WARN` / `BLOCK` Preflight semantics. F-025 must not create a competing health truth source.

Examples:

- exact required application + content hashes + required extensions/bindings present -> PASS candidate;
- optional dependency absent or reference-only portability limitation -> WARN candidate;
- required application/version/project asset missing or content hash mismatch -> BLOCK candidate.

The exact inspection rules belong to the next F-025 readiness slice, not this manifest-only contract.

## F-019 Show Capsule relationship

F-019 should consume the canonical F-025 manifest and its content identity.

For `CONTENT_BOUND` assets, a Capsule may include or resolve the already content-addressed bytes according to later export policy. For `REFERENCE_ONLY` assets, the Capsule must preserve the requirement and limitation but cannot claim those bytes are embedded.

This keeps environment description independent from the transport/package that carries it.

## Adapter rule

Engine adapters translate application-specific facts into this generic model. They may add adapter-owned metadata in future versioned extension points only after a concrete need is proven.

Core must not gain top-level fields such as `vdmx_workspace`, `qlab_workspace`, `ableton_set`, or application-specific runtime state.

## Deferred from v1 foundation

- persistence and Project/revision binding;
- Operator API/UI;
- real workstation discovery and readiness inspection;
- application launch/open/reconnect authority;
- capture/copy/backup workflows;
- VDMX Execution Environment Snapshot specifics;
- QLab/Ableton/TouchDesigner adapter implementations;
- Show Capsule packaging and restore;
- migration of existing media records into execution environments.

These are subsequent dependency-first slices built on the same manifest identity.
