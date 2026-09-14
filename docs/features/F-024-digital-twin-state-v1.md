# F-024 Digital Twin State v1

**Feature:** F-024 — Full Show Simulation / Digital Twin Mode  
**Slice:** B2 — state truth, immutable Snapshot binding and observability  
**Status:** software implementation candidate; physical/product qualification remains deferred under #148

## Authority

A Digital Twin belongs to exactly one authoritative `SIMULATION` Session and that Session's immutable `RuntimeSnapshotID`.

The Twin must never be rebound to another Runtime Snapshot. Caller-supplied Session or Snapshot identity is not authority; `SessionExecutor` resolves and canonicalizes persisted Session ownership before execution.

## State contract

`DigitalTwinStateContractVersion1 = 1`.

Each virtual target exposes explicit state-truth layers:

- `desired_online` — only present when simulation control explicitly requested an online/offline state;
- `observed_online` — current Digital Twin observation;
- `verified_online` — state verified by the deterministic Digital Twin itself;
- `restorable` — whether later checkpoint reconstruction is currently proven;
- `restoration_reason` — why automatic restoration is or is not available.

`scope` is always `SIMULATION_ONLY`. A simulation-verified value is never a physical device acknowledgement and must never populate Stage Device trust/runtime state.

Until Slice C captures a checkpoint, `restorable=false` with `restoration_reason=checkpoint_not_captured`.

## Scenario mutations

Fault configuration, fault clearing, explicit virtual online/offline changes and Twin reset are limited to ACTIVE `SIMULATION` Sessions when the canonical Store is attached.

A mutation is recorded in the canonical `event_records` Flight Recorder before the in-memory change is applied. If the Flight Recorder write fails, the requested mutation fails and is not applied.

Canonical event types:

- `simulation.fault.configured`
- `simulation.fault.cleared`
- `simulation.fault.consumed`
- `simulation.target.state_changed`
- `simulation.twin.reset`
- `simulation.execution.completed`
- `simulation.execution.failed`
- `simulation.execution.timed_out`
- `simulation.execution.cancelled`

Events carry project and Runtime Snapshot authority and source `stagecore.simulator.digital_twin`.

## Execution truth

Simulation outcomes keep `AckLevel=NONE` unless a later explicit virtual capability contract defines stronger simulation-only semantics. They do not fabricate transport latency, physical health, Companion acknowledgement or device acknowledgement.

One-shot fault consumption is observable. Disconnect/offline state persists in the Twin until reconnect, explicit state change or reset.

## Isolation

The Digital Twin does not mutate:

- `stage_devices` or Stage Device runtime state;
- pairing/trust/revocation records;
- Companion sessions;
- physical OSC/HTTP/script/device output;
- immutable Runtime Snapshots.

REHEARSAL and SHOW remain on the physical execution path unchanged.

## Slice C handoff

RANGE/CHECKPOINT work must reuse this state contract and F-027 Session state truth. Checkpoint capture may mark specific Twin state as restorable only when the captured state can be reconstructed deterministically without replaying historical commands.
