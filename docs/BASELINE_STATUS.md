# StageCore Document Baseline Status

The current engineering baseline consists of the ordered StageCore documents `00–10`:

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

## Status

**Architecture / Product / Reliability baseline and planned technology decision spikes are complete enough for MVP implementation.**

### Accepted Technology Spikes

- **SPK-01 — Core Technology Stack** — Go Hub; SQLite/WAL persistence direction; HTTP+JSON commands; SSE browser events; TypeScript + React + Vite UI.
- **SPK-02 — Real OSC** — OSC 1.0 UDP `osc.send`, logical endpoint resolution and truthful `TRANSPORT_ONLY` acknowledgement.
- **SPK-03 — macOS Companion** — Swift CompanionCore, versioned WebSocket runtime channel, Machine Role/Snapshot reconciliation and duplicate/stale execution protection.
- **SPK-04 — Plugin Process / IPC** — external Plugin process, JSON Lines stdio IPC, capability handshake, crash/hang containment, lazy restart and no automatic replay.
- **SPK-05 — Vault & Large File Transfer** — filesystem Vault objects, SHA-256 identity, staging/atomic promotion, HTTP range/resume, verified cache promotion, SHOW transfer gate and runtime storage reserve.
- **SPK-06 — Hub Deployment on ARM64 / Mini-PC** — 64-bit Linux deployment shape, native `amd64`/`arm64` binaries, systemd service lifecycle, local-first boot and independent Data/Vault roots suitable for SSD/NVMe.

## Implementation Transition

The next phase is **M0 — Core Persistence**, not another broad planning document.

M0 must:

1. establish the real Go Hub source tree rather than another disposable spike;
2. pin and validate the SQLite driver on the supported build matrix;
3. enable SQLite WAL and versioned migrations;
4. persist stable Project/ProjectRevision/Cue/Action/alias/Input/Route foundations required by the MVP boundary;
5. prove restart persistence with automated tests;
6. retain configurable Hub data roots and deployment/service boundaries from SPK-06.

After M0 passes, implementation proceeds to **M1 — Cue Engine + Simulator**.

## Outstanding Qualification Gates

Accepted architecture does not mean every target device is already production-qualified. The following remain explicit implementation/qualification work:

- real macOS SwiftUI/Keychain/background/signing/notarization tests for Companion;
- Plugin OS sandbox/package-signing/resource telemetry as required;
- Swift Companion media-cache and authenticated Vault endpoints;
- 2 GiB interrupted-transfer acceptance test;
- final selected SQLite driver runtime/WAL/backup validation;
- real ARM64/Pi and Mini-PC SSD/NVMe, power-loss, thermal, Stage LAN and soak qualification.

Changes to an established baseline decision should be made as explicit deltas/ADRs or superseding spikes rather than silently rewriting prior intent.
