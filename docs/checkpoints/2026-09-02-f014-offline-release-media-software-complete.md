# F-014 — Offline Release Media — Software Acceptance Checkpoint

**Date:** 2026-09-02  
**Feature:** F-014  
**Status:** SOFTWARE COMPLETE — PHYSICAL PHASE 2 QUALIFICATION PENDING

## Scope closed by this checkpoint

The supported removable/offline entry point is `./stagecore-offline` from the generated `stagecore-offline-media` artifact.

The media contains both supported Linux architectures, a fixed catalog, nested F-005 bundle checksums, an outer media checksum manifest, the launcher, release revision, and operator README. The launcher verifies the complete media before install/update delegation and selects only the exact supported host architecture.

`install` delegates to F-005 and `update` delegates to F-010. F-014 does not create alternate install/update semantics.

## Offline and host-prerequisite boundary

The launcher contains no downloader, package-manager, Git, compiler, or toolchain path. All StageCore-owned product bytes are already present on the media.

The media is not a Linux distribution image. Supported targets must provide the OS primitives required by the deployment/security contract. In particular, `install` and `update` now fail closed before delegation when Bubblewrap (`bwrap`) is absent, because F-015 third-party extensions must never silently lose their sandbox boundary.

F-014 intentionally does not vendor an arbitrary Bubblewrap binary or invoke a distro package manager. The supported OS/image or an approved offline OS-administration source owns that prerequisite.

`verify` and `info` remain usable without `bwrap` so media integrity can be checked before target preparation.

## Automated acceptance evidence

The `internal/deployment` offline-media tests prove:

- valid media verifies;
- modified product bytes fail verification;
- symlinked bundle content is rejected;
- checksum-manifest traversal is rejected;
- `aarch64` selects the arm64 bundle;
- install refuses to delegate when `bwrap` is absent;
- the release builder includes both architecture bundles, catalog, media checksum manifest, launcher and portable archive;
- the launcher contains no network downloader or package-manager commands.

The existing F-005/F-010 suites continue to own the delegated install/update safety contracts.

## Remaining Phase 2 integration check

The cumulative Phase 2 software review must ensure every supported fresh-host entry path communicates the Bubblewrap prerequisite consistently. F-014 itself is fail-closed and complete; this checkpoint does not claim that older direct F-005 bundle entry points already provide the same prerequisite UX.

## Remaining physical verification

Physical verification is deliberately deferred to the cumulative Raspberry Pi ARM64 Phase 2 gate. That gate must use exact final Phase 2 release media with target WAN disabled and prove media verification, arm64 selection, Bubblewrap availability, dry-run non-mutation, real install/update behavior, and F-010 rollback preservation.

No physical qualification claim is made by this checkpoint.
