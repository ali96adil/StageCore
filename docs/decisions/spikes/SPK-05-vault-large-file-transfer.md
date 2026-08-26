# SPK-05 — Vault & Large File Transfer

**Status:** ACCEPTED for Vault object + resumable bulk-transfer baseline  
**Scope:** managed object import, content identity, HTTP range delivery, resumable verified cache, SHOW-mode transfer gate and runtime storage reserve  
**Validated by:** `prototypes/spk-05-vault-transfer`

## Decision

StageCore v0.1 keeps large managed content as **filesystem objects on Hub SSD/NVMe**, outside SQLite BLOB storage. The database stores metadata/manifests; the Vault stores bytes.

Managed content identity is **SHA-256**. Import uses:

```text
incoming stream
 -> Vault staging file
 -> SHA-256 calculated while writing
 -> fsync / verification boundary
 -> atomic rename to content-addressed object path
 -> metadata may reference committed object
```

Reference object layout:

```text
<vault-root>/
├── staging/
└── objects/
    └── sha256/
        └── <first-two-hex>/
            └── <full-sha256>
```

Filename, source path and modification time remain metadata. They are not content identity.

## Large-File Delivery

The first Hub transfer protocol is ordinary authenticated HTTP(S) object download with byte-range support.

```text
GET /objects/<sha256>
Range: bytes=<offset>-
```

The server returns `206 Partial Content` with `Content-Range` for resume requests and advertises `Accept-Ranges: bytes`.

The object response is streamed using bounded chunks. The Hub must not read an entire show-media file into RAM before sending it.

This same delivery primitive can later serve required media, StageCore installers and Plugin packages; authorization/policy differs by resource type, not the byte-transfer mechanism.

## Companion Cache Contract

The receiving side uses a temporary local file:

```text
required content identity
 -> inspect existing .part size
 -> request remaining byte range
 -> append to .part
 -> full SHA-256 verification
 -> atomic rename to final cache path
 -> report verified content identity
```

A partial or checksum-mismatched file never becomes READY and is never promoted as valid media.

The spike client is written in Go for repeatable cross-platform testing. Production macOS CompanionCore will implement the same protocol in Swift; SPK-05 does not claim that Swift cache code is already finished.

## SHOW-Mode Transfer Gate

Bulk transfer is lower priority than runtime work.

The accepted first behavior is:

- `EDIT`: bulk transfer allowed;
- `REHEARSAL`: explicitly needed transfer may run under bounded policy;
- `SHOW`: nonessential bulk transfer pauses at bounded chunk boundaries;
- leaving `SHOW`: eligible transfer resumes after policy re-check.

The transfer worker waits on a dedicated mode gate. Runtime HTTP/command handling is not placed behind that gate.

The prototype starts a real bulk HTTP transfer, enters `SHOW`, verifies transfer progress stops, calls an independent runtime endpoint successfully, exits `SHOW`, and verifies the transfer completes.

This proves the architectural separation. Production P0/P1 queues still require the priority/soak tests defined in 10 — Testing & Reliability Plan.

## Runtime Storage Reserve

Bulk writes are admitted only if the projected remaining free space stays at or above the configured runtime reserve.

Reference policy remains the Storage specification default:

```text
runtime reserve = 2 GiB (configurable)
```

The spike implements and tests this admission rule as a pure capacity policy. Production code must obtain real filesystem free-space values and repeat the check at the appropriate commit/admission boundaries.

## Content Deduplication Boundary

When identical imported bytes produce the same SHA-256 and size, the Vault can reuse the existing immutable object rather than store a second physical copy.

This does **not** merge logical `MediaAsset` records. Multiple logical assets may reference the same content object while retaining separate project metadata.

## HTTP / Security Boundary

SPK-05 validates transfer behavior, not anonymous production access.

Production rules remain:

- protected objects require authenticated/authorized access according to 09 — Security Model;
- Companion download authority is scoped to its trusted role/project requirements;
- package download policy follows Software Repository rules;
- SHOW-mode policy can reject new downloads even if the caller is otherwise authorized;
- secrets never appear in URLs/object names/logs;
- the content hash is identity/integrity data, not an authorization token.

## Prototype Evidence

Repeatable validation completed in the available Go 1.23 environment:

- `go test ./...` passes;
- `go test -race ./internal/transfer ./internal/vault` passes;
- managed import uses a staging file and SHA-256 streaming before atomic promotion;
- importing identical bytes resolves to the same immutable object identity/path;
- a 16 MiB transfer is intentionally interrupted after 2 MiB, resumes using HTTP `Range`, verifies full SHA-256 and promotes the cache file;
- intentionally corrupt partial data produces checksum failure and never promotes the final file;
- an active 32 MiB transfer pauses after entering `SHOW` and resumes after returning to `EDIT`;
- while the bulk transfer is paused, the independent `/runtime/ping` request succeeds within the test latency bound;
- 2 GiB runtime-reserve admission logic accepts safe writes and rejects a write that would breach reserve;
- manual 256 MiB end-to-end import + HTTP transfer + verification succeeds;
- the built demo process measured about **8.5 MiB peak RSS** during that 256 MiB run in the validation environment, supporting the bounded-buffer design.

The 256 MiB measurement is environment-specific evidence, not a product memory SLA.

## Why Not Put Media in SQLite

SQLite remains the authoritative metadata/state database selected in SPK-01, but large show-media objects stay on the filesystem because:

- streaming/range delivery maps naturally to files;
- large BLOB writes would unnecessarily couple runtime DB behavior to media ingestion;
- content-addressed immutable files simplify checksum verification and cache distribution;
- backup/archive policy can treat DB state and large immutable objects differently.

SQLite can still reference object identity, size, logical asset/version and availability state transactionally.

## Acceptance Result

**ACCEPTED** for:

- filesystem Vault objects on local SSD/NVMe;
- SHA-256 content identity;
- staging + atomic promotion;
- content-addressed immutable object path;
- HTTP(S) byte-range delivery as first transfer mechanism;
- resumable `.part` cache contract;
- full checksum verification before final cache promotion/READY;
- bounded streaming buffers;
- dedicated SHOW-mode bulk-transfer gate;
- configurable runtime storage reserve admission;
- reusing the same transfer primitive for media/software/plugin package bytes.

**Still required during implementation/qualification:**

- SQLite metadata/manifests wired to the real Vault;
- Swift Companion cache implementation and Required Media manifest reconciliation;
- authenticated/scoped production object endpoints;
- actual filesystem free-space probing;
- bandwidth/concurrency limits and operator progress UI;
- 2 GiB interruption/resume acceptance test on selected Hub + Companion storage;
- external SSD/NAS backup/restore integration;
- filesystem behavior validation on final Hub deployment hardware.

## Next Spike

**SPK-06 — Hub Deployment on ARM64 / Mini-PC** should prove that the chosen Go/SQLite/Vault shape builds and runs on the intended Linux ARM64 Hub class, with local SSD/NVMe storage and stage-network behavior, before the project transitions from decision spikes into implementation slices.
