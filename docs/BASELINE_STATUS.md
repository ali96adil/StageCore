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

### Next

- **SPK-03 — macOS Companion** — prove trusted Hub-to-Mac execution, Machine Role assignment, reconnect reconciliation and replacement without Cue edits.

Changes to an established baseline decision should be made as explicit deltas/ADRs or superseding spikes rather than silently rewriting prior intent.
