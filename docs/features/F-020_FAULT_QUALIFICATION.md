# F-020 — Simulation Fault Qualification

Status: B6 software qualification in progress.

Base: `483a4f6434f3a516e615822b3c5a8b551f3a5a21`.

Physical/product qualification remains deferred under GitHub Issue #148. This matrix is a software/CI acceptance gate and does not claim Raspberry Pi, network, device, or venue qualification.

## Purpose

B6 proves the already-delivered F-020 single-Hub recovery boundaries against deterministic F-024 fault scenarios. It does not introduce a second recovery coordinator and it does not make reconnect or restart an implicit authorization to replay a show-control command.

The invariant under every case is:

`replay_allowed=false`

A reconnect may restore component or virtual-target liveness. Historical Cue/Action/transport work is never recreated merely because liveness returns.

## Qualification matrix

| Fault / condition | Recovery authority | Required result |
| --- | --- | --- |
| Extension startup handshake interruption | Existing bounded extension recovery | Retry only the transient component probe within the existing 3-attempt bounded budget; no Cue/Action replay |
| Extension integrity/configuration/permanent failure | Existing fail-fast classifier | Stop immediately; no retry authorization from a transient sibling error |
| Extension runtime crash | Existing crash supervisor | Bounded restart within generation/desired-state fencing; disable/close cancels recovery; restart-storm budget remains capped |
| Companion disconnect/reconnect | Existing authenticated Companion presence path | Presence/role truth may recover from a fresh authenticated report; connectivity is not command acknowledgement |
| Stage Device stale/ambiguous command after reconnect | Existing stale-command reconciliation | Terminalize stale work truthfully; never resend the original live command automatically |
| F-024 `DISCONNECT` followed by checkpoint and Hub restart | F-020 restart policy + F-024 checkpoint | Old SIMULATION ends `ABORTED`; trusted checkpoint becomes `MANUAL_CONFIRMATION_REQUIRED`; a new CHECKPOINT Session is required; no replay and no physical dispatch |
| Explicit F-024 `RECONNECT` after checkpoint reconstruction | New SIMULATION Session only | Execute only the newly requested next Cue; restored virtual target may become online; historical Cue/Action executions remain unchanged |
| F-024 `FAIL` without checkpoint | Restart policy | Old SIMULATION ends `ABORTED`, restoration `UNAVAILABLE`, no replay |
| F-024 `REJECT` without checkpoint | Restart policy | Old SIMULATION ends `ABORTED`, restoration `UNAVAILABLE`, no replay |
| F-024 `OFFLINE` without checkpoint | Restart policy | Old SIMULATION ends `ABORTED`, restoration `UNAVAILABLE`, no replay |
| F-024 `UNAVAILABLE` without checkpoint | Restart policy | Old SIMULATION ends `ABORTED`, restoration `UNAVAILABLE`, no replay |
| F-024 `TIMEOUT` with a real action deadline and no checkpoint | Restart policy | Cue/action reaches a terminal timeout result; restart does not retry or replay it; restoration `UNAVAILABLE` |
| SHOW interrupted by Hub restart | Existing fail-closed SHOW rule | `ABORT`; never auto-preserve or replay |
| Clean INTERNAL-timecode REHEARSAL with no in-flight work | Existing narrow continuity rule | Preserve Session continuity only; `replay_allowed=false` |
| External/ambiguous timecode or in-flight REHEARSAL | Existing fail-closed rule | Abort/suspend according to the established Session contract; no replay |

## New B6 cross-layer tests

`internal/simulationcontrol/f020_recovery_fault_qualification_test.go` adds two acceptance paths:

1. `DISCONNECT -> failed Cue -> checkpoint -> Hub restart -> old Session ABORTED/manual -> fresh Digital Twin -> new CHECKPOINT Session -> explicit RECONNECT -> next Cue only`.
2. `FAIL / REJECT / OFFLINE / UNAVAILABLE / TIMEOUT -> terminal Cue outcome -> Hub restart -> ABORTED + UNAVAILABLE -> no new execution`.

Both paths use the normal Cue Engine, canonical Session authority, F-024 Digital Twin, Store checkpoint/restart reconciliation, Flight Recorder evidence, and a wrapped physical executor probe. Any physical call is a test failure.

## Existing evidence reused by B6

B6 intentionally does not duplicate lower-level tests that already prove:

- deterministic F-024 fault matching and Session isolation;
- one-shot failure consumption;
- persistent disconnect until explicit reconnect;
- timeout behavior under caller deadline;
- bounded extension startup retries;
- extension integrity fail-fast behavior;
- automatic crash-recovery budget/fencing/cancellation;
- Companion reconnect presence fencing;
- Stage Device stale-command terminalization;
- checkpoint SHA-256/version/Runtime Snapshot authority checks.

The B6 tests connect those established primitives across the real software layers instead of creating alternative fault or recovery models.

## Exit gate

B6 is accepted only when:

- exact-head Core CI passes module lock, tests, vet, race, and Linux ARM64 CGo-free product builds;
- no production recovery behavior was loosened to make the qualification pass;
- all recovery-decision evidence remains explicit and replay-free;
- the wrapped physical executor remains untouched in SIMULATION;
- the merged exact-main SHA passes Core CI again.

Only after this gate may Phase 5 proceed to Optional HA / leader-fencing design and implementation.
