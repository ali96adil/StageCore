# 13 — MVP Acceptance Criteria

## 1. MVP Release Gate

StageCore MVP v0.1 is considered complete only when all **MUST** criteria below pass on a documented reference setup.

## 2. Implementation Slices

### Slice M0 — Core Persistence

**MUST**

- create/open Project;
- create stable ProjectRevision;
- persist and reload Cues, Actions, aliases, Inputs and Routes;
- structured IDs and migrations/test reset strategy exist;
- restart does not lose committed data.

### Slice M1 — Cue Engine + Simulated Adapter

**MUST**

- create Cue and Action;
- publish minimal Runtime Snapshot;
- start Rehearsal;
- `cue.go` runs expected next Cue;
- action result is recorded;
- CueExecution/ActionExecution survive restart;
- failed simulated Action produces failed Cue/Action result according to policy.

This slice must pass before real device protocols are added.

### Slice M2 — Real OSC Integration

**MUST**

- configure OSC target manually;
- send OSC Action from Cue;
- send OSC Action from Route;
- inspect actual acknowledgement level honestly (`sent` is not `verified`);
- handle unreachable/misconfigured target without false success;
- use a real OSC receiver or reproducible test receiver in acceptance test.

### Slice M3 — Routing

**MUST**

- inject test input;
- receive supported OSC input;
- evaluate simple condition;
- dispatch Cue or Output Action;
- show Route Trace;
- disabled/non-matching Route dispatches nothing;
- debounce behavior is testable.

### Slice M4 — Companion + Machine Role

**MUST**

- pair one Companion;
- assign one Machine Role;
- execute one local capability through Companion;
- report READY/OFFLINE;
- disconnect/reconnect without replaying previous command;
- replace assigned Companion without changing Cue logical target.

### Slice M5 — Publish & Preflight

**MUST**

- Draft edits do not mutate active Snapshot;
- validation blocks broken references;
- Publish generates distinct immutable Snapshot identity;
- endpoint/snapshot mismatch is visible;
- Preflight returns PASS/WARN/BLOCK items;
- SHOW cannot start when a configured blocking requirement fails.

### Slice M6 — Operator Runtime UI

**MUST**

Operator can run normal flow without terminal/database editing:

- open Project;
- see mode and Snapshot;
- see current/next Cue;
- press GO/STOP;
- inspect latest result/error;
- start/stop Rehearsal;
- add a Note;
- inspect Session execution history.

### Slice M7 — Basic HTTP/MIDI

**SHOULD for MVP final; MUST NOT delay M0–M6**

- one basic HTTP Action through the same capability/action contract;
- one basic MIDI send/receive path where platform libraries are stable;
- no protocol-specific logic leaks into Cue domain model.

If these integrations threaten the core release gate, they move to v0.1.1 rather than delaying the proven StageCore loop.

## 3. End-to-End Demo Scenario

The following scenario must work from a clean install/configuration state:

1. Create Project `Demo Show`.
2. Add logical target `VIDEO-MAIN`.
3. Configure an OSC receiver target.
4. Create Cue `1 — Intro` with OSC Action.
5. Create Input `TEST-GO` and Route it to another Cue or Output.
6. Validate and Publish Runtime Snapshot `v1`.
7. Start Rehearsal.
8. Press GO and confirm the receiver gets the expected OSC message.
9. Inject `TEST-GO` and confirm routing executes exactly once.
10. Disconnect/unavailable target test produces explicit non-success/degraded state.
11. Add a Note.
12. Stop Rehearsal.
13. Restart StageCore.
14. Reopen the Project and verify Cue definitions, Snapshot identity, Session history, Action results, and Note are present.

## 4. Performance Acceptance

On the selected reference hardware and local network:

- simple routing evaluation p95 target <= 20 ms before adapter dispatch;
- accepted P1 command to internal dispatch p95 target <= 50 ms;
- Hub runtime event to local operator UI update p95 target <= 250 ms;
- execute the reference 100-Cue/200-Action project without unbounded queue growth or runtime crash.

If a target is missed, record the measurement and investigate before calling the relevant runtime slice complete. Do not hide the result by relaxing metrics without evidence.

## 5. Reliability Acceptance

Must demonstrate repeatably:

- application restart preserves committed Project data;
- completed execution history persists;
- reconnect never automatically replays last non-idempotent Action;
- invalid Draft cannot become active runtime;
- Companion loss is visible;
- adapter failure is isolated from Project data;
- Internet disconnected does not break local runtime loop.

## 6. Definition of Done for Each GitHub Issue

An MVP implementation issue is Done only when:

- behavior matches this spec or an approved documented change;
- relevant unit/integration test exists;
- failure path is tested where applicable;
- no secrets are committed/logged;
- documentation/contracts are updated if behavior changed;
- code is reviewable and does not introduce an out-of-scope subsystem as a hidden dependency.

## 7. Release Decision

The final MVP question is simple:

> Can one operator prepare a small StageCore project, publish it, run real cues and routed actions locally, see truthful results, survive ordinary endpoint failures, and reopen a trustworthy rehearsal history afterward?

If yes and all MUST gates pass, MVP v0.1 is releasable. If not, adding more features does not make it more complete.
