# F-020 HA-A2 — Durable Witness Lease Authority

Status: implementation in progress — software qualification only.

Physical/product qualification remains deferred under GitHub Issue #148.

## Goal

Provide the durable fencing state machine that a later StageCore HA witness transport can expose to two Hubs.

HA-A1 established one physical-dispatch authority gate inside each Hub. HA-A2 defines the single external authority that can grant one Hub a bounded lease at one monotonically increasing fencing epoch.

This slice deliberately separates fencing correctness from networking, authentication, and promotion policy.

## Authority model

The witness owns exactly one lease record:

- `holder_id` — the Hub identity that currently owns the lease;
- `epoch` — a monotonically increasing fencing generation;
- `expires_at` — witness-clock expiry for the current grant.

The intended future holder identity is the existing durable `hubsecurity.Identity.HubID`. HA-A2 does not create a second node-identity model.

### Acquire

`Acquire(holder)` may grant authority only when no active lease exists.

- If the current active lease belongs to another holder, acquisition is rejected.
- Repeating acquire by the active holder is idempotent and does not extend expiry.
- After release or expiry, the next successful acquisition increments the persisted epoch.
- An old epoch is never reused.

### Renew

`Renew(holder, epoch)` succeeds only when the exact holder and epoch still own an unexpired lease.

- Expired leases cannot be revived.
- Stale holders and stale epochs cannot extend a newer grant.
- Renewal uses the witness-configured duration; the Hub cannot request an arbitrarily long lease.

### Release

`Release(holder, epoch)` may clear only the exact current holder+epoch pair.

The epoch remains durable after release so a later acquisition receives a higher fencing generation.

## Lease duration policy

The witness, not the Hub, controls lease duration.

Current software bounds:

- default: 5 seconds;
- minimum: 1 second;
- maximum: 30 seconds.

These are bounded authority windows, not automatic failover timers. A later promotion policy must still account for in-flight command ambiguity before allowing a standby Hub to become live.

## Persistence

HA-A2 uses a dedicated SQLite database for the witness authority state rather than either Hub's normal StageCore database.

The database uses:

- WAL journaling;
- `synchronous=FULL`;
- immediate write transactions;
- a single durable singleton lease row;
- persisted epochs across process restart.

The witness is the single serialization point. Tests also open two service instances against the same local witness database and prove that simultaneous acquisition yields only one active holder.

## Failure behavior

Safety is preferred over availability.

- persistence/read/write errors return errors rather than manufacturing authority;
- expired state is not authority even though its epoch remains persisted;
- stale holder/epoch operations are rejected;
- invalid holder identity is rejected;
- fencing epoch exhaustion fails closed.

A later Hub-side lease source must translate witness unavailability or expired cached authority into `STANDBY`/unavailable authority at the HA-A1 dispatch gate.

## Explicit non-goals

HA-A2 does **not** implement:

- witness network transport;
- Hub authentication to the witness;
- leader election between Hubs;
- automatic standby promotion;
- Runtime Snapshot/Session replication;
- command replication or replay;
- downstream transport fencing-token propagation;
- physical deployment or qualification.

Those require later slices and must not be inferred merely because the lease state machine exists.

## Software acceptance

HA-A2 is software-acceptable only when CI proves:

1. one active holder at a time;
2. same-holder acquire is idempotent and does not renew implicitly;
3. renew requires the exact active holder+epoch;
4. expired epochs cannot be revived;
5. release does not allow epoch reuse;
6. epoch monotonicity survives witness restart;
7. concurrent acquisition through separate service instances yields exactly one winner;
8. invalid duration/holder/epoch input fails closed;
9. `go test`, `go vet`, race tests and ARM64 product builds remain green.
