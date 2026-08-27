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
**M5 — STORAGE / VAULT / MEDIA READINESS: COMPLETE**  
**NEXT: PHYSICAL RASPBERRY PI M0–M5 QUALIFICATION — ISSUE #21**

Completion evidence:

- `docs/checkpoints/2026-08-26-m0-core-persistence-complete.md`
- `docs/checkpoints/2026-08-26-m1-cue-engine-simulator-complete.md`
- `docs/checkpoints/2026-08-26-m2-real-osc-complete.md`
- `docs/checkpoints/2026-08-27-m3-routing-complete.md`
- `docs/checkpoints/2026-08-27-m4-companion-machine-role-complete.md`
- `docs/checkpoints/2026-08-27-m5-storage-vault-complete.md`

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

99552d6d58512836ea325393812d52dbbded6f1d
M5 Storage / Vault / Media Readiness (#23)
```

Latest merged-main verification:

- M5 merged tree: `3e68ac45ccef668ff73c10875f9fd35b9865d9f9`;
- final tested M5 branch tree: `3e68ac45ccef668ff73c10875f9fd35b9865d9f9` — byte-identical to merged `main`;
- post-merge Core CI #146 — PASS;
- post-merge Companion Core CI #78 — PASS;
- real macOS Companion replacement acceptance — PASS;
- `2 GiB + 1 byte` interrupted/resumed media transfer + SHA-256 acceptance — PASS.

### Accepted Technology Direction

- **SPK-01 — Core Technology Stack** — Go Hub; SQLite/WAL; HTTP+JSON; SSE browser events; TypeScript + React + Vite UI.
- **SPK-02 — Real OSC** — OSC 1.0 UDP `osc.send`; logical endpoint resolution; truthful `TRANSPORT_ONLY` acknowledgement.
- **SPK-03 — macOS Companion** — Swift CompanionCore; versioned authenticated WebSocket runtime channel; Machine Role/Snapshot reconciliation; duplicate/stale execution protection; Keychain-backed device identity.
- **SPK-04 — Plugin Process / IPC** — external Plugin process; JSON Lines stdio IPC; capability handshake; crash/hang containment; no automatic replay.
- **SPK-05 — Vault & Large File Transfer** — filesystem Vault objects; SHA-256 identity; staging/atomic promotion; HTTP Range/resume; verified cache; SHOW transfer gate.
- **SPK-06 — Hub Deployment on ARM64 / Mini-PC** — 64-bit Linux; native `amd64`/`arm64`; systemd; local-first boot; independent Data/Vault roots for SSD/NVMe. Physical hardware qualification remains mandatory.

## Delivered Product Foundation

### M0 delivered

- real Go Hub product source under `cmd/` + `internal/`;
- SQLite/WAL persistence with required safety/durability settings;
- embedded migrations and persisted UUIDv7 identities;
- Project/Revision/Cue/Action and foundational target/input/output/route persistence;
- frozen-revision guards, FK/transaction evidence and restart/reopen persistence;
- Go CI, race evidence and Linux ARM64 CGo-free build evidence.

### M1 delivered

- production Command/Event envelopes and persisted Event journal;
- immutable Runtime Snapshot identity using canonical JSON + SHA-256;
- Session/CueExecution/ActionExecution history;
- deterministic COMPLETE/FAIL/TIMEOUT simulator;
- sequential, parallel and barrier Cue execution;
- duplicate/idempotency protection and restart-safe no-replay;
- runtime uses Snapshot-captured definitions rather than mutable live state.

### M2 delivered

- generic capability executor/registry;
- immutable Snapshot logical-target resolution;
- real `osc.send` OSC 1.0 UDP typed-argument capability;
- truthful `TRANSPORT_ONLY` acknowledgement;
- external `stagecore.osc` Plugin with versioned JSON Lines IPC;
- capability/permission checks, crash/hang containment and no replay after Plugin failure;
- ARM64 CGo-free Hub + OSC Plugin builds.

### M3 delivered

- deterministic Routing from typed Test/OSC inputs;
- Snapshot-only Route lookup;
- bounded conditions/transforms/debounce;
- persistent Route Trace and explicit failures;
- Route -> Cue through validated command path;
- Route -> Output through generic capability registry;
- real Route -> OSC path;
- typed input authority and external OSC receive isolation;
- duplicate/restart/no-replay preservation and routing latency evidence.

### M4 delivered

- stable Companion identity independent of hostname/IP;
- MachineRole + one-active assignment;
- truthful readiness and heartbeat -> OFFLINE;
- RuntimeSnapshotID propagated through Cue/Route execution;
- P-256 Keychain-backed macOS identity;
- explicit pairing, challenge/response authentication and bounded runtime sessions;
- authenticated Hub WebSocket runtime;
- stale Snapshot/wrong-role/unsupported/duplicate rejection;
- revocation removes runtime authority;
- headless real `stagecore-companion` executable;
- real macOS `osc.send` with truthful `TRANSPORT_ONLY`;
- real macOS replacement/no-replay acceptance with distinct Keychain identities.

### M5 delivered

- independent configurable Hub Data Root and Vault Root;
- SQLite metadata for immutable Vault objects, logical MediaAssets and content versions without storing large blobs in DB;
- staged streaming imports with SHA-256 content identity and atomic no-overwrite promotion;
- content-addressed immutable Vault object storage;
- authenticated disk-backed HTTP Range serving;
- macOS `.part` cache with bounded 8 MiB range chunks and restart-resume behavior;
- exact size + SHA-256 verification before final media promotion;
- Required Media captured by immutable Runtime Snapshot identity;
- truthful READY / BLOCKED / MISMATCH media readiness;
- local Software Repository with platform/arch/API/checksum/signing/notarization/channel metadata;
- local Downloads/Setup path that does not depend on WAN/Internet access;
- bulk-job policy isolated from P0/P1 runtime;
- SHOW blocks/pauses nonessential transfer/software/backup/archive work while P1 Cue execution remains functional;
- free-space health with reference 15% warning threshold;
- configurable runtime reserve with 2 GiB default and write admission enforcement;
- verified consistent state backup, tamper evidence and non-destructive restore to a new Data Root;
- `2 GiB + 1 byte` forced-interruption transfer acceptance: 16 MiB `.part` retained, HTTP Range resume, exact SHA-256, atomic promote.

## Physical Raspberry Pi Qualification Gate — Issue #21

The pre-M5 Pi smoke was intentionally deferred while M5 was being built. M5 is now merged, so Issue #21 becomes the next bounded engineering gate and should exercise the **complete M0–M5 baseline** on real ARM64 hardware.

Recommended physical qualification boundary:

```text
Pi 64-bit Linux
→ native StageCore ARM64 binaries
→ Data Root + independent Vault Root on intended storage
→ SQLite WAL restart/reopen
→ real OSC bench path
→ macOS Companion pair/auth/connect
→ VIDEO-MAIN assignment
→ Cue + Route runtime execution
→ disconnect/reconnect no replay
→ managed Vault import + SHA-256
→ real LAN media transfer + interruption/resume + checksum
→ Companion required-media READY transition
→ SHOW bulk pause while P1 Cue remains functional
→ storage reserve / low-space behavior
→ backup/restore drill
→ WAN disconnected local operation
→ controlled restart/power-recovery checks
→ CPU/memory/storage/temperature observation
```

Passing CI/cross-build and hosted macOS acceptance does not by itself qualify a Raspberry Pi, SSD/NVMe, power supply, thermal configuration or Stage LAN as rehearsal-ready/show-ready. That claim requires the physical evidence owned by Issue #21 and the Testing & Reliability baseline.

## Remaining Work Is Owned — Not Unbounded TBD

- physical ARM64/Pi and Mini-PC qualification is the immediate next gate (#21);
- Security SEC0–SEC2 remain required before intentional non-loopback control/configuration APIs are considered production-exposed on the Stage LAN;
- full Plugin permission administration and secret-bearing integration gates remain under SEC4/SEC5;
- SHOW security operations remain owned by SEC6 and later rehearsal qualification;
- Operator Web UI remains in its MVP slice;
- production macOS signing/notarization/background packaging remains a later product gate;
- Hardware Nodes, full DMX/lighting automation, AI/Vision, HA/cloud and distributed offline authority remain explicitly later/post-MVP work.

Every known deferred item remains assigned in `docs/adr/addendum-002/04-deferred-register-and-ownership-gates.md`.

Changes to an established decision require an explicit superseding ADR/decision with evidence; implementation must not silently drift the baseline.
