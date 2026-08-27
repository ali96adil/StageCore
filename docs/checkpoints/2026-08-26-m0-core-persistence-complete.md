# 2026-08-26 — M0 Core Persistence Completion Checkpoint

## Verdict

**M0 — Core Persistence: COMPLETE**

StageCore now has its first real product implementation slice outside `prototypes/`.

This checkpoint records evidence for the M0 Definition of Done from `docs/adr/addendum-002/05-final-m0-entry-gate.md`. It does not promote any M1+ feature into M0.

## Product Commit

Merged product commit:

```text
3b300ccf2549f417b3f86c4de841a4530902f9ca
m0: establish core persistence foundation
```

Source is under `cmd/` and `internal/`; `prototypes/` remains evidence/reference only.

## Implemented M0 Foundation

- Go Hub module and executable entry point.
- Pinned `go.mod` and committed `go.sum`.
- SQLite through `modernc.org/sqlite` with `database/sql`.
- WAL, foreign keys, FULL synchronous mode, busy timeout, trusted-schema off, DQS/defensive verification.
- Single-connection authoritative M0 policy.
- Embedded Goose migrations.
- UUIDv7 persisted entity IDs.
- UTC Unix-microsecond persistence timestamps.
- Project + initial ProjectRevision atomic creation.
- Cue + Action persistence.
- ProjectDeviceAlias persistence.
- InputDefinition / OutputDefinition persistence.
- Route + RouteAction persistence.
- Draft-only mutation guards for revision-owned definitions.
- Cross-revision routing guardrails.
- Explicit transaction rollback behavior.
- Foreign-key rejection.
- Close/reopen restart persistence proof.
- `VACUUM INTO` local-copy creation and reopen verification.
- Loopback-only development health surface; no Stage LAN control exposure.

## CI Evidence

PR validation completed successfully before merge and the post-merge `main` workflow also completed successfully.

Post-merge workflow:

```text
M0 CI run 32960572160
commit 3b300ccf2549f417b3f86c4de841a4530902f9ca
```

Verified:

- Go 1.26.x: module-lock verification PASS.
- Go 1.26.x: `go test ./...` PASS.
- Go 1.26.x: `go vet ./...` PASS.
- Go 1.26.x: `go test -race ./...` PASS.
- Go 1.26.x: Linux ARM64 `CGO_ENABLED=0` cross-build PASS.
- Go 1.27.x: module-lock verification PASS.
- Go 1.27.x: `go test ./...` PASS.
- Go 1.27.x: `go vet ./...` PASS.

The heavier native race/ARM64 checks are intentionally performed once in the Go 1.26 matrix leg; they are not silently omitted from the workflow.

## M0 Acceptance Mapping

All sixteen M0 entry-gate completion requirements have implementation/test evidence:

1. real source tree — PASS;
2. pinned dependencies/checksums — PASS;
3. required effective SQLite settings — PASS;
4. clean embedded migration path — PASS;
5. atomic Project + initial revision — PASS;
6. Cue/Action persistence/reload — PASS;
7. Alias/Input/Output/Route persistence/reload — PASS;
8. frozen revision mutation rejection — PASS;
9. foreign-key rejection — PASS;
10. induced aggregate rollback — PASS;
11. close/reopen/restart preservation — PASS;
12. consistent local DB copy + reopen verification — PASS;
13. Linux amd64 tests — PASS;
14. Linux arm64 CGo-free cross-build — PASS;
15. native race tests — PASS;
16. no Stage LAN control before Security convergence — PASS.

## Explicit Non-Claims

M0 completion does **not** mean StageCore is rehearsal-ready. M0 does not yet provide Cue execution, Runtime Snapshot publication, OSC product execution, Plugin product integration, Companion trust, Vault media workflows, authentication/operator UI, Nodes, DMX, AI/Vision or HA/cloud behavior.

Those remain owned by the existing deferred register and later gates.

## Transition

The next implementation slice is:

**M1 — Cue Engine + Simulator**

M1 owns production Command/Event structs, `trace_context` synchronization, Hub journal sequence, minimal immutable Runtime Snapshot identity/serialization, Cue/Action execution state machine, deterministic simulated adapter, and the first runtime execution evidence.
