# 02 — Automated Test Layers & Deterministic Simulation

## Test Pyramid

### Unit Tests

Cover pure rules such as:

- Cue ordering/current-next transition;
- Action execution policies;
- Route condition/transform/debounce logic;
- permission/role checks;
- Snapshot immutability/validation;
- capability parameter validation;
- checksum/manifest comparison;
- retry/idempotency policy.

### Contract Tests

Every stable Command/Event/Plugin/Companion contract has serialization + validation tests. Unknown/incompatible schema versions fail explicitly rather than being guessed.

### Integration Tests

Run real persistence and service boundaries with simulated external adapters:

`API -> Core -> Cue/Route -> capability dispatch -> result -> EventRecord/Session`

Integration tests must include success, rejection, timeout and duplicate-command paths.

### End-to-End Tests

Use real local transport and at least one real OSC receiver plus one real macOS Companion before the MVP is declared complete.

## Simulated Adapter

A first-class simulated capability provider supports deterministic behaviors:

- `COMPLETE_AFTER(ms)`;
- `FAIL(code)`;
- `TIMEOUT`;
- `HANG_UNTIL_CANCELLED`;
- duplicate result attempt;
- delayed/out-of-order result;
- disconnect during execution.

This lets failure behavior be tested without physical devices.

## Deterministic Event Assertions

Tests assert:

- stable execution/correlation relationships;
- required events exist in logical order within an authoritative stream;
- no duplicate ActionExecution for a deduplicated command;
- no semantic success from transport ACK alone;
- no command is generated merely by replaying historical Events.

Global total event ordering is not assumed unless the implementation explicitly provides it.

## Database / Migration Tests

Each schema migration must prove:

- clean database creation;
- migration from the previous supported schema;
- committed Project/Snapshot/Session identities survive;
- failed migration does not leave the database falsely marked healthy.

## Regression Rule

Every production-significant bug gets a reproducible regression test at the lowest practical layer before the fix is considered complete.