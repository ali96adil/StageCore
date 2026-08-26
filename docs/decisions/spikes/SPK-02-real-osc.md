# SPK-02 — Real OSC

**Status:** ACCEPTED for first external integration baseline  
**Scope:** first real `osc.send` capability, logical target resolution, UDP transport semantics and reproducible receiver test  
**Validated by:** `prototypes/spk-02-real-osc`

## Decision

The first real StageCore external action uses **OSC 1.0 messages over UDP** through the capability key `osc.send`.

The runtime-facing Action references a logical `target_id`; network location belongs to endpoint configuration and is resolved at execution time.

```text
Cue / Route Action
 -> capability: osc.send
 -> target_id: VIDEO-MAIN
 -> resolve VIDEO-MAIN -> OSC endpoint host + port
 -> validate OSC address + typed arguments
 -> encode one OSC message
 -> send one UDP datagram
 -> report result
```

A successful UDP write reports:

```text
status: COMPLETED
ack_level: TRANSPORT_ONLY
```

It never claims `DEVICE_ACK` or verified remote execution without an explicit future feedback mechanism.

## Why This Boundary

Keeping host/port outside Cue definitions preserves StageCore's logical show intent. Replacing a Mac, moving to another venue, or changing an endpoint should update mapping/configuration rather than require editing every Cue.

The Core continues to reason in terms of capability + logical target. OSC-specific address encoding and transport behavior remain adapter/plugin responsibility.

## v0.1 OSC Message Contract

Required fields:

- `target_id` — logical StageCore endpoint/alias;
- `address` — OSC address beginning with `/`;
- `arguments` — ordered typed arguments, optional.

The spike validates these argument types:

- `int32`;
- `float32`;
- `string`;
- `bool` using OSC `T/F` tags.

The explicit type wrapper is deliberate. JSON numbers do not preserve OSC integer-vs-float intent reliably by themselves.

## UDP Semantics

UDP is appropriate for the first OSC path because it is widely supported by show-control/media applications and keeps the send path small.

Important truthfulness rule:

- local validation/encoding/DNS/socket/write failure -> `FAILED` or `TIMED_OUT` as observed;
- complete local UDP datagram write -> `COMPLETED / TRANSPORT_ONLY`;
- no receiver response -> StageCore **cannot infer** remote failure from plain OSC/UDP alone;
- no automatic retry is introduced by the OSC sender. Retry/idempotency remains a Core/Action policy decision.

An unreachable device can therefore still produce a successful local UDP write. Preflight and the UI must not label that as verified device readiness.

## Spike Implementation Choice

The executable spike uses Go's standard `net` package plus a small OSC 1.0 codec so the protocol bytes and acknowledgement semantics can be tested without choosing a third-party library prematurely.

This is **not** a permanent decision that StageCore must maintain its own full OSC library. Before productionizing the plugin we may either:

1. keep the bounded codec after interoperability/fuzz testing, or
2. adopt a maintained OSC library if it reduces risk without leaking its API into StageCore contracts.

The stable decision is the StageCore capability/target/result contract, not the internal codec package.

## Evidence

`go test ./...` in `prototypes/spk-02-real-osc` passes with tests that:

1. encode/decode the supported OSC message types;
2. compare a known simple OSC packet byte-for-byte;
3. open a real localhost UDP receiver socket;
4. dispatch `osc.send` to logical target `VIDEO-MAIN`;
5. receive and decode the actual UDP packet;
6. verify the expected OSC address/arguments;
7. verify one dispatch emits one packet, not an automatic duplicate;
8. reject missing target and invalid endpoint configuration before claiming success.

The prototype also includes standalone sender/receiver commands for manual interoperability tests against external OSC software.

## What This Spike Does Not Decide

Deferred intentionally:

- `osc.receive` and Routing input lifecycle;
- OSC bundles/timetags;
- blobs and the full OSC type surface;
- TCP OSC variants;
- multicast/broadcast discovery;
- device-specific acknowledgement protocols;
- Plugin process isolation/IPC — SPK-04;
- Companion transport — SPK-03;
- final third-party OSC library selection.

## Integration Consequence

SPK-01's simulated Action path can now be replaced incrementally by an adapter interface whose first real implementation is `osc.send`.

The next spike is **SPK-03 — macOS Companion**. It must prove that the same logical Action can be delivered to a replaceable Mac execution agent without moving Project authority away from the Hub.
