# 04 — Companion, Plugin & Device Failure Injection

## Companion Failures

Required tests:

- Companion process exits while idle;
- Companion process exits during an Action;
- network disconnect during accepted Action;
- stale Runtime Snapshot after reconnect;
- incompatible Companion API/version;
- required local permission/capability disappears;
- replacement Mac takes the same Machine Role.

Expected:

- role state becomes explicit `OFFLINE/DEGRADED/MISMATCH/BLOCKED` as appropriate;
- in-flight result becomes failed/timed-out/unknown according to what can be proven;
- Hub does not fabricate completion;
- reconnect reconciles before READY;
- previous non-idempotent Action is never automatically replayed;
- replacement does not require Cue/Route edits.

## Plugin Failures

The reference OSC Plugin and simulated Plugin host must be tested for:

- clean process crash;
- startup failure;
- execution timeout/hang;
- malformed result;
- capability registration mismatch;
- permission denied;
- incompatible Plugin API version;
- disable/uninstall while Project references remain.

Expected:

- Critical Core remains alive;
- affected capability becomes unavailable/degraded;
- existing Project references remain visible, not deleted;
- Preflight blocks required missing capability;
- bounded restart/supervision cannot create an infinite hot crash loop that starves runtime.

## External Device Failures

For OSC/UDP, tests must distinguish:

- syntactically invalid endpoint/configuration;
- local socket/send failure;
- packet sent but no device verification available.

`TRANSPORT_ONLY` success must never be presented as verified device state.

For future acknowledged protocols, adapter tests must prove the strongest acknowledgement level they claim.

## Failure Matrix Evidence

Each injected fault records:

- trigger method/time;
- affected component;
- expected state/result;
- actual operator-visible state;
- correlation/execution IDs;
- recovery steps and recovery duration;
- whether any duplicate execution occurred.

A duplicate non-idempotent Action is a release-blocking reliability defect.