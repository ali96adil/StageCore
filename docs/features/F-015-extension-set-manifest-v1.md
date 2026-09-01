# F-015 — Extension Set Manifest v1

## Purpose

Extension Set Manifest v1 provides a portable, content-bound inventory of the extensions installed on a StageCore Hub. It is intended for backup, migration, offline recovery and repeatable setup without treating local database identifiers or historical runtime state as portable truth.

## Export contract

`GET /api/v1/extensions/set-manifest` exports JSON using format `stagecore-extension-set-v1` and schema version `1`.

Each entry records portable content identity:

- extension ID and version;
- kind and source;
- manifest SHA-256;
- installed payload SHA-256 and exact byte size;
- platform and architecture.

The export deliberately omits:

- local `installation_id` values;
- local `package_id` values;
- permission review decisions;
- observed runtime state;
- runtime desired/enable state;
- transient runtime process information.

The export operation verifies that installed payload metadata still agrees with the immutable package metadata before producing the manifest.

## Restore trust model

Restore never treats the exported file as a package source or as an authority to grant permissions.

`POST /api/v1/extensions/set-manifest/restore-plan` parses the manifest strictly and resolves every entry against packages already present in the destination Hub's local Extension Library. A candidate must match the exported identity exactly, including version, kind, source, manifest hash, payload hash, payload size, platform and architecture.

Required dependencies must also be present in the set at a compatible version. Optional dependencies remain optional, matching the normal StageCore dependency solver semantics. Required packages are ordered dependency-first before installation.

A restore plan is one of:

- `READY` — one or more exact packages can be installed;
- `NOOP` — the Hub already has the exact set installed;
- `BLOCKED` — restore cannot proceed safely.

Typical blockers include unavailable exact packages, incompatible exact packages, a different artifact already installed, a missing required dependency, a dependency version mismatch, or a dependency cycle.

## Restore execution

`POST /api/v1/extensions/set-manifest/restore` requires `plugin.manage` authorization and is blocked during an active `SHOW` session.

Execution re-plans from the supplied manifest immediately before installation. New entries are installed through the normal verified installer path rather than by writing installation records directly.

Newly restored Plugins remain `DISABLED`. Permission approvals are not restored. The Operator must perform a fresh permission review and normal readiness/runtime enable flow on the destination Hub.

This separation is intentional: a portable backup can describe which code was installed, but it cannot authorize that code to regain capabilities or start running on another Hub.

## Operator UI

The bilingual Operator Extension Manager exposes a **Backup and restore / النسخ والاستعادة** panel.

The guided flow is:

1. Export the current extension set as JSON.
2. Choose an Extension Set JSON file on the destination Hub.
3. Review the server-generated restore plan and blockers.
4. Confirm execution only when the plan is `READY`.
5. After restore, review permissions and explicitly enable Plugins as needed.

The UI never invents restore readiness locally; plan and execution decisions come from the authenticated server APIs.

## Safety properties

- No package bytes are embedded in the Extension Set Manifest.
- No local database identifiers are portable authority.
- Unknown JSON fields are rejected.
- Input size is bounded.
- Exact package content identity is required.
- Restore cannot import permission approvals.
- Restore cannot import runtime enable intent or observed runtime truth.
- New Plugins remain disabled.
- Restore remains subject to normal SHOW mutation locking and installer integrity checks.
- Reapplying an already satisfied exact set is idempotent and returns `NOOP`.

## Relationship to offline bundles

Extension Set Manifest v1 is an inventory/restore description, not an offline software bundle. If an exact package is not already available in the destination Hub's local Extension Library, the Operator must first import or synchronize that package through the existing trusted catalog/offline bundle workflow. Only then can the restore plan become ready.
