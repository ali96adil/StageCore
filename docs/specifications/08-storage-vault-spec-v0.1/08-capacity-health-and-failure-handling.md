# 08 — Capacity, Health & Failure Handling

## Storage Health States

Hub storage exposes a simple operator-facing state:

- `HEALTHY`
- `WARNING`
- `DEGRADED`
- `CRITICAL`
- `UNAVAILABLE`

Health is calculated from observable filesystem/database conditions rather than a decorative status.

## Capacity Admission

Reference v0.1 behavior:

- warn when free space falls below 15% of the authoritative data filesystem;
- maintain a configurable runtime reserve, default 2 GiB;
- reject/pause bulk import/sync/package/backup writes when projected free space would breach the runtime reserve;
- treat actual free space below the reserve as `CRITICAL` until remediated.

Deployments may tune thresholds, but they cannot disable the rule that bulk work must leave space for critical persistence.

## Preflight Impact

Preflight returns BLOCK when StageCore cannot safely persist required runtime/session data, the authoritative database is unavailable, or the filesystem is in a critical/unwritable condition.

Low but non-critical capacity can be WARN if the active runtime still has adequate reserve.

## Failure Cases

### Storage Full / Near Full

- shed/pause bulk P3 work first;
- block new large imports/downloads;
- preserve runtime logging reserve;
- show operator remediation and affected jobs.

### Vault Object Corruption

- mark affected content unavailable/mismatched;
- block required-media READY when applicable;
- restore/re-sync/re-import from a verified source;
- do not change expected checksum to hide the error.

### Database Unavailable

- reject state-changing operations that cannot be safely committed;
- do not claim successful Project/session persistence;
- preserve only the already-loaded bounded runtime behavior permitted by System Architecture policy;
- require recovery/integrity check before normal authority resumes.

### Interrupted Transfer

- retain resumable staging state when valid;
- never expose partial bytes as completed object;
- verify full checksum before promotion.

### External Backup/NAS Offline

- active show continues locally;
- backup job becomes pending/failed/interrupted;
- operator sees last verified backup state;
- retry occurs outside SHOW according to policy.

## Monitoring

Minimum metrics/status include:

- total/free/usable bytes;
- runtime reserve;
- DB writable/healthy state;
- Vault root writable state;
- active transfer count/bytes;
- failed integrity jobs;
- last successful/verified backup where configured.