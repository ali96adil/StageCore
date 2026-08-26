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

## OSC v0.1 Capability

`osc.send` parameters:

- `address`: required valid OSC address;
- `arguments`: optional ordered OSC argument list;
- `target_id`: resolves to an `osc.endpoint` configuration;
- transport: UDP only in v0.1.

The endpoint configuration contains host and port outside the Cue definition.

## Reference End-to-End Flow

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

`osc.receive` is deferred because it introduces listener lifecycle, input normalization, port ownership, rate limiting and Routing integration.
