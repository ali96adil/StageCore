# StageCore Documentation

## Reading Order

- **00 — Master Plan v0.2:** [Product baseline](baseline/master-plan-v0.2/README.md)
- **01 — Architectural Decisions Addendum 001:** [Approved design decisions](adr/addendum-001/README.md)
- **02 — System Architecture v0.1:** [System architecture](architecture/system-architecture-v0.1/README.md)
- **03 — Data Model v0.1:** [Entities and relationships](specifications/03-data-model-v0.1.md)
- **04 — Event & Command Contracts v0.1:** [Runtime contracts](specifications/04-event-command-contracts-v0.1.md)
- **05 — MVP Product Specification v0.1:** [Executable MVP behavior and acceptance](specifications/05-mvp-product-spec-v0.1/README.md)
- **06 — Plugin Contract v0.1:** [Capability extensions and native UI integration](specifications/06-plugin-contract-v0.1/README.md)
- **07 — Companion Specification v0.1:** [Clients, pairing, roles, sync and macOS Companion](specifications/07-companion-spec-v0.1/README.md)
- **08 — Storage & Vault Specification v0.1:** [Vault, heavy file sync, software repository, backup and SHOW-safe storage](specifications/08-storage-vault-spec-v0.1/README.md)
- **09 — Security Model v0.1:** [Identity, trust, users, pairing, permissions and secrets](specifications/09-security-model-v0.1/README.md)
- **10 — Testing & Reliability Plan v0.1:** [Verification, failure injection and rehearsal release gates](specifications/10-testing-reliability-plan-v0.1/README.md)

## Implementation Decisions

The `00–10` baseline defines what StageCore must do. Concrete stack choices live separately under [Engineering Decisions](decisions/README.md) so technology can be validated or changed without silently rewriting product intent.

Current decision-spike status:

- **SPK-01 — Core Technology Stack:** ACCEPTED — Go Hub, SQLite/WAL persistence direction, HTTP+JSON + SSE browser transport, TypeScript/React/Vite product UI.
- **SPK-02 — Real OSC:** ACCEPTED — OSC 1.0 UDP `osc.send`, logical target mapping and `TRANSPORT_ONLY` acknowledgement.
- **SPK-03 — macOS Companion:** next.

## Documentation Convention

Documents are ordered numerically and named by the subject they cover. New specification files use the pattern `<NN>-<descriptive-topic>-v<version>` so sequence and purpose remain obvious.

Product/specification documents must distinguish implementable milestone requirements from future ideas. Future capabilities stay explicitly out of scope until promoted by a documented decision.

- [Specifications index](specifications/README.md)
- [Engineering decisions](decisions/README.md)
- [Documentation source policy](SOURCE_POLICY.md)
