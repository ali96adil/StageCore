# SC-024 — Project-scoped Owner grants: phase 1 (persistence + policy, NOT yet enforced)

**Draft / non-deployed foundation.** This phase adds migration `00053_project_permission_foundation.sql` and the standalone `internal/projectaccess` policy. **It does NOT replace current system-wide UserRole authorization in the HTTP API.** A successful unit test here does not imply that ordinary operators are currently restricted to their assigned Projects. Do not expose new media endpoints or ship this to a production SHOW as an ACL fix.

## Current security limitation

Existing StageCore `userauth.Authorize(role, permission)` is a global permission check; in the deployed application, OPERATOR and VIEWER project lists and direct Project URLs remain globally readable. The new policy `Check(userID, projectID, permission)` is deliberately **not wired to those endpoints** yet. Introducing a project picker that only hides unassigned Projects is insufficient; file downloads, Cue IDs, runtime snapshots, device routes, SSE and ShowCapsules also require server-side enforcement.

## Future permission model

| Capability | Purpose | Default non-OWNER |
| --- | --- | --- |
| `project.view` | Find/open the Project's overview | Denied |
| `project.rename` | Rename Project | Denied |
| `cue.edit` | Edit Draft Cue | Denied |
| `media.read` | Read Vault-scoped files | Denied |
| `media.upload` | Import media to Project Vault | Denied |
| `media.manage` | Delete/organize media references | Denied |
| `snapshot.publish` | Publish Runtime Snapshot | Denied |
| `rehearsal.control` | Control rehearsals | Denied |
| `show.go` | Execute GO | Denied |
| `show.enter_exit` | Start/end Show Mode | Denied |
| `device.configure` | Change assigned devices | Denied |
| `capsule.export` | Export Project show capsule | Denied |

All non-OWNER capabilities must be explicitly granted by an **enabled current OWNER**. Every non-empty membership must include `project.view`. The Owner's authority is universal and is obtained from the **current database role**, not an earlier browser-session claim. Non-owner OPERATOR/TECHNICIAN/VIEWER are all restricted in this future policy, regardless of former global-role privileges. No grants are copied when a Project is duplicated. A disabled user has no access; grant replacement/revocation is immediately reflected by uncached queries. Grants survive account disabling and would become effective again if the same account is re-enabled; Owner should review before enabling. A later change may intentionally revoke grants on account disable after explicit approval.

**No automatic migration of historic OPERATOR accounts to full access** should occur. The future cutover must provide an Owner-controlled permissions review and rehearsed rollback plan so operators do not lose emergency SHOW access unexpectedly.

## Persistence

- `project_memberships`: unique `(project_id, user_id)` with Owner grantor and creation/update timestamps; FK to Projects and Users.
- `project_permission_grants`: `(project_id, user_id, permission)` with grantor and time, cascade-delete when membership removed.
- `SetPermissions` is an atomic Owner-only **replace** operation: a new exact allowlist supersedes the old set, and an empty list deletes the membership. Partial updates do not leave unauthorized permissions from earlier grants.
- `Check` consults enabled user and live grants; fails closed for unknown users, unknown capabilities and missing Projects. `AccessibleProjectIDs` supplies a future restricted Project-list query.
- No HTTP route, UI menu, auto-invite, or GO/runtime code has been changed in this PR.

## Mandatory follow-up implementation

1. Owner-only secured API for member selection and grant/revoke, using existing CSRF and durable security audit, with clear deny-by-default UI and explicit conflict checks.
2. Complete endpoint inventory and new authenticated **Project-scoped** policy middleware. Every direct-ID endpoint must resolve authoritative ProjectID **before** checking access, including:
   - Project list/get/create/duplicate/rename and all revisions, Cue Actions, composite Cue links, templates, notes and media references.
   - Publish/validate/export and all Show Capsules, backup scopes, vault blob retrieval, StageAudio file IDs, tablets and capture uploads.
   - SHOW/REHEARSAL enter/exit, live GO/JUMP, lights, DMX USB/ESP32, runtime status and hardware commands.
   - SSE event broadcasts and live session subscriptions; ensure event sources cannot leak Project names or private media.
   - Companion and Stage Devices use separate device identities; preserve their existing authenticated transport and add project assignment checks at the Hub boundary, NOT browser credentials on devices.
3. Review emergency blackout separately: legitimate failsafe/safety commands must not get stranded by revoking normal GO permission during an active show. Design with operator and theatre staff before shipping.
4. User management/account disable: current UserRole and assigned capabilities must be reviewed together. Changing grants while a socket or tab is open must revoke access at the server; do not cache authorization only in frontend state.
5. Backward-compatibility preview of which users currently have all-project access, selective migration to assignments with Owner approval, backup and rollback; then switch enforcement atomically and verify real Raspberry Pi 4 on a non-show session.
6. **Only then** implement SC-023 Project Files as a logical Vault workspace with secure per-Project access, not a shared unrestricted file browser.

## Test coverage in this phase

Go tests create fresh migrated SQLite DBs and verify Owner override, default-deny OPERATOR/TECHNICIAN/VIEWER, Project A vs B isolation, distinct capabilities, Owner-only grant changes, immediate permission replacement/revocation, disabled user denial, duplicate Projects not inheriting membership, and invalid IDs/capabilities rejected.

**Explicit status:** persisted grants are preparatory data only. Current StageCore authorization is unchanged until later full review, integration, CI and operator approval. No merge or Pi deployment is performed.
