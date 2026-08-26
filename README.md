# StageCore

Local-first show control, cue, routing, rehearsal and stage integration platform.

## Current implementation phase

**M0 — Core Persistence** is the first production implementation slice.

The Hub product source now lives under `cmd/` and `internal/`; technology prototypes remain under `prototypes/` as decision evidence only.

### Development run

```bash
go run ./cmd/stagecore-hub --data-root ./stagecore-data
```

The default listen address is loopback-only (`127.0.0.1:7840`) until the Security convergence gate is implemented.

Health endpoints:

- `GET /health/live`
- `GET /health/ready`

See `docs/adr/addendum-002/` for the pinned M0 entry baseline and persistence rules.
