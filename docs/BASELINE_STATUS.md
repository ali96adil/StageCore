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

## Status

**READY FOR M0 — NO KNOWN UNOWNED PRE-M0 DECISIONS**

The product baseline is complete enough for implementation, the planned technology spikes are accepted, and known pre-M0 cross-document differences/entry choices are resolved or explicitly assigned to later gates.

### Accepted Technology Direction

- **SPK-01 — Core Technology Stack** — Go Hub; SQLite/WAL; HTTP+JSON; SSE browser events; TypeScript + React + Vite UI.
- **SPK-02 — Real OSC** — OSC 1.0 UDP `osc.send`; logical endpoint resolution; truthful `TRANSPORT_ONLY` acknowledgement.
- **SPK-03 — macOS Companion** — Swift CompanionCore; versioned WebSocket runtime channel; Machine Role/Snapshot reconciliation; duplicate/stale execution protection.
- **SPK-04 — Plugin Process / IPC** — external Plugin process; JSON Lines stdio IPC; capability handshake; crash/hang containment; no automatic replay.
- **SPK-05 — Vault & Large File Transfer** — filesystem Vault objects; SHA-256 identity; staging/atomic promotion; HTTP range/resume; verified cache; SHOW transfer gate.
- **SPK-06 — Hub Deployment on ARM64 / Mini-PC** — 64-bit Linux; native `amd64`/`arm64`; systemd; local-first boot; independent Data/Vault roots for SSD/NVMe.

### Finalized M0 Entry Choices

Addendum 002 pins:

- Go 1.26 minimum with Go 1.27 CI coverage;
- `modernc.org/sqlite` v1.57.0;
- `database/sql` and no ORM in M0;
- `github.com/pressly/goose/v3` v3.27.3 with embedded SQL migrations;
- UUIDv7 IDs using `github.com/google/uuid` v1.6.0 initially;
- UTC Unix-microsecond persisted timestamps;
- validated/versioned JSON TEXT for extensible payloads;
- required SQLite durability/hardening settings;
- explicit ProjectRevision freeze semantics;
- resolved Event `trace_context`/journal sequence interpretation;
- resolved Companion RoleAssignment state vocabulary;
- explicit ownership gates for all known deferred items.

## Implementation Transition

The next phase is **M0 — Core Persistence**, not another broad planning document or disposable technology spike.

M0 must establish the first real StageCore product source tree and prove the pinned persistence stack through migrations, transaction/FK tests, Project/Revision/Cue/Action/alias/Input/Route persistence, restart/reopen proof, local DB copy/reopen verification, amd64 tests and arm64 CGo-free cross-build.

After M0 passes, implementation proceeds to **M1 — Cue Engine + Simulator**.

## Remaining Work Is Qualification or Later-Slice Scope — Not Unowned TBDs

Examples include:

- actual selected SQLite-driver build/WAL/backup acceptance evidence inside M0;
- production Command/Event structs and Snapshot hashing in M1;
- Security SEC0–SEC2 before non-loopback Stage LAN control;
- Plugin permission/product integration in M2/SEC5;
- Routing implementation in M3;
- Companion trust + real macOS bundle/Keychain/signing in M4/SEC3;
- media-aware Vault/cache/readiness work in Storage/M5;
- 2 GiB transfer, real ARM64/Mini-PC SSD/NVMe, power-loss, thermal, Stage LAN and soak qualification before first rehearsal;
- Node MCU, full DMX/lighting automation, AI/Vision, HA/cloud and other explicitly post-MVP work.

Every known deferred item is assigned in `docs/adr/addendum-002/04-deferred-register-and-ownership-gates.md`.

Changes to an established decision require an explicit superseding ADR/decision with evidence; implementation must not silently drift the baseline.
