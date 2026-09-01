# F-015 — Operator Extension Manager UI v1

## Scope

The Hub-global `Extensions` page is the bilingual Operator surface for the F-015 Extension Manager.

It exposes only service-layer operations that StageCore can execute safely. The current UI covers:

- local Extension Library browsing;
- verified install-plan review and ordered dependency installation;
- installed-extension inspection;
- explicit permission review;
- readiness assessment;
- supervised Plugin enable/disable;
- safe uninstall;
- dependency-aware update and rollback;
- immutable-package repair.

Catalog import/synchronization and installed-manifest export/restore remain separate later F-015 slices.

## Operator experience

The page is Hub-scoped because extension packages, installations, permission reviews and Plugin runtime intent belong to the StageCore Hub rather than to one Project.

Each installed extension card shows its immutable package identity, version, permission state, readiness state and, for Plugins, desired/observed runtime state. Authorized operators also receive maintenance and removal controls.

ADDON installations truthfully report that the native Plugin runtime is not applicable.

## Authorization

Authenticated roles may inspect the Manager through the existing read permission. Mutating controls are shown only to roles with `plugin.manage`, currently `OWNER` and `TECHNICIAN`.

The browser-side role check is presentation only. Every mutation remains protected by the authenticated API permission and CSRF boundary.

## Installation and dependency planning

The UI never constructs a dependency graph itself.

For a new package it requests:

`GET /api/v1/extensions/packages/{package_id}/install-plan`

The server remains authoritative for compatibility, installed-version conflicts, dependency constraints, cycles, candidate selection, warnings and blockers. Executable steps are applied in the exact order returned by the server, stopping on the first failure.

## Permission review and readiness

Permission decisions are explicit. Installing a package, changing version or opening the Manager never grants a permission automatically.

The existing readiness assessment remains authoritative for payload integrity, compatibility, trust, runtime artifact compatibility, dependencies, permission review and runtime/isolation checks.

## Supervised Plugin runtime

For `PLUGIN` installations the page reads:

`GET /api/v1/extensions/installations/{installation_id}/runtime`

It displays desired state, observed state, generation and the latest runtime error. Enable/disable remains under the Runtime Supervisor; the browser never launches a process or bypasses isolation, probe, handshake, integrity or network-broker policy.

## Update and rollback

Version maintenance is plan-first:

`GET /api/v1/extensions/installations/{installation_id}/update-plan?target_package_id={package_id}`

The browser does not compare semantic versions to decide whether the operation is an update or rollback. Direction is returned by the server.

Before execution the operator sees blockers, warnings and required dependency steps. If dependencies are required, the UI can install the verified steps first and then request a fresh plan.

Execution uses:

`POST /api/v1/extensions/installations/{installation_id}/update`

Changing version preserves the stable `installation_id` but deliberately clears previous permission reviews and runtime enable intent. The replacement version therefore returns to a fail-closed state and permissions must be reviewed again before enable.

## Repair

Repair uses:

`POST /api/v1/extensions/installations/{installation_id}/repair`

It reconstructs the installed payload from the immutable local software/Vault object and verifies it against the stored hash and size. Repair does not change version and does not clear approved permissions.

## Uninstall

Uninstall removes the installation and its runtime state while leaving the immutable package in the local library for possible reinstall or rollback use elsewhere.

Installed dependents and active Plugin runtime state can block removal.

## SHOW and runtime safety

The Manager remains readable during SHOW, but lifecycle mutations are rejected by the backend while an authoritative SHOW Session is active.

Update, rollback, repair and uninstall additionally require a Plugin to be `DISABLED` with observed state `STOPPED`. UI button state is only advisory; the server re-checks the condition immediately before mutation.

## Localization and responsive layout

F-015 Operator copy is bilingual English/`ar-IQ`, Arabic-first with RTL-safe logical layout. The maintenance layer owns bilingual copy for version-reset consequences, update/rollback planning, repair, blockers and SHOW/runtime safety.

Technical identifiers and backend error/check codes remain canonical tokens where translating them would reduce diagnostic accuracy.

## Asset delivery

`extensions.js`, `extensions.css`, the uninstall layer and the maintenance layer are compiled into the Hub binary.

The Hub composes the uninstall and maintenance JavaScript in deterministic order through the same secure, no-store, same-origin asset route. Regression coverage verifies both the embedded asset and composition order.

## Remaining F-015 UI-adjacent work

The current Manager intentionally does not yet claim completion for:

- official bundled/offline catalog import and optional online catalog synchronization;
- installed extension manifest export/restore.

Those require their own service-first contracts before final F-015 closure.

## Acceptance

Core CI must prove:

- embedded extension assets are served by the compiled Hub;
- read and mutation authority remain separated by existing RBAC/CSRF boundaries;
- install/update plans are server-authoritative;
- update/rollback direction is not inferred in browser code;
- update preserves installation identity but resets permission/runtime intent;
- repair restores a tampered payload from the immutable package;
- SHOW and runtime-stop gates remain fail-closed;
- bilingual maintenance copy and responsive/RTL guardrails remain present;
- module lock, Test, Vet, Race and Linux ARM64 CGo-free product builds remain green.
