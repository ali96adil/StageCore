# SPK-02 — Real OSC

Executable spike proving the first real StageCore external action path.

## Proven path

```text
Logical Action target VIDEO-MAIN
 -> resolve OSC endpoint host/port
 -> capability osc.send
 -> encode OSC 1.0 message
 -> send one UDP datagram
 -> return COMPLETED / TRANSPORT_ONLY
 -> local test receiver decodes the expected message
```

The Cue/Action-facing model uses `target_id`; host/port live in endpoint configuration.

## Run tests

```bash
go test ./...
```

The tests open a real localhost UDP socket, send a real OSC packet, receive it, decode it, and verify no duplicate packet is emitted.

## Manual interoperability check

Terminal 1 — start the included receiver:

```bash
go run ./cmd/osc-receiver -host 127.0.0.1 -port 9000
```

Terminal 2 — send through the StageCore OSC executor:

```bash
go run ./cmd/osc-demo -host 127.0.0.1 -port 9000 -address /stagecore/demo
```

The same sender can target another OSC application by changing host/port/address.

A successful UDP write reports `TRANSPORT_ONLY`. It does **not** claim the remote application executed the command.

## v0.1 supported argument types

- `int32`
- `float32`
- `string`
- `bool`

OSC bundles, blobs, timetags and receive/routing are deliberately deferred until the send path is integrated into the main Hub runtime.
