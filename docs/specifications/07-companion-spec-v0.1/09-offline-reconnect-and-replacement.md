# 09 — Offline, Reconnect & Replacement

## Disconnect

When the Hub loses the Companion heartbeat/channel:

- Companion state becomes `OFFLINE` after defined timeout;
- assigned Machine Role becomes unavailable/degraded;
- affected Preflight/runtime status is updated;
- Hub does not fabricate completion for in-flight work without a result;
- operator can inspect affected execution/role.

## Companion Behavior Without Hub

MVP Companion has very limited authority while disconnected. It may finish an already accepted bounded local Action if safe/defined, but it does not accept new Project commands from stale cache as though the Hub were present.

Distributed offline show authority/local Node rules are outside this Companion MVP.

## Reconnect

```text
Transport reconnect
 -> authenticate trusted identity
 -> report Companion/version/capabilities
 -> report role + applied Snapshot/config/media state
 -> reconcile in-flight execution IDs
 -> Hub determines READY/MISMATCH/DEGRADED
```

Reconnect never means replay the last command.

## In-Flight Result Ambiguity

If connection drops after dispatch and the final result cannot be proven, StageCore records an explicit unknown/interrupted outcome according to the capability contract. It must not silently mark success or retry non-idempotent work.

## Replacement Machine

Replacement is an explicit operation:

1. pair new Companion;
2. assign existing Machine Role;
3. synchronize config/media;
4. run Preflight;
5. release/revoke old assignment as appropriate;
6. become READY.

Cue/Route definitions remain unchanged because they target logical role/alias.

## Acceptance

- unplug network -> role visibly leaves READY;
- reconnect -> no duplicate previous Action;
- stale Snapshot -> `MISMATCH` until sync;
- new Mac can take same role without editing Cue definitions.