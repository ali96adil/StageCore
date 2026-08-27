# 05 — Runtime Channel & Configuration Sync

## Connection Model

The Companion maintains an authenticated connection to the Hub and reports:

- Companion identity/version;
- assigned Machine Role;
- supported capabilities;
- applied Runtime Snapshot ID;
- configuration revision/hash;
- health/readiness;
- local media manifest state.

## Configuration Separation

StageCore keeps three layers distinct:

1. **Project Configuration** — cues, routes, logical targets and project requirements.
2. **Role Configuration** — settings/media/capabilities required for `VIDEO-MAIN`, etc.
3. **Machine Configuration** — local paths, permissions, app locations and machine-only settings.

The Companion receives only the subset required for its assignment.

## Runtime Sync

Before a role becomes READY:

```text
Hub assigns role
 -> Companion receives required config + Snapshot identity
 -> validates local prerequisites
 -> applies bounded runtime cache
 -> reports applied hashes/versions
 -> Hub compares expected vs actual
 -> READY or MISMATCH/DEGRADED
```

## Runtime Command Rule

Every runtime execution request includes identifiers sufficient to reject stale work, including execution/correlation identity and active Runtime Snapshot identity.

A Companion must reject commands for an incompatible/stale snapshot rather than guessing.

## Cache Rule

Companion cache exists only for resilience/performance. It is not editable authoritative Project state.

## Reconnect

After reconnect, Hub and Companion reconcile state first. The Hub does not resend the last non-idempotent command merely because the transport reconnected.