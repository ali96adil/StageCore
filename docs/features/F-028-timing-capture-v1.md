# F-028 Timing Capture v1

**Feature:** F-028 — Rehearsal Timing Intelligence & Expected Next Cue
**Slice:** Phase 1 raw capture foundation
**Status:** Implemented by the F-028 timing-capture branch; prediction remains deferred

## Purpose

Collect useful rehearsal/show timing immediately after the F-027 Session foundation without creating a parallel analytics log or allowing timing data to influence GO authority.

This slice is deliberately raw capture. It does not calculate medians, confidence, expected-next-cue windows, or automatic timing decisions.

## Canonical ownership

F-028 reuses existing runtime truth:

- `sessions.started_at_us` is the Session elapsed-time anchor;
- `command_records.issued_at_us` is the accepted `cue.go` request timestamp when a correlated command record exists;
- `cue_executions.started_at_us` is the persisted Cue execution start;
- `cue_executions.completed_at_us` and `result` remain the exact terminal Cue result record;
- `action_executions.started_at_us` / `completed_at_us`, action events, `latency_ms`, and `ack_level` remain the dispatch/result/acknowledgement evidence where available;
- `event_records` remains the only canonical event/Flight Recorder journal;
- `operator_notes` remains the note store with optional Session/Cue linkage rather than copying note bodies into timing records.

No `timing_samples`, analytics history, or second event log is introduced.

## `cue.timing_observed` event

Migration `00014_rehearsal_timing_capture_f028.sql` installs an insert trigger on `cue_executions` for `REHEARSAL` and `SHOW` Sessions.

Each new Cue execution appends one `cue.timing_observed` record to `event_records` before later Cue/action result events are written.

The v1 payload records:

- capture version and raw-quality state;
- Session type and Session start;
- Cue execution and Cue identity;
- Cue start time and trigger source;
- manual-override truth already present on the Cue execution;
- correlated `cue.go` issue time when available;
- request-to-start elapsed microseconds;
- Session elapsed microseconds;
- previous Cue execution/Cue/start when available;
- Cue-to-Cue elapsed microseconds;
- path classification and skipped enabled Cue IDs;
- explicit clock-basis/health metadata.

The event `causation_id` points to the correlated `cue.go` command when one can be resolved. Direct internal/store-created Cue executions remain valid raw observations with no invented command causation.

## Path classification

Capture v1 distinguishes:

- `START` — first enabled Cue from a Session with no current Cue;
- `START_AT_CUE` — starting with a later enabled Cue, with earlier enabled Cues listed as skipped;
- `NEXT` — the next enabled Cue after current state;
- `REPEAT` — the current Cue is executed again;
- `FORWARD_JUMP` — one or more enabled Cues are skipped; their IDs are persisted;
- `BACK_JUMP` — execution moves to an earlier Cue.

Disabled Cues do not turn a normal transition into a skip.

## Clock truth

Capture v1 labels interval measurements:

- `basis = HUB_UTC_WALL`;
- `health = UNASSESSED`;
- `interval_scope = SINGLE_HUB`;
- request timestamps use `COMMAND_ENVELOPE_UTC` when a correlated command exists, otherwise `UNAVAILABLE`.

This is intentionally conservative. Until the Clock & Time Health foundation measures offset/drift and supplies monotonic/cross-machine health, F-028 must not silently promote these raw observations to trusted cross-machine timing.

Signed microsecond deltas are retained rather than clamped so a future clock-quality layer can detect regressions/skew instead of hiding them.

## Result and action timing

F-028 does not add duplicate result events.

Terminal Cue time/result already exists in `cue_executions.completed_at_us` plus `cue.completed` / `cue.failed`. Action dispatch and result/ack timing already exists in `action_executions` plus `action.started`, `action.completed`, `action.failed`, or `action.timed_out`.

Later statistical analysis must derive from these canonical records and the `cue.timing_observed` start observation.

## Historical migration rule

Migration 14 does not backfill `cue.timing_observed` for Cue executions created before the trigger existed. Historical Cue/Action timestamps remain available, but F-028 does not invent a historical path classification that was never captured at execution time.

## Deliberately not claimed by this slice

- no prediction or Expected Next Cue UI;
- no trusted-session selection/scoring;
- no confidence/median/range calculation;
- no auto-GO or timing-based command execution;
- no new pause/resume operation;
- no fabricated pause/resume events when F-027 has not yet exposed those operations;
- no external-machine clock synchronization or drift qualification;
- no Scene/Range/Checkpoint timing reconstruction beyond the currently persisted Cue path;
- no duplicated note body inside timing events.

Existing F-027 restart events such as `rehearsal.suspended` and `show.interrupted` remain canonical timing-affecting interruptions. When explicit pause/resume/repeat/skip workflows are added, they must continue to emit into the same `event_records` timeline rather than a separate F-028 log.

## Safety invariant

Timing capture is observational only. The trigger appends a P2 event and does not select a Cue, dispatch an Action, alter Session progress, change a command result, or issue GO.
