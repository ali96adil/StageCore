# StageCore Documentation

## Reading Order

- **00 — Master Plan v0.2:** [Product baseline](baseline/master-plan-v0.2/README.md)
- **01 — Architectural Decisions Addendum 001:** [Approved design decisions](adr/addendum-001/README.md)
- **02 — System Architecture v0.1:** [System architecture](architecture/system-architecture-v0.1/README.md)
- **03 — Data Model v0.1:** [Entities and relationships](specifications/03-data-model-v0.1.md)
- **04 — Event & Command Contracts v0.1:** [Runtime contracts](specifications/04-event-command-contracts-v0.1.md)
- **05 — MVP Product Specification v0.1:** [Executable MVP behavior and acceptance](specifications/05-mvp-product-spec-v0.1/README.md)
- **06 — Plugin Contract v0.1:** [Capability extensions and native UI integration](specifications/06-plugin-contract-v0.1/README.md)
- **07 — Companion Specification v0.1:** [Clients, pairing, roles, sync and macOS Companion](specifications/07-companion-spec-v0.1/README.md)
- **08 — Storage & Vault Specification v0.1:** [Vault, heavy file sync, software repository, backup and SHOW-safe storage](specifications/08-storage-vault-spec-v0.1/README.md)
- **09 — Security Model v0.1:** [Identity, trust, users, pairing, permissions and secrets](specifications/09-security-model-v0.1/README.md)
- **10 — Testing & Reliability Plan v0.1:** [Verification, failure injection and rehearsal release gates](specifications/10-testing-reliability-plan-v0.1/README.md)

## Implementation Decisions

The `00–10` baseline defines what StageCore must do. Concrete stack choices and implementation-entry clarifications live separately so technology can evolve without silently weakening product intent.

Accepted technology spikes:

- **SPK-01 — Core Technology Stack:** Go Hub, SQLite/WAL direction, HTTP+JSON + SSE browser transport, TypeScript/React/Vite UI.
- **SPK-02 — Real OSC:** OSC 1.0 UDP `osc.send`, logical target mapping and truthful `TRANSPORT_ONLY` acknowledgement.
- **SPK-03 — macOS Companion:** Swift CompanionCore, WebSocket runtime channel, role/snapshot reconciliation and duplicate/stale execution guards.
- **SPK-04 — Plugin Process / IPC:** external Plugin process, JSON Lines stdio IPC, capability handshake, deadline/crash containment and no automatic replay.
- **SPK-05 — Vault & Large File Transfer:** filesystem Vault objects, SHA-256 identity, byte-range resume, verified cache promotion, SHOW transfer gate and runtime storage reserve.
- **SPK-06 — Hub Deployment on ARM64 / Mini-PC:** Linux `amd64`/`arm64` native binaries, systemd lifecycle, local-first boot and configurable SSD/NVMe storage.

Final pre-M0 decisions and consistency corrections:

- [Architectural Decisions — Addendum 002: Implementation Baseline Finalization](adr/addendum-002/README.md)

## Engineering Checkpoints

- [2026-08-26 — Implementation Baseline Checkpoint](checkpoints/2026-08-26-implementation-baseline.md)
- [2026-08-26 — Pre-M0 Finalization Checkpoint](checkpoints/2026-08-26-pre-m0-finalization.md)

The finalization checkpoint records the current entry state: **READY FOR M0 — NO KNOWN UNOWNED PRE-M0 DECISIONS**. Items not needed by M0 are not ignored; Addendum 002 assigns them to explicit later gates.

## Implementation Phase

The next milestone is **M0 — Core Persistence**: first real StageCore product code, pinned SQLite/migration/ID dependencies, migrations, authoritative Project persistence, restart/backup proof, and supported build validation.

## Documentation Convention

Documents are ordered numerically and named by subject. New specification files use `<NN>-<descriptive-topic>-v<version>` where they belong to the ordered product/specification sequence.

Product/specification documents must distinguish implementable milestone requirements from future ideas. Future capabilities remain explicitly out of scope until promoted by a documented decision.

- [Specifications index](specifications/README.md)
- [Engineering decisions](decisions/README.md)
- [Engineering checkpoints](checkpoints/README.md)
- [Documentation source policy](SOURCE_POLICY.md)
