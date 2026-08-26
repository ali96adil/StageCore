# 05 — Storage, Process Crash & Power-Loss Recovery

## Process Crash Tests

Kill the Hub process without graceful shutdown during controlled states:

- idle after committed Project edit;
- active Rehearsal after completed Cues;
- during pending Action result;
- during Vault import staging;
- during backup job.

After restart, StageCore must:

- reopen committed Project data;
- preserve Published Snapshot identity;
- preserve completed execution history;
- mark interrupted Session/job state honestly;
- distinguish committed Vault objects from orphan staging files;
- never invent completion for in-flight work;
- not replay the last Cue/Action automatically.

## Storage Pressure Tests

Test:

- low-space warning threshold;
- projected bulk write breaching runtime reserve;
- actual free space below reserve;
- read-only/unwritable authoritative data path;
- unavailable database;
- intentionally mismatched Vault checksum.

Expected:

- bulk work is rejected/paused before consuming reserved runtime space;
- Preflight blocks when authoritative persistence cannot be trusted;
- corrupted required media becomes mismatch/unavailable;
- expected checksum is never silently changed to match damaged bytes.

## Interrupted Transfer Tests

For a large file of at least 2 GiB:

1. start Companion download;
2. interrupt network/process;
3. restart/reconnect;
4. resume from valid staging progress;
5. verify full SHA-256;
6. atomically promote final file.

Partial content must never be reported READY.

## Hard Power-Loss Test

Only on disposable/reference hardware, remove Hub power during a test Rehearsal after known committed state and at selected write boundaries.

On reboot:

- storage/database integrity checks run before normal authority;
- last cleanly committed state is recoverable;
- active Session is not falsely marked cleanly ended;
- Snapshot identity remains consistent or a clear recovery blocker is shown;
- no automatic execution occurs during recovery.

## Filesystem/Database Corruption

Synthetic copies of data may be damaged deliberately to verify fail-closed behavior. Do not run destructive corruption tests against the only valid project copy.

## Recovery Evidence

Record last known committed operation, crash/power-loss point, first recovered state, detected inconsistencies and operator action required.