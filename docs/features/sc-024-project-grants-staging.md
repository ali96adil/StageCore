# SC-024 Phase 1: Owner-staged per-project permission plans

**State: staging only; all real StageCore access control still uses existing global roles.** This is a data model and owner-only policy preparation API, NOT an enforcement switch and NOT a replacement for the existing SHOW authorization system.

## Reason for deliberately staging grants first

Today `userauth.Authorize(role, permission)` gates Operator routes globally. Project cards, direct project IDs, snapshots, media assets, device paths, StageAudio, SSE and GO handlers do not all have project-aware checks. Enabling a single filter on the Project list would be a security illusion and may strand operators in rehearsal or SHOW. No existing account should silently lose access as a side effect of a new migration.

We create `project_access_grants` and an atomically recorded `project_access_grant_changes` history in migration 00053. It does **not** modify `local_users`, `browser_sessions`, existing role permissions, `projects`, runtime snapshots, Cues, any device assignments, or any Show Capsule paths.

For now Owner may **stage** intended capabilities for a single existing enabled non-Owner account on an existing Project. Entries are additive policy *plans*, not grants of immediate authority; no route reads them for effective access. An empty list stages removal. Duplicate, unknown and whitespace-modified capability names fail closed; `project.view` must be included in each nonempty set. All changes, including revoke, are recorded atomically with actor, target, Project, prior and new capability sets.

## Exact API (same-origin browser session; local network / HTTPS)

- `GET /api/v1/security/projects/{project_id}/grants/staging` — Owner only, read staged grants and available capabilities
- `PUT /api/v1/security/projects/{project_id}/grants/staging/{user_id}` — Owner only, replaces one user's staged capabilities. Existing browser `X-StageCore-CSRF` is required.

Example payload:

```json
{
  "capabilities": ["project.view", "media.read", "show.go"]
}
```

Every response explicitly marks `"enforced": false`, `"phase":"STAGING_ONLY"`, and warning `PROJECT_GRANTS_NOT_ENFORCED`. Nothing in this phase grants access to a restricted route or revokes existing global OPERATOR role capabilities. OWNER alone may stage; TECHNICIAN/OPERATOR/VIEWER never manage this policy.

A browser-facing policy editor is deferred until the migration and backend tests have been reviewed; this is an API foundation.

## Phase 2: safe enforcement (separate Draft PRs)

1. Inventory each HTTP handler that accepts ProjectID directly or indirectly, plus Media Vault, Show Capsule exports, Stage Devices and SSE event scopes. Build central project-aware authorization with explicit `project_id` from trusted store data, not browser-provided strings alone.
2. Define a role/capability matrix and migration report for current non-Owner accounts. Explicit OWNER approval of membership before enforcement. Make unassigned Projects invisible/denied only after every server route is secured.
3. Enforce revocation on each request (do not trust stale browser authorizations). Handle in-progress SHOW and emergency stop policies carefully; never allow ACL implementation to block safety-critical outputs. Test direct ID guessing, copied Project, failed media download, session reconnect and cross-project events.
4. Pair with [SC-023 Project Files](https://github.com/ali96adil/StageCore/issues/510) and [SC-021 StageAudio](https://github.com/ali96adil/StageCore/issues/502) to ensure both only use Vault content explicitly authorized for their Project.
5. Hardware-attended regression on Pi 4 with representative owner/operator/technician/viewer identities, offline network, real Cue GO and blackout. Only then consider explicit enforcement activation and deployment.

## No merging or deployment

The PR remains Draft on a feature branch. No existing Pi installation or production SHOW state is changed. **CI green does not mean project-scoped ACL is enforced.**
