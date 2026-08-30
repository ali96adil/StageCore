# F-012 — Show Mode Configuration Lock

**Status:** Implementation contract — v1 project configuration lock

## Goal

Reduce accidental structural configuration changes while a live SHOW Session is active without weakening the runtime controls required to run or safely stop the show.

The lock is a runtime-derived safety state, not a manually toggled preference.

## Lock activation

A Project is configuration-locked when it has an active Session whose `session_type` is `SHOW`.

The lock is derived from canonical Session truth. F-012 does not persist a second lock flag that could become stale after a crash or show exit.

The v1 lock view reports:

- contract version;
- whether the Project is locked;
- active SHOW Session identity;
- Runtime Snapshot identity used by that SHOW;
- SHOW start time;
- reason `ACTIVE_SHOW_SESSION`;
- scope `PROJECT_CONFIGURATION`;
- unlock action `SHOW_EXIT`;
- `override_supported = false`.

## Unlock path

There is no in-show configuration override in v1.

The deliberate authorized path back to configuration mode is the existing authenticated `show.exit` runtime operation. Once the SHOW Session is no longer active, the derived lock clears automatically.

This avoids an ambiguous state where the UI says SHOW while configuration writes are secretly permitted.

## Protected structural configuration

While a Project is locked, structural writes associated with that Project are rejected, including:

- Project structural updates/deletion;
- Project revision creation/change/deletion;
- Cue and Action creation/change/deletion;
- logical target aliases;
- Inputs, Outputs, Routes and Route Actions;
- Runtime Snapshot creation/change/deletion;
- Machine Role configuration;
- Media Asset/content requirements that define the show environment.

The database enforces this boundary in addition to application/API guards so a caller cannot bypass the lock merely by reaching a lower-level store method.

Creating or editing a different Project remains allowed.

## Operations that continue

The lock does not freeze live runtime truth. During SHOW, StageCore must continue to allow:

- GO, STOP, Jump and other authorized runtime commands defined by the active Published Snapshot;
- Session, Cue Execution, Action Execution, command and event journal writes;
- operator notes/session memory;
- live health/readiness observations;
- Companion/runtime state needed to execute the show;
- emergency security revocation and containment required by the Security Model.

Published Runtime Snapshot immutability remains a separate invariant and is not replaced by F-012.

## Operator experience

Configuration read APIs expose the lock state. The Operator Configuration page must visibly state that SHOW MODE configuration is locked and disable structural edit controls, including the action that forks a new Draft.

The message directs the operator to exit SHOW through authorized runtime controls before editing. Runtime controls remain available in their runtime workspace.

Server-side enforcement is authoritative; disabling buttons is only a usability layer.

## Failure behavior

A blocked application/API mutation returns a dedicated `SHOW_CONFIGURATION_LOCKED` conflict surface rather than pretending the mutation succeeded.

Database-level protected writes fail with the same stable marker so lower-level violations are diagnosable.

## Out of scope for v1

- no emergency configuration override while SHOW remains active;
- no per-field allowlist inside structural Project configuration;
- no new GO/STOP semantics;
- no replacement for existing security-operation SHOW policy;
- no lock of operational telemetry, runtime acknowledgements or emergency revocation;
- no cross-project global freeze.

## Acceptance

F-012 v1 is accepted when tests prove:

1. an active SHOW automatically reports the Project as locked;
2. representative structural writes are rejected at the database boundary;
3. the Operator API returns an explicit locked response for configuration mutation;
4. a different Project remains editable;
5. REHEARSAL does not activate the configuration lock;
6. runtime/event writes continue during SHOW;
7. after authorized SHOW exit/end, the same Project becomes editable again;
8. Operator Configuration visibly exposes the lock instead of merely hiding edit actions.
