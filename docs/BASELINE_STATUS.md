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

**Architecture / Product / Reliability planning baseline complete for MVP implementation.**

Technology selection is proceeding through executable decision spikes rather than additional broad planning documents.

### Accepted

- **SPK-01 — Core Technology Stack** — Go Hub; SQLite/WAL persistence direction; HTTP+JSON commands; SSE browser events; TypeScript + React + Vite product UI direction.
- Executable Core prototype proves Project -> Cue -> Publish -> GO -> simulated result -> event/history -> restart/reload.
- **SPK-02 — Real OSC** — OSC 1.0 UDP `osc.send`; logical target resolution; typed arguments; one datagram per dispatch; successful UDP write reports `TRANSPORT_ONLY` only.
- Executable OSC prototype opens a real UDP receiver, sends/decodes the expected packet and verifies no automatic duplicate send.
- **SPK-03 — macOS Companion** — Swift CompanionCore; versioned JSON over persistent WebSocket; stable identity abstraction; `VIDEO-MAIN` / Runtime Snapshot reconciliation; duplicate and stale execution rejection.
- Executable Swift/Go prototype proves Hub -> Companion Action -> result, intentional disconnect -> reconnect with same identity, duplicate execution rejection, stale Snapshot rejection, then successful new execution.
- **SPK-04 — Plugin Process / IPC** — external Plugin process; versioned JSON Lines over stdin/stdout; `plugin.ready` capability handshake; Core-side target resolution; serialized MVP execution; deadline kill; crash/EOF containment; lazy restart; no automatic replay.
- Executable Go prototype performs real OSC UDP through the external Plugin, passes crash-once and hang-once containment/recovery tests, and passes Go race detection.
- **SPK-05 — Vault & Large File Transfer** — filesystem Vault objects; SHA-256 content identity; staging/atomic promotion; HTTP byte-range resume; verified `.part` cache promotion; SHOW-mode bulk-transfer gate; runtime storage reserve admission.
- Executable Go prototype passes normal/race tests, resumes an interrupted transfer, rejects corrupt cache data, pauses bulk work during SHOW while runtime ping remains responsive, and completes a manual 256 MiB streaming run with bounded memory behavior.

### Next

- **SPK-06 — Hub Deployment on ARM64 / Mini-PC** — prove the selected Go/SQLite/Vault stack on intended Linux deployment-class hardware and stage-network topology.

### Explicit SPK-03 Follow-up

The current validation environment is not macOS. SwiftUI app-bundle, Keychain credential storage, background/login behavior, local macOS permissions, signing and notarization still require validation on a real Mac/Xcode implementation. These do not change the accepted CompanionCore/channel contract.

### Explicit SPK-04 Follow-up

SPK-04 validates the external process/IPC boundary, not a complete OS sandbox. Plugin package extraction/signing, OS sandbox technology, idle heartbeat, resource telemetry and platform-specific production supervisor behavior remain implementation work under the existing Plugin/Security specifications.

### Explicit SPK-05 Follow-up

SPK-05 validates the Vault/transfer mechanism. SQLite metadata wiring, Swift Companion media-cache code, authenticated object authorization, real filesystem free-space probing, bandwidth/concurrency controls, the mandatory 2 GiB interrupted-transfer qualification, and final-hardware filesystem behavior remain implementation/qualification work.

Changes to an established baseline decision should be made as explicit deltas/ADRs or superseding spikes rather than silently rewriting prior intent.
