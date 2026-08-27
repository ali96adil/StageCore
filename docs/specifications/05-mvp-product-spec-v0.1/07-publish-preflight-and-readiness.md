# 07 — Publish, Preflight & Readiness

## 1. Draft vs Published

Editing always changes Draft configuration. Runtime execution reads only the active Published Runtime Snapshot.

The UI must make these states obvious:

- Draft clean / no unpublished changes;
- Draft modified;
- validation failed;
- validation passed;
- Published Snapshot active.

## 2. Validation

Minimum Publish blockers:

- missing Cue/Action references;
- missing target alias/capability;
- invalid Action parameters against capability schema;
- required plugin/adapter missing;
- duplicate or invalid ordering/reference constraints that make execution ambiguous;
- invalid Route target;
- required Companion role unresolved if the Snapshot depends on it.

Warnings may include non-blocking unreachable optional targets or degraded observability.

## 3. Publish

Publish must:

1. validate the selected Draft revision;
2. create a new immutable `RuntimeSnapshot` identity;
3. store Cue/Route/target requirement manifests sufficient to reproduce runtime behavior;
4. mark it as published;
5. never mutate a previous Snapshot in place.

Publishing configuration does not by itself execute any Cue or physical device action.

## 4. Preflight

Before REHEARSAL/SHOW, Preflight checks:

- active Snapshot exists;
- Snapshot and project match;
- required adapters/plugins available;
- required target configuration exists;
- required Companion is connected and reports expected snapshot/config readiness where applicable;
- storage is not in a state that prevents safe runtime logging;
- no blocking validation errors.

A simple list with `PASS / WARN / BLOCK` is enough for MVP.

## 5. Snapshot Sync

For a Companion-dependent runtime:

- Hub sends required runtime identity/config subset;
- Companion reports the applied Snapshot ID;
- mismatch status is visible;
- role is not READY until reconciled.

Full distributed artifact deployment is not required in v0.1.

## 6. Rollback

Rollback in MVP may be limited to selecting a previously valid Snapshot and activating it through an explicit command. It must never silently overwrite history.

## 7. Acceptance

- Edit after Publish leaves current Snapshot unchanged.
- Invalid target reference blocks Publish.
- Successful Publish creates a distinct Snapshot ID/version.
- Preflight identifies an offline required Companion.
- Snapshot mismatch prevents endpoint READY.
