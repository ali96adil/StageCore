# 06 — Rehearsal & Show Mode

## 1. Modes

MVP supports:

- `EDIT`
- `REHEARSAL`
- `SHOW`

`SIMULATION` and advanced diagnostic modes may be added later.

## 2. Rehearsal Mode

REHEARSAL is the primary proof environment for MVP. It runs the published Runtime Snapshot and records a Rehearsal Session.

Required behavior:

- start/stop Rehearsal Session;
- display current/next Cue;
- GO/STOP/Jump;
- record CueExecution and ActionExecution;
- allow quick Note creation;
- preserve errors and manual overrides.

## 3. Show Mode

SHOW uses the same runtime engine but with stricter gates:

- Published Snapshot required;
- required endpoint readiness must pass;
- blocking validation errors prevent entry;
- editing Draft remains possible only if it cannot affect active runtime;
- runtime controls remain focused and accidental destructive actions require confirmation.

MVP SHOW mode is not High Availability and does not claim certified safety behavior.

## 4. Runtime Screen

Minimum visible elements:

- mode;
- Project + Snapshot version;
- current Cue;
- next Cue;
- large GO control;
- STOP control;
- cue result/status;
- endpoint degraded/offline warning;
- compact recent execution/error feed.

## 5. Session Rules

- one active Rehearsal/Show Session at a time for the active project;
- every execution references the active Runtime Snapshot;
- Session end time is explicit;
- application crash/power loss must not invent a clean Session completion; recovery marks it interrupted if needed.

## 6. Acceptance

- REHEARSAL can run a published Cue sequence and preserve execution history.
- SHOW cannot start with no published Snapshot.
- SHOW cannot start when a required endpoint is known mismatched/offline and configured as blocking.
- editing Draft during runtime does not change executed Cue content until a new Publish.
