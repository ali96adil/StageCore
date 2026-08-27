# 05 — MVP Product Specification — v0.1

**Document Type:** Executable Product Specification  
**Status:** Initial implementation baseline  
**Based on:** 02 System Architecture v0.1 + 03 Data Model v0.1 + 04 Event & Command Contracts v0.1  
**Goal:** define the smallest StageCore product that can be implemented, tested, and used for a real rehearsal without pretending to solve the full platform.

## MVP Definition

The MVP is complete only when this loop works end-to-end:

`Project -> Device Alias -> Input/Manual GO -> Route/Cue -> Action -> Companion/Plugin -> Result -> Rehearsal Log`

The first supported external action path is generic OSC. Basic HTTP and MIDI are included only after the OSC path is stable. Hardware Nodes, AI/Vision, full DMX, cloud dependency, and production HA are not part of this MVP.

## Product Rule

Every MVP feature must have:

1. a visible user behavior;
2. a concrete data model representation;
3. a command/event trace where runtime behavior is involved;
4. an explicit failure state;
5. acceptance criteria that can be tested.

If a feature cannot meet those five points in v0.1, it stays out of scope.

## Files

- [00 — Purpose & Scope](00-purpose-and-scope.md)
- [01 — MVP User & Use Cases](01-mvp-user-and-use-cases.md)
- [02 — Core User Flows](02-core-user-flows.md)
- [03 — Project & Dashboard](03-project-and-dashboard.md)
- [04 — Cue Workspace](04-cue-workspace.md)
- [05 — Routing & Inputs](05-routing-and-inputs.md)
- [06 — Rehearsal & Show Mode](06-rehearsal-and-show-mode.md)
- [07 — Publish, Preflight & Readiness](07-publish-preflight-and-readiness.md)
- [08 — Companion & Device Readiness](08-companion-and-device-readiness.md)
- [09 — Notes, Logs & Session Memory](09-notes-logs-and-session-memory.md)
- [10 — Errors, Degraded States & Safety](10-errors-degraded-states-and-safety.md)
- [11 — Non-Functional Requirements](11-non-functional-requirements.md)
- [12 — Out of Scope](12-out-of-scope.md)
- [13 — MVP Acceptance Criteria](13-mvp-acceptance-criteria.md)
