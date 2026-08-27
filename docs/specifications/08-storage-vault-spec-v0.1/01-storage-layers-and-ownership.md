# 01 — Storage Layers & Ownership

## Storage Layers

StageCore treats storage as separate logical layers even when one SSD physically hosts several of them.

| Layer | Examples | Authority | Runtime Priority |
|---|---|---|---|
| Critical State | Project DB, Runtime Snapshot metadata, Session/CueExecution records, critical journal | Hub | High |
| Vault Metadata | MediaAsset, content versions, manifests, package metadata | Hub | Medium |
| Vault Objects | video/audio/images/documents/installers/plugin packages | Hub Vault | Low during runtime |
| Endpoint Cache | Companion media/runtime cache | Companion local disk | Local execution support |
| Backup/Archive | DB snapshots, project packages, copied Vault objects | backup destination | Lowest |

## Ownership Rules

- Project definitions, published Runtime Snapshot identity and Session history remain authoritative on Hub.
- Vault metadata determines what a managed file is; raw filenames do not.
- A Companion cache is replaceable and may be rebuilt from Hub/Vault or another approved source.
- Machine-only local configuration is not silently overwritten by Vault sync.
- Backup copies never become active authority merely because the primary copy is unavailable.

## Separation from Runtime

Heavy object reads/writes, checksumming, indexing, software downloads and backup jobs must run outside the critical Cue/Route process. A slow disk copy must not hold a runtime transaction or P1 execution queue.

## Storage Root

Implementation may expose one configurable StageCore data root, but internal paths remain separated by responsibility. Conceptually:

```text
stagecore-data/
├── state/        # DB/journal/runtime metadata
├── vault/
│   ├── objects/
│   └── manifests/
├── software/
├── plugins/
├── transfers/
├── logs/
└── backup-staging/
```

Exact OS paths are platform/deployment choices; code must not hard-code one Raspberry Pi-specific layout.