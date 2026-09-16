# F-020 HA-A5 — Explicit Operator Authority and Same-Epoch Renewal

Status: implementation in progress — software qualification only.

Physical/product qualification remains deferred under GitHub Issue #148. No Raspberry Pi deployment is part of HA-A5.

## Goal

Make optional WITNESS mode operable without turning standby availability into automatic promotion authority.

HA-A5 adds one explicit, OWNER-controlled activation path and automatic maintenance of only the lease epoch that the operator deliberately acquired. It does not add automatic acquisition, automatic standby promotion, state replication, or command replay.

## Safety model

A WITNESS-mode Hub always starts `STANDBY` after process start or restart.

Fresh authority requires an explicit `Activate` operation. Before contacting the witness, StageCore reads canonical Session truth from the Store:

- ACTIVE `SHOW` blocks activation;
- ACTIVE `REHEARSAL` blocks activation;
- `SIMULATION` does not block activation because its execution path remains inside the Digital Twin and never crosses the physical-dispatch authority gate;
- no active operational Session permits activation.

This guard exists because F-020 does not replicate live Session/Cue/Action runtime state between Hubs. Acquiring physical authority while a local physical Session is already active would therefore manufacture continuity that StageCore cannot prove.

## Same-epoch renewal

After successful explicit activation, the supervisor renews only the exact holder and fencing epoch returned by the witness.

Renewal is liveness maintenance, not promotion:

- the supervisor never calls `Acquire` from its renewal loop;
- any renewal error immediately invalidates local physical authority;
- lease expiry immediately invalidates local physical authority;
- after renewal failure or expiry, the Hub remains `STANDBY` until another explicit operator activation;
- a process restart never restores a previous lease automatically.

The renewal wait and request timeout are derived from the locally conservative remaining lease interval supplied by the HA-A4 controller.

## Renewal-generation fencing

An older renewal goroutine must never demote or overwrite a later explicit activation.

The supervisor therefore owns a monotonically increasing local renewal generation. Activation, release, demotion, and shutdown advance that generation. A delayed renewal result may change authority only if it still belongs to the current generation.

The final fail-closed demotion step is serialized with explicit authority operations so an old renewal loop cannot pass a generation check and then demote a newly activated lease.

## Release, demotion, and shutdown

`Release` stops renewal and removes local physical authority before waiting for the witness release request. A witness release failure cannot restore local `LEADER` authority.

`Demote` is an emergency local fence:

- it stops renewal;
- immediately changes local authority to `STANDBY`;
- performs no witness network I/O;
- leaves any unreachable remote lease to expire naturally at the witness.

Hub shutdown is also local-first and non-blocking with respect to the witness. Physical authority is fenced before StageCore tears down device/Companion/plugin transports or closes persistence. A restarted Hub begins `STANDBY` and requires fresh explicit activation.

## Operator authorization

HA management uses the dedicated permission:

`ha.manage`

Only `OWNER` receives this permission. It is intentionally separate from `runtime.control`; an `OPERATOR` may operate an already-authorized show but may not promote or demote the Hub itself.

WITNESS mode registers these authenticated Operator endpoints:

- `GET /api/v1/ha/authority`
- `POST /api/v1/ha/authority/activate`
- `POST /api/v1/ha/authority/release`
- `POST /api/v1/ha/authority/demote`

STANDALONE mode has no HA authority routes because no supervised HA authority surface exists.

Unsafe mutations require the existing browser CSRF protection. Successful and rejected mutations write Security Audit events:

- `ha.authority.activate`
- `ha.authority.release`
- `ha.authority.demote`

Audit metadata records the local authority mode, holder, epoch, remaining lease interval, and whether renewal is active.

## Explicit non-goals

HA-A5 does not implement:

- automatic standby promotion;
- automatic lease acquisition after startup, restart, expiry, or network recovery;
- Runtime Snapshot, Session, Cue, Action, or Command state replication between Hubs;
- transparent continuation of an interrupted SHOW or REHEARSAL on another Hub;
- command replay after failover;
- downstream external-device fencing-token propagation;
- quorum/consensus between multiple witnesses;
- physical deployment or qualification.

## Software acceptance

HA-A5 is software-acceptable only when CI proves:

1. WITNESS startup remains `STANDBY` and performs no acquire;
2. ACTIVE SHOW and REHEARSAL block activation before witness I/O;
3. SIMULATION does not create a false physical-session block;
4. activation is explicit and starts renewal only after a valid local `LEADER` grant is installed;
5. renewal failure/expiry demotes locally and never reacquires automatically;
6. stale renewal generations cannot demote a later explicit activation;
7. release and emergency demotion remain fail-closed;
8. shutdown fences locally without depending on witness reachability;
9. `ha.manage` is OWNER-only;
10. Operator mutations preserve browser CSRF and Security Audit behavior;
11. STANDALONE exposes no HA authority control route;
12. exact-head and exact-main module-lock, tests, vet, race, and Linux ARM64 product-build gates pass.
