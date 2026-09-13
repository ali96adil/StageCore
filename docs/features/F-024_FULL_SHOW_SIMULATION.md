# F-024 — Full Show Simulation / Digital Twin Mode

**Status:** implementation in progress — software qualification only  
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

The action payload cannot promote itself out of Simulation. The decision to suppress physical execution is derived from the persisted Session linked to the persisted ActionExecution.

## Non-negotiable safety rule

For Cue actions belonging to a `SIMULATION` Session:

```text
Cue Engine
  -> persisted ActionExecution
  -> authoritative Session-mode gate
  -> deterministic simulator
```

The following path is forbidden:

```text
SIMULATION -> physical capability executor
```

That means a simulated action whose original capability is `osc.send`, HTTP, Script, Stage Device, Companion-forwarded execution or another real output retains that original identity in the Snapshot/execution history, but the physical executor is not called.

If the authoritative Session mode cannot be resolved, execution fails closed. StageCore never guesses that physical output is safe.

`REHEARSAL` and `SHOW` continue to use the existing physical capability executor unchanged.

## Slice A — Execution safety foundation

The first F-024 slice establishes the boundary before adding Digital Twin state or operator tooling.

### Existing primitive reused

`internal/simulator.Adapter` already supplies deterministic behavior through the ordinary `capability.Result` contract:

- `COMPLETE`;
- `FAIL`;
- `TIMEOUT`;
- optional bounded delay;
- explicit simulated error code/message.

A missing simulation override defaults to deterministic completion. No real device acknowledgement or verified state is fabricated; simulator acknowledgement remains `NONE`.

### Session-mode gate

The execution gate resolves Session type through the already-persisted chain:

```text
action_executions
  -> cue_executions
  -> sessions
```

The decision therefore cannot be supplied by an action's JSON parameters, target configuration, network endpoint or client request.

### Required evidence

Tests must prove at minimum:

1. a SIMULATION action carrying a real capability never calls the physical executor;
2. simulated COMPLETE / FAIL / TIMEOUT remain deterministic;
3. REHEARSAL delegates to the existing physical executor;
4. SHOW delegates to the existing physical executor;
5. missing/unresolvable Session authority fails closed;
6. the canonical Cue/Action execution records still receive the simulated result.

## Slice B — Digital Twin state and scenarios

The next slice adds a versioned virtual state model scoped to:

```text
Session + Runtime Snapshot + logical target
```

It must not become a device registry or alternate source of production truth.

Virtual state will distinguish at least:

- desired state;
- simulated observed state;
- simulated verified state where the virtual capability can prove it;
- restorable state;
- unknown/manual-only state.

Planned bounded scenario controls include:

- offline;
- command rejection;
- execution failure;
- timeout;
- delay;
- reconnect;
- capability unavailable.

Scenario changes are simulation-only and must not mutate real device pairing, trust, configuration, runtime state or health records.

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
