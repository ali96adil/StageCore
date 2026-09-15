# F-020 — Self-Healing Runtime & Optional High Availability

Status: single-Hub recovery implementation and B6 fault qualification complete; optional HA dispatch-authority foundation in progress. Software qualification only.

Physical/product qualification remains deferred under GitHub Issue #148. This feature must not be described as physically qualified until the cumulative hardware campaign passes.

## Goal

Make StageCore recover from bounded, non-safety-critical runtime faults without turning restart or reconnect into implicit permission to replay live commands.

F-020 is intentionally staged. Safe single-Hub recovery semantics are proven first. Optional High Availability proceeds only by adding explicit authority/fencing layers that preserve those semantics.

## Authority boundaries

Recovery may use only canonical persisted truth:

- Session type, status and lifecycle state;
- immutable Runtime Snapshot configuration;
- persisted Command Envelope identity/idempotency state;
- persisted Cue/Action execution state;
- explicit checkpoints and runtime observations where their contracts permit it.

Recovery must not infer that a device executed a command merely because it reconnects. It must not promote observed connectivity into command acknowledgement. It must not bypass SHOW safety, pairing/trust, Runtime Snapshot immutability, RBAC, or Flight Recorder evidence.

## Restart policy vocabulary

### Dispositions

- `PRESERVE` — leave the active Session intact because continuity is proven safe by the current narrow policy.
- `ABORT` — fail closed and terminate interrupted runtime work.
- `MANUAL_CONFIRMATION_REQUIRED` — terminate the interrupted Session but expose a trusted reconstruction reference that an operator may explicitly use to create a new Session.

### Replay rule

`replay_allowed=false` for all restart decisions.

A Hub restart, component restart, reconnect, timeout, checkpoint discovery, HA role change, or future failover event never authorizes replay of a prior live command by itself. Bounded retry remains restricted to explicitly classified component-liveness work and is not a generic Cue/Action retry wrapper.

### Current automatic preservation rule

Only an `ACTIVE` `REHEARSAL` Session with exactly one valid `TIMECODE_SOURCE` configured as `INTERNAL` and with no RUNNING Cue/Action execution may remain active across Hub restart.

The following remain fail-closed:

- every `SHOW` Session;
- every interrupted SIMULATION Session as an old runtime authority;
- external, missing, invalid or ambiguous timecode authority;
- any otherwise-preservable rehearsal with in-flight Cue/Action work;
- unsupported or non-active runtime shapes.

An interrupted SIMULATION may expose a trusted F-024 checkpoint as `MANUAL_CONFIRMATION_REQUIRED`, but the old Session is still `ABORTED`. Reconstruction requires an explicit new `CHECKPOINT` SIMULATION Session. Without a valid checkpoint, restoration is `UNAVAILABLE`.

Interrupted RUNNING Action executions remain `CANCELLED` with `HUB_RESTART_INTERRUPTED`; RUNNING Cue executions remain `CANCELLED`; their interrupted Session does not gain replay authority.

## Canonical evidence

Every active Session inspected during Hub restart emits a `runtime.recovery.decision` Flight Recorder event in the same database transaction as reconciliation.

The event records:

- decision version and `SESSION_RESTART` scope;
- Session type and lifecycle state;
- `PRESERVE` / `ABORT` / `MANUAL_CONFIRMATION_REQUIRED` disposition;
- stable reason code;
- whether the decision is automatic;
- `replay_allowed`;
- whether manual confirmation is required;
- immutable-snapshot timecode authority classification;
- whether in-flight runtime work was present;
- trusted checkpoint identity/version/hash and new-Session reconstruction requirement when applicable.

If decision evidence cannot be persisted, the reconciliation transaction fails instead of silently changing runtime state without an audit trail.

## Delivered single-Hub slices

1. Restart recovery policy vocabulary, immutable-Snapshot authority classification, and atomic Flight Recorder evidence.
2. Bounded extension startup recovery with explicit transient/permanent classification.
3. Bounded extension crash recovery with attempt budget, generation fencing, desired-state authority, and cancellation.
4. Companion disconnect/reconnect presence recovery without treating connectivity as command acknowledgement.
5. Stage Device stale-command terminalization without reconnect replay.
6. Checkpoint-aware SIMULATION reconstruction with SHA-256 integrity, state-contract/version checks, immutable Runtime Snapshot binding, newest-valid fallback, and explicit manual confirmation/new-Session semantics.
7. B6 F-024 fault-driven cross-layer qualification for disconnect, timeout, reconnect and fail-closed fault outcomes.

B6 is SOFTWARE COMPLETE on exact `main` SHA `323771bd665d9abb4da9ab9cef05c0aff710e7f7`. Exact-main Core CI #847 / run `34939096619` passed module lock, tests, vet, race tests and Linux ARM64 CGo-free product builds.

## Optional HA — current gate

HA-A1 establishes one physical-dispatch authority boundary before any real capability transport.

The authority contract uses:

- `STANDALONE` — preserves current single-Hub behavior;
- `LEADER` — requires explicit holder identity plus a non-zero epoch before physical dispatch may proceed;
- `STANDBY` — rejects physical dispatch before the backend executor is called.

Unknown, malformed, or unavailable authority fails closed.

The same gated physical executor is used by Cue/Action execution and Routing outputs. F-024 SIMULATION is consumed by the Digital Twin before this authority boundary and therefore does not consult HA authority or reach physical transports.

The product remains statically `STANDALONE` during HA-A1. A later slice must provide a real cross-node authority/lease source before `LEADER` or `STANDBY` becomes a production configuration.

See `docs/features/F-020_HA_DISPATCH_AUTHORITY.md`.

## HA-A1 acceptance gate

CI must prove:

- standalone product behavior remains unchanged;
- standby cannot invoke a physical capability backend;
- leader-shaped authority is rejected without holder identity and non-zero epoch;
- authority lookup failures and unknown modes fail closed;
- SIMULATION never consults physical-dispatch authority;
- REHEARSAL and SHOW honor the authority gate;
- exact-head and exact-main Core CI are green.

Only after HA-A1 passes may StageCore add cross-node lease/epoch ownership, renewal/expiry, standby promotion rules, and later failover qualification.

## Explicit HA non-goals for A1

HA-A1 does not implement:

- cross-node state replication;
- leader election;
- lease acquisition or renewal;
- automatic standby promotion;
- automatic command replay after failover;
- external-device fencing-token propagation;
- complete split-brain protection;
- Raspberry Pi deployment while #148 is active.
