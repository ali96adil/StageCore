# 08 — Health, Readiness & Preflight

## Health Report

Companion reports observable machine state such as:

- connection/heartbeat;
- agent version;
- assigned role;
- CPU/memory pressure summary;
- free disk space relevant to cache/logging;
- local capability/plugin availability;
- required app/integration state where verifiable;
- Runtime Snapshot/config hash;
- required media status.

## Readiness States

- `UNKNOWN`
- `SYNCING`
- `READY`
- `DEGRADED`
- `OFFLINE`
- `MISMATCH`
- `BLOCKED`

Readiness is calculated from declared requirements, not a decorative green light.

## Preflight Checks

For an assigned required role, MVP checks at minimum:

- Companion trusted and connected;
- expected Companion/agent API compatible;
- required capability available;
- assigned Runtime Snapshot matches;
- role configuration applied;
- required media present and checksummed if media is used;
- minimum required storage available;
- no blocking local configuration error.

## PASS / WARN / BLOCK

Each check returns one of `PASS`, `WARN`, `BLOCK`, plus a short operator-readable reason and remediation hint when known.

## Truthfulness Rule

If StageCore cannot verify an external application's internal state, it must say `UNKNOWN` or `DEGRADED` rather than `READY` based on assumption.

## Show Gate

A required role with a blocking Preflight result prevents entry into SHOW according to Project policy. Optional/degraded roles may remain warnings.