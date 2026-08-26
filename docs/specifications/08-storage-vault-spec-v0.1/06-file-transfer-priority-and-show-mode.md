# 06 — File Transfer Priority & SHOW Mode

## Priority Rule

Bulk storage work is never part of the critical P0/P1 runtime path. Media copies, software/plugin downloads, backup, archive, garbage collection and deep integrity scans run as lower-priority work with bounded resource usage.

## Separate Transfer Service

The implementation should isolate bulk transfer from Cue/Route execution using separate workers/queues and, where practical, separate HTTP/file-transfer endpoints. Runtime command/event queues must not wait behind large file chunks.

## Mode Policy

### EDIT / PREP

- normal media/software/plugin transfer allowed;
- backup/archive may run;
- bandwidth/concurrency remain bounded.

### REHEARSAL

- required media sync may run when explicitly needed;
- background backup/archive should be deferred or heavily throttled;
- operator can see active bulk jobs.

### SHOW

Default policy:

- pause queued/running nonessential media sync;
- reject new software/plugin package downloads from managed StageCore clients where product control exists;
- block install/update/uninstall operations;
- pause backup/archive/garbage collection/deep scans;
- allow only small runtime-critical persistence and explicitly approved emergency operational transfers.

Leaving SHOW can resume paused jobs after revalidation.

## Bandwidth / Concurrency Controls

The transfer service exposes configurable limits such as:

- maximum concurrent bulk transfers;
- maximum per-transfer bandwidth;
- maximum aggregate bulk bandwidth;
- chunk size;
- pause/resume state.

Exact default numbers are deployment tuning values, not protocol semantics.

## Runtime Storage Reserve

Bulk writes are admitted only if the projected write leaves the configured runtime reserve intact. The reference MVP default reserve is **2 GiB** on the authoritative Hub data filesystem, configurable per deployment.

If a planned transfer would consume the reserve, it is rejected/paused before writing the final object.

## Observability

The operator can see active transfer state, progress, source/destination, pause reason and whether SHOW policy is blocking the job. Storage jobs must not flood the main runtime log with per-chunk noise.