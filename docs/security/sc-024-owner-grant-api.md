# SC-024 Phase 2 — Owner-only Project grant API (prepared, not enforced)

**Draft/stacked on #518**. No production install, main merge, Project picker, or GO permission cutover. This API prepares a future permission model and must **not** be mistaken for current enforcement. All successful responses include `"enforced":false` and `"phase":"PREPARED_NOT_ENFORCED"`.

## HTTP contracts

`GET /api/v1/projects/{project_id}/access/users/{user_id}`
- Current enabled **OWNER**, authenticated same-origin local/TLS session, `user.manage` role permission. GET uses no CSRF and returns only that user's prepared permissions for that Project; anonymous or non-Owner rejected.
- Result: `{project_id,user_id,permissions:[...],enforced:false,phase:"PREPARED_NOT_ENFORCED"}`.
- Missing user/Project: 404. No wildcard or unscoped listings. Account password and token are not returned.

`PUT /api/v1/projects/{project_id}/access/users/{user_id}`
- Requires the same authenticated Owner **and valid CSRF token**. Accepts only an explicit JSON array:
  - `{"permissions":["project.view","show.go"]}` — replace, not merge
  - `{"permissions":[]}` — atomically revoke Project membership and all associated grants.
- Missing/null/invalid field cannot be interpreted as revoke.
- Requests rejected while **any REHEARSAL or SHOW session is operational**; unknown session state fails closed.
- `project.view` must accompany any nonempty permission set. Enabled target must be OPERATOR or TECHNICIAN for control operations; VIEWER may receive only project.view/media.read. OWNER accounts are not grant targets. No permission copied on Project duplication.
- All changes are written transactionally by `SetPermissionsAudited`: `securityaudit.Prepare` validates and redacts the event **before** opening a SQLite writer transaction, then `securityaudit.AppendPreparedTx` writes the `project.permissions.replace` audit record **within that same grant transaction**, including the target user and exact grant set. If auditing fails the mutation is rolled back. This pre-redaction ordering is required because StageCore uses a single-connection SQLite pool and Secret Store redaction may query the database. The existing standalone `securityaudit.Append` behavior stays unchanged.

No `DELETE` endpoint is needed: explicit `PUT []` revokes. This route does not enumerate all users; the existing Owner user-management route lists accounts.

## Security cutoff NOT finished

**Current StageCore's global Role-based authorization remains active on all existing routes.** An Operator who has not yet been assigned a project will **still** see and operate existing Projects through legacy routes. The new APIs only manage prospective data. Do not advertise project privacy until the next phases protect **every** endpoint. In particular:

- Core Projects list/open/duplicate/rename, Cues, groups and edit, published Snapshot/authoring, runtime status and GO/JUMP/start/stop/blackout.
- Vault media, file upload/download, Content Hash/Content Version lookup, Project Files and StageAudio local media.
- Device assignment, ESP32/USB DMX, StageLaser, tablets, Companion and OSC routing; IDs must resolve server-side to the authoritative Project.
- Show Capsules, export/import, backup, screenshot/streaming, dynamic SSE subscriptions and shared/global configuration.
- Security-specific global functions (emergency blackout; platform-wide settings) need an explicitly reviewed policy separate from normal Project GO.

**Race note:** the Session freeze is a read-only gate and does not atomically exclude a concurrently starting SHOW. Grant changes currently have no effect on GO, but full runtime enforcement must use a dispatch-time authority fence before SHOW.
**Audit note:** the transactional success event is atomic to grants, not a replacement for a separate security-denial audit of blocked unauthenticated requests.
**Migration note:** #517 and #518 are mutually incompatible alternative goose migration 53 approaches; only one may ever be merged.

## Software tests and rollout gates

- Owner-only read/write, Origin/CSRF, explicit array, no implicit revoke, role ceiling, Project A/B isolation.
- Stable readback and immediate revoke from the *prospective* database evaluator.
- Injected audit INSERT failure -> rollback and original permissions retained.
- Active operational SHOW blocks mutation; no GO/DMX code modified.
- Go 1.26/1.27 CI/race/vet and ARM64 build to be verified.

This phase can be implemented and reviewed offline. **No physical Pi 4 is needed until final atomic Project access enforcement and GO/physical-output qualification.**
