# 04 — Cue Workspace

## 1. Cue List

Required fields visible in list:

- display number/label;
- name;
- enabled state;
- Action count;
- validation status;
- optional warning/error indicator.

Required operations:

- create Cue;
- edit Cue;
- duplicate Cue;
- enable/disable Cue;
- reorder Cue;
- delete Draft Cue with confirmation.

## 2. Cue Editor

Cue fields required for MVP:

- number/display label;
- name;
- optional Scene;
- enabled;
- criticality: `NORMAL | CRITICAL`; `SAFETY_CRITICAL` is reserved and not executable in MVP;
- execution policy.

## 3. Action Editor

Each Action contains:

- target logical alias or Machine Role;
- capability key;
- parameters;
- execution order;
- execution mode;
- timeout policy;
- error policy;
- priority class.

First real Action type: `osc.send`.

Basic HTTP/MIDI Actions may be added after OSC acceptance passes, using the same Action model rather than separate Cue types.

## 4. Multi-Action Behavior

MVP must implement:

- `SEQUENTIAL`;
- `PARALLEL`;
- `PARALLEL_BARRIER` only if it can be implemented without destabilizing the first two; otherwise it may ship in the final MVP slice.

Every Action gets its own ActionExecution record.

## 5. Runtime Controls

During REHEARSAL/SHOW runtime view:

- Current Cue;
- Next Cue;
- GO;
- STOP;
- Jump to Cue with explicit confirmation;
- latest Cue result.

`BACK` may exist only if a deterministic semantic is defined; it must not be implemented as blind reverse execution.

## 6. Acceptance

- A Cue with one OSC Action can be created entirely through UI/API supported workflow.
- GO executes exactly one expected next Cue per accepted command.
- A failed Action does not appear as completed.
- Sequential Actions execute in order.
- Repeated accidental `cue.go` command with same idempotency context must not double-fire silently.
