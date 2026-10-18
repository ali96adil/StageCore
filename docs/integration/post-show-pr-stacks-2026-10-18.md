# StageCore post-show PR integration map — 2026-10-18

**Planning document only. No merge, rebase, deployment, Pi action or
authorization cutover is performed by recording this map.**

## Canonical, linear SC-024 Project-security stack

These are *stacked diffs*, not independent `main` PRs. Do **not** merge
them directly to `main` out of order or cherry-pick randomly.

| Order | PR | Base | Content | State |
|---|---|---|---|---|
| 1 | #518 | main | Canonical schema-53 membership/grant foundation | Draft, no enforcement |
| 2 | #520 | #518 branch | Owner-only grant administration + atomic audit | Draft, no cutover |
| 3 | #521 | #520 branch | Test-only scoped Project reads + route inventory | Draft |
| 4 | #522 | #521 branch | Media Asset/Version/Location read qualifiers | Draft |
| 5 | #523 | #522 branch | Active V2 Stage Device read qualifiers | Draft |
| 6 | #524 | #523 branch | Vault media-version/hash read qualifier + SSE audit | Draft |
| 7 | #525 | #524 branch | Virtual Project Files + local Capsule export qualifier | Draft |
| 8 | #526 | #525 branch | Opt-in Files read-only UI + scoped browser API | Draft |
| 9 | #527 | #526 branch | Opt-in scoped verified local Capsule exports inventory | Draft |
| 10 | #528 | #527 branch | Owner-only verified Capsule import inventory and read-only plan | Draft |
| 11 | #529 | #528 branch | AST route registration drift CI gate and atomic ACL cutover matrix | Draft |

**Conflict:** #517 and #519 are **alternative** Project grants foundations
that compete with canonical #518 and use an incompatible Goose migration 53.
Do not combine these alternatives; explicitly close/supersede only after
the canonical design is reviewed and selected by the Owner.


## Unified integration preview

**[PR #530](https://github.com/ali96adil/StageCore/pull/530)** is one **DRAFT** review target from current `main`, collecting the entire canonical security stack without merging its individual Draft PRs. Its base was compared to `main` on 2026-10-18 (`behind_by=0` at that check). It remains NOT READY TO MERGE because global Project ACL enforcement, complete direct-ID probe tests, emergency STOP and migration preflight are missing.

Integration-specific audit: **262 HTTP registrations, 17 dynamic**; reviewed source digest `37853db9ba6cd95c5ee9ab7ea6dd2858236cd0cb699672fa73a8850fed0f82c0`. Added an HTTP route for embedded `/project-files.js` absent in #526 and an actual Hub asset response test. See `docs/security/sc-024-dynamic-route-audit-17.md`.

The previous #529 Go 1.26 `-race` job exceeded Go's 10-minute package deadline while SQLite fixtures were running, with no `DATA RACE` report. Integration PR #530 runs the **same entire test matrix** with bounded race package parallelism and a 15-minute per-package test budget. This CI-level change is under review, not permission to ignore hangs; a second timeout remains blocking.

## Other open post-show feature families are separate integration lanes

- #506 → #507 → #508: StageAudio local two-deck engine + wired/Bluetooth
  output + Operator UI. Do not confuse Audio MVP with safe live cue authority.
- #514 → #516: DMXking ultraDMX Micro USB protocol and FTDI transport.
- #512: Project rename, must preserve immutable Project ID and Vault refs.
- #500: Cue Move-to-position / drag-and-drop.
- #503: V2 lighting configuration authoring.
- #504 → #505: Offline Hub Date/Time visibility and safe preview.
- #515: cross-feature integration **Draft**. Never use as a production
  merge merely because individual feature PRs have green CI.
- Legacy overlapping-GO/pre-show PRs (#456, #475, #476, #485) need a
  separate conflict and release-baseline comparison, not blind merging.

## Recommended consolidation after review

1. Freeze new overlapping ACL schema variants. Preserve a **single** chosen
   migration chain with unique Goose numbering.
2. Review all eleven incremental scoped-security PRs as one logical security
   programme: each has local tests, but only the final, system-wide route
   matrix can prevent cross-route bypass. Verify parent/head SHAs and all
   CI jobs in the selected integration commit.
3. Prefer a **fresh reviewed integration branch** from latest `main`,
   with the canonical stack merged/squashed in order and full test suite;
   if `main` moved since stack origin, resolve conflicts deliberately,
   rather than merging Drafts automatically. Do not close provenance PRs
   until integration is fully reviewed.
4. Keep Owner membership migration dry-run and operator role fallback
   explicit, including rollback and real SHOW preflight validation. A
   permitted `project.view` is not `show.go`, emergency STOP or
   `backup.restore`.
5. Do **not** activate the Project ACL behind only Files/Capsule endpoints:
   `/api/v1/projects`, Cue/runtime IDs, Stage Device metadata,
   raw-hash Companion Vault, StageAudio, Software downloads,
   Show Capsule global listing/plan, SSE and any direct-ID route must
   undergo the *same* atomic policy transition.
6. After a reviewed CI-clean integration, test on a non-SHOW test Hub with
   backup/restore verification and real authenticated accounts. Only then
   schedule attended Pi/hardware qualification. Keep the installed
   show-tested runtime untouched until explicit Owner release approval.

**No merge has been performed by this plan; this is not a release checklist
signoff, and green CI does not prove physical theater/show safety.**
