# SPK-06 — Hub Deployment on ARM64 / Mini-PC

**Status:** ACCEPTED for Linux deployment/release shape; real ARM64 hardware qualification still required  
**Scope:** Hub process packaging, Linux service lifecycle, ARM64/amd64 release builds, SSD/NVMe data layout, local-first startup and deployment boundaries  
**Validated by:** `prototypes/spk-06-hub-deployment`

## Decision

The first StageCore Hub deployment target is a **64-bit Linux host managed by systemd**, supporting both `amd64` Mini-PC class hardware and `arm64` Hub-class hardware such as Raspberry Pi-class devices.

The Hub is deployed as native binaries and local filesystem data. Docker/Kubernetes are **not** required for the MVP appliance/runtime path.

Reference deployment shape:

```text
64-bit Linux Hub
├── /opt/stagecore/bin/          executable binaries
├── /etc/stagecore/              deployment configuration
└── /var/lib/stagecore/          authoritative local data
    ├── db/                      SQLite state location in M0
    ├── runtime/                 runtime-local state
    ├── plugins/                 plugin runtime data/packages as policy allows
    ├── software/                local software repository metadata/content refs
    └── vault/                   managed large-file storage
        ├── staging/
        └── objects/sha256/
```

A deployment may mount SSD/NVMe directly at `/var/lib/stagecore` or configure `STAGECORE_VAULT_ROOT` to a separate local SSD/NVMe mount. The database and Vault remain local to the Hub; live show operation does not depend on NAS/cloud availability.

## Release Artifacts

The Hub release pipeline produces at least:

```text
stagecore-hub-linux-amd64
stagecore-hub-linux-arm64
```

with SHA-256 checksum metadata. The SPK-06 prototype builds these with:

```text
CGO_ENABLED=0
GOOS=linux
GOARCH=amd64 | arm64
```

for the current standard-library deployment shell.

This confirms that the Hub process boundary itself has no C toolchain/runtime dependency. When SQLite is wired in M0, the selected Go SQLite driver must preserve the required `amd64`/`arm64` build matrix or the driver choice is rejected.

## Service Lifecycle

The first Linux service manager is **systemd**.

The reference unit:

- starts after local filesystem availability and normal local networking;
- does **not** wait for `network-online.target` or WAN/Internet readiness;
- uses `Restart=on-failure`;
- sends `SIGTERM` for bounded graceful shutdown;
- runs as a dedicated `stagecore` user/group;
- uses a restrictive umask;
- enables basic service sandboxing (`NoNewPrivileges`, `ProtectSystem`, `ProtectHome`, `PrivateTmp`);
- limits writable service paths to StageCore authoritative storage in the reference layout;
- can require the StageCore data mount before launch.

Internet availability is not a boot prerequisite. A router may provide Internet to the Stage LAN, but Hub authority/runtime startup must succeed when WAN is absent.

## Hub Identity & Restart

The deployment shell creates/loads one stable Hub identity under the authoritative data root. Process restart does not create a new Hub identity.

This prototype identity file is only deployment/restart scaffolding. Production security identity follows 09 — Security Model and will use the appropriate asymmetric key material/secure storage rules. The important deployment invariant is that identity belongs to persistent Hub state, not hostname/IP/process lifetime.

## Data / Vault Mount Boundary

The Hub exposes independent configuration for:

- `STAGECORE_DATA_ROOT`
- `STAGECORE_VAULT_ROOT`

This supports a practical appliance layout where system/root storage is separate from the SSD/NVMe carrying heavy managed media.

The service must fail readiness when authoritative storage is not writable. Final deployment must also add the Storage specification capacity reserve, filesystem free-space probing and SQLite/Vault integrity checks.

## Network Boundary

The Hub listen address is deployment configuration, not identity.

The prototype defaults to loopback for safe development. A real Stage LAN deployment binds to the selected local interface/address only after the production authentication/TLS controls from 09 are active.

No discovery protocol is frozen by SPK-06. mDNS/DNS-SD remains a likely later local-discovery implementation, while manual host/IP fallback remains required.

## Health Endpoints

The deployment shell proves distinct health concepts:

- `/health/live` — process HTTP loop is alive;
- `/health/ready` — required local storage is writable and stable Hub identity is loaded;
- `/runtime/ping` — independent local runtime-facing path remains responsive.

These are prototype endpoints. Production readiness will additionally include database/migration state, Runtime Snapshot state, Plugin/Companion requirements and storage capacity policy.

## Prototype Evidence

Validation completed in the available Linux `amd64` environment using Go 1.23:

- `go test ./...` passes;
- native `linux/amd64` Hub binary builds and runs;
- `linux/arm64` cross-build succeeds as an ELF 64-bit ARM AArch64 executable;
- both release artifacts are statically linked for the current zero-dependency deployment shell;
- build script emits SHA-256 checksums;
- smoke test starts the Hub with an isolated Data Root and a separate simulated SSD Vault Root;
- `/health/ready` and `/runtime/ping` respond successfully;
- process receives SIGTERM and stops;
- restart with the same Data Root returns the same Hub identity;
- Vault directory remains on the independently configured storage path;
- the reference systemd unit parses successfully once its expected executable path exists;
- the unit orders after `local-fs.target`/`network.target`, not `network-online.target`.

Observed prototype artifact sizes in this environment were approximately 5 MiB each for stripped `amd64` and `arm64` binaries. Artifact size is evidence only, not a product SLA.

## What Is Not Proven Yet

This environment has no ARM64/QEMU runtime and no physical Raspberry Pi/Mini-PC test host. Therefore SPK-06 does **not** claim that Raspberry Pi 5 or any specific Mini-PC is production-qualified yet.

Before a hardware SKU is called show-ready, run the Testing & Reliability Plan on that exact class with:

- native ARM64/amd64 boot and restart tests;
- selected SQLite driver + WAL behavior;
- SSD/NVMe mount/unmount and filesystem checks;
- unexpected power-loss/recovery tests;
- 2 GiB interrupted media transfer and checksum verification;
- sustained Cue/Plugin/Companion traffic during bulk storage pressure;
- thermal/CPU/memory/disk soak;
- Ethernet/Wi-Fi Stage LAN failure/recovery;
- WAN disconnected for the full local runtime loop;
- actual service user/permissions and production TLS/authentication.

## SQLite Driver Gate

SPK-01 selected SQLite/WAL as the database architecture but deliberately left the Go driver to implementation validation. SPK-06 does not pretend that this unresolved dependency has been executed on ARM64.

**M0 must pin the SQLite driver before Core Persistence is accepted.** The preferred direction remains a maintained CGo-free/pure-Go driver if it passes:

1. Linux `amd64` build/test;
2. Linux `arm64` build/test;
3. WAL/concurrency behavior required by StageCore;
4. backup/recovery tests;
5. acceptable binary/runtime cost.

If it fails those gates, the driver choice changes; the SQLite data-model decision does not need to change automatically.

## Acceptance Result

**ACCEPTED** for:

- 64-bit Linux as first Hub deployment OS family;
- `amd64` + `arm64` release targets;
- native binary deployment without mandatory container runtime;
- systemd as first service lifecycle mechanism;
- local-first boot with no WAN/network-online dependency;
- `/opt/stagecore` + `/etc/stagecore` + `/var/lib/stagecore` reference layout;
- configurable independent Data/Vault roots for SSD/NVMe deployment;
- stable persistent Hub identity across restart;
- static cross-build of the current Go Hub shell for ARM64;
- SHA-256 release artifact checksums.

**Hardware qualification remains mandatory** before naming a particular Pi/Mini-PC model production-ready.

## Transition to Implementation

The planned technology spikes `SPK-01` through `SPK-06` are now complete enough to start implementation slices.

The next engineering milestone is:

**M0 — Core Persistence**

M0 should create the real StageCore Hub skeleton, pin/validate the SQLite driver, implement migrations and authoritative Project persistence, and retain the deployment boundaries proven here.
