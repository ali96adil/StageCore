# StageCore Specifications

Documentation sequence uses a numeric prefix so the project can be read in order while every chapter remains named by the subject it covers.

## Sequence

- `00` — Master Plan v0.2 — product vision and boundaries
- `01` — Architectural Decisions Addendum 001 — approved architecture decisions
- `02` — System Architecture v0.1 — system components and runtime architecture
- [`03` — Data Model v0.1](03-data-model-v0.1.md) — entities, ownership and relationships
- [`04` — Event & Command Contracts v0.1](04-event-command-contracts-v0.1.md) — runtime communication semantics
- [`05` — MVP Product Specification v0.1](05-mvp-product-spec-v0.1/README.md) — executable product behavior and release gates
- [`06` — Plugin Contract v0.1](06-plugin-contract-v0.1/README.md) — capability extensions, native UI contributions and OSC reference plugin
- [`07` — Companion Specification v0.1](07-companion-spec-v0.1/README.md) — client/agent separation, pairing, roles, sync, bootstrap and macOS reference Companion
- [`08` — Storage & Vault Specification v0.1](08-storage-vault-spec-v0.1/README.md) — authoritative storage, managed files, media sync, software repository, backup and SHOW-safe transfer policy
- [`09` — Security Model v0.1](09-security-model-v0.1/README.md) — Hub/user/device identity, pairing, authorization, secrets, Plugin permissions and local security operations
- [`10` — Testing & Reliability Plan v0.1](10-testing-reliability-plan-v0.1/README.md) — automated/fault/performance/recovery testing and first-rehearsal release gates

## Baseline State

The ordered planning/specification baseline `00–10` is now defined. The next engineering phase is **Technology Selection / Decision Spikes**, followed by implementation slices and GitHub Issues derived from these executable specifications.

## Naming Rule

Use: `<NN>-<descriptive-topic>-v<version>.md`

For a document split into multiple files, keep the top-level sequence number in the folder name and use descriptive numbered filenames inside it.

## Implementation Rule

Specifications must remain executable: behavior, data/contracts, failure states, and acceptance criteria should be explicit. Features that cannot be implemented and tested in the target milestone stay out of scope rather than becoming aspirational requirements.
