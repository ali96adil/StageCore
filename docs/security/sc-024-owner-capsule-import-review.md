# SC-024 Phase 4D — Owner-only Show Capsule import review (read-only)

**2026-10-18 · Draft branch · NOT installed, no GO/SHOW operations.**

## Existing baseline

The legacy `GET /api/v1/show-capsules` and
`GET /api/v1/show-capsules/imports/{capsule_id}/plan` still accept the
globally scoped `project.read` browser role. This exposes imported Show
metadata beyond future per-Project grants.

The legacy `POST .../materialize` already requires
`backup.restore` + CSRF. `showcapsule.PlanImport` detects collisions and
active SHOW as blockers; `Materialize` retries the plan during import.
This PR does not alter or replace those routes.

## Optional read-only endpoints

`WithOwnerCapsuleImportReview(auth, planner, trustedCapsuleRoot)` is an
**unwired** Hub option. Unlike production ShowCapsuleService, the planner
interface exposes only `PlanImport`, not `Materialize`.

| Request | Owner result | Non-Owner result |
|---|---|---|
| GET `/api/v1/show-capsules/imports/review` | Verified import inventory, skipped count | 403 |
| GET `/api/v1/show-capsules/imports/{capsule_id}/review` | Fresh readiness plan | 403 |

Anonymous requests → 401, non-TLS/non-loopback requests → 426,
and invalid/missing IDs → 404. The effective Owner role is tested **in
addition** to the current `backup.restore` global capability; assigning
an Operator a Project grant never gives the Operator import visibility.
The API applies `Cache-Control:no-store` and always declares
`read_only:true`, `authorized_to_restore:false`. This response is not
a restore permit, approval nonce, transport credential or command.

Inventory uses the trusted Hub imports root, skips invalid/unverified
Capsules with an explicit skipped count, never scans exports. Both routes:

1. Require a canonical UUID directory entry matching the manifest's
   verified `capsule_id`.
2. `Lstat` the imports root and child before entering; reject symlinks.
3. Walk each capsule tree and reject symlinks, sockets, FIFOs, devices,
   nonregular files, oversized metadata, more than 4,096 entries or
   more than 8 GiB of bytes.
4. `showcapsule.Verify` the canonical manifest+checksum+included
   content objects; the single review additionally runs the existing
   authoritative `PlanImport` against this Hub's current DB/Vault.
5. Refuse to list more than 128 top-level entries at all rather than
   silently present an incomplete inventory.

A readonly review may inspect a capsule during SHOW, but
`PlanImport` MUST report `runtime.active_show` and
`materialization_ready:false`; no restore button is provided and no
StageCore runtime state is mutated.

## Threat model and **rollout blockers**

These are conservative checks, NOT a complete untrusted filesystem
sandbox. `Lstat`, `WalkDir`, `Verify` and `PlanImport` operate by
paths, leaving filesystem mutation races (TOCTOU). The imports root
**must not be writable by an untrusted user/process**. Before a live
Owner-facing import, implement fd-rooted no-symlink traversal (e.g.
Linux openat2 RESOLVE_BENEATH/NO_SYMLINKS), resource/time budgets,
file content provenance and signature/trust policy, and expose only
audited error envelopes.

**The new code never authorizes materialization.** A later explicit,
CSRF-protected Owner POST must revalidate Owner role, current trusted
Capsule identity, hashes, SHOW inactivity, readiness/identity collision,
audit events, backup+rollback and transaction safety at the irreversible
write boundary. Existing `/imports/{id}/materialize` stays unchanged
until a reviewed atomic security cutover; adding this opt-in route alone
does NOT secure old global import listing or preview.

Do not enable the feature for ordinary users or claim Project privacy
until all previously inventoried Project, Vault, event, Capsule and device
routes use the same tested authority model.

CI success is *not* signoff for physical show usage. No Pi update or
`main` merge occurs in this Draft.
