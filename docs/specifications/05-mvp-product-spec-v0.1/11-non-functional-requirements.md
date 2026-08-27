# 11 — Non-Functional Requirements

## 1. Local-First

The full MVP runtime path must work without Internet access after installation/configuration. Loss of Internet must not stop Project load, Publish, Rehearsal, Cue execution, routing, or local Session logging.

## 2. Reference Scale

The MVP should be tested with at least:

- 1 active Project;
- 100 Cues;
- 200 Actions total;
- 50 Routes;
- 10 configured targets/aliases;
- 1 active Companion;
- a 2-hour Rehearsal Session.

These are MVP engineering test sizes, not final product limits.

## 3. Latency Targets

On a reference local deployment under normal load:

- accepted P1 command -> internal dispatch request: target p95 <= 50 ms;
- local UI runtime state update after Hub event: target p95 <= 250 ms;
- routing evaluation for simple conditions/transforms: target p95 <= 20 ms before adapter dispatch.

External device/network acknowledgement time is measured separately and is not hidden inside these targets.

These values are test targets, not hard real-time certification. If the selected stack cannot meet them reliably, the stack/design must be revisited.

## 4. Reliability

- P1 runtime work must not wait on backup, report generation, media indexing, AI, or Vision.
- queues used for runtime work are bounded.
- a Plugin/Companion failure must not corrupt Project data.
- restarting StageCore must preserve committed Project configuration and completed Session records.
- runtime must not auto-replay previous non-idempotent Commands after reconnect/restart.

## 5. Persistence & Integrity

- configuration updates use transactional/atomic persistence appropriate to the chosen database;
- Published Runtime Snapshot is immutable;
- stable IDs survive renames;
- execution records maintain correlation IDs;
- secrets are never written to normal logs.

## 6. Usability

For the primary operator, the normal rehearsal loop should require no terminal use and no direct JSON/database editing.

Critical runtime information must remain visible without opening diagnostic pages: mode, snapshot, current/next Cue, latest result, and blocking endpoint state.

## 7. Platform Targets

- Hub: development computer first; Raspberry Pi 5 / Mini-PC class deployment must remain feasible pending technology spike.
- Companion: macOS is the first practical target; architecture must not embed macOS-only semantics into Core contracts.
- UI: modern desktop browser minimum; packaging as desktop app is a later technology choice.

## 8. Testability

Core runtime must support a simulated adapter/target so automated integration tests can execute complete Cue and Route flows without physical equipment.
