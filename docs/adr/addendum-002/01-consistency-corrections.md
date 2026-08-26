# 01 — Consistency Corrections

This file resolves the known cross-document differences identified before M0. These resolutions are authoritative immediately; they are not waiting for a later implementation to become true.

## 1. Event Envelope — `trace_context`

`02 — System Architecture v0.1` includes `trace_context` in the Event Envelope while `04 — Event & Command Contracts v0.1` omitted it from its field table.

### Resolution

The StageCore Event Envelope includes:

- `trace_context` — optional, non-sensitive trace propagation metadata.

Rules:

- it may be absent;
- it must never contain secrets, credentials, raw tokens, passwords, private keys, or protected Project payloads;
- consumers must not require it to understand the Event's business semantics;
- unknown optional trace fields are ignored safely.

This addendum supersedes the omission in 04 until that specification is versioned again.

## 2. Event `sequence` Scope

The older contract described `sequence` only as optional stream ordering, which is too ambiguous for implementation.

### Resolution for MVP

When the Hub appends an authoritative `EventRecord`, it assigns a monotonically increasing Hub-journal `sequence`.

Properties:

- ordering scope is the authoritative Hub event journal;
- it is not a globally distributed clock;
- it is not evidence that one physical event happened before another on different machines;
- it is suitable for deterministic local replay, UI resume/catch-up, and ordered journal inspection;
- external/source transport sequence numbers remain source/transport metadata and do not overwrite the Hub journal sequence.

## 3. `RoleAssignment` State Vocabulary

`03 — Data Model v0.1` lists a smaller assignment status set than the later Companion specification.

### Resolution

Persisted/operational assignment states for the MVP are:

- `ASSIGNED`
- `SYNCING`
- `READY`
- `DEGRADED`
- `OFFLINE`
- `MISMATCH`
- `RELEASED`

`UNASSIGNED` is a derived **Machine Role state** meaning no active assignment row exists. It is not required as a persisted `RoleAssignment.status` value.

The later Companion Specification remains authoritative for readiness/reconnect behavior.

## 4. Project Revision Mutability

The baseline correctly separates Draft from Published Runtime but did not make the revision freeze rule explicit enough for SQL implementation.

### Resolution

- `DRAFT` revision: mutable through normal edit operations.
- `VALIDATED` revision: frozen/immutable.
- `SUPERSEDED` revision: frozen/immutable.
- Any edit requested against a frozen revision creates a new `DRAFT` child revision with `parent_revision_id` pointing to the source revision.
- Publish accepts only a known validated/frozen revision.
- Publishing does not mutate the revision; it creates a distinct immutable `RuntimeSnapshot`.
- A later edit can never change an already Published Snapshot by modifying source rows in place.

## 5. Historical Open-Technology Statements

Some architecture/security documents correctly recorded technology choices as open when they were written.

### Resolution

Accepted decision spikes resolve those historical questions where they overlap:

- Hub backend: Go — `SPK-01`.
- Database architecture: SQLite/WAL — `SPK-01`.
- Browser UI: TypeScript + React + Vite — `SPK-01`.
- Browser command/realtime baseline: HTTP+JSON + SSE — `SPK-01`.
- Real OSC semantics: OSC 1.0 over UDP — `SPK-02`.
- macOS Companion core/channel: Swift + WebSocket — `SPK-03`.
- Plugin process model/IPC: external process + JSON Lines stdio — `SPK-04`.
- Vault transfer mechanism: filesystem/SHA-256/HTTP Range — `SPK-05`.
- Hub deployment: 64-bit Linux, amd64/arm64, native binary + systemd — `SPK-06`.

Older text remains useful for requirements and historical context, but it must not be read as reopening these accepted choices.

## 6. Source-of-Truth Precedence

`docs/SOURCE_POLICY.md` is authoritative for documentation precedence.

The practical rule is:

1. explicit newer ADR/specification delta or accepted decision;
2. more specific active specification;
3. system architecture;
4. older broad product baseline.

A decision spike may select technology but cannot silently weaken product/safety/security/reliability requirements.

## 7. Prototype vs Product

Anything under `prototypes/` remains decision evidence only.

M0 starts the first real StageCore product source. Prototype behavior may be reused conceptually or copied after review, but no prototype is authoritative runtime code merely because its spike passed.

## 8. No Known Pre-M0 Contradiction Left Unowned

The review found no remaining known contradiction that blocks M0 after applying the resolutions above.

Any newly discovered conflict must be treated as a documentation defect and resolved before implementing the affected slice.