# SC-024 — Resolved dynamic HTTP registration audit (17/17)

**Review date:** 2026-10-18. **Status:** checked against source,
not authorized for production cutover.

Source inventory on integration #530 found 262 `HandleFunc`/`Handle` call sites,
including **17 non-literal registered expressions**. This document
resolves *all 17* to the path constructed in its declaring source.
Names within braces are Go ServeMux path variables, not user grants.
The missing literal `GET /project-files.js` was added in the integration
review; the updated 262-registration SHA-256 is
`37853db9ba6cd95c5ee9ab7ea6dd2858236cd0cb699672fa73a8850fed0f82c0`.
It remains one of the static registrations, not an extra dynamic route.

## Resolved routes

Abbreviations (expanded by exact code in each declaration):

`P=/api/v1/projects/{project_id}`

`E=P/revisions/{revision_id}/execution-environments`

`I=E/{execution_environment_id}`

`A=I/assets/{asset_key}`

| # | Actual method and route | File | Legacy browser authorization | Required Project ACL at cutover |
|---|---|---|---|---|
| 1 | GET `A/vault-status` | operator_execution_environment_capture.go | global project.read | Actual Project, Revision, Environment and Vault object; `media.read` |
| 2 | POST `A/capture` | same | global project.edit | Actual Project, Revision, Environment and asset; separate explicit upload/capture grant |
| 3 | POST `I/operations` | operator_execution_environment_operations.go | global runtime.control | Project authority + trusted Machine Role/operation, SHOW lock and device safety |
| 4 | GET `I/rebuild-plan` | operator_execution_environment_rebuild_plan.go | global project.read | Scoped Project/Revision/Environment |
| 5 | POST `I/rebuild-plan/seed` | same | global project.edit | Scoped `project.edit`, object and Snapshot authority |
| 6 | PUT `I/rebuild-plan` | same | global project.edit | Same as #5; stale revision fence |
| 7 | DELETE `I/rebuild-plan` | same | global project.edit | Same as #5; protect published/rehearsal/SHOW state |
| 8 | GET `I/snapshots` | operator_execution_environment_snapshot_diff.go | global project.read | Scoped Snapshot list filtered by actual Revision |
| 9 | GET `I/snapshots/{snapshot_id}` | same | global project.read | Resolve child Snapshot by DB identity before response |
| 10 | GET `I/snapshots/diff` | same | global project.read | Resolve both compared Snapshot IDs and reject foreign/unknown |
| 11 | GET `E` | operator_execution_environments.go | global project.read | `project.view` + actual Revision relationship |
| 12 | POST `E` | same | global project.edit | `project.edit` and Draft revision check |
| 13 | PUT `I` | same | global project.edit | Actual environment Project ownership; Draft-only |
| 14 | PUT `I/machine-role` | same | global project.edit | Authoritative Machine Role → Project match |
| 15 | DELETE `I` | same | global project.edit | No orphaned relationships; respect Snapshot/SHOW |
| 16 | GET `P/access/users/{user_id}` | operator_project_access.go | global user.manage (Owner only) | Owner-only grant read of explicitly requested Project/user |
| 17 | PUT `P/access/users/{user_id}` | same | global user.manage (Owner only) + CSRF | Owner-only exact replacement + durable audit, never allow self escalation |

`P`, `E`, `I` and `A` are documentation abbreviations only.
The original `HandleFunc` declarations use literal prefixes combined
with constants/local strings. Go's actual router receives the expanded
patterns.

All 17 are **registered** by source but current user-role checks are
**global**, not yet canonical per-Project membership enforcement.
A Project path `{project_id}` alone does not prove a Cue, Revision,
Snapshot, Machine Role, Vault object, or Execution Environment ID's
ownership. Replacing middleware without direct-ID negative tests would
leave cross-Project attack paths.

## Automated review

`TestSC024AllDynamicRegistrationExpressionsAreReviewed` checks that all
17 expressions are specifically accounted for, without a wildcard/default.
`TestSC024DynamicProjectRouteFamiliesHaveExplicitLegacyAuthBoundary`
checks corresponding legacy authorization markers, so a change forces a
security review rather than silently asserting per-Project enforcement.

The AST registration SHA test in #529 separately covers all static patterns.
It **must** change when the missing `GET /project-files.js` was registered
in the Operator web server. The new
`TestProjectFilesAssetIsActuallyServedByOfflineOperatorHub` exercises the
actual embedded-asset route and its TLS/loopback transport checks.

## Remaining gates

The full route-security matrix has many more than these 17 expressions.
Potential dynamic extension/plugin routes outside the HttpAPI package,
headers, nested IDs, software/Companion device authority, future SSE
Project events, OSC input and physical emergency STOP must be reviewed
separately. The current AST test is **not** evidence of global ACL
enforcement. No active SHOW credentials, device commands or deployments
change in this integration review.
