# SC-024 Project-scoped Operator permissions — data foundation

**Status:** Draft design + persistent capability grants. **NOT an enforced ACL in production or Operator HTTP.** No deployed Pi 4 files, Runtime, GO, or authentication role behavior is modified by this phase.

## Request and security boundary

OWNER may oversee every existing Project. Individual OPERATORS should access only Projects explicitly granted to them, with distinct permissions for Cues, files, published snapshots and GO. UI filtering is never a security boundary.

The existing StageCore role map in `internal/userauth/service.go` and many API handlers still provide **global** access. This PR deliberately does not change those routes while the full inventory is being reviewed. Claiming that a Project is private before those paths are protected would be inaccurate.

## What is implemented

Migration `00053_project_capability_grants.sql` defines normalized `(project_id, user_id, capability)` grants with foreign keys, no default wildcard, grants traceable to the authorizing OWNER and a timestamp. The existing `projects`, `local_users`, Cues, snapshots and Vault data are unaffected.

`userauth.Service` now exposes internal repository primitives:
- `ReplaceProjectGrants(ctx, ownerID, userID, projectID, capabilities)`: transactionally replaces permissions for a non-owner user for a single existing Project, including empty list for revoke. Fresh role and enabled flag read from SQLite; an OPERATOR cannot grant, and no unknown capability/wildcard is accepted.
- `CheckProjectCapability(ctx, userID, projectID, capability)`: dynamic permission evaluation. Existing OWNER bypasses *grants* but cannot access a nonexistent Project. Everyone else needs an exact matching grant; disabled users are denied; an expired or stale grant cannot be cached in the service.
- `ListAccessibleProjectIDs`: future filtered Project selector; non-owners see Projects with explicit `project.view`. The current HTTP `GET /api/v1/projects` is **not** switched to this function yet.
- `ListGrantsForProject`: OWNER-only read of one user's permissions for one Project.

Explicit capabilities:
`project.view`, `project.rename`, `project.edit`, `cue.edit`, `snapshot.publish`, `rehearsal.control`, `show.go`, `show.enter_exit`, `device.configure`, `media.view`, `media.upload`, `media.manage`, `audio.control`, `project.export`.

This foundation uses **explicit** capabilities rather than deriving `show.go` from `project.view`; a future UX will offer presets but enforce the expanded set at the backend.

## What must happen before enabling this policy

1. Inventory all Project-bearing routes and data lookup paths (Project list, dashboard, Cue authoring, published Snapshot, local media/Vault by IDs and SHA, file download/uploads, Show Capsules/export, Stage Devices/DMX, machine roles, Companion, tablet commands, StageAudio, websocket/SSE and GO). Apply shared Project capability authorization on the **backend** to each path, not only the menu.
2. Owner-only authenticated APIs with CSRF and audit for grant/revoke/list, with validated account IDs and Project IDs. A caller must never supply `ownerID` without authenticating it first.
3. A safe migration plan for existing OPERATOR/TECHNICIAN/VIEWER accounts. Review existing access, assign required Projects, and communicate new restrictions. Default-deny after cutover must not unexpectedly block an ongoing theatre SHOW. Maintain Operator-only Show Mode and a separately qualified safety/blackout authority.
4. Avoid TOCTOU: GO and device output must revalidate current authority at **dispatch**, atomically relative to show/session transition. Login/session changes and revoked permissions need prompt effect even on already authenticated browsers.
5. Owner Web UX: Users → Project membership → granular capabilities (or role presets) with confirmation, clear distinction between project files and Project-wide global admin permissions.
6. Project duplicates should never copy account grants automatically. Vault items must not leak across Projects through raw content hashes or shared object paths.

## Qualification

- Unit tests: owner override, default-deny, project A vs B isolation, capability independence, Owner-only grant management, no wildcard/duplicates, atomic replace, immediate revoke/disable, duplicate Project has no inherited grant.
- Integration tests not yet written: every HTTP project route including direct IDs/stream endpoints must deny cross-Project actors. SHOW/GO behavior, safe blackout and scheduled USB-DMX commands must be tested in non-show Pi 4 rehearsal.
- Separate physical smoke tests and explicit operator approval precede any `main` merge or Pi deployment.

Related: [#511](https://github.com/ali96adil/StageCore/issues/511), [#510](https://github.com/ali96adil/StageCore/issues/510), [#502](https://github.com/ali96adil/StageCore/issues/502). Project Files built on shared Vault should wait for this policy before allowing external file operations.
