# F-024 — Operator Simulation Workflow

**Slice:** D  
**Phase:** 5  
**Tracker:** #149  
**Physical qualification:** deferred under #148

## Purpose

Slice D makes the F-024 Digital Twin usable from the authenticated StageCore Operator without creating a second runtime system.

The workflow is layered over the existing authoritative components:

```text
Operator Web
  -> authenticated project-scoped Simulation API
  -> simulationcontrol.Service
  -> canonical Store / Command Envelope / Cue Engine
  -> SessionExecutor
  -> session-scoped Digital Twin
```

A SIMULATION command never bypasses the F-024 execution gate. REHEARSAL and SHOW remain separate runtime modes and keep their existing physical execution behavior.

## Operator workspace

The project workspace exposes a dedicated bilingual Arabic/English Simulation page with a persistent safety banner:

```text
SIMULATION · NO PHYSICAL OUTPUT
```

The workspace supports:

- BEGINNING start from the latest published Runtime Snapshot;
- bounded RANGE start;
- CHECKPOINT start and replay-free virtual-state restore;
- explicit operator confirmation when a mid-show RANGE start cannot reconstruct prior state truth;
- canonical GO through the ordinary Cue Engine;
- deterministic simulation-only fault injection and clearing;
- virtual target online/offline mutation;
- Digital Twin reset;
- checkpoint capture and restore;
- Session, Digital Twin, Cue execution and Flight Recorder inspection;
- explicit stop of the active Simulation Session.

No control in this workspace calls the physical REHEARSAL/SHOW runtime GO/JUMP endpoints.

## Start semantics

### BEGINNING

Creates an ACTIVE `SIMULATION` Session against the latest published Runtime Snapshot and starts from normal beginning semantics.

### RANGE

Requires both start and end Cue IDs. The persisted F-027 range boundary remains authoritative. Because starting in the middle does not prove prior state reconstruction, the operator must explicitly accept the virtual start state before GO is allowed.

The first GO uses the persisted Session `next_cue_id`, so it begins at the selected range start rather than accidentally resolving the first Cue in the show.

### CHECKPOINT

Requires a durable simulation checkpoint that belongs to the same project and immutable Runtime Snapshot. The checkpoint is restored directly into Digital Twin state without replaying historical Cue or Action commands.

## Fault controls

Fault mutation is scoped to the active SIMULATION Session only. Supported deterministic behaviors are inherited from the Digital Twin contract, including completion, delay, failure, timeout, offline/disconnect, reject, reconnect and capability unavailable.

A selector may use a resolved logical target, a capability, or both. If an authored action target is not resolved by the Runtime Snapshot target alias table, capability-scoped faulting remains truthful; Slice E reports the missing mapping separately instead of fabricating a resolved physical target.

## Command truth

Simulation start, stop and GO preserve canonical command identity/idempotency. Operator status surfaces use canonical persisted Session/Cue/Event truth plus the application-owned Digital Twin snapshot.

Simulation result acknowledgements remain simulation-only. They do not claim physical device acknowledgement, measured real latency, physical health, trust or connectivity.

## Safety invariants

Slice D must preserve all of the following:

- `SIMULATION -> physical capability executor` is forbidden;
- authoritative Session type is resolved server-side;
- action payloads cannot promote themselves into REHEARSAL or SHOW;
- Runtime Snapshot identity cannot be changed by an Operator control;
- Digital Twin mutation cannot alter `stage_devices`, pairing/trust, Companion sessions or network observations;
- RANGE and CHECKPOINT state truth is never upgraded to physically verified truth;
- checkpoint restoration does not replay historical commands;
- REHEARSAL and SHOW behavior is unchanged.

## Software evidence

The Slice D exact PR head must pass Core CI for:

- module lock;
- unit/integration tests;
- vet;
- race tests on the primary Go toolchain;
- Linux ARM64 CGo-free product builds;
- Operator Simulation safety/bilingual contract tests;
- integration proof that Operator Simulation GO and injected faults remain inside the Digital Twin and never call the physical executor.

Physical/product acceptance remains deferred under #148 and is not implied by software CI success.
