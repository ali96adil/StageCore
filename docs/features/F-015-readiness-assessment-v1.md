# F-015 — Pre-activation Readiness Assessment v1

## Status

Implementation checkpoint for F-015 Plugin & Add-on Library / Manager.

This checkpoint adds a derived, read-only assessment that answers one narrow question:

> Is this exact installed extension package eligible to proceed to a future activation step?

It does **not** activate, enable, execute, or health-check a running extension.

## Why readiness is derived

Readiness depends on facts that can change independently:

- installed payload integrity;
- current package compatibility policy;
- current production-ready signing/release policy;
- installed dependency versions;
- current permission-review decisions.

Persisting a READY flag would become stale when one of those facts changes. StageCore therefore derives readiness on demand from authoritative sources instead of adding another lifecycle row or database migration.

## Overall states

- `READY_FOR_ACTIVATION` — all pre-activation blockers currently pass;
- `NOT_READY` — one or more pre-activation blockers remain.

`READY_FOR_ACTIVATION` is deliberately not named `READY`, `RUNNING`, or `HEALTHY` because no runtime process has been activated yet.

## Checks

### Installed integrity

The assessment begins through `Installer.Get`, which verifies the managed installed payload:

- safe managed path;
- no symlink escape;
- regular-file type;
- expected immutable size;
- expected SHA-256;
- expected non-writable/non-executable mode.

If this check fails, the assessment itself fails integrity validation rather than returning a misleading NOT_READY result.

### Package compatibility

The registered package must remain compatible with the current Hub API policy.

An incompatible package blocks activation eligibility.

### Package production readiness

The underlying immutable Software Repository package must satisfy the current production-ready signing/release policy.

LOCAL or COMMUNITY packages may still exist in the library and may be installed for controlled testing, but unsigned/development packages are not eligible for production activation through this readiness contract.

### Required dependencies

The existing deterministic dependency planner remains authoritative.

For an already installed root package, readiness requires:

- the root installation to be recognized;
- no dependency blockers;
- no missing required dependency install steps;
- all required installed versions to satisfy accumulated semantic-version constraints.

Optional dependencies never block readiness. Their planner warnings are returned as advisories.

### Permission review

The installation-scoped Permission Review foundation remains authoritative.

- `APPROVED` passes;
- `NOT_REQUIRED` passes;
- `PENDING` blocks;
- `DENIED` blocks.

Permission approval still does not mutate runtime permission grants.

### Runtime health

Runtime health is explicitly reported as:

- status: `NOT_APPLICABLE`;
- code: `ACTIVATION_NOT_IMPLEMENTED`.

This is a product-safety requirement, not a placeholder success. StageCore must not claim a process is healthy before it has an activation/runtime lifecycle to start and probe that process.

## Operator API

Read readiness:

`GET /api/v1/extensions/installations/{installation_id}/readiness`

The endpoint is read-only and requires normal project-read permission, so a Viewer may inspect readiness.

There is no readiness mutation endpoint.

Integrity failures return an explicit conflict instead of a derived readiness response.

## SHOW behavior

Readiness assessment is read-only and may be inspected during SHOW.

This checkpoint introduces no lifecycle mutation and therefore does not alter SHOW state.

Future activation, disable, repair, update, removal, or permission-grant changes remain separately SHOW-gated.

## Acceptance covered by this checkpoint

- production-ready installation with no permissions or dependencies can become `READY_FOR_ACTIVATION`;
- a pending permission review blocks readiness;
- explicit approval allows readiness without granting runtime authority;
- a missing required dependency blocks readiness;
- installing the required dependency allows readiness to recover;
- a missing optional dependency produces an advisory but does not block;
- unsigned/development package metadata blocks production activation eligibility;
- installed-payload tamper causes integrity failure;
- Viewer can read readiness through the authenticated Operator API;
- readiness responses do not invent `enabled` or `running` state;
- runtime health remains explicitly not applicable.

## Deliberately incomplete

This checkpoint does not add:

- runtime permission grants derived from approval;
- activation/start/stop lifecycle;
- post-start plugin.ready health probing through the Extension Manager;
- enable/disable persistence;
- update/rollback/repair/remove;
- automatic multi-package plan execution;
- online/offline catalogs;
- bilingual Operator Manager UI.

F-015 therefore remains unchecked in `docs/FEATURE_BACKLOG.md`.

## Next dependency-first slice

The next safe step is the activation contract. It must consume only a `READY_FOR_ACTIVATION` installation, explicitly bridge only reviewed permissions into runtime authority, validate plugin identity/capabilities during startup, persist lifecycle intent safely, and remain blocked during SHOW unless a future explicitly safe policy is defined.
