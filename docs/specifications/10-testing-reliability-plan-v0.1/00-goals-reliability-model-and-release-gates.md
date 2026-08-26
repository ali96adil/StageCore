# 00 — Goals, Reliability Model & Release Gates

## Goals

The plan must verify that StageCore:

- executes the defined MVP loop repeatedly, not only once;
- reports failures truthfully and keeps correlation traces inspectable;
- preserves committed Project, Snapshot, Session and trust state across restart;
- never auto-replays previous non-idempotent work after reconnect/restart;
- keeps P0/P1 runtime work isolated from bulk storage, backup and management work;
- remains operational on the stage LAN when Internet/WAN is unavailable;
- detects required Companion/Plugin/storage/security blockers before SHOW;
- can restore from a verified backup without silently replacing live state.

## Reliability Model

The MVP is not High Availability. A Hub failure may interrupt authority. Reliability means the system fails visibly, preserves trustworthy state where possible, and returns through a defined recovery path.

Reliability is evaluated across:

1. correctness;
2. isolation;
3. observability;
4. bounded timing/resource use;
5. persistence/integrity;
6. recovery;
7. repeatability.

## Release Gates

### G0 — Unit / Contract Gate
Pure domain rules, validation, serialization and command/event contracts pass automatically.

### G1 — Integration Gate
Database + Core + simulated adapters + API/realtime paths pass end-to-end on one machine.

### G2 — Real Integration Gate
Reference OSC receiver and macOS Companion pass the real local-network flow.

### G3 — Fault Gate
Required network/process/storage/trust failures produce the specified degraded/error/recovery behavior.

### G4 — Performance / Soak Gate
Reference latency targets and bounded-queue/resource behavior are measured under representative load.

### G5 — Recovery Gate
Restart, interrupted transfer, backup/restore and selected power-loss scenarios pass without invented success or silent corruption.

### G6 — First Rehearsal Qualification
A documented reference deployment completes the field checklist in `10-first-rehearsal-qualification.md`.

## Rule

A failing MUST gate is fixed or explicitly descoped through a documented product/architecture decision. It is not hidden by changing the test after implementation.