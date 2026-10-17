# SC-024 Phase 3D — Project-bound Vault download and SSE boundary

**2026-10-18 | Draft qualification only | NO production enforcement or Pi change**

Stack: #518 → #520 → #521 → #522 → #523 → this phase.
#517/#519 are competing migration-53 alternatives; do not merge them
alongside the canonical #518 data model.

## Verified current behavior

- `GET /api/v1/vault/objects/{content_hash}` is registered by `New` in
  `internal/httpapi/server.go`, and implemented by
  `internal/httpapi/vault_routes.go`.
- That path requires secure transport plus an authenticated
  `StageCoreSession` Companion runtime token. It then opens the Vault object
  by raw content hash. It does **not** receive a Project ID, Media Asset ID or
  Media Version ID; therefore the token+hash path cannot be described as
  Project-scoped media ACL today. This is a *device distribution* surface,
  not an authenticated browser user download contract.
- Deduplicated `vault_objects` may have many Project-owned Media Versions.
  A hash alone does not determine which Show owns a reference.

## Qualification-only primitive

`CheckScopedVaultObjectRead(ctx, userID, expectedProjectID, mediaVersionID,
contentHash)` (unwired) checks all of:
1. Valid lowercase 64-hex SHA-256 identifier;
2. The specific Media Content Version exists and references a registered
   immutable Vault Object;
3. That Version belongs to a Media Asset whose actual Project ID equals the
   expected Project ID;
4. The stored SHA-256 matches the requested hash exactly;
5. The caller currently has `media.read`, including `project.view`.

It returns `ErrDenied` for nonexistent or foreign media identities, bad
hashes and revoked grants, rather than confirming existence. It is not
authentication, not an HTTP handler, and not a complete download fence.

Test-only fixtures verify two Projects referencing the **same Vault blob**:
an Operator assigned to Project A may qualify through Project A's Media
Version but not Project B's Media Version with the same hash. They also cover
bad/stale version IDs, mismatch, read-only Project grant, Owner, revocation,
disabled user, unreferenced blobs and missing access service.

## Separate authority tracks needed before a production cutover

**Browser:** design a new authenticated Project media version download route,
resolved through immutable Project + Media Version identity, with live
`media.read`, range/read streaming, quotas, content verification, and no
cross-project metadata leakage. Never expose an unrestricted hash-only
browser route.

**Companion / Tablet / device distribution:** separately establish
machine-role/Stage Device assignment, current Runtime Snapshot/Session,
required manifest media content references, transport identity and live
revocation. Existing device runtime tokens are not ordinary Owner/Operator
browser sessions. Do not inject `userID` or caller-supplied `projectID` into
the current hash-only device route and pretend it enforces these rights.

**SSE:** `GET /api/v1/events` in `internal/httpapi/user_auth.go` currently
emits a `session.ready` payload (user+Hub identity) and periodic keepalives;
it does *not currently broadcast Project Cue/media or device events through
that function*. Browser session validity is rechecked every 250ms.
Future Project-specific events must bind an event to its trusted owning
Project before delivery, restrict each subscriber to live `project.view`
(or stronger relevant permission), recheck after grant revoke/account
disable, handle reconnect/replay/Last-Event-ID, and prevent metadata leakage
via event names, counts, envelopes and subscription URLs. Avoid treating
current absence of Project SSE payloads as proof that all future websocket/
module streams are protected.

## Invariants and rollout blockers

- No wiring in existing HTTP routes, no Companion protocol change, no GO
  command, no DMX/laser/tablet actuation, no database migration here.
- StageAudio, Project Files, Show Capsule, module and service-auth direct
  downloads, device access, current GO/rehearsal operator controls remain
  subject to the *existing* global permission model.
- Complete all endpoint/event paths, safe SHOW authority and emergency
  blackout semantics, Owner grant impact/migration preview, full CI and
  attended non-SHOW Pi tests before **atomic** enforcement switch.
