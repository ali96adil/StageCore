# 08 — Backup, Restore & Recovery Drills

## Backup Verification

A backup job is not considered sufficient because files were copied. Reliability testing must prove:

- consistent database/state snapshot;
- expected Project/Snapshot/Session manifest exists;
- required copied object checksums validate;
- backup destination loss produces explicit failed/interrupted state;
- backup work can pause/defer during SHOW without affecting runtime.

## Restore Drill

At least once before first rehearsal qualification:

1. create known Project with Published Snapshot and completed Session;
2. create verified State/Project backup;
3. stop the test Hub;
4. restore into a separate staging/new data root;
5. run integrity/compatibility checks;
6. launch StageCore against restored state;
7. verify Cues, Snapshot identity, Notes, Session history and required metadata;
8. verify no runtime action executes merely because restore completed.

## Lost Hub Scenario

A later field drill should prove that a replacement Hub installation can restore protected data and clearly handle Hub identity/trust implications. Restoring Project data must not silently pretend a different Hub identity is the old trusted Hub.

## Archive Check

When archive functionality is implemented, one `FINAL -> ARCHIVED -> revival revision` drill verifies historical state remains immutable and required archive content is available/verified according to policy.

## Evidence

Record backup ID, source StageCore version, destination, checksum/integrity result, restore duration, restored Project/Snapshot IDs and any manual remediation.

## Release Rule

A release that cannot restore its own supported backup into a clean test environment is not considered recovery-ready.