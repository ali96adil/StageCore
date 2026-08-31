# F-015 Activation Staging Gate v1

Status: implementation checkpoint

## Purpose

Prove that a PLUGIN which is already `READY_FOR_ACTIVATION` can pass a host-specific, integrity-preserving staging check without executing extension code.

This slice intentionally stops before process creation.

## Why execution remains blocked

StageCore permission review and `capability_permissions` currently constrain which capability requests the Hub may authorize. They do not yet provide an operating-system sandbox around arbitrary extension process startup.

A spawned process would inherit ambient service-account access before the Hub sends any capability request. Therefore this checkpoint treats execution isolation as a hard prerequisite rather than assuming signing metadata or logical permissions are a sandbox.

Successful staging returns:

- `status: STAGING_VERIFIED`
- `execution_authorized: false`
- `execution_blocker: RUNTIME_EXECUTION_ISOLATION_REQUIRED`

## Preconditions

The staging check requires:

- verified installed-payload integrity;
- compatible package/API metadata;
- production-ready package policy;
- valid Runtime Artifact Contract v1;
- package/ELF architecture consistency;
- package platform and architecture matching the current Hub host;
- satisfied required dependencies;
- explicit permission review for the exact installation identity;
- no active SHOW Session.

The SHOW gate is checked before staging and again before the staged copy is accepted for cleanup/return.

## Managed staging behavior

StageCore uses:

`<extensions-root>/runtime/staging-check`

For each check it:

1. re-verifies the installed artifact through the Installer;
2. copies the artifact to a managed `stage-*.bin` temporary file;
3. calculates SHA-256 while copying;
4. requires exact size and hash match with the installation record;
5. sets the temporary copy to mode `0400`;
6. verifies the staged file is regular, non-symlink, read-only and non-executable;
7. re-verifies the installed source after staging;
8. removes the temporary copy before returning;
9. syncs the staging directory after create/remove boundaries.

Startup cleanup removes only stale managed `stage-*.bin` files. Any unexpected entry causes initialization to fail closed rather than deleting an unknown file.

The immutable installed `payload.pkg` remains mode `0440` and is never made executable.

## Permission boundary

The staging result may report the permissions explicitly approved for that exact installation so the future execution design has a deterministic authority input.

This slice does not write to `plugin_permission_grants` and does not create persistent runtime authority.

## Operator API

Requires plugin-management permission:

`POST /api/v1/extensions/installations/{installation_id}/activation-staging-check`

Important responses include:

- `200` with `STAGING_VERIFIED` and explicit execution blocker;
- `409 / SHOW_CONFIGURATION_LOCKED`;
- `409 / EXTENSION_ACTIVATION_NOT_READY` with readiness assessment;
- `409 / EXTENSION_ACTIVATION_STAGING_INTEGRITY_FAILED`;
- `404 / EXTENSION_INSTALLATION_NOT_FOUND`.

When security audit is available, attempts are recorded as:

`extension.activation.staging_check`

## No runtime claims

This endpoint does not create or claim:

- `enabled`;
- `running`;
- `healthy`;
- `plugin.ready`;
- process ID;
- durable permission grant.

No extension process is started.

## Physical Raspberry Pi qualification

After final PR CI and post-merge CI are green, ARM64 physical qualification should confirm on the real StageCore data root:

1. readiness reports `RUNTIME_ARTIFACT_VERIFIED` and `RUNTIME_HOST_COMPATIBLE`;
2. staging returns `STAGING_VERIFIED`;
3. response reports `execution_authorized:false` and `RUNTIME_EXECUTION_ISOLATION_REQUIRED`;
4. installed payload remains `0440`;
5. `runtime/staging-check` is empty after the request;
6. no extension process appears;
7. no durable runtime permission grant is created;
8. active SHOW blocks the staging operation.

Only after this gate is physically proven should F-015 proceed to an enforceable runtime isolation/trust design and then a live `plugin.ready` probe.
