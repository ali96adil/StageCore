# 03 — Persistence Semantics & Data Rules

This file turns the logical Data Model into safe M0 persistence rules without pretending to define every future table.

## 1. Database Location

Reference Hub database path:

```text
<STAGECORE_DATA_ROOT>/db/stagecore.sqlite3
```

The database lives on authoritative local Hub storage, not on a network share.

Vault objects remain outside SQLite.

## 2. Required SQLite Connection Settings

Every product connection must be configured so these effective settings are true:

- `foreign_keys = ON`
- `journal_mode = WAL`
- `synchronous = FULL`
- `busy_timeout = 5000` ms
- defensive connection mode enabled where supported by the selected driver
- double-quoted-string compatibility disabled where supported (`DQS` off)
- `trusted_schema = OFF`

For the selected modernc driver, the initial DSN direction is equivalent to:

```text
_foreign_keys=on
_journal_mode=WAL
_synchronous=FULL
_busy_timeout=5000
_defensive=1
_dqs=0
_pragma=trusted_schema(OFF)
_txlock=immediate
```

M0 startup must query and verify critical effective PRAGMAs rather than assuming the DSN was honored. If mandatory settings are not active, readiness fails.

## 3. Connection Pool Policy — M0

M0 uses one open SQLite connection for authoritative operations:

- `SetMaxOpenConns(1)`
- `SetMaxIdleConns(1)`

Reason: correctness and deterministic write behavior come before read concurrency in the persistence slice.

Before M1/M6 performance acceptance, StageCore may add a bounded read strategy only after tests show it is needed. That change must retain one-writer semantics and all connection PRAGMAs.

## 4. Transaction Rules

Use explicit transactions whenever one logical operation changes more than one row/entity.

Examples:

- creating a Project + initial ProjectRevision;
- creating/updating a Cue and its Actions;
- moving/reordering multiple Cues;
- cloning a frozen ProjectRevision into a new Draft;
- updating a Route and RouteActions together.

Rules:

- no partial aggregate success;
- return an error if commit fails;
- never report success before commit completes;
- context cancellation/timeout rolls the transaction back;
- retry is operation-specific and never blindly replays a non-idempotent runtime action.

M0 concerns configuration persistence only; it does not invent runtime retry behavior.

## 5. Schema Naming

Initial SQL naming convention:

- plural `snake_case` table names;
- explicit primary key names, e.g. `project_id`, `revision_id`, `cue_id`;
- explicit foreign-key columns;
- state/enum values stored as uppercase canonical text matching contracts;
- booleans stored as constrained integer `0/1` where appropriate;
- timestamps stored as UTC Unix microseconds in `INTEGER` columns.

Critical closed-state fields should use both domain validation and DB constraints when the vocabulary is stable.

## 6. Foreign-Key / Delete Policy

Foreign keys are mandatory.

Initial rules:

- definitions owned by a Draft revision may be removed/changed through explicit domain operations;
- child definition rows such as Actions may cascade only where deletion of the owning definition is itself valid and explicit;
- published/frozen revision data is not edited/deleted as part of normal project editing;
- execution/session/event history introduced later is append-oriented and must not disappear because a Project definition is edited;
- destructive whole-project purge is not an M0 feature.

When uncertain, prefer `RESTRICT` over broad cascade.

## 7. Project / ProjectRevision Invariants

### Project

- stable `project_id`;
- points to the current editable revision only through an explicit relationship;
- changing Project metadata does not rewrite frozen revisions.

### ProjectRevision

- exactly one owning Project;
- `DRAFT` is editable;
- `VALIDATED` and `SUPERSEDED` are immutable;
- a frozen revision can be the parent/template for a new Draft;
- `revision_number` is monotonic within a Project;
- a Published Runtime Snapshot later references an exact frozen revision.

A store method must reject attempts to mutate frozen revision-owned definitions.

## 8. Cue / Action Ordering

`order_index` remains explicit business data.

Do not rely on UUID order or row insertion order for Cue/Action sequence.

Within one owning scope, ordering constraints must prevent ambiguous duplicate positions after a committed reorder operation. Reorders should be transactional.

## 9. Alias / Input / Route Persistence Boundary

M0 persists these as configuration definitions only:

- `ProjectDeviceAlias`
- `InputDefinition`
- `OutputDefinition` where required by Route representation
- `Route`
- `RouteAction`

M0 does **not** execute them.

Protocol-specific IP/port logic must not leak into Cue/Route core tables when a logical target/capability reference exists.

## 10. JSON Fields

JSON-backed fields are for schema-driven/extension data, not a replacement for relational modeling.

Rules:

- reject malformed JSON before persistence;
- store a canonical empty object/array rather than several semantically equivalent ad hoc encodings where practical;
- version plugin/contract-owned payloads explicitly;
- no secrets inside Project JSON fields;
- fields that require FK, uniqueness, sorting, filtering, or safety validation should be first-class columns/entities.

## 11. Migrations

Migration files are embedded and ordered.

Initial pattern:

```text
internal/db/migrations/
  00001_initial_schema.sql
  00002_...
```

Migration policy:

- one committed schema change = one explicit migration;
- never edit a migration already used by a released database; add a new migration;
- startup applies pending migrations before serving normal requests;
- migration failure blocks readiness;
- schema version is inspectable;
- tests include migration from empty DB and from at least the immediately previous fixture once multiple versions exist.

## 12. Backup Proof Required by M0

M0 does not implement the whole StageCore backup product, but the selected DB stack must prove a consistent local copy can be produced and reopened.

Initial validation mechanism: SQLite `VACUUM INTO` to a temporary destination, followed by reopen and data verification.

Rules:

- destination is created outside the live DB path;
- incomplete/failed output is not promoted as a valid backup;
- validation checks schema and representative persisted rows;
- later Storage/Backup implementation may wrap or supersede the mechanism without changing the requirement for verified consistency.

Before any future destructive production migration, StageCore must create/verify an appropriate pre-migration backup according to the Storage specification.

## 13. Startup / Readiness Order

Reference M0 startup order:

```text
resolve config/data root
 -> create/check required local directories
 -> open SQLite with required settings
 -> verify critical PRAGMAs
 -> apply embedded migrations
 -> verify schema/version
 -> run lightweight DB readiness check
 -> expose READY
```

If authoritative storage is missing/unwritable, database open fails, required PRAGMAs fail, or migrations fail, the Hub must not advertise READY.

## 14. Product Source Layout

Initial real source layout:

```text
cmd/stagecore-hub/
internal/app/
internal/config/
internal/db/
internal/db/migrations/
internal/domain/
internal/id/
internal/store/
internal/clock/
internal/httpapi/
```

Guidance:

- `domain` owns product types/invariants, not SQL;
- `store` owns persistence-facing interfaces/operations;
- `db` owns SQLite open/config/migrations/transactions;
- `httpapi` remains a thin adapter around application/domain operations;
- runtime/plugin/companion packages are added only in their implementation slices.

Do not copy the prototype directory structure mechanically into product code.

## 15. Test Requirements for M0

M0 must include repeatable tests for:

- new database migration from empty;
- Project + initial revision atomic creation;
- Cue/Action persistence and reload;
- alias/Input/Route persistence and reload;
- frozen revision mutation rejection;
- transaction rollback on induced failure;
- foreign-key rejection;
- restart/reopen preserving committed state;
- backup copy/reopen verification;
- required PRAGMA verification;
- Linux `amd64` normal tests;
- Linux `arm64` CGo-free cross-build;
- `go test -race` on supported native CI where practical.

Passing happy-path CRUD alone is not M0 completion.