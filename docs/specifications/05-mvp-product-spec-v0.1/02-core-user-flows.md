# 02 — Core User Flows

## Flow A — First working show-control loop

1. Create Project.
2. Add logical target alias.
3. Configure OSC host/port/address template.
4. Create Cue 1.
5. Add OSC Action.
6. Validate Draft.
7. Publish Snapshot.
8. Start Rehearsal Session.
9. Press GO.
10. Observe `cue.started` -> `action.started` -> result -> `cue.completed|cue.failed`.
11. Open Session Log and inspect execution.

No step may require editing source files or database rows manually.

## Flow B — Input routing

1. Create InputDefinition.
2. Create Route referencing Input.
3. Add condition only if needed; simple equality/range is enough for MVP.
4. Map Route to Cue or OutputDefinition.
5. Use `input.inject_test` or supported real input.
6. Inspect Route Trace.

## Flow C — Edit after publication

1. Operator edits Draft.
2. Current published runtime remains unchanged.
3. UI shows `Unpublished Changes`.
4. Validate creates a validation result.
5. Publish creates a new immutable Snapshot ID/version.
6. Runtime switches only after successful publication/readiness policy.

## Flow D — Device failure

1. Action targets unavailable endpoint.
2. Adapter returns explicit failure/timeout.
3. ActionExecution becomes failed/timed-out.
4. Cue result follows its error policy.
5. Operator sees actionable error.
6. Retry is offered only when action contract permits it.

## Flow E — Reconnect

1. Companion disconnects.
2. Role becomes unavailable/degraded.
3. Companion reconnects and reports identity, version, capabilities, snapshot.
4. Hub reconciles state.
5. Endpoint becomes READY only after required checks pass.
6. Last command is never replayed automatically.

## UX Rule

The operator should always be able to answer four questions from the runtime UI:

- What project/snapshot am I running?
- What Cue is current and what is next?
- What just happened?
- If something failed, what failed and what can I do next?
