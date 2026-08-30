# F-027 Session Foundation v1

**Feature:** F-027 — Rehearsal & Show Session Modes  
**Slice:** dependency-first session/state foundation  
**Status:** foundation contract only; full F-027 remains incomplete

## Purpose

This contract establishes the durable Session primitives that later F-027 rehearsal resume, checkpoint, preparation, range/loop and SHOW recovery flows build on. It extends the M1–M6 Session model; it does not create a second runtime or state authority.

The immutable Published Runtime Snapshot referenced by a Session remains the authoritative executable definition. Session state records runtime history and recovery intent; it never mutates or replaces that Snapshot.

## Versioning

New Sessions use `session_contract_version = 1`.

Start-position and state-truth structures are independently versioned at `1` so later compatible metadata can be added without reinterpreting historical records.

Rows created before this migration are preserved with `start_position_kind = UNSPECIFIED`. Migration must not invent a starting position that was not previously recorded.

## Lifecycle

`lifecycle_state` is the F-027 authoritative Session lifecycle:

- `ACTIVE` — currently operational.
- `COMPLETED` — deliberately completed.
- `STOPPED` — deliberately stopped before completion.
- `SUSPENDED` — inactive rehearsal history that is a candidate for a later deliberate resume/reconciliation flow.
- `ABORTED` — interrupted/aborted and not automatically resumable.

The M1 `status` column remains a compatibility projection for existing runtime gates:

- `ACTIVE` -> `ACTIVE`
- `COMPLETED | STOPPED` -> `COMPLETED`
- `SUSPENDED | ABORTED` -> `ABORTED`

No `SUSPENDED` Session is operationally active.

A Hub restart keeps the existing no-replay reconciliation. In-flight Cue/Action work is cancelled. An active `REHEARSAL` becomes `SUSPENDED`; an active `SHOW` becomes `ABORTED`. Both require manual confirmation before any future restoration claim. Restart reconciliation writes a canonical session-scoped event to `event_records`.

## Start position

`start_position_kind` v1 reserves:

- `UNSPECIFIED` — migration-only historical value.
- `BEGINNING` — normal default show/rehearsal beginning.
- `CUE` — selected Cue in the Session Runtime Snapshot.
- `SCENE`, `RANGE`, `CHECKPOINT` — reserved compatibility vocabulary for later F-027 slices; not executable in this slice.

For `BEGINNING`, `current_cue_id` is initially empty and `next_cue_id` is the first enabled Cue in the immutable Session Runtime Snapshot.

For `CUE`, `current_cue_id` remains empty because the selected Cue has not happened yet. `start_cue_id` and `next_cue_id` identify the selected Cue. A selected-Cue rehearsal start is classified `MANUAL_CONFIRMATION_REQUIRED` until a later preparation/reconstruction flow proves more. A selected-Cue SHOW start is rejected in this slice; SHOW recovery remains deliberately stricter.

This foundation does **not** claim that changing a pointer reconstructs pre-Cue state.

## Progress truth

The fields have distinct meanings:

- `current_cue_id` — historical last Cue entered/attempted by runtime.
- `last_completed_cue_id` — historical last Cue with a terminal `COMPLETED` execution.
- `next_cue_id` — desired logical next candidate; it is not evidence that physical state is ready.

A terminal Cue result `COMPLETED | FAILED | TIMED_OUT` advances the persisted logical position. A `CANCELLED` Cue does not auto-advance `next_cue_id`; an interrupted Cue remains the reconciliation candidate instead of being silently skipped.

## State truth

State restoration truth is independent from Cue position:

- `NOT_ASSESSED`
- `NOT_REQUIRED`
- `RESTORABLE`
- `MANUAL_CONFIRMATION_REQUIRED`
- `UNAVAILABLE`

`desired_state_ref` and `verified_state_ref` are nullable versioned reference hooks. This slice does not invent a Live State Snapshot/checkpoint store before its ownership contract exists. A missing `verified_state_ref` must never be presented as verified physical state.

Physical scenery, actor/prop position, manual lighting, safety systems, motors, pyro and other external/irreversible state remain outside automatic verification unless a later capability provides explicit evidence.

## Existing invariants preserved

- Published Runtime Snapshot execution remains immutable and authoritative.
- Existing command idempotency and duplicate protection remain unchanged.
- Hub restart/Companion reconnect never replays historical commands.
- SHOW preflight/security/admin protections remain unchanged.
- Event history remains append-oriented and uses the canonical `event_records` journal.
- Existing Session IDs and M0–M6 history remain stable.
- Migration is additive; prior migrations are not rewritten.

## Explicitly not complete in this slice

The following remain later F-027 ownership:

- runtime/API/UI wiring for selected-Cue start;
- pre-Cue state calculation and preparation plans;
- Scene, Range and Checkpoint execution semantics;
- deliberate Resume Rehearsal command/UI and state reconstruction;
- pause/resume/repeat/skip/back-up/loop workflows;
- Live State Snapshot/checkpoint persistence once its ownership contract is defined;
- SHOW recovery/resume flow after operator/device/physical-state reconciliation.

## F-028 handoff

After this foundation is proven, F-028 timing capture can use the existing ordered `event_records` journal and Cue/Action execution timestamps keyed to stable Session identity. F-028 should add explicit timing-quality events for pause/resume/repeat/skip/jump/interruption through that same canonical path; it must not introduce a parallel analytics log.
