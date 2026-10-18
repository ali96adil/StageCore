# SC-024 Phase 4C — Opt-in scoped Show Capsule local export inventory

**Draft / candidate only (not wired into the production Hub assembler).**

## Existing issue

`GET /api/v1/show-capsules` currently uses global
`userauth.PermissionProjectRead` to enumerate **both** imported and exported
Show Capsules, including Project names, IDs and Snapshot IDs. The scoped
Project selector and Files UI alone cannot make a current Hub private.

`GET /api/v1/show-capsules/imports/{capsule_id}/plan` likewise uses global
Project read. The restore mutation uses the separately privileged
`PermissionBackupRestore`. Neither endpoint is changed by this PR.

## New, separately opted-in read-only export route

`WithScopedLocalCapsuleExports(auth, projectaccess, trustedCapsuleRoot)`
registers `GET /api/v1/projects/{project_id}/show-capsules/exports`
**only** when an assembler explicitly supplies this Option. No fallback to
the global role or broad Capsule library is permitted.

- Authenticate current browser cookie on secure TLS/loopback transport.
- Check exact requested Project's live `capsule.export` grant,
  implicitly requiring `project.view` for non-Owners.
- Scan only the Hub-controlled `exports` directory; never enumerate
  `imports`, never take paths from the request, ignore symlinked entries
  and invalid Capsule IDs. Reject symlinked export root.
- Verify each candidate's canonical Show Capsule manifest/checksum and
  included files using `showcapsule.Verify`. Enforce directory name ==
  manifest Capsule ID and **actual** `manifest.Project.ProjectID`.
- Bind Project+Revision+Snapshot+SnapshotSHA256 to the immutable local
  published `runtime_snapshots` record with
  `CheckLocalExportCapsuleRead`. Never authorize based on a mutable title.
- Emit only verified, Project-owned export metadata (no object bytes,
  Vault hash download paths, absolute locations or import plans).
- Deny unknown Project and unauthorized Project uniformly as HTTP 404;
  avoid reflecting any foreign Project name, Capsule ID or Snapshot.
- Recheck permissions before responding; disable HTTP caches. Future
  streaming endpoints require an atomic revocation fence.
- Bound a single scan to 128 entries, then **fail the entire request**
  (503) rather than return a misleading partial inventory. Replace with a
  stable indexed cursor inventory before general availability.

## Safety caveats

A trusted Hub-owned directory is a **precondition**. `os.Lstat` is
defense-in-depth against symlinks, not a complete cure for malicious
concurrent filesystem mutation, TOCTOU or adversarial content resource
exhaustion. Full export privacy/streaming rollout needs rooted filesystem
handles, capsule size/count quotas, cancellation-aware verification,
indexing/pagination and audited transport/download policy.

This new route remains unwired; current Capsule library, import plan,
restore and all other globally authorized Hub routes are **unchanged**.
Do not advertise project-wide privacy or enable for ordinary users until
an atomic same-policy cutover covers sibling routes.

## Import/restore separate gate

Imported Capsule ownership is not proven by the current Hub's Project DB:
the whole point is restoring a Show that may not exist locally. A later
Owner-only import review must require verified canonical imported Capsule,
controlled import root with path/symlink protections, explicit signed or
trusted provenance policy, collision checks, restore dry-run, backup,
SHOW inactivity, CSRF and an atomic transaction. Ordinary
`capsule.export` or `project.view` MUST NEVER imply import/restore.

Existing runtime `backup.restore` behavior must remain untouched until
a separately reviewed safe cutover, especially during SHOW.
