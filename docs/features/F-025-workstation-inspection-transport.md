# F-025 — Authenticated Workstation Inspection Transport

**Status:** Phase 3 implementation slice
**Feature ID:** F-025
**Builds on:** Execution Environment Manifest v1, F-025 persistence/readiness, authenticated macOS Companion runtime session

## Purpose

This slice gives StageCore a truthful way to obtain workstation facts needed by `executionenv.EvaluateReadiness` without converting inspection into execution authority.

Inspection reuses the existing authenticated Hub ↔ Companion WebSocket session. It does not add a second runtime endpoint, shell channel, process launcher, or application-specific state to Core.

## Wire contract

Runtime schema v1 adds two messages:

- `inspection.request` — Hub to Companion;
- `inspection.result` — Companion to Hub.

An inspection request contains:

- a unique `inspection_id` used only for correlation/idempotency;
- the manifest `adapter_key`;
- the exact normalized, canonical Execution Environment Manifest whose declared requirements may be inspected;
- a bounded `timeout_ms` between 1 and 30000 ms.

The full manifest is intentionally supplied instead of an ambient workstation query. A provider therefore receives the precise application, asset, extension, and binding requirements that StageCore is asking it to verify.

An inspection result contains:

- the same `inspection_id` and `adapter_key`;
- `COMPLETED`, `UNSUPPORTED`, or `FAILED`;
- an explicit error code/summary when appropriate;
- an engine-neutral observation only for a completed inspection.

The observation vocabulary matches the existing F-025 readiness evaluator:

- host OS and architecture;
- application presence/version/constraint result;
- declared asset presence, inspectability, hash and size;
- declared external extension presence/version/constraint result;
- declared binding presence.

## Companion provider registry

The macOS Companion owns a bounded registry of read-only `CompanionInspectionProvider` implementations keyed by exact `adapter_key`.

Registration rejects:

- an empty adapter key;
- duplicate adapter keys.

If no provider exists for a requested adapter, the Companion returns `UNSUPPORTED / INSPECTION_ADAPTER_UNSUPPORTED` with no fabricated observation.

This slice deliberately ships the registry and transport contract before any VDMX, QLab, Ableton Live, TouchDesigner, or other production adapter. A later adapter can be added without changing the Core observation model or granting generic execution authority.

## Separation from execution

Inspection does not enter:

- `CompanionCapabilityExecutor`;
- `ExecutionGuard`;
- `execution.request` / `execution.result`;
- Cue execution idempotency or acknowledgement semantics.

`WebSocketCompanionAgent` routes `inspection.request` through `CompanionInspectionRouter` before handing non-inspection messages to the existing `CompanionSession`.

This keeps inspection and execution registries disjoint even though they share the authenticated transport.

## Authentication and revocation

The Hub sends inspection only over a currently connected authenticated Companion runtime session and revalidates the runtime token before sending.

The Companion router also refuses inspection unless the current runtime state remains authenticated.

The Hub already polls runtime-session revocation. When a session is revoked or the WebSocket disconnects:

- the runtime connection closes;
- pending inspection cannot gain new authority;
- the caller receives a failed/interrupted result rather than a guessed observation;
- a new inspection requires a newly authenticated runtime session.

## Correlation and replay boundary

The Hub retains a bounded inspection result window.

For a given Companion and `inspection_id`:

- the first request owns the pending record and binds the ID to the SHA-256 of its canonical manifest;
- concurrent/duplicate calls with the same declared requirements wait for or reuse that same terminal result;
- reuse of the same ID with different declared requirements fails with `INSPECTION_ID_CONFLICT` before another probe runs;
- a terminal ID is not immediately recycled;
- late results cannot replace an already terminal result;
- results from a replacement connection cannot satisfy a request owned by the old connection;
- an `adapter_key` mismatch closes the connection and fails the inspection.

This is independent from execution replay protection.

## Size and timeout bounds

The existing runtime message limit remains 64 KiB.

If the canonical manifest plus inspection envelope exceeds that transport bound, the Hub fails closed with `INSPECTION_REQUEST_TOO_LARGE`; the limit is not silently raised for F-025.

Inspection duration defaults to 5 seconds and may not exceed 30 seconds. Both Hub and Companion enforce the bound.

## Security and privacy boundary

A read-only inspection provider may inspect only facts necessary to answer the requirements declared in the supplied manifest.

This slice grants no authority for:

- shell or arbitrary command execution;
- launching/quitting/restarting applications;
- application automation;
- broad filesystem inventory or home-directory scanning;
- file copy, mutation, restore, replacement, or deletion;
- installing/updating extensions or software;
- bypassing licensing or application security controls;
- transmitting unrelated workstation files or user data.

Future adapter implementations must keep their probes deterministic and narrowly tied to declared requirements.

## Readiness integration

A completed transport result produces the existing `executionenv.Observation`, which can be passed directly to `executionenv.EvaluateReadiness`.

`UNSUPPORTED`, `FAILED`, timeout, disconnect, invalid session, malformed result, or adapter mismatch do **not** manufacture an observation and therefore cannot become a false PASS.

This slice still does not automatically add execution-environment checks to SHOW Preflight. That integration should happen only after at least one real production adapter supplies truthful observations.

## Verification

Acceptance requires tests proving:

- registered Companion providers return engine-neutral observations;
- unknown adapters return `UNSUPPORTED` without an observation;
- unauthenticated inspection never calls a provider;
- duplicate provider keys are rejected;
- non-inspection runtime messages continue to fall through to the existing Companion session;
- authenticated Hub inspection reaches the Companion over the existing runtime WebSocket;
- the returned observation can drive the existing readiness evaluator;
- duplicate `inspection_id` with identical requirements does not repeat the workstation probe;
- duplicate `inspection_id` with different requirements fails before a second probe;
- revocation removes inspection authority;
- invalid manifests and excessive timeout values fail before transport;
- Core CI and Companion macOS CI pass on the exact PR head.

## Deferred

- production VDMX/QLab/Ableton/TouchDesigner inspection providers;
- Operator API/UI for requesting and viewing inspection;
- persistence/history of inspection reports;
- automatic SHOW Preflight wiring;
- application launch/open/reconnect controls;
- capture/copy/backup/restore workflows;
- F-019 Show Capsule packaging and restore.
