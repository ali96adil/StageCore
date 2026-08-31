# F-015 — Runtime Isolation Contract v1

**Status:** implementation checkpoint  
**Scope:** enforceable Linux namespace boundary and fail-closed readiness for future installable Plugin execution.

## Purpose

The previous activation-staging checkpoint proved package integrity and host compatibility without starting extension code. This slice defines the operating-system boundary that must exist before StageCore may perform even a live `plugin.ready` probe for an installable third-party Plugin.

This is deliberately separate from the existing first-party Plugin Host. Bundled StageCore binaries such as the OSC plugin already have a trusted deployment path; installable extension bytes do not inherit that trust.

## Isolation engine

Runtime Isolation Contract v1 uses Linux Bubblewrap (`bwrap`) as the namespace launcher.

The contract is fail closed:

- if the installation is not `READY_FOR_ACTIVATION`, isolation is not authorized;
- if the runtime artifact is not a statically linked Linux ELF64 executable, isolation is not authorized;
- if Bubblewrap is unavailable or not executable, isolation is not authorized;
- if the installation has an approved `network.*` permission, isolation is not authorized until StageCore has a bounded network broker for that authority.

A successful assessment is `READY_FOR_ISOLATED_PROBE`. It does **not** mean enabled, running or healthy.

## Filesystem boundary

The planned sandbox exposes only the minimum runtime surface required for a static StageCore Plugin protocol executable:

- a private `/stagecore` directory;
- the verified transient Plugin executable read-only at `/stagecore/plugin`;
- a private `/proc` namespace;
- a private minimal `/dev`;
- a private tmpfs at `/tmp`.

The StageCore data root, Vault, configuration, installed-extension store, host home directories and normal host filesystem are not mounted into the sandbox.

The environment is cleared before StageCore adds only the protocol identity required by the Plugin contract.

## Network boundary

The launcher uses `--unshare-all`, which includes a private network namespace. Runtime Isolation v1 never adds `--share-net`.

Therefore installable Plugin code receives no direct Stage LAN or host network authority from this contract.

The currently understood extension permissions `network.udp.send` and `network.udp.listen` remain valid manifest/review vocabulary, but an approved installation requesting either permission is blocked with:

`RUNTIME_NETWORK_BROKER_REQUIRED`

This is intentional. Permission review is an authorization decision, not permission enforcement. Future network access must be mediated by a StageCore-owned broker that can enforce the exact approved operation rather than exposing the Hub service account's unrestricted sockets.

## Static executable requirement

Runtime Isolation v1 requires no ELF `PT_INTERP` segment.

This intentionally constrains the first safe executable format to a static Linux binary. It avoids mounting `/lib`, `/usr`, dynamic loader configuration or other broad host runtime trees into an untrusted sandbox. A later contract may introduce a verified runtime image if dynamic extensions become a product requirement.

## Planned Bubblewrap boundary

The generated launch plan includes:

- `--die-with-parent`;
- `--new-session`;
- `--unshare-all`;
- `--clearenv`;
- read-only binding of only the transient Plugin executable;
- private `/proc`, `/dev` and `/tmp`;
- working directory `/stagecore`.

No process is started by this slice.

## Security invariants

1. Library presence never authorizes execution.
2. Installation never authorizes execution.
3. Permission approval never grants ambient OS authority.
4. The immutable installed `payload.pkg` remains non-executable and is never launched directly.
5. Future execution must use a freshly verified transient runtime copy.
6. A missing sandbox dependency is a blocker, not a downgrade to direct `exec.Command`.
7. Network permissions remain blocked until an enforceable broker exists.
8. SHOW mutation/activation policy remains authoritative and must be rechecked around future process startup.

## Explicit non-goals

This checkpoint does not yet:

- copy an installed payload into an executable runtime location;
- call Bubblewrap or start an extension process;
- accept a live `plugin.ready` handshake;
- persist enabled/disabled/running state;
- grant durable Plugin permissions;
- provide a network broker;
- migrate the trusted bundled OSC Plugin Host into this extension path;
- expose Operator lifecycle controls.

## Verification

Unit coverage proves that:

- a ready no-network installation produces a private Bubblewrap probe plan;
- the plan contains `--unshare-all`, a cleared environment and only the minimal filesystem mounts;
- no host-network sharing flag is introduced;
- an approved network permission fails closed with `RUNTIME_NETWORK_BROKER_REQUIRED`;
- a missing sandbox engine fails closed;
- relative executable paths cannot be planned.

## Next dependency-first slice

The next F-015 slice should build directly on this contract:

1. create a verified transient runtime copy without changing the immutable installed payload;
2. make only that transient copy executable;
3. launch it through the approved Bubblewrap plan;
4. require a bounded, identity-matching `plugin.ready` handshake;
5. terminate the process and remove the transient copy after the probe;
6. prove that the Plugin cannot read StageCore data/config/Vault paths and has no host network;
7. only then introduce persistent Enable/Disable lifecycle state.
