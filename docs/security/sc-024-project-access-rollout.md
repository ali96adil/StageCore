# SC-024 — Project-scoped access, phase 1 (NOT ENFORCED YET)

**Draft, not for deployment.** Existing StageCore accounts currently use global
`OWNER / TECHNICIAN / OPERATOR / VIEWER` roles. Existing Project listing,
Cue routes, runtime GO, Stage Devices, Vault media and Capsule routes are
**not** protected by Project-scoped grants in this first phase. Do not rely on
the new grant administration API to hide files or isolate users.

## Why an explicit staged rollout?

Cross-cutting route enforcement can break an active show if introduced halfway
without auditing all HTTP and runtime/telemetry paths. This phase introduces
data tables, Owner-managed grant mutation and a fresh authorization evaluator,
but intentionally does not gate old endpoints yet.

- `project_user_memberships`: membership keyed by immutable ProjectID/UserID.
- `project_user_capabilities`: exact allowlisted actions, never inherited on
  Project duplication. Membership requires `project.view`.
- `project_user_access_audit`: transactional before/after, actor, timestamp.
- `CheckProjectAccess`: OWNER enabled override, otherwise exact matching
  project/user/capability, checked on each invocation; disabled user is denied,
  downgraded VIEWER cannot keep a stale GO permission.
- No preseed/backfill of global OPERATOR into all Projects (would silently
  preserve inappropriate blanket access once enforcement switches on).

## Preparation API (OWNER only; HTTPS or secure local loopback)

All browser write calls require same-origin and StageCore CSRF:

```text
GET    /api/v1/projects/{project_id}/access
PUT    /api/v1/projects/{project_id}/access/{user_id}
DELETE /api/v1/projects/{project_id}/access/{user_id}
```

PUT body: `{"capabilities":["project.view","rehearsal.control","show.go"]}`.
All success responses include `"enforcement_active":false` and an explicit
warning. **This is an administrative preview, not effective protection in
the existing Operator UI.** No Owner ACL UI is bundled with phase 1.

Owner may explicitly authorize distinct scopes:
`project.view`, `project.edit`, `project.rename`,
`media.read`, `media.upload`, `media.manage`, `cue.edit`,
`snapshot.publish`, `rehearsal.control`, `show.go`,
`show.enter_exit`, `device.configure`, `project.export`.
VIEWER may only be granted `project.view` and `media.read`.
These are design-phase proposed capabilities; effective route mapping requires
a reviewed audit before rollout.

## Required enforcement audits before enabling any new access model

1. Enumerate all REST, SSE, streaming, plugin, and live runtime entry points
   that accept a ProjectID, RevisionID, SnapshotID, CueID, DeviceID, AssetID,
   content hash or implicit active Project. Map each to the correct Project.
   **Never authorize a caller-supplied ProjectID in isolation** when a second
   object identifier actually belongs to another Project.
2. Filter Project listing and protect Project create/rename/duplicate. Do not
   grant copied Project to the source operator automatically.
3. Protect Project workspace Vault asset lists, upload/download URLs, hashed
   content, previews, Show Capsule import/export, audio media and tablet video.
   Unknown or wrong-Project resource must not leak through different error
   shapes or unprotected binary responses.
4. Authorize Cue authoring, Snapshot publish, SHOW GO/JUMP, device bindings,
   OSC/MIDI/lighting/StageAudio output, and side effects before dispatch.
   Preserve the current emergency blackout safety policy independently.
5. Enforce Owner-controlled grants across long-lived sessions and live
   streams; on revocation, recheck not just at HTTP entry but before each
   sensitive output. Design an atomic runtime fence to prevent stale GO
   commands crossing a permission transition.
6. Migration preview lists existing non-Owner accounts and Projects to Owner
   for review. No silent grant-all and no silent lockout on a live show.
7. Only once *every* path has server-side checks: enable the new model in a
   separate PR, test all users and media, run security regression and
   attended Pi 4 non-show qualification, then request explicit approval.

**This PR does not enable access enforcement.** Global legacy role semantics
remain unchanged, so users currently have their previous broad access.
