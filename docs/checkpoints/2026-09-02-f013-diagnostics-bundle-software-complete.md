# F-013 — Exportable Diagnostics Bundle — Software Acceptance Checkpoint

**Date:** 2026-09-02  
**Feature:** F-013  
**Status:** SOFTWARE COMPLETE — PHYSICAL PHASE 2 QUALIFICATION PENDING

## Scope closed by this checkpoint

The supported operator/appliance command is `stagecore support-bundle`. It creates a bounded, private, redacted `.tar.gz` archive suitable for offline support without requiring a healthy Hub or developer tooling.

The implemented bundle includes the documented `manifest.json`, Doctor report, host/build summary, strict allowlisted deployment metadata, managed binary metadata, read-only aggregate database state summary when available, and bounded recent Hub journal output.

The archive does not export the raw StageCore database, raw `stagecore.env`, Vault/media payloads, private keys, browser/auth sessions, pairing/setup credentials, raw audit rows, extension package payloads, or arbitrary filesystem trees.

## Automated acceptance evidence

The `internal/diagnosticsbundle` test suite proves the F-013 foundation acceptance contract:

- archive creation uses `.tar.gz` and mode `0600`;
- existing output is never overwritten;
- Doctor `BLOCKED` does not prevent bundle creation;
- collector failures are retained as manifest collection warnings;
- deployment environment export is strict allowlist-only;
- the current migrated SQLite schema produces the aggregate state summary through a read-only path;
- journal line requests and captured bytes are bounded;
- every manifest entry records a size and SHA-256 that match the bytes actually stored in the produced archive;
- injected deployment, Doctor and journal secrets are absent after archive extraction;
- JWT/token/authorization/query-credential/private-key patterns and sensitive structured fields are redacted.

The manifest entry hash/size assertion is an explicit regression test added by this checkpoint so acceptance is proven rather than inferred from implementation.

## Failure and privacy semantics

Diagnostic collectors are best-effort, while archive creation itself is strict. Source-collection failure does not discard other evidence, but unsafe output conditions such as an existing destination or filesystem/archive failure stop the command.

Any future diagnostic source must define its privacy boundary and add leak/regression tests before it is included.

## Deferred scope

Automatic support upload, cloud case integration, packet capture, arbitrary database export, remote support access, automatic personal-data collection, and UI-driven bundle creation remain outside the F-013 foundation contract.

## Remaining verification

Software acceptance is complete when CI for the exact checkpoint head passes. Physical verification remains deliberately deferred to the cumulative Phase 2 Raspberry Pi ARM64 qualification gate.

That later gate must run the real `stagecore support-bundle` command, verify archive permissions and contents, and confirm that known deployment secrets are absent.

No physical qualification claim is made by this checkpoint.
