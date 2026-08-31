# F-015 Runtime Artifact Contract v1

Status: implementation checkpoint

## Purpose

Define the first executable artifact contract required before StageCore may introduce extension activation authority.

This checkpoint does not enable, start, stop, chmod, execute, or grant runtime permissions to any extension.

## Plugin runtime manifest

A PLUGIN may declare an optional `runtime` object. The Library remains backward-compatible with earlier PLUGIN manifests that do not declare runtime metadata, but pre-activation readiness blocks those installations with `RUNTIME_CONTRACT_MISSING`.

Runtime v1 is:

```json
{
  "runtime": {
    "protocol": "stagecore.plugin.v1",
    "artifact": "native-executable",
    "capability_permissions": {
      "osc.send": ["network.udp.send"]
    }
  }
}
```

### Contract rules

- `runtime` is valid only for `PLUGIN` manifests.
- `ADDON` manifests must not declare a plugin runtime contract.
- protocol is exactly `stagecore.plugin.v1`.
- artifact is exactly `native-executable`.
- a runtime-bearing PLUGIN must declare at least one capability.
- every declared capability must have exactly one `capability_permissions` entry.
- a mapping may contain zero permissions.
- mapped permissions must already be declared in `manifest.permissions`.
- duplicate permission references within one capability are rejected.
- every permission requested by the manifest must be used by at least one capability.

The last rule prevents a package from requesting broad runtime authority that no declared capability needs.

## Artifact format

For Runtime Artifact Contract v1, the immutable installed `payload.pkg` is itself the native executable image.

The installer continues to materialize it as a verified, regular, non-writable, non-executable file with mode `0440`.

Runtime artifact inspection is read-only. It never adds execute permission and never starts the process.

### Linux validation

v1 supports Linux native executables only.

The inspector requires:

- valid ELF;
- ELF64;
- little-endian encoding;
- `ET_EXEC` or `ET_DYN` so PIE executables are accepted;
- ELF machine matching immutable software package architecture metadata.

Architecture mappings in v1:

| Package architecture | ELF machine |
| --- | --- |
| `arm64` / `aarch64` | `EM_AARCH64` |
| `amd64` / `x86_64` | `EM_X86_64` |

Unsupported platforms or architectures fail closed with `ErrRuntimeArtifactInvalid`.

## Readiness integration

PLUGIN readiness now includes a `runtime_artifact` check.

Possible results include:

- `PASS / RUNTIME_ARTIFACT_VERIFIED`
- `BLOCKED / RUNTIME_CONTRACT_MISSING`
- `BLOCKED / RUNTIME_ARTIFACT_INVALID`

ADDON readiness reports:

- `NOT_APPLICABLE / ADDON_RUNTIME_NOT_REQUIRED`

This check is independent from runtime health. `runtime_health` remains:

- `NOT_APPLICABLE / ACTIVATION_NOT_IMPLEMENTED`

until an activation contract is implemented.

## Security boundaries

This checkpoint deliberately preserves these boundaries:

1. install does not execute code;
2. runtime inspection does not execute code;
3. installed payload remains non-executable;
4. permission review remains separate from runtime grants;
5. readiness remains read-only and derived;
6. `READY_FOR_ACTIVATION` is not `enabled`, `running`, or `healthy`;
7. SHOW-mode lifecycle mutation behavior is unchanged because this slice adds no lifecycle mutation.

## Verification

Acceptance coverage includes:

- valid runtime manifest;
- missing capability mapping rejection;
- undeclared mapped permission rejection;
- unused requested permission rejection;
- ADDON runtime rejection;
- valid Linux ARM64 ELF inspection;
- installed payload remains non-executable after inspection;
- ARM64 package metadata rejects an AMD64 ELF;
- PLUGIN without runtime contract is `NOT_READY`;
- existing permission/dependency/trust readiness gates continue to work;
- authenticated readiness API exposes `RUNTIME_ARTIFACT_VERIFIED` without inventing runtime state.

## Next dependency-first slice

The next slice may introduce an activation probe/contract only after this checkpoint is green on `main`.

That later activation slice must still:

- require `READY_FOR_ACTIVATION`;
- create an executable runtime copy/staging artifact rather than mutating the immutable installed payload;
- bridge only explicitly reviewed permissions into runtime authority;
- validate `plugin.ready` identity, protocol, and advertised capabilities;
- fail closed and clean up on probe/start failure;
- remain SHOW-gated for lifecycle mutations;
- persist lifecycle intent separately from transient process health.
