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

Technology selection has started through executable decision spikes rather than additional broad planning documents.

### Accepted

- **SPK-01 — Core Technology Stack** — Go Hub; SQLite/WAL persistence direction; HTTP+JSON commands; SSE browser events; TypeScript + React + Vite product UI direction.
- Executable zero-dependency Core prototype proves Project -> Cue -> Publish -> GO -> simulated result -> event/history -> restart/reload.

### Next

- **SPK-02 — Real OSC** — replace the simulated adapter path with a real UDP `osc.send` adapter and reproducible receiver.

Changes to an established baseline decision should be made as explicit deltas/ADRs or superseding spikes rather than silently rewriting prior intent.
