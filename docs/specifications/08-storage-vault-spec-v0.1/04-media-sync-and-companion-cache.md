# 04 — Media Sync & Companion Cache

## Principle

The Hub/Vault owns managed media masters and manifests. A Companion owns its local cache bytes, but that cache is replaceable and validated against Hub requirements.

StageCore does not use the Hub as the normal live playback stream for large show media. Required media is synchronized to the assigned Companion before the role becomes READY.

## Sync Flow

```text
Hub sends Required Media Manifest
 -> Companion compares local content identities
 -> reports PRESENT / MISSING / MISMATCH
 -> Hub schedules missing content transfer
 -> Companion writes to temp cache file
 -> transfer may resume after interruption
 -> checksum verified locally
 -> atomic promote to cache
 -> Companion reports verified content identity
 -> role readiness recalculated
```

## Transfer Contract

A transfer job should expose:

- transfer/job ID;
- content identity;
- total size;
- source and destination identities;
- current offset/progress;
- state: `QUEUED | RUNNING | PAUSED | VERIFYING | COMPLETED | FAILED | CANCELLED`;
- failure reason;
- timestamps;
- retry/resume eligibility.

For heavy-file practicality, the v0.1 transfer API must be designed for byte-range/resumable download. The reference implementation acceptance test includes interruption and resume of a multi-gigabyte object.

## Cache Pinning

Content required by the active Published Runtime Snapshot is pinned while that Snapshot is active/needed for readiness. Cache cleanup cannot evict pinned required content.

## Cache Eviction

Non-pinned old content may be removed according to local cache policy when storage is needed. Eviction changes only the Companion cache location; it does not delete the Hub master or logical MediaAsset.

## Local Path Mapping

Machine-specific playback paths may be derived from cache configuration and reported to the Companion/local integration. Cue definitions should continue to reference logical assets/roles rather than hard-coded absolute Mac paths when a managed media mapping exists.

## Readiness

Required media is READY only when the exact expected content identity is locally verified. Same filename with different bytes is `MISMATCH`, not READY.