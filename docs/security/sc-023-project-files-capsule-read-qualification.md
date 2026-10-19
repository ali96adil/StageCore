# SC-023 / SC-024 Phase 4A — Project Files workspace and local Show Capsule export read qualification

**Draft only · not wired to production routes · no Pi deployment.**

Stack: #518 → #520 → #521 → #522 → #523 → #524 → this phase.
#517 / #519 offer incompatible migration 53 alternatives. Do not merge
both models. No new migration is needed for this phase.

## Project Files model (read-only)

StageCore's existing `media_assets.project_id` and `media_content_versions`
already provide per-Project identities over the immutable, deduplicated
`vault_objects`. A virtual workspace therefore exists for every existing
Project, even when no media has been uploaded. It is not a physical directory
named after the Show.

`ListScopedProjectFiles` (unwired) provides the shape:
`{project_id, project_name, files:[{media_asset_id,media_asset_name,
asset_policy,content_version_id,original_filename,size_bytes,category}]}`.

- `project_id` is the stable identifier; changing the display name does
  not relocate Vault data, change media IDs or invalidate existing Cues.
- Current `media.read` **and** `project.view` are needed before any file
  metadata is returned; Owner retains its global override.
- Every Media Version is a separate entry; an Asset without a Version can
  still be represented. File names and categories are **presentation only**,
  not permission, MIME validation, safe decoding or a filesystem path.
- No raw Vault storage paths or unscoped file browse exposed.
- Query reads at most 501 entries; over 500 returns an explicit
  `ErrWorkspacePaginationRequired` rather than silently hiding media.
  Production UI needs stable cursor/page API before activation.
- Original Project and duplicate have distinct media Asset/Version IDs
  even when deduplicated blob bytes are shared. Project memberships do not
  transfer with the duplicate.
- Grant rechecks before and after the DB listing are defense-in-depth, not
  an atomic revoke guarantee for future slow HTTP streams. Live endpoint
  cutover must recheck after changes and at write/stream boundaries.

**Not yet implemented:** UI, uploads, folders/subfolders, media naming and
tag mutation, trash/refcount-aware deletion, upload quotas, streaming, binary
download, explicit attachment workflow, tablet media sync, backups or
cross-device StageAudio. Do not advertise a user-visible Files page yet.

## Local Show Capsule exports

The current `GET /api/v1/show-capsules` lists both import and export
filesystems under a global `project.read` browser permission. It verifies
manifests but does not filter the entries by per-Project grants. Metadata
includes the Project name/ID, Snapshot ID and creation date: replacing only
the Projects selector would leave this as a Project-information leak.

`CheckLocalExportCapsuleRead` (unwired) qualifies a **local existing
export** by current `capsule.export` grant and immutable published
`runtime_snapshots` row matching source `ProjectID + RevisionID +
RuntimeSnapshotID + content_hash`. It never authorizes based on a
mutable Project name. Tests cover Project A/B isolation, missing/foreign
Snapshot, bad SHA, rename and immediate revoke.

**Critical caller requirement:** Call `showcapsule.Verify` on an
ID-validated capsule *under the trusted exports root* before constructing
`LocalExportCapsuleIdentity` from the verified manifest. The helper itself
is not a file/manifest integrity verifier, and must not accept raw untrusted
browser fields. Use the full manifest / checksum / object-verify result
before responding with metadata. Any export index that lacks an authoritative
local Snapshot must be hidden from ordinary scoped users, with Owner-only
recovery pathways reviewed separately.

**Imports/restore remain a different privilege:** Capsules from another Hub
cannot be looked up against a local Snapshot before importing. Do not
silently grant `capsule.export` users restore/import/plan privileges. Existing
`POST /api/v1/show-capsules/imports/{capsule_id}/materialize` uses global
`backup.restore`; it needs a distinct Owner-approved, path-contained,
signature/integrity-checked and SHOW-safe import policy. The global imports
list and plan endpoints also need separate Owner restriction or safely
redacted summary at atomic ACL cutover.

## Physical safety & unresolved cutover

All new helpers are unused by production Hub registration. Existing global
browser roles, Companion device Vault downloads, GO, StageAudio, DMX, Laser,
Tablets, SSE and Show Capsule routes remain unchanged. Security enforcement
cannot be claimed from these tests.

Before full cutover:
1. Add comprehensive, centrally enforced route/event/download scope map,
   including imported capsules and other service credential streams.
2. Build Owner UI/impact report and explicit memberships for existing
   Operators; prevent in-progress SHOW privilege changes from stranding
   emergency STOP/blackout.
3. Implement a single atomic fail-closed feature/rollback gate, revalidate
   grants at authorized file/GO boundaries and test direct-ID probes.
4. Run all Go CI/race/ARM64 builds plus attended non-SHOW Pi, device and
   fallback tests. Do not deploy Draft code to the live Show installation.
