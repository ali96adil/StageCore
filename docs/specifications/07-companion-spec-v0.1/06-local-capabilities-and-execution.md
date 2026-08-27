# 06 — Local Capabilities & Execution

## Purpose

The Companion executes capabilities that must run on that machine or reach software/devices local to it.

Reference MVP capabilities may include:

- `osc.send` from the Companion machine;
- basic MIDI send later;
- approved local application integration;
- local file/media presence checks;
- controlled script/integration execution where explicitly enabled.

## Execution Flow

```text
Hub validates Cue/Action
 -> dispatches execution request to assigned Companion
 -> Companion validates Snapshot/role/capability/local config
 -> local adapter executes
 -> Companion returns explicit result
 -> Hub records ActionExecution/Event trace
```

## Result Requirements

The Companion returns at minimum:

- `execution_id`;
- result: `COMPLETED | FAILED | TIMED_OUT | CANCELLED`;
- acknowledgement level actually proven;
- duration/timestamps;
- sanitized error code/message when failed.

## No False Success

Launching a packet/process/request is not automatically equivalent to verified external state. Results follow the acknowledgement model in 04 — Event & Command Contracts.

## Timeouts & Cancellation

- each execution has bounded timeout policy;
- STOP/cancel works only when the capability defines a real cancellation semantic;
- Companion does not guess inverse actions;
- non-idempotent Actions are never automatically retried after reconnect.

## Isolation

Failure-prone adapters/scripts should remain outside the critical Hub runtime. A Companion adapter crash changes capability/role health; it must not corrupt Hub Project data.