# 09 — CI, Test Evidence & Defect Policy

## CI Expectations

Once implementation begins, every pull request that changes runtime behavior should automatically run relevant:

- unit tests;
- contract/schema tests;
- database/migration tests;
- simulated Cue/Route integration tests;
- security denial-path tests where applicable;
- lint/static checks chosen by the implementation stack.

Physical-network/macOS/hard-power tests may run as scheduled/manual gates rather than on every commit.

## Test Categories

Use stable labels/categories so the suite can be selected independently:

- `unit`
- `contract`
- `integration`
- `e2e`
- `fault`
- `performance`
- `soak`
- `security`
- `recovery`
- `field`

## Evidence Required

A reliability gate records:

- StageCore commit/build;
- environment/hardware versions;
- test fixture identity;
- start/end time;
- pass/fail/skipped counts;
- performance measurements where relevant;
- fault injected and expected behavior;
- logs/traces needed to reproduce failures without secrets.

## Flaky Test Policy

A flaky test is a defect, not background noise. A test may be quarantined temporarily only with an issue explaining owner, impact and reproduction history. Critical runtime/security/recovery gates cannot be waived indefinitely because they are inconvenient.

## Severity

### Blocker

Examples: data corruption, unauthorized runtime control, duplicate non-idempotent Action, published Snapshot mutation, false successful execution, failed restore of supported backup.

### High

Examples: required Companion loss not visible, repeated runtime crash, P1 starvation under normal bulk work, secrets in logs, reconnect unable to reconcile without manual database edits.

### Normal

Non-critical UI/diagnostic defects that do not compromise runtime truth, safety boundaries, data integrity or release acceptance.

## Definition of Done

An implementation issue affecting runtime/reliability is Done only when the happy path and relevant failure path have test coverage or a documented repeatable manual test, and the observed behavior matches the current specification.