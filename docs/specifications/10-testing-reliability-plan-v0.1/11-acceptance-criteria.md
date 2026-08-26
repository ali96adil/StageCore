# 11 — Acceptance Criteria

The Testing & Reliability Plan v0.1 is implementation-ready when the following behaviors can be demonstrated repeatably on the documented reference environments.

## Automated Core

- Cue/Route/Action contract tests cover success/rejection/timeout/duplicate paths.
- Simulated adapter can deterministically complete, fail, timeout, hang and disconnect.
- Database create/migrate/restart tests preserve committed Project/Snapshot/Session identities.
- Permission-denial and trust-revocation tests run without relying on UI hiding controls.

## Local-First Network

- With Hub/Client/Companion on stage LAN, disconnect all Internet/WAN and continue Project load, authentication, Rehearsal, Cue execution, Routing, Notes and Session logging.
- Restoring WAN creates no duplicate runtime command.
- Companion LAN disconnect becomes visible; reconnect reconciles before READY.
- Router/AP loss produces explicit disconnection/degraded state and no false remote success.
- Where Dual-WAN is used, WAN failover does not become a requirement for local runtime.

## Component Failure

- Kill Plugin process: Core remains alive and affected capability becomes unavailable/degraded.
- Kill/disconnect Companion: role state changes and in-flight ambiguity is recorded truthfully.
- Reconnect never automatically replays previous non-idempotent Action.
- Replacement Mac assumes the same Machine Role without Cue/Route edits.

## Storage & Recovery

- Interrupted >=2 GiB transfer resumes and verifies SHA-256 before READY.
- Runtime reserve prevents bulk write from consuming critical persistence capacity.
- Hub process crash preserves committed data and marks interrupted work honestly.
- Controlled hard power-loss test on disposable/reference hardware recovers to a valid or explicitly blocked state without automatic execution.
- Verified backup restores into a clean/staged environment with expected Project, Snapshot and Session history.

## Performance & Soak

On the selected reference hardware/network:

- Route evaluation p95 target <= 20 ms before adapter dispatch;
- accepted P1 command -> internal dispatch p95 target <= 50 ms;
- Hub runtime event -> local operator UI p95 target <= 250 ms;
- 2-hour reference rehearsal completes without unbounded queue/memory growth or progressive runtime failure;
- bulk P3 work does not starve normal P1 Cue execution.

## Security Regression

- untrusted LAN machine cannot issue protected runtime commands;
- revoked session/Companion cannot regain authority with old credentials;
- Plugin permission denial is enforced;
- synthetic secret values do not appear in normal logs/traces/Project export;
- SHOW-mode security administration restrictions and emergency revocation behave as specified.

## First Rehearsal Gate

The checklist in `10-first-rehearsal-qualification.md` passes on the exact build/topology intended for the first real rehearsal, with no unresolved No-Go condition.

## Release Decision

StageCore is rehearsal-ready only when all MUST MVP acceptance criteria plus G0–G6 reliability gates relevant to the implemented scope have evidence. Passing more feature demos does not compensate for a failed integrity, duplicate-execution, authorization or recovery gate.