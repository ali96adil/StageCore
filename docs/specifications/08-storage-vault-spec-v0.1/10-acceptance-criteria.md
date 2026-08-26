# 10 — Acceptance Criteria

The Storage & Vault Specification v0.1 is implementation-ready when the following behaviors can be demonstrated repeatably on the reference Hub + macOS Companion setup.

## Persistence & Vault

- Project/runtime/session metadata survives Hub restart.
- Importing a managed file writes through staging, calculates SHA-256 and commits only after verification.
- Same filename with different bytes produces different content identity.
- Identical verified bytes can be recognized as identical content without confusing logical MediaAsset identity.
- Interrupted import never appears as a completed Vault object.

## Heavy File Transfer

- Hub can serve a managed object to Companion without reading the full object into RAM.
- A file of at least 2 GiB can begin downloading, be interrupted, then resume from saved progress.
- Final Companion file is promoted only after full checksum verification.
- Intentional corruption produces `MISMATCH/FAILED`, never READY.

## Media Readiness

- `VIDEO-MAIN` receives a Required Media manifest.
- Missing content prevents READY when marked required.
- Verified local content allows media check to PASS.
- Replacing the Mac allows the same required set to sync without Cue edits.

## Software Bootstrap

- StageCore Web exposes a macOS package from the local Software Repository.
- Package metadata includes version/platform/architecture/API compatibility/size/checksum.
- Package can be downloaded with Internet disconnected.
- An incompatible package is clearly marked/rejected rather than presented as the preferred build.

## SHOW Protection

- Start a bulk media transfer, enter SHOW, and observe the job pause/block according to policy.
- P1 Cue execution remains functional while paused bulk jobs exist.
- Software/plugin install/update and backup/archive jobs do not start automatically in SHOW.
- Leaving SHOW can resume an eligible paused transfer.

## Capacity & Failure

- A configured low-space threshold produces visible WARNING.
- A bulk write that would breach runtime reserve is rejected before final commit.
- Unwritable/critical authoritative storage blocks Show readiness where persistence cannot be guaranteed.
- External backup destination loss does not stop the local runtime loop.

## Backup & Restore

- Create a state/project backup from known Project + Snapshot + Session data.
- Verify backup manifest/integrity.
- Restore into staging/new location.
- Reopen restored Project and confirm expected Snapshot identity, Cues and Session history.
- Restore never silently replaces an active SHOW runtime.

## Reference End-to-End Scenario

```text
Import managed media to Hub Vault
 -> verify SHA-256
 -> assign media to VIDEO-MAIN
 -> pair new Mac
 -> sync/resume media to Companion
 -> verify local checksum
 -> Preflight READY
 -> download compatible StageCore macOS package from Hub Web
 -> start Rehearsal / execute Cue
 -> enter SHOW and confirm bulk jobs pause
 -> exit SHOW
 -> create verified Project/state backup
 -> restore and reopen data
```

Passing this scenario proves the Hub is both a trustworthy source of project files/software and a show-control system whose critical runtime is protected from bulk storage work.