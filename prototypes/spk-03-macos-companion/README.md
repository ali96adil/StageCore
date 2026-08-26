# SPK-03 — macOS Companion Prototype

This prototype validates the StageCore Hub-to-Companion execution channel before building the production macOS UI.

## Components

- `hub-sim/` — Go Hub simulator using a minimal spike-only WebSocket server.
- `companion/` — Swift 6 package containing `StageCoreCompanionCore` and a CLI harness.
- `scripts/run-e2e.sh` — repeatable end-to-end reconnect/execution scenario.

## What It Proves

1. Swift Companion connects to the Go Hub over WebSocket.
2. Companion presents a stable identity and capabilities.
3. Hub assigns `VIDEO-MAIN` and Runtime Snapshot `snap-1`.
4. Companion executes `local.echo` and returns an explicit result.
5. Hub intentionally disconnects the channel.
6. Companion reconnects with the same identity.
7. Replayed `execution_id` is rejected as `DUPLICATE_EXECUTION` rather than executed twice.
8. Command for a stale Runtime Snapshot is rejected as `SNAPSHOT_MISMATCH`.
9. A new valid execution after reconnect completes normally.

## Run

```bash
cd companion
swift test

cd ../hub-sim
go test ./...

cd ..
./scripts/run-e2e.sh
```

The production Companion will use authenticated `wss://`, pairing credentials and macOS Keychain. The file identity store and plain local `ws://` connection exist only in this transport spike.

The current validation environment can compile and run the Swift package but is not macOS, so SwiftUI app-bundle, background-service, signing and notarization behavior are intentionally not claimed by this prototype.
