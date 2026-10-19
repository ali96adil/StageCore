# SC-024 Phase 3B — Media resource-to-Project scope qualification

**Status: draft/read-only helper, NOT production enforcement; no Pi update.**

This stacks on #521 -> #520 -> #518 (canonical membership migration 53).
The independent #517 and #519 migrations conflict with #518 and must not
be merged together. Existing Project/Vault/GO routes still use global roles.

## Qualified resource IDs

`internal/projectaccess/resource_scope.go` now resolves:
- `media_asset_id -> media_assets.project_id`
- `content_version_id -> media_content_versions.media_asset_id -> Project`
- `media_location_id -> media_locations.content_version_id -> asset -> Project`

A future authorized Project media read requires **both** the database-resolved
Project matching the expected Project ID and an immediately evaluated
`media.read` grant (which itself requires `project.view`).
A mere `project.view` grant does **not** qualify media access.
OWNER still has its live-database override.

**Never resolve a raw SHA-256 `content_hash` to one Project.** Vault blobs are
deduplicated and can be referenced by multiple Project-specific media
asset/version identities. Tests create two different assets pointing to the
same Vault object and verify that an Operator of Project A cannot read the
Project B media IDs, even with a matching hash.

## Still explicitly unqualified

- `GET /api/v1/vault/objects/{content_hash}`: raw hash downloads must not
  be reauthorized using hash as if it were a Project-scoped media ID.
  All callers (browser, tablets, Companions, relay, offline prefetch) need
  audit of their distinct session/device trust paths, and any file download
  permission must bind to a specific allowed media asset/version.
- Stage Device v2 assignments and safety handshakes require authoritative
  ACTIVE assignment/epoch ownership, not legacy `stage_devices.project_id`.
- Show Capsule imports/exports and filesystem paths require independent
  manifest verification and scope authorization, not user-supplied IDs.
- Realtime SSE, media streaming, StageAudio and all direct-ID HTTP handlers
  are not wired to this resolver. Do not claim Projects are private.
- This helper handles **reads only**. GO/DMX/tablet/laser control and active
  SHOW revocation must have a separately qualified dispatch-time fence.

Before any activation: comprehensive route inventory, no cross-Project
leaks, complete Owner impact/migration review, single atomic server-wide
cutover/rollback gate, Go/race/arm64 CI, and attended Pi 4 validation outside
an active SHOW. No direct physical output is changed by this PR.
