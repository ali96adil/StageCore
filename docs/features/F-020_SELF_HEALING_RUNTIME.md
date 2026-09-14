# F-020 — Self-Healing Runtime & Optional High Availability

Status: implementation in progress — software qualification only.

Physical/product qualification remains deferred under GitHub Issue #148. This feature must not be described as physically qualified until the cumulative hardware campaign passes.

## Goal

Make StageCore recover from bounded, non-safety-critical runtime faults without turning restart or reconnect into implicit permission to replay live commands.

F-020 is intentionally staged. Safe single-Hub recovery semantics come first. Optional High Availability and leader fencing come only after single-node behavior is proven through F-024 simulation and CI.

## Authority boundaries

Recovery may use only canonical persisted truth:

- Session type, status and lifecycle state;
- immutable Runtime Snapshot configuration;
- persisted Command Envelope identity/idempotency state;
- persisted Cue/Action execution state;
- explicit checkpoints and runtime observations where their contracts permit it.

Recovery must not infer that a device executed a command merely because it reconnects. It must not promote observed connectivity into command acknowledgement. It must not bypass SHOW safety, pairing/trust, Runtime Snapshot immutability, RBAC, or Flight Recorder evidence.

## Slice A — Restart policy vocabulary and evidence

The first slice formalizes the existing Hub restart behavior without expanding automatic recovery.

### Dispositions

- `PRESERVE` — leave the active Session intact because continuity is proven safe by the current narrow policy.
- `ABORT` — fail closed and terminate interrupted runtime work.
- `MANUAL_CONFIRMATION_REQUIRED` — reserved vocabulary for later slices where persisted state is sufficient to offer recovery but not sufficient to act automatically.

### Replay rule

`replay_allowed=false` for all Slice A restart decisions.

A Hub restart or component reconnect never authorizes replay of a prior command. Later bounded retry/reconnect work must separately prove command freshness, idempotency and target-specific safety before any retry is allowed.

### Current automatic preservation rule

Only an `ACTIVE` `REHEARSAL` Session with exactly one valid `TIMECODE_SOURCE` configured as `INTERNAL` and with no RUNNING Cue/Action execution is preserved across Hub restart.

The following remain fail-closed and are aborted exactly as before F-020:

- `SHOW` Sessions;
- `SIMULATION` Sessions;
- external, missing, invalid or ambiguous timecode authority;
- any otherwise-preservable rehearsal with in-flight Cue/Action work;
- unsupported or non-active runtime shapes.

Interrupted RUNNING Action executions remain `CANCELLED` with `HUB_RESTART_INTERRUPTED`; RUNNING Cue executions remain `CANCELLED`; the Session becomes `ABORTED`.

### Canonical evidence

Every active Session inspected during Hub restart emits a `runtime.recovery.decision` Flight Recorder event in the same database transaction as reconciliation.

The event records:

- decision version and `SESSION_RESTART` scope;
- Session type and lifecycle state;
- `PRESERVE` / `ABORT` disposition;
- stable reason code;
- whether the decision is automatic;
- `replay_allowed`;
- whether manual confirmation is required;
- immutable-snapshot timecode authority classification;
- whether in-flight runtime work was present.

If decision evidence cannot be persisted, the reconciliation transaction fails instead of silently changing runtime state without an audit trail.

## Planned later slices

1. Safe reconnect recovery for bounded non-safety-critical components and targets.
2. Bounded retry/backoff honoring command deadlines, idempotency and expiry.
3. Checkpoint-aware reconstruction with explicit manual-confirmation cases.
4. F-024 fault-driven validation for disconnect, timeout, reconnect and partial-failure cases.
5. Recovery health/preflight/diagnostics/post-show surfaces.
6. Optional standby Hub / HA with explicit leader ownership and fencing only after single-Hub semantics are proven.

## Non-goals for Slice A

Slice A does not:

- replay or retry any command;
- reconnect or restart plugins/devices automatically;
- preserve SHOW across Hub restart;
- recover in-flight Cue or Action executions;
- introduce a second runtime state model;
- implement HA, elections or leader fencing;
- deploy to the Raspberry Pi while #148 is active.
