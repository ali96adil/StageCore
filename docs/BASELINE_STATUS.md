# StageCore Document Baseline Status

The ordered StageCore product/architecture/reliability baseline is `00–10`:

1. 00 — StageCore Master Plan v0.2
2. 01 — Architectural Decisions — Addendum 001
3. 02 — StageCore System Architecture v0.1
4. 03 — StageCore Data Model v0.1
5. 04 — StageCore Event & Command Contracts v0.1
6. 05 — StageCore MVP Product Specification v0.1
7. 06 — StageCore Plugin Contract v0.1
8. 07 — StageCore Companion Specification v0.1
9. 08 — StageCore Storage & Vault Specification v0.1
10. 09 — StageCore Security Model v0.1
11. 10 — StageCore Testing & Reliability Plan v0.1

Implementation technology was validated through `SPK-01`–`SPK-06`, and final pre-M0 consistency/entry decisions are recorded in **Architectural Decisions — Addendum 002: Implementation Baseline Finalization**.

## Current Implementation Status

**M0 — CORE PERSISTENCE: COMPLETE**  
**M1 — CUE ENGINE + SIMULATOR: COMPLETE**  
**M2 — REAL OSC: COMPLETE**  
**M3 — ROUTING: COMPLETE**  
**M4 — COMPANION + MACHINE ROLE: COMPLETE**  
**NEXT: PRE-M5 RASPBERRY PI SMOKE → M5 STORAGE / VAULT**

Completion evidence:

- `docs/checkpoints/2026-08-26-m0-core-persistence-complete.md`
- `docs/checkpoints/2026-08-26-m1-cue-engine-simulator-complete.md`
- `docs/checkpoints/2026-08-26-m2-real-osc-complete.md`
- `docs/checkpoints/2026-08-27-m3-routing-complete.md`
- `docs/checkpoints/2026-08-27-m4-companion-machine-role-complete.md`

Merged product commits:

```text
3b300ccf2549f417b3f86c4de841a4530902f9ca
m0: establish core persistence foundation

a5af7c269d516055831720fb4055276457757001
m1: implement cue engine and deterministic simulator

56feab35b7ec65fed4047bc106c12c30899adf0c
m2: implement real OSC capability path

7f573c32d8e8bbad151105025900869cf8eee5
m3: implement deterministic routing runtime

d2dab103fff7979953ac3c1af096d9bb4245d1de
m4: implement Companion and Machine Role runtime
```

Latest merged-main verification:

- M4 merged tree: `11447ef7157e81dc434edc33fee3ceaee8e3ad64`, byte-identical to the final reviewed/tested M4 tree;
- post-merge Core CI #79 — PASS;
- post-merge Companion Core CI #25 — PASS, including the real macOS Companion replacement acceptance.

### Accepted Technology Direction

- **SPK-01 — Core Technology Stack** — Go Hub; SQLite/WAL; HTTP+JSON; SSE browser events; TypeScript + React + Vite UI.
- **SPK-02 — Real OSC** — OSC 1.0 UDP `osc.send`; logical endpoint resolution; truthful `TRANSPORT_ONLY` acknowledgement.
- **SPK-03 — macOS Companion** — Swift CompanionCore; versioned authenticated WebSocket runtime channel; Machine Role/Snapshot reconciliation; duplicate/stale execution protection; Keychain-backed device identity.
- **SPK-04 — Plugin Process / IPC** — external Plugin process; JSON Lines stdio IPC; capability handshake; crash/hang containment; no automatic replay.
- **SPK-05 — Vault & Large File Transfer** — filesystem Vault objects; SHA-256 identity; staging/atomic promotion; HTTP range/resume; verified cache; SHOW transfer gate.
- **SPK-06 — Hub Deployment on ARM64 / Mini-PC** — 64-bit Linux; native `amd64`/`arm64`; systemd; local-first boot; independent Data/Vault roots for SSD/NVMe. Physical hardware qualification remains mandatory.

## Delivered Product Foundation

### M0 delivered

- real Go Hub product source under `cmd/` + `internal/`;
- pinned dependency graph and checksums;
- SQLite/WAL with required effective safety/durability settings;
- embedded Goose migrations;
- UUIDv7 persisted identities;
- Project + ProjectRevision transactional persistence;
- Cue/Action persistence;
- ProjectDeviceAlias/Input/Output/Route foundations;
- frozen-revision guards;
- FK and transaction rollback evidence;
- restart/reopen persistence evidence;
- verified local DB copy/reopen path;
- Go 1.26/1.27 CI, race evidence and Linux ARM64 CGo-free build evidence.

### M1 delivered

- production Command/Event Go envelopes and Event `trace_context`;
- authoritative persisted Event journal with monotonic Hub `sequence`;
- immutable Runtime Snapshot identity using canonical JSON + SHA-256;
- Session, CueExecution, ActionExecution and EventRecord persistence;
- deterministic COMPLETE/FAIL/TIMEOUT simulator;
- sequential, parallel and parallel-barrier Cue execution;
- FAIL_CUE / CONTINUE behavior;
- duplicate/idempotency protection and restart-safe no-replay;
- runtime proof that GO consumes Snapshot-captured definitions rather than mutable live state.

### M2 delivered

- generic capability executor/registry shared by simulated and real Actions;
- immutable Snapshot logical-target resolution;
- real `osc.send` OSC 1.0 UDP capability with typed arguments;
- truthful `COMPLETED / TRANSPORT_ONLY` acknowledgement;
- external `stagecore.osc` Plugin with versioned JSON Lines IPC;
- Plugin capability/permission checks and crash/hang containment;
- no replay after Plugin failure/restart;
- Hub composition wired to the actual OSC product path;
- ARM64 CGo-free Hub + OSC Plugin builds.

### M3 delivered

- deterministic Routing from typed Test/OSC inputs;
- Snapshot-only Route lookup;
- bounded conditions, transforms and debounce;
- persistent Route Trace and explicit failures;
- Route -> Cue through the normal validated command path;
- Route -> Output through the generic capability registry;
- real Route -> OSC path;
- route-atomic manual Test safety preflight;
- typed input authority and external OSC receive isolation;
- duplicate/restart/no-replay preservation;
- implementation-level routing latency evidence.

### M4 delivered

- stable Companion identity independent of hostname/IP;
- Project MachineRole + one-active assignment;
- truthful readiness, heartbeat -> OFFLINE and Snapshot/config evidence;
- RuntimeSnapshotID propagated through Cue and Route execution;
- secure P-256 macOS identity with private key in Keychain;
- explicit pairing approval, challenge/response authentication and bounded runtime sessions;
- authenticated Hub WebSocket runtime;
- stale Snapshot/wrong-role/unsupported/duplicate rejection;
- revocation removes runtime authority;
- headless real `stagecore-companion` executable;
- real macOS `osc.send` executor with truthful `TRANSPORT_ONLY` acknowledgement;
- replacement/no-replay with the same immutable Cue and Runtime Snapshot;
- real macOS acceptance with two distinct Keychain-backed Companion identities.

## Pre-M5 Raspberry Pi Smoke Gate

A bounded native Raspberry Pi smoke deployment is now appropriate **before M5 implementation begins**.

The current `main` already has Linux ARM64 CGo-free build evidence for the Hub, OSC Plugin and pairing CLI, but CI cross-build evidence is not the same as physical Pi qualification. The pre-M5 smoke should therefore validate the current M0–M4 control/runtime foundation on the selected Pi without claiming rehearsal-ready or show-ready hardware status.

Recommended smoke boundary:

```text
Pi 64-bit Linux
→ native StageCore binaries
→ persistent Data Root / SQLite WAL
→ restart/reopen
→ real OSC bench path
→ macOS Companion pair/auth/connect
→ VIDEO-MAIN assignment
→ Cue/Route runtime execution
→ disconnect/reconnect no replay
→ WAN disconnected local operation
→ short CPU/memory/temperature observation
```

Full hardware qualification remains later because the First Rehearsal gate requires real SSD/NVMe behavior, controlled power-loss/recovery, at least 2 GiB interrupted/resumed media transfer with SHA-256 verification, storage-pressure/thermal soak, Stage LAN failure/recovery and backup/restore evidence. Those storage-heavy proofs depend on M5 capabilities and therefore must not be falsely claimed by the pre-M5 smoke.

## M5 Entry Scope

M5 owns the first real Storage/Vault and media-aware runtime slice.

Reference implementation order:

```text
S0 Storage Root + DB persistence
→ S1 Vault object import + checksum
→ S2 File download/transfer jobs
→ S3 Companion media cache sync
→ S4 Hub Software Downloads
→ S5 SHOW traffic gates + capacity reserve
→ S6 Backup/restore proof
```

M5 must prove at least:

- managed Vault import through staging -> SHA-256 verification -> atomic promotion;
- database metadata for managed content identity;
- streaming/local-network download without loading whole files into application memory;
- resumable Companion download and `.part` cache behavior;
- required-media manifest comparison and checksum before media READY;
- SHOW-mode bulk-transfer pause/block behavior;
- filesystem capacity reserve/admission checks;
- one repeatable backup/restore path.

M5 must preserve the M0–M4 authority model: immutable Runtime Snapshot identity, truthful readiness/results, no hidden replay and logical Machine Role replacement.

## Remaining Work Is Owned — Not Unbounded TBD

- Security SEC0–SEC2 before intentional non-loopback control/configuration APIs are considered production-exposed on the Stage LAN;
- full Plugin permission administration and secret-bearing integration gates remain under SEC4/SEC5;
- Storage/Vault/media-aware readiness work is M5;
- SHOW security operations are owned by SEC6 + M5/M6 before rehearsal qualification;
- Operator Web UI remains in its MVP slice;
- production macOS signing/notarization/background packaging remains a later product gate beyond the bounded M4 runtime contract;
- real ARM64/Pi and Mini-PC hardware qualification remains required before naming a hardware SKU rehearsal/show-ready;
- Hardware Nodes, full DMX/lighting automation, AI/Vision, HA/cloud and distributed offline authority remain explicitly later/post-MVP work.

Every known deferred item remains assigned in `docs/adr/addendum-002/04-deferred-register-and-ownership-gates.md`.

Changes to an established decision require an explicit superseding ADR/decision with evidence; implementation must not silently drift the baseline.
