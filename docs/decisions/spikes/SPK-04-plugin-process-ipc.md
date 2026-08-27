# SPK-04 — Plugin Process / IPC

**Status:** ACCEPTED for external Plugin process + IPC baseline  
**Scope:** Plugin process isolation, capability dispatch IPC, crash/hang containment, restart semantics and OSC reference execution  
**Validated by:** `prototypes/spk-04-plugin-ipc`

## Decision

StageCore v0.1 external Plugins use a **separate OS process per Plugin host**. The first IPC transport is **newline-delimited, versioned JSON over the child process stdin/stdout**.

```text
StageCore Core / Plugin Supervisor
          |
   JSON Lines over stdio
          |
 External Plugin Process
          |
  protocol / vendor SDK / I/O
```

The Plugin process stdout is protocol-only. Plugin diagnostics go to stderr so arbitrary log text cannot corrupt the IPC stream.

This requires no Redis, message broker, local TCP port registry or platform-specific Unix-domain socket API for the first implementation.

## Startup Handshake

After process start, the Plugin must emit one versioned `plugin.ready` message before it can receive execution work.

Reference fields:

```json
{
  "type": "plugin.ready",
  "schema_version": 1,
  "plugin_id": "stagecore.osc",
  "plugin_version": "0.1.0",
  "capabilities": ["osc.send"]
}
```

Core validates the handshake against the installed/activated manifest and refuses capability dispatch when the Plugin does not advertise the requested capability.

Package inspection and permission approval still happen before code activation as defined by the Plugin Contract. The runtime handshake is not a replacement for manifest validation.

## Capability Execution Contract

The Core keeps the generic capability model from 06. It performs Project/Snapshot/permission/safety checks and resolves logical targets before IPC dispatch.

Reference request shape:

```json
{
  "type": "execution.request",
  "schema_version": 1,
  "execution_id": "exec-123",
  "capability": "osc.send",
  "target": {
    "host": "127.0.0.1",
    "port": 53000
  },
  "parameters": {
    "address": "/scene/go",
    "arguments": [4, "intro", true]
  },
  "priority": "P1",
  "timeout_ms": 500,
  "correlation_id": "corr-77"
}
```

Important boundary: `VIDEO-MAIN` or another Project alias is resolved by Core. The external OSC Plugin receives only the approved endpoint configuration needed to perform `osc.send`; it does not own Project routing or Runtime Snapshot authority.

Result shape remains aligned with the existing capability contract:

```json
{
  "type": "execution.result",
  "schema_version": 1,
  "execution_id": "exec-123",
  "status": "COMPLETED",
  "ack_level": "TRANSPORT_ONLY",
  "duration_ms": 2
}
```

## Process Model for MVP

The first Plugin host serializes execution within one Plugin process. This gives a simple bounded execution model and prevents a blocked Plugin request from creating an unbounded number of in-process goroutines or child requests.

The Core-side supervisor may queue only within explicit bounded policy. A future Plugin that proves it needs concurrency can add negotiated/multiplexed execution without changing the capability semantics.

No `IN_PROCESS_TRUSTED` Plugin exception is required by current evidence. The stdio process boundary is simple enough for the reference OSC Plugin. If future measured latency or platform constraints justify in-process execution, that must be a separate explicit decision; third-party/untrusted Plugins remain external by default.

## Crash Behavior

If the Plugin exits or stdout closes before a matching result:

- current execution becomes explicit `FAILED` / `PLUGIN_FAILURE`;
- Core remains alive;
- Plugin state becomes unavailable/degraded;
- the failed execution is **not** automatically replayed;
- supervisor may start a fresh Plugin process for a later explicit execution.

A process restart is infrastructure recovery, not permission to resend the last Action.

## Hang / Deadline Behavior

Every execution has a bounded deadline. If the Plugin does not return a result before the deadline:

1. current execution becomes `FAILED` with timeout semantics (`TIMEOUT` / mapped `TIMED_OUT` at the Action contract boundary);
2. the Plugin process is terminated because its execution state is no longer trusted;
3. Core remains alive;
4. no automatic retry occurs;
5. a later explicit execution can start a fresh process after normal readiness checks.

This is intentionally conservative for non-idempotent show-control work.

## Restart Supervision

For v0.1, restart is **lazy/on-demand** after a crash or forced timeout: the next explicit execution or health recovery operation may start a new process and require a fresh `plugin.ready` handshake.

We do not need a permanently spinning crash-restart loop. Repeated startup failure should move the Plugin to `DEGRADED/FAILED` with operator-visible diagnostics instead of consuming resources indefinitely.

## Security / Permissions Boundary

SPK-04 validates process separation, not a complete OS sandbox.

Required production rules remain:

- Core checks Plugin grants before dispatch;
- Plugin receives only required resolved configuration/secret handles;
- secrets should use scoped Core/Vault APIs rather than arbitrary Project dumps;
- process environment/working directory must not become an accidental secret channel;
- stdout protocol and stderr logs are bounded/redacted;
- Plugin cannot upgrade acknowledgement semantics beyond what it can prove.

OS sandbox/container implementation remains a later technology decision if needed by threat model/platform support.

## Reference OSC Plugin

The prototype externalizes the same `osc.send` semantics accepted in SPK-02:

```text
Core request
 -> external stagecore.osc process
 -> OSC encode
 -> one UDP datagram
 -> execution.result
```

A successful UDP write still reports only `TRANSPORT_ONLY`.

## Prototype Evidence

Repeatable validation completed in the available Go 1.23 environment:

- `go test ./...` passes;
- `go test -race ./internal/pluginhost` passes;
- external Plugin performs a real UDP `osc.send` to a local receiver;
- Plugin advertises `stagecore.osc` + `osc.send` through `plugin.ready`;
- normal execution returns `COMPLETED + TRANSPORT_ONLY`;
- test-injected first-request process crash produces a contained `PLUGIN_FAILURE` instead of crashing the host;
- the crashed execution is not replayed;
- a new execution ID causes a fresh Plugin process/handshake and succeeds;
- test-injected first-request hang reaches a 120 ms deadline and the Plugin process is killed;
- the timed-out execution is not replayed;
- a later explicit execution restarts the Plugin and succeeds;
- race detection passes for the host manager tests.

## Acceptance Result

**ACCEPTED** for:

- external Plugin process as the v0.1 default;
- versioned JSON Lines over stdin/stdout as the first local Plugin IPC;
- stderr-only Plugin diagnostics;
- `plugin.ready` capability handshake;
- Core-side target resolution before Plugin dispatch;
- one-at-a-time execution per Plugin host for MVP;
- deadline kill on hung Plugin;
- crash/EOF containment;
- lazy restart for future explicit executions;
- no automatic replay after crash/timeout.

**Not yet selected/validated:**

- OS sandbox technology;
- long-idle heartbeat protocol;
- resource limits/telemetry implementation;
- multi-request IPC multiplexing;
- Plugin package extraction/signature implementation;
- cross-platform process supervisor details on Windows/macOS.

## Next Spike

**SPK-05 — Vault & Large File Transfer** should validate streaming/resumable large-file transfer, checksum verification and SHOW-mode pause behavior without putting media blobs in SQLite or blocking P0/P1 runtime work.
