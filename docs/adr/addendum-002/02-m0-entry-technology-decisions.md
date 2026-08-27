# 02 — M0 Entry Technology Decisions

These decisions are pinned before product code begins. They are implementation choices, not new product requirements.

## 1. Go Toolchain Baseline

### Decision

- StageCore Hub minimum language/toolchain baseline for M0: **Go 1.26**.
- Development/CI must test the latest supported Go 1.26 patch and current Go 1.27.
- M0 code must not require Go 1.27-only language/library features while the `go` directive remains 1.26.
- Use the latest security/bug-fix patch within a supported Go release; do not pin an obsolete patch for reproducibility.

### Initial repository target

- `go 1.26.0`
- CI matrix: Go `1.26.x` + `1.27.x`.

Reason: Go 1.27 was released immediately before this baseline; using 1.26 as the minimum gives a mature supported floor while still testing the current release.

## 2. SQLite Driver

### Decision

Use:

- `modernc.org/sqlite` **v1.57.0** as the initial pinned SQLite driver;
- standard `database/sql` as the application DB API;
- `CGO_ENABLED=0` remains a required release property for the Hub build path.

Reasons:

- CGo-free SQLite implementation;
- standard `database/sql` integration;
- aligns with `SPK-06` static/cross-build deployment direction;
- supports DSN-level connection PRAGMAs so required settings apply consistently per connection.

### Dependency rule

Commit `go.mod` and `go.sum`. Do not manually force an incompatible `modernc.org/libc` version; the module graph must retain the driver-compatible transitive version.

### Acceptance gate

The technology choice is resolved, but M0 is blocked from completion unless tests prove:

- Linux `amd64` build/test;
- Linux `arm64` CGo-free cross-build;
- WAL activation;
- foreign-key enforcement;
- restart persistence;
- migration behavior;
- consistent local backup/reopen behavior.

If the driver fails one of these mandatory gates, changing drivers requires a superseding implementation decision. The choice is not silently swapped inside code review.

## 3. Schema Migration Tool

### Decision

Use:

- `github.com/pressly/goose/v3` **v3.27.3**;
- SQL migrations embedded into the Hub binary with Go `embed`;
- SQLite dialect through the same `database/sql` connection.

### Policy

- migrations are ordered and versioned;
- product startup automatically applies approved **up** migrations before becoming READY;
- migration failure makes readiness fail and stops normal service startup;
- production startup never automatically runs a down migration;
- down migrations, when safe and useful, are development/test tooling only;
- migration history remains in the database and is tested;
- destructive future migrations require an explicit migration plan and verified pre-migration backup.

No schema is modified ad hoc from request handlers.

## 4. ORM / Query Layer

### Decision

M0 uses **no ORM** and no runtime query builder.

Use:

- `database/sql`;
- explicit SQL;
- small repository/store interfaces around domain operations;
- explicit transactions for aggregate writes.

Reason: the MVP schema is modest, transactional behavior needs to remain obvious, and StageCore should not introduce an ORM abstraction before there is evidence it reduces complexity.

A later code-generation/query-layer decision may be evaluated if query volume becomes difficult, but it must not change persistence semantics.

## 5. Stable Entity IDs

### Decision

Use **UUIDv7** for persisted StageCore entity IDs.

Initial Go implementation:

- `github.com/google/uuid` **v1.6.0**;
- generate with `uuid.NewV7()`;
- store canonical lowercase UUID text in SQLite `TEXT` columns;
- expose the same canonical representation through JSON/API contracts.

Rules:

- IDs are opaque to business logic;
- names, filenames, IPs, hostnames and display labels are never identity;
- ID ordering is a storage/debug convenience only and must not replace explicit `order_index`, event sequence, or timestamps.

## 6. Time Representation

### Decision

Persist ordinary timestamps as **UTC Unix microseconds in SQLite `INTEGER` columns**.

Rules:

- convert to/from `time.Time` in Go at repository boundaries;
- external JSON/API timestamps use RFC3339/RFC3339Nano UTC representation;
- Event journal ordering uses its explicit `sequence`, not timestamp sorting alone;
- durations use explicit unit-suffixed columns/fields such as `_ms` or `_us` rather than overloaded timestamps.

Reason: integer UTC storage is unambiguous, sortable and independent of string-format quirks.

## 7. Structured/Extensible Payload Storage

### Decision

For configuration/payload fields that are intentionally schema-driven or plugin-extensible:

- persist UTF-8 JSON as SQLite `TEXT`;
- validate with Go JSON decoding/`json.Valid` before write;
- keep an explicit schema/version field where the payload is externally versioned;
- do not depend on SQLite JSON functions for correctness in M0;
- do not hide query-critical first-class fields inside JSON.

Runtime Snapshot canonical hashing will use deterministic application serialization defined in M1/M5; it must not hash arbitrary raw database JSON text.

## 8. HTTP Server / Routing

### Decision

M0 uses Go standard-library `net/http`/`http.ServeMux` for the minimal development API/health surface.

Do not add a web framework until concrete routing/middleware needs justify it.

Until the Security convergence gate is met, control/configuration endpoints remain loopback/private-test only. A Stage LAN is not an authentication boundary.

## 9. Logging

### Decision

Use Go standard-library `log/slog` for structured Hub logs.

Rules:

- JSON handler for service/deployment logs;
- human-readable handler may be used in local development;
- correlation/entity IDs should be structured fields;
- secrets/tokens/passwords/private keys are never log fields;
- P0/P1 runtime paths later keep logging bounded/asynchronous according to the architecture.

## 10. Configuration Input

### Decision

M0 configuration remains intentionally small:

- command-line flags and environment variables;
- flags override environment values;
- explicit defaults for development only;
- authoritative data paths retain `STAGECORE_DATA_ROOT` / `STAGECORE_VAULT_ROOT` semantics from SPK-06.

Do not introduce YAML/TOML configuration merely to mirror environment variables.

## 11. Dependency Discipline

M0 third-party dependencies are limited to dependencies that clearly buy correctness or portability:

- SQLite driver;
- migration library;
- UUID implementation.

Prefer the Go standard library for HTTP, logging, JSON, crypto primitives, filesystem and tests unless a later requirement demonstrates a gap.

All direct dependency versions are pinned in `go.mod`, checksums are committed, and dependency upgrades are explicit reviewable changes.