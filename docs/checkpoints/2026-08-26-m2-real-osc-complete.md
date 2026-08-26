# StageCore — M2 Real OSC Completion Checkpoint

**Date:** 2026-08-26  
**Implementation slice:** M2 — Real OSC  
**Status:** COMPLETE  
**Merged product commit:** `56feab35b7ec65fed4047bc106c12c30899adf0c`  
**Pull request:** #13  
**Tracking issue:** #12 — closed as completed

## 1. Completion Decision

M2 is accepted as complete.

StageCore now has its first real transport-backed product Action path. `osc.send` is no longer prototype-only: the Hub composition owns a generic capability registry, resolves logical targets from the immutable Runtime Snapshot, dispatches to an external `stagecore.osc` process through bounded JSON Lines IPC, sends a real OSC 1.0 UDP datagram, and persists truthful Action/Cue/Event history.

**Decision:** `M2 COMPLETE — M3 ROUTING IS NEXT`.

## 2. Product Capability Delivered

```text
Validated ProjectRevision
        ↓
Runtime Snapshot v2
  ├─ Cue / Action definitions
  └─ Snapshot-captured logical target mapping
        ↓
Rehearsal Session
        ↓
cue.go
        ↓
Cue Engine
        ↓
generic capability registry
        ↓
osc.send
        ↓
Core resolves VIDEO-MAIN from Snapshot
        ↓
external stagecore.osc Plugin
        ↓
OSC 1.0 UDP datagram
        ↓
receiver
        ↓
COMPLETED / TRANSPORT_ONLY
        ↓
ActionExecution / CueExecution / Event journal
```

Delivered capabilities:

- generic product `capability.Executor` / registry boundary;
- M1 simulator retained through the same capability contract as `sim.test`;
- Runtime Snapshot manifest schema 2 with deterministic target capture;
- logical target resolution from immutable Snapshot content rather than mutable live alias state;
- `osc.send` product capability;
- OSC 1.0 UDP sender;
- explicit typed OSC arguments: `int32`, `float32`, `string`, `bool`;
- external `stagecore.osc` process;
- versioned JSON Lines stdin/stdout IPC;
- `plugin.ready` identity/capability handshake;
- reference Plugin identity enforcement;
- explicit `network.udp.send` permission enforcement for the M2 OSC path;
- bounded execution deadline;
- crash/EOF containment;
- hung Plugin termination;
- lazy fresh process start for a later explicit execution;
- no automatic replay after crash, timeout or cancellation;
- cancellation preserved distinctly at the Plugin execution boundary;
- configurable OSC Plugin executable path through `--osc-plugin-path` / `STAGECORE_OSC_PLUGIN_PATH`, with sibling-binary default;
- Hub `App.Open` composition wires both simulator and real OSC capability into the actual Cue Engine;
- ARM64 CGo-free CI builds both `stagecore-hub` and `stagecore-osc-plugin`.

## 3. Acknowledgement Truthfulness

Plain OSC over UDP has no remote execution acknowledgement.

Therefore a complete local UDP datagram write records only:

```text
status = COMPLETED
ack_level = TRANSPORT_ONLY
```

It must not be presented as `DEVICE_ACK` or verified remote state.

An unreachable remote host may still accept a local UDP write because UDP itself does not prove delivery. M2 therefore treats local validation/socket/write errors truthfully while refusing to infer remote execution success. Future readiness/feedback protocols may provide stronger evidence, but M2 does not fabricate it.

## 4. Snapshot Target Immutability Evidence

The real OSC integration test publishes a Snapshot containing `VIDEO-MAIN -> localhost:primary-port`, then mutates the live Project alias to a different secondary port.

Runtime execution still sends to the original primary port captured by the Snapshot, and the secondary receiver receives nothing.

This proves that live Project mapping edits do not silently mutate active runtime behavior.

## 5. Failure / Isolation Evidence

Automated coverage includes:

- missing logical target -> explicit `TARGET_NOT_FOUND` before Plugin start;
- invalid OSC target configuration -> explicit `TARGET_CONFIG_INVALID` before Plugin start;
- invalid OSC address/parameters -> explicit validation failure;
- missing `network.udp.send` grant -> explicit `PLUGIN_PERMISSION_DENIED` before Plugin start;
- Plugin crash/EOF -> contained Plugin failure; Core remains alive;
- Plugin hang -> bounded timeout and process termination;
- cancellation -> explicit cancellation at the Plugin boundary;
- after crash/hang/cancellation, the failed execution is not replayed;
- a later execution with a new execution ID starts a fresh Plugin process and succeeds;
- test-only crash/hang hooks live under `internal/pluginhost/testdata/`, not in the production `stagecore-osc-plugin` binary.

## 6. Hub Composition Evidence

A dedicated App-level integration test opens the real StageCore `App`, uses the configured product OSC Plugin binary, creates Project/Alias/Cue/Snapshot/Session data through the authoritative Store, executes through `App.CueEngine`, and receives the expected OSC datagram on a real localhost UDP receiver.

This closes an implementation gap found during M2 review: OSC was initially proven only through an injected test executor. Before merge, the actual Hub composition was updated so `osc.send` is a real product capability owned by `App.Open`.

## 7. CI Evidence

### Final PR-head gate

Head:

```text
f2466332735477716a7283f5e9f500f124c20c35
```

Core CI run:

```text
32972125448
```

Result: `SUCCESS`.

### Post-merge `main` gate

Merged commit:

```text
56feab35b7ec65fed4047bc106c12c30899adf0c
```

Core CI run:

```text
32972408148
```

Result: `SUCCESS`.

The merged-main gate passed:

- Go `1.26.x` tests;
- Go `1.26.x` vet;
- Go `1.26.x` native race tests;
- Go `1.27.x` tests;
- Go `1.27.x` vet;
- committed module-lock verification;
- Linux ARM64 `CGO_ENABLED=0` build for `stagecore-hub`;
- Linux ARM64 `CGO_ENABLED=0` build for `stagecore-osc-plugin`.

## 8. Boundary Preserved

M2 intentionally does **not** claim completion of:

- `osc.receive`;
- Routing execution;
- Route-origin OSC dispatch acceptance;
- full Security SEC5 Plugin permission administration;
- non-loopback Stage LAN control API;
- Companion trust/runtime channel;
- Operator UI;
- Vault/media runtime workflows;
- StageNodes;
- full DMX/lighting;
- AI/Vision;
- HA/cloud/failover work.

The M2 implementation provides the generic capability dispatch path that Routing will reuse. The older MVP wording that placed Route-origin OSC inside M2 is explicitly resolved by the current implementation order: **M3 owns Routing and closes Route-origin OSC acceptance.**

The explicit OSC Plugin grant in M2 is the minimum reference-path enforcement required to prove the permission boundary; it does not claim the complete SEC5 administration model before the preceding Security slices converge.

## 9. Transition to M3

The next implementation slice is:

**M3 — Routing**

Expected vertical path:

```text
Test Input / supported OSC input
→ Input normalization
→ Route lookup
→ simple condition
→ debounce
→ Route Trace
→ Cue dispatch OR Output capability dispatch
→ existing capability registry
→ sim.test / osc.send
→ execution result + Event history
```

M3 must preserve Snapshot authority, bounded runtime behavior, no duplicate dispatch, truthful capability results and the M0–M2 CI gates.

## 10. Reference Rule

This checkpoint is the implementation transition reference after M2. It does not supersede the architecture/product baseline. Any later semantic change to OSC acknowledgement, Plugin isolation, Snapshot mapping or routing ownership requires an explicit documented delta with regression evidence.
