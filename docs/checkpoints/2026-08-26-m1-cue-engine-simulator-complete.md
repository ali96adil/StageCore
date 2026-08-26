# StageCore — M1 Cue Engine + Simulator Completion Checkpoint

**Date:** 2026-08-26  
**Implementation slice:** M1 — Cue Engine + Simulator  
**Status:** COMPLETE  
**Merged product commit:** `a5af7c269d516055831720fb4055276457757001`  
**Pull request:** #10  
**Tracking issue:** #9 — closed as completed

## 1. Completion Decision

M1 is accepted as complete.

The first real StageCore runtime loop now exists on top of the M0 persistence foundation. Runtime execution is still simulator-only by design, but it is no longer a disposable spike: the code is product source under `internal/`, persists authoritative runtime history in SQLite, executes only immutable Runtime Snapshot content, and passed the project CI gates on `main` after merge.

**Decision:** `M1 COMPLETE — M2 REAL OSC IS NEXT`.

## 2. Product Capability Delivered

M1 establishes this product path:

```text
Validated ProjectRevision
        ↓
Minimal immutable Runtime Snapshot
        ↓
Simulation Session
        ↓
cue.go Command
        ↓
Cue Engine validation / duplicate guard
        ↓
CueExecution
        ↓
ActionExecution(s)
        ↓
Deterministic simulated adapter
        ↓
Command Result + Event journal
        ↓
Restart
        ↓
History remains authoritative; last command is not replayed
```

Delivered runtime capabilities:

- production Go Command and Event envelope types;
- Event `trace_context` aligned with System Architecture / Addendum 002 / specification 04;
- authoritative Hub Event journal with persisted monotonic `sequence`;
- minimal immutable Runtime Snapshot built from an exact `VALIDATED` ProjectRevision;
- deterministic canonical JSON serialization and SHA-256 snapshot content identity;
- Snapshot content immutability enforced in SQLite;
- Simulation Session persistence;
- CueExecution and ActionExecution persistence;
- deterministic simulator behaviors `COMPLETE`, `FAIL`, and `TIMEOUT`;
- sequential Action execution;
- bounded parallel and parallel-barrier semantics required by M1;
- Action error policy handling for `FAIL_CUE` and `CONTINUE`;
- truthful failure/timeout results and Events;
- duplicate/idempotency protection for `cue.go`;
- restart-safe command history with no automatic replay;
- explicit snapshot mismatch and expected-current-cue rejection guards;
- proof that GO execution consumes Snapshot-captured Action definitions rather than rereading mutable/live definition state.

## 3. Acceptance Evidence

Issue #9 required the following. All items are satisfied by merged code and tests:

| Requirement | Evidence |
|---|---|
| Production Command/Event envelopes | `internal/contracts/contracts.go` |
| `trace_context` | contract struct, Event persistence test, synchronized specification 04 |
| Monotonic journal sequence | `event_records.sequence` + reopen test |
| Immutable Runtime Snapshot | `internal/snapshot/`, migration 00002 trigger |
| Canonical serialization + SHA-256 | `internal/canonicaljson/`, Snapshot builder/tests |
| Snapshot-only Cue/Action execution | explicit direct-live-definition mutation rejection/proof test |
| COMPLETE / FAIL / TIMEOUT simulator | `internal/simulator/` + Cue Engine tests |
| CueExecution / ActionExecution / EventRecord | migration 00002 + `internal/store/runtime.go` |
| Sequential execution | success/failure ordering tests |
| Parallel / barrier semantics | dedicated ordering tests |
| Error policy controls Cue result | FAIL_CUE / CONTINUE tests |
| No duplicate replay | duplicate and unresolved duplicate tests |
| Restart preserves history | close/reopen duplicate test |
| Rejection / failure / timeout coverage | Cue Engine and rejection test suites |
| M0 gates remain green | post-merge Core CI evidence below |

## 4. CI Evidence

### Final PR-head gate

Head before merge:

```text
7c8b88072690594e995c8a2beb4a17491930a101
```

GitHub Actions run:

```text
32963405911
```

Result: `SUCCESS`.

### Post-merge `main` gate

Merged commit:

```text
a5af7c269d516055831720fb4055276457757001
```

GitHub Actions run:

```text
32966785414
```

The run passed:

- Go `1.26.x` tests;
- Go `1.26.x` vet;
- Go `1.26.x` native race tests;
- Linux ARM64 `CGO_ENABLED=0` cross-build;
- Go `1.27.x` tests;
- Go `1.27.x` vet;
- committed module-lock verification.

Therefore M1 acceptance is based on merged `main`, not only the feature branch.

## 5. Problems Found and Corrected During M1

Two failures were discovered by CI and corrected before merge:

1. Goose initially split the SQLite Runtime Snapshot immutability trigger at the semicolon inside `BEGIN ... END`. The migration was corrected with Goose statement-boundary directives before the migration entered `main`.
2. The M0 database test still expected schema version `1`; M1 introduced migration `00002`, so the test was corrected to expect version `2`.

A pre-existing documentation consistency item was also closed: `docs/specifications/04-event-command-contracts-v0.1.md` now explicitly includes `trace_context` and the authoritative Hub journal interpretation of `sequence`.

## 6. M1 Boundary Preserved

M1 intentionally did **not** introduce:

- real OSC product dispatch;
- Plugin supervisor product integration;
- Routing execution;
- Companion trust/runtime channel;
- non-loopback Stage LAN control;
- Vault/media product path;
- Operator Web UI;
- StageNodes;
- full DMX/lighting control;
- AI/Vision;
- HA/cloud/failover production work.

Those remain owned by later implementation/security/storage gates.

## 7. Transition to M2

The next implementation slice is:

**M2 — Real OSC**

M2 should replace one simulator-only Action path with the first real transport-backed capability while retaining all M1 truthfulness, Snapshot, execution-history, duplicate and failure semantics.

Expected vertical path:

```text
Published Runtime Snapshot
→ cue.go
→ Cue Engine
→ Action: osc.send
→ logical endpoint resolution
→ OSC UDP adapter / Plugin boundary
→ real local OSC receiver
→ truthful TRANSPORT_ONLY result
→ Action/Cue/Event history
```

M2 must not regress M0/M1 gates and must not expose unauthenticated non-loopback Stage LAN control before Security SEC0–SEC2 convergence.

## 8. Reference Rule

This checkpoint is the implementation transition reference after M1. It does not supersede the architecture/product baseline. If later implementation changes an established semantic, the change must be explicit through the appropriate ADR/decision and regression evidence.
