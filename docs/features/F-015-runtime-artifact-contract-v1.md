# F-015 Runtime Artifact Contract v1

Status: implementation checkpoint

## Purpose

Define the first executable artifact contract required before StageCore may introduce extension activation authority.

This checkpoint does not enable, start, stop, chmod-to-executable, execute, or grant runtime permissions to any extension.

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

## Host compatibility

Readiness also verifies that the immutable package platform and architecture match the StageCore Hub that would eventually host the runtime.

This is deliberately separate from internal ELF/package consistency: a valid ARM64 ELF inside an ARM64 package is still not activation-ready on an AMD64 Hub.

PLUGIN readiness therefore includes a `runtime_host` check with results such as:

- `PASS / RUNTIME_HOST_COMPATIBLE`
- `BLOCKED / RUNTIME_HOST_MISMATCH`

## Readiness integration

PLUGIN readiness includes a `runtime_artifact` check.

Possible results include:

- `PASS / RUNTIME_ARTIFACT_VERIFIED`
- `BLOCKED / RUNTIME_CONTRACT_MISSING`
- `BLOCKED / RUNTIME_ARTIFACT_INVALID`

ADDON readiness reports:

- `NOT_APPLICABLE / ADDON_RUNTIME_NOT_REQUIRED`

for both runtime artifact and runtime host checks.

This remains independent from persistent runtime health. `runtime_health` remains:

- `NOT_APPLICABLE / ACTIVATION_NOT_IMPLEMENTED`

until an isolated execution lifecycle is implemented.

## Security boundaries

This checkpoint deliberately preserves these boundaries:

1. install does not execute code;
2. runtime inspection does not execute code;
3. installed payload remains non-executable;
4. permission review remains separate from runtime grants;
5. readiness remains read-only and derived;
6. `READY_FOR_ACTIVATION` is not `enabled`, `running`, or `healthy`;
7. SHOW-mode lifecycle mutation behavior is unchanged because this slice adds no lifecycle mutation.

## Execution-isolation finding

A logical capability/permission gate is not an operating-system sandbox.

Starting an extension process would otherwise allow that process to inherit the StageCore service account's ambient filesystem and network access before any `execution.request` permission check occurs. Production-ready/signing metadata alone is not sufficient to turn arbitrary package bytes into safe executable authority.

Therefore F-015 must not expose a general process-start boundary until StageCore has an enforceable runtime trust/isolation design. The immediate dependency-first slice is a non-executing activation staging gate, not a process probe.

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
- package/ELF consistency is distinct from Hub host compatibility;
- PLUGIN without runtime contract is `NOT_READY`;
- existing permission/dependency/trust readiness gates continue to work;
- authenticated readiness API exposes runtime and host verification without inventing runtime state.

## Next dependency-first slice

The next slice may stage and re-verify an activation candidate only after this checkpoint is green on `main`.

That staging slice must:

- require `READY_FOR_ACTIVATION`;
- remain SHOW-gated;
- derive reviewed permissions for the exact installation identity without creating durable runtime grants;
- copy into a managed transient runtime staging root;
- re-verify immutable hash and size;
- keep the staged copy non-executable;
- clean it before returning;
- report explicitly that execution is not authorized until a separately reviewed isolation/trust gate exists.

A later execution slice must additionally provide enforceable isolation before it may start arbitrary extension bytes. Only then should StageCore validate live `plugin.ready`, bridge runtime authority, and introduce persistent Enable/Disable semantics.
