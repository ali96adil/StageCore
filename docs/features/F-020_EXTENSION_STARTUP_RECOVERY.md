# F-020 Extension Startup Recovery

Status: implementation slice, software verification only.

This slice applies the bounded recovery primitive to Hub startup restoration of extension runtimes whose persisted desired state is `ENABLED`.

## Scope

`RuntimeSupervisor.ReconcileBounded` wraps the existing startup `Reconcile` behavior with a small retry budget:

- maximum 3 attempts;
- initial backoff 250 ms;
- maximum backoff 1 s;
- caller context remains the cancellation/deadline authority;
- only `ErrRuntimeProbeHandshake` is classified as transient.

The existing reconciliation logic remains authoritative for lifecycle state, generation checks, runtime isolation, integrity verification, and process supervision.

## Fail-closed classification

The following classes do not authorize retry:

- runtime artifact or installed payload integrity failures;
- permission or isolation blockers;
- invalid runtime contracts or configuration;
- lifecycle generation conflicts;
- any mixed reconciliation result containing both transient and permanent failures.

When multiple extensions fail in one pass, every leaf error must be a transient handshake failure before another startup reconciliation attempt is allowed.

## Command safety boundary

This is component-liveness recovery only. It does not replay Cue/Action executions, Stage Device commands, Companion executions, OSC, HTTP, or script actions.

`runtime.recovery.decision` remains fail-closed for command replay; restart or reconnect does not grant replay authority.

## Verification

Automated tests cover:

- transient startup handshake failure followed by successful recovery;
- permanent integrity failure with exactly one attempt;
- mixed transient/permanent failures rejected by the retry classifier.

Physical/product qualification remains deferred under Issue #148.
