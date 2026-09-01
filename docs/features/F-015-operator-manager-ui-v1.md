# F-015 — Operator Extension Manager UI v1

## Scope

This slice exposes the already-proven F-015 Extension Library, installation planning, permission review, readiness assessment and supervised Plugin runtime lifecycle through one bilingual Hub-global Operator page.

It does **not** invent lifecycle actions that the backend cannot yet execute safely. Update, rollback, repair, uninstall, catalog synchronization and installed-manifest export remain deferred until their service-layer contracts exist.

## Operator experience

The Operator sidebar contains a global `Extensions` page. It is Hub-scoped rather than Project-scoped because extension packages, installations, permission reviews and Plugin runtime intent belong to the StageCore Hub.

The page provides:

- local Extension Library package count and cards;
- installed-extension count and cards;
- activation-readiness and running-runtime summaries;
- localized package name and summary from the Extension Manifest;
- version, kind, source, Hub compatibility and production-readiness/trust state;
- requested permission and required-dependency counts;
- verified install-plan review before mutation;
- ordered dependency/root installation using the existing plan steps;
- per-installation permission review and explicit approve/deny decisions;
- readiness checks and blocker/advisory details;
- supervised Plugin desired/observed runtime state and generation;
- enable/disable controls for supported Plugin installations.

ADDON installations truthfully report that the native Plugin runtime is not applicable.

## Authorization

All authenticated StageCore roles may read the Extension Manager because the underlying library/install/readiness/runtime status APIs use the existing project-read permission.

Only roles with the existing `plugin.manage` authority — currently `OWNER` and `TECHNICIAN` — receive mutating controls in the UI.

The browser-side role check is presentation only. Every mutation remains protected by the authenticated API permission and CSRF boundary.

## Installation contract

The UI does not construct its own dependency graph.

For a selected package it requests:

`GET /api/v1/extensions/packages/{package_id}/install-plan`

The server remains authoritative for compatibility, installed-version conflicts, dependency constraints, cycles, candidate selection, warnings and blockers.

When a plan is executable, the UI invokes the returned ordered steps sequentially through the existing verified installation endpoint. A failure stops the browser workflow at that step and is surfaced through normal Operator error handling; the UI never claims an unexecuted remainder succeeded.

## Trust and readiness

Compatibility and production readiness are displayed separately.

A package can be compatible with the current Hub API while still failing the production-ready trust policy. The Manager therefore shows the package `production_ready` state before installation instead of presenting compatibility as equivalent to trust.

After installation, the existing readiness assessment remains authoritative for:

- installed payload integrity;
- package compatibility;
- package trust/production readiness;
- runtime artifact and host compatibility where applicable;
- required dependencies;
- permission review;
- later runtime/isolation checks already defined by F-015 contracts.

The UI does not manufacture a second readiness model.

## Permission review

Requested permissions are read from:

`GET /api/v1/extensions/installations/{installation_id}/permission-review`

An authorized operator can submit explicit `APPROVED` or `DENIED` decisions through the existing permission-review API.

No permission is auto-approved by installing a package or opening the Manager.

## Supervised Plugin runtime

For `PLUGIN` installations the page reads:

`GET /api/v1/extensions/installations/{installation_id}/runtime`

It displays desired state, observed state, generation and the latest runtime error when present.

Authorized enable/disable actions use the existing supervised lifecycle endpoints. The browser never launches an extension process itself and never bypasses runtime isolation, probe, handshake, integrity or broker policy.

## SHOW safety

The page remains readable during SHOW.

Installation, permission decisions and runtime enable/disable continue to be rejected by the service/API layer while an authoritative SHOW Session is active. The Manager explains this boundary but does not attempt to replace it with a UI-only lock.

## Localization and responsive layout

This is a user-facing F-015 slice and therefore follows the F-001 Feature Localization Contract:

- keyed English and `ar-IQ` copy owned by the F-015 asset;
- Arabic-first default consistent with the existing Operator preference;
- RTL-safe logical layout;
- responsive summary, cards, metadata, plan rows, permission rows and readiness rows for narrow viewports;
- no hard-coded LTR requirement for operational labels.

Technical identifiers, protocol values, extension IDs, package IDs and backend error/check codes remain canonical technical tokens where translating them would reduce diagnostic accuracy.

## Asset delivery

`extensions.js` and `extensions.css` are compiled into the Hub binary and served through the same secure, no-store, same-origin Operator asset path as the rest of the local UI.

Regression coverage verifies the compiled asset routes rather than relying only on source-file presence.

## Explicitly deferred F-015 lifecycle operations

This UI slice intentionally does not expose:

- Update or rollback;
- Repair;
- Uninstall/remove;
- online catalog refresh/download;
- one-step offline package import;
- installed extension manifest export/restore.

Those operations remain part of the broader F-015 backlog and must be implemented service-first with SHOW gating, integrity, reference-preservation and audit semantics before buttons are added.

## Acceptance

The slice is accepted when normal Core CI proves:

- embedded extension assets are served by the compiled Hub;
- the Operator shell owns the Extensions tab and assets;
- F-015 uses the existing lifecycle APIs rather than parallel browser logic;
- `OWNER`/`TECHNICIAN` mutation presentation matches `plugin.manage` authority;
- unsupported lifecycle actions are absent;
- bilingual keyed copy and RTL/responsive guardrails are present;
- module lock, Test, Vet, Race and Linux ARM64 CGo-free product builds remain green.
