# F-020 — Self-Healing Runtime & Optional High Availability

Status: single-Hub recovery and B6 fault qualification complete; HA-A1 through HA-A4b complete; HA-A5 explicit operator authority supervision in progress. Software qualification only.

Physical/product qualification remains deferred under GitHub Issue #148. This feature must not be described as physically qualified until the cumulative hardware campaign passes.

## Goal

Make StageCore recover from bounded, non-safety-critical runtime faults without turning restart, reconnect, lease expiry, or HA role changes into implicit permission to replay live commands.

F-020 is intentionally staged. Safe single-Hub recovery semantics are proven first. Optional High Availability adds explicit authority/fencing layers without changing those replay and Session-truth rules.

## Authority boundaries

Recovery may use only canonical persisted truth:

- Session type, status and lifecycle state;
- immutable Runtime Snapshot configuration;
- persisted Command Envelope identity/idempotency state;
- persisted Cue/Action execution state;
- explicit checkpoints and runtime observations where their contracts permit it.

Recovery must not infer that a device executed a command merely because it reconnects. It must not promote observed connectivity or witness reachability into command acknowledgement. It must not bypass SHOW safety, pairing/trust, Runtime Snapshot immutability, RBAC, or Flight Recorder evidence.

## Restart policy vocabulary

### Dispositions

- `PRESERVE` — leave the active Session intact because continuity is proven safe by the current narrow policy.
- `ABORT` — fail closed and terminate interrupted runtime work.
- `MANUAL_CONFIRMATION_REQUIRED` — terminate the interrupted Session but expose a trusted reconstruction reference that an operator may explicitly use to create a new Session.

### Replay rule

`replay_allowed=false` for all restart decisions.

A Hub restart, component restart, reconnect, timeout, checkpoint discovery, HA role change, lease renewal, expiry, demotion, or future failover event never authorizes replay of a prior live command by itself. Bounded retry remains restricted to explicitly classified component-liveness work and is not a generic Cue/Action retry wrapper.

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

The event records the disposition/reason, whether the decision is automatic, `replay_allowed`, manual-confirmation state, immutable-snapshot timecode authority, in-flight work, and trusted checkpoint reconstruction evidence when applicable.

If decision evidence cannot be persisted, reconciliation fails instead of silently changing runtime state without an audit trail.

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

HA-A1 established one physical-dispatch authority boundary before any real capability transport.

The authority contract uses `STANDALONE`, `LEADER`, and `STANDBY`. Unknown, malformed, or unavailable authority fails closed. The same gated physical executor is used by Cue/Action execution and Routing outputs, while F-024 SIMULATION remains inside the Digital Twin and never reaches the physical gate.

HA-A1 is SOFTWARE COMPLETE on exact `main` SHA `d15e83b1261a74df58c72024c63b4d3f5b5cd4fb`. Exact-main Core CI #856 / run `34977009301` passed module lock, tests, vet, race tests and Linux ARM64 CGo-free product builds.

See `docs/features/F-020_HA_DISPATCH_AUTHORITY.md`.

## Optional HA — HA-A2 delivered

HA-A2 added the durable lease state machine for an external witness authority:

- one holder identity;
- one monotonically increasing fencing epoch;
- one expiry timestamp under witness time;
- exact holder+epoch renewal/release;
- no epoch reuse after release, expiry, or process restart;
- one serialized winner under concurrent acquisition;
- witness-owned bounded lease duration.

HA-A2 is SOFTWARE COMPLETE on exact `main` SHA `b42f96e0719cb770733677de1ad49ca85929649b`. Exact-main Core CI #858 / run `34980595236` passed module lock, tests, vet, race tests and Linux ARM64 CGo-free product builds.

See `docs/features/F-020_HA_LEASE_AUTHORITY.md`.

## Optional HA — HA-A3 delivered

HA-A3 added the authenticated witness network boundary:

- persistent Ed25519 witness transport identity;
- TLS 1.3 mutual authentication;
- explicit `hub` and `witness` StageCore certificate roles;
- pinned SHA-256 fingerprints of raw Ed25519 public keys;
- witness allowlist mapping durable Hub IDs to pinned Hub fingerprints;
- Hub client pinning the expected witness ID and fingerprint;
- current/acquire/renew/release lease API;
- holder identity derived exclusively from the authenticated Hub certificate;
- conservative local lease deadlines derived from witness remaining interval rather than synchronized clocks;
- optional `stagecore-ha-witness` product binary included in the ARM64 build gate.

HA-A3 is SOFTWARE COMPLETE on exact `main` SHA `b4126576d09c845c80bc32649d2f6f198f512565`. Exact-main Core CI #866 / run `35011279569` passed module lock, tests, vet, race tests and Linux ARM64 CGo-free product builds.

See `docs/features/F-020_HA_WITNESS_TRANSPORT.md`.

## Optional HA — HA-A4a delivered

HA-A4a added the lease-backed Hub authority controller that translates authenticated witness truth into the HA-A1 dispatch source.

Safety properties include:

- startup `STANDBY`;
- explicit `Acquire` only;
- `Renew` restricted to the exact current holder+epoch;
- any renewal failure invalidates local authority;
- release/demotion removes local authority before network completion;
- controller-generation fencing prevents delayed Acquire/Renew results from overwriting later demotion/release;
- conservative monotonic and wall-clock bounds fail closed on expiry, suspend, or unsafe clock rollback;
- malformed holder/epoch/lease observations fail closed.

HA-A4a is SOFTWARE COMPLETE on exact `main` SHA `ca0f3c439621ea7a23278db70cc5608b6a864c74`. Exact-main Core CI #869 / run `35024125162` passed module lock, tests, vet, race tests and Linux ARM64 CGo-free product builds.

## Optional HA — HA-A4b delivered

HA-A4b wired optional WITNESS mode into the Hub product while preserving `STANDALONE` as the default.

WITNESS configuration requires an HTTPS witness URL plus pinned witness identity/fingerprint. The Hub reuses its durable Ed25519 Hub identity for mTLS. Startup performs no lease acquisition and begins `STANDBY`. The same HA-A1 gate continues to protect Cue/Action and Routing outputs before real transports.

HA-A4b is SOFTWARE COMPLETE on exact `main` SHA `0c7b4894b377daa7eded50b0c8291f7903d415ec`. Exact-main Core CI #871 / run `35040664600` passed module lock, tests, vet, race tests and Linux ARM64 CGo-free product builds.

## Optional HA — HA-A5 current scope

HA-A5 makes WITNESS mode deliberately operable without adding automatic failover.

Fresh physical authority remains an explicit OWNER action. Before witness acquisition, canonical Store truth blocks activation whenever an ACTIVE `SHOW` or `REHEARSAL` exists on this Hub. `SIMULATION` remains eligible because it never crosses the physical-output boundary.

After explicit acquisition, a supervisor renews only that same holder+epoch. It never reacquires automatically. Renewal failure, expiry, emergency demotion, shutdown, or process restart returns the Hub to local `STANDBY`; another explicit activation is required.

Supervisor-generation fencing prevents a delayed old renewal result from demoting a later explicit activation.

HA management uses the dedicated OWNER-only permission `ha.manage`, distinct from `runtime.control`. WITNESS mode exposes authenticated status/activate/release/demote endpoints with normal browser CSRF protections and Security Audit events. STANDALONE registers no HA authority control route.

See `docs/features/F-020_HA_OPERATOR_AUTHORITY.md`.

## HA-A5 acceptance gate

CI must prove:

- WITNESS startup remains `STANDBY` and performs no acquire;
- ACTIVE SHOW/REHEARSAL block activation before witness I/O;
- SIMULATION does not create a false physical-session block;
- activation is explicit and starts renewal only after a safe LEADER grant;
- renewal failure/expiry demotes locally and never reacquires;
- stale renewal generations cannot affect a later activation;
- release, emergency demotion, and shutdown remain fail-closed;
- `ha.manage` is OWNER-only;
- unsafe Operator mutations preserve CSRF and Security Audit behavior;
- STANDALONE exposes no HA authority route;
- exact-head and exact-main Core CI are green.

## Explicit HA non-goals through A5

F-020 optional HA still does **not** implement:

- automatic standby promotion;
- automatic lease acquisition after startup, restart, expiry, or network recovery;
- Runtime Snapshot/Session/Cue/Action/Command state replication between Hubs;
- transparent continuation of an interrupted SHOW or REHEARSAL on another Hub;
- automatic command replay after failover;
- downstream external-device fencing-token propagation;
- quorum/consensus between multiple witnesses;
- physical deployment or qualification while #148 is active.

These omissions are deliberate safety boundaries, not implied future authority. Optional HA may remain operator-controlled unless a later feature explicitly introduces and qualifies replicated runtime state and automatic promotion semantics.
