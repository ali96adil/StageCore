# F-024 — Simulation Report

**Slice:** E  
**Phase:** 5  
**Tracker:** #149  
**Physical qualification:** deferred under #148

## Purpose

The Simulation Report is a derived, read-only diagnostic artifact. It does not become a new source of runtime truth and it does not upgrade simulated evidence into physical evidence.

The report is generated for one authoritative `SIMULATION` Session and one immutable published Runtime Snapshot.

## Evidence sources

The report combines only evidence StageCore already owns:

- the Session and immutable Runtime Snapshot manifest;
- persisted CueExecution and ActionExecution history;
- Flight Recorder events;
- the session-scoped in-memory Digital Twin snapshot, when available;
- Phase 4 Stage Device runtime observations for explicitly bound devices.

The report does not probe hardware, send commands, or mutate Digital Twin / Stage Device state.

## Report categories

### Missing mappings

Enabled Cue Actions and Outputs are checked against `RuntimeSnapshot.targets`.

If an authored `target_ref` has no target alias in the immutable Snapshot, the report records:

```text
TARGET_ALIAS_NOT_FOUND
```

The report does not fabricate a target mapping from the capability name or from a currently connected device.

### Timing risks

ActionExecution latency is simulation evidence only.

The report identifies:

- `SIMULATED_TIMEOUT` when a simulated Action timed out;
- `SIMULATED_LATENCY_AT_OR_OVER_TIMEOUT` when recorded simulated latency reaches/exceeds the configured Action timeout;
- `SIMULATED_LATENCY_NEAR_TIMEOUT` when simulated latency reaches at least 80% of the configured timeout.

Every timing finding is marked `SIMULATION_ONLY`. It is not a physical-device latency measurement.

### Unrecovered failures

Failed, timed-out or cancelled simulated Actions are reported when the Action does not continue under an explicit `CONTINUE` error policy.

Default/explicit `FAIL_CUE` behavior is reported as a failure that was not recovered inside the simulated show flow. Unsupported error-policy values are reported separately as `UNSUPPORTED_ERROR_POLICY`.

This section is intended to show operator-visible scenarios that stop or break the simulated flow; it does not imply that StageCore lost the failure event.

### Simulation vs observed Stage

Physical comparison is intentionally conservative.

A Runtime Snapshot target participates in Stage Device comparison only when its immutable target configuration contains an explicit:

```json
{"device_id":"..."}
```

The configured ID is matched only against the Project's Phase 4 `stage_devices` / `stage_device_runtime_state` observation.

Possible comparison states:

- `MATCH` — Digital Twin online/offline state agrees with the observed Stage Device connection state;
- `DIFFERENT` — both sides are observed and their connection state differs;
- `UNKNOWN` — either side lacks enough evidence.

Examples that remain `UNKNOWN`:

- no explicit `device_id` binding;
- the configured Stage Device is not registered;
- the Stage Device has no runtime observation;
- the Digital Twin has no in-memory target observation.

`UNKNOWN` is a required truth state, not a warning workaround.

## Hub restart behavior

Cue/Action execution history and Flight Recorder evidence are persisted and remain reportable after restart.

Digital Twin runtime state is intentionally session-scoped and in-memory. If no current Twin observation survives a Hub restart, Stage comparison that depends on that virtual state becomes `UNKNOWN`; StageCore does not reconstruct or invent it from physical state.

Operators can use simulation checkpoints when they need replay-free virtual-state restoration, but a checkpoint does not convert simulated truth into physical truth.

## API

The authenticated read-only endpoint is:

```text
GET /api/v1/projects/{project_id}/simulation/report/{session_id}
```

The requested Session must be a `SIMULATION` Session and must belong to the Project in the URL. Cross-project access returns not found rather than leaking report data.

## Operator UI

The Simulation workspace displays the report inline and provides:

- summary counts;
- missing mapping table;
- timing risk table;
- unrecovered failure table;
- Simulation vs observed Stage comparison;
- report refresh;
- JSON export.

The UI remembers the latest Simulation Session for the selected Project so the most recent report remains easy to inspect after `Stop` in the same browser profile.

The report UI is explicitly labeled:

```text
EVIDENCE-BASED · READ ONLY
```

and does not call REHEARSAL/SHOW GO or JUMP controls.

## Safety invariants

- report generation is read-only;
- no report path can dispatch physical capability output;
- Runtime Snapshot data is never modified;
- Stage Device observations are never overwritten by simulated state;
- Digital Twin state is never promoted to physical acknowledgement/readiness/health;
- missing physical evidence remains `UNKNOWN`;
- simulation latency remains `SIMULATION_ONLY`;
- REHEARSAL and SHOW behavior is unchanged.

## Software qualification

The final Slice E PR head must pass Core CI for:

- module lock;
- unit/integration tests;
- vet;
- race tests on the primary toolchain;
- Linux ARM64 CGo-free product builds;
- evidence-generation integration tests;
- Operator evidence/bilingual contract tests.

Physical/product qualification remains deferred under #148 and is not implied by report CI success.
