# F-026 Slice E2 — Visual Engine Runtime Snapshot / Show Capsule Portability

Status: software qualification in progress.

Physical GPU/display/projector/capture qualification remains deferred under GitHub Issue #148.

## Goal

Prove that F-026 Visual Engine show state uses StageCore's existing portable authority model instead of inventing Visual-specific media or filesystem semantics.

Slice E2 covers immutable Visual Engine configuration and managed media identity across:

`Project Revision -> Runtime Snapshot -> Show Capsule -> verification`

## Authority model

Visual Engine portability reuses existing StageCore contracts:

- Visual commands remain immutable Action capability + parameter records inside the Runtime Snapshot.
- Visual renderer targets remain normal `machine_role` targets with immutable target configuration.
- `visual.preload` references content by `content_version_id` + SHA-256 `content_hash`, not by a Hub-local filesystem path.
- Required managed media is registered through the existing Machine Role media-requirement model.
- Runtime Snapshot `required_media` remains the canonical list of portable media authority.
- Show Capsule derives its media/object set from the Runtime Snapshot and includes required Vault bytes for `SELF_CONTAINED` export.

No Visual-specific portability table or second media identity model is introduced.

## Qualification scenario

The Slice E2 acceptance test builds a real project with:

- a required Visual Engine Machine Role;
- a `machine_role` target;
- valid `visual.output.configure` state;
- a valid `visual.preload` command referencing managed content by immutable content-version identity and SHA-256;
- an `ARCHIVE_REQUIRED` managed Vault object bound to the Visual Engine role.

The test then creates a Runtime Snapshot, exports a self-contained Show Capsule, verifies the capsule, and proves:

1. Visual target configuration survives unchanged.
2. Visual Action capability/parameters survive and still pass `visualengine.ValidateCommand`.
3. Runtime Snapshot `required_media` binds the Visual Machine Role to the exact content-version/hash.
4. Show Capsule media metadata carries the same immutable identity.
5. The required media object is physically included in the self-contained capsule and verifies byte-for-byte.
6. The Runtime Snapshot does not contain the Hub's Vault root or Vault relative path as portable authority.

## Deliberate non-goals

Slice E2 does not:

- add renderer execution or touch the physical renderer path;
- add new Runtime Snapshot or Show Capsule schemas;
- parse arbitrary Visual Action JSON inside Show Capsule to discover media;
- make local filesystem paths portable authority;
- change F-024 Simulation behavior;
- deploy to the Raspberry Pi or perform physical GPU/projector qualification.

## Acceptance

Slice E2 is software-complete only when exact-head and exact-main Core CI pass, including race tests and Linux ARM64 CGo-free product builds. Companion CI is not expected to change because this slice does not modify the macOS renderer/runtime.
