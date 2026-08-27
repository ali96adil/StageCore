# 07 — Backup, Restore & Archive

## Backup Levels

StageCore distinguishes at least:

1. **State Backup** — database/configuration, Runtime Snapshot metadata, Session/Note history and manifests.
2. **Project Backup** — one Project plus required metadata/manifests and selected managed content.
3. **Full StageCore Backup** — authoritative state plus Vault objects/package repositories according to policy.
4. **System Recovery Material** — deployment configuration needed to rebuild a Hub, without treating secrets casually.

## Destinations

Practical destinations include:

- external SSD;
- trusted NAS/network share;
- later encrypted cloud/object storage.

The stage runtime must not depend on the backup destination being online.

## Backup Job State

A backup job records:

- job ID/type;
- source snapshot/project identity;
- destination identity;
- started/completed time;
- manifest/checksum summary;
- state: `QUEUED | RUNNING | PAUSED | VERIFYING | VERIFIED | FAILED | INTERRUPTED`;
- last verified restore/integrity result where available.

## Consistency

Database/state backup must use a consistent snapshot mechanism appropriate to the selected database. Large Vault objects should be copied by immutable content identity so unchanged verified objects do not need to be duplicated unnecessarily by every logical backup format.

## Verification

A backup is not considered verified merely because the copy command exited successfully. Verification must at least validate backup manifest integrity and sampled/all required object checksums according to backup type.

Release/reliability testing must include an actual restore drill.

## Restore

Restore is explicit and non-destructive by default:

```text
select backup
 -> inspect metadata/version
 -> validate compatibility + integrity
 -> restore into staging/new location
 -> verify database/manifests/objects
 -> operator confirms activation/replacement
```

A restore must not silently overwrite an active SHOW runtime.

## SHOW Mode

Scheduled/trusted-network backup is deferred during SHOW by default. Interrupted backup resumes/restarts after SHOW according to destination capability; it does not consume P0/P1 resources.

## Archive Lifecycle

Project lifecycle remains:

`ACTIVE -> FINAL -> ARCHIVED`

Archiving can require an `ARCHIVE_REQUIRED` asset set to have verified protected copies. Reopening an archived production creates a new revival/revision rather than editing preserved historical state in place.

## MVP Boundary

The MVP needs a repeatable state/project backup and restore path plus job/integrity semantics. Fully automated NAS/cloud schedules, long-term retention policy UI and complete production archive management can follow after the core loop is proven.