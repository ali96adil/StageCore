# F-020 — Bounded Component Recovery

Status: implementation in progress; software verification only.

This document defines the retry boundary introduced after the F-020 restart-policy foundation. It does **not** authorize Cue or Action replay.

## Safety rule

A reconnect, process restart, Hub restart, timeout, or transient component failure does not by itself make a previously issued live command safe to execute again.

`runtime.recovery.decision` remains authoritative for restart decisions and currently records `replay_allowed=false`.

The bounded recovery runner is for component-recovery operations such as:

- restarting an enabled extension/plugin runtime;
- retrying a component readiness/probe handshake when the failure is explicitly classified transient;
- reconnect/repair operations that do not themselves emit a show-control command.

It is **not** a generic capability retry wrapper.

## Boundaries

- Maximum attempts are hard-capped by `MaxBoundedRecoveryAttempts`.
- Backoff is exponential and hard-capped.
- The caller context is the recovery authority deadline/cancellation boundary.
- Every caller must provide an explicit `shouldRetry(error)` classifier.
- Permanent integrity, trust, permission, configuration, snapshot mismatch, or other structural failures must fail fast.
- Command timeouts, interrupted executions, or unknown completion remain ambiguous and are not replayed by this primitive.
- Recovery of Stage Device and Companion transport must preserve their existing idempotency/deduplication contracts rather than generating a second logical live command.

## Delivery order

1. Reusable bounded recovery primitive and deterministic tests.
2. Apply it to narrowly classified extension/runtime startup and crash recovery.
3. Add reconnect recovery for Companion/Stage Device health without command replay.
4. Add checkpoint-aware state reconstruction and explicit manual-confirmation cases.
5. Exercise all recovery policies through F-024 fault scenarios.
6. Consider optional HA/leader fencing only after single-Hub recovery semantics are accepted.

Physical/product qualification remains deferred under #148. No Pi deployment is required for this foundation slice.
