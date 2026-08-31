# F-009 — `stagecore doctor` Diagnostics Foundation

**Status:** Foundation slice in implementation  
**Feature ID:** F-009  
**Phase:** 2 — Installation, diagnostics, discovery, update, and extension operations

## Goal

StageCore Doctor provides one repeatable, read-only command that answers four support questions on a Hub host:

1. Is the StageCore deployment shape intact?
2. Is authoritative storage/database state inspectable and healthy enough to continue?
3. Is the Hub service live and ready?
4. If something is wrong, what is the next safe action?

Doctor is diagnostics, not repair. It must remain safe to run while investigating a fault and must not silently change the host in order to make a check pass.

## User-facing definition

**English**

> StageCore Doctor runs read-only host diagnostics and explains what is healthy, what is warning, what blocks readiness, and the next safe action.

**العربية — ar-IQ**

> طبيب StageCore يشغّل تشخيصات المضيف بوضع القراءة فقط ويشرح ما هو سليم وما يحتاج تحذيراً وما يمنع الجاهزية وما هو الإجراء الآمن التالي.

Human output is available in English and Arabic from the same CLI. Stable machine output is language-neutral JSON.

## Command surface

Ordinary support path:

```bash
stagecore doctor
```

Arabic human output:

```bash
stagecore doctor --locale ar-IQ
```

Machine-readable output:

```bash
stagecore doctor --json
```

The top-level executable is intentionally named `stagecore`; `doctor` is its first product subcommand. This creates a stable CLI namespace without inventing a separate `stagecore-doctor` binary.

Full host inspection requires enough OS permission to read the protected StageCore environment file and database. On a normally locked-down appliance that may mean invoking the command through an authorized elevated shell. Elevated invocation does **not** give Doctor repair behavior: the command remains read-only by implementation.

## Diagnostic status contract

Each check emits one canonical status:

- `READY` — check passed;
- `WARNING` — the system can still be inspected/used, but a condition needs attention;
- `ADVISORY` — informational result or a dependent check intentionally skipped because its prerequisite was unavailable;
- `BLOCKER` — condition prevents a truthful ready diagnosis and requires intervention.

Report overall state is:

- `READY` when there are no warnings or blockers;
- `WARNING` when at least one warning exists and no blocker exists;
- `BLOCKED` when at least one blocker exists.

CLI exit status is `1` only for `BLOCKED`. `READY` and `WARNING` reports exit `0` so automation can distinguish hard failure from operator-visible warnings without parsing localized prose.

## Foundation checks

The first slice runs checks in dependency order.

### Deployment configuration

Doctor reads the configured `stagecore.env` and validates the deployment-critical values already established by F-005:

- `STAGECORE_DATA_ROOT`;
- `STAGECORE_VAULT_ROOT`;
- `STAGECORE_LISTEN`;
- `STAGECORE_OSC_PLUGIN_PATH`.

It validates absolute paths, distinct Data/Vault roots, the Hub listen address and the managed OSC plugin path. It does not emit unknown environment values, so unrelated secret-bearing configuration is not copied into the report.

### Installed product binaries

Doctor verifies every binary in the canonical F-005 product registry is present as a regular executable and is not a symlink.

F-009 extends that registry with the umbrella `stagecore` CLI itself. The normal release bundle therefore installs and validates:

```text
stagecore
stagecore-hub
stagecore-osc-plugin
stagecore-pairing
stagecore-setup
```

Release checksum/ELF validation remains owned by F-005 and applies to the new CLI through the same registry.

### systemd deployment

Doctor reads the installed `stagecore-hub.service` and confirms its managed `ExecStart` and `EnvironmentFile` paths. When the unit is valid it then reads, but does not modify:

- `systemctl is-enabled stagecore-hub.service`;
- `systemctl is-active stagecore-hub.service`.

Doctor never runs enable/start/restart/daemon-reload.

### Data Root and Vault Root capacity

Doctor uses read-only filesystem metadata (`statfs`) to inspect total/free capacity against the StageCore storage policy:

- runtime reserve: 2 GiB;
- warning threshold: 15% free.

This direct check deliberately does not create a temporary file. Writable-state authority remains the existing Hub storage-health/readiness contract when the Hub is running.

### SQLite read-only health

Doctor opens the existing StageCore database using SQLite read-only mode plus `query_only` and a bounded busy timeout. It does not use normal `db.Open`, because normal Hub startup may create directories and apply migrations.

The diagnostic path checks:

- database can be opened without creating it;
- a basic `SELECT 1` works;
- bounded `PRAGMA quick_check(1)` reports `ok`;
- the highest applied Goose schema version is readable.

No migration, repair, vacuum, schema write or application-data write is performed.

### Companion pairing/readiness summary

When the database read-only check passes, Doctor summarizes `companions` by trust/readiness state. The report includes only aggregate counts such as trusted-ready or trusted-unready; it does not emit Companion IDs, credentials, public-key material or session tokens.

A trusted Companion that is not `READY` is a warning in this foundation. Future F-004 discovery/reconnect work may add richer diagnostic checks without replacing this common report model.

### Hub liveness and readiness

Doctor derives the local health endpoints from the configured StageCore listen address and checks:

- `/health/live` must return HTTP 200 with `LIVE`;
- `/health/ready` must return HTTP 200 with `READY`.

Health response reading is timeout-bounded and response bodies are size-bounded. The readiness result remains the authoritative source for current Hub storage/readiness blockers.

## Dependency-aware reporting

Doctor avoids cascading noise. If a prerequisite cannot be established, dependent checks are emitted as `ADVISORY` rather than pretending each downstream symptom is an independent failure.

Examples:

- unreadable/invalid deployment config → storage/database/Hub HTTP checks are skipped;
- unreadable/mismatched systemd unit → service enabled/active checks are skipped;
- failed database read-only health → Companion summary is skipped;
- failed Hub liveness → Hub readiness is skipped.

The original blocker remains visible with an actionable remedy.

## Localization contract

F-009 is user-facing and therefore follows the StageCore Feature Localization Contract even though this slice does not add Operator Web assets.

The CLI owns stable message/check/remedy keys and ships local catalogs for:

- `en`;
- `ar-IQ`.

Technical identifiers, paths, commands and machine JSON remain stable and recognizable. The Arabic surface translates explanations/actions rather than translating protocol/domain tokens such as `READY`, `systemctl`, paths or JSON field names inside machine output.

## JSON report contract

`stagecore doctor --json` emits report schema version `1` with:

- generation timestamp;
- overall state;
- status counts;
- ordered checks;
- stable check ID;
- canonical status;
- stable message/remedy keys;
- non-secret diagnostic detail.

JSON is intentionally not localized so future support tooling and F-013 diagnostics bundles can consume it without parsing prose.

## Read-only and security invariants

Doctor must not:

- create or migrate the StageCore database;
- write Data Root or Vault Root probe files;
- alter systemd state;
- restart the Hub;
- alter deployment configuration;
- pair/revoke a Companion;
- repair SQLite automatically;
- read or export credential/session-token contents;
- require WAN/cloud access.

When a condition needs mutation, Doctor reports the safe next action instead of performing it.

## Release and CI integration

The `stagecore` executable is a normal product binary in the supported F-005 Linux bundle. It must pass the same 64-bit Linux constraints as the rest of the appliance:

- `amd64` and `arm64` release builds;
- CGo disabled product build;
- bundle checksum and ELF validation;
- installer copy/verification through the common required-binary registry;
- Core CI Test/Vet/Race;
- explicit Linux ARM64 CGo-free `stagecore` build gate.

## Acceptance criteria for this foundation

Software acceptance requires tests proving at minimum:

- a healthy synthetic deployment produces an all-`READY` report;
- blockers/warnings/advisories aggregate correctly;
- missing configuration skips dependent checks rather than guessing paths;
- SQLite read-only DSN cannot create a missing database;
- database schema/integrity and Companion summary can be read without normal startup/migrations;
- Arabic human output is present;
- JSON output remains stable and language-neutral;
- release builder and installer registry include `stagecore`;
- Core CI passes for both supported Go lines, Race, and Linux ARM64 CGo-free product builds.

Physical acceptance is deferred until access to the qualified Raspberry Pi is available. The later Pi gate should run the installed `stagecore doctor` against the real F-005 deployment and compare its result with the already-known systemd/storage/Hub state.

## Deliberately deferred

This foundation does not implement:

- automatic repair/restart/remediation;
- exportable diagnostic archives/log bundles — F-013;
- transactional update backup/rollback — F-010;
- zero-configuration discovery/reconnect diagnostics beyond current persisted Companion state — F-004;
- Plugin/Add-on diagnostics — F-015;
- Stage Network Cockpit visualization — F-022;
- an Operator Web Doctor panel;
- remote/cloud support upload.

Later features should add checks to the common Doctor report rather than create independent health vocabularies.