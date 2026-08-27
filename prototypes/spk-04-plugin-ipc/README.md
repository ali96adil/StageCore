# SPK-04 — Plugin Process / IPC Prototype

This executable spike validates StageCore external Plugin isolation using a real `osc.send` Plugin process.

## What it proves

- Plugin runs outside the Core process.
- IPC is versioned JSON Lines over stdin/stdout.
- Plugin starts with a `plugin.ready` handshake and capability list.
- Core resolves the logical target before dispatch; Plugin receives host/port configuration, not Project authority.
- Real OSC UDP send returns `COMPLETED + TRANSPORT_ONLY`.
- Plugin crash becomes an explicit `PLUGIN_FAILURE`; Core/host process survives.
- Plugin hang reaches the execution deadline, is killed, and returns `TIMEOUT`.
- Failed/timed-out execution is never automatically replayed after Plugin restart.
- A later explicit execution with a new `execution_id` can restart the Plugin and succeed.
- stdout is reserved for the IPC protocol; stderr is reserved for Plugin diagnostics/logging.

## Run

```bash
go test ./...
go test -race ./internal/pluginhost
```

Manual demo:

```bash
go build -o /tmp/stagecore-osc-plugin ./cmd/stagecore-osc-plugin
go run ./cmd/plugin-demo /tmp/stagecore-osc-plugin
```

Expected demo shape:

```text
plugin=stagecore.osc status=COMPLETED ack=TRANSPORT_ONLY bytes=...
```

## Important Limitations

This spike validates the process/IPC and failure-containment boundary. It does not implement the full Plugin package installer, manifest permission UI, OS sandbox, secret broker, idle heartbeat, resource telemetry, or production supervisor policy. Those remain implementation work under the existing Plugin and Security specifications.
