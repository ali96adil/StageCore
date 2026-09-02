# F-025 — Execution Environment Persistence & Readiness

**Status:** Phase 3 implementation slice  
**Feature ID:** F-025  
**Builds on:** `F-025-execution-environment-manifest-v1.md`, Project revisions, F-012 SHOW configuration lock, canonical JSON/content identity

## Purpose

This slice makes Execution Environment Manifest v1 durable StageCore Project configuration and defines how a future workstation adapter reports environment readiness without introducing application-specific state into Core.

The manifest contract remains engine-neutral. VDMX, QLab, Ableton Live, TouchDesigner, lighting applications, and future engines must translate their facts into the same model rather than adding vendor-specific top-level Core fields.

## Revision persistence

Execution environments are stored in `execution_environment_manifests` and belong to a specific `project_revisions` row.

Each row stores:

- a durable `environment_manifest_id`;
- `revision_id`;
- stable `environment_key`, `adapter_key`, and external `application_key` identity columns;
- exact canonical manifest JSON;
- SHA-256 over those canonical manifest bytes;
- bounded creation actor and timestamp.

`(revision_id, environment_key)` is unique. Multiple engines may exist in one revision, but a revision cannot silently contain two different definitions for the same semantic environment key.

Content-bound asset bytes are **not** copied into this table. F-025 continues to use content hashes compatible with StageCore's existing Vault/media integrity model rather than creating another blob store.

## Canonical durable integrity

Writes pass through `internal/executionenv` validation and canonical encoding before storage.

Reads fail closed unless all durable identity remains internally consistent:

1. `manifest_json` must decode with no unknown fields or trailing JSON;
2. the stored bytes must be exactly the canonical representation, not merely semantically equivalent JSON;
3. the canonical SHA-256 must equal `content_sha256`;
4. manifest `environment_key`, `adapter_key`, and application key must equal the duplicated indexed identity columns.

This means database whitespace/order drift, hash tampering, identity-column drift, or malformed durable data is surfaced as a conflict instead of silently normalized.

## Draft and revision lineage

Execution-environment mutation is Project configuration:

- create/delete requires a `DRAFT` revision;
- validated/superseded revisions remain immutable historical truth;
- read access is allowed for any retained revision.

When `EnsureProjectDraft` forks a validated revision, migration-level revision-fork behavior clones each execution-environment requirement into the successor Draft with:

- a new manifest row identity;
- the same canonical manifest bytes and content SHA-256;
- the new revision ID;
- the fork actor/time as creation audit metadata.

The validated source row is never mutated.

## SHOW configuration lock

F-025 persistence inherits the existing F-012 Project lock rather than inventing another runtime mode rule.

While an active SHOW Session exists for a Project:

- get/list remain read-only and available;
- insert/update/delete of execution-environment rows are blocked by SQLite triggers with `SHOW_CONFIGURATION_LOCKED`;
- Store mutation paths surface `domain.ErrShowConfigurationLocked`;
- there is no override path.

This protects direct SQL mutation as well as normal Store APIs.

## Readiness observation boundary

`internal/executionenv.EvaluateReadiness` is deliberately pure. It receives:

1. a validated execution-environment Manifest; and
2. an explicit adapter-supplied Observation.

It does **not** scan files, inspect processes, query applications, launch software, contact Companion, or infer facts that were not observed.

An Observation can report:

- host OS and architecture;
- external application presence, observed version, and adapter-evaluated version compatibility;
- asset presence, inspectability, observed hash, and observed size;
- external application extension/add-on presence/version compatibility;
- declared binding presence.

Application version constraints remain intentionally adapter-owned strings because third-party applications do not share one reliable version grammar. The adapter therefore supplies an explicit compatibility result instead of Core pretending every engine uses SemVer.

## PASS / WARN / BLOCK semantics

The evaluator reuses StageCore's established readiness vocabulary.

### BLOCK

Examples:

- unsupported host OS/architecture;
- required application missing;
- application version incompatible or not verifiably compatible;
- required content-bound asset missing;
- content-bound asset SHA-256 or size mismatch;
- required reference-only locator absent;
- required external extension missing/incompatible/unverified;
- required binding missing.

### WARN

Examples:

- optional external extension missing, incompatible, or unverified;
- optional binding missing;
- a `REFERENCE_ONLY` asset is present but StageCore cannot make a content-integrity or backup claim about its bytes.

A reference-only dependency therefore remains a portability warning even when the locator is present. Presence is not upgraded into a false content-backed PASS.

### PASS

A check passes only when the adapter observation supplies enough evidence for the corresponding requirement to be satisfied.

Report status uses worst-severity-wins. Checks are sorted by stable keys so equivalent observations produce deterministic output regardless of input list order.

## Preflight integration boundary

This slice does **not** wire execution environments into SHOW Preflight yet.

Doing so before a real observation provider exists would create false readiness truth. A later F-025 slice must add a bounded inspector/provider registry and only then map its actual observations into the existing Preflight `PASS` / `WARN` / `BLOCK` report.

F-025 must not create a parallel health subsystem.

## Security and authority boundaries

This slice grants no new ambient authority:

- no process launch;
- no file copy, restore, or replacement;
- no workstation filesystem scan;
- no remote Companion execution;
- no application automation;
- no third-party plug-in installation;
- no bypass of licensing restrictions.

A manifest launch target remains declarative metadata only.

## F-019 relationship

F-019 Show Capsule should consume these persisted canonical manifests and their content identities.

- `CONTENT_BOUND` assets can later be resolved through legitimate content-addressed export policy.
- `REFERENCE_ONLY` requirements must be carried as explicit portability limitations and must never be represented as embedded bytes.

This keeps environment definition, workstation inspection, and transport/package responsibilities separate.

## Verification scope

This slice is accepted only when tests prove:

- migration 20 applies cleanly;
- canonical persistence round-trips without identity drift;
- duplicate environment keys within one revision are rejected;
- noncanonical/tampered durable manifests fail closed;
- frozen revisions reject mutation;
- active SHOW permits reads but rejects Store and direct-SQL mutation;
- revision forks inherit environment requirements without mutating the source;
- readiness covers host/application/content mismatch, required versus optional dependencies, reference-only warnings, deterministic order, and worst-severity aggregation;
- repository module-lock, test, vet, race, and Linux ARM64 product-build gates pass on the exact PR head;
- post-merge Core CI passes on the exact merged `main` SHA.

## Deferred

- real workstation inspector/provider registry;
- VDMX/QLab/Ableton/TouchDesigner-specific observation adapters;
- Operator API/UI;
- application launch/open/reconnect authority;
- file capture/copy/backup/restore workflows;
- SHOW Preflight wiring based on real observations;
- F-019 Show Capsule packaging and restore.
