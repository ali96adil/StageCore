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
**NEXT: M1 — CUE ENGINE + SIMULATOR**

M0 completion evidence is recorded in `docs/checkpoints/2026-08-26-m0-core-persistence-complete.md`.

Merged M0 product commit:

```text
3b300ccf2549f417b3f86c4de841a4530902f9ca
m0: establish core persistence foundation
```

The post-merge `main` M0 CI run `32960572160` passed the required Go 1.26/1.27 test/vet matrix, native race tests and Linux ARM64 CGo-free cross-build gate.

### Accepted Technology Direction

- **SPK-01 — Core Technology Stack** — Go Hub; SQLite/WAL; HTTP+JSON; SSE browser events; TypeScript + React + Vite UI.
- **SPK-02 — Real OSC** — OSC 1.0 UDP `osc.send`; logical endpoint resolution; truthful `TRANSPORT_ONLY` acknowledgement.
- **SPK-03 — macOS Companion** — Swift CompanionCore; versioned WebSocket runtime channel; Machine Role/Snapshot reconciliation; duplicate/stale execution protection.
- **SPK-04 — Plugin Process / IPC** — external Plugin process; JSON Lines stdio IPC; capability handshake; crash/hang containment; no automatic replay.
- **SPK-05 — Vault & Large File Transfer** — filesystem Vault objects; SHA-256 identity; staging/atomic promotion; HTTP range/resume; verified cache; SHOW transfer gate.
- **SPK-06 — Hub Deployment on ARM64 / Mini-PC** — 64-bit Linux; native `amd64`/`arm64`; systemd; local-first boot; independent Data/Vault roots for SSD/NVMe.

### M0 Delivered

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
- loopback-only development health surface pending Security convergence;
- Go 1.26/1.27 CI, race evidence and Linux ARM64 CGo-free build evidence.

## M1 Entry Scope

M1 is the next product slice. It owns:

- production Command/Event Go structs aligned with the contract baseline;
- direct synchronization of Event `trace_context`;
- monotonic authoritative Hub journal `sequence`;
- minimal immutable Runtime Snapshot serialization/identity sufficient for deterministic simulated execution;
- Cue/Action execution state machine and error-policy behavior;
- deterministic simulated adapter;
- execution/result/event persistence required to prove the first runtime loop;
- M1 tests for success, rejection, failure, timeout, duplicate handling and restart truthfulness.

M1 does **not** silently promote real OSC, Plugin product integration, Companion trust, Routing execution, Vault/media workflows, Stage LAN exposure, Nodes, DMX, AI/Vision or HA/cloud work.

## Remaining Work Is Owned — Not Unbounded TBD

- Security SEC0–SEC2 before non-loopback Stage LAN control;
- Plugin permission/product integration in M2/SEC5;
- Routing implementation in M3;
- Companion trust + real macOS bundle/Keychain/signing in M4/SEC3;
- media-aware Vault/cache/readiness work in Storage/M5;
- 2 GiB transfer, real ARM64/Mini-PC SSD/NVMe, power-loss, thermal, Stage LAN and soak qualification before first rehearsal;
- Node MCU, full DMX/lighting automation, AI/Vision, HA/cloud and other explicitly post-MVP work.

Every known deferred item remains assigned in `docs/adr/addendum-002/04-deferred-register-and-ownership-gates.md`.

Changes to an established decision require an explicit superseding ADR/decision with evidence; implementation must not silently drift the baseline.
