# StageCore

Local-first show control, cue, routing, rehearsal and stage integration platform.

## Current implementation phase

**M0–M6 software MVP: COMPLETE**

**Raspberry Pi ARM64 M0–M6 physical qualification: PASS** — Issue #21, for the tested reference configuration.

**Current engineering state: FEATURE EXPANSION READY**

The next implementation target is **F-027 — Rehearsal & Show Session Modes**, followed by the **F-028 rehearsal timing-capture foundation** defined by the dependency-first feature order.

The Hub product source lives under `cmd/` and `internal/`; technology prototypes remain under `prototypes/` as decision evidence only.

### Development run

```bash
go run ./cmd/stagecore-hub --data-root ./stagecore-data
```

The safe default listen address remains loopback-only (`127.0.0.1:7840`). Supported non-loopback operation must continue to use the established security and transport policy rather than weakening the default.

Health endpoints:

- `GET /health/live`
- `GET /health/ready`

Current engineering status and transition evidence:

- `docs/BASELINE_STATUS.md`
- `docs/checkpoints/2026-08-30-m0-m6-physical-qualification-complete.md`
- `docs/FEATURE_IMPLEMENTATION_ORDER.md`
