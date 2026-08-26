# StageCore Engineering Decisions

Specifications define required behavior. Decision spikes and ADR addenda choose/validate concrete implementation technology without silently rewriting product intent.

## Accepted Spikes

- [SPK-01 — Core Technology Stack](spikes/SPK-01-core-technology-stack.md) — Go Hub, SQLite/WAL, HTTP+JSON, SSE, React/TypeScript/Vite direction.
- [SPK-02 — Real OSC](spikes/SPK-02-real-osc.md) — `osc.send` over UDP, logical endpoint resolution, typed OSC arguments and truthful `TRANSPORT_ONLY` acknowledgement.
- [SPK-03 — macOS Companion](spikes/SPK-03-macos-companion.md) — native Swift CompanionCore, persistent WebSocket command/result channel, Machine Role/Snapshot reconciliation and reconnect duplicate protection.
- [SPK-04 — Plugin Process / IPC](spikes/SPK-04-plugin-process-ipc.md) — external Plugin process, versioned JSON Lines over stdio, capability handshake, deadline/crash containment and no automatic replay.
- [SPK-05 — Vault & Large File Transfer](spikes/SPK-05-vault-large-file-transfer.md) — filesystem Vault objects, SHA-256 identity, HTTP range/resume, verified local cache, SHOW transfer gate and runtime storage reserve.
- [SPK-06 — Hub Deployment on ARM64 / Mini-PC](spikes/SPK-06-hub-deployment-arm64-minipc.md) — Linux `amd64`/`arm64` native binaries, systemd lifecycle, local-first boot, independent SSD/NVMe Vault root and explicit hardware-qualification gate.

## Finalized Implementation Entry

- [Architectural Decisions — Addendum 002: Implementation Baseline Finalization](../adr/addendum-002/README.md) — closes known pre-M0 consistency differences and pins the M0 entry stack/data rules.

Pinned M0 entry direction includes Go 1.26 minimum with Go 1.27 CI coverage, `modernc.org/sqlite` + SQLite/WAL, embedded `goose` SQL migrations, UUIDv7 IDs, explicit `database/sql` repositories without an ORM, UTC integer timestamps, required SQLite durability/hardening settings, and explicit ProjectRevision immutability rules.

## Current Entry State

The pre-M0 decision phase is closed:

- no known pre-M0 decision is left as an unowned `TBD`;
- mandatory implementation validation still belongs to M0 acceptance tests;
- later concerns are explicitly assigned to M1–M6, Security, Storage, hardware qualification, or post-MVP gates.

The next engineering milestone is **M0 — Core Persistence**.

## Rule

A decision changes only through explicit evidence and a superseding decision/ADR. Do not silently swap a driver, migration strategy, identity format, durability policy, protocol boundary, or product invariant inside implementation code.
