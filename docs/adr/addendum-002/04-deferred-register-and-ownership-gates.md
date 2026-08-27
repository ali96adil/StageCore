# 04 — Deferred Register & Ownership Gates

A deferred item is acceptable only when its owner milestone/gate is explicit. This register prevents a future implementation from treating an old open question as accidental permission to improvise.

## A. M1 — Cue Engine + Simulator

Owned by M1 before the slice is complete:

- production Command/Event Go structs;
- direct synchronization of the 04 contract with `trace_context`;
- Hub journal `sequence` implementation;
- Runtime Snapshot minimal serialization and immutable identity mechanism;
- canonical deterministic serialization used for Snapshot content hashing;
- Cue/Action execution state machine and error-policy behavior;
- deterministic simulated adapter;
- performance decision on whether SQLite read concurrency needs to expand beyond the M0 single-connection baseline.

None of these are M0 blockers because M0 does not execute Cues or publish Runtime Snapshots.

## B. SEC0–SEC2 — Before Non-Loopback Stage LAN Control

Owned by the Security implementation before any real control/configuration API is intentionally exposed to the Stage LAN:

- persistent asymmetric Hub identity rather than SPK-06 identity scaffolding;
- first OWNER bootstrap/setup-code flow;
- local user/password authentication;
- OWNER/TECHNICIAN/OPERATOR/VIEWER authorization;
- authenticated browser/API sessions;
- CSRF/session protection as applicable;
- authenticated realtime channel;
- production local transport/TLS policy and certificate handling for the supported deployment path.

Until this gate passes, development endpoints are loopback/private-test only.

## C. M2 + SEC5 — Plugin Product Integration

Owned before the real OSC integration is called product-ready:

- product Plugin Supervisor using the SPK-04 external-process contract;
- manifest validation and capability registration;
- OSC Plugin permission `network.udp.send` enforcement;
- crash/hang containment and bounded restart behavior in product code;
- no replay of prior execution after Plugin restart;
- production logging separation (`stdout` contract, `stderr` logs);
- Plugin package layout/version compatibility rules required by the first-party OSC package.

Full public marketplace/signing PKI remains post-MVP.

## D. SEC4/SEC5 — Before Secret-Bearing or Privileged Plugins

Owned before any capability requires credentials or privileged external access:

- encrypted Secret Store;
- secret references rather than secret values in Project configuration;
- redaction in logs/events/traces/errors/exports;
- Plugin permission grants/enforcement;
- secret access auditing.

## E. M3 — Routing

Owned by M3:

- actual Route evaluation semantics and condition/transform implementation;
- debounce/rate-limit implementation;
- Route Trace persistence/shape;
- input ordering/dedup rules for supported OSC/test inputs;
- performance proof for the route p95 target.

M0 persists Route definitions only.

## F. M4 + SEC3 — Companion Product / Trust

Owned before Companion is called real/trusted product functionality:

- Companion asymmetric identity/key storage;
- Hub fingerprint verification;
- explicit pairing approval;
- revocation;
- WebSocket authentication/session binding;
- product heartbeat/readiness/offline thresholds;
- operational `RoleAssignment` states from Addendum 002;
- replaceable Machine Role flow;
- reconnect reconciliation with no replay;
- stale Snapshot rejection;
- real macOS SwiftUI bundle validation;
- Keychain credential storage;
- macOS permissions/background/login behavior;
- signed/notarized release validation.

## G. Storage Slice + M5 — Media-Aware Publish/Preflight

Owned before media-aware READY/Preflight is claimed:

- real Vault metadata wired to SQLite;
- authenticated object/download authorization;
- Companion resumable `.part` media cache;
- SHA-256 verification and atomic promotion;
- required-media manifest comparison by content identity;
- filesystem free-space probing;
- runtime storage reserve enforcement against the actual filesystem;
- transfer bandwidth/concurrency controls needed for the reference setup;
- SHOW-mode bulk-transfer pause/throttle in product code.

## H. SEC6 + M5/M6 — SHOW Security Operations

Owned before a build is rehearsal-qualified:

- SHOW-mode administration blocks;
- emergency OWNER revocation;
- security audit records;
- session renewal behavior that does not require Internet;
- Preflight security checks;
- user/session/Companion revocation tests during runtime.

## I. First Rehearsal Qualification Gate

Must be completed on the selected reference hardware/network before StageCore is called rehearsal-ready:

- real Hub hardware recorded;
- real SSD/NVMe filesystem behavior;
- native ARM64 execution if ARM64 is the selected Hub;
- selected Mini-PC native execution if amd64 is selected;
- SQLite WAL/runtime/backup on that hardware;
- controlled process crash/restart recovery;
- controlled power-loss/recovery test on disposable/reference hardware;
- at least 2 GiB interrupted/resumed media transfer + SHA-256 verification;
- thermal/CPU/memory/disk soak;
- WAN disconnected for the local runtime loop;
- Companion disconnect/reconnect without duplicate action;
- real OSC receiver path;
- one safe target/adapter failure with truthful non-success;
- verified backup + restore drill;
- security denial-path checks;
- no hidden Snapshot/media/role mismatch reported as READY.

## J. Hardware Nodes — Post-Software-MVP Node Spike

Explicitly deferred until after the software MVP loop is proven:

- Node MCU/SoC family;
- firmware/RTOS choice;
- Node transport selection by family;
- OTA/update mechanism;
- hardware watchdog implementation;
- relay/sensor/DMX physical I/O design;
- detailed offline TTL/safe-state matrices;
- production enclosure/power/isolation.

The existing Node logical contract remains valid; no Node hardware choice is required by M0–M6.

## K. Lighting / DMX — Post-MVP Lighting Spike

Deferred:

- DMX gateway/interface hardware SKU;
- Art-Net vs sACN deployment specifics by venue;
- full patch editor;
- logical lighting scaling UI;
- Adapt Show to Venue automation.

StageCore remains an integration/show-logic layer, not a full lighting console.

## L. Filesystem / Hardware Product SKU

SPK-06 resolved the local-filesystem + SSD/NVMe direction, but no single filesystem or product SKU is globally mandated yet.

Owned by hardware/deployment qualification:

- exact filesystem for an appliance image;
- mount options;
- SSD/NVMe model/endurance requirements;
- Mini-PC/Pi product tier;
- thermal enclosure/power design.

Any selected filesystem must prove atomic rename/fsync behavior, recovery after abrupt loss, adequate large-file performance, and SQLite support on the target kernel/storage stack.

## M. Router/AP Product Choice

No vendor is part of StageCore architecture.

Owned by deployment/reference-bench qualification:

- reference Router/AP class/SKU;
- multicast/discovery behavior;
- coverage/interference testing;
- dual-WAN/failover only where deployed.

WAN remains optional and outside the critical local runtime dependency chain.

## N. Advanced Runtime Features — Post-MVP Unless Promoted by ADR

Deferred:

- production HA Hub cluster;
- automatic Hub failover;
- active-active Machine Roles;
- advanced Emergency Patch overlay system;
- distributed Companion offline show authority;
- cloud control/sync dependency;
- AI/Vision execution;
- automated projection mapping;
- performance-recording analysis pipeline;
- public third-party Plugin marketplace/PKI.

These cannot enter MVP silently. Promotion requires an explicit issue/spec/ADR and cannot weaken existing gates.

## O. Rule for Newly Discovered Open Items

A new open question must be recorded immediately with:

1. impact/scope;
2. blocking vs non-blocking status;
3. owning milestone/gate;
4. required evidence to close it.

`TBD` without an owner/gate is not an acceptable engineering state.