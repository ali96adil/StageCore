# 09 — MVP Implementation Boundary

## MUST Implement for the First Real Storage Slice

The first StageCore implementation does not need every future Vault feature. It must prove these concrete behaviors:

1. configurable Hub data root on normal filesystem storage;
2. authoritative database/state persistence separated from large file blobs;
3. managed Vault import using staging -> SHA-256 verify -> atomic promote;
4. `MediaAsset`/content-version metadata referencing managed content identity;
5. HTTP/local-network download of managed content without loading the whole file into application memory;
6. resumable transfer contract and one working resumable Companion download path;
7. required-media manifest comparison on one macOS Companion;
8. checksum verification before Companion media READY;
9. local Software Repository entry downloadable from StageCore Web;
10. SHOW-mode bulk transfer pause/block behavior;
11. runtime storage reserve/admission check;
12. one repeatable state/project backup and restore path.

## SHOULD Implement After the Above Is Stable

- cache eviction UI/policies;
- plugin package repository UI;
- external SSD/NAS backup destination automation;
- periodic integrity scan;
- deduplicated full-project export format;
- archive lifecycle UI.

## Explicitly Deferred

- cloud synchronization dependency;
- S3-compatible distributed object store requirement;
- automatic WAN download/update service;
- video transcoding/proxy generation;
- remote media streaming to playback machines;
- multi-Hub replicated Vault;
- complex retention/legal archive management.

## Reference Implementation Order

```text
S0 Storage Root + DB persistence
 -> S1 Vault object import + checksum
 -> S2 File download/transfer jobs
 -> S3 Companion media cache sync
 -> S4 Hub Software Downloads
 -> S5 SHOW traffic gates + capacity reserve
 -> S6 Backup/restore proof
```

Each slice should ship with tests before the next layer adds more storage behavior.

## Technology Guardrail

Do not choose a database, web framework or object-storage product merely because the future architecture might need it. The first implementation should use the simplest local technologies that satisfy atomicity, streaming transfers, checksums and recovery while keeping migration boundaries explicit.