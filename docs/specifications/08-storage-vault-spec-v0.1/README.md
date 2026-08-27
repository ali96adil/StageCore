# 08 — Storage & Vault Specification — v0.1

**Document Type:** Executable Storage, File Distribution & Recovery Specification  
**Status:** Initial implementation baseline  
**Based on:** 02 System Architecture + 03 Data Model + 05 MVP Product Specification + 07 Companion Specification

## Core Principle

StageCore separates show-critical persistence from heavy file storage and bulk transfer. The Hub is the authoritative metadata/source-of-truth service, while large media, installers, plugin packages, backups and archives live in a managed Vault on SSD/NVMe-class storage.

Live playback should normally use local Companion storage. The Hub distributes and verifies required files before show; it is not the normal real-time media streaming server for projector playback.

Bulk file work is lower priority than P0/P1 show control. SHOW mode can pause or reject media sync, software downloads, backup and archive jobs so they cannot compete with runtime traffic.

## Files

- [00 — Goals & Boundaries](00-goals-and-boundaries.md)
- [01 — Storage Layers & Ownership](01-storage-layers-and-ownership.md)
- [02 — Vault Layout & Asset Policies](02-vault-layout-and-asset-policies.md)
- [03 — Content Identity, Manifests & Integrity](03-content-identity-manifests-and-integrity.md)
- [04 — Media Sync & Companion Cache](04-media-sync-and-companion-cache.md)
- [05 — Software & Plugin Repository](05-software-and-plugin-repository.md)
- [06 — File Transfer Priority & SHOW Mode](06-file-transfer-priority-and-show-mode.md)
- [07 — Backup, Restore & Archive](07-backup-restore-and-archive.md)
- [08 — Capacity, Health & Failure Handling](08-capacity-health-and-failure-handling.md)
- [09 — MVP Implementation Boundary](09-mvp-implementation-boundary.md)
- [10 — Acceptance Criteria](10-acceptance-criteria.md)

## Reference Storage Shape

```text
StageCore Hub
├── Runtime DB / journal       ← critical, small, durable
├── Vault metadata             ← authoritative manifests
├── Vault object store         ← media/files on SSD/NVMe
├── Software repository        ← StageCore installers
├── Plugin package repository
├── Transfer staging           ← temporary, resumable
└── Backup / archive jobs      ← outside critical path
```

The exact filesystem/database technology remains a later implementation choice, but the behaviors in this specification are required.