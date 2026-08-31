# F-015 — Permission Review Foundation v1

## Status

Implementation checkpoint for F-015 Plugin & Add-on Library / Manager.

This checkpoint introduces explicit, persistent review of permissions requested by an installed extension. It does **not** grant runtime authority and it does **not** activate an extension.

## User goal

Before StageCore can later activate a Plugin or Add-on, an authorized operator must be able to see the permissions requested by the exact installed package and explicitly approve or deny each one.

The system must never treat installation as implicit permission approval.

## State model

Permission review is derived per `installation_id` from the immutable installed package manifest and persistent review decisions.

Review statuses:

- `NOT_REQUIRED` — the installed manifest requests no permissions;
- `PENDING` — at least one requested permission has no decision and none are denied;
- `APPROVED` — every requested permission is explicitly approved;
- `DENIED` — at least one requested permission is explicitly denied.

Each requested permission has one of:

- `PENDING` — no persisted decision exists;
- `APPROVED` — an authorized operator approved it;
- `DENIED` — an authorized operator denied it.

## Installation-scoped decisions

Decisions are keyed by `installation_id + permission`, not only by extension ID.

This is deliberate. A future update that installs a different package/version must not silently inherit approval from an older installed package. The new installation receives its own review state based on its own manifest.

## Persistence

Schema v18 adds `extension_permission_reviews`.

Each decision stores:

- installation ID;
- requested permission;
- decision;
- reviewing actor;
- review timestamp.

The installation foreign key uses delete restriction so review history cannot become detached from its installed package record.

## Integrity rules

The review service always loads the installation through the verified Installer path first, so installed payload integrity remains authoritative.

A decision may be created only for a permission present in the installed package manifest.

If persisted review rows refer to permissions no longer present in the verified manifest, the review is treated as an integrity failure rather than silently accepted.

## Security boundary

Reading review state requires normal project-read permission.

Changing a decision requires `plugin.manage`, authenticated browser state, and the existing CSRF protections.

All successful and rejected Operator review mutations are eligible for Security Audit recording.

## SHOW policy

Permission decisions are configuration/lifecycle mutations and are blocked while a SHOW session is active.

Read-only review remains available during SHOW.

The SHOW check is enforced in the review service itself, not only by Operator UI/API code.

## Critical separation from runtime grants

Permission review decisions are intentionally separate from the existing `plugin_permission_grants` runtime authority store.

An `APPROVED` review means only:

> the operator reviewed and accepted the permission request for this exact installed package.

It does **not** mean:

- the permission has been granted to a running plugin;
- the extension is enabled;
- the extension is ready;
- the extension is healthy;
- the extension is running;
- the extension may execute during SHOW.

The activation slice must explicitly bridge reviewed permissions into runtime authority under its own safety contract. This checkpoint does not do so.

## Operator API

Read review:

`GET /api/v1/extensions/installations/{installation_id}/permission-review`

Set one requested permission decision:

`PUT /api/v1/extensions/installations/{installation_id}/permissions/{permission}`

Request body:

```json
{
  "decision": "APPROVED"
}
```

Supported decisions are `APPROVED` and `DENIED`.

## Acceptance covered by this checkpoint

- schema v18 migration;
- initial `PENDING` state for permission-bearing installations;
- `NOT_REQUIRED` for installations requesting no permissions;
- partial approval remains `PENDING`;
- all approved derives `APPROVED`;
- any denied derives `DENIED`;
- decisions persist independently of reviewer object lifetime;
- unrequested permissions cannot be reviewed;
- review approval does not create runtime permission grants;
- review mutation is blocked during SHOW and succeeds after SHOW exit;
- Viewer can read but cannot mutate review state;
- Owner mutation requires the existing authenticated/CSRF path;
- Operator API returns explicit review state and errors.

## Deliberately incomplete

This checkpoint does not add:

- automatic runtime permission grants;
- post-install health checks;
- `READY` lifecycle state;
- activation or execution;
- enable/disable;
- update/rollback/repair/remove;
- automatic multi-package plan execution;
- bilingual Operator Manager UI.

F-015 therefore remains unchecked in `docs/FEATURE_BACKLOG.md`.

## Next dependency-first slice

The next safe layer is extension readiness/health assessment: verify installed integrity, dependency satisfaction, permission-review status, and extension-specific health prerequisites before introducing activation authority.
