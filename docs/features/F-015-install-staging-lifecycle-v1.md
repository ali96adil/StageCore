# F-015 — Secure Install Staging & Installed Lifecycle v1

**Base:** F-015 Extension Library foundation merged on `main` as `49d711a3c83948835832700bd848d17d71caefca`; post-merge Core CI #392 passed.  
**Scope:** materialize a registered extension package into StageCore-managed storage and persist truthful installed state.  
**Not yet:** bundle extraction, dependency resolution, permission approval, runtime activation, enable/disable, health readiness, update, repair, rollback, remove, or Operator Manager UI.

## State boundary

F-015 keeps three concepts separate:

1. **Library presence** — StageCore knows a validated immutable package and manifest.
2. **Installed** — the exact package payload has been copied into managed install storage, integrity-verified, made non-writable/non-executable, and recorded persistently.
3. **Activated** — future runtime authority to execute or load the extension.

This slice implements only step 2.

`INSTALLED` therefore does **not** mean `Enabled`, `Ready`, `Healthy`, or `Running`. Those fields are intentionally absent from the API and schema.

## Schema v17

Migration `00017_extension_installations_f015.sql` adds `extension_installations`.

Each installation records:

- installation ID;
- linked registered Extension Library package ID;
- lifecycle state (`INSTALLED` only in this version);
- managed relative payload path;
- payload SHA-256;
- payload size;
- installing actor;
- installation time.

The package ID is unique so the same immutable package cannot create duplicate installed-state rows.

Only `INSTALLED` is accepted by schema v17. Later lifecycle states must be introduced by an explicit migration together with real operations; the database does not advertise fake enable/disable/remove capability in advance.

## Managed storage layout

The Hub uses:

`<data-root>/extensions/`

with private managed directories:

- `staging/`
- `installed/<extension-id>/<version>/<package-id>/payload.pkg`

The API persists and returns only the relative path under `installed/`; absolute server filesystem roots are not exposed as installation metadata.

The package is deliberately still opaque. F-015 does not invent a ZIP/TAR layout before StageCore has a canonical extension bundle format.

## Install transaction

For a registered Library package, Install performs:

1. reject the mutation during authoritative SHOW;
2. validate Library manifest integrity and current Hub compatibility;
3. reject a second installed version of the same extension until update/rollback semantics exist;
4. check the configured runtime storage reserve for the planned package bytes;
5. open the immutable package through the existing Software Repository / Vault path;
6. copy into a private staging file while calculating SHA-256;
7. require copied byte count and SHA-256 to match immutable Software Repository metadata;
8. `fsync` the staged payload;
9. change the staged payload to mode `0440` so it remains non-writable and non-executable before activation;
10. atomically rename it into the managed installed location;
11. sync the containing directory;
12. re-verify file type, mode, size and SHA-256;
13. persist the `INSTALLED` row.

If a crash happens after atomic promotion but before database persistence, a retry recognizes the existing deterministic payload path, re-verifies it, and completes the DB registration. The operation is therefore idempotent for the same package.

## Filesystem safety

All installed path components are derived from already validated extension ID, version and package ID values. No caller-supplied filesystem path is accepted.

Managed extension directories must:

- be real directories, not symlinks;
- not be group/world writable.

Reads re-check every managed parent directory so replacing an installed path component with a symlink after installation fails closed.

Payload verification requires:

- a regular non-symlink file;
- no write or execute bits;
- exact expected size;
- exact expected SHA-256;
- stable file identity across open/hash verification.

Integrity failure is surfaced as `EXTENSION_INSTALL_INTEGRITY_FAILED` rather than returning stale `INSTALLED` state as healthy.

## Storage reserve

Install uses the same StageCore `storagehealth.Policy` configuration as the Hub (`RuntimeReserveBytes` and warning policy).

Before copying a package, the installer calls capacity admission for the package size. If the copy would breach the protected runtime reserve, installation is rejected with `EXTENSION_INSTALL_STORAGE_RESERVE` / HTTP 507.

The staging file and final payload are the same file promoted by rename, so planned capacity is one package payload, not two simultaneous copies inside install storage.

## SHOW policy

Install is a system-level mutation and is rejected while an authoritative SHOW is active using existing `SHOW_CONFIGURATION_LOCKED` semantics.

Installed-state reads remain available during SHOW because they are diagnostics only.

The SHOW guard is enforced inside the Installer service, not only at HTTP/UI level.

## Version policy in v1

Only one package version per extension may be installed in this slice.

Attempting to install another package version for the same extension returns `EXTENSION_VERSION_ALREADY_INSTALLED`.

This is intentional. F-015 will not create side-by-side versions until update, rollback and activation selection semantics are explicit and tested.

## Operator API

Authenticated routes added by this slice:

- `GET /api/v1/extensions/installations`
- `GET /api/v1/extensions/installations/{installation_id}`
- `POST /api/v1/extensions/packages/{package_id}/install`

Read access uses the existing operator read permission. Install requires existing `plugin.manage` authority and the normal browser CSRF boundary.

Install attempts are written to Security Audit when audit service is available.

No Operator Web buttons are added in this slice, so there is no new user-facing asset requiring an F-001 localization-manifest entry yet.

## Verification

Tests cover:

- schema v17 migration;
- real Vault + Software Repository package import;
- Library registration before install;
- staged payload byte/hash verification;
- non-writable/non-executable installed mode;
- empty staging directory after successful install;
- idempotent repeated install;
- close/reopen persistence;
- second-version rejection;
- managed-directory symlink rejection;
- post-install parent-symlink substitution rejection;
- injected runtime-reserve rejection before payload copy;
- SHOW install rejection and post-SHOW success;
- Viewer read versus Owner install RBAC;
- CSRF-protected install API;
- tampered installed permissions producing an integrity conflict;
- absence of fake `enabled` / `running` response fields.

## Next F-015 slice

The next dependency-first slice should add a **dependency resolution / install plan** that evaluates required and optional dependency version ranges against Library and Installed state before any multi-package mutation.

After that:

1. permission review/grant flow;
2. post-install health/readiness contract;
3. runtime activation and real enable/disable semantics;
4. update/rollback/repair/remove;
5. bundled/offline catalog and later optional online catalog;
6. bilingual guided Operator Manager UI under the Feature Localization Contract;
7. installed-extension export/restore for reproducible StageCore servers and future Show Capsules.
