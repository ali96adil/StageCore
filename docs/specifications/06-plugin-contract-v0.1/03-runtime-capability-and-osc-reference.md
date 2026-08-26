# 03 — Runtime Capability Contract & OSC Reference

## Generic Runtime Call

The Core executes plugin functionality through a capability-oriented request rather than protocol-specific Core methods.

Conceptual request:

```json
{
  "execution_id": "exec-123",
  "capability": "osc.send",
  "target_id": "video-main",
  "parameters": {
    "address": "/scene/go",
    "arguments": [4]
  },
  "priority": "P1",
  "timeout_ms": 500,
  "correlation_id": "corr-77"
}
```

The Core resolves the logical target to approved plugin/device configuration before dispatch. Cue definitions should not duplicate raw IP/port values when a target mapping exists.

## Generic Result

```json
{
  "execution_id": "exec-123",
  "status": "COMPLETED",
  "ack_level": "TRANSPORT_ONLY",
  "duration_ms": 3
}
```

Supported result states align with the Event & Command Contracts: `COMPLETED`, `FAILED`, `TIMED_OUT`, `CANCELLED`.

## Acknowledgement Accuracy

The plugin must report only what it can prove. Sending an OSC UDP datagram does not prove that the external application executed the requested behavior. Therefore the reference OSC sender reports at most `TRANSPORT_ONLY` unless a future verified feedback mechanism exists.

## OSC v0.1 Send Capability

`osc.send` parameters:

- `address`: required valid OSC address;
- `arguments`: optional ordered OSC argument list;
- `target_id`: resolves to an `osc.endpoint` configuration;
- transport: UDP only in v0.1.

The endpoint configuration contains host and port outside the Cue definition.

## Reference Send Flow

```text
Operator GO
 -> Core validates Cue / Snapshot / permissions
 -> ActionExecution created
 -> Core resolves VIDEO-MAIN -> OSC Endpoint
 -> execute capability osc.send
 -> external OSC plugin encodes and sends UDP packet
 -> plugin result returned
 -> ActionExecution completed/failed
 -> CueExecution result calculated
 -> events and Rehearsal Log updated
```

## M3 Promotion — OSC Receive Routing Input

The earlier M2 baseline deferred `osc.receive` because it requires listener lifecycle, input normalization, port ownership, rate limiting/debounce and Routing integration. **M3 Routing owns and promotes that deferred item.**

The receive path remains behind the same `stagecore.osc` external-process boundary:

```text
OSC UDP datagram
 -> external stagecore.osc process in receive mode
 -> normalized plugin `input.event`
 -> Hub validates Plugin identity / permission / input contract
 -> active Runtime Snapshot resolves matching InputDefinition
 -> Routing Engine evaluates Route
 -> Cue or generic capability dispatch
 -> persistent Route Trace
```

Rules for the M3 receive contribution:

- contribution key: `osc.receive`;
- Plugin process model remains `external`;
- required permission: `network.udp.listen`;
- the external Plugin owns the UDP socket; Core does not open an OSC network listener directly;
- malformed UDP datagrams are isolated and do not create input events;
- the Plugin normalizes transport data only; authoritative InputDefinition matching, Snapshot checks, Route evaluation, debounce and dispatch remain in Hub Core;
- each received UDP datagram is treated as a new input occurrence; StageCore does not invent network-level deduplication or replay. Route debounce is the bounded duplicate/burst control in M3;
- until the SEC0–SEC2 Stage LAN security gate is implemented, product OSC receive listeners are restricted to loopback addresses only. Non-loopback Stage LAN control must not be enabled implicitly.
