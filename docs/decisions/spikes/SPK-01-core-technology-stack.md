# SPK-01 — Core Technology Stack

**Status:** ACCEPTED for first implementation baseline  
**Scope:** StageCore Hub core, persistence, browser client transport, build/deployment shape  
**Validated by:** `prototypes/spk-01-core-stack`

## Decision

Use the following first implementation stack:

| Concern | Decision |
|---|---|
| Hub language/runtime | **Go** |
| Hub HTTP server | Go standard `net/http` first; add a router only when routing complexity proves it is needed |
| Persistent database | **SQLite 3**, local filesystem, **WAL mode** |
| Go SQLite integration | Prefer a CGo-free driver; `modernc.org/sqlite` is the current candidate, pinned only after cross-build/runtime validation |
| Browser command API | Versioned **HTTP + JSON** |
| Browser realtime state/events | **Server-Sent Events (SSE)** for the first product slice |
| Companion persistent channel | Separate transport-neutral contract; WebSocket remains the leading implementation candidate for SPK-03 |
| Internal runtime communication | In-process typed dispatch/queues; no Redis/NATS/Kafka dependency in MVP |
| Product Web UI | **TypeScript + React + Vite** |
| Static UI delivery | Hub serves built frontend assets; Go `embed` is acceptable for packaged releases |
| Logging | Go `log/slog` structured logging |
| Heavy files | Filesystem/Vault services, never database BLOBs for show media |

## Why Go for the Hub

StageCore needs a small local service that can run on a development Mac, Linux Mini-PC and ARM64 Hub with predictable deployment. Go directly targets Linux/macOS and ARM64/amd64 and builds executable binaries without requiring the stage machine to run a JavaScript application server/runtime for the Hub.

This keeps the critical runtime implementation smaller than making Node.js/TypeScript responsible for both UI and show-control execution. TypeScript remains the preferred UI language where its ecosystem gives us the most value.

Rust remains a valid future option for narrowly scoped components that prove they need stronger low-level control, but using it as the first Hub implementation would add implementation complexity before StageCore has evidence that Go cannot meet the latency/reliability gates.

## Why SQLite

The StageCore MVP has one authoritative Hub and does not need a remote database server. SQLite gives transactional local persistence and WAL mode allows reads to proceed concurrently with writes while keeping the database on the same host — exactly the intended Hub topology.

Important operational rules:

- database files stay on the Hub's local SSD/NVMe, not a network filesystem;
- WAL mode is enabled explicitly;
- write transactions remain short;
- checkpoint behavior is measured during soak/power-loss testing;
- state backups use a SQLite-safe consistent backup method, not a blind copy of a live database file;
- migration SQL is versioned in the repository and applied before normal authority starts.

If measured MVP workloads later demonstrate a real multi-writer/server-database requirement, migration is possible because persistence remains behind repository interfaces. PostgreSQL is not introduced preemptively.

## HTTP + SSE for Web Clients

The browser needs two distinct behaviors:

1. commands/queries such as Create Project, Publish, GO and Notes;
2. realtime observation such as `cue.started`, `cue.completed`, readiness and errors.

For the first Web client:

- commands use normal authenticated HTTP requests;
- realtime events use SSE;
- reconnecting SSE never causes command replay;
- Core Command/Event semantics remain transport independent.

This is simpler than forcing every browser operation through a bidirectional socket. The Companion has different requirements and will get its own persistent-channel spike.

## UI Decision

The product UI uses TypeScript + React + Vite. The SPK-01 executable harness deliberately uses plain embedded HTML/JavaScript because this spike measures the Hub/API/runtime shape, not the final design system.

The production UI must remain a client of the same Hub API. No Cue logic or authoritative show state belongs only in React components.

## Dependency Rule

Start with the standard library where it is sufficient. Add external infrastructure only when a concrete requirement justifies it.

Not selected for MVP core:

- Redis;
- NATS/Kafka/RabbitMQ;
- PostgreSQL server;
- Kubernetes;
- Docker as a runtime requirement;
- Electron as a requirement for the Web UI;
- cloud authentication/runtime dependency.

Containers may still be useful for development/CI later; they are not part of the StageCore runtime contract.

## Prototype Result

`prototypes/spk-01-core-stack` compiles and passes automated tests with Go 1.23 using only the standard library. It demonstrates:

```text
Create Project
 -> Add Cue
 -> Publish immutable spike Snapshot
 -> GO
 -> simulated Action completes
 -> cue/action Events emitted
 -> execution history persisted
 -> process restart
 -> Project + Snapshot + history reload correctly
```

A manual HTTP run was also executed: the Hub returned healthy state, executed one Cue as `COMPLETED`, was restarted, and reloaded the same Project, Snapshot and one execution record.

The spike uses an atomic JSON file persister only to keep this validation zero-dependency. That persister is **not** an accepted production database. The next Core implementation slice replaces it with SQLite and reruns the same tests plus crash/WAL checks.

## Rejection / Revisit Triggers

Revisit this decision only with evidence, such as:

- Go misses defined latency/soak targets on reference hardware;
- SQLite write contention appears under the representative StageCore workload;
- the Hub genuinely requires multiple independent writer processes or remote database authority;
- SSE cannot satisfy measured client realtime behavior;
- a concrete native platform requirement cannot be cleanly expressed through the current API contracts.

## Next Spike

**SPK-02 — Real OSC**

Replace the simulated Action path with one real `osc.send` UDP adapter and a reproducible receiver while preserving the exact Core execution/result/event semantics.
