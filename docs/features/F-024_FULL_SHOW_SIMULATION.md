# F-024 — Full Show Simulation / Digital Twin Mode

**Status:** implementation in progress — Slices A/B software qualification  
**Phase:** 5  
**Tracker:** #149  
**Physical qualification:** deferred under #148

## Purpose

F-024 provides a safe, deterministic way to run a published StageCore show model without commanding real stage hardware or external execution engines.

Simulation is not a second Cue Engine and not a second project format. It reuses the ordinary immutable Runtime Snapshot, F-027 Session identity, Cue/Action execution records, Command Envelope identity and canonical Flight Recorder path.

## Authority model

A simulation run is an ordinary StageCore Session whose authoritative type is:

```text
SIMULATION
```

The Session and its immutable Runtime Snapshot remain the authority for:

- project identity;
- cue ordering and enablement;
- action capability identity;
- logical target identity;
- timeout and error policy;
- command correlation/idempotency;
- execution/result history.

The action payload cannot promote itself out of Simulation. The execution boundary resolves the complete canonical Session from persisted execution ancestry, then overwrites caller-supplied Session/Project/Snapshot metadata before dispatch. A caller therefore cannot redirect Digital Twin state into another Session.

## Non-negotiable safety rule

For work belonging to a `SIMULATION` Session:

```text
Cue / Routing execution
  -> persisted or snapshot-scoped execution authority
  -> canonical Session-mode gate
  -> session-scoped Digital Twin
```

The following path is forbidden:

```text
SIMULATION -> physical capability executor
```

That means a simulated action whose original capability is `osc.send`, HTTP, Script, Stage Device, Companion-forwarded execution or another real output retains that original identity in the Snapshot/execution history, but the physical executor is not called.

If authoritative Session ownership cannot be resolved, execution fails closed. StageCore never guesses that physical output is safe.

`REHEARSAL` and `SHOW` continue to use the existing physical capability executor unchanged.

## Slice A — Execution safety foundation

Slice A established the no-real-output boundary before adding stateful Digital Twin behavior.

### Existing primitive reused

`internal/simulator.Adapter` supplies deterministic inline behavior through the ordinary `capability.Result` contract:

- `COMPLETE`;
- `FAIL`;
- `TIMEOUT`;
- optional bounded delay;
- explicit simulated error code/message.

A missing inline simulation override defaults to deterministic completion. No real device acknowledgement or verified state is fabricated; simulator acknowledgement remains `NONE`.

### Session-mode gate

Cue actions resolve Session authority through:

```text
action_executions
  -> cue_executions
  -> sessions
```

Direct Route outputs that do not create an ActionExecution first resolve the exactly-one ACTIVE Session that owns the immutable Runtime Snapshot. Zero or ambiguous ownership fails closed.

The boundary covers direct Cue GO, direct Route outputs, and Route-triggered Cues.

### Slice A evidence

CI tests prove:

1. a SIMULATION action carrying a real capability never calls the physical executor;
2. direct simulated Route output never starts the physical OSC plugin;
3. simulated COMPLETE / FAIL / TIMEOUT remain deterministic;
4. REHEARSAL delegates to the existing physical executor;
5. SHOW delegates to the existing physical executor;
6. missing/unresolvable Session authority fails closed;
7. canonical Cue/Action/Route execution records receive the simulated result.

## Slice B — Session-scoped Digital Twin state and scenarios

Slice B adds a runtime-ephemeral virtual state model scoped primarily by:

```text
canonical Session ID + logical target
```

The immutable Runtime Snapshot still defines the target/capability identities. The Digital Twin is runtime execution state, not another project or device registry.

### Shared runtime

The application owns one `DigitalTwin` instance and shares it across Cue and Routing execution boundaries. A target therefore has one simulated runtime state for the Session regardless of whether it was reached by direct GO, a direct Route output, or a Route-triggered Cue.

Separate SIMULATION Sessions receive separate state maps and cannot contaminate one another.

### Virtual target state

For each logical target observed during a Simulation Session, the twin tracks:

- simulated online/offline state;
- execution count;
- last capability;
- last simulated result;
- last error code;
- last response summary.

New simulated targets default online. `OFFLINE` / `DISCONNECT` persist an offline condition for subsequent commands until an explicit `RECONNECT` or operator/runtime state change restores the virtual target.

This state is deliberately separate from `stage_devices`, physical pairing/trust, Companion sessions, network health, and real device telemetry.

### Deterministic fault scenarios

A fault scenario is scoped to one Simulation Session and selects at least one of:

- logical target reference;
- capability key.

When selectors overlap, the most specific matching rule wins. Supported bounded behaviors are:

- `COMPLETE`;
- `DELAY`;
- `FAIL`;
- `TIMEOUT`;
- `OFFLINE`;
- `DISCONNECT`;
- `REJECT`;
- `RECONNECT`.

A scenario can be persistent (`Uses = 0`) or finite/one-shot (`Uses > 0`). Negative uses or delays and unsupported behavior names are rejected.

The Digital Twin never holds its runtime mutex while sleeping or waiting for a timeout, so parallel Cue execution is not serialized by simulated delays.

### Runtime lifetime

Slice B state/scenarios are intentionally process-runtime/session-runtime state. They are not yet reusable scenario presets. Restart reconciliation already terminates interrupted runtime Sessions; a future F-024 slice may add explicit persistent scenario presets without converting runtime Twin truth into production device truth.

### Slice B evidence

Tests cover:

1. identical targets in separate Simulation Sessions remain isolated;
2. one-shot failures are consumed deterministically;
3. disconnect persists offline state across later commands;
4. reconnect restores the virtual target;
5. target+capability rules do not leak into unrelated capabilities;
6. timeout follows the caller's execution deadline;
7. snapshots are deterministic and record execution outcome;
8. a configured Cue fault reaches the ordinary Cue/Action execution records without physical dispatch;
9. a configured direct Route fault reaches the ordinary Route result path without starting physical OSC output;
10. caller-supplied fake Session identity cannot redirect Twin state.

## Slice C — F-027 checkpoint/range completion

F-024 will complete the simulation-relevant parts of the shared F-027 Session framework rather than create simulation-specific checkpoint semantics.

Planned work:

- RANGE start;
- CHECKPOINT start;
- deterministic checkpoint capture;
- explicit restorable/manual-confirmation/unavailable truth;
- no replay of already-completed physical commands;
- selected scene/range execution only where the published project model resolves it truthfully.

## Slice D — Operator workflow

A bilingual Arabic/English, RTL-safe Simulation workspace will expose:

- start Simulation from the published Runtime Snapshot;
- run full show or selected supported range;
- inspect virtual target state;
- inject/reset bounded faults;
- inspect Cue/Action/command/event results;
- obvious persistent visual distinction between `SIMULATION`, `REHEARSAL` and `SHOW`.

No Simulation operator control may directly address physical hardware.

Slice B exposes the application-owned Digital Twin runtime needed by this later operator/API surface; it does not yet claim the operator workspace is complete.

## Relationship to F-020

F-020 Self-Healing / Optional HA is intentionally after F-024.

Recovery policies must first be exercised against deterministic simulated failures, reconnects, timeouts, checkpoint state and command replay boundaries. F-020 may reuse F-024 fault scenarios, but it may not weaken the Simulation safety boundary or create another recovery-state authority.

## Qualification policy while travelling

During the #148 deferral:

- exact-head deterministic CI is mandatory;
- repository cleanliness and module lock remain mandatory;
- Phase 5 software freeze SHAs are recorded;
- no Raspberry Pi deployment is required for each slice;
- F-024 is not called physically COMPLETE from CI/simulator evidence alone.

Final physical/product qualification will be cumulative after the remaining planned software phases are built and frozen.
