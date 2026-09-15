# F-020 HA-A4a — Lease-Backed Dispatch Authority Source

Status: software qualification in progress.

Physical/product qualification remains deferred under GitHub Issue #148. No Raspberry Pi deployment is part of this slice.

## Goal

Connect the authenticated HA-A3 witness lease contract to the HA-A1 `dispatchauthority.Source` boundary without yet enabling HA in the StageCore Hub product.

HA-A4a is deliberately a controller/source slice. It does not add Hub configuration, automatic standby promotion, state replication, or command replay.

## Authority model

`internal/haauthority.Controller` starts with no local grant and therefore reports `STANDBY`.

Authority can become `LEADER` only after an explicit `Acquire` call returns a valid authenticated witness observation for this Hub. The controller never acquires a lease from `Current()` and never treats witness reconnect as promotion authority.

`Current()` performs no network I/O. Every physical dispatch therefore consumes only a bounded local authority snapshot derived from the last authenticated witness response.

A process restart loses all local lease authority and starts `STANDBY`; persisted witness state is not enough to make the restarted Hub a leader.

## Explicit operations

### Acquire

`Acquire` asks the authenticated witness client for a lease only when a caller explicitly invokes it.

The returned observation must:

- belong to the local durable Hub identity;
- contain a non-zero fencing epoch;
- be active;
- contain complete witness/request timing evidence;
- still have positive witness-owned remaining authority.

Otherwise local authority remains `STANDBY`.

### Renew

`Renew` is permitted only for the exact locally authoritative epoch. The witness must return the same epoch.

Any renewal error, mismatched epoch, invalid observation, or superseded operation removes local dispatch authority fail-closed.

### Release

`Release` removes local authority **before** waiting for the witness network request. A network failure therefore cannot leave the Hub able to emit physical output.

### Demote

`Demote` is a local emergency/fail-closed operation. It does not wait for witness I/O and does not attempt promotion, reacquisition, or replay.

## Local generation fencing

Acquire/Renew network calls can outlive a local demotion decision. HA-A4a uses a monotonically increasing local controller generation to fence those responses.

A Demote or Release advances the local generation immediately and clears the grant. Any delayed Acquire/Renew response captured under an earlier generation is rejected with `ErrAuthoritySuperseded` and cannot reinstall `LEADER`.

This is local operation fencing; it does not replace the witness fencing epoch.

## Lease-expiry safety

The HA-A3 client converts witness expiry into a conservative local remaining interval. HA-A4a then checks that interval at dispatch time using both monotonic and wall-clock evidence.

This dual check exists because a monotonic clock may not include time spent suspended on some systems while the witness lease continues expiring.

The controller fails closed to `STANDBY` when:

- monotonic elapsed time reaches the lease interval;
- wall-clock elapsed time reaches the lease interval, including a suspend-like forward jump;
- wall time moves backwards far enough to make authority timing uncertain;
- timing evidence is negative, incomplete, or otherwise invalid.

Small clock-read/scheduler differences are tolerated, but clock movement is never used to extend lease authority.

## Dispatch semantics

HA-A4a implements the existing `dispatchauthority.Source` contract:

- valid local lease -> `LEADER` with exact Hub holder ID and witness fencing epoch;
- no valid local lease -> `STANDBY`;
- unavailable/malformed controller -> authority error, which the HA-A1 gate fails closed.

The controller does not call devices, Companion, OSC, HTTP, scripts, or Routing directly. The existing HA-A1 physical-dispatch gate remains the single enforcement point.

## Acceptance gate

CI must prove:

1. a new controller starts `STANDBY`;
2. `Current()` performs zero witness/network calls;
3. explicit Acquire installs only a valid local Hub lease;
4. Renew uses the exact current fencing epoch;
5. renewal failure removes local authority;
6. Release removes local authority even when witness release fails;
7. lease expiry removes authority;
8. suspend-like wall-clock advancement removes authority even if monotonic elapsed time is shorter;
9. significant wall-clock rollback removes authority;
10. Demote immediately fences an in-flight Renew response;
11. Demote immediately fences an in-flight Acquire response;
12. Release demotes before a blocked Renew finishes and the stale Renew cannot restore authority;
13. module lock, tests, vet, race tests and ARM64 product builds remain green on exact head and exact main.

## Non-goals

HA-A4a does not implement:

- `app.Open()` HA wiring;
- Hub HA configuration or Operator/API controls;
- automatic lease acquisition on startup;
- automatic standby promotion after witness reconnect;
- cross-Hub Runtime Snapshot/Session replication;
- command replication or replay;
- downstream device fencing-token propagation;
- physical deployment or qualification.

Those remain later slices and must not be inferred from the presence of a lease-backed authority source.
