# StageCore Implementation Baseline Checkpoint — 2026-08-26

**Checkpoint Type:** Full planning / specification / decision-spike consistency review  
**Review Base:** `8afa04557a4d7d9c373813de499332c6550137a4` (`main` after SPK-06)  
**Verdict:** **READY FOR M0 — WITH RECORDED FOLLOW-UP GATES**

## 1. Purpose

This checkpoint is the reference point between StageCore planning/technology validation and real product implementation.

It records:

- what the product is and is not;
- which architecture/product decisions are authoritative;
- which technology choices were proven by spikes;
- what prototype evidence does **not** yet count as product code;
- known consistency gaps and their resolution/owner milestone;
- the exact M0 boundary;
- cross-cutting Security/Storage gates that must converge before LAN/Companion/Rehearsal use.

This checkpoint does not silently rewrite older documents. If a later change alters an established invariant, use an explicit ADR, specification delta or superseding decision.

## 2. Reviewed Baseline

The review covered the active `00–10` baseline and the accepted technology spikes `SPK-01` through `SPK-06`.

### Product / architecture baseline

- `00` — Master Plan v0.2
- `01` — Architectural Decisions Addendum 001
- `02` — System Architecture v0.1
- `03` — Data Model v0.1
- `04` — Event & Command Contracts v0.1
- `05` — MVP Product Specification v0.1
- `06` — Plugin Contract v0.1
- `07` — Companion Specification v0.1
- `08` — Storage & Vault Specification v0.1
- `09` — Security Model v0.1
- `10` — Testing & Reliability Plan v0.1

### Accepted technology decisions

- `SPK-01` — Go Hub; SQLite/WAL direction; HTTP+JSON; SSE browser events; React/TypeScript/Vite.
- `SPK-02` — OSC 1.0 UDP `osc.send`; logical endpoint resolution; truthful `TRANSPORT_ONLY` acknowledgement.
- `SPK-03` — native Swift CompanionCore; versioned JSON over persistent WebSocket; reconnect/snapshot/duplicate guards.
- `SPK-04` — external Plugin process; JSON Lines over stdio; capability handshake; crash/hang containment; no automatic replay.
- `SPK-05` — filesystem Vault; SHA-256; staging/atomic promotion; HTTP Range resume; verified cache; SHOW transfer gate.
- `SPK-06` — 64-bit Linux Hub; `amd64`/`arm64`; native binary + systemd; local-first startup; configurable Data/Vault roots.

## 3. Product Invariants Confirmed

The review found the core product direction internally consistent.

1. **Local First** — Internet/WAN is optional and never required for the local show-control loop.
2. **Hub Authority** — Hub owns authoritative Project/runtime/session state. Clients, Companions and Nodes do not become alternate project authorities.
3. **Draft != Published Runtime** — EDIT changes do not mutate the active show runtime. Runtime executes an immutable Published Runtime Snapshot.
4. **Client != Companion** — Client is a user interface; Companion is a replaceable local execution agent. They may ship together on macOS without merging authority boundaries.
5. **Physical != Logical** — Cues/Routes prefer logical aliases, Machine Roles and capabilities rather than raw IP/channel/device identity.
6. **Capability Based** — protocol/device integrations remain behind generic capability contracts.
7. **Plugin First, but isolated** — external Plugin process is the v0.1 default. Plugin failure must not directly crash the critical Core process.
8. **No false success** — UDP send is not device execution proof; acknowledgement strength is truthful.
9. **No unsafe replay** — reconnect/restart never grants permission to replay the previous non-idempotent Action automatically.
10. **P0–P3 separation** — bulk storage, backup, UI enrichment and management work do not sit in front of P0/P1 runtime work.
11. **Media local at playback** — Hub/Vault may own masters; normal show playback uses verified Companion-local media.
12. **Safety boundary remains external** — StageCore does not replace certified E-Stops, interlocks, motion/safety controllers or other required safety hardware.

## 4. Prototype Evidence vs Product Reality

The accepted spikes prove feasibility and semantics, but the repository still has no production StageCore implementation under `src/` beyond the placeholder.

### SPK-01

**Proven:** Go Core/API shape, Project/Cue/Publish/GO/event/history/restart flow.  
**Not product proof:** the prototype persistence is JSON scaffolding, not the selected SQLite implementation.

### SPK-02

**Proven:** real OSC UDP packet transmission/decoding and truthful transport acknowledgement.  
**Later refinement:** product OSC execution should use the external Plugin boundary selected by SPK-04, not reintroduce protocol code into Core.

### SPK-03

**Proven:** Swift CompanionCore/Go Hub WebSocket flow, stable identity abstraction, snapshot mismatch and duplicate rejection.  
**Not proven:** real macOS SwiftUI app bundle, Keychain, local permissions, background/login behavior, signing/notarization.

### SPK-04

**Proven:** external Plugin process, stdio JSON Lines IPC, real OSC, crash/hang containment, deadline kill, lazy restart, no replay.  
**Not proven:** complete OS sandbox, package signature/extraction, resource telemetry and production cross-platform supervisor behavior.

### SPK-05

**Proven:** SHA-256 content-addressed Vault import, streaming HTTP Range resume, checksum-gated cache promotion, SHOW pause gate and reserve policy. Manual 256 MiB transfer succeeded with bounded memory behavior.  
**Not proven:** required 2 GiB qualification, Swift Companion cache implementation, production authorization, final hardware/filesystem behavior.

### SPK-06

**Proven:** Linux `amd64` native execution, `arm64` cross-build, systemd deployment shape, separate Data/Vault roots, local-first startup and restart-stable identity scaffolding.  
**Not proven:** execution on a real ARM64/Pi target, final Mini-PC SKU, SQLite/WAL on target hardware, power-loss/thermal/storage soak.

## 5. Consistency Findings

### 5.1 Event Envelope `trace_context` — resolved for implementation

System Architecture v0.1 includes `trace_context` in the Event Envelope; 04 Event & Command Contracts v0.1 omitted it from its field table.

**Resolution:** implementation treats `trace_context` as part of the Event Envelope, containing non-sensitive trace metadata when available. It must never carry secrets. The direct 04 document should be synchronized when the runtime contract structs are introduced in M1; until then this checkpoint resolves the discrepancy.

### 5.2 Documentation Source Policy — corrected with this checkpoint

The previous Source Policy named only baseline/ADR/architecture even though Specifications and accepted Engineering Decisions now carry active implementation requirements.

**Resolution:** `docs/SOURCE_POLICY.md` is updated to include baseline, ADR, architecture, specifications, decisions and checkpoints plus conflict-resolution rules.

### 5.3 `RoleAssignment` state vocabulary — defer to M4 model sync

Data Model v0.1 lists `ASSIGNED | READY | DEGRADED | RELEASED`; Companion Specification later defines the operational role state set `UNASSIGNED | ASSIGNED | SYNCING | READY | DEGRADED | OFFLINE | MISMATCH | RELEASED`.

**Resolution:** Companion Specification is the newer operational behavior. Before M4 schema/runtime work, synchronize the data model. `UNASSIGNED` should be treated carefully as a role-level derived state rather than necessarily a persisted assignment row state.

### 5.4 Historical “open technology” text — resolved by accepted spikes

Architecture/Security/Storage documents intentionally preserve older statements that language, database, IPC or framework choices were open at that time.

**Resolution:** accepted `SPK-01`–`SPK-06` decisions resolve those technology questions where they overlap. The older documents remain valid for requirements and invariants.

### 5.5 MVP vs Storage scope — consistent

MVP excludes full Vault lifecycle/archive UI, transcoding and automatic NAS/cloud workflows. Storage Specification requires a narrow real storage slice: managed objects, resume, required-media verification, local software downloads and backup/restore proof.

These are compatible. The narrow storage mechanism is an MVP support requirement; the full archive/media platform is not.

### 5.6 Companion offline wording — consistent refinement

Architecture allows limited endpoint resilience. Companion Specification narrows the MVP behavior: a disconnected Companion may finish a bounded already-accepted local Action where safe, but it does not accept new Project commands from stale cache. Distributed offline show authority is deferred.

This is a valid refinement, not a contradiction.

## 6. Open Decisions That Belong Inside M0/M1

These are implementation decisions, not reasons to reopen architecture planning.

### M0 entry decisions

- pin the Go SQLite driver and prove Linux `amd64` + `arm64` build compatibility;
- verify WAL, transaction, restart and backup behavior for that driver;
- choose the stable ID representation used by real persisted entities;
- choose the migration mechanism and test reset strategy;
- define SQLite connection/pragmas appropriate to one authoritative local Hub;
- define how structured/config payload fields are stored/versioned without protocol-specific leakage.

### Before M1 Publish/runtime implementation

- define ProjectRevision state-transition/freeze semantics precisely enough that a Published Snapshot can never depend on later-mutated configuration;
- synchronize the 04 Event Envelope with `trace_context` in code/spec;
- define the scope of optional Event `sequence` ordering where it is used.

## 7. M0 — Exact Boundary

M0 is the first production StageCore code. It is **not another spike**.

### M0 MUST

- establish the real Go Hub source/package tree;
- use a configurable authoritative Data Root consistent with SPK-06;
- pin and validate the SQLite driver;
- enable SQLite WAL as selected by SPK-01;
- implement versioned migrations;
- create/open `Project`;
- create stable `ProjectRevision`;
- persist/reload Cues and Actions;
- persist/reload Project device aliases required by the MVP foundation;
- persist/reload Inputs and Routes as definitions only;
- use structured stable IDs;
- enforce foreign-key/transaction consistency appropriate to the logical data model;
- prove committed state survives Hub restart with automated tests;
- provide deterministic test database/reset support;
- preserve deployment separation between application binaries and authoritative data.

### M0 MUST NOT silently expand into

- Cue execution/runtime scheduler;
- Runtime Snapshot publication;
- OSC/device execution;
- Plugin supervisor product integration;
- Companion pairing/channel;
- media transfer/Vault implementation beyond required storage-root boundaries;
- authentication UI or operator UI;
- Nodes, DMX, AI/Vision, cloud or HA.

Those belong to later slices/gates.

## 8. Cross-Cutting Convergence Gates

The product slices `M0–M6`, Security slices `SEC0–SEC6`, and Storage slices `S0–S6` are separate plans. This checkpoint defines how they must converge.

### Gate A — before any non-loopback Stage LAN control API

Complete at least the relevant Security foundation:

- `SEC0` Hub identity + first OWNER;
- `SEC1` user authentication/authorization;
- `SEC2` authenticated Web/realtime sessions;
- transport protection appropriate to the selected local deployment.

Until then, development HTTP endpoints bind to loopback/private test environments only. A local LAN is not a trust boundary.

### Gate B — before M4 Companion is considered real/trusted

Complete `SEC3` Companion keys, pairing, Hub fingerprint verification and revocation semantics.

### Gate C — before secret-bearing or privileged Plugins are product-ready

Complete `SEC4` Secret Store/redaction and `SEC5` Plugin permission enforcement as applicable.

### Gate D — before M5 can claim media-aware READY

Implement the required Storage pieces for real Project/Vault metadata, required-media manifest comparison, Companion resumable cache, checksum verification and capacity/readiness checks.

### Gate E — before First Rehearsal Qualification

Complete the required Security audit/SHOW gates, verified state/project backup/restore proof, real media readiness, real OSC target, trusted Companion, WAN-disconnected operation and Testing Plan qualification checklist.

## 9. First Rehearsal Remains a Separate Qualification Gate

Starting M0 does not mean StageCore is rehearsal-ready.

Before the first controlled rehearsal, the documented setup must prove at least:

- Project/Snapshot/Session state survives restart;
- no false success or duplicate Action replay;
- required Companion is trusted and READY;
- required media matches exact content identity;
- local runtime continues with WAN disconnected;
- authenticated/authorized runtime control;
- storage health/reserve is safe;
- verified recent Project/state backup and restore path;
- selected Mac permissions/integrations work;
- selected Hub hardware/network/storage passes the relevant qualification tests.

## 10. Hardware Qualification Status

No specific Raspberry Pi or Mini-PC model is yet called production/show-ready.

The architecture supports Linux `amd64` and `arm64`, but a selected hardware class still requires:

- native build/run;
- SQLite/WAL verification;
- SSD/NVMe mount and power-loss recovery;
- 2 GiB interrupted media transfer;
- runtime traffic under storage pressure;
- thermal/CPU/memory/disk soak;
- Stage LAN loss/recovery;
- WAN-disconnected full local loop;
- production service permissions and security transport.

## 11. Implementation Order After This Checkpoint

Primary product path remains:

```text
M0 Core Persistence
 -> M1 Cue Engine + Simulator
 -> M2 Real OSC through accepted Plugin boundary
 -> M3 Routing
 -> M4 Companion + Machine Role
 -> M5 Publish + Preflight
 -> M6 Operator Runtime UI
```

Security and Storage gates run as cross-cutting prerequisites described above; they are not optional post-MVP cleanup.

## 12. M0 Entry Verdict

**READY FOR M0.**

There is no architecture blocker requiring another broad planning document or another general technology spike.

The first M0 task should be narrow:

> Create the real Go Hub skeleton, select/validate the SQLite driver, create migration `0001`, and prove Project + ProjectRevision persistence/restart on a configurable Data Root.

Only after that passes should M0 add Cue/Action/Alias/Input/Route persistence incrementally.
