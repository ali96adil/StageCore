# 2026-08-27 — M5 Storage / Vault / Media Readiness Completion Checkpoint

## Status

**M5 — Storage / Vault / Media Readiness: COMPLETE**

M5 was merged through PR #23.

Merged product commit:

```text
99552d6d58512836ea325393812d52dbbded6f1d
M5 Storage / Vault / Media Readiness (#23)
```

Merged tree:

```text
3e68ac45ccef668ff73c10875f9fd35b9865d9f9
```

The merged tree is byte-identical to final tested branch HEAD:

```text
92cf1d9ffae97632b24f193d512d88f1e04a2ca3
```

## Verification

Final pre-merge evidence:

- Core CI #145 — PASS;
- Companion Core CI #77 — PASS;
- real macOS Companion replacement acceptance — PASS;
- `2 GiB + 1 byte` interrupted media transfer acceptance — PASS.

Post-merge verification on `main`:

- Core CI #146 — PASS;
- Companion Core CI #78 — PASS;
- real macOS Companion replacement acceptance — PASS;
- `2 GiB + 1 byte` interrupted media transfer acceptance — PASS.

## Delivered M5 boundary

### S0 — Storage roots + authoritative metadata

- configurable Hub Data Root and independent Vault Root;
- SQLite/WAL remains authoritative for state/metadata;
- large content bytes remain outside SQLite;
- required directories are created and checked;
- storage health reports truthful warning/critical/unavailable conditions.

### S1 — Vault + content identity

- staged imports;
- streaming SHA-256;
- content-addressed immutable objects;
- atomic no-overwrite promotion;
- interrupted/failed import does not appear complete;
- identical bytes may deduplicate without collapsing logical MediaAsset identity.

### S2 — Streaming + resumable transfer

- Hub streams immutable Vault objects from disk;
- authenticated HTTP Range support;
- Companion `.part` offset provides restart-recoverable transfer state;
- final content is promoted only after exact size and SHA-256 verification.

Large-file acceptance uses deterministic `2 GiB + 1 byte` content. The test forces an interruption after 16 MiB of completed chunks, verifies that only `.part` exists, resumes from the same HTTP Range boundary, verifies the final SHA-256, then atomically promotes the completed object.

### S3 — Companion media cache + readiness

- macOS cache uses resumable `.part` files;
- downloads use bounded 8 MiB Range chunks rather than byte-by-byte whole-transfer processing;
- Required Media is captured by immutable Runtime Snapshot identity;
- missing/corrupt required content produces BLOCKED/MISMATCH rather than READY;
- verified content permits READY;
- Machine Role/Snapshot replacement semantics remain independent from Cue edits.

### S4 — Local Software Repository

- local package metadata includes version, platform, architecture, Hub/API compatibility, size, SHA-256, signing/notarization status and channel;
- Hub exposes a local Downloads/Setup path without Internet dependency;
- incompatible packages are marked/rejected;
- package bytes reuse immutable Vault storage and Range serving.

### S5 — SHOW protection + capacity reserve

- bulk work is separate from P0/P1 runtime paths;
- SHOW pauses/rejects nonessential bulk work;
- P1 Cue execution remains functional while bulk work is paused;
- eligible work resumes after leaving SHOW;
- filesystem free-space probing is implemented;
- reference warning threshold is 15%;
- configurable runtime reserve defaults to 2 GiB;
- bulk writes that would breach reserve are rejected;
- critical/unwritable authoritative storage blocks readiness.

### S6 — Backup / restore

- repeatable consistent SQLite state backup;
- backup manifest/checksum integrity evidence;
- tamper detection;
- non-destructive restore to a new/staging Data Root;
- restored Project, Runtime Snapshot, Cue, Session and runtime history are verified;
- backup/restore is blocked during active SHOW.

## Preserved invariants

M5 did not weaken the M0–M4 runtime model:

- Runtime Snapshot remains immutable and authoritative;
- Machine Role remains logical target semantics;
- readiness/results remain truthful;
- no hidden replay was introduced;
- Companion authentication/revocation remains enforced;
- P1 Cue/Route runtime does not depend on bulk media/software operations.

## Explicitly not claimed

M5 completion does **not** mean the selected Raspberry Pi/SSD/NVMe deployment is rehearsal-ready or show-ready.

No claim is made here for:

- physical Raspberry Pi power-loss recovery;
- real Pi SSD/NVMe sustained throughput or thermal behavior;
- Stage LAN fault/recovery on physical hardware;
- production macOS signing/notarization/background packaging;
- cloud storage, transcoding, remote streaming or multi-Hub replication.

## Next gate

The next engineering gate is Issue #21:

**Pre-M5 Raspberry Pi ARM64 Smoke Gate**, now repurposed as the **post-M5 physical Raspberry Pi M0–M5 smoke/storage/media qualification**.

That hardware pass should exercise the merged M0–M5 stack on the intended Pi/storage path before advancing the baseline beyond this checkpoint.
