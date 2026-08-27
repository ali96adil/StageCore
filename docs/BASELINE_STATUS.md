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
**NEXT: M3 — ROUTING**

Completion evidence:

- `docs/checkpoints/2026-08-26-m0-core-persistence-complete.md`
- `docs/checkpoints/2026-08-26-m1-cue-engine-simulator-complete.md`
- `docs/checkpoints/2026-08-26-m2-real-osc-complete.md`

Merged product commits:

```text
3b300ccf2549f417b3f86c4de841a4530902f9ca
m0: establish core persistence foundation

a5af7c269d516055831720fb4055276457757001
m1: implement cue engine and deterministic simulator

56feab35b7ec65fed4047bc106c12c30899adf0c
m2: implement real OSC capability path
```

The M2 post-merge `main` Core CI run `32972408148` passed Go 1.26/1.27 tests and vet, native race tests, module-lock verification, and Linux ARM64 CGo-free builds for both the Hub and OSC Plugin binaries.

### Accepted Technology Direction

- **SPK-01 — Core Technology Stack** — Go Hub; SQLite/WAL; HTTP+JSON; SSE browser events; TypeScript + React + Vite UI.
- **SPK-02 — Real OSC** — OSC 1.0 UDP `osc.send`; logical endpoint resolution; truthful `TRANSPORT_ONLY` acknowledgement.
- **SPK-03 — macOS Companion** — Swift CompanionCore; versioned WebSocket runtime channel; Machine Role/Snapshot reconciliation; duplicate/stale execution protection.
- **SPK-04 — Plugin Process / IPC** — external Plugin process; JSON Lines stdio IPC; capability handshake; crash/hang containment; no automatic replay.
- **SPK-05 — Vault & Large File Transfer** — filesystem Vault objects; SHA-256 identity; staging/atomic promotion; HTTP range/resume; verified cache; SHOW transfer gate.
- **SPK-06 — Hub Deployment on ARM64 / Mini-PC** — 64-bit Linux; native `amd64`/`arm64`; systemd; local-first boot; independent Data/Vault roots for SSD/NVMe.

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
- loopback-only development health surface pending Security convergence;
- Go 1.26/1.27 CI, race evidence and Linux ARM64 CGo-free build evidence.

### M1 delivered

- production Command/Event Go envelopes;
- synchronized Event `trace_context` contract;
- authoritative persisted Event journal with monotonic Hub `sequence`;
- minimal immutable Runtime Snapshot from exact validated ProjectRevision;
- canonical JSON + SHA-256 Snapshot content identity;
- Simulation Session, CueExecution, ActionExecution and EventRecord persistence;
- deterministic COMPLETE/FAIL/TIMEOUT simulator;
- sequential, parallel and parallel-barrier Cue Action execution;
- FAIL_CUE / CONTINUE error-policy behavior;
- snapshot/current-Cue rejection guards;
- duplicate/idempotency protection;
- restart-safe command history with no automatic replay;
- explicit proof that runtime GO consumes Snapshot-captured definitions rather than live definition state.

### M2 delivered

- generic product capability executor/registry shared by simulated and real Actions;
- Runtime Snapshot schema 2 with deterministic logical target capture;
- Snapshot-only endpoint resolution so later live alias edits do not mutate active runtime behavior;
- product `osc.send` capability using OSC 1.0 over UDP;
- explicit typed OSC arguments: `int32`, `float32`, `string`, `bool`;
- truthful `COMPLETED / TRANSPORT_ONLY` acknowledgement for successful local UDP writes;
- external `stagecore.osc` Plugin process;
- versioned JSON Lines stdin/stdout IPC and `plugin.ready` handshake;
- Plugin identity/capability validation;
- explicit reference-path `network.udp.send` grant enforcement;
- crash/EOF/hang/cancellation containment and no automatic replay;
- lazy fresh Plugin process for later explicit execution;
- Hub composition wiring for the actual OSC product path;
- configurable OSC Plugin executable path with sibling-binary default;
- real localhost UDP receiver tests and App-level end-to-end evidence;
- ARM64 CGo-free builds for both `stagecore-hub` and `stagecore-osc-plugin`.

## M3 Entry Scope

M3 owns Routing execution and closes the Route-origin OSC acceptance intentionally deferred from M2:

```text
Test Input / supported OSC input
→ normalized Input event
→ Runtime Snapshot Route lookup
→ simple condition evaluation
→ debounce
→ Route Trace
→ Cue dispatch OR Output capability dispatch
→ existing capability registry
→ sim.test / osc.send
→ truthful result + Event history
```

M3 must prove that disabled/non-matching Routes dispatch nothing, debounce behavior is deterministic/testable, and each accepted input causes no duplicate Action/Cue dispatch.

M3 must reuse the capability boundary delivered by M2 rather than embedding protocol-specific OSC logic inside the Routing domain.

M3 does **not** silently promote Companion trust, Vault/media workflows, non-loopback Stage LAN control, Operator UI, Nodes, full DMX/lighting, AI/Vision or HA/cloud work.

## Remaining Work Is Owned — Not Unbounded TBD

- Security SEC0–SEC2 before non-loopback Stage LAN control;
- full Plugin permission administration remains in Security SEC5; M2 proved only the explicit reference OSC grant boundary;
- Routing implementation and `osc.receive` lifecycle in M3;
- Companion trust + real macOS bundle/Keychain/signing in M4/SEC3;
- Publish/Preflight/Readiness convergence before show-facing operation;
- media-aware Vault/cache/readiness work in Storage/M5;
- Operator Web UI in its MVP slice;
- 2 GiB transfer, real ARM64/Mini-PC SSD/NVMe, power-loss, thermal, Stage LAN and soak qualification before first rehearsal;
- Node MCU, full DMX/lighting automation, AI/Vision, HA/cloud and other explicitly post-MVP work.

Every known deferred item remains assigned in `docs/adr/addendum-002/04-deferred-register-and-ownership-gates.md`.

Changes to an established decision require an explicit superseding ADR/decision with evidence; implementation must not silently drift the baseline.
