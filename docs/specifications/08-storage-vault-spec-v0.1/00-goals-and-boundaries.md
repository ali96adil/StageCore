# 00 — Goals & Boundaries

## Goals

The Storage/Vault design must let StageCore:

- persist Project/runtime/session data safely;
- manage large project files without embedding them in the database;
- distribute required media to Companions before runtime;
- host compatible StageCore installers from the local Hub Web UI;
- retain plugin packages and package metadata;
- verify file identity by checksum rather than filename;
- recover from interrupted uploads/downloads without exposing partial files as valid;
- maintain a runtime storage reserve;
- support practical backup/restore and later archive workflows;
- work without Internet on the stage network.

## Non-Goals for v0.1

- replacing a dedicated media server;
- streaming high-bitrate show media from Hub as the normal playback path;
- heavy transcoding/rendering on the Hub;
- cloud-first storage;
- distributed object-storage clusters;
- production HA storage replication;
- public software/plugin marketplace;
- automatic cloud/NAS lifecycle management in the first MVP.

## Hardware Direction

For prototype/field Hub storage, StageCore data and Vault files should use SSD/NVMe-class storage. A microSD card may boot a prototype device but is not the preferred long-term location for the authoritative database, event/session history or large managed Vault.

## Implementation Rule

Every file operation that changes authoritative state must have an explicit lifecycle: staged, verified, committed or failed. A filename appearing on disk is not enough to declare the asset valid.