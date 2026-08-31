# F-015 — Plugin & Add-on Library / Manager — Foundation

**Status:** Foundation slice implemented on `f015/extension-manager-foundation` pending final CI/merge.  
**Scope:** Trusted Extension Library package/manifest contract and authenticated registration API.  
**Not yet:** runtime installation/activation, enable/disable, update/remove, dependency installation, online catalog, or Operator Manager UI.

## Goal

Give StageCore one durable, security-bounded model for Plugin and Add-on packages before exposing lifecycle buttons to operators.

This slice intentionally does **not** create fake Install, Enable, Disable, Update, Repair, or Uninstall controls. Those actions become real only after StageCore has a corresponding tested lifecycle/runtime operation.

## Existing foundations reused

F-015 builds on existing StageCore systems instead of creating parallel package or permission models:

- Software Repository for immutable package metadata and Hub API compatibility;
- Vault for content-addressed package payload storage;
- Plugin Host for runtime plugin protocol/handshake boundaries;
- Plugin Permissions for explicit runtime permission grants;
- local RBAC and `plugin.manage` for extension-management authority;
- Security Audit for management-event history;
- F-012 SHOW rules for mutation safety.

## Schema v16

Migration `00016_extension_library_f015.sql` adds `extension_packages`.

Each Extension Library registration is linked by foreign key to an existing immutable `software_packages` row and stores:

- package ID;
- stable extension ID;
- extension version;
- kind (`PLUGIN` or `ADDON`);
- source (`OFFICIAL`, `LOCAL`, or `COMMUNITY`);
- canonical manifest JSON;
- SHA-256 of that canonical manifest;
- registration actor and time.

There is deliberately no `enabled` or `installed` flag in this foundation. A package being present in the library is not evidence that StageCore can currently execute it.

## Manifest schema v1

An Extension Manifest describes operator-understandable metadata and future lifecycle requirements:

- `schema_version`;
- `extension_id` and `version`;
- `kind` and `source`;
- bilingual English + `ar-IQ` name and summary;
- Hub API compatibility range;
- supported platform and architecture values;
- requested permissions;
- advertised capabilities;
- declared dependencies.

The parser rejects unknown JSON fields rather than silently accepting data StageCore does not understand.

### Localization contract

Every manifest version must carry both English and Arabic (`ar-IQ`) definitions. Arabic fields must contain Arabic text.

This slice adds no Operator Web asset, so it does not create an artificial F-015 entry in the Operator localization asset manifest. When a real Extension Manager UI is introduced, that same change must add keyed Operator localization, Arabic strings, RTL-safe UI, and feature asset ownership under the existing Feature Localization Contract.

## Permission contract

A manifest may request only permissions that StageCore currently understands and can enforce.

The foundation currently recognizes the existing first-party runtime permission surface:

- `network.udp.send`;
- `network.udp.listen`.

Unknown permission strings are rejected. Adding a future permission requires extending the enforceable permission model in the same feature change; manifests cannot invent authority by naming it.

`ADDON` packages cannot request runtime Plugin permissions in this slice.

## Trust and source rules

`source` is not a decorative badge.

### LOCAL / COMMUNITY

Operator-managed registration may register LOCAL or COMMUNITY manifests when the linked software package and manifest metadata are valid.

### OFFICIAL

The normal Operator/API registration path **cannot** self-assert `OFFICIAL`.

`OFFICIAL` registration is reserved for a trusted StageCore-bundled/internal catalog path. In addition, the linked Software Repository package must be production-ready.

A signed local package therefore does not automatically become an official StageCore package.

## Package/manifest binding

Registration verifies the manifest against the already-stored immutable software package:

- `extension_id` must equal Software Repository `product_id`;
- version must match exactly;
- Hub API minimum/maximum must match exactly;
- manifest platform set must include the package platform;
- manifest architecture set must include the package architecture;
- the software package must be compatible with the current Hub API;
- trusted OFFICIAL registration additionally requires a production-ready package.

The manifest is canonicalized and SHA-256 hashed before persistence. Reads re-parse and re-hash the stored manifest and fail closed if the persisted representation is inconsistent.

The package payload itself remains content-addressed in the existing Vault/Software Repository path.

## SHOW safety

Extension Library reads are non-mutating and remain available during SHOW.

Registration is a system-level configuration mutation. The Library service checks the canonical active operational session state and rejects registration while any authoritative SHOW is active with `SHOW_CONFIGURATION_LOCKED` semantics.

This protection is inside the service, not only in the UI.

Future install, update, enable, disable, repair, and uninstall operations must inherit the same or stricter SHOW policy unless a separately proven safe policy is introduced.

## Operator API foundation

Authenticated routes:

- `GET /api/v1/extensions`
- `GET /api/v1/extensions/packages/{package_id}`
- `POST /api/v1/extensions/register`

Read access uses the existing operator read permission. Registration requires `plugin.manage` and CSRF protection through the normal browser API boundary.

Registration attempts can be security-audited without copying package contents or secrets into the audit record.

## Explicit non-goals of this slice

This foundation does **not** yet:

- unpack or install an extension into an executable location;
- activate arbitrary Plugin processes;
- dynamically register new runtime capabilities;
- grant requested permissions automatically;
- mark an extension enabled/disabled;
- resolve or install dependencies;
- compare dependency version ranges against installed state;
- update, rollback, repair, or uninstall packages;
- fetch an online extension catalog;
- import offline extension bundles as a one-step workflow;
- expose a Plugin & Add-on Manager page.

Those capabilities must build on this contract rather than bypass it.

## Verification in this slice

Tests cover:

- strict manifest decoding and unknown-field rejection;
- required English + Arabic text;
- unknown permission rejection;
- self-dependency rejection;
- real package payload import through Vault + Software Repository;
- package/manifest binding;
- schema v16 persistence;
- close/reopen persistence and manifest-hash verification;
- public OFFICIAL-source rejection;
- SHOW mutation rejection;
- Operator authentication/RBAC/CSRF boundaries;
- Viewer read access versus Owner registration authority.

Final merge still requires the normal Core CI gate on the final HEAD: module lock, Test, Vet, Race, and Linux ARM64 CGo-free product builds.

## Next F-015 slices

The intended dependency-first continuation is:

1. installation staging/extraction contract with path traversal and executable integrity defenses;
2. installed-state/lifecycle model distinct from Library presence;
3. dependency resolution and compatibility plan before mutation;
4. permission-review flow before activation;
5. post-install health check and truthful Ready/Failed state;
6. safe enable/disable/remove/update/repair operations with SHOW gating;
7. official bundled/offline catalog and later optional online catalog;
8. bilingual guided Operator Manager UI using keyed localization;
9. backup/export of the installed extension manifest for reproducible servers and future Show Capsules.

The design rule remains: **library presence is knowledge; installation is a controlled mutation; activation is runtime authority. They are not the same state.**
