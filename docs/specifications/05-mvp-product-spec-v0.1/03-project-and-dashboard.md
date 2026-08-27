# 03 — Project & Dashboard

## 1. Project List

Required UI:

- list existing Projects;
- create Project;
- open Project;
- show lifecycle state and last modified time;
- archive/delete is not required in first implementation.

Create Project fields:

- `name` required;
- `description` optional.

On create, StageCore creates `Project` + initial `ProjectRevision(DRAFT)`.

## 2. Project Workspace

Minimum navigation:

- Dashboard
- Cues
- Routing
- Devices / Targets
- Runtime / Publish
- Rehearsal Sessions
- Notes

The exact visual framework is not fixed by this specification.

## 3. Dashboard Information

Must show:

- Project name;
- Draft revision identity/status;
- active Runtime Snapshot identity or `Not Published`;
- current mode: EDIT / REHEARSAL / SHOW;
- current Cue and next Cue when runtime is active;
- endpoint readiness summary;
- unpublished-changes indicator;
- active Session if any;
- latest runtime error/warning count.

## 4. Editing Rules

- Editing occurs against Draft configuration.
- Saving Draft does not change active runtime.
- invalid references are visible before Publish.
- IDs are stable even if names are changed.
- Project load must restore all Cue/Route/alias relationships.

## 5. MVP Acceptance

- Create, close, restart application, reopen Project with no data loss.
- Editing a published Cue must not alter the currently active Snapshot.
- Dashboard clearly distinguishes Draft and Published runtime.
- A Project with no Snapshot cannot enter SHOW mode.
