# F-020 — Optional HA Dispatch Authority Foundation

Status: implementation/qualification in progress; software only.

This slice begins the optional High Availability work only after the single-Hub F-020 recovery policies passed F-024 fault qualification.

## Goal

Establish one fail-closed authority boundary immediately before every physical capability dispatch without changing standalone StageCore behavior.

This is a prerequisite for later redundant-Hub leader ownership and fencing. It is **not** leader election, replication, automatic failover, or complete split-brain protection by itself.

## Modes

The dispatch authority contract has three modes:

- `STANDALONE` — preserves the existing single-Hub runtime and allows physical dispatch.
- `LEADER` — allows physical dispatch only when a non-empty holder identity and non-zero epoch are present.
- `STANDBY` — rejects physical dispatch before any transport/plugin/device executor is called.

Unknown, incomplete, or unavailable authority fails closed.

## Central execution boundary

The product capability registry remains the capability/target catalog used by discovery and preflight.

Physical execution is wrapped once by `internal/dispatchauthority.Gate` and the same gated executor is supplied to:

- direct Cue/Action execution through the session-aware executor;
- Routing outputs, including route-triggered physical capabilities.

F-024 remains outside this physical boundary: `SIMULATION` is consumed by the session-scoped Digital Twin before the dispatch-authority source is consulted. Simulation therefore cannot become dependent on HA leader state and cannot reach a real transport.

## Current product behavior

The product currently wires a static `STANDALONE` authority source. Therefore this slice does not add a user-facing HA switch and does not change production behavior for a normal single Hub.

A future HA slice may replace the static source with a cross-node authority/lease implementation. That source must provide real leader ownership and monotonic fencing epoch semantics; an in-memory boolean leader switch is explicitly insufficient.

## Fail-closed results

- authority source unavailable: `PHYSICAL_DISPATCH_AUTHORITY_UNAVAILABLE`
- malformed/unknown authority: `PHYSICAL_DISPATCH_AUTHORITY_INVALID`
- standby Hub: `PHYSICAL_DISPATCH_NOT_LEADER`

All failures return no device acknowledgement and do not delegate to the physical executor.

## Verification requirements

CI must prove:

1. `STANDALONE` delegates exactly once and preserves the request.
2. `STANDBY` never invokes the physical executor.
3. `LEADER` requires holder identity plus non-zero epoch.
4. unavailable or unknown authority fails closed.
5. SIMULATION does not consult physical-dispatch authority and does not call the physical executor.
6. REHEARSAL and SHOW honor the authority gate.
7. existing product HTTP/script/OSC/routing tests remain green, proving standalone compatibility.

## Explicit non-goals

This slice does not implement:

- cross-node state replication;
- leader election;
- lease acquisition/renewal;
- automatic standby promotion;
- automatic command replay after failover;
- propagation of fencing tokens to external devices;
- claims of complete split-brain protection;
- Raspberry Pi deployment while #148 is active.

Physical/product qualification remains deferred under #148.
