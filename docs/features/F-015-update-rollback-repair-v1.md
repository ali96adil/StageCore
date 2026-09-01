# F-015 — Extension Update, Rollback and Repair v1

## Purpose

This slice completes the safe installed-extension maintenance lifecycle without weakening F-015 integrity, SHOW, permission or runtime isolation boundaries.

## Stable installation identity

Update and rollback replace the package behind an existing installation while preserving its `installation_id`.

This is intentional: references to an installed extension must not break merely because its package version changes. The operation is therefore not implemented as uninstall followed by install.

## Server-authoritative version plan

A version change begins with `PlanUpdate` and is exposed through:

`GET /api/v1/extensions/installations/{installation_id}/update-plan?target_package_id={package_id}`

The server verifies:

- the installation and target package exist;
- target extension identity and kind match the installed extension;
- the target is a different semantic version;
- target Hub/platform/architecture compatibility;
- currently installed dependents continue to accept the target version;
- required dependency constraints can be solved;
- dependency cycles and blockers;
- required installation steps and warnings.

The server labels the plan `UPDATE` or `ROLLBACK` using semantic-version comparison. Browser code does not infer direction.

## Execution gates

`POST /api/v1/extensions/installations/{installation_id}/update`

re-checks all mutation safety immediately before replacement.

The operation is rejected when:

- an authoritative SHOW Session is active;
- the Plugin is not durably `DISABLED` and observed `STOPPED`;
- a process is still supervised for the installation;
- required dependencies are still missing;
- an installed dependent would be broken by the target version;
- target package compatibility or integrity checks fail;
- StageCore storage reserve policy refuses the materialization.

## Immutable package materialization

The target payload is copied from the immutable StageCore software/Vault object into managed staging, hash/size verified, fsynced and promoted atomically to the target installed path.

The immutable source object is never modified.

After promotion, the durable installation row is replaced transactionally while keeping the same `installation_id`.

## Permission and runtime reset on version change

A new package version may request different behavior even when its extension ID is unchanged. Therefore version replacement deliberately deletes:

- prior `extension_permission_reviews` for the installation;
- prior `extension_runtime_lifecycle` intent/observation row.

The replacement returns to fail-closed default runtime state. Permissions must be reviewed again before the new version can pass readiness and enable.

This prevents a previously approved version from silently transferring authority to different package bytes.

## Old payload cleanup

After the durable replacement succeeds, the old payload is inert because the installation record no longer references it.

StageCore attempts best-effort cleanup of that old payload. Cleanup failure does not roll back the already-committed safe replacement; instead the result reports `OLD_PAYLOAD_CLEANUP_INCOMPLETE` so the Operator can surface a warning.

The stale file is never selected as the active payload by the installation record.

## Repair

`POST /api/v1/extensions/installations/{installation_id}/repair`

repairs the current installed package without changing its version or identity.

Repair:

1. enforces SHOW and stopped-runtime gates;
2. verifies the current installed payload;
3. returns `already_healthy` when hash, size, type and mode are valid;
4. otherwise reconstructs the exact current package bytes from immutable software/Vault storage;
5. atomically promotes and re-verifies the repaired payload.

Because Repair restores the same package identity and bytes described by existing immutable metadata, it does **not** clear permission reviews.

## Operator API and authorization

Planning requires the existing read authority.

Update/rollback and repair require `plugin.manage`, authenticated browser session and CSRF protection. Successful and rejected mutations are written to the security audit service as:

- `extension.package.update`;
- `extension.package.rollback`;
- `extension.package.repair`.

## Operator UI

The bilingual Extensions page presents:

- compatible alternative versions from the local library;
- a server-generated version-change plan;
- blockers, warnings and required dependency steps;
- explicit Update or Rollback action according to server direction;
- a warning that version change clears previous permission approvals and runtime enable intent;
- Repair for the current installed version;
- Arabic/English SHOW and stopped-runtime guidance.

The UI may disable controls based on observed status for usability, but the server remains authoritative.

## Regression coverage

Tests prove:

- update and rollback preserve the installation ID;
- permission review and runtime intent reset after version replacement;
- installed dependent ranges can block a target version;
- missing dependencies must be installed before replacement;
- a tampered payload is repaired from immutable storage;
- non-stopped runtime blocks maintenance;
- Viewer cannot mutate; Owner mutation requires CSRF;
- HTTP update → rollback → tamper → repair succeeds under the expected contract;
- browser direction comes from the server plan rather than local semver logic;
- embedded maintenance UI is composed after the existing uninstall layer.
