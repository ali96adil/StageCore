# 05 — Final M0 Entry Gate

## Verdict

**READY FOR M0 — NO KNOWN UNOWNED PRE-M0 DECISIONS**

This verdict means the design/technology choices needed to begin Core Persistence are resolved. It does **not** mean M0 implementation tests have already passed.

## Required Inputs — Resolved

- Hub architecture: authoritative local-first Hub.
- Product language: Go.
- Minimum Go baseline: 1.26; current Go 1.27 also tested in CI.
- Database: SQLite with WAL.
- SQLite driver: `modernc.org/sqlite` v1.57.0.
- DB API: standard `database/sql`.
- Migration tool: `github.com/pressly/goose/v3` v3.27.3 with embedded SQL migrations.
- ORM/query abstraction: no ORM in M0; explicit SQL/store layer.
- Entity IDs: UUIDv7, initial implementation `github.com/google/uuid` v1.6.0.
- Persisted timestamps: UTC Unix microseconds in SQLite INTEGER columns.
- Extensible payloads: validated/versioned JSON TEXT where appropriate.
- DB path: `<STAGECORE_DATA_ROOT>/db/stagecore.sqlite3`.
- SQLite safety settings: foreign keys ON, WAL, synchronous FULL, busy timeout, defensive mode, DQS off, trusted schema off.
- M0 connection policy: one open/idle DB connection until later performance evidence justifies bounded read concurrency.
- Revision semantics: only DRAFT mutable; validated/superseded revisions frozen; edit creates new Draft.
- Event contract discrepancy: `trace_context` is part of the Event Envelope.
- Event ordering: Hub journal sequence is monotonic within the authoritative Hub event journal.
- Role assignment states: resolved by Addendum 002; `UNASSIGNED` is derived role state.
- Product source layout: real code under `cmd/` + `internal/`, not under `prototypes/`.
- HTTP/logging/config baseline: standard `net/http`, `log/slog`, flags/environment.
- LAN exposure: blocked until SEC0–SEC2 security convergence gate.

## M0 Definition of Done

M0 is complete only when the real StageCore Hub code demonstrates all of the following:

1. real Go module/source tree exists outside `prototypes/`;
2. dependencies are pinned and checksums committed;
3. SQLite opens with required effective settings;
4. migrations apply from a clean database;
5. Project + initial ProjectRevision are created atomically;
6. Cue/Action definitions persist/reload;
7. Project alias/Input/Output/Route foundations persist/reload;
8. frozen revision mutation is rejected;
9. foreign-key violations are rejected;
10. induced transaction failure leaves no partial aggregate;
11. close/reopen/restart preserves committed data;
12. a consistent local DB copy can be created and reopened/verified;
13. Linux amd64 tests pass;
14. Linux arm64 CGo-free cross-build passes;
15. relevant race tests pass on native CI;
16. no control endpoint is exposed to the Stage LAN before the Security gate.

## M0 Explicit Non-Goals

M0 does not implement:

- Cue execution;
- Runtime Snapshot publication;
- OSC execution;
- Plugin supervisor product integration;
- Companion pairing/runtime channel;
- Vault/media transfer product path;
- user authentication UI/operator UI;
- StageNodes;
- DMX;
- AI/Vision;
- cloud/HA.

These are assigned to later slices in the deferred register.

## Change-Control Rule

If implementation discovers that a pinned M0 decision cannot satisfy a mandatory test:

1. stop the affected implementation;
2. record the evidence;
3. create a superseding implementation decision/ADR;
4. update the entry baseline;
5. only then continue with the alternative.

Do not silently replace a driver, migration strategy, identity format, durability policy, or revision invariant inside a code commit.

## Entry Point

The next commit after this finalization should begin **M0 product code**, not another broad architecture document or disposable technology spike.