# SC-024 — Endpoint authorization inventory and Phase 3 scoped-read qualification

**Date:** 2026-10-17. **Status: DRAFT / NOT ACTIVE IN HUB.**
This records verified route families in StageCore's Go sources and implements *unwired* helpers with tests for future server-side Project list/get qualification. It **does not** hide Projects in the currently deployed Operator API, does not modify GO, and does not authorize downloads or device output. Changing only the Project selector without all direct routes would be a security vulnerability.

## Initial source audit (not exhaustive)

The route registry is distributed across many `internal/httpapi/*.go` files. Direct source inspection of **30 selected files** identified **127 method-pattern registrations**, excluding dynamic routes, imported extension handlers and routes registered through aliases or with separate calls. The counts below are therefore a *partial inventory*, not an assertion that these are the only endpoints.

| Path family | Source | Minimum planned capability / treatment |
| --- | --- | --- |
| `GET /api/v1/projects` | `operator_projects.go` | `project.view` per item, Owner sees all |
| `GET /api/v1/projects/{project_id}`, `.../dashboard` | `operator_projects.go` | Check exact project ID server-side before any details; 404 whether missing or unauthorized |
| `POST /api/v1/projects` | `operator_projects.go` | Owner-only Project creation policy (new resource has no existing membership) |
| `POST /api/v1/projects/{project_id}/duplicate` | `operator_projects.go` | Source Project `project.view` + `project.edit` (or explicit `project.duplicate`) and target creation authority; no grants copied |
| `/api/v1/projects/{project_id}/cues*`, `.../validation`, `.../publish` | `operator_cues_publish.go`, `operator_cue_reorder.go` | `project.view` or `cue.edit`; `snapshot.publish`; Cue ID must resolve to current Project/revision |
| `/api/v1/projects/{project_id}/runtime*` | `operator_runtime.go` | Scope live sessions/Snapshots; `rehearsal.control`, `show.go`, `show.enter_exit`; emergency blackout gets separately reviewed safety authority |
| `.../lighting-controller*`, `.../stagelaser-controller*` | `operator_lighting_*.go`, `operator_stagelaser_controller.go` | `cue.edit` / `lighting.manual` / `device.configure`; do not remove physical fail-safe |
| `.../tablet-controller*`, `.../visual-engine*` | `operator_tablet_*.go`, `operator_visual_engine.go` | `project.view`, `cue.edit`, or restricted runtime control, with assigned device identity |
| `.../stage-devices*`, `/api/v1/stage-devices/{device_id}*` | `operator_stage_devices.go` | Resolve device assignment to current Project and distinguish global inventory from per-Project control |
| `.../configuration*`, `.../inputs*`, `.../outputs*`, `.../routes*` | `operator_configuration*.go` | `project.view` / `project.edit`, cross-check subordinate IDs |
| `.../sessions*`, `.../notes*`, `.../preflight`, `.../timecode` | `operator_memory.go`, `operator_preflight.go`, `operator_timecode.go` | `project.view` and stronger edit capability when mutating, with server-resolved session/Note owners |
| `.../simulation*` | `operator_simulation*.go` | `project.view` / `rehearsal.control`, never treat simulation GO as actual SHOW authority |
| `.../show-capsules*`, `/api/v1/show-capsules*` | `operator_show_capsules.go` | `capsule.export`, snapshot/project source binding, prevent unrelated capsule download/import inspection |
| `GET /api/v1/vault/objects/{content_hash}` | `server.go` + `vault_routes.go` | **Must audit existing device/service authentication separately.** A content hash is not Project authorization. Media download needs an allowed media asset/content version and Project grant, without breaking authenticated device distribution. |
| `GET /api/v1/events` | `user_auth.go` | Review streamed payload types and subscription, revoke dynamically; do not expose other Project metadata |
| `/api/v1/security/*`, `/downloads/software/*` | `security_operations.go`, `server.go` | Platform/Owner authority is different from Project `project.view`; security and binary downloads have independent requirements |
| `/api/v1/system/audio/*` (stacked SC-021 Draft) | `operator_stageaudio.go` in #508 | File import/list + playback needs live Project ACL and Snapshot/Session SHOW safety fence |

## Phase 3 isolated qualification

Unwired primitives in `internal/httpapi/project_scope_read.go`:
- `scopedProjectList(ctx, access, store, userID)` maps DB-derived **current** authorized ProjectIDs to Project data; if access/store unavailable, fails closed. No Project names of other Shows returned.
- `withScopedProjectRead(...)` validates current browser session and checks `project.view` from live Project grants before a **ProjectID path read**. Unknown and unauthorized ProjectIDs return the **same 404** without a Project name or revision ID.
- Tests register these on **test-only** `/probe/projects*` routes; no real route changes, Hub command wiring or runtime control changes. Tests cover Owner/global visibility, Operator Project A only, Viewer unassigned, direct-ID probe denial, revoke during an existing session, disabled account denial and nil ACL/store safety.
- Additional `internal/projectaccess/resource_scope.go` read-only DB resolvers safely map **Cue→Revision→Project**, Revision→Project, Snapshot→Project and Session→Snapshot→Project. These never trust a request's alleged ProjectID as the child resource's owner. `CheckProjectResourceRead` requires the real owner to match the expected Project path, and checks the live `project.view` grant. Tests deliberately try to read Project B Cues, Revisions, Snapshots and Sessions beneath Project A routes, verifying denial. These are **NOT yet connected** to production endpoints or GO.

**Do not wire this middleware to the Hub in isolation.** To protect all Projects we need a centrally applied, atomic cutover that protects *all* resource paths, including IDs which do not carry a ProjectID in the path, dynamic module/extension routes and device/service credentials. Do not infer Project ownership solely from a user-provided path or a file hash.

## Future rollout and safety

1. Finish comprehensive generated endpoint policy map + static and dynamic route coverage checks.
2. Complete canonical resolver(s): **Cue/Revision/Snapshot/Session ownership is now qualified read-only in this Draft**, but device→assignment→Project; media asset/content version→Project; Show Capsule→source Project and other derived IDs still require coverage. Test foreign direct-ID access on every family before any production cutover.
3. Add Owner UI for prepared grants and show impact report comparing current all-Project roles with future assignments.
4. Perform **atomic** enforcement cutover, with a feature/rollback gate that cannot be partially enabled for Project selector alone; no implicit Operator membership, and no Project cross-leak through SSE/audio/exports. Recheck revocation during active subscription and GO dispatch.
5. Verify emergency blackout and stopping safety independent of ordinary GO authority; harden Pi 4 fallback and rollback, with tests in non-SHOW mode before merging or deployment.

Related Draft PRs: #518 (canonical data model), #520 (Owner grant API), and competing **#517** (incompatible migration 53; do not merge both). Related user requests: #511 (scoped permissions), #510 (Project Files), #502 (StageAudio).

**Zero changes to the production Pi 4.**
