# F-024 — Simulation Checkpoints and Bounded Range Runs

**Slice:** C  
**Phase:** 5  
**Tracker:** #149  
**Physical qualification:** deferred under #148

## Purpose

Slice C completes the simulation-relevant F-027 start-position semantics without creating a second show model. `RANGE` and `CHECKPOINT` runs remain ordinary `SIMULATION` Sessions bound to one immutable published Runtime Snapshot.

## RANGE authority

A RANGE start is allowed only for an ACTIVE `SIMULATION` Session. The selected start and end Cues must both belong to the Session Runtime Snapshot and the start Cue may not sort after the end Cue.

The declared range is a server-side safety boundary. SQLite guards reject CueExecution creation outside the persisted range even when a caller explicitly requests another Cue. When the range-end Cue reaches a terminal execution result, the Session becomes `COMPLETED`, records `RANGE_END_REACHED`, clears `next_cue_id`, and emits `simulation.range.completed`. A later GO therefore cannot escape the range.

Starting in the middle of a show does not imply that earlier state was reconstructed. Until the operator explicitly accepts the current virtual start state, Session truth remains manual-confirmation-required. Confirmation changes restoration truth to `UNAVAILABLE`, not `VERIFIED`, and emits `simulation.start_state.confirmed` with `replay=false`.

## CHECKPOINT authority

A simulation checkpoint stores:

- source Session identity;
- project identity;
- immutable Runtime Snapshot identity;
- logical current / last-completed / next Cue positions;
- the versioned Digital Twin snapshot;
- SHA-256 content identity.

A checkpoint may be restored only into an ACTIVE `SIMULATION` Session with the same project and Runtime Snapshot. Cross-project or cross-Snapshot restoration is rejected.

## Replay-free restore

Checkpoint restoration installs the captured Digital Twin state directly. Historical Cue or Action commands are not replayed.

A normal successful restore marks virtual target truth as checkpoint-restored and persists the Session restoration reference. If persistence of that Session truth fails after the in-memory Twin was changed, StageCore restores the exact previous Twin snapshot, including its prior restoration metadata and fault state. A failed durable restore therefore cannot leave a silently mutated virtual runtime.

## SCENE status

StageCore currently has no published Scene entity/model in the canonical project schema. F-024 therefore does not fabricate Scene semantics. `SCENE` start remains explicitly unsupported until a real project Scene model exists.

## Safety invariants

Slice C does not weaken the F-024 execution boundary:

- no SIMULATION command may reach physical OSC/HTTP/script/Companion/device output;
- checkpoints contain simulation-only truth and never physical device truth;
- Runtime Snapshot identity cannot change during capture or restore;
- checkpoint restore does not mutate `stage_devices`, pairing/trust, Companion sessions, or network observations;
- REHEARSAL and SHOW behavior is unchanged.

## Software evidence required before merge

The Slice C exact PR head must pass Core CI for:

- module lock;
- unit/integration tests;
- vet;
- Go race tests on the primary toolchain;
- Linux ARM64 CGo-free product builds.

Physical/product acceptance remains deferred under #148 and is not implied by CI success.
