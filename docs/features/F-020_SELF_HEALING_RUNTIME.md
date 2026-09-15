# F-020 — Self-Healing Runtime & Optional High Availability

Status: single-Hub recovery and B6 fault qualification complete; HA-A1 physical-dispatch authority foundation complete; HA-A2 durable witness lease authority in progress. Software qualification only.

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

## Optional HA — HA-A1 delivered

HA-A1 establishes one physical-dispatch authority boundary before any real capability transport.

The authority contract uses:

- `STANDALONE` — preserves current single-Hub behavior;
- `LEADER` — requires explicit holder identity plus a non-zero epoch before physical dispatch may proceed;
- `STANDBY` — rejects physical dispatch before the backend executor is called.

Unknown, malformed, or unavailable authority fails closed.

The same gated physical executor is used by Cue/Action execution and Routing outputs. F-024 SIMULATION is consumed by the Digital Twin before this authority boundary and therefore does not consult HA authority or reach physical transports.

The product remains statically `STANDALONE`; HA-A1 deliberately did not expose a mutable local leader switch.

HA-A1 is SOFTWARE COMPLETE on exact `main` SHA `d15e83b1261a74df58c72024c63b4d3f5b5cd4fb`. Exact-main Core CI #856 / run `34977009301` passed module lock, tests, vet, race tests and Linux ARM64 CGo-free product builds.

See `docs/features/F-020_HA_DISPATCH_AUTHORITY.md`.

## Optional HA — HA-A2 current scope

HA-A2 adds the durable lease state machine for a future external witness authority.

The witness owns one bounded lease containing:

- one holder identity;
- one monotonically increasing fencing epoch;
- one expiry timestamp under witness time.

The intended holder identity is the existing durable `hubsecurity.Identity.HubID`; no second HA node-identity model is introduced.

Rules:

- `Acquire` succeeds only when no active lease exists;
- same-holder acquire is idempotent and never renews implicitly;
- `Renew` requires the exact active holder+epoch;
- expired epochs cannot be revived;
- release preserves the epoch so it can never be reused;
- witness restart preserves the last epoch;
- concurrent acquisition is serialized to one winner;
- the witness, not a Hub request, controls bounded lease duration.

HA-A2 is intentionally persistence/state-machine only. Network transport, authentication, Hub configuration, automatic promotion and failover remain later slices.

See `docs/features/F-020_HA_LEASE_AUTHORITY.md`.

## HA-A2 acceptance gate

CI must prove:

- exactly one active holder may acquire the witness lease;
- expiry removes authority without erasing fencing history;
- stale holder/epoch operations cannot renew or release a newer grant;
- epochs remain monotonic across release, expiry and process restart;
- separate service instances racing on the same witness database produce one winner;
- invalid holder/duration/epoch input fails closed;
- exact-head and exact-main Core CI are green.

Only after HA-A2 passes may StageCore add an authenticated witness transport and Hub-side lease source.

## Explicit HA non-goals through A2

HA-A1/A2 do not yet implement:

- Runtime Snapshot/Session state replication between Hubs;
- authenticated witness network transport;
- peer-based leader election;
- automatic standby promotion;
- automatic command replay after failover;
- downstream device fencing-token propagation;
- complete automatic split-brain failover qualification;
- Raspberry Pi deployment while #148 is active.
