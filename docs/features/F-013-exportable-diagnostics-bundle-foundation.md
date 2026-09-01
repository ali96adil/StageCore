# F-013 — Exportable Diagnostics Bundle — Foundation

**Status:** Phase 2 foundation slice  
**Command:** `stagecore support-bundle`

## Goal

Create one portable support archive that captures enough StageCore deployment, health, runtime-state and recent-log context to diagnose common failures without asking an operator to manually copy databases, configuration files, journal output or credentials.

The bundle is deliberately useful when StageCore is unhealthy: a Doctor `BLOCKED` result does **not** prevent bundle creation.

## Operator command

```text
stagecore support-bundle [options]
```

Default output:

```text
stagecore-support-YYYYMMDDTHHMMSSZ.tar.gz
```

The output archive is created with mode `0600` and is never silently overwritten.

Useful options:

- `--output <path.tar.gz>`
- `--install-root`
- `--config-root`
- `--systemd-unit`
- `--http-timeout`
- `--journal-lines` (default 2000, hard maximum 20000)

## Bundle contents

### `manifest.json`

Contains:

- bundle schema version;
- generation timestamp;
- Doctor overall state;
- entry names, descriptions, sizes and SHA-256 hashes;
- collection warnings;
- total redaction count;
- the privacy contract applied to the archive.

### `doctor.json`

The normal structured read-only `stagecore doctor` report after string-field redaction.

### `system.json`

A bounded host/build summary:

- hostname;
- GOOS / GOARCH;
- Go runtime version;
- StageCore module path;
- embedded VCS revision/time/modified state when available;
- kernel/platform string from `uname -srmo` when available.

No interface list, MAC addresses, saved Wi-Fi credentials, shell environment, process environment or user home contents are collected.

### `deployment.json`

Only an explicit allowlist is exported from the managed `stagecore.env`:

- `STAGECORE_DATA_ROOT`
- `STAGECORE_VAULT_ROOT`
- `STAGECORE_LISTEN`
- `STAGECORE_DEVICE_LISTEN`
- `STAGECORE_OSC_PLUGIN_PATH`
- `STAGECORE_OSC_INPUT_LISTEN`
- `STAGECORE_OSC_INPUT_PROJECT_ID`
- `STAGECORE_RUNTIME_RESERVE_BYTES`
- `STAGECORE_STORAGE_WARNING_PERCENT`

The raw environment file is never copied. Unknown keys are counted but neither their names nor their values are exported. This is an allowlist boundary rather than a best-effort secret blacklist.

### `binaries.json`

For each managed StageCore product binary:

- managed path;
- file type/mode/size;
- SHA-256;
- ELF machine when readable;
- embedded VCS revision when readable.

Binary payloads themselves are not included.

### `state-summary.json`

Read from the authoritative SQLite database in read-only/query-only mode and contains only support-oriented aggregate/non-secret metadata:

- schema version;
- project count;
- Runtime Snapshot count;
- Cue Execution count;
- Event Record count;
- local-user count;
- session counts grouped by type/status;
- Companion counts grouped by trust/readiness;
- installed extension ID/version/kind/source/lifecycle and runtime desired/observed state plus last error **code**.

It deliberately excludes:

- project names and project configuration;
- Cue payloads;
- Event Record payloads;
- user names/password data;
- browser sessions;
- Companion keys/challenges/session credentials;
- audit payloads;
- extension manifests, installed paths, actors and free-form runtime error messages.

If the database is unavailable or incompatible, bundle creation continues and records a collection warning; Doctor remains the primary integrity signal.

### `logs/stagecore-hub.log`

Bounded recent `stagecore-hub.service` journal output using `journalctl --output=cat`.

- request is line-bounded;
- captured bytes are additionally capped at 4 MiB, preserving the most recent portion;
- text passes through the diagnostics redactor before archive creation;
- journal access failure is recorded as a collection warning instead of aborting the whole bundle.

## Secret-redaction boundary

The redactor is defense-in-depth on top of the strict source allowlist. It removes common credential shapes including:

- password/passwd/secret/token fields and assignments;
- Authorization/Cookie/API-key/private-key/setup-code/pairing-code/credential fields;
- Bearer and Basic authorization values;
- token-like sensitive query parameters;
- JWT-shaped values;
- PEM private-key blocks.

Structured JSON string fields also pass through redaction, and explicitly sensitive field names are replaced even if their value shape is unfamiliar.

Tests inject known secrets into Doctor details, journald text and ignored environment keys, then unpack the produced archive and assert those values are absent.

## Failure semantics

Bundle creation is best-effort for diagnostic sources and strict for the archive itself.

A collector such as `uname`, state summary or journald may fail without losing the other evidence; the sanitized error appears in `manifest.json`.

The command fails when the archive cannot be safely created, including invalid paths/options, an existing output path, encoding/archive errors or local filesystem write failures.

## Explicit privacy exclusions

The foundation bundle never includes:

- raw SQLite database/WAL/SHM files;
- raw `stagecore.env`;
- Vault/media contents;
- TLS or other private keys;
- Secret Store values;
- passwords or credential hashes;
- browser/auth session records;
- setup/pairing codes;
- raw security-audit rows;
- raw extension manifests or package payloads;
- arbitrary filesystem trees.

## Acceptance for this foundation

Automated acceptance must prove:

1. the archive is created as `.tar.gz` with mode `0600`;
2. existing output is never overwritten;
3. Doctor `BLOCKED` still produces a bundle;
4. collector failures become manifest warnings rather than aborting the archive;
5. deployment environment export is allowlist-only;
6. current migrated SQLite schema can produce the aggregate state summary read-only;
7. journal collection is bounded;
8. entry SHA-256 values are recorded;
9. injected env/Doctor/journal secrets and private-key material are absent after unpacking;
10. sensitive structured field names and common free-form credential shapes are redacted.

Physical Phase 2 qualification later should run the real command on the Raspberry Pi, inspect the archive contents/permissions, and verify no known deployment secrets are present before F-013 is considered fully verified.

## Deferred

This foundation does not yet add:

- automatic upload to a support service;
- cloud case/ticket integration;
- full network packet captures;
- arbitrary database export;
- remote support access;
- automatic personal-data collection;
- UI-driven bundle creation.

Any future source added to the bundle must declare its privacy boundary and receive explicit secret-leak tests before inclusion.
