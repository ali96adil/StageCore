# 2026-11-14 — Autonomous software completion campaign handoff

## Repository and Git checkpoint

- Repository: `ali96adil/StageCore`
- Branch / PR: `integration/stagecore-rc` / Draft PR #534
- Last remotely verified branch and PR head before this batch:
  `701cd7b5f7111153a98284734b8e1021249f4d18`
- Local functional commit: `c6d4407` — performance benchmarks and single-pass
  linked-Cue graph validation.
- Local qualification/documentation commit: `b7d48bc` — SC completion matrix,
  refreshed related-component heads and performance report.
- Handoff commit: `814cf84` — checkpoint record and index update. It was pushed
  normally to `integration/stagecore-rc`; local and remote HEAD matched at
  `814cf84b38a6b33379cfcd79270a5ba414e3d9d2` before the follow-on doc edits now
  in the worktree.
- Local SC-015 implementation commit: `af3e1ba` — explicit delivery-evidence
  classification in the real Session trace, Hub dispatch-attempt count,
  operator correlation/duration/evidence labels, translation and regressions.
  It is pushed in the current PR head.
- Latest pushed head: `c67423461a8e32a76fff849b5dfdedd0b99c5543`.
- Follow-on commits pending push: `0ef6f84` fixes the SC-015 Node delivery
  harness to include `cueTraceCard`; `4352f67` adds SC-012 client-side Cue
  search and filters with localized UI, per-Project saved view and tests.
- Local HEAD at this checkpoint: `4352f67`; remote remains `c674234`.
- PR #534 remains open/draft. No branch or PR was created; no force push or
  merge was performed.

## CI and linked repositories

At remote HEAD `c674234`, Core run `43533953848` passed Go 1.27, isolated
tests/ARM64, Companion replacement/media and Swift CompanionCore. Go 1.26
failed in `Check Operator JS syntax` when the Node delivery test loaded
`renderSessionDetail` without `cueTraceCard`; test harness fix `0ef6f84` passes
locally. The run is complete. Push the pending commits normally to #534, then
wait for exact-head CI before another push.

Related StageCore issue PRs already cover SC-003 (#503, run `39030208327`),
SC-002A (#504/#505, runs `39030641841` / `39033942474`) and the stacked
StageAudio feature (#506–#508, runs `39035955572`, `39037378296`,
`39038867865`). Their Go matrix checks pass and the PRs remain drafts. Do not
duplicate their implementation on #534; physical Pi qualification is still
separate.

Last reviewed linked PRs and passing software checks remain the ones in
[`related-component-qualification-matrix.md`](../architecture/related-component-qualification-matrix.md):
TabletPlayer #36 (`91613da`), ESP32 DMX Lighting #39 (`3ad7749`), ESP32 Camera
#20 (`455b066`), StageLaser #56 (`b14a578`) and #57 (`c4ff10b`). Do not merge
or flash any of these; StageLaser still has an independent beam-off hardware
blocker. The StageAudio issue stack #506–#508 has software source in StageCore,
but target Pi audio output is not qualified.

## Completed in this batch

- Added tests for Cue group graph validation: valid nested chain, missing or
  disabled linked Cue, multiple parents and cycle.
- Replaced repeated full-manifest `resolveCueGroup` validation from
  `nestedCueIDs` with one Cue index and one DFS. Snapshot validation remains
  fail-closed; no cache was added.
- Added simulation-only benchmarks for flat Mixed Cue sizes 50/100/200 Actions,
  nested linked chains 25/50, 50/100 and 100/200 Cue/Action, scheduled linked
  starts, 100-Cue/200-Action Rehearsal sessions, and STOP/managed Emergency
  Blackout with 1/8/32 delayed GO requests. A guarded physical executor stays
  at zero.
- Added `docs/architecture/software-completion-matrix.md` for SC-001..SC-025,
  with code/test evidence, remaining acceptance, affected components and
  hardware requirements. Review found an existing green implementation for
  SC-003 in PR #503 and existing #504/#505/#506–#508 stacks; do not repeat it.
- Updated the performance and integration-qualification reports with exact
  remote heads/checks and the present Pi access boundary.

## Performance evidence

Apple M3 Max, Go 1.27.1, one CPU, temporary SQLite, no HTTP/ACL/device latency
inside Cue Engine measurements. The 100-Cue/200-Action linked benchmark
completed in P50/P95/P99 `78.70/82.30/84.51 ms`, with P95 latency jitter
`3.597 ms`; the scheduled 25/50 case had schedule jitter P50/P95/P99
`0.781/1.604/5.477 ms`. STOP/Blackout and sustained rehearsal details are in
[`cue-execution-performance-qualification.md`](../architecture/cue-execution-performance-qualification.md).

The 100-Cue/200-Action, 100-GO sustained Rehearsal benchmark improved from
`414.01 ms` to `305.29 ms` session mean after the measured graph-validation
change. Per-session allocation fell `217,224,530` → `39,004,484` bytes;
GCs fell `84.9` → `15.05`; per-GO P95/P99 became `4.176/4.795 ms`. The
post-fix profile still shows SQLite syscalls and `GetRuntimeSnapshot` as major
costs. The benchmark command's process-tree peak RSS was about `214.6 MiB`,
including migrations and test setup; this is not Pi or Hub-only memory.

## Tests and checks

Passed on the local macOS arm64 host:

- `go test ./internal/cueengine ./internal/runtimecontrol -count=1`
- `go test -race ./internal/cueengine ./internal/runtimecontrol -count=1`
- `go vet ./internal/cueengine ./internal/runtimecontrol`
- `git diff --check`
- `TestReference100Cue200ActionLinkedChainRunsOnlyInSimulation`
- `TestSustainedReferenceRehearsalRuns100Cues200Actions`
- Mixed/linked/scheduled scale and delayed GO safety benchmarks (logs under
  `/private/tmp/stagecore-*-perf-2026-11-14.log`).

## Open work and boundaries

1. Local commits `0ef6f84` and `4352f67` are ahead of remote `c674234`; this
   handoff/matrix correction is also local. Core run on `c674234` is complete
   with one Go 1.26 Node harness failure now fixed locally. Review and commit
   these docs, then push normally to the same PR #534 and wait for CI on its
   exact new head.
2. SC-015's session/Cue/Action evidence now makes
   `COMPLETED + TRANSPORT_ONLY` report external execution as unknown. Delay,
   timeout, duplicate-after-reconnect, offline-reconnect and signed application
   ACK simulations already exist in Cue/Companion channel tests; don't copy
   them. Remaining report work: connection/warning/snapshot/safety timeline
   and safe export/compare. SC-001 still lacks a first-party Ableton execution
   provider/observation contract.
3. Continue SC-002 beyond the existing #504/#505 status/preview stack. Do not
   duplicate their work or add a host clock write without isolated qualification.
4. Raspberry Pi 4 performance qualification remains open. The explicit
   read-only SSH attempt to `stagecore-qualification` failed during handshake
   before host identity was established. Do not run commands via the ambiguous
   Pi alias. Use a verified isolated Pi lab when access is restored.
5. Do not access production Hub/Camera Relay/data/Vault, send to real stage
   devices, flash firmware, merge to `main`, force-push, or enable production
   Project ACL/SC-024. Simulation and temporary databases only.

## SC-015 and SC-012 follow-on

The follow-on worktree changes (not covered by the completed CI run on `814cf84`)
add delivery evidence derived only from the explicit ACK level. A Hub Action
with `COMPLETED` plus `TRANSPORT_ONLY` reports `EXTERNAL_EXECUTION_UNKNOWN`;
no downstream execution is inferred. Session Memory carries recorded dispatch
attempts and shows Cue duration, correlation and a localized evidence label. A
regression test covers completed CoreMIDI transport with unknown external
execution, and the embedded UI contract requires that distinction. Local
SC-012 commit `4352f67` adds per-Project search state, nested chain/action/target
search and client-only multi-filters for Mixed/action type, enabled state and
linked/group membership. The filter helper Node tests prove chained Mixed
selection, combined filters, project isolation, malformed storage fallback
and preserved sequence order. Full 100+ Cue browser acceptance remains open.

Focused checks passed before local commits `0ef6f84` and `4352f67`:
`node --test tests/cue_delivery.test.js tests/cue_list_filters.test.js`,
`node --check internal/operatorweb/static/app.js`,
`go test ./internal/operatorweb -run '^(TestCueListSearchAndFiltersStayViewOnlyAndProjectScoped|TestCueStudioTypeFirstAuthoringAndLegacyRoundTrip)$' -count=1`,
`go test ./internal/sessionmemory -count=1`, the two
Session Memory/embedded Operator HTTP tests, their Race runs,
`go test -race ./internal/operatorweb -count=1`, `go vet` for all three
packages, `node --check internal/operatorweb/static/memory.js` and
`git diff --check`. The full `go test -race ./internal/httpapi -count=1`
exceeded the package's 10-minute timeout in the unrelated
`TestStageDeviceSetupAPSuppressesACKAfterOwnerRevocation` Argon2 login test;
the focused Session Memory/Operator HTTP tests pass under Race. SC-012 focused
Operator/web and HTTP tests, Race contract test, `go vet`, JS syntax and
`git diff --check` also pass. Both follow-ons still need exact-head Core CI.

## Software/hardware/release state

This work adds software performance evidence and a bounded Cue Engine
optimization. It does **not** establish Software Complete. Pi ARM64 execution,
physical device integration/ACK/Blackout, and supervised acceptance remain
`Hardware Qualification Pending`; production rollout and SC-024 enablement
remain `Production Release Approval Pending`.

## Resume update — 2026-11-14 00:49 UTC

This section supersedes the older HEAD/CI pointers above for the current
resume. Earlier entries are retained as history.

- Branch: `integration/stagecore-rc`; Draft PR #534 remains open.
- Last pushed Local/Remote HEAD: `558a6adf2765c79780ab290efb2e423bfd8ae5ee`.
- Local HEAD before the checkpoint edit: `8b96dfb2776af1db0a05be3e6a304a7ad8fabef6`; four
  functional commits are ahead and are not pushed. This checkpoint edit is
  the fifth commit; read `git rev-parse HEAD` for its resulting tip. No pre-existing uncommitted user
  changes were present at resume.
- Local commits: `864071c` safe Session Memory event timeline,
  `a52f4d8` same-Project completed Session comparison, `1007fa9` synthetic
  120-Cue/240-Action linked-list filter test, and `8b96dfb` safe warning
  categories. This handoff/matrix update is the fifth local commit.
- Current exact pushed-head CI: run `43535359362` for Go. Go 1.27, isolated
  tests/ARM64, Companion replacement/media, and Swift CompanionCore pass.
  Go 1.26 JS/firmware/Test/Browser/Vet steps pass; its full Race step is still
  in progress and ARM64 build has not started. **Do not push until the exact
  run has completed.** This run does not include the four functional local commits.
- Local verification after the Session Memory changes: focused Go tests in
  `internal/sessionmemory` and `internal/operatorweb` pass; the authenticated
  Session Memory HTTP API regression passes; `go vet` passes for
  `sessionmemory`, `operatorweb`, and `httpapi`; `node --check` passes for the
  changed UI/localization assets; the full Core Operator Node suite passes
  (34 tests), and the 120-Cue/240-Action test passes in about 23 ms on this
  Mac. Focused Race checks pass for `sessionmemory`, `operatorweb`, and the
  authenticated Session Memory HTTP API.
- SC-015 now projects only allowlisted session lifecycle, safety, device
  command, and simulation events. Start warnings expose safe categories and a
  count, not raw device/error text. Category filtering and sanitized JSON
  export are implemented. Comparison reports summary counts, duration and
  Snapshot match; it is not action-by-action. The current event store has no
  per-output connection/health event stream, so Session Memory does not infer
  connectivity from a sent command.
- SC-012 search/filter code is already pushed at `558a6ad`; local regression
  adds 120 linked Mixed Cues and 240 Actions. Real-browser keyboard/visual/
  touch and RTL/LTR acceptance remains open.
- The previous isolated Pi SSH attempt failed before host identity was
  verified. No Pi performance qualification, real endpoint commands,
  production services/data/Vault access, firmware flash, force push, merge, or
  production ACL enablement occurred.
- First task after this checkpoint: wait for run `43535359362` to finish. If
  green, push these five commits normally to the existing branch/PR and wait
  for the exact new head CI. Then investigate SC-001's missing Ableton-specific
  execution/observation contract using simulated endpoints; do not equate MIDI
  transport acceptance with Live execution. Continue to label Pi and physical
  endpoint acceptance as hardware qualification.

## Resume update — 2026-11-14 01:00 UTC

This is the latest checkpoint; it supersedes the previous resume action.

- Remote remains `558a6adf2765c79780ab290efb2e423bfd8ae5ee`; Draft PR #534 is
  open on `integration/stagecore-rc`.
- Exact remote Core run `43535359362` is now fully green: Go 1.26 (including
  Race and ARM64 builds), Go 1.27, isolated tests/ARM64, macOS Companion
  replacement/media and Swift CompanionCore. This run only tested `558a6ad`.
- Local HEAD before this checkpoint update is
  `84cd293177a65029957bf7a85174a9621d63f20f`; six commits are ahead before the
  docs commit below. Functional commits are `864071c`, `a52f4d8`, `1007fa9`,
  `8b96dfb`, and `84cd293`; `0f4164a` is the earlier matrix/handoff update.
  This resume/matrix update is the seventh commit. No force push, branch
  creation or merge occurred.
- SC-015 local code adds a safe event timeline, warning categories/counts,
  category filters, sanitized JSON export and summary comparison. Current
  event records still lack per-output connection/health events, so those are
  not inferred.
- SC-012 has a regression for 120 linked Mixed Cues / 240 Actions with nested
  search and stable sequence ordering. It is synthetic Node coverage, not a
  real-browser visual/keyboard/touch qualification.
- SC-013 Cue Inspector now states that linked Cues launch concurrently with
  independent delays measured from GO; it explains action Sequential,
  Parallel, Barrier, delay, timeout, required application ACK and the limited
  pre-dispatch retry. Planning remains read-only: no publish or dispatch.
- SC-001 already has the opt-in Max for Live / signed receiver implementation
  and mock integration. The receiver-ACK Lab passed all 73 Node tests locally;
  the remote Go 1.26 Node step also passed. Installed Live/Max acceptance and
  diagnosis of the copied production misses remain pending. Do not duplicate
  the existing receiver/gateway work.
- Local checks: the combined Core Operator plus receiver-ACK Lab Node suite
  passed 107 tests; Cue Inspector Node tests (8), Cue Inspector contract Go
  test, Session Memory/API focused tests, focused Race tests and `go vet`
  passed. No local full `go test ./...` was run in this follow-on.
- Related PRs rechecked: TabletPlayer #36 `91613da` (Android PASS); ESP32 DMX
  #39 `3ad7749` (six firmware jobs PASS); ESP32 Camera #20 `455b066`
  (`hardware-probe` PASS); StageLaser #56 `b14a578` (host/build PASS) and #57
  `c4ff10b` (host/build PASS, open and not draft). StageCore #503–#508 and
  #514–#516 are open at the matrix heads with their Go checks passing; no PR
  was merged or modified. Physical device qualification remains separate.
- The previous remote CI is complete, so the next action is to commit this
  documentation checkpoint and push the seven local commits normally to the
  same branch/PR. Then wait for exact-head CI before another push. After that,
  investigate SC-002 offline-clock software gaps beyond the existing #504/#505
  status/preview stack without writing host time or touching production.
- No Pi qualification became available. Production services/data/Vault remain
  untouched; no device commands, firmware flash, merge, force push or
  production Project ACL enablement occurred.

## Resume update — 2026-11-14 01:16 UTC

This checkpoint records the local SC-013 follow-on. The remote CI state below
is for the last pushed HEAD and does not include these local edits.

- Branch: `integration/stagecore-rc`; last pushed Remote HEAD remains
  `f64cde026a0fe890eb39e0b0fee89eda7af84d9d` on Draft PR #534. Local commit
  `e5fd6f1` contains the SC-013 Cue Inspector feature and tests. Uncommitted
  work also contains the SC-020 SQLite restart regression and updated matrix
  and checkpoint. Preserve all of it.
- Exact pushed-head CI: Go 1.27, isolated tests/ARM64, real macOS Companion
  replacement/media and Swift CompanionCore pass. Go 1.26 unit tests, Browser
  Cue Delivery and Vet pass; its Race tests are still in progress. **Wait for
  run `43538108851` to finish before pushing.**
- SC-013 now has a local-only Draft timeline for linked Mixed Cues. It models
  independent GO-relative Cue delays, per-Action delay before dispatch,
  SEQUENTIAL completion, PARALLEL fan-out and barrier joins. It includes
  authored `fade_ms` and timeout values, virtual offline target, missing
  Companion for known MACHINE_ROLE targets, missing DMX for lighting actions,
  and fixed assumed latency. Unknown target metadata is labeled unverified.
  Fail-soft errors continue; strict `FAIL_CUE` and fatal pending failures stop
  later work at the engine's corresponding sequential/barrier boundary. The
  UI says explicitly that this does not prove dispatch, application ACK or
  physically observed output. The planner contains no API call and sends no
  device command.
- Local tests: `node --test tests/lighting_preview.test.js
  tests/cue_list_filters.test.js` passed 16 tests, including linked Mixed Cue
  timing, target fault reasons, timeout, fail-soft/strict policy and large
  120-Cue/240-Action filtering. The Cue Studio Go contract passed and
  `go vet ./internal/operatorweb ./internal/cueengine` passed. `node --check`
  and `git diff --check` passed. No browser-based accessibility/RTL/LTR
  acceptance is claimed.
- Remaining SC-013 work: fixed latency is an explicit hypothetical, not a
  measured distribution; the no-latency schedule is a lower bound because
  capability completion time is unknown; fade planning currently recognizes
  `fade_ms`; fresh target metadata and real-browser interaction still need
  qualification.
- No Pi identity was established. Existing Mac performance measurements are
  recorded in the qualification report and were not repeated. Do not infer Pi
  performance from ARM64 cross-builds. No production service/data/Vault,
  physical output, firmware flash, force push, merge, or production ACL change
  occurred.
- Next: wait for the exact-head Go 1.26 Race job. If the entire pushed-head CI
  is green, commit the tested docs and SC-020 regression, push normally to the
  same branch/PR, then follow exact-head CI. Continue with SC-002 only
  beyond the existing #504/#505 status/preview work and with SC-024 isolated
  tests; keep production clock writes and Project ACL activation out of scope.

## Resume update — 2026-11-14 01:20 UTC

- Local commit `e5fd6f1dd1feb3c6b73017d2e62b112c04ad9f39` is the tested SC-013
  Draft path simulation; it is not pushed yet. Matrix/handoff edits and the
  SC-020 restart test remain uncommitted. Remote HEAD is still `f64cde0`.
- The SC-020 test `TestTimingCapturePauseSurvivesRestartClockJumpAndKeepsGOAvailable`
  closes and reopens the isolated SQLite DB while an active rehearsal is
  paused. It verifies paused state survives, duplicate Pause is idempotent,
  a simulated backward clock correction does not change event-order state, the first
  post-Resume observation reanchors, and the next positive 2-second interval
  is the only learned transition. Store records are retained; there is no
  OS/system clock write. Full `go test ./internal/timingintelligence`, focused
  Race, Vet and `git diff --check` passed.
- A separate Runtime Control/Cue Engine regression completes a simulated GO
  while capture is paused and verifies the raw timing event remains visible
  but is excluded from learning. The authenticated timing API test verifies
  Owner + CSRF can pause and rejects Viewer or missing-CSRF writes. Focused
  normal/Race tests and `go vet ./internal/runtimecontrol ./internal/httpapi`
  pass.
- Initial test assertion incorrectly expected the first post-Resume Cue to be
  trainable; it correctly remains excluded for reanchoring. No production code
  defect was found. Real-browser interaction and Pi clock qualification remain
  open.
- Exact remote run `43538108851` is still waiting on Go 1.26 Race; all other
  jobs previously listed for `f64cde0` passed. Do not push until this run is
  complete.
- Next: finish the Go 1.26 Race check; if green, commit the pending SC-020
  Store/Runtime Control/HTTP regressions and qualification docs, push the local
  commit group to the existing branch, and monitor exact-head CI. Continue
  SC-002 beyond #504/#505 with browser/Pi qualification still open.

## Resume update — 2026-11-14 01:25 UTC

This section supersedes the 01:20 local-work pointers above.

- Branch: `integration/stagecore-rc`; pushed Remote HEAD remains
  `f64cde026a0fe890eb39e0b0fee89eda7af84d9d`, Draft PR #534. Local HEAD is
  `68a64c967f22c23cb33a5b6dbec409ee38c146fe`, two commits ahead:
  `e5fd6f1` local SC-013 path simulation and `68a64c9` SC-020 pause/lifecycle
  regressions. Matrix and handoff documentation are the only uncommitted
  changes. No pre-existing user edits were overwritten.
- Core CI run `43538108851` is still active at Go 1.26 Race. Go 1.27,
  isolated tests/ARM64, macOS Companion/media and Swift CompanionCore pass;
  Go 1.26 test, Browser Cue Delivery and Vet pass. Do not push until the exact
  run finishes.
- SC-020 tests now include active SQLite close/reopen, idempotent Pause,
  simulated wall-clock rollback while paused, pause reanchor, one valid
  post-resume interval, actual simulated Runtime Control/Cue Engine GO while
  capture is paused, and HTTP Owner+CSRF authorization versus Viewer/missing
  CSRF rejection. `go test` for `timingintelligence`, `runtimecontrol` and the
  focused `httpapi` route passed; focused Race runs and Vet passed. The GO test
  uses only the registered simulator adapter; raw timing evidence is retained
  but excluded from learning while paused.
- SC-013 and SC-020 remain partial overall: real-browser keyboard/touch/RTL
  review is pending; SC-013 assumed latency is not measured endpoint timing;
  Pi 4 qualification is blocked on a verified isolated host identity. No
  production clock write, ACL activation, real output, force push or merge.
- Next: wait for the Go 1.26 Race job. If run `43538108851` is green, commit
  the pending matrix/handoff edits and push both local feature/test commits
  normally to the existing branch/PR. Then monitor all checks for the exact
  new HEAD before starting another push; continue with SC-002 beyond #504/#505.

## Resume update — 2026-11-14 01:31 UTC

This section supersedes the local HEAD and worktree statements above.

- Pre-checkpoint Local HEAD: `6b3fb1280abd43d4736c64ba5a7d93837960f2d2`;
  Remote HEAD: `f64cde026a0fe890eb39e0b0fee89eda7af84d9d`. Three local commits
  are ahead: `e5fd6f1` SC-013 local Draft simulation, `68a64c9` SC-020 Store,
  Runtime Control and HTTP lifecycle regressions, `6b3fb12` matrix/handoff
  qualification update. Worktree was clean before this resume entry.
- Exact remote run `43538108851` remains in Go 1.26 Race; all other Core CI
  checks for `f64cde0` pass. Workflow runs `go test -race -timeout 45m ./...`;
  do not push while it is active.
- Local evidence since `f64cde0`: SC-013 Node suite (16 including SC-012
  large-list tests), Cue Studio contract, and `go vet` pass. SC-020
  `timingintelligence` full package, `runtimecontrol` simulated GO, and
  `httpapi` authenticated pause-route tests pass; focused Race and Vet pass
  for the touched packages. No production code change was needed for the
  paused-capture lifecycle: it is opt-in learning exclusion and reanchors on
  the first post-resume observation while GO remains available.
- #504/#505 are still open draft PRs and already cover clock status/correction
  preview; no duplicate edits or real clock write. Pi identity remains
  unverified. Production Project ACL remains disabled.
- Next: wait for run `43538108851`. If green, push the local feature/test/doc
  commits normally to PR #534 and wait for exact-new-head CI before another
  push. Then continue the next verified software gap while retaining browser,
  Pi and production qualification boundaries.

## Resume update — 2026-11-14 01:40 UTC

This entry supersedes the prior CI and next-action statements.

- Branch `integration/stagecore-rc`; pre-batch local HEAD
  `6b3fb1280abd43d4736c64ba5a7d93837960f2d2`; remote HEAD
  `f64cde026a0fe890eb39e0b0fee89eda7af84d9d`; Draft PR #534 remains open.
  The three commits since remote are the SC-013 simulator, SC-020 lifecycle
  regressions, and their matrix/checkpoint update. SC-024 access-level code,
  tests, qualification docs and the 01:31/01:40 handoff entries are the
  current additional local work. Earlier user edits have been preserved.
- Core run `43538108851` for exact remote `f64cde0` completed successfully at
  01:35 UTC. All Go 1.26/1.27, Race, Vet, isolated tests/ARM64 and macOS
  Companion checks passed. The normal push gate for this exact base is clear.
- SC-024 now has opt-in API support for `VIEW`, `EDIT`, `OPERATE`, `MANAGE`
  and `NONE`, with existing fine-grained custom permissions preserved.
  `MANAGE` cannot grant membership administration; STOP and Blackout remain
  separate underlying capabilities. Tests verify Viewer restrictions,
  independent STOP/Blackout custom grants, audit/readback and the explicit
  `PREPARED_NOT_ENFORCED` state. This is not installed in production.
- Verification of the authorization batch: full
  `go test ./internal/projectaccess ./internal/httpapi -count=1` passed;
  after splitting a new focused API test file to avoid unrelated gofmt churn,
  focused access-level/API tests passed again. Focused Race and Vet passed for
  the final test layout. `git diff --check` passed. The production assembly
  source guard remains unchanged.
- CUA browser inspection could not run because the Mac was locked. No browser
  or app interaction occurred. Real-browser UX and Pi 4 qualification remain
  open; no physical output or production resource was accessed.
- Next: commit the tested SC-024 API and docs to this local group, push
  normally to existing PR #534, and wait for exact-new-head Core CI before
  another push. Continue a verified software gap while that CI runs; retain
  Pi identity and production ACL boundaries.

## Resume update — 2026-11-14 01:44 UTC

This entry supersedes the preceding push instruction.

- SC-024 access-level batch commit `c9f2d47487bc90aaee80b6705a8fefab898c65a5`
  is pushed to the existing `integration/stagecore-rc` branch and Draft PR
  #534. Local and remote HEAD both equal `c9f2d47`; worktree is clean.
- The prior exact-head Core run `43538108851` on `f64cde0` completed green
  before push. New exact-head Core jobs are pending/in progress at run
  `43542130109`; the Companion workflow is `43542130081` and isolated/ARM64
  is `43542130095`. Wait for checks at `c9f2d47` before the next push.
- `c9f2d47` contains the tested opt-in `VIEW/EDIT/OPERATE/MANAGE/NONE` API
  mapping and compatibility tests. Production assembly remains unchanged.
- CUA inventory reported the Mac locked; no UI/browser actions occurred.
  Continue software work locally while CI runs, with real-browser and Pi 4
  checks still unqualified.

## Resume update — 2026-11-14 01:47 UTC

- Local base is `46d3b4ff4670dadab5c5e017b2fea380ea83821b`; pushed remote and
  PR #534 remain `c9f2d47487bc90aaee80b6705a8fefab898c65a5`. The new local
  SC-024 revoke cleanup is currently uncommitted with its matrix and
  qualification notes.
- Core exact-head run `43542130109` is still in Go 1.26 tests/Race workflow;
  Go 1.27, macOS Companion, Swift CompanionCore and isolated/ARM64 are green.
  Do not push this follow-up until the `c9f2d47` checks complete.
- A red-first regression failed before the fix because `SetPermissions(nil)`
  rejected a disabled target. The service now permits only grant removal for
  that account; nonempty access replacements remain denied. An HTTP test
  covers `NONE` revoke and verifies re-enabling the user does not restore
  access. Focused normal and Race tests, Vet and `git diff --check` pass.
- Next: review and commit this tested revocation batch locally. Continue the
  SC-024 endpoint audit while exact-head CI runs; push only after that CI is
  complete, then wait for CI on the new exact HEAD.

## Resume update — 2026-11-14 01:49 UTC

- Local starting commit is `46d3b4ff4670dadab5c5e017b2fea380ea83821b`; remote
  remains `c9f2d47487bc90aaee80b6705a8fefab898c65a5`. The disabled-user revoke
  fix, regressions and SC-024 qualification updates remain local and uncommitted.
- Core CI at `c9f2d47` still has Go 1.26 Race running; Go 1.27, Browser Cue
  Delivery, Vet, Companion, Swift and isolated/ARM64 jobs have passed. Do not
  push the local follow-up yet.
- Red-first test `TestOwnerCanRemoveProjectGrantsFromDisabledUser` failed
  before code change on `ErrDenied`. The updated service allows an Owner's
  empty grant replacement for a disabled account, while nonempty replacements
  remain denied. Service and HTTP regressions prove stored grants are removed
  and do not return if the account is re-enabled. The full touched-package
  suite, focused Race, Vet and diff checks pass.
- Next: commit this small fix and docs to the existing local chain, keep
  auditing software gaps while `c9f2d47` CI runs, and push only after its exact
  Core CI run completes. Production ACL stays disabled.

## Resume update — 2026-11-14 01:50 UTC

- Revocation fix is committed locally as
  `66ea38979ac03ca47193c33c9a6a909ab18bb7b9`; local HEAD is `66ea389`, remote
  remains `c9f2d47487bc90aaee80b6705a8fefab898c65a5`. The branch is two commits
  ahead. The only uncommitted change updates the matrix's reviewed HEAD/CI
  state.
- Exact remote Core run `43542130109` remains active in Go 1.26 Race. Go 1.27,
  Go 1.26 normal tests/Browser Cue Delivery/Vet, Companion/Swift and isolated
  ARM64 pass. No further push before Race completes.
- Red-first disabled-user revoke regression, service/API tests, full
  `projectaccess` and `httpapi` package tests, focused Race, Vet and
  `git diff --check` all pass locally. Temporary disable/re-enable without an
  Owner revoke remains unchanged and retains its stored grants.
- Next: commit this documentation checkpoint locally, then continue the ACL
  route audit or another verified software gap while waiting for exact-head
  CI. Push the local group only after run `43542130109` completes.

## Resume update — 2026-11-14 01:59 UTC

- Local code base is `0fc26289de2736c1492b662536f2b131ddc7bd05` on
  `integration/stagecore-rc`; remote
  remains `c9f2d47487bc90aaee80b6705a8fefab898c65a5`. The gated Security
  grant editor, Node regression suite, workflow wiring, and SC-024 docs are
  local and uncommitted. Prior local commits are preserved.
- Core run `43542130109` for `c9f2d47` still has Go 1.26 Race in progress;
  Go 1.27, normal tests, Browser Cue Delivery, Vet, Companion/Swift and
  isolated/ARM64 pass. Do not push yet.
- The Owner grant UI is rendered only when optional cutover preview returns
  `enforced=false`, warns that grants do not restrict access, requires current
  grant readback and explicit confirmation, and checks the same prepared state
  on write response. The test executes mocked form load/save and checks
  Viewer, disabled and unknown-role restrictions. No production route or
  authority was added.
- `node --check internal/operatorweb/static/security.js` passes; the combined
  local Node suite passed 43 tests, and the new `security_project_access`
  suite passes 6 tests. `git diff --check` passes. Actual browser visual,
  keyboard, touch and RTL acceptance remains unavailable because the Mac is
  locked.
- Next: commit the UI/tests/workflow/docs locally, wait for Core run
  `43542130109` to finish, then push the accumulated commits normally to PR
  #534. Continue other software gaps while CI runs and preserve the hardware
  and production ACL boundaries.

## Resume update — 2026-11-14 02:13 UTC

- Latest committed source HEAD is
  `60bf4877dba21f43bca870ca4633a3ebee0bcd96` on
  `integration/stagecore-rc`; remote remains
  `c9f2d47487bc90aaee80b6705a8fefab898c65a5` before the next push. Four local
  commits are ahead of remote (`46d3b4f`, `66ea389`, `0fc2628`, `60bf487`). The
  matrix and this handoff have a final documentation update in the worktree.
- Current remote CI is complete and green for `c9f2d47`: Core `43542130109`
  passed Go 1.26/1.27, Browser Cue Delivery, Vet, Race and Linux ARM64 builds;
  Companion/Swift `43542130081` and isolated/ARM64 `43542130095` passed. This
  satisfies the wait-before-push condition for that exact HEAD. Recheck the
  remote ref before the normal fast-forward push and then follow CI for the new
  exact HEAD.
- Commit `60bf487` adds a concurrent Mixed-Cue burst regression and benchmark,
  plus the prepared Owner grant editor and stale-read guard. Red-first UI
  regression failed while a Project was changed during an in-flight access
  read, then passed after the result was bound to its requested Project/User.
  Production API assembly and Project ACL enforcement remain unchanged.
- New simulation-only burst benchmark: 4 and 8 independent GO requests, each
  running a six-Cue/24-Action linked compound chain in its own SIMULATION
  Session; 100 bursts per worker count at Go `-cpu=1,4`. On M3, eight-way GO
  P95/P99 was 73.30/78.02 ms at CPU 1 and 92.53/97.27 ms at CPU 4; burst P95/P99
  was 83.75/85.91 ms and 94.65/101.00 ms. The eight-way CPU 4 case allocated
  3,809,552 B and 70,550 allocations per burst, averaged 1.77 GC/burst with
  60.14 µs pause; release-to-worker start skew P95/P99 was 12.17/18.00 ms.
  The complete benchmark command used 27.33 s wall, 13.31 s user CPU, 12.17 s
  system CPU and 224,460,800 bytes peak process-tree RSS. These Mac figures
  include setup/migrations where applicable and are not Hub-only or Pi data;
  the 100 burst P99 is a stress indicator. No optimization was made without a
  justified profile.
- Test evidence: the new eight-session regression passed with 192 simulated
  Actions and zero physical-executor calls; its focused Race test passed.
  `go test ./internal/cueengine ./internal/operatorweb -count=1`, package Vet,
  `node --check` for Security, and the combined Core Node UI set (44/44) passed.
  `git diff --check` passed before source commit.
- Pi qualification remains open. The explicitly named qualification SSH
  endpoint returned `websocket: bad handshake` and closed before a remote
  command ran. Do not infer Pi performance from M3 results or ARM64 cross-builds;
  do not try the ambiguous `stagecore-pi` alias. No production service/data,
  original show, physical output or ACL activation was touched.
- First action on resume: read current `git rev-parse HEAD`, `git status`, and
  `git ls-remote`; commit the matrix/handoff documentation update, push normally
  to the existing branch/PR #534 only if it remains a fast-forward, and watch
  new exact-head Core CI. Then pursue the next verified software gap from the
  matrix (SC-011 Preflight drift/remedies or SC-024 route fencing) while keeping
  Pi performance and browser visual acceptance explicitly open.

## Resume update — 2026-11-14 02:35 UTC

- Local source HEAD is `14ea1caa762d912c5de86d098a54a27d58535beb` on
  `integration/stagecore-rc`; pushed remote HEAD remains
  `447849f43eaf89cd501a6bd1b345195a61725312`. The five local code commits are
  `d67da3c`, `80f20a9`, `9bf2696`, `4f703ce`, and `14ea1ca`. No uncommitted
  source changes remain; the matrix and this handoff are being updated. No
  reset, clean, force push, branch change, or PR replacement was used.
- Exact pushed-head Core run `43545195433` is still in progress: Go 1.26 tests,
  Browser Cue Delivery and Vet passed, Go 1.27 CI passed, but the Go 1.26 Race
  step is still running and ARM64 builds have not started. Companion/Swift run
  `43545195409` and isolated/ARM64 run `43545195451` passed. Do not push the
  local batch until `43545195433` completes; then confirm the branch still
  fast-forwards from `447849f` and push normally to PR #534.
- SC-024 changes add safe revoke of disabled-user grants and an Owner-only,
  isolated, `PREPARED_NOT_ENFORCED` access-level editor. Stale async loads and
  completed saves are bound to their original Project/User selection. The
  production Hub still does not assemble Project ACL routes; production ACL is
  not activated. Security UI tests cover 8 cases.
- SC-011 follow-ons export the already evaluated Preflight report as a
  timestamped JSON download, explain stale RTC clock consequences for local
  authentication, and distinguish Companion MIDI inventory from Ableton
  receipt/execution. Commit `14ea1ca` adds a read-only comparison of current
  revision lighting bindings with the selected published Snapshot. Drift is an
  advisory WARN with a remediation link; the published Snapshot remains the
  runtime authority. Its red-first SQLite test proves match PASS and changed
  lighting WARN without blocking SHOW. No output probes or device commands
  were added.
- Local verification: full UI Node suite 49/49; Preflight package tests pass;
  `go test -race ./internal/preflight -count=1`, `go vet ./internal/preflight`,
  both Preflight JavaScript syntax checks, and `git diff --check` pass. The
  lighting drift regression failed before the service implementation as
  expected, then passed. M3 benchmark evidence remains as recorded above;
  Pi 4 performance qualification is still blocked by the explicitly named
  `stagecore-qualification` SSH endpoint returning `websocket: bad handshake`
  before a remote command could start. Do not infer Pi results from M3 or CI
  ARM64 builds.
- The current matrix records SC-011 report export, RTC/authentication guidance,
  MIDI inventory-only semantics and lighting Snapshot drift, and the corrected
  SC-024 UI test count. Related PR heads/CI states remain as recorded in
  `related-component-qualification-matrix.md`; no linked repository was edited.
- First action on resume: poll exact-head Core run `43545195433`. If it is
  green and `origin/integration/stagecore-rc` is still `447849f`, push the
  accumulated local commits and documentation to the same branch/PR, then
  follow new exact-head CI. Next software review is the SC-024 route-to-policy
  and in-flight revocation audit; keep ACL production-disabled. Preserve the
  no-production/no-real-device boundaries, and continue keeping Pi ARM64
  performance and real-browser visual/touch acceptance open.

## Resume update — 2026-11-14 02:45 UTC

- Current code HEAD is `6472e27be5ced0b4d81d61bf817b7bfd5aa1411c` on
  `integration/stagecore-rc`; pushed remote is still
  `447849f43eaf89cd501a6bd1b345195a61725312`. Seven local code commits and the
  checkpoint commit are ahead. The worktree has only the matrix update in
  progress. Recheck both refs before a normal push; no force push or branch/PR
  change is allowed.
- Core run `43545195433` on exact pushed HEAD `447849f` remains in progress at
  Go 1.26 Race. Its normal Go 1.26 tests, Browser Cue Delivery, Vet and the
  complete Go 1.27 job passed; ARM64 build has not run yet. Companion/Swift
  `43545195409` and isolated/ARM64 `43545195451` remain green. Push must wait
  until the Core run is complete.
- SC-010 Issue #489 had a concrete missing acceptance item: preset rows only
  exposed Add/Delete, with no content preview or target-availability notice.
  The Cue Studio now has a native disclosure with the saved Action JSON and a
  clear target-present/missing status. Missing targets remain visible and
  editable in the Draft; the insertion path still clears `action_id` and does
  not actuate output. A Node harness reproduced the missing preview before
  the change and now covers both target states. The focused test is included
  in Core CI.
- Verification after that change: full Node UI suite 51/51; `go test
  ./internal/operatorweb ./internal/httpapi -count=1`; focused Preflight and
  Route/UI contract tests; JS syntax checks and `git diff --check`. The
  combined OperatorWeb/API tests passed on this Mac. SC-011 lighting drift
  already passed Preflight package normal/Race/Vet tests as recorded above.
- SC-024 route review confirmed the opt-in implementation rechecks delayed
  GO/JUMP Action authority at the dispatch boundary and has regressions for
  grant revocation after the first simulated Action, post-operation authority
  loss, session/project mismatch, and blackout latch preservation. This
  narrows the remaining audit; it does not authorize the production assembly,
  which intentionally omits the optional routes. A full route-to-policy,
  realtime, ticket and Companion compatibility review remains required.
- Pi qualification stays open. The only configured `stagecore-qualification`
  alias resolves to `ai.ali96adil.com:22`, and the earlier SSH attempt failed
  before host identity or remote execution; do not substitute an ambiguous
  host. M3 scale data remains non-Pi evidence. No production service/data,
  original Vault/show, device command, firmware or ACL cutover was touched.
- First action on resume: poll `43545195433` again. When it completes, check
  `origin/integration/stagecore-rc` is still exactly `447849f`; push the seven
  code commits and documentation normally to existing PR #534, then follow
  the new exact-head Core CI. Continue with SC-024 route-to-policy and realtime
  evidence plus the next unclosed software acceptance; keep ACL disabled in
  production and Pi/real-browser qualification boundaries explicit.

## Resume update — 2026-11-14 02:46 UTC

- Latest code HEAD is `dd77070aa05288939240919b51ba73aaac9434b4`. Remote remains
  `447849f43eaf89cd501a6bd1b345195a61725312`; branch is ahead by nine total
  commits: eight code commits plus checkpoint `030fbaf`. The matrix and this
  note are the only uncommitted changes. Keep all history and push only a
  normal fast-forward to existing PR #534.
- Core run `43545195433` is still in Go 1.26 Race on exact remote HEAD
  `447849f`; Go 1.26 normal tests, Browser Cue Delivery and Vet passed, and
  Go 1.27, Companion/Swift and isolated/ARM64 are green. ARM64 product builds
  remain pending. Wait for this run to finish before push.
- SC-010 follow-on is committed in `6472e27` and `dd77070`. Preset rows now
  show saved Action content in an accessible disclosure, identify missing vs
  configured Project targets before insertion, and retain editable Draft
  behavior. A red-first UI regression failed because the old list had no
  preview. Tests confirm that adding a preset gives the Draft an empty Action
  ID and does not mutate the stored preset object. The test is included in the
  Core Node command; the newly expanded Node suite passed 51/51 and the focused
  two-case suite passed again after the identity check was added.
- Current focused evidence remains green: `go test
  ./internal/operatorweb ./internal/httpapi -count=1`; Preflight normal/Race,
  Vet and JavaScript syntax checks; `git diff --check`. No device output was
  sent and no production route or data changed. The qualification alias still
  cannot establish a remote host identity; Pi metrics remain open.
- The matrix now reflects SC-010 preview/target/copy evidence and the narrower
  remaining scope, SC-011 lighting drift and the eight local code commits.
  First action on resume: poll exact-head Core run `43545195433`. If green and
  the remote ref remains `447849f`, push eight code commits plus the two
  documentation commits without force, then wait on CI for the pushed head.
  Continue campaign work after CI; SC-024 remains opt-in and production ACL
  must stay disabled.

## Resume update — 2026-11-14 02:48 UTC

- The preceding Core run `43545195433` completed successfully on pushed
  `447849f`, including Go 1.26/1.27 tests, Browser Cue Delivery, Vet, Race and
  ARM64 builds. Companion/Swift `43545195409` and isolated/ARM64 `43545195451`
  also passed. The remote ref still matched `447849f` before push.
- Pushed the accumulated batch normally to the existing
  `integration/stagecore-rc` branch and Draft PR #534: `447849f..70a7061`.
  This includes eight code commits and two checkpoint/matrix commits. No
  force push, rebase, branch creation or PR replacement was used.
- New exact-head CI is active for `70a7061d2597e14273fb51468b269c7dff73ea71`:
  Core `43548179642` is in progress on Go 1.26 and Go 1.27; Companion/Swift
  `43548179629` is in progress; isolated/ARM64 `43548179637` passed. Do not
  push more work until the Core and Companion/Swift checks complete.
- The only local modification is the matrix update to show the pushed HEAD and
  new CI IDs; it is not a source change. Commit it locally after `git
  diff --check`, but leave it unpushed until exact-head CI completes. Preserve
  current remote HEAD and confirm fast-forward before any subsequent push.
- Next independent software review while CI runs: SC-016 Action clipboard
  behavior, specifically payload/timing fidelity, new Action IDs and selected
  clipboard fallback. Existing tests cover current-user/current-Project
  isolation; check before adding tests so completed behavior is not repeated.
  Keep the hardware, production service/data and ACL restrictions unchanged.

## Resume update — 2026-11-14 02:51 UTC

- Pushed HEAD remains `70a7061d2597e14273fb51468b269c7dff73ea71`; latest local
  code HEAD is `b6284cb48829614e3119c0e389e711e030f6001f`. Local branch is ahead
  by two commits (`5822020` checkpoint and `b6284cb` source), and the matrix
  update is being committed. Do not push before the current exact-head Core CI
  completes.
- CI for `70a7061`: Core run `43548179642` passed Go 1.26/1.27 tests, Browser
  Cue Delivery and Vet; Go 1.26 Race is in progress and ARM64 builds are
  pending. Companion/Swift `43548179629` and isolated/ARM64 `43548179637` both
  passed. This is the only current pushed HEAD; the previous `447849f` set is
  fully green.
- SC-016 Issue #495 review found the clipboard was already local/offline,
  scoped to the same User and Project, ID-clearing and cloning on each paste.
  Its existing test had not covered error policy, mode, priority, enabled
  state, or target loss. Commit `b6284cb` adds a red-first target-unavailable
  warning that leaves the pasted action editable, plus coverage for full
  execution-policy fidelity and suppressing warnings for a configured target.
  Cross-Project paste is still refused by the existing scope fence.
- Local checks after SC-016: full Node suite 51/51, focused clipboard suite
  included in that run, `go test ./internal/operatorweb -count=1`, Cue Studio
  JS syntax and `git diff --check`. The pre-fix test failed because no target
  warning was emitted; it passed after the change. The operator is not required
  to grant native clipboard access.
- The matrix now records this evidence and keeps multi-select copy and
  interactive remapping as remaining SC-016 work. First action on resume:
  poll Core run `43548179642`; when Core Race/ARM64 finish successfully, verify
  the branch remote remains exactly `70a7061` and fast-forward push the local
  code/doc batch to PR #534. Then follow exact-head CI before another push.
  Production ACL remains disabled; no devices, production services or show
  data were changed.

## Resume update — 2026-11-14 02:57 UTC

- Local HEAD is `0737c00197c38a528773424e459defa0b6c1c165`; remote HEAD and
  Draft PR #534 remain `70a7061d2597e14273fb51468b269c7dff73ea71`. The local
  branch is ahead by four commits: `5822020`, `b6284cb`, `ba295ab`, and
  `0737c00`. The matrix and this new checkpoint update are uncommitted. Keep
  this work and push only after the exact-head Core CI completes.
- Core run `43548179642` on `70a7061` still passes Go 1.26/1.27 tests, Browser
  Cue Delivery and Vet; Go 1.26 Race is in progress and Linux ARM64 builds are
  pending. Companion/Swift `43548179629` and isolated/ARM64 `43548179637`
  passed. No later push has occurred, so these runs have not been cancelled.
- SC-016 follow-on commit `0737c00` adds selection controls and ordered
  session-local copy/paste for multiple Mixed-Cue Actions. Copies preserve
  payload, per-action delays, error/retry policy, execution mode, priority and
  enabled state; pasted actions receive blank IDs and cloned objects. Current
  User/Project fencing and target-availability warning remain active. A
  red-first test failed before the function existed; it now proves ordered
  Lighting + OSC copy, policy fidelity, source identity preservation, and
  cross-Project refusal.
- Verification on local `0737c00`: full Node suite 52/52; focused Cue Studio
  tests 12/12; `go test ./internal/operatorweb -count=1`; `go vet
  ./internal/operatorweb`; JavaScript syntax and `git diff --check` all pass.
  No network dispatch or device output is part of this feature.
- The matrix records multi-select as implemented in code; interactive
  cross-Project target remapping remains partial. The next campaign task after
  the CI gate is the opt-in SC-024 route-to-policy and in-flight session/ticket
  fencing audit. Project ACL remains disabled in production. Pi ARM64
  performance acceptance is still hardware-environment pending; do not use
  real outputs.
- First resume action: poll run `43548179642` until Race and ARM64 build jobs
  finish. If green and remote still equals `70a7061`, verify fast-forward then
  push the local code and checkpoint commits normally to the existing branch
  and PR #534; follow the resulting exact-head CI before another push.

## Resume update — 2026-11-14 03:04 UTC

- Local code/test HEAD is `73c08859ff5f5b1bb2f2a0ae7c43d28670d5aaf8`; remote
  remains `70a7061d2597e14273fb51468b269c7dff73ea71`; PR #534 remains the same
  open Draft. The local branch is six commits ahead before this documentation
  checkpoint. Do not push until Core run `43548179642` completes.
- Re-audited SC-024 issue #511 and route/runtime boundary evidence. The static
  route inventory digest and dynamic registration allowlist fail on unseen
  route changes; qualification mode requires a policy for every project-ID
  route. `/api/v1/events` currently publishes only `session.ready` plus
  keepalives and rechecks the exact browser session; it has no Project data.
  Companion capture ticket authority is checked against the issuing browser
  session and optional Project grants around bounded transfer/receipt paths.
  Runtime intent checks bind the Project, active Session and Snapshot; GO/JUMP
  delayed dispatch, independent STOP/Blackout grants and Owner-only blackout
  clear are covered in code/tests.
- Focused SC-024 verification passed: `go test ./internal/httpapi -run
  'SC024|Qualified|ShowAdmissionRechecksGrant|DelayedCueStopsDispatchAfterShowGrantRevocation|DelayedJumpStopsDispatchAfterShowGrantRevocation|ShowExitRechecksGrant|EmergencyBlackout' -count=1`,
  `go test ./internal/store -run
  'CompanionUploadTicket|Project.*Grant|SimulationCheckpoint.*ACL|CueExecutionInsertRejectsRevokedSimulationGrant' -count=1`,
  and `go test -race ./internal/projectaccess -count=1` (55.3s). This review
  found no new reproducible gap in these already-covered boundaries; it does
  not close the whole-server cutover or authorize production ACL.
- SC-016's bulk-copy regression now also pastes the mixed Lighting+OSC
  selection twice, mutates the later copy, and proves the earlier payload is
  unchanged; Project switching still blocks stale clipboard use. Latest full
  Node suite: 52/52. `73c0885` records this test refinement. The matrix records
  this along with the SC-024 session/ticket findings.
- Core run `43548179642` on pushed `70a7061` is still in Go 1.26 Race; ARM64
  build remains pending. Go 1.27, Go 1.26 regular tests/Browser Cue Delivery/
  Vet passed. Companion/Swift `43548179629` and isolated/ARM64
  `43548179637` passed. No push has cancelled the current run.
- Next action: finish the remote CI gate; if successful, verify exact remote
  `70a7061` and fast-forward-push the accumulated local batch to existing PR
  #534. After its CI, continue the next software-only gap: SC-010 multi-action
  Cue template/preset support, then other open SC rows. Pi performance remains
  pending the isolated ARM64 host; do not substitute Mac results or real
  devices. Production services/data/Vault remain untouched and Project ACL
  remains disabled.

## Resume update — 2026-11-14 03:16 UTC

- Local code HEAD is `8690da0f120766699a00c4df6225dbb34b63e83f`; remote
  `integration/stagecore-rc` and Draft PR #534 are still at
  `70a7061d2597e14273fb51468b269c7dff73ea71`. Eight local commits precede
  this checkpoint. Worktree changes are documentation only. Keep the local
  batch and wait for the exact-head Core run before pushing.
- SC-010 Issue #489 now supports saving selected mixed Actions as a named,
  persistent Cue preset. It stores the ordered JSON array in the existing
  preset column and returns it as `actions`; existing single presets still
  use the original `action` property and need no schema migration. Cue Studio
  previews the group, warns if any target is absent, and inserts ordered deep
  copies with blank Action IDs. Repeated insertion is independent. The red
  regression failed on the old UI (`actions` was treated as an empty
  single-action preset) before the implementation.
- SC-010 verification: Node preset tests 4/4; full Node suite 54/54; full
  `go test ./internal/httpapi -count=1` passed (59.7s); full
  `go test ./internal/store -count=1` passed (5.3s); targeted HTTP Race passed
  (7.1s); `go vet ./internal/httpapi ./internal/store`, JS syntax and
  `git diff --check` passed. No migration, Snapshot publish or output dispatch
  was involved.
- Core CI `43548179642` on remote `70a7061` still has Go 1.26 Race in progress;
  Go 1.26 standard tests, Browser Cue Delivery, Vet and Go 1.27 passed. Linux
  ARM64 product builds remain pending. Companion/Swift `43548179629` and
  isolated/ARM64 `43548179637` passed. Do not push while this run is active.
- The matrix now marks multi-action Cue presets implemented and keeps preset
  search/descriptions and cross-Project target remapping partial. First resume
  action: poll `43548179642`; when Core Race/ARM64 finish successfully and the
  remote still equals `70a7061`, fast-forward-push local work to existing PR
  #534, then follow exact-head CI. Continue software-only qualification; Pi
  P95/P99/Jitter and real device acceptance remain pending the isolated Pi
  host. Production Hub/Relay, show data/Vault and production ACL remain
  untouched/disabled.

## Resume update — 2026-11-14 03:28 UTC

- Current pushed `integration/stagecore-rc` and PR #534 head:
  `9d5b775aa94875591d9ff7964e3eb8057f775c9b`. The local search improvement is
  committed as `ba495b867de432f7ac07cf089de57119464f9c98`; matrix and this
  checkpoint update are local follow-on documentation. No reset, clean,
  rebase, force push or production operation occurred.
- SC-010 Cue Studio now offers a local search across preset names and nested
  saved action JSON, including target/capability/parameters. The red-first
  test failed before the input/filter existed, then passed with the change.
  The feature keeps existing project/user scoping, API shape and schema.
- Verification after the code commit: focused preset tests 5/5; full Node suite
  55/55; `go test ./internal/operatorweb -count=1`; `go vet
  ./internal/operatorweb`; JavaScript syntax; `git diff --check` all passed.
- Exact pushed-head Core run `43551276214` is still active: Go 1.27 passed;
  Go 1.26 Test, Browser Cue Delivery and Vet passed; Go 1.26 Race is running
  and Linux ARM64 product builds are pending. Companion Core
  `43551276183` (Swift Core plus full macOS replacement/media acceptance) and
  Camera Relay `43551276104` passed. Do not push until Core completes and the
  remote branch is verified unchanged.
- Rechecked related PR heads and current associated CI: TabletPlayer #36
  `91613da1` / run `42719885664`, DMX Lighting #39 `3ad7749` / run
  `42759555665`, Camera #20 `455b066` / run `42761096513`, StageLaser #56
  `b14a578` / run `42765890415`, and StageLaser #57 `c4ff10b` / run
  `38054411190` are all open and their listed source/build checks pass.
  Their installed/physical requirements remain separate; no related PR was
  changed in this checkpoint.
- Remaining: descriptions and safe cross-Project target mapping/import in
  SC-010; Pi 4 ARM64 performance run remains unavailable after the known SSH
  websocket handshake failure. Continue with the highest evidenced software
  gap after the pending CI gate. Keep ACL production-disabled; do not touch
  production Hub/Relay, show data/Vault or real devices.

## Resume update — 2026-11-14 03:33 UTC

- Follow-on code HEAD is `b6242b4dd12b9ae891d62268a29a037136ac10ab`, two
  tested feature commits ahead of remote `9d5b775aa94875591d9ff7964e3eb8057f775c9b`;
  the completion matrices and handoff have local documentation updates.
- SC-010 also now lets the operator remap each missing target in a saved
  Mixed-Cue action group to a target from the current Project. Each selection
  applies only to the inserted Draft copy, never edits the saved preset, keeps
  action order/parameters, and warns to verify compatibility before publish.
  Leaving a target unchanged keeps the missing-target warning. This does not
  import presets across Projects.
- Red-first per-action remapping regression passed after implementation. Latest
  focused preset tests 6/6 and full Node suite 56/56 passed; Operator web Go
  tests and Vet, JS syntax, and `git diff --check` passed. No API, DB schema,
  Snapshot, runtime dispatch or external output changed.
- At 03:33 UTC, exact pushed-head Core CI `43551276214` remained in Go 1.26
  Race after Go 1.26 Test/Browser Cue Delivery/Vet and Go 1.27 had passed;
  Linux ARM64 product builds were pending. Companion `43551276183` and Camera
  Relay `43551276104` passed. Do not push until the full Core run completes,
  then verify remote still equals `9d5b775` before normal fast-forward push to
  PR #534.
- The related hardware PRs remain open with their exact-head software checks
  passing: Tablet #36 `91613da`, DMX #39 `3ad7749`, Camera #20 `455b066`,
  StageLaser #56 `b14a578`, and #57 `c4ff10b`. No related branch was modified.
- First resume action: poll Core run `43551276214`; if successful, verify the
  remote and push the three local commits (`ba495b8`, `b6242b4`, plus the
  completion-matrix/handoff checkpoint) to the existing branch/PR, then follow
  new exact-head CI. If CI fails, diagnose its failure before pushing. Continue
  to SC-010 descriptions/cross-Project import and the next evidenced software
  gap. Raspberry Pi 4 P95/P99/Jitter qualification remains blocked by the
  unverified SSH websocket handshake; do not substitute Mac or CI cross-build
  measurements. Production ACL remains disabled; production services/data,
  original Vault/show files and real devices stay untouched.

## Resume update — 2026-11-14 03:38 UTC

- Local HEAD is `184b719d8b74cf12ae467680b0da4efe4e632b06`, four code/test
  commits ahead of pushed PR #534 head `9d5b775aa94875591d9ff7964e3eb8057f775c9b`.
  Three matrix/handoff files contain the pending documentation checkpoint.
- SC-010 selector generation is now lazy: a missing-target section is collapsed
  by default, and it groups Actions that share the same old target. This avoids
  eagerly constructing action-by-target option lists for every saved preset.
  The per-target map changes only inserted Draft copies; no saved preset or
  execution path changes.
- SC-005 evidence was extended: store duplication tests preserve the Cue label
  and operator color, while a full self-contained Show Capsule round-trip test
  checks the same metadata across export and materialization from the published
  Snapshot. No production show or Vault was used.
- Verification: SC-010 Node preset tests 6/6 and full Node suite 56/56; Cue
  Studio JavaScript syntax and `go test`/`go vet` for `internal/operatorweb`;
  full `go test ./internal/store ./internal/showcapsule -count=1` and Vet for
  both packages; `git diff --check` all pass. No race-specific behavior was
  changed in the store/capsule additions.
- Exact pushed-head Core `43551276214` remains in Go 1.26 Race: Go 1.26
  Test/Browser Cue Delivery/Vet and Go 1.27 passed; ARM64 product build is
  pending. Companion `43551276183` and isolated/ARM64 Camera Relay
  `43551276104` passed. Continue not to push until Race and ARM64 finish and
  verify remote still equals `9d5b775`.
- Next step once CI succeeds: review the four-commit source/test diff and the
  updated completion matrices, create the fifth documentation checkpoint
  commit, recheck exact remote head, then fast-forward push all five commits to
  existing branch/PR #534 and monitor new exact-head CI. Afterward continue
  the remaining cross-Project preset import/description gap and other highest
  open software rows; Pi performance still requires a verified ARM64 host.
  Production ACL stays disabled and production/device boundaries remain in
  force.

## Resume update — 2026-11-14 03:43 UTC

- The fifth checkpoint commit is now saved: local HEAD `45070db8400b83bb9ad2870cb04b3322700ef933`. The working tree is clean and the branch is five commits ahead of `origin/integration/stagecore-rc` at `9d5b775aa94875591d9ff7964e3eb8057f775c9b`.
- Exact-head Core run `43551276214` is still in its Go 1.26 Race step; all earlier reported Go 1.26/1.27 test, Browser Cue Delivery, and Vet steps are successful. Linux ARM64 product build follows Race. Companion and Camera Relay on that exact head pass. No push has occurred.
- The five-commit range is `ba495b8` preset search, `b6242b4` per-target Draft remapping, `46b76f9` lazy remap controls, `184b719` label/color duplicate and Capsule regressions, and `45070db` completion matrix/handoff documentation.
- Local verification remains: Node 56/56, focused preset tests 6/6, Operator web Go tests/Vet, full Store and Show Capsule tests/Vet, JavaScript syntax and diff checks all pass. Exact remote pointer was read with `git ls-remote`; PR #534 remains open/draft at the same SHA.
- Next action is unchanged: wait for Core Race and ARM64, diagnose any failure before pushing, then re-read remote SHA and fast-forward push to existing PR #534 only if it is still `9d5b775`. Follow new exact-head CI after push. Pi performance, production ACL cutoff, hardware, and deployment boundaries are unchanged.

## Resume update — 2026-11-14 03:47 UTC

- Local source/test HEAD is `714ade572bfc77556b049fb455bf5837d649d6c4`; the
  branch is eight commits ahead of remote `9d5b775aa94875591d9ff7964e3eb8057f775c9b`.
  Completion matrices are being updated with this coverage checkpoint.
- SC-010 round-trip tests now use a single four-action Mixed Cue containing
  Lighting fade, MIDI send, OSC send and Tablet media play. Cue Studio inserts
  all four in order with independent blank Action IDs and independent payload
  copies; the authenticated API persists and reads the ordered group while
  retaining the legacy single-action JSON shape. The capability coverage is
  software serialization only and never dispatches an output.
- Verification: full Node suite 56/56, preset tests 6/6, JavaScript syntax;
  targeted `TestCueEditorRecoveryIsPrivatePersistentAndFencesStaleTabs` passed
  normally and under Race; `go vet ./internal/httpapi` and `git diff --check`
  passed. No implementation code changed in this follow-on; this closes a
  capability-coverage test gap.
- Exact remote Core run `43551276214` is still in Go 1.26 Race, which started
  at 03:25:18 UTC; the workflow has a 45-minute timeout for its full `go test
  -race -timeout 45m ./...`. Earlier Go 1.26 Test, browser delivery and Vet,
  and Go 1.27 passed. Linux ARM64 build is still pending. Companion and Camera
  Relay runs on pushed `9d5b775` pass. Do not push until full Core completes.
- After updating the matrix, push the tested local range to the same PR only
  after re-reading `git ls-remote` and confirming the remote remains
  `9d5b775`. Monitor the resulting exact-head Core CI before the next remote
  batch; continue with the next evidenced gap afterward. Pi execution remains
  unavailable, and no production service/data, Vault, device or production ACL
  was touched.

## Resume update — 2026-11-14 03:49 UTC · pre-push gate passed

- Exact pushed-head Core run `43551276214` on `9d5b775aa94875591d9ff7964e3eb8057f775c9b` is complete and successful. Go 1.26 and 1.27 Tests, Browser Cue Delivery, Vet, full Race (`go test -race -timeout 45m ./...`), and Linux ARM64 product builds passed. Companion `43551276183` and Camera Relay `43551276104` also passed on that exact head.
- Local HEAD before this checkpoint is `d457504ba322189a3f62766965b82e6fad894295`, nine commits ahead of remote `9d5b775`. The range is `ba495b8`, `b6242b4`, `46b76f9`, `184b719`, `45070db`, `da56f8d`, `2078a2f`, `714ade5`, `d457504`. It adds SC-010 preset search and per-target Draft remapping; lazy mapping UI; SC-005 duplicate and Show Capsule label/color regressions; four-action Lighting/MIDI/OSC/Tablet preset round-trip tests; and evidence/matrix checkpoints.
- Local verification: Node suite 56/56 and preset tests 6/6; Operator web tests/Vet; full Store and Show Capsule tests/Vet; the HTTP multi-action preset test passed normally and under Race; JS syntax and range `git diff --check` passed. The nine-commit range is a fast-forward descendant of remote.
- Before the push, read `git ls-remote origin refs/heads/integration/stagecore-rc` again. Push only if it still equals `9d5b775`; use a normal fast-forward to the existing branch/PR #534. Do not force push or merge. The new head will start fresh Core/Companion/Camera checks; wait for those before the next push.
- Hardware/software boundary is unchanged: Pi 4 runtime performance remains unqualified because the authorized host handshake did not establish identity; no substitute ARM64 CI build for P95/P99/Jitter. Production Hub/Relay, show/Vault data, real devices, firmware, production ACL and main branch remain untouched.

## Resume update — 2026-11-14 04:00 UTC · SC-008 metadata preservation

- Local code/test HEAD is `4e89372b2c88b08df298d96d9acc5701419985ef` on
  `integration/stagecore-rc`; pushed remote HEAD and PR #534 remain
  `ce927d63e5f37f6a02d6a745b98567f9eb9928d5`. The worktree contains only this
  campaign's matrix/checkpoint edits. No reset, clean, force push or rebase was
  used.
- SC-008 red-first coverage reproduced loss of unknown OSC parameter metadata
  after a guided-field edit. `f002SyncActionCard` now retains unsupported
  top-level OSC/MIDI parameter fields and per-argument OSC metadata while
  updating typed fields. Advanced JSON and existing cue payload shapes remain
  unchanged; no execution, API, database or device path changed.
- Verification: full Node suite 57/57; `go test ./internal/operatorweb
  -count=1`; `go test -race ./internal/operatorweb -count=1` (25.6 s); `go vet
  ./internal/operatorweb`; JavaScript syntax and `git diff --check` all pass.
  The physical executor is not reachable from these UI tests.
- Exact-head Core run `43553686731` for pushed `ce927d6` was still in progress
  at 04:00 UTC in Go 1.26; Go 1.27 passed. Companion `43553686720` and Camera
  Relay `43553686712` completed successfully on the same exact SHA. Do not
  push the local code or documentation until Core is green; then re-read the
  remote pointer and fast-forward push to the same PR only if it still equals
  `ce927d6`.
- Pi 4 performance is still not qualified. A strict, read-only retry of
  `stagecore-qualification` again failed with `websocket: bad handshake`
  before host identity; no remote command ran. Mac figures and ARM64 builds
  remain separate evidence, not Pi runtime P95/P99/Jitter or CPU/RAM evidence.
- Next software task after the CI gate: complete browser-model create/edit/save/
  reopen/preview regression coverage for nested compound Mixed Cues, including
  action ordering, policies and unsupported fields. Continue with SC-009
  durable history and SC-010 preset portability after that. Keep production
  Hub/Relay/show/Vault data untouched, all physical outputs simulated, and
  production Project ACL disabled.

## Resume update — 2026-11-14 04:08 UTC · nested Mixed-Cue round trip

- Local source/test HEAD is `0435b0e8c8b9d0a2d24c95d54118f47c2c4a6283`;
  pushed branch and PR #534 remain at `ce927d63e5f37f6a02d6a745b98567f9eb9928d5`.
  The three local commits are `4e89372` (preserve unknown typed OSC/MIDI
  metadata), `9c9c299` (SC-008 checkpoint), and `0435b0e` (nested Mixed-Cue
  round-trip test). A matrix/handoff refresh is being committed with this entry.
- `tests/cue_studio_roundtrip.test.js` exercises the actual Cue editor submit
  handler for edit (PUT) and creation (POST), saves five mixed Actions across
  OSC, MIDI, Lighting, Tablet and Laser in a three-level linked group, checks
  order, barriers, start delay, error policy and JSON extension fields, reopens
  the saved payloads, then runs the Inspector's local no-dispatch plan. API
  calls are captured by an in-memory stub; outputs are never invoked.
- The full Node suite passes 58/58. The prior SC-008 implementation batch also
  passed Operator Web package tests, Race, Vet, both relevant JavaScript syntax
  checks and `git diff --check`.
- At 04:08 UTC exact-head Core `43553686731` on remote `ce927d6` remains in Go
  1.26; Go 1.27 is green. Companion `43553686720` and Camera Relay
  `43553686712` are green. No push until exact-head Core completes; re-read
  remote before a normal fast-forward push of the accumulated tested commits.
- Strict read-only SSH to `stagecore-qualification` still fails before host
  identity with `websocket: bad handshake`; no remote command ran. Pi 4 full
  Engine, burst/sustained P95/P99/Jitter, and CPU/RAM qualification remain
  open. No Mac or CI ARM64 value is substituted for Pi runtime evidence.
- SC-008's nested mixed create/edit/save/reopen/preview software-model gap is
  now covered. First next implementation/qualification task is SC-009 undo/
  redo across Mixed Action edits, ordering, Cue links, reload and overlapping
  tabs; durable named checkpoints remain open only if the existing recovery
  workflow cannot meet that acceptance. Production/data/device/ACL boundaries
  remain unchanged.

## Resume update — 2026-11-14 04:12 UTC · SC-009 Mixed-Cue history

- Local code/test HEAD is `75068afe830cd3f40028dd7514527fffb13871df`;
  remote branch and Draft PR #534 stay at `ce927d63e5f37f6a02d6a745b98567f9eb9928d5`.
  Five local commits form the tested continuation: `4e89372`, `9c9c299`,
  `0435b0e`, `a7eea73`, `75068af`; this matrix/handoff update is pending its
  documentation commit. The worktree was otherwise clean.
- SC-009 history regression now undoes and redoes a two-Action mixed Cue after
  reordering/editing, including Cue links, GO-relative delay, timecode, OSC and
  MIDI payload metadata, Action start delays, timeout/error policy, priority
  and enabled state. Full Node suite remains 58/58. Existing `tests/cue_recovery.test.js`
  already covers closed-tab recovery, overlapping editors and stale generation
  fencing, so those paths were not recreated.
- Reviewing Issue #488 confirmed timestamped named checkpoints with restore
  preview and Cue-level create/delete/duplicate/reorder undo are still
  unimplemented. Current server recovery stores only the latest Draft per
  user/Project and Cue history is dialog-local. Next software task: implement
  a bounded Draft-only timestamped checkpoint workflow with explicit preview
  and restore, scoped to the current user/Project/Cue, without publishing or
  affecting runtime snapshots.
- At 04:11 UTC exact-head Core run `43553686731` on pushed `ce927d6` remained
  in Go 1.26; Go 1.27 passed. Companion `43553686720` and Camera Relay
  `43553686712` passed. Wait for Core to finish before pushing the five
  existing local commits plus this checkpoint; re-read remote `ce927d6` first.
- Pi performance remains unqualified because strict SSH still fails before
  host identity. No remote command, production service/data, Vault, physical
  output or production ACL was touched.

## Resume update — 2026-11-14 04:31 UTC · named Draft checkpoints

- Code HEAD: `c6a99decf37deb58a1b3bba72043c3640117b4ed`; remote branch and PR
  #534 remain at `ce927d63e5f37f6a02d6a745b98567f9eb9928d5`. Seven local
  commits are ahead of the same branch; no reset, clean, rebase or force push
  was used. The matrix/security inventory and this handoff update are the
  documentation commit still to make before pushing.
- SC-009 now has migration 59 for bounded (20 newest) named checkpoints scoped
  by User/Project/Cue. The Cue Studio panel lists timestamps, previews the full
  Draft, and restores only into the open editor after explicit operator
  action; Cue save/publish remains separate. API checks Cue→Project scope,
  edit grants and live session at commit/response; records are private and
  deleted when the Cue is deleted. The production ACL assembly is unchanged.
- Verification: Node suite 60/60; `go test ./internal/store -count=1`,
  `go test ./internal/db -count=1`, `go test ./internal/httpapi -count=1`
  pass. Focused Store and HTTP API Race checks, `go vet ./internal/store
  ./internal/httpapi`, changed JavaScript syntax, route-inventory and schema
  tests, and `git diff --check` pass. The full HTTP API package took about 64s.
- The earlier exact pushed-head Core run `43553686731` on `ce927d6` completed
  successfully at 04:24 UTC: Go 1.26/1.27 tests, Browser Cue Delivery, Vet,
  Go 1.26 Race and Linux ARM64 product builds. Companion `43553686720` and
  Camera Relay `43553686712` passed on the same SHA. Before pushing, re-read
  `git ls-remote`; push normally only if `integration/stagecore-rc` is still
  exactly `ce927d6`. Then wait for the full new exact-head CI before the next
  push.
- Remaining SC-009 software gap: Project-wide Undo/Redo for Cue
  create/delete/duplicate/reorder and checkpoint recovery before a new Cue's
  first save. Start with code/API behavior and regression design; keep all
  outputs mocked. Pi 4 full-engine timing/CPU/RAM remains unqualified because
  the strict SSH handshake still failed before host identity. No real devices,
  production Hub/Relay/data/Vault or production ACL were touched.

## Resume update — 2026-11-14 04:35 UTC · pushed checkpoint workflow

- Local and remote `integration/stagecore-rc` HEAD now match at
  `a3b6f5d7ec009b15fb878e8da995356cee18c939`; eight commits since `ce927d6`
  were pushed normally to the existing Draft PR #534. The PR description and
  completion matrix reflect the current head.
- Exact-head CI is running: Core `43557284478`, Companion Core `43557284414`,
  and Camera Relay `43557284429`. Do not push another batch until all finish.
  Their parent runs on `ce927d6` passed; local full Store, DB, HTTP API, Node,
  focused Race and Vet checks for this batch passed before the push.
- Next SC-009 task is durable Project-level Undo/Redo across Cue create/edit/
  delete/duplicate/reorder, including linked Mixed Cues, with atomic Draft-only
  state restore and immutable published/runtime state. Existing per-dialog
  Mixed Action Undo/Redo and named Cue checkpoints are already covered; do not
  repeat them. Start regression-first and inspect revision/concurrency
  semantics before choosing a history representation.
- Pi performance remains pending after strict qualification-host SSH failed
  before identity. Production Hub/Relay/data/Vault, real devices, firmware and
  production Project ACL remain untouched/disabled.

## Resume update — 2026-11-14 04:41 UTC · editor Undo/Redo shortcuts

- Remote PR #534 remains at `a3b6f5d7ec009b15fb878e8da995356cee18c939`.
  Local HEAD is `fdfcf53ea14ef09ad05cdbae62d710291f9e338b`, two commits ahead:
  `f2e2359` refreshed the post-push checkpoint and `fdfcf53` adds Cue editor
  Cmd/Ctrl+Z and Redo keyboard shortcuts. No push was made during exact-head
  CI.
- The keyboard handler uses physical `KeyZ`/`KeyY` codes for Arabic layouts,
  supports Shift+Z/Ctrl+Y Redo, and leaves input/textarea/select/contenteditable
  native history untouched. Buttons expose `aria-keyshortcuts`.
- Focused `tests/lighting_preview.test.js` passes 14/14; full Node suite passes
  61/61; JavaScript syntax and `git diff --check` pass. No Go code changed in
  this follow-on.
- Exact remote CI: Camera Relay `43557284429` and Companion Core `43557284414`
  passed on `a3b6f5d`. Core `43557284478` Go 1.27 passed; Go 1.26 Race remains
  in progress. Wait for Core completion before pushing the two local commits;
  re-read the remote first.
- Next larger SC-009 gap remains Project-wide atomic Draft Undo/Redo across Cue
  creation, edit, deletion, duplication and ordering. Inspect revision,
  concurrent-editor and publish semantics, and add a red-first regression
  before implementing. Production services, data, Vault, physical devices and
  production ACL remain untouched/disabled; Pi runtime qualification remains
  pending.

## Resume update — 2026-11-14 04:44 UTC · new Cue recovery evidence

- Remote HEAD is still `a3b6f5d7ec009b15fb878e8da995356cee18c939`; local HEAD is
  `f47f88e0ce4afd0958ea68c9216b3b9abbc33d1b`, four commits ahead including
  checkpoint docs, Undo/Redo shortcuts and their recovery regression. The
  matrix update is pending a documentation commit. Nothing has been pushed
  since `a3b6f5d`.
- `tests/cue_recovery.test.js` now proves the existing Project/User recovery
  handles a new Cue before it has a persistent Cue ID, both in local storage
  and in Hub recovery. Named checkpoints stay hidden until the Cue exists.
  This closes that SC-009 acceptance gap without changing runtime code.
- Node suite passes 62/62; focused Cue recovery 11/11 and editor history
  14/14 pass. JavaScript syntax and `git diff --check` pass.
- On remote `a3b6f5d`, Camera Relay `43557284429` and Companion Core
  `43557284414` pass; Go 1.27 in Core `43557284478` passes. Go 1.26 Race is
  still running. Wait for the exact Core run to finish before pushing local
  commits; re-read the remote first.
- Project-wide Draft Undo/Redo remains the main SC-009 gap. Published Snapshots
  and active runtime must remain immutable. Pi performance remains pending;
  production services/data, Vault, devices and production ACL remain untouched
  or disabled.

## Resume update — 2026-11-14 04:46 UTC · local documentation checkpoint

- Local HEAD is `54f26edf3bf9632827eb171e93f918f58de1a43b`, five commits ahead
  of remote `a3b6f5d7ec009b15fb878e8da995356cee18c939`; the working tree is
  clean. Local commits include checkpoint status-doc updates, editor shortcuts,
  and recovery tests for new unsaved Cues.
- Exact-head Core `43557284478` remains in Go 1.26 Race; Go 1.27 is green.
  Companion and Camera Relay runs `43557284414` and `43557284429` are green.
  Do not push until Core completes and the remote branch still matches `a3b6f5d`.
- Local UI evidence remains Node 62/62 with focused recovery 11/11 and history
  14/14. Next SC-009 feature is project-wide Draft history; implementation has
  not started. Production and hardware boundaries remain unchanged.

## Resume update — 2026-11-14 05:10 UTC · follow-on pushed

- Local code and remote PR #534 HEAD are `76f9a9f9b57d89a40b8f1938d098ae25f9ad2d08`.
  Six commits since `a3b6f5d` were pushed normally after all three exact-head
  parent runs passed. The PR remains open/draft; its description is current.
- Core `43560053370` and Companion Core `43560053384` are in progress on
  `76f9a9f`; Camera Relay `43560053362` passed. Do not push another batch
  until Core and Companion complete. Read the exact run results and confirm the
  remote SHA before any subsequent push.
- The pushed follow-on adds physical-key Cmd/Ctrl+Z/Redo in the Cue editor,
  preserves native text-field history, and tests User/Project recovery for an
  unsaved new Cue without an ID. Node 62/62; prior feature full Store, DB and
  HTTP API, focused Race, Vet and syntax checks passed. SC-009's remaining
  software gap is project-wide Draft Undo/Redo across Cue create/edit/delete/
 duplicate/reorder; no implementation has started.
- Related PRs remain at their verified heads with green software checks. Pi 4
  runtime performance remains pending host access; production Hub/Relay/data/
  Vault, devices/firmware and production Project ACL remain untouched/disabled.

## Resume update — 2026-11-14 05:43 UTC · exact-head CI green and issue review

- Remote branch / Draft PR #534 remains at
  `76f9a9f9b57d89a40b8f1938d098ae25f9ad2d08`. The exact-head runs are now all
  complete and successful: Core `43560053370` (Go 1.26/1.27, Race, Linux
  ARM64 CGo-free product builds), Companion Core `43560053384`, and Camera
  Relay `43560053362`.
- Local HEAD before this documentation update was `df52634`; local commits
  `2170e37` and `df52634` are docs-only and have not been pushed. This update
  refreshes the matrix and records the newly completed CI / open-issue review.
  Re-read `git rev-parse HEAD` after committing; then verify `git ls-remote`
  still reports `76f9a9f` before a normal push to the existing branch.
- The matrix now classifies open non-SC backlog items. #455 remains a Core
  software gap: concurrent GO requires selection/advance fencing, multiple
  tracked executions, STOP/Blackout over all active work and output-conflict
  rules; do not remove the active-run gate in isolation. #446's TLS bootstrap
  software fix is already in Foundation commit
  `12be83968efff876d8cb239b79368b3dafbf044b`, pinned by open Drafts StageLaser
  #56, DMX #39 and Camera #20; their software/no-actuation checks pass, with
  device trust and attended pairing still unqualified. #467 explicitly asks
  for no additional preview UI. #483/#459/#429/#307 remain deployment or
  physical-acceptance gates.
- Linked PR heads were refreshed: TabletPlayer #36
  `91613da1c8e0455a501c7be4b6df8ac1ce73a6bc`; DMX #39
  `3ad774988b2f78eb385f1046f7668a7f29049ab1`; Camera #20
  `455b066c6520e1c8a576d26dd2afd4b377652dde`; StageLaser #56
  `b14a5789132549f2724f77b342bbe4c6b0fc880c`; StageLaser #57
  `c4ff10b37dc2fbfd3ebe31e55f0f8d4869d3dc54`. Their recorded software
  checks are green; no linked repository was changed.
- Docs validation: `git diff --check` passed. No code changed in this checkpoint;
  earlier current-head evidence remains Node 62/62, full Store/DB/HTTP API
  tests, focused Race/Vet, JS syntax and diff checks. No reason to rerun them
  for documentation-only changes.
- Next software task remains SC-009 Project-wide Draft Undo/Redo. Before code,
  define history/concurrency fences against the revision fork and publish path,
  then add red-first Store/API regressions for mixed linked Cue create/edit/
  delete/duplicate/reorder, identity/link preservation, cross-editor conflict,
  restart recovery and immutable Published Snapshot. After pushing the docs,
  wait for exact-head CI before any later push.
- Pi 4 P95/P99/Jitter runtime qualification remains pending: strict read-only
  SSH to `stagecore-qualification` failed before host identity with
  `websocket: bad handshake`; no remote command ran. Mac/ARM64 CI is not Pi
  runtime evidence. Production Hub/Relay/data/Vault, real devices and outputs,
  and production SC-024 remain untouched/disabled. No force push or merge.

## Resume update — 2026-11-14 06:22 UTC · Project Cue Draft Undo/Redo

- The current branch is `integration/stagecore-rc`; PR #534 remains open and
  draft. Local code commits are `dffe840` (persistent Store history and
  migration 60) and `2715a38` (scoped HTTP API and Draft-only Cue Studio
  controls). The remote is still `14887b6699709a71f61fd86a68adc38577abb3a2`;
  these feature commits and the refreshed matrix/SC-024 qualification notes
  are not pushed yet.
- Current pushed-head CI finished successfully on exact SHA `14887b6`:
  Core `43563053431`, Companion `43563053405`, Camera Relay `43563053512`.
  Confirm the remote SHA still matches before normally pushing the completed
  feature batch to the same PR. Do not force-push or merge.
- Migration 60 stores up to 50 actor-scoped Draft operations per revision.
  Cue create/edit/delete/duplicate/reorder, single Cue Assistant proposals and
  Cue mutations in atomic Assistant batches now record identity-preserving
  deltas. Undo/Redo restores Mixed/linked Cue fields, Actions, order and named
  checkpoints removed by Cue deletion. Hash/cursor checks reject stale edits;
  SHOW configuration lock remains enforced. Removal conflicts when external
  Routes/rehearsal records reference the Cue. History covers Cue/Action Draft
  changes, not unrelated Project objects such as Routes.
- Verification on the current code: `go test ./internal/db ./internal/store
  -count=1`, `go test ./internal/httpapi -count=1`, focused Store Race,
  `go vet ./internal/store ./internal/httpapi`, and
  `go test ./internal/operatorweb -count=1` passed. `node --test tests/*.test.js`
  passed 64/64; JS syntax and `git diff --check` passed. Core's next full CI
  run must validate the commits after push.
- The opened linked repository PR heads remain those recorded in the previous
  checkpoint; no linked repo was modified. Pi 4 runtime P95/P99/Jitter remains
  pending because strict SSH to `stagecore-qualification` failed before host
  identity (`websocket: bad handshake`); no remote command ran. Do not infer
  Pi runtime qualification from Mac or cross-build results.
- Production Hub/Camera Relay/data/Vault, physical devices and outputs remain
  untouched. Project ACL/SC-024 remains disabled in production.
- First next action after push: watch Core/Companion/Camera CI on the new exact
  head and handle any failure before another push. Then continue the highest
  open software gaps in the matrix; revisit #455 concurrent GO only with a
  complete tracked-execution/STOP/Blackout/output-conflict design and simulated
  race/fault coverage. Hardware and release gates do not block software work.

## Resume update — 2026-11-14 06:24 UTC · history batch pushed

- The three commits `dffe840`, `2715a38` and `05c8c96` are now pushed normally
  to `integration/stagecore-rc`; local and remote HEAD are
  `05c8c961325d38b8b39e069d807eee10d351f43c`. Draft PR #534 remains open.
- The prior exact-head runs on `14887b6` were all green before the push. New
  exact-head runs on `05c8c96` are Core `43566116341` (queued), Companion Core
  `43566116367` (queued), and Camera Relay `43566116321` (in progress). Do not
  push again until all three finish and confirm remote SHA first.
- The feature tests on the pushed code all passed locally: DB/Store, full HTTP
  API, Store Cue-history Race, Store/HTTP Vet, Operator Web, Node 64/64, JS
  syntax and diff check. The new Core CI remains the first complete workflow
  validation of this feature.
- PR description still needs its top checkpoint refreshed to the current HEAD
  and run IDs while preserving its older qualification history. The existing
  linked firmware/tablet PRs remain unchanged. Pi runtime qualification,
  production protections, and next software gaps are as recorded above.

## Resume update — 2026-11-14 06:29 UTC · concurrent GO qualification

- The newly pushed head remains `05c8c961325d38b8b39e069d807eee10d351f43c`.
  Its exact runs: Core `43566116341` is still in progress; Companion Core
  `43566116367` and Camera Relay `43566116321` are green. Do not push until the
  current Core run completes.
- Re-read open Issue #455 against the actual integration source. Concurrent GO
  is partly implemented: RuntimeControl tracks active GO requests per Session,
  Engine serializes selection/current-Cue advancement and releases before Cue
  Actions, STOP CUE cancels all active runs, and Session STOP/Blackout snapshot
  all active executions. Existing tests cover overlapping delayed GO, three
  simultaneous distinct selections, late completion cursor fencing and STOP
  of every delayed Cue. The matrix has been corrected from its stale single-run
  claim.
- Added a local simulated regression for Emergency Blackout with two Actions
  blocked in a controlled executor: it verifies the durable latch and safe
  state dispatch while both Cue runs remain active, then verifies both are
  canceled before Blackout returns. This test and its matrix update are local
  and unpushed at this checkpoint.
- Verified `go test ./internal/runtimecontrol -count=1`, focused concurrent-GO,
  STOP and Blackout tests under Race, `go vet ./internal/runtimecontrol`, and
  `git diff --check`. Remaining #455 software gaps are the synchronous GO
  response through action completion, lack of an explicit same-output conflict
  policy, no active-execution management surface, and safe-but-unresolved RUNNING
  row behavior after restart. Do not describe the issue as complete.
- Keep production and all real outputs untouched. The next action is to commit
  the regression and matrix evidence locally, then wait for Core run
  `43566116341`; any later push must verify all three exact-head checks and the
  remote SHA.

## Resume update — 2026-11-14 06:38 UTC · overlapping execution surface

- Local HEAD is `b2a1a4d3c48de3658894f7f39d6ffe00d9d2fa39`; remote remains
  `05c8c961325d38b8b39e069d807eee10d351f43c`. Four commits are ahead of
  `origin/integration/stagecore-rc`: `f3ec183`, `8d9d427`, `0f411b9`, and
  `b2a1a4d`. Worktree is clean. Keep these local until Core CI
  `43566116341` finishes; Companion `43566116367` and Camera Relay
  `43566116321` already passed on the remote SHA.
- #455 now has a simulated Emergency Blackout ordering regression for two
  concurrent active Cues, plus the Runtime API/UI exposes every currently
  RUNNING execution and its Cue/Execution IDs while preserving Latest Result.
  Runtime and Cue Check surfaces now say STOP ALL CUES and describe the
  all-execution cancellation semantics. The matrix no longer lists missing
  active-execution visibility as an open gap.
- Verification on these local changes: focused `internal/httpapi` Runtime
  status and GO/JUMP/STOP tests passed; focused HTTP API Race test passed;
  `go vet ./internal/httpapi ./internal/operatorweb` passed; the
  `internal/operatorweb` contract suite passed after updating its stale STOP
  label assertion; all JavaScript tests passed (67/67); app and localization
  JavaScript syntax checks and `git diff --check` passed. The prior local
  #455 batch also passed full `go test ./internal/runtimecontrol`, its
  concurrent STOP/Blackout Race tests, and runtimecontrol Vet.
- No runtime optimization was introduced from the M3 data. Raspberry Pi 4
  execution P95/P99/Jitter, CPU/RAM, burst and sustained qualification remain
  open because the strict qualification SSH connection failed before host
  identity; no remote command ran. Production services/data, physical outputs,
  firmware and production ACL remain untouched.
- Remaining #455 software gaps are the synchronous GO response through
  completion, a specified conflict policy for concurrent Cues that share an
  exclusive output, and restart handling for persisted RUNNING rows. Do not
  weaken the safe fail-closed restart behavior or invent output arbitration
  without a tested product rule.
- Next action: wait for the current exact-head CI; if green, verify remote SHA,
  push these five commits normally to this same branch, update PR #534's
  checkpoint, and watch all exact-head CI. Then continue an independent
  software gap. Pi/runtime qualification is Hardware Qualification Pending;
  production release and SC-024 activation remain separately gated.

## Resume update — 2026-11-14 06:54 UTC · full-suite and fault-fixture follow-up

- Local HEAD is `303228a1936d10ce56eaf70794e902d3a336adca`; remote stays at
  `05c8c961325d38b8b39e069d807eee10d351f43c`. Six commits are ahead locally:
  `f3ec183`, `8d9d427`, `0f411b9`, `b2a1a4d`, `ff67499`, `303228a`.
  Worktree is clean. Do not push while exact-head Core CI `43566116341` is
  still in its Go 1.26 Race tests; Go 1.27, Companion and Camera Relay checks
  on `05c8c96` have passed.
- Ran the full repository suite on this Mac with a short temp root:
  `TMPDIR=/tmp go test ./... -count=1` passed. The three Linux offline-media
  verification tests now skip explicitly on Darwin because their subject is
  the Linux-only installer; Linux CI continues to execute them. The short
  TMPDIR avoids macOS Unix-domain socket path limits in StageAudio tests.
- Fixed the plugin fault fixture's `hang-once` mode to sleep rather than trigger
  Go's all-goroutines-asleep process exit. The cancellation integration test
  now waits until the fake records that it entered the faulting execution
  before cancelling, avoiding a timing race during process startup. Full
  `internal/pluginhost` tests and its Race suite pass.
- Updated the existing embedded Operator UX contract for STOP ALL CUES. The
  isolated Runtime status regression and HTTP API test pass, Operator Web
  tests pass, all Node tests pass (67/67), `go vet` on HTTP API/Operator Web,
  pluginhost and deployment passes, and JavaScript syntax/diff checks pass.
- Strict SSH retry to `stagecore-qualification` again failed with
  `websocket: bad handshake` before host identity; no Pi command ran. Pi ARM64
  P95/P99/Jitter and CPU/RAM gates remain unqualified. Production and real
  outputs remain untouched; SC-024 stays disabled in production.
- Next: finish waiting for `43566116341`; verify the remote branch still equals
  `05c8c96`, push the six local commits normally to the existing branch only
  after all exact-head workflows pass, update PR #534's checkpoint, and watch
  CI on the new exact SHA. Continue software gaps while Pi access is blocked.

## Resume update — 2026-11-14 07:00 UTC · overlap batch pushed

- The seven-commit batch was pushed normally to the existing branch and PR:
  `origin/integration/stagecore-rc` and Draft PR #534 now point to
  `abe7ad8017744283acab76bfa4f3b76ad4797f52`. The local source worktree was
  clean immediately after the push; this checkpoint update is the only local
  follow-up being prepared.
- The prior exact-head checks for `05c8c96` all passed, including the 34m29s
  Go 1.26 Race + Linux ARM64 build job. New exact-head runs for `abe7ad8`:
  Core `43568904863` and Companion Core `43568904858` are in progress;
  Camera Relay `43568904853` passed. Do not push the pending checkpoint commit
  or another feature until both in-progress runs finish and the branch SHA is
  confirmed.
- PR #534's opening summary now names the `abe7ad8` head, seven-commit scope,
  tests, current run IDs, Pi blocker and production protections; its older
  qualification archive is preserved.
- The local full Go suite passed with `TMPDIR=/tmp`; the three Linux-only
  offline-media verification tests are skipped on Darwin and remain exercised
  by Linux CI. `go vet ./...`, `go test -race ./internal/pluginhost`, Node
  67/67, focused HTTP API/Operator Web checks, JS syntax, and diff checks passed.
- Pi 4 qualification still cannot start: the strict alias fails before host
  identity (`websocket: bad handshake`). No command ran remotely. Keep this
  classified as Hardware Qualification Pending; production services/data,
  physical outputs, firmware and production ACL remain untouched.
- Resume by checking Core `43568904863` and Companion `43568904858`. If green,
  verify Remote HEAD, push the local checkpoint commit normally, then watch the
  exact-head CI for that new HEAD before another push. Continue the next
  software gap while respecting these CI gates.

## Resume update — 2026-11-14 07:05 UTC · Draft history backup restore

- While CI runs on pushed HEAD `abe7ad8017744283acab76bfa4f3b76ad4797f52`,
  added a regression to `internal/backup`: create a Cue Draft Undo entry,
  perform the isolated State Backup/Restore flow, reopen the restored SQLite
  database, verify Undo is still available, then Undo the Cue and verify its
  identity is absent and Redo is available. This uses only temporary test
  DataRoots; no production database or Vault was opened.
- The focused restore test, full `internal/backup` package, focused backup Race,
  and `go vet ./internal/backup` passed. The SC-009 matrix now distinguishes
  tested State Backup/Restore of Draft history from remaining update rollback
  and real-browser qualification.
- At this checkpoint Core CI `43568904863` remains in its Go 1.26 Race step;
  Go 1.27 has passed. Companion `43568904858` and Camera Relay `43568904853`
  have passed on `abe7ad8`. Keep local backup test/matrix/log changes unpushed
  until Core finishes. Current Remote SHA remains `abe7ad8`.
- Next: finish exact-head CI for `abe7ad8`; then verify Remote SHA and push the
  local handoff plus SC-009 regression in a normal push to the same PR. Watch
  all new exact-head CI before another push. Continue software work; Pi ARM64
  measurements remain blocked by the qualification SSH handshake, production
  services/data/outputs and ACL remain untouched.

## Resume update — 2026-11-14 07:09 UTC · keyboard focus and Cue labels

- Local feature commit `1cdae8609a3e752cb2b5386c00f7e29ed5f8db81` adds a global
  visible keyboard focus outline for Operator controls and a structural test
  that Cue display-label text remains present alongside supplemental color
  styling. SC-005 and SC-018 now record that evidence while keeping browser
  contrast, RTL, responsive and keyboard acceptance open.
- Local HEAD is `1cdae86`; remote and Draft PR #534 remain at
  `abe7ad8017744283acab76bfa4f3b76ad4797f52`. Three commits are ahead locally:
  `4c68f94`, `4c15d95`, and `1cdae86`. The working tree was clean after the
  feature commit.
- Focused accessibility Node tests passed 2/2; all `tests/*.test.js` passed
  69/69; `go test ./internal/operatorweb -count=1`, `go vet
  ./internal/operatorweb`, JavaScript syntax checks, and `git diff --check`
  passed. The earlier SC-009 backup/restore full package, focused Race and Vet
  checks also passed.
- Exact-head Core CI `43568904863` is still in progress on remote `abe7ad8`:
  Go 1.27 and Go 1.26 test/Vet steps passed, Go 1.26 Race is running, and
  Linux ARM64 product builds are pending. Companion `43568904858` and Camera
  Relay `43568904853` passed. Do not push the local commits until Core CI is
  complete and green, then recheck the remote SHA and push normally to the
  existing branch/PR.
- Pi 4 qualification remains blocked before host identity by the strict SSH
  alias handshake; no Pi command ran. No physical output, production service,
  production data/Vault, or production ACL was touched. SC-024 stays disabled
  in production.
- First task after the CI gate: push the four local commits including this
  checkpoint to PR #534, refresh its opening status, and follow exact-head CI.
  Then resume the next matrix gap, starting with SC-007 Draft-to-published
  lighting reconciliation, using isolated tests/simulation only.

## Resume update — 2026-11-14 07:16 UTC · lighting authoring validation

- The prior SC-007 checkpoint task was completed locally in
  `2e1641c185ccfab26bc811f30894a32f4c0a7b9e`. Lighting Cue authoring now
  rejects blank/non-finite and out-of-range levels, stale logical aliases,
  and non-integer/out-of-range Fade/Blackout durations instead of silently
  coercing them. Stale alias/device/capability/configuration API errors now
  show English or Arabic operator remedies. No output is dispatched by these
  validation paths.
- Existing Preflight Draft-versus-Snapshot fingerprinting was verified with
  `TestPreflightWarnsWhenDraftLightingDiffersFromPublishedSnapshot`: a Draft
  mismatch is an advisory naming the immutable Snapshot; runtime remains
  bound to that Snapshot. SC-007 matrix evidence now describes the remaining
  real-browser and route-level regression gaps accurately.
- `node --test tests/lighting_preview.test.js` passed 15/15; the full
  `tests/*.test.js` suite passed 70/70. Operator Web package tests and Vet,
  the focused Preflight mismatch test, HTTP lighting alias/grouping tests,
  JavaScript syntax and diff checks passed.
- Local HEAD is `2e1641c`; remote and Draft PR #534 remain at
  `abe7ad8017744283acab76bfa4f3b76ad4797f52`. Five local commits are ahead:
  `4c68f94`, `4c15d95`, `1cdae86`, `bc14580`, and `2e1641c`; the handoff update
  in this entry is being committed separately. The worktree was clean after
  the SC-007 feature commit.
- Core `43568904863` is still running on exact remote `abe7ad8`: Go 1.27 and
  Go 1.26 Test/Vet passed; Go 1.26 Race remains in progress and Linux ARM64
  product builds are pending. Companion `43568904858` and Camera Relay
  `43568904853` passed. Wait for Core to finish, verify that remote SHA is
  unchanged, then push the local batch normally to this same PR and follow CI
  on the new exact head. Do not cancel the current CI.
- Pi 4 access still fails before host identity through the strict qualification
  SSH alias; no Pi command ran. No physical output, production service,
  production data/Vault, firmware, or production ACL was touched. SC-024 stays
  disabled in production.
- Next after the CI gate: update PR #534's opening status and follow exact-head
  CI for the push; then continue SC-008's real-browser authoring/round-trip
  acceptance with simulated targets and preserve the Mixed/linked Cue priority.

## Resume update — 2026-11-14 07:21 UTC · lighting API fault coverage

- Added `TestLightingCueActionRejectsInvalidValuesAliasesAndDeviceAssignments`
  in local commit `4bb997d03b98f11ae4254b086667c241ff516389`. The HTTP
  simulation now proves out-of-range levels, stale aliases, a node disabled
  after binding, and missing capability are refused with the expected error
  before target-alias creation. The disabled-node case creates a valid binding
  first, then changes the isolated test database to simulate a mid-authoring
  state change.
- The focused lighting HTTP tests passed normally and under `-race`; `go vet
  ./internal/httpapi` and `git diff --check` passed. The SC-007 matrix now
  records the client and API evidence and keeps real-browser checks open.
- Local feature HEAD is `4bb997d`; remote remains `abe7ad8017744283acab76bfa4f3b76ad4797f52`.
  Seven commits are ahead locally: `4c68f94`, `4c15d95`, `1cdae86`, `bc14580`,
  `2e1641c`, `32a823e`, and `4bb997d`. The worktree was clean after the feature
  commit; this checkpoint is the following documentation commit.
- At 07:21 UTC, Core run `43568904863` was still in Go 1.26 Race, with Linux
  ARM64 product builds pending; the run is pinned to pushed HEAD `abe7ad8`.
  Companion `43568904858` and Camera Relay `43568904853` passed on that exact
  SHA. No push until Core completes successfully. Afterward, verify the remote
  SHA and push all local work normally to existing PR #534, refresh the PR
  checkpoint, and follow CI for the resulting exact head.
- Pi 4 runtime performance remains unmeasured because strict SSH fails before
  host identity. No production services/data, Vault originals, physical output,
  firmware or production ACL were touched. ACL remains disabled in production.
- Next software task after the CI/push gate: SC-008 real-browser Cue authoring
  and Mixed/linked round-trip acceptance using a local isolated app and
  simulated targets; continue the broader performance and integration gaps.

## Resume update — 2026-11-14 07:27 UTC · CI gate complete

- Exact-head CI completed successfully on remote `abe7ad8017744283acab76bfa4f3b76ad4797f52`:
  Core run `43568904863` passed, including Go 1.26 Race and Linux ARM64
  CGo-free product builds; Companion run `43568904858` and Camera Relay run
  `43568904853` also passed. ARM64 product build success is not a Pi runtime
  performance measurement.
- `git fetch origin integration/stagecore-rc` confirmed the remote SHA is
  still `abe7ad8017744283acab76bfa4f3b76ad4797f52`. Draft PR #534 remains
  OPEN, targets `main`, and points to the same SHA. Local feature HEAD before
  this update is `a52a4f35537058ca3e6791de396ca4c4f8c30899`, eight commits
  ahead of remote: `4c68f94`, `4c15d95`, `1cdae86`, `bc14580`, `2e1641c`,
  `32a823e`, `4bb997d`, and `a52a4f3`. This CI/browser status update is the
  ninth local commit in the pending normal push.
- The isolated local browser review started Hub at `127.0.0.1:17840` with
  temporary DataRoot `/tmp/stagecore-ui.ixe3LJ`, claimed only a temporary test
  Owner, and created a temporary test Project. Playwright browser interaction
  then timed out while the Mac was locked. No Cue was saved and no hardware or
  external output was contacted. The local Hub process has been stopped; the
  temporary root is preserved. SC-008 real-browser acceptance remains open.
- Push the nine local commits normally to the existing
  `integration/stagecore-rc` branch and PR #534 now that the exact remote CI is
  green. Refresh the PR opening checkpoint with the new SHA, commit scope and
  run IDs, then wait for Core/Companion/Camera CI on that exact new SHA before
  any further push.
- Pi 4 live performance qualification remains blocked by the strict SSH
  handshake failure before host identity. Keep all production services,
  original data/Vault, physical outputs, firmware and SC-024 production ACL
  unchanged; production ACL stays disabled.
- Resume after the push gate with SC-008 browser-model/interaction gaps and
  Mixed/linked Cue behavior, then continue the remaining software matrix. Do
  not report Software Complete until Pi runtime performance and the other
  explicitly open qualifications have evidence or precise blocker status.

## Resume update — 2026-11-14 07:29 UTC · nine-commit PR update

- The nine-commit batch was pushed normally to the existing branch. Local and
  remote `integration/stagecore-rc` and Draft PR #534 now point to
  `bc31914bc944ce6735a6c10224a57d42b2bc00cb`; the working tree is clean.
  PR #534 remains OPEN/DRAFT against `main`; its opening summary now records
  the new commit scope, local evidence, Pi/browser status and run IDs, while
  preserving the earlier qualification archive.
- New exact-head runs: Core CI `43571216920` is in progress; Companion Core
  `43571216881` and Camera Relay `43571216851` have passed. No push may
  interrupt Core on this SHA. Wait for it to complete, diagnose any failure,
  and record the exact result before another push.
- The pushed local changes include the SC-009 Draft Undo-after-Backup/Restore
  regression; SC-005/SC-018 visible focus outline and label/color structural
  test; and SC-007 UI validation plus server-side rejection coverage. On the
  prior head, the full Go suite and `go vet ./...` passed. This batch passed
  Node 70/70, focused Operator Web tests/Vet, Preflight Snapshot mismatch,
  lighting HTTP normal/Race tests, HTTP API Vet, JavaScript syntax and diff
  checks. Do not describe the prior full-suite run as a full-suite run on
  `bc31914`.
- The isolated local browser run reached a temporary Owner and Project in
  `/tmp/stagecore-ui.ixe3LJ` but timed out before a Cue was saved while the Mac
  was locked. The loopback Hub was stopped; no physical/device or production
  state was contacted. The test-only DataRoot remains preserved. SC-008
  browser acceptance is still open.
- Pi 4 runtime benchmarks remain blocked by strict SSH failure before host
  identity. ARM64 product-build CI does not qualify Pi latency, jitter, CPU or
  RAM. Production services, original data/Vault, real outputs and firmware
  remain untouched; production Project ACL remains disabled.
- Resume by watching Core `43571216920` on `bc31914`. After it is green,
  continue SC-008 and Mixed/linked Cue software acceptance locally without
  pushing until any later exact-head CI gate has completed; then proceed to
  the next matrix software gap.

## Resume update — 2026-11-14 07:38 UTC · Linux USB-DMX write faults

- Remote remains `bc31914bc944ce6735a6c10224a57d42b2bc00cb`; this work is local
  on `integration/stagecore-rc` in commits `508d619` and `a785dfa`, after the
  existing local handoff commit `06a9c6a`. The working tree is clean after the
  checkpoint commit. Do not push until Core run `43571216920` on `bc31914`
  completes; its Go 1.27 job passed and its Go 1.26 race job was still running
  at this checkpoint. Companion/Camera runs for `bc31914` passed.
- SC-025 Linux `WritePacket` now exposes only an internal writer/poller seam;
  production defaults still call `unix.Write` and `unix.Poll`. Deterministic
  Linux-tagged tests cover short-write byte ordering, EAGAIN/EINTR, zero
  progress, HUP/ERR/NVAL, poll failure, and cancellation after a partial
  frame. Matrix row SC-025 records the coverage and remaining qualifications.
- Local validation passed: `go test ./internal/usbdmx -count=1`,
  `go test -race ./internal/usbdmx -count=1`, `go vet ./internal/usbdmx`,
  Linux ARM64 `go vet`, and Linux ARM64/AMD64 test-binary cross-compilation.
  The macOS test executions do not run Linux-tagged port tests. The new exact
  PR CI must execute those tests on Linux before treating runtime behavior as
  verified; cross-compilation is not runtime evidence.
- A read-only attempt to `stagecore-qualification` again failed with
  `websocket: bad handshake` before opening a shell; no Pi command ran. Pi 4
  engine/burst/sustained performance and SC-025 ARM64 USB qualification remain
  open. Docker is installed locally but its daemon is unavailable, and no
  qemu runner is installed.
- Next: wait for Core `43571216920` on `bc31914`. If it passes, push the
  accumulated local commits normally to the existing PR #534 branch, then
  inspect the new exact-head Core Linux test/race/build result plus Companion
  and Camera workflows. If any fail, diagnose before another change. Continue
  with the highest proven software gap; keep Pi performance explicitly
  unqualified until the isolated qualification alias becomes reachable.
- Safety state unchanged: no production service/data/Vault access, physical
  output, firmware action, or production Project ACL activation.

## CI gate update — 2026-11-14 07:57 UTC · prior remote head cleared

- Core CI `43571216920` completed successfully on remote HEAD
  `bc31914bc944ce6735a6c10224a57d42b2bc00cb`: Go 1.27 passed in 3m06s and
  Go 1.26 passed Test, Browser Cue Delivery, Vet, Race and Linux ARM64 product
  builds in 29m03s. Companion Core `43571216881` and Camera Relay
  `43571216851` had already passed on the same exact SHA.
- The local SC-025 batch through `dc6fd91` has focused host validation and
  Linux ARM64/AMD64 compile/Vet evidence recorded above. Push it normally to
  the existing branch and PR #534 now that the old-head CI gate is clear; wait
  for every new exact-head workflow, particularly Linux runtime execution of
  `internal/usbdmx` tests, before starting the next push batch.

## Resume update — 2026-11-14 08:10 UTC · USB-DMX request bounds

- Remote PR head is `7b82afe6026dcadbe8e11fd64cb7656950cbae2b`; local follow-up
  commits `877ac29` and `03b37b7` add only tests and the SC-025 matrix update.
  The worktree is clean. Keep these local until the exact-head Core run below
  completes; do not interrupt it with a push.
- Manual USB-DMX tests now verify invalid channel count, generic timed output
  duration, Hub timed duration and continuous lease bounds are rejected before
  opening a port. Existing protocol tests cover exact full-universe framing;
  existing lease tests cover expiry, STOP, blackout, authority loss, failed
  refresh and failed final zero. The matrix retains refresh timing-under-load,
  disconnect/reopen and real port identity as outstanding qualification.
- Local checks passed: `go test ./internal/usbdmx -count=1`,
  `go test -race ./internal/usbdmx -count=1`, `go vet ./internal/usbdmx`, and
  Linux ARM64/AMD64 test-binary compilation. On pushed head `7b82afe`, Core
  Go 1.27 (`43573536661`) passed, so the Linux-tagged port-fault tests executed
  successfully on the Linux runner. Core Go 1.26 Race is still in progress;
  Companion `43573536683` and Camera Relay `43573536650` passed.
- Pi performance remains unqualified. The named qualification SSH alias still
  fails before session establishment, and prior read-only evidence shows
  production-like Hub/Camera services active on that host. Do not run the Pi
  benchmark there; no Pi or real output was contacted during this batch.
- Next: wait for Core `43573536661` to finish on `7b82afe`. On success, record
  the exact result and push the local USB-DMX bounds test + matrix/handoff
  commits normally to PR #534. Then follow all new exact-head workflows. On
  failure, diagnose the failed job before pushing.

## CI gate update — 2026-11-14 08:19 UTC · USB-DMX fault batch passed

- Core CI `43573536661` passed on exact remote head
  `7b82afe6026dcadbe8e11fd64cb7656950cbae2b`: Go 1.27 completed in 3m26s;
  Go 1.26 completed Test, Browser Cue Delivery, Vet, Race and Linux ARM64
  product builds in 21m02s. Companion `43573536683` and Camera Relay
  `43573536650` also passed at that same head.
- Linux Go 1.27 CI executed the Linux-tagged `LinuxPort.WritePacket` tests,
  covering short writes/retries/disconnect/cancellation without opening a
  physical device. The local follow-up commits `877ac29` and `03b37b7` add
  rejection-before-open and inclusive endpoint/zero-cleanup tests; local
  package, Race, Vet and ARM64/AMD64 test compilation all pass. These newer
  tests still need exact-head CI after push.
- Push the current local USB-DMX bounds batch normally to the existing PR #534
  branch now that the preceding exact-head CI is green. Verify remote/PR SHA,
  update PR evidence, and wait for all new Core/Companion/Camera runs before
  another push. The Pi performance blocker and production safety boundaries
  are unchanged.

## Resume update — 2026-11-14 08:23 UTC · refresh interval regression

- Remote PR head is `ead0b48e817e9039594328718a87a4438f27e13e`; local commits
  `a7f8598` and `995249b` add a refresh-interval regression and matrix detail.
  Keep the clean local worktree unpushed until Core `43575317269` completes.
- The injected-device-loss test now records the first refresh write attempt
  and rejects an interval below 200 ms from the authored frame; the production
  ticker target is 250 ms. Focused normal test passed 5 repetitions, focused
  Race passed 3 repetitions, and package Vet passed. This tests the lower
  scheduling bound only; tail jitter/CPU under sustained load remains open.
- On `ead0b48`, Go 1.27 Core passed in 2m23s; Core Go 1.26 was still running
  Test/Race. Companion `43575317263` and Camera Relay `43575317254` passed.
  These CI runs do not contain the new unpushed refresh test.
- Next: wait for exact-head Core success/failure, then push the current local
  refresh interval batch to PR #534 only on success, update PR evidence and
  wait for all workflows on the new head. Do not contact the production-like
  Pi host or run Pi benchmarks until a verified isolated ARM64 target exists.

## CI gate update — 2026-11-14 08:51 UTC · USB-DMX bounds head passed

- Core CI `43575317269` passed on exact remote HEAD
  `ead0b48e817e9039594328718a87a4438f27e13e`: Go 1.27 passed in 2m23s;
  Go 1.26 completed Tests, Browser Cue Delivery, Vet, Race and Linux ARM64
  product builds in 30m40s. Companion `43575317263` and Camera Relay
  `43575317254` passed on that exact head.
- Current local commits `a7f8598` and `995249b` record a deterministic
  continuous-refresh scheduling floor (>=200 ms; configured ticker 250 ms)
  and update the SC-025 matrix. Focused test passed 5 times normally and 3
  times under Race; Vet passed. These commits were not part of `ead0b48` CI.
- Push the local refresh interval batch normally to existing branch/PR #534
  now that the old-head CI is green. Record the new head and run IDs in the PR
  description, then wait for every exact-head workflow before more changes.
  Full refresh tail jitter/CPU measurement under sustained load and isolated
  Pi 4 qualification remain open. No production or real-device operations.

## Resume update — 2026-11-14 08:55 UTC · Linux default syscall pipe test

- Remote head is `11309b63ede84abcea2eca4b99f71fc0e57e79c6`; local commits
  `834c051` and `f1287a0` add a Linux-only isolated-pipe regression and matrix
  note. The worktree is clean; keep this follow-up local until Core
  `43577820286` on `11309b6` completes.
- The new Linux test exercises the public `LinuxPort.WritePacket` default
  `unix.Write` path with an isolated nonblocking pipe, checks the exact 518
  frame bytes, verifies `POLLOUT`, then closes the pipe reader and checks the
  poll fault event. It never discovers or opens USB hardware. Local package
  tests, Race, Vet, Linux ARM64/AMD64 test-binary builds and ARM64 Vet passed;
  the macOS host cannot execute the Linux-tagged test itself.
- On pushed head `11309b6`, Core Go 1.27 passed, Core Go 1.26 Test was still
  running at this checkpoint, Companion `43577820339` passed both Companion
  jobs, and Camera Relay `43577820216` passed. The pipe test is not in those
  runs and requires next-head Linux CI execution.
- Next: wait for exact-head Core on `11309b6`; if green, push the two local
  pipe-test commits normally to PR #534 and wait for exact-head CI before
  another push. Pi performance remains unqualified; do not run workloads on
  the production-like Pi alias or touch production/real outputs.

## Resume update — 2026-11-14 09:04 UTC · close-state regression and current CI gate

- Remote PR #534 remains at `11309b63ede84abcea2eca4b99f71fc0e57e79c6`.
  Local `integration/stagecore-rc` is four commits ahead, ending at
  `677d18d` (`test: verify closed Linux USB-DMX port quarantine`); the
  worktree is clean. This last commit checks idempotent close and rejects a
  packet write after close, updating the SC-025 matrix accordingly.
- Focused package, Race, Vet, and Linux ARM64/AMD64 compile checks for the
  pipe test batch had passed before the last close-state assertions were
  added; the close-state assertions then passed focused package/Race/Vet and
  ARM64/AMD64 test-binary compile checks. The exact Linux-tagged assertions
  still require Linux CI execution.
- Do not push until Core CI `43577820286` finishes on exact remote head
  `11309b6`. At this checkpoint Go 1.27 passed; Go 1.26 Test, Browser Cue
  Delivery and Vet passed; its Race step was running and Linux ARM64 build
  pending. Companion `43577820339` and Camera Relay `43577820216` passed.
- Read-only cross-repository review on 2026-11-14 found current related PRs
  open on their existing heads: TabletPlayer #36 `91613da` (run
  `42719885664` PASS), ESP32 DMX #39 `3ad7749` (six checks, run
  `42759555665` PASS), ESP32 Camera #20 `455b066` (run `42761096513` PASS),
  StageLaser #56 `b14a578` (run `42765890415` PASS), and #57 `c4ff10b` (run
  `38054411190` PASS). No linked-repository files were changed.
- Pi 4 engine/burst/sustained performance remains unqualified. The only
  available host has production Hub/Camera services active alongside a
  separate isolated qualification unit; the remembered SSH qualification
  alias fails before session establishment. Do not run load there or use
  hardware. No production service/data, real device, firmware or ACL setting
  was touched; production ACL remains disabled.
- Next: wait for Core `43577820286`. If green, update this checkpoint and the
  related-component matrix with exact completed run results, then push local
  commits normally to the same branch/PR #534 and follow all exact-head CI.
  Diagnose any failure before pushing. Preserve the Pi and production safety
  boundary.

## CI gate update — 2026-11-14 09:25 UTC · previous head passed

- Core CI `43577820286` completed successfully on exact PR head
  `11309b63ede84abcea2eca4b99f71fc0e57e79c6`. Go 1.27 passed in 2m08s; Go
  1.26 passed Test, Browser Cue Delivery, Vet, Race, and Linux ARM64 product
  builds, completing in 32m40s. `gh run view` reports overall `completed`,
  `success`; the PR still points at this same SHA and remains OPEN/DRAFT.
- Companion `43577820339` and Camera Relay `43577820216` also passed on
  `11309b6`. Related current PR checks were re-read: TabletPlayer #36
  `91613da` pass (`42719885664`); ESP32 DMX #39 `3ad7749` six checks pass
  (`42759555665`); ESP32 Camera #20 `455b066` pass (`42761096513`); StageLaser
  #56 `b14a578` pass (`42765890415`); and StageLaser #57 `c4ff10b` pass
  (`38054411190`). Those PR heads were not changed.
- Before this checkpoint update, local branch head was
  `cab57f9aae74c41401b057ba83bb2908898fb142`, five commits ahead of remote:
  `834c051`, `f1287a0`, `f2fce09`, `677d18d`, and checkpoint `cab57f9`. This
  gate/matrix update is the sixth local commit. The pending pipe regression
  also checks post-close quarantine and
  idempotent Close. Local package/Race/Vet and ARM64/AMD64 test-binary build
  checks passed; exact Linux CI is still needed to execute its Linux-tagged
  tests.
- The SC-025 matrix now records exact current component heads and the prior
  Core/Companion/Relay run IDs. The next authorized operation is a normal
  fast-forward push of these six local commits to the existing branch/PR
  #534; do not force-push. Then verify remote SHA and await exact-head CI
  before another source batch.
- Pi 4 full-engine, burst, sustained-load, schedule-tail-jitter and service
  CPU/RAM performance remain unqualified. The isolated RC unit shares a host
  on which production Hub/Camera Relay services were confirmed active, while
  the qualification SSH alias still fails before session establishment. No
  Pi load, real output, firmware action, production/data/Vault operation, or
  production ACL enablement occurred. Keep production ACL disabled and treat
  Pi performance and any physical-output acceptance as separate pending
  gates.

## Resume update — 2026-11-14 10:11 UTC · active-SHOW rename lock regression

- Exact pushed head `32abf83072e28e3f6b9932f2e62f693c9c93e242` passed all
  checks: Core `43580593795` (Go 1.26 in 34m31s, Go 1.27 in 3m09s), Companion
  `43580593763` (Swift Core and full replacement/media acceptance), and
  Camera Relay `43580593768`. The Linux Go tests executed the isolated-pipe
  `LinuxPort` default-syscall regression; PR #534 remains OPEN/DRAFT.
- A new test reproduced SC-022's metadata-only Project rename failing with
  HTTP 503 while a SHOW session was active. Root cause: F-012's generic
  `BEFORE UPDATE ON projects` trigger also blocked updates to the display
  name. Commit `845ba4f` adds migration 61 to permit only `name` plus
  `updated_at_us` changes while a SHOW is active; all other Project-row
  changes remain blocked. The regression confirms session, published
  Snapshot, Cue, Project and revision identities stay fixed, and a direct
  description update remains locked. Migration 61 has a Down path restoring
  the original full-row lock.
- Test migration history: the StageLaser-58 test fixture is deliberately
  partial, so its focused migration helper now stops at version 58 instead of
  applying unrelated later schema migrations; the full database-open test
  now expects schema version 61.
- Local checks passed: `go test ./internal/httpapi -run 'Rename' -count=1`,
  focused `go test -race ./internal/httpapi` for the new regression,
  `go test ./internal/httpapi -count=1` (65.2s), `go test ./internal/db
  -count=1`, `go test -race ./internal/db -count=1`, and
  `go vet ./internal/httpapi ./internal/db`.
- Before this documentation update, local head was `845ba4f`, one commit
  ahead of remote `32abf83`; the SC-022/SC-025 matrix update is this second
  local commit. The prior exact-head CI gate is green, so the next operation
  is a normal push of both commits to the same branch/PR #534, followed by
  exact-head Core/Companion/Camera CI. No source change should be pushed if
  the remote SHA changed; recheck immediately before push.
- This was a SQLite migration and simulation-only HTTP test. No production
  service/data/Vault, ACL setting, physical output, or firmware was touched.
  Pi 4 full-engine performance and device-output acceptance remain pending;
  Project ACL remains disabled.

## Resume update — 2026-11-14 · schema CI correction and local suite

- Remote branch and Draft PR #534 remain at `b32e87faaf506ab7ac001f760cfce185faa7436b`; all workflows for that head are complete. Core `43584115052` failed on Go 1.26 and 1.27 because `TestExtensionLibraryPersistsVerifiedPackageAcrossRestart` expected schema 60 after migration 61. Companion `43584115115` and Camera Relay `43584115072` passed. This exact-head CI gate is complete; no workflow is being cancelled.
- Three local commits are ready, ending at `b3ee24b35f2ea67762e11604d69e2d7d3995e0ca`: `936f976` updates the test's schema expectation; `e9f8756` gives the StageAudio Unix socket tests short per-test paths, avoiding macOS `sockaddr_un` length errors; `b3ee24b` prints the structured ContractError in the Routing integration assertion after one full-suite failure exposed an unhelpful pointer-only diagnostic.
- Verification: `TMPDIR=/tmp/sc-go-test-tmp go test -p 2 -count=1 ./...` passed every package; `go vet ./...` passed. StageAudio package tests and Race passed; Routing Race passed; the local-OSC integration test passed 10 repetitions and a final focused run. `git diff --check` passed. An unrestricted uncached full run had transient failures in `cueengine` and `routing`; both packages passed individually, and the final bounded-parallelism full run passed. Preserve that history rather than attributing a cause that was not established.
- The local worktree is clean and three commits ahead of the remote. Next: recheck remote SHA and PR state, then push normally to the existing `integration/stagecore-rc` branch / PR #534. Record the new exact-head Core, Companion and Camera Relay run IDs in the PR and wait for all checks before another source push.
- After the CI gate, continue the highest outstanding software qualification: complete SC-024's whole-server endpoint-to-policy and asynchronous delivery review, with production ACL disabled. Continue Mixed Cue/linked-chain work using simulation; Pi 4 ARM64 performance remains pending because the only reachable host is production-like and the qualification SSH alias failed before session establishment. Do not run Pi load there.
- No production service, show data, Vault, real output, device firmware or ACL activation was touched. Keep the PR draft, do not merge, and do not force-push.

## Resume update — 2026-11-14 · SHOW intent grant route regression

- Remote and PR #534 are at `691257c2c5e95e0bf9d980dd858b4a06e738945c`. Core CI `43585768481` is still running on that exact head: Go 1.27 passed; Go 1.26 Test, Browser Cue Delivery and Vet passed; its Race job is running and Linux ARM64 product builds are pending. Companion `43585768471` and Camera Relay `43585768474` passed. Do not push while Core Race/ARM64 is active.
- Local commit `3c98b35` adds `TestQualifiedShowGOStopAndBlackoutUseIndependentGrants`. It uses a temporary SQLite Project, active simulated SHOW, an in-process test executor and an in-process Blackout callback. Project-view + STOP + Blackout grants deny GO; ShowGO permits GO only and does not imply STOP or Blackout; ShowBlackoutEngage independently permits simulated Blackout and latches the safety state. Normal and Race tests pass; `go vet ./internal/httpapi` and `git diff --check` pass.
- Commit `e499157` records this route-level evidence in the SC-024 qualification report and completion matrix. Local HEAD is `e499157053cce2f5ae9c5a147324c3ba8a8fa3d0`, two commits ahead of remote; worktree is clean. These two commits are not in the running Core workflow. After the exact `691257c` Core run completes successfully, recheck remote SHA and push both commits normally to the same branch/PR, update its check IDs, and wait for all exact-head CI before any further push.
- Next software gap after this gate remains the whole-server ACL/asynchronous delivery and installed-copy restore qualification; keep production ACL disabled. Broader matrix gaps include cue-editor browser acceptance and Show Capsule asset/copy semantics. Performance data already covers simulated Mixed Cue/linked chains, 100 Cues/200 Actions, schedule jitter, burst, sustained Rehearsal, STOP and Emergency Blackout on M3; Pi 4 qualification remains pending because the only host is production-like and the qualification alias fails before session establishment.
- No production services/data/Vault, physical output, firmware or ACL state changed. Keep simulation-only output, keep PR #534 draft, and do not merge or force-push.

## Resume update — 2026-11-14 10:52 UTC · Show Capsule asset round-trip

- The remote branch and open Draft PR #534 remain at `691257c2c5e95e0bf9d980dd858b4a06e738945c`. Core CI `43585768481` is still running against that exact SHA: Go 1.27 and Go 1.26 Test, Browser Cue Delivery, and Vet passed; Go 1.26 Race is running and Linux ARM64 CGo-free product builds are pending. Companion `43585768471` and Camera Relay `43585768474` passed. Do not push until Core finishes.
- Local commit `f95e18f` adds `TestSelfContainedMaterializePreservesExecutionEnvironmentAndExtensionAssets` and SC-023 matrix evidence. The test creates a disposable SQLite DB, Vault and extension install root; self-contained capsule export/materialization preserves a content-bound Execution Environment project file, captured snapshot object, and `osc.send` capability-selected extension software package. It verifies every restored object's bytes and metadata, and confirms materialization does not create an extension installation or execute it. No hardware/output path is used.
- Verification on local `f95e18f`: the new focused test passed, full `go test ./internal/showcapsule -count=1` passed, full `go test -race ./internal/showcapsule -count=1` passed, `go vet ./internal/showcapsule` and `git diff --check` passed. This evidence closes those two capsule asset classes only; SC-023 remains Partial for other asset classes, duplicate/copy semantics, and ACL/export qualification.
- Local `integration/stagecore-rc` is four commits ahead of remote (`3c98b35`, `e499157`, `e358f15`, `f95e18f`); worktree is clean. After exact-head Core `43585768481` completes green, recheck remote/PR SHA and push these four commits normally to the same branch and PR. Then record and await exact-head Core, Companion and Camera Relay CI before a new source batch.
- The route inventory remains a registration/review guard, not production enforcement. Continue the isolated whole-server Project-policy and asynchronous-result review after the CI gate; keep production ACL opt-in/off. Mixed Cue/linked nested chains, 100 Cue/200 Action, burst, sustained rehearsal, schedule jitter, STOP and simulated Emergency Blackout already have Mac evidence; do not repeat that work without a measured reason. Pi 4 ARM64 performance still lacks a safe reachable qualification environment; do not load the host running production Hub/Camera services. No production service/data/Vault, real output, firmware or production ACL state was touched.

## Resume update — 2026-11-14 11:16 UTC · OSC plugin startup cancellation

- Rechecked Git before editing: branch `integration/stagecore-rc`, local source HEAD `dec79f5723b9f87a888e3d11ab01ab9b0b7a9e3d`, remote HEAD `691257c2c5e95e0bf9d980dd858b4a06e738945c`; six saved commits are ahead, with no reset/clean/rebase/force-push. PR #534 is still OPEN/DRAFT. Exact-head CI on remote `691257c` is complete and green: Core `43585768481`, Companion `43585768471`, Camera Relay `43585768474`.
- The preceding full-suite run exposed a cancellation-boundary defect: Plugin Host waited on an internal one-second handshake detached from the caller's Cue context. Startup now uses the caller context, retains a five-second cap when the caller has no earlier deadline, and `Probe` passes its context. Startup cancellation stops the child; caller deadlines produce a timeout result, while cancellation remains cancelled. OSC maps a deadline consistently to `ExecutionTimedOut`.
- Added deterministic helper-plugin startup delay and the regressions `TestExecuteCancellationStopsPluginStartup` and `TestProbeCancellationStopsPluginStartup` for cancellation and deadline during a 2-second delayed `plugin.ready`. Routing's dispatched OSC fixtures preflight the plugin before testing routing/delivery, keeping cold process startup outside their action assertions. No real devices or external OSC destinations are used.
- Verification passed: focused normal tests for Plugin Host, Cue Engine, App and Routing; Race for those four packages; `go vet ./...`; and `git diff --check`. `TMPDIR=/tmp/sc-go-test-tmp go test -p 2 -count=1 ./...` produced 92 package results with no `FAIL` (the shell wrapper then hit zsh's read-only `status` variable while trying to print the already-completed log; all package results themselves were successful).
- Startup-cancellation code and tests are saved in commit `5af927cdb07843f6a072241efab408ceaf52b64e`; the source head was seven commits ahead of remote (six earlier saved commits plus this one). This checkpoint follows that source commit. Next, recheck the remote SHA/PR and push all local commits through this checkpoint normally to the same branch/PR. Wait for all exact-head Core/Companion/Camera CI before the next source batch.
- Pi 4 ARM64 runtime performance remains unqualified because the reachable host is production-like and the qualification SSH alias previously failed before opening a session. Do not run load there. No production service/data/Vault, physical output, firmware or production Project ACL state was touched; keep PR #534 draft and ACL disabled.

## Resume update — 2026-11-14 11:32 UTC · offline Cue recovery evidence

- The startup-cancellation batch is remote at `5c8e6fe38d48e174d052753d97965c12283df104`, PR #534 still OPEN/DRAFT. Camera Relay CI `43589510380` and Companion Core CI `43589510438` passed. Core CI `43589510454`: Go 1.27 passed; Go 1.26 Tests, Browser Cue Delivery and Vet passed; Race remains in progress and Linux ARM64 builds are pending. Do not push during this run.
- Inspected SC-009 implementation and tests instead of reimplementing recovery: browser-local revision records already survive an offline tab close, are scoped to User/Project/Cue, preserve changed-base edits for review without applying them, and handle damaged records and late cross-tab writes. `node --test tests/cue_recovery.test.js` passed all 11 tests. Commit `6a690b5` corrects the SC-009 matrix evidence to reflect this existing behavior.
- Current local branch is one commit ahead of remote solely for `6a690b5`; worktree is otherwise clean. After Core `43589510454` passes on exact remote head `5c8e6fe`, push this documentation correction normally to the same PR and wait for the new exact-head checks. Then continue the next demonstrated software gap from SC-011 readiness evidence freshness/offline remedies or the SC-024 endpoint-policy delivery review. Do not repeat the already qualified Mac Mixed/Linked performance suite.
- Pi 4 ARM64 runtime qualification still has no safe reachable isolated target: the known alias fails before opening a session and the accessible Pi host is production-like. No load or device commands ran. Production services/data/Vault and production Project ACL remain untouched; keep ACL disabled and PR #534 in Draft.

## Resume update — 2026-11-14 11:51 UTC · exact-head CI after documentation correction

- The preceding Core gate on `5c8e6fe38d48e174d052753d97965c12283df104` completed green: Core `43589510454` passed Go 1.26/1.27, Race and Linux ARM64 product builds; Companion `43589510438` and Camera Relay `43589510380` passed.
- Then pushed the two already-tested documentation commits normally to the same branch/PR: `6a690b5` records SC-009's existing browser-local offline recovery evidence (Node test 11/11); `112e3f5` records this resume state. PR #534 remains OPEN/DRAFT. PR description is now a concise current-head summary with the old duplicated snapshots removed; the full historical resume record remains in this file.
- Current local and remote HEAD is `112e3f529bd7afe95b2795a71c88a7b5d31480c2`, with a clean worktree. Exact-head workflows are running: Core `43591879418`, Companion `43591879475`, Camera Relay `43591879428`. The source code is unchanged from the prior fully green `5c8e6fe`; this head adds only the verified SC-009 matrix correction and this checkpoint. Do not push while these runs are active.
- After all three exact-head checks pass, continue with the next demonstrable gap. Current matrix candidates: SC-011's uncovered offline readiness/remedies, SC-016's safe cross-Project target remapping, and SC-024's remaining endpoint-policy/delivery review. Inspect their actual code/tests before choosing; do not repeat completed recovery or Mac Mixed/Linked performance work.
- Pi 4 performance remains unqualified for the previously documented isolation/access reason. No production services, show data/Vault, physical output, firmware or Production ACL were touched; keep Project ACL disabled and PR #534 in Draft.

## Resume update — 2026-11-14 11:55 UTC · SC-016 cross-Project Mixed-Cue copy

- The exact `112e3f529bd7afe95b2795a71c88a7b5d31480c2` CI gate is still running: Camera Relay `43591879428` and Companion `43591879475` passed; Core `43591879418` Go 1.27 passed and Go 1.26 Test, Browser Cue Delivery and Vet passed, with Race active and Linux ARM64 builds pending. Do not push until this exact-head run is complete.
- SC-016's prior clipboard code explicitly refused cross-Project paste despite Issue #495's copy/remap requirement. Commit `9b35fbf` now allows a same-editor-session user to paste one or more copied Actions into another Project's Draft. It copies only Action data, keeps target names, clears Action IDs, deep-clones Mixed actions, warns when targets are missing, and warns that matching logical names may refer to different destination configuration. Switching the User session invalidates access to the clipboard. No Project device configuration is copied and no publish/execute API is called.
- Focused Node tests passed 21/21 (`lighting_preview` + `cue_action_presets`); full `node --test tests/*.test.js` passed 70/70; `node --check internal/operatorweb/static/cue-studio.js` and `git diff --check` passed. The completion matrix now records this behavior and leaves bulk target-map selection/browser acceptance Partial.
- Local branch is two commits ahead of the remote: `fe524f1` checkpoint and `9b35fbf` SC-016 code. The code commit is not included in the running CI on remote `112e3f5`. After that gate passes, recheck remote/PR and push local commits normally to the same branch, then wait for the new exact-head CI.
- The Pi 4 safety gate and production restrictions are unchanged: no load on the production-like host, no real output/firmware, no production services/data/Vault changes, ACL stays disabled and PR #534 stays Draft.

## Resume update — 2026-11-14 12:27 UTC · SC-016 bulk target mapping

- Verified local `HEAD` before this batch was remote `2cad37b5d27bf9987a1d61e5f960dbfa34b36fcd`; the worktree was clean and PR #534 remained OPEN/DRAFT at that exact SHA. The PR body was confirmed current. Exact-head Core `43594362832`, Companion `43594362838`, and Camera Relay `43594362840` were in progress when work began.
- Camera Relay and Companion Core `43594362838` have since passed. Core `43594362832` has Go 1.27, Go 1.26 tests, Browser Cue Delivery, and Vet passed; Go 1.26 Race is still running and Linux ARM64 CGo-free product builds are pending. Do not push until the entire exact-head gate completes successfully.
- SC-016 commit `215fe9b24ed2999b70139680d49425d1daae8986` adds an accessible bulk target-map dialog for cross-Project copied Actions. It groups by source logical target, requires a configured destination selection, and edits only the Draft; it copies no hardware configuration and reminds the operator to verify compatibility. Added a regression for one-to-many application, destination-picker dispatch, and leaving unselected targets unchanged. The matrix now reserves only real-browser workflow acceptance and compatibility review as partial.
- Verification passed: focused `node --test tests/lighting_preview.test.js` (16/16), full `node --test tests/*.test.js` (71/71), `node --check internal/operatorweb/static/cue-studio.js`, and `git diff --check`. The local commit is one commit ahead of remote; the worktree was clean after committing. No real output or hardware was used.
- Next: wait for exact `2cad37b` Core Race and ARM64 jobs; recheck branch/PR SHA; push `215fe9b` normally to the same branch/PR only if the gate is green. Record new exact-head workflow IDs and wait for all checks before the next source push. Then continue the SC-024 endpoint/delivery review or another evidenced software gap.
- Pi 4 runtime performance remains unqualified because no safe isolated host is reachable; do not load the production-like host. Keep Project ACL disabled, do not touch production services/data/Vault, do not send real device commands or flash firmware, and keep PR #534 in Draft.

## Resume update — 2026-11-14 12:31 UTC · target-map session fencing

- Follow-on commit `73a75321a59f0ae57a19e87c5b0233a7772cdb0c` ties pending bulk target mapping to the destination editor User and Project, drops detached Draft cards, and clears the pending state when mapping is applied or context changes. This prevents a stale map control from acting on another user's or Project's editor state.
- Regression coverage now includes target-map invalidation across destination Project/User changes. The first focused run exposed only a test-harness omission (the extracted source fixture lacked the new closure state); the fixture was corrected, then focused `tests/lighting_preview.test.js` passed 17/17 and full `node --test tests/*.test.js` passed 72/72. JavaScript syntax and `git diff --check` passed.
- Exact-head CI gate on remote `2cad37b5d27bf9987a1d61e5f960dbfa34b36fcd` remains: Companion `43594362838` and Camera Relay `43594362840` passed; Core `43594362832` Go 1.27 and Go 1.26 Test/Browser Cue Delivery/Vet passed; Go 1.26 Race remains in progress, Linux ARM64 product builds pending. Local branch contains three unpushed commits after `2cad37b`: `215fe9b` bulk mapping, `4774cdf` checkpoint, `73a7532` context fencing. No push until all exact-head CI is green and the remote SHA is rechecked.
- The only visible browser was the pre-existing logged-in Owner Operator at `http://127.0.0.1:17840/`; it showed StageAudio unavailable / `Failed to fetch`. Inspection was read-only and did not open or change a Project, Cue, Show, Vault, or browser Draft. Real-browser mapping acceptance remains pending an isolated browser/API fixture.
- After the current Core gate passes: recheck the same branch/PR, normally push these commits, update PR #534 with the new exact-head run IDs, and wait for all checks. Then continue the evidenced SC-024 endpoint-to-policy/asynchronous-delivery review. Pi 4 ARM64 runtime performance, real browser acceptance, physical acceptance, and production release remain pending; production ACL remains disabled and PR remains Draft.

## Resume update — 2026-11-14 12:35 UTC · submit-time target-map revalidation

- Commit `85bcbe76e1d2ea47eb46ef047b10ae54ed50f446` rechecks the destination editor User/Project and Draft-card attachment when the bulk target-map dialog is submitted, so a context change while the dialog is open drops the operation. The regression also proves a detached card invalidates the context predicate.
- Verification after the final code edit passed: `node --test tests/lighting_preview.test.js` 17/17, full `node --test tests/*.test.js` 72/72, `node --check internal/operatorweb/static/cue-studio.js`, and `git diff --check`. Local branch is five commits ahead of remote `2cad37b` and was clean after the code commit.
- Continued read-only SC-024 asynchronous delivery review: `/api/v1/events` emits only `session.ready` (authenticated User and Hub identity) plus keepalives, not Project payload; it validates the exact browser Session/User before opening and every 15 seconds, and closes after revocation. `TestAuthenticatedSSEClosesAfterSessionRevocation` and `TestSSEReadyDoesNotUseRevokedSession` cover the boundary. No additional code gap was reproduced. The staged route inventory remains opt-in qualification, not production enforcement.
- Exact-head remote CI remains Core `43594362832` (Go 1.26 Race active, ARM64 builds pending), Companion `43594362838` passed, Camera Relay `43594362840` passed. Never push during the active Core run; after it is fully green, verify the remote SHA and PR state, then push all five local commits normally to the existing branch/PR.
- Keep browser acceptance pending an isolated browser/API fixture; the visible logged-in local Operator session is not an isolated test workspace. Keep Pi load off the production-like host, Project ACL disabled, PR #534 in Draft, and no production/device/Vault changes.

## Resume update — 2026-11-14 12:54 UTC · active-SHOW workspace pin

- Verified local and remote branch `integration/stagecore-rc` at the start of this batch: both were `a15b3ff4341ebe8b5331934a6c929ea95eac7d00`, with a clean worktree. Draft PR #534 remains open on that same head.
- Exact-head Core run `43596154343` on `a15b3ff` is still active: Go 1.27, Go 1.26 Test/Browser/Vet, Companion `43596154379`, Swift CompanionCore and isolated ARM64 `43596154349` have passed; Go 1.26 Race is running and Linux ARM64 product builds have not started. **Do not push until this Core run completes successfully; recheck remote SHA immediately before push.**
- Local commit `ea007cec3819ada674b7fe4582ff113aa91ff210` addresses a reproducible SC-017 presentation gap: opening/reloading a Project during active SHOW could follow its remembered authoring page. The dashboard/runtime response now pins the UI to Runtime, hides sidebar/profile navigation, and preserves the live GO, STOP and Emergency Blackout controls; stopped/non-SHOW sessions restore navigation. Backend authorization and runtime checks remain the actual safety boundary. The completion matrix records that this is still Partial pending real-browser visual/keyboard/touch/RTL and role/device-width acceptance.
- Verification passed on the local change: `go test ./internal/operatorweb -count=1`, `go test -race ./internal/operatorweb -count=1`, `go vet ./internal/operatorweb`, `node --check` for `app.js` and `workspace-profile.js`, full `node --test tests/*.test.js` (72/72), and `git diff --check`.
- The code/matrix commit is one commit ahead of remote; this handoff checkpoint is being recorded separately. Next, wait for Core `43596154343`, confirm all jobs are green and recheck remote/PR; then push the local commits normally to this existing branch and wait on exact-head Core/Companion/Camera Relay checks before another source push. Continue the next evidence-backed partial gap after that gate.
- Immediately before this final checkpoint note, local HEAD is `33d589ed81c0206b0ea5aadcf64de026a609c3b7`, three commits ahead of remote `a15b3ff4341ebe8b5331934a6c929ea95eac7d00`: `ea007ce` (SC-017 code + matrix), `5a8ed6b` (resume entry), and `33d589e` (checkpoint wording correction). The worktree is clean. This note is the fourth local commit in the batch.
- No production service/data/Vault, real stage output, firmware, or production ACL was touched. Pi performance remains unqualified without an isolated target; keep ACL disabled, PR #534 Draft, and do not merge/force-push.

## Resume update — 2026-11-14 13:19 UTC · pushed SC-017 workspace pin

- Previous exact-head CI on `a15b3ff4341ebe8b5331934a6c929ea95eac7d00` is fully green: Core `43596154343` (Go 1.26/1.27, full Race and Linux ARM64 products), Companion `43596154379`, and Camera Relay `43596154349`.
- Pushed the four-commit batch normally to the existing `integration/stagecore-rc` branch and Draft PR #534: `ea007ce` SC-017 active-SHOW Runtime pin; `5a8ed6b`, `33d589e`, and `cf4f3ba` durable handoff updates. Local/remote source HEAD is `cf4f3ba017e52767b67a48e114c16a80be96154c`; worktree was clean at checkpoint time. PR body and linked matrix/handoff were updated to this head.
- Exact-head checks for `cf4f3ba` are now running: Core `43599099278`, Companion `43599099231`, Camera Relay `43599099257`. Do not push while these jobs run; recheck PR head and each workflow before the next batch.
- SC-017 keeps navigation/profile preferences out of the active SHOW surface and routes reload/remembered-page entry back to Runtime; GO, STOP and Emergency Blackout remain in Runtime. Local checks passed: Operator web package tests, Race, Vet, JS syntax, all 72 Node tests and `git diff --check`. Matrix marks real-browser accessibility/role-width acceptance Partial.
- No further SC-011 code change was made: the current readiness evidence is deliberately advisory where no trustworthy endpoint/ACK data exists; do not add network probes or infer physical output. Pi 4 runtime performance remains pending an isolated target. No production services/data/Vault, real devices, firmware, or production ACL were touched; ACL remains disabled and PR #534 remains Draft.
- Next: finish exact-head `cf4f3ba` CI gate. After all checks pass, continue the next reproducible software gap (or isolated browser acceptance if a safe fixture is available), then test, update matrix/handoff, push only to this branch, and wait for its new exact-head CI.

## Resume update — 2026-11-14 13:27 UTC · SC-017 CI fix and SC-010 descriptions

- Exact-head workflows for remote `cf4f3ba017e52767b67a48e114c16a80be96154c` have completed. Go 1.26 Core failed in Browser Cue Delivery because the reduced browser fixture omits `#appView` and the new presentation helper dereferenced it; this was a shell-fixture robustness bug, not cue delivery behavior. Go 1.27, Companion, and Camera Relay passed.
- Fixed the null shell boundary in commit `772bade`. The existing Chrome-backed `TestBrowserCueDeliveryThroughPersistedTrace` now passes locally using `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`; `go test ./internal/operatorweb`, its Race run and Vet pass.
- Continued SC-010 in `151d4f1`: migration 62 adds an optional description with a 280-character limit and a default empty value for existing presets. Store/API return and validate it; Cue Studio displays it as text, searches it, and saves/clears the optional field. Issue #489 describes cross-Project/shared-library import as later optional work, so the matrix no longer treats that as a blocking acceptance condition.
- SC-010 checks passed: focused API recovery/preset test, `go test ./internal/db`, the existing extension persistence/schema test, focused HTTP API Race, `go vet ./internal/httpapi ./internal/store`, `node --check` for the changed UI scripts, all Node tests (72/72), and `git diff --check`. API rejects descriptions over 280 Unicode code points.
- Local HEAD before this checkpoint is `151d4f1a7446f2d4ffc6b9295277f04568da64a1`, three commits ahead of remote `cf4f3ba`: local checkpoint `ef98830`, SC-017 browser-shell guard `772bade`, and SC-010 preset descriptions `151d4f1`. Worktree is clean. This handoff update will be the fourth local commit for the next push.
- Previous exact-head CI is complete (including the failure above), so the next action after recording this checkpoint is to recheck PR/remote SHA and push these local commits normally to the same branch. Update PR #534 with the new SHA and workflow IDs, then wait for Core/Companion/Camera Relay to pass before any further push.
- No production, show data, Vault, physical output, firmware, or production ACL was touched. Keep Project ACL disabled, do not load the production-like Pi for performance tests, and keep PR #534 Draft.

## Resume update — 2026-11-14 13:28 UTC · pushed SC-010 and SC-017 fix

- Pushed four commits normally to existing branch/PR #534 after the completed `cf4f3ba` CI failure was diagnosed and locally fixed: `ef98830` checkpoint, `772bade` reduced-browser shell guard, `151d4f1` SC-010 descriptions and migration 62, and `fd740b8` this campaign checkpoint. Local and remote `integration/stagecore-rc` now match at `fd740b88a12d90a4874eff1fb69fc097b54ab330`; worktree was clean before this note. PR remains OPEN/DRAFT and its body now includes SC-010, SC-016, SC-017, SC-019 and current check links.
- The completed `cf4f3ba` Core failure was the browser harness TypeError at `setShowSessionPresentation` because its temporary page does not include the app shell. The null guard is committed and the Chrome-backed browser test passes locally.
- Current exact-head workflows are in progress for `fd740b8`: Core `43599903021`, Companion `43599903035`, and Camera Relay `43599903018`. Do not push any further commits while they run; inspect all outcomes and refresh PR/remote SHA before the next push.
- SC-010 adds optional 280-character preset descriptions through Store/API and Cue Studio, searches and displays descriptions as text, and preserves the empty default for old rows. The main acceptance is project-scoped use; shared/cross-Project library import remains optional later work in Issue #489. Local API, migration, Store/schema, HTTP Race, Vet, Node 72/72, JS syntax and diff checks passed.
- No production services/data/Vault, physical device commands, firmware, or production ACL changed. Pi 4 performance is still pending a safe isolated target; production ACL stays disabled and the PR stays Draft.
- Next task: wait for all exact-head `fd740b8` checks. Once green, move to the next reproducible software gap or an isolated browser fixture if available, validate locally, update the matrix/handoff and push only to this existing PR.

## Resume update — 2026-11-14 14:12 UTC · isolated SHOW workspace browser test

- Rechecked PR #534 and the branch before resuming: `integration/stagecore-rc` is OPEN/DRAFT at remote `fd740b88a12d90a4874eff1fb69fc097b54ab330`. Exact-head workflows completed successfully: Core `43599903021`, Companion `43599903035`, Camera Relay `43599903018`.
- Local HEAD is `d3f4f818e2a26fb7e724e06025d01bc46c3200cc`, three commits ahead of that remote: the already-saved checkpoint `bd44eb9`, `9c7c68b` (Chrome-backed active-SHOW workspace test and Core CI registration), and `d3f4f81` (SC-017 matrix evidence). The latest handoff note is this fourth commit in the next push batch. Worktree changes in this batch are limited to the named browser test, test script, workflow, matrix and checkpoint files; no reset/clean/rebase/force-push was used.
- `TestBrowserShowWorkspacePinsRuntime` builds a temporary SQLite project and published Snapshot, seeds only a local active SHOW session, and uses a simulator registry without executing cues. Real Chrome verifies a remembered Cues page cannot escape to authoring, Runtime retains GO/STOP/Emergency Blackout controls, and the active-SHOW CSS hides the sidebar/profile control and restores them when the presentation state clears. It does not send device commands and does not exercise operational SHOW admission.
- Local verification passed: both Chrome-backed HTTP browser tests in one focused run; the new Chrome test under Go Race; `TestShowSessionPinsOperatorToRuntimeWorkspace`; `go vet ./internal/httpapi`; JS syntax and `git diff --check`. SC-017 remains Partial for browser keyboard/touch/RTL, target-width and per-role acceptance, and re-entry after an actual stopped session.
- Immediately before the next push, recheck the remote SHA, PR state, and exact-head CI. Since all `fd740b8` checks are complete and green, push these three local commits normally to the existing branch/PR #534, update the PR summary with the new SHA and workflow IDs, then wait for all exact-head workflows before any further push.
- Next software task after that CI gate: continue isolated real-browser acceptance for SC-008 Mixed Cues and nested linked chains, building on existing Node-level authoring/round-trip evidence rather than repeating it. Pi 4 ARM64 performance qualification still has no safe reachable isolated host; do not load the production-like host or infer its P95/P99/Jitter from Mac or cross-build results.
- Production Hub/Camera Relay, show data, Vault, physical outputs, firmware and production ACL remain untouched; ACL is disabled, PR #534 stays Draft, and no merge/force-push is allowed.

## Resume update — 2026-11-14 14:34 UTC · nested Mixed Cue browser acceptance

- Remote PR #534 remains OPEN/DRAFT at `c1a4e492416d9cd894202fd660490493c92cb2ed`. Its exact-head Core CI `43603870346` is still in progress; Camera Relay `43603870345` and Companion `43603870310` passed. Do not push until Core completes successfully.
- Local HEAD `58f930d810aee7ee3e8b24a5ddf06ede46384510` is two commits ahead of remote: `94dfc2e` adds a Chrome-backed Mixed Cue authoring test and adds it to Core's Browser Cue Delivery gate; `58f930d` records SC-008 evidence in the completion matrix. This resume note will be the third local commit in the next push batch.
- `TestBrowserCreatesNestedMixedCueChainInDraft` uses the real embedded Operator shell, Cue Studio picker/editor, authenticated APIs, temporary SQLite and a simulator-only `SIM` alias. It creates three two-Action Draft Cues linked across three levels, covers five capability types, action ordering and per-Cue delay, then verifies the persisted graph and no runtime session. It does not publish, execute, or contact hardware. The test follows Cue Studio's real Action-type picker before editing each card.
- Verification passed: all three Chrome-backed browser tests together; the new Mixed Cue browser test under Go Race; the full `go test ./internal/httpapi -count=1` suite (64.194s); `go vet ./internal/httpapi`; JS syntax and `git diff --check`.
- After exact-head Core `43603870346` finishes green, recheck the remote SHA and PR state, push these local commits normally to the same branch/PR #534, update the PR body and wait for all new exact-head workflows before any further push.
- SC-008 remains Partial for keyboard/touch/layout acceptance across all adapters and persistence through restart/restore. Next continue that isolated browser/rollback work after this CI gate. Pi 4 runtime performance still lacks a safe reachable isolated host; do not run load on the production-like host. Keep PR #534 Draft, Project ACL disabled, and all production/Vault/device/firmware boundaries unchanged.

## Resume update — 2026-11-14 14:37 UTC · nested Mixed Cue edit round-trip

- Remote and Draft PR #534 remain at exact SHA `c1a4e492416d9cd894202fd660490493c92cb2ed`; Core `43603870346` is still in progress, and Companion `43603870310` plus Camera Relay `43603870345` passed. No push until this exact Core run completes green.
- The isolated Chrome scenario now edits the parent after creating the three-level Mixed Cue chain. The real UI reopens both actions, restores the parent→middle checkbox, saves a name change, and verifies the original Action IDs and link remain stable in the edited Draft. Go then re-reads SQLite and checks the edited name, Action identities, all five capability types, sequence, delays and nested Cue IDs.
- Local HEAD `c52df5ee1c3dd40e1f710f22e6de4e20f17560e9` is four commits ahead of remote: `94dfc2e` browser create-chain test + Core gate, `58f930d` SC-008 matrix evidence, `a68141b` checkpoint, and `c52df5e` browser edit round-trip. This checkpoint/matrix update will be the fifth local commit.
- Verification on the edit extension passed: the Mixed Cue browser test in Chrome normally and under Race; all three Chrome-backed browser tests together; `go vet ./internal/httpapi`; JavaScript syntax and `git diff --check`. The full `go test ./internal/httpapi -count=1` passed in 64.194s on the preceding code commit `94dfc2e`, before the edit-only test extension.
- Recheck remote and PR after Core completes. If the SHA remains `c1a4e49` and all checks pass, push the five local commits normally to this existing PR, update the PR body and wait for exact-head Core/Companion/Camera CI before further pushes.
- SC-008 remains Partial for keyboard/touch/RTL/layout acceptance across adapters and restart/restore persistence. Pi 4 ARM64 performance remains unqualified because no safe isolated target is reachable. No hardware or real output was contacted; all Cue targets are simulator aliases in temporary SQLite. Keep ACL disabled, PR Draft, and production/Vault/device/firmware boundaries untouched.

## Resume update — 2026-11-14 14:43 UTC · nested Cue Inspector simulation

- Environment and branch rechecked: Apple Silicon `darwin/arm64`, Go 1.27.1. Local `integration/stagecore-rc` HEAD is `f5deb5d7c3277528de45676345c04b2a19debdc4`, six commits ahead of upstream `c1a4e492416d9cd894202fd660490493c92cb2ed`; the source worktree was clean before this matrix/checkpoint edit. PR #534 remains OPEN/DRAFT.
- Exact remote Core CI `43603870346` is still running Go 1.26 Race; Go 1.27 passed, and Go 1.26 Test, Browser Cue Delivery, and Vet passed. Linux ARM64 product builds have not completed. Companion `43603870310` and Camera Relay `43603870345` passed. No push until the full Core workflow succeeds; check exact remote SHA again before pushing.
- Commit `f5deb5d` extends the nested Mixed Cue Chrome test to open the real Cue Inspector and check all five tested capability summaries across three linked levels. It runs the virtual-offline-target simulation against all six Actions, confirms all are marked simulated-blocked, and verifies the simulation performs no additional network call and no GO/STOP/Blackout request. It uses temporary SQLite and `SIM` only; nothing is published or executed.
- Local verification passed: the focused Chrome test; all three Chrome browser acceptance tests together under Go Race; `go vet ./internal/httpapi`; `node --check internal/httpapi/testdata/mixed_cue_browser.js`; and `git diff --check`. Matrix now records SC-014's evidence and remaining 50+ Cue navigation, note/label editing, keyboard/touch/RTL, and Snapshot/SHOW immutability checks.
- Rechecked companion PRs: TabletPlayer #36, ESP32 DMX #39, ESP32 Camera #20, and StageLaser #56 are open drafts at their existing heads with their listed exact-head jobs green; StageLaser #57 remains open/non-draft with its listed checks green. StageCore #503–#508 and #514–#516 remain open at matrix-listed heads with their listed Go 1.26/1.27 checks green. Their code checkouts are not present in this workspace, so no other repository was modified. The related-component matrix now records this status refresh.
- The source/test commit `f5deb5d` plus the prior five commits remain local. Matrix and handoff changes form the next documentation commit. After Core completes, recheck the remote/PR head and exact workflows, push these seven commits normally to existing PR #534 only if all checks pass, update its description with the new SC-008/SC-014 browser evidence, and wait for all exact-head CI before another push.
- Next software work: continue browser and Draft recovery/rollback acceptance for Mixed and Linked Cues, then take the next code-proven gap. Pi 4 ARM64 full-engine/burst/sustained performance remains blocked by the lack of a verified isolated target; do not use the production-like host or infer results from cross-builds/Mac data. No production service/data/Vault, physical output, firmware, or ACL cutover was touched; ACL remains disabled, PR #534 remains Draft, and merge/force-push remain prohibited.

## Resume update — 2026-11-14 14:51 UTC · SC-014 Draft quick edits

- Rechecked the published PR head and CI: PR #534 is OPEN/DRAFT at `c1a4e492416d9cd894202fd660490493c92cb2ed`. Exact-head Core `43603870346` is now fully green, including Go 1.26/1.27 Test, Browser Cue Delivery, Vet, Race, and Linux ARM64 product builds. Companion `43603870310` and Camera Relay `43603870345` are green. The Core run completed before any push; verify the same PR/remote SHA again immediately before pushing.
- Local source HEAD `cc32d7d72ae475ed3066902202e70e2afc9b76a8` is eight commits ahead of remote, with a clean source worktree before this checkpoint update. `cc32d7d` adds a safe Cue Inspector quick edit for Draft label and operator notes. It sends only the existing Cue Draft PUT, preserves the full serialized Action list and execution policy, checks current Project/revision/edit eligibility before saving, rerenders the saved Inspector while preserving scroll/focus, and never publishes or calls runtime routes.
- Chrome coverage in `TestBrowserCreatesNestedMixedCueChainInDraft` now checks label/note editing on a three-level Linked Mixed Cue, verifies stored notes and labels in SQLite, proves Action identities/capabilities/order and nested links remain intact, and confirms no runtime session starts. Inspector preview remains local-only and all Actions are marked blocked in the virtual offline-target scenario. Temporary SQLite and `SIM` only.
- Final local checks after the feature passed: focused Chrome test; all three Chrome-backed browser tests under Go Race; `go vet ./internal/httpapi`; JavaScript syntax checks for `cue-studio.js` and the browser script; full Node suite (72/72); and `git diff --check`. An initial test-only selector assumed English UI text; the check was changed to validate locale-independent label structure and passed. Matrix SC-014 now records quick-edit evidence and remaining 50+ Cue navigation/scroll, keyboard/touch/RTL, and explicit Snapshot/active-SHOW immutability acceptance.
- The preceding six local commits cover nested Mixed Cue creation/edit round-trip, their durable notes, and browser-only Inspector simulation; `4429c8c` refreshes SC-014/cross-repository evidence; `cc32d7d` adds quick editing. This matrix/checkpoint commit will be the ninth commit ahead of remote. Related PRs #36, #39, #20, #56, and #57 are open at their checked heads with checks green; StageCore #503–#508 and #514–#516 likewise remain open at matrix-listed heads with their listed Go checks green. No linked repository checkout was available, so none was modified.
- The next step is a normal push of these nine commits to existing Draft PR #534 after verifying its remote SHA and green exact-head workflows, then update the PR body and wait for the new exact-head Core/Companion/Camera Relay checks before another push. Continue with Draft recovery/rollback and the next evidence-backed software gap. Pi 4 ARM64 full-engine, burst, and sustained qualification still requires a verified isolated target. Keep all production services/data/Vault untouched, outputs simulated, production ACL disabled, PR Draft, and do not merge or force-push.

## Resume update — 2026-11-14 14:54 UTC · pushed SC-008/SC-014 batch

- The nine-commit batch was pushed normally to the existing branch/PR after exact-head `c1a4e49` completed fully green. Local and remote source HEAD are now `66bff3bb276eb64b60926e209ad476221bbbbcda` on `integration/stagecore-rc`; PR #534 remains OPEN/DRAFT. The source worktree was clean immediately after the push.
- The batch includes the Chrome-backed nested Mixed Cue authoring/edit round-trip, Cue Inspector dry-run safety checks, SC-014 label/notes quick editing, updated SC-008/SC-014 evidence, and cross-repository status/handoff refresh. The PR body now summarizes these changes, links the current matrix/checkpoint, and distinguishes the simulated Inspector preview from execution.
- Current exact-head checks for `66bff3b`: Camera Relay `43607240271` passed; Core `43607240278` Go 1.26 and Go 1.27 remain in progress; Companion `43607240272` remains in progress. Do not push while either Core or Companion is running. Wait for every check to complete, then verify PR and remote SHA before a further push.
- Next independent work while waiting: continue browser acceptance around 50+ Cue navigation/scroll/selection and test Draft restore/rollback evidence, choosing only gaps not already covered. The last source batch passed focused Chrome, all three Chrome tests under Race, Vet, JS syntax, Node 72/72, and diff checks locally.
- Pi 4 ARM64 full-engine/burst/sustained performance is still blocked by lack of a verified isolated target; do not load the production-like host or substitute Mac/cross-build figures. No production service/data/Vault, physical output, firmware, or production ACL changed. Keep ACL disabled, PR in Draft, and do not merge or force-push.

## Resume update — 2026-11-14 15:04 UTC · long-list Inspector navigation

- Remote PR #534 is OPEN/DRAFT at `66bff3bb276eb64b60926e209ad476221bbbbcda`. Camera Relay `43607240271` and Companion `43607240272` passed. Core `43607240278` Go 1.27 passed, but Go 1.26 Browser Cue Delivery failed in `TestBrowserCreatesNestedMixedCueChainInDraft` because the test held an Inspector node detached by the rerender after quick edit. This is a browser-test synchronization defect; the test now re-queries the current Inspector node and passes locally.
- Local HEAD `41d5279db0b547f7c79703b09f75c540b2aee993` is two commits ahead of remote before this matrix/checkpoint update; `16079f8` records the push and `41d5279` adds Inspector navigation and corrects the stale-node test query. This update will be the third local commit; do not push without first verifying exact remote/PR state and the completed `66bff3b` CI result.
- Previous exact head `c1a4e49` was fully green. Core's `66bff3b` Go 1.26 failure has been inspected; the failure is isolated to the stale DOM reference after the successful Draft PUT and rerender. No production behavior or security boundary failed.
- SC-014 now has Previous/Next controls in the Inspector. A real Chrome run navigates across 63 Cues, confirms each row scrolls into view, moves focus to its Inspector, and verifies the displayed Action context; the first/last boundary controls are disabled. The same test retains the three-level Mixed Cue preview and label/note quick-edit assertions. The matrix retains keyboard shortcut, RTL/touch layout, and Published Snapshot/active-SHOW immutability as open acceptance.
- Verification after this change passed: `STAGECORE_TEST_CHROME=... go test ./internal/httpapi -run '^TestBrowserCreatesNestedMixedCueChainInDraft$' -count=1`; the three Chrome browser tests under Race; `go vet ./internal/httpapi`; JS syntax checks; full Node suite (72/72); full `STAGECORE_TEST_CHROME=... go test ./internal/httpapi -count=1` (67.441s); and `git diff --check`.
- Push the completed 3-commit local batch normally to existing PR #534 after rechecking remote SHA/state; this starts a new exact-head CI. Update the PR body for the 63-Cue navigation and current CI IDs, then wait for all Core/Companion/Camera Relay checks before any additional push. Continue next with Draft restore/rollback evidence. Pi 4 performance still needs a verified isolated target. No production service/data/Vault, physical output, firmware, or production ACL changed; ACL stays disabled, PR stays Draft, and merge/force-push are prohibited.

## Resume update — 2026-11-14 15:12 UTC · 123-Cue browser search and Inspector

- Rechecked `git status`, local/remote HEAD and PR #534. The worktree was clean
  at resume; local and remote both matched `759bcbd21ae331759443c2c37fe451d0e96bc78a`.
  PR #534 remains OPEN/DRAFT on `integration/stagecore-rc`.
- Exact pushed-head Camera Relay `43608326224` and Companion `43608326350`
  passed. Core `43608326253` Go 1.27 passed; Go 1.26 Race remained in progress
  on the last query. No push is allowed until this exact Core run completes and
  the PR/remote SHA are rechecked.
- Local follow-on expands the isolated Chrome Draft fixture from 63 to 123
  Cues. It searches the 120th Cue, finds a Lighting Action inside a nested
  three-Cue Mixed chain, combines Mixed and Linked filters, clears the filters,
  and verifies the complete row sequence is unchanged. It then navigates the
  Inspector through all 123 rows and checks Action context, focus, viewport and
  boundary buttons. All data is in temporary SQLite; target is `SIM`; no Cue is
  published, run, or sent to a device.
- The first Race run exposed a browser-test-only rerender race between two DOM
  assertions. Replaced the split assertions with a stable combined DOM query;
  the focused Chrome test then passed normally (7.9 s) and under Race (12.4 s).
  `node --test tests/cue_list_filters.test.js` passed 5/5, browser script
  `node --check`, `go vet ./internal/httpapi`, and `git diff --check` passed.
- Matrix SC-012 and SC-014 now record the 123-Cue browser evidence and the
  remaining actual keyboard/touch, RTL/LTR and narrow-layout checks. The
  related-component matrix was refreshed: TabletPlayer #36 (`91613da`), DMX
  #39 (`3ad7749`), Camera #20 (`455b066`), StageLaser #56 (`b14a578`) and #57
  (`c4ff10b`) remain open at the reviewed heads. No linked checkout was
  available, so no linked repository was changed.
- Four files are modified locally (the two browser test files and two
  qualification matrices); this checkpoint is the fifth. Commit the tested
  batch locally, then wait for Core `43608326253` before any push. Once that
  run is green, verify source HEAD and PR #534, push normally to the existing
  branch, update the PR description with exact new CI and browser evidence, and
  wait for the new exact-head checks.
- Next software focus after the CI gate: continue SC-024 only on a reproducible
  endpoint-to-policy/asynchronous-delivery gap, without changing Production
  Assembler or enabling ACL. Pi 4 runtime P95/P99, jitter, CPU/RAM and sustained
  ARM64 measurements remain blocked because no verified isolated target is
  available; Mac evidence is already recorded and is not being repeated.
  Production services/data/Vault, physical output and firmware remain untouched;
  PR stays Draft, no merge/force push, and SC-024 stays off in Production.

## Resume update — 2026-11-14 12:40 UTC · SC-019 reversible approval evidence

- Commit `e457922955124e86f8a3e6455f6246b05bc8951e` extends the existing timing approval regression: it records raw timing observations before approval, repeats INCLUDE while the Rehearsal is active, verifies the active candidate remains pending, then verifies learning after stop and reversibility through AUTO/EXCLUDE. It asserts the complete raw observation records are identical before and after all selection changes. No production implementation change was necessary; the completion matrix now records the verified test evidence.
- Checks passed: focused test normal and Race; full `go test ./internal/timingintelligence -count=1` and `go test -race ./internal/timingintelligence -count=1`; `go vet ./internal/timingintelligence`; and `git diff --check`. Local branch is seven commits ahead of remote `2cad37b`; worktree was clean after commit.
- The active remote CI is still for exact SHA `2cad37b5d27bf9987a1d61e5f960dbfa34b36fcd`, which predates all seven local commits: Go 1.27 and Go 1.26 tests, Browser Cue Delivery and Vet passed; Go 1.26 Race remains active and Linux ARM64 builds have not started. Companion `43594362838` and Camera Relay `43594362840` passed. No push until the complete Core gate finishes successfully.
- After the CI gate, recheck the remote branch and PR #534, push the seven local commits normally to the existing branch, update PR description with the new exact-head checks, and wait for those checks before another source push. Continue with another partial software gap; keep production ACL disabled and all production/device/Vault boundaries intact.

## Resume update — 2026-11-14 15:19 UTC · prepared Project access browser gate

- Local commit `4c84b6b7ac1d18e23e5b5535d5e398c57abad2e4` adds the 123-Cue
  real-browser search/filter/Inspector qualification. Local commit
  `a8f93bc28de947325075e99e9347edba1e05f6bf` adds the real-Chrome Owner
  Project-access editor gate and updates the SC-024 evidence row. Both are
  local-only; remote remains `759bcbd21ae331759443c2c37fe451d0e96bc78a`.
- The access browser test runs the actual Security page against temporary
  SQLite and the optional qualification routes. It loads a Project and
  Operator's existing grants, stores `OPERATE`, checks the UI's not-enforced
  disclosure and verifies API readback. It confirms no publish, GO/JUMP/STOP,
  or Emergency Blackout request occurs. It does not assemble production ACL.
- Focused Chrome test passed normally and under Go Race; the existing
  `tests/security_project_access.test.js` passed 8/8; JS syntax, `go vet
  ./internal/httpapi`, and `git diff --check` passed. Existing API tests verify
  the stored `OPERATE` permission set and that it does not grant Cue editing.
- Exact pushed-head Camera Relay `43608326224` and Companion `43608326350`
  pass. Core `43608326253` remained in progress at the last check, with Go
  1.27 green and Go 1.26 Race still running. Wait for this run before pushing
  either local commit, then recheck remote/PR SHA and completion. After a
  normal push, update PR #534's description with the new browser evidence and
  exact workflow links, then wait for every new exact-head check.
- SC-024 remains partial: whole-server endpoint-to-policy/asynchronous
  reconciliation, keyboard/touch/RTL browser checks, Companion compatibility,
  and installed-copy backup/rollback/full-suite qualification remain. ACL is
  not enforced in Production. Pi 4 engine performance remains blocked on a
  verified isolated target; existing Mac measurements were not repeated.
  Production services/data/Vault and physical outputs were not accessed.

## Resume update — 2026-11-14 15:20 UTC · PR and local batch state

- The local source branch now contains four commits beyond remote: `4c84b6b`
  (123-Cue browser search/filter/Inspector), `a8f93bc` (Owner grant browser
  gate), `cd31925` (checkpoint), and this documentation checkpoint. Read
  `git rev-parse HEAD` for the resulting exact tip. Remote and PR #534 remain
  at `759bcbd21ae331759443c2c37fe451d0e96bc78a`.
- Updated the existing PR #534 body in place to name the actual remote HEAD,
  describe 63-Cue Inspector evidence, link current exact-head workflows, and
  report Core's Go 1.26 Race as still running. The unpushed 123-Cue and grant
  browser changes are not represented as merged or CI-verified in the PR.
- Core `43608326253` has passed Go 1.27, Go 1.26 Test, Browser Cue Delivery,
  and Vet. Go 1.26 Race is still in progress and Linux ARM64 builds are
  pending. Companion `43608326350` and Camera Relay `43608326224` passed.
  Wait for the full Core job, then verify the SHA and push the accumulated
  five-commit local batch normally to the same PR if all checks succeed.

## Resume update — 2026-11-14 15:23 UTC · full HTTP API package passed

- Full local `STAGECORE_TEST_CHROME=... go test ./internal/httpapi -count=1`
  passed in 70.511 seconds on local `9ffdd7e1cbcc7e7bab942ea8719e8db040f7b28b`.
  This includes the 123-Cue Mixed/Linked browser flow and the Owner Project
  access browser gate alongside the package's existing API tests.
- Exact remote Core run `43608326253` still has Go 1.26 Race in progress;
  Linux ARM64 product builds have not started. The local branch was clean and
  five commits ahead of remote as of the preceding status check. Do not push.

## Resume update — 2026-11-14 15:27 UTC · include SC-024 browser gate in CI

- Core CI's Browser Cue Delivery step uses an explicit test-name allowlist.
  Added `TestBrowserOwnerProjectAccessEditorStoresPreparedLevel` to that
  allowlist so the real-Chrome prepared-grant workflow and 123-Cue browser
  acceptance run in the next exact-head CI, not only locally. The active CI on
  `759bcbd` predates this change and does not include the new grant browser
  test.
- Workflow diff is one test-name addition; `git diff --check` passes. The
  focused grant browser test and the full HTTP API package already pass
  locally with Chrome, including Race for the focused grant test. No
  `actionlint` binary is installed on this Mac.
- Current exact remote run `43608326253` remains active in Go 1.26 Race;
  Linux ARM64 builds are pending. Do not push any accumulated local commits
  until this run completes, then recheck PR #534 and the branch SHA.

## Resume update — 2026-11-14 15:33 UTC · exact local checkpoint

- Immediately before this checkpoint, the clean local branch tip was
  `4d1c0ebf5992c556a9d5ee64d4e7aaabd66d761f`, six commits ahead of remote
  `759bcbd21ae331759443c2c37fe451d0e96bc78a`. This handoff update is included
  in the latest local checkpoint commit; run `git rev-parse HEAD` for its tip.
- The same remote Core run `43608326253` is still in progress in Go 1.26 Race;
  its Linux ARM64 build is pending. Go 1.27, Core Test, Browser Cue Delivery,
  and Vet passed; Companion `43608326350` and Camera Relay `43608326224` passed.
- PR #534 remains OPEN/DRAFT at the remote SHA. Its body now reflects that
  exact SHA and links those workflow runs. The next push is the accumulated
  local batch to the existing branch only after run `43608326253` completes.

## Resume update — 2026-11-14 15:46 UTC · 123-Cue and SC-024 browser batch pushed

- After exact-head CI `43608326253` completed green for Core Go 1.26/1.27,
  Race and Linux ARM64 builds, and after Companion/Camera Relay also passed,
  pushed the six-commit batch normally to the existing branch and Draft PR
  #534. New exact Local/Remote/PR HEAD is
  `5b6e1e356f0a02395a2630962a1695df0ba70707`; no force push or merge.
- The six commits add real-Chrome coverage for 123-Cue search/filter/Inspector
  traversal and the prepared Owner grant editor; add the grant browser test to
  Core CI's Browser Cue Delivery allowlist; update SC-012/SC-014/SC-024 and
  cross-component matrices; and record checkpoints. The PR body was updated at
  this HEAD with the evidence, local test results, and exact workflow links.
- Local checks before push: full Chrome-enabled `go test ./internal/httpapi
  -count=1` passed in 70.511s; the focused grant Chrome test passed normally
  and under Race; `node --test tests/cue_list_filters.test.js` passed 5/5;
  `node --test tests/security_project_access.test.js` passed 8/8; `go vet
  ./internal/httpapi`, both browser-script syntax checks, workflow YAML parse,
  and diff checks passed.
- New exact-head CI at `5b6e1e3`: Core `43611749231` remains in progress;
  Companion `43611749229` and Camera Relay `43611749232` passed. Updated the PR
  body to reflect the current statuses. Wait for Core before another source
  push. Current PR is OPEN/DRAFT.
- SC-024 remains only prepared in qualification assembly; Production ACL is
  not enabled. No production service/data/Vault access, physical device output,
  firmware flash, or hardware performance run occurred. Pi 4 remains blocked
  on verified isolated access. Continue with an evidence-backed software gap
  while CI runs, but preserve this exact-head gate.

## Resume update — 2026-11-14 15:51 UTC · action-by-action rehearsal comparison

- Local commit `f89764b7eb9b016fbda35adb171153d7dd27e506` extends Session
  Memory comparison from aggregate counts to per-Action rows. It aligns by
  Cue ID, Cue occurrence order within the Session, and Action ID; includes
  current/compared Cue and Action captions, result, ACK level, ACK-derived
  delivery evidence, latency, safe error code, and one-sided rows for unmatched
  executions. Actions without stable IDs are deliberately not paired. No raw
  adapter response summary is included, and transport completion does not
  imply external execution. `software-completion-matrix.md` now records this.
- The full Core Operator Node suite passed 74/74. Focused session-memory
  timeline tests passed 5/5, `node --check internal/operatorweb/static/memory.js`,
  `go vet ./internal/operatorweb`, and `git diff --check` passed. No backend
  session trace or hardware behavior changed.
- Current local tip before this checkpoint is `f89764b7`; remote/PR remains
  `5b6e1e356f0a02395a2630962a1695df0ba70707`. Core `43611749231` is still in
  Go 1.26 Race; Go 1.27 passed. Companion `43611749229` and Camera Relay
  `43611749232` passed. This SC-015 change and its checkpoint stay local until
  the current Core run finishes.
- Next push, after exact-head Core passes: push the accumulated local
  checkpoint and SC-015 commits normally to `integration/stagecore-rc`, update
  PR #534 with the action-level evidence and exact CI, then wait for all new
  exact-head workflows. Production ACL stays disabled; all external outputs
  remain simulated; Pi 4 performance remains blocked on verified isolated
  access.

## Resume update — 2026-11-14 15:58 UTC · Session Memory browser comparison

- Added `TestBrowserSessionMemoryActionComparison` and its Chrome fixture to
  exercise the actual `memory.js` action comparison within the 560 px mobile
  breakpoint. It checks repeated Cue occurrence/Action matching, one-sided rows,
  all 12 evidence columns, safe ACK wording, exclusion of free-form adapter
  response text, and horizontal scrolling. The fixture contains only static
  simulated records and exposes no execution API.
- Added this browser gate to Core CI's explicit Browser Cue Delivery allowlist
  and recorded its scope in the SC-015 matrix row. Commit:
  `206823c4a8b5e58c9f1acb1c9fde2bb4df0705c5`.
- Local Chrome test passed normally and under `go test -race`; `go vet
  ./internal/httpapi`, browser script syntax, Ruby YAML parsing and
  `git diff --check` also passed. The earlier full Operator Node suite remains
  74/74; no production data, runtime state, hardware, or output was touched.
- Local HEAD is `206823c4`; the accumulated local branch is four commits ahead
  of remote/PR `5b6e1e356f0a02395a2630962a1695df0ba70707`. Exact-head Core run
  `43611749231` is still reported `in_progress` in Race with no conclusion;
  Companion `43611749229` and Camera Relay `43611749232` passed. Remote branch
  remains at `5b6e1e3`, and PR #534 remains OPEN/DRAFT. Do not push until Core
  run `43611749231` completes; then re-read PR/remote SHA and all exact-head
  checks before deciding on a normal push.
- Production ACL remains disabled. Pi 4 ARM64 performance and hardware
  qualification remain pending verified isolated access. Immediate resume:
  inspect the Core Race job state, preserve the exact-head push gate, then
  continue on the highest testable software gap and update the PR/handoff after
  the batch is pushed and verified.

## Resume update — 2026-11-14 16:05 UTC · narrow RTL grant-editor acceptance

- Extended `TestBrowserOwnerProjectAccessEditorStoresPreparedLevel` to run in
  Chrome's narrow responsive breakpoint with the document in RTL. The browser
  now verifies vertically stacked grant fields, no page-wide horizontal
  overflow, naturally focusable form controls, successful `OPERATE` readback,
  explicit `PREPARED_NOT_ENFORCED`, and no publish/runtime/safety request.
- Extended the Session Memory comparison browser report to include the
  measured viewport width and require the narrow breakpoint. Local Chrome
  reports 500 CSS px even with `--window-size=390`; the tests now assert the
  actual measured width is within the 560 px mobile breakpoint rather than
  claiming a 390 px viewport. This Chrome build did not expose keyboard focus
  modality through scripted `.focus()`, so the browser test checks native
  focusability only; actual keyboard activation/focus-ring review remains open.
- Both prepared Owner grant and Session comparison browser gates passed
  normally and under `go test -race ./internal/httpapi -run
  '^(TestBrowserOwnerProjectAccessEditorStoresPreparedLevel|TestBrowserSessionMemoryActionComparison)$'`.
  `go vet ./internal/httpapi`, both browser-script syntax checks, workflow YAML
  parsing and `git diff --check` passed. No production ACL or external output
  was enabled.
- Core run `43611749231` is still active in Go 1.26 Race at remote head
  `5b6e1e356f0a02395a2630962a1695df0ba70707`; Companion and Camera Relay runs
  at that SHA are green. No push has occurred. The source/test/docs edits in
  this update are committed through `85b6024`; confirm exact local/remote state
  and wait for Core before any push. Actual keyboard/touch workflow, full ACL
  endpoint-policy/async review, legacy Companion compatibility, isolated
  installed-copy rollback, and verified Pi 4 performance remain pending.

## Resume update — 2026-11-14 16:08 UTC · SSE Project-payload fence

- Strengthened `TestAuthenticatedSSEClosesAfterSessionRevocation` to parse the
  initial `/api/v1/events` `session.ready` payload, require the exact four
  reviewed fields (`type`, `user`, `hub_id`, `fingerprint`), reject any
  Project/Snapshot/device/session identifiers, and retain its stream-close
  assertion after logout. This is a compatibility guard for the current
  payload-free SSE contract; future Project events still need their own
  delivery authorization review.
- Commit `b30256bc0d11bceb5a2715856a49cf307e40e954`. Focused HTTP API test,
  focused Race test, `go vet ./internal/httpapi`, and `git diff --check` passed.
- Local HEAD at this checkpoint is `b30256b`, seven commits ahead of remote
  `5b6e1e356f0a02395a2630962a1695df0ba70707`. Core run `43611749231` is still
  the exact-head push gate; Companion `43611749229` and Camera Relay
  `43611749232` passed. Do not push until Core completes, then verify the
  branch/PR SHA and all checks before a normal push. Production ACL remains
  disabled and no service, Vault data, or external output was touched.

## Resume update — 2026-11-14 16:30 UTC · SC-016 browser acceptance and close-guard fix

- The exact-head Core CI gate has now completed successfully at remote
  `5b6e1e356f0a02395a2630962a1695df0ba70707`: run `43611749231`, Go 1.26 and
  Go 1.27 both green, including the Go 1.26 Race suite and Linux ARM64 CGo-free
  product builds. Companion `43611749229` and Camera Relay `43611749232` also
  passed. PR #534 remains OPEN/DRAFT at the same remote SHA.
- Added Chrome `TestBrowserCopiesMixedActionsAcrossProjectsAndMapsTargets` for
  two temporary SQLite Projects. It copies Lighting+OSC Actions from a Mixed
  Cue, preserves ordering and payload while assigning fresh Action IDs, maps
  each source target to the destination Project, confirms neither Project's
  target configuration was copied, and observes no publish/runtime/safety call.
  The browser also confirms clipboard selection alone does not warn about
  unsaved work, while an actual Cue name edit still triggers the close guard.
- Fixed the false close warning by comparing serialized Draft state
  (`readDraftHistory(true)`) instead of every form control. Recovery/history/
  preset/clipboard widgets and delayed builder normalization are not Cue edits;
  an unreadable Draft still fails closed and warns.
- Core's six Chrome browser gates passed locally; the new SC-016 browser test
  passed both normally and under `go test -race`. Focused Node suite passed
  19/19, `go vet ./internal/httpapi`, JS syntax checks, Ruby 2.6 YAML parsing,
  and `git diff --check` passed.
- Commit `bce7bc540cc29f04b0631a976e19aa7dd554e417`. Local branch is nine
  commits ahead of remote. No Push has occurred yet. Immediately before the
  next Push, recheck the remote/PR SHA and current checks; the old exact-head
  Core run has completed, so a normal push to `integration/stagecore-rc` is now
  safe. No production service/data, ACL enforcement, or real output was used.
- Remaining SC-016 qualification: operator verification of target
  compatibility before publishing and broader keyboard/touch/RTL acceptance.
  Pi 4 performance still needs verified isolated access. Next independent
  software batch: audit SC-024 grant/revoke behavior and route/session fences
  with temporary stores only; keep production ACL disabled.

## Resume update — 2026-11-14 16:35 UTC · post-push CI and linked PR audit

- Normal Push completed: remote `integration/stagecore-rc` and PR #534 now
  point to `e9ece082debaa303f7cb5f4edf2b9491e4ef41a3` (SC-016 batch
  `bce7bc5` plus this checkpoint). Core run `43616028757`, Companion Core
  `43616028766`, and Camera Relay CI `43616028829` started on that exact SHA
  and are in progress. Do not push another HEAD until these checks complete.
- Read-only linked PR audit: TabletPlayer #36 is OPEN/DRAFT, CLEAN, with Android
  CI green at `91613da`; DMX Lighting #39 is OPEN/DRAFT, CLEAN, all six host/
  firmware jobs green at `3ad7749`; Camera #20 is OPEN/DRAFT, CLEAN, hardware-
  probe CI green at `455b066`; StageLaser #56 is OPEN/DRAFT, CLEAN, host and
  no-actuation firmware CI green at `b14a578`; StageLaser #57 is OPEN, CLEAN,
  host and no-actuation firmware CI green at `c4ff10b`. These PRs were not
  modified. Their remaining physical acceptance is not inferred from CI.
- SC-024 route inventory/dynamic registration tests and the production-shaped
  isolated ACL assembly were inspected. The matrix already records route
  classification, delayed-dispatch fences, independent STOP/Blackout grants,
  Companion/session/upload scope, and explicitly pending full cutover review;
  no concrete safe missing behavior was identified in this read-only audit.
  Production ACL remains unassembled and disabled. Continue only with a
  reproducible, testable software gap.

## Resume update — 2026-11-14 16:40 UTC · StageAudio process lifecycle contract

- Added `internal/stageaudio/mpv_test.go` using a temporary fake `mpv` launcher
  and helper process. It proves two independent child processes and private
  mode-0700 Unix sockets, deck-specific IPC request routing, returned command
  errors, repeatable Close, child reaping and socket-directory cleanup. A
  second-deck startup failure must clean the first deck and leave no runtime
  artifacts. The helper never opens TCP, media output or Bluetooth.
- Focused StageAudio tests passed; complete `go test
  ./internal/stageaudio ./cmd/stagecore-audio -count=1` and full package Race
  tests passed, as did `go vet` on both packages and formatting/diff checks.
  SC-021 matrix now distinguishes this host fake-process evidence from actual
  mpv/Pi/audio qualification.
- Local code commit `051a13f58db880156c8e2e42a560b1641fd68fb7`; local branch is
  two commits ahead of remote `e9ece082debaa303f7cb5f4edf2b9491e4ef41a3` and
  the worktree is clean. No Push has occurred for this batch. The exact-head
  Core run `43616028757` remains in Go 1.26 Race; Go 1.27 and Companion Core
  `43616028766` and Camera Relay `43616028829` passed. Wait for that Core job
  before any new Push.
- Next StageAudio regression: with `mpv` absent from an isolated PATH, assert
  audio stays disabled, returns its explicit unavailable error, and creates
  no socket/runtime directory. Then continue to the next reproducible software
  gap while Pi access and physical audio acceptance remain separate.

## Resume update — 2026-11-14 16:41 UTC · StageAudio missing-binary fallback

- Added `TestMPVBackendMissingBinaryStaysDisabledWithoutRuntimeArtifacts`:
  with a temporary empty PATH, `NewMPVBackend` returns the explicit
  “audio stays disabled” error and creates no child runtime/socket directory.
  Matrix row SC-021 now records the host process lifecycle and no-mpv contracts
  while keeping actual mpv/Pi/audio qualification open.
- Complete StageAudio and sidecar package tests passed normally and under Race;
  `go vet ./internal/stageaudio ./cmd/stagecore-audio`, gofmt, and diff checks
  passed. No host audio service, Bluetooth, or hardware was started.
- Local commit `67f527626b0f61c906dcbc92caa164874c53aa6d`; local is four commits
  ahead of remote `e9ece082debaa303f7cb5f4edf2b9491e4ef41a3`, with a clean
  worktree. Core run `43616028757` at remote `e9ece08` is still in Go 1.26 Race;
  all other exact-head Core/Companion/Camera Relay checks are green. Wait for
  that final Core job, then recheck the remote/PR SHA and push the accumulated
  small batches normally.
- Next independent software gap remains to be selected from the matrix after
  the exact-head CI gate; do not infer Pi ARM64 performance or real audio
  behavior from these host tests. Production ACL stays disabled.

## Resume update — 2026-11-14 16:50 UTC · Capsule portability and Vault reopen

- Added `TestSelfContainedMaterializePreservesExecutionEnvironmentAndExtensionAssets`
  coverage for OSC target Alias portability: ProjectConfig and logical identity
  survive a Show Capsule round-trip, while the host/device-local TargetRef is
  intentionally cleared for remapping. Updated SC-023 in the completion matrix.
  Focused normal and Race tests plus `go vet ./internal/showcapsule` passed.
- Added `TestManagedVaultObjectAndSQLiteMetadataSurviveReopen`. It imports a
  managed media object using temporary SQLite/Vault roots, closes and reopens
  SQLite, verifies `PRAGMA integrity_check`, then confirms Project, Media Asset,
  content-version metadata and exact Vault bytes remain available. Full
  `go test ./internal/vault -count=1`, the focused Race test, `go vet
  ./internal/vault`, formatting and diff checks passed.
- Current local code HEAD is `4f9417695034850166f922250b83ab58a1efdbdc`,
  seven commits ahead of remote `e9ece082debaa303f7cb5f4edf2b9491e4ef41a3`.
  The latest code commits are `a788128` (Capsule alias portability) and
  `4f94176` (Vault reopen regression). No push has occurred.
- PR #534 remains OPEN/DRAFT at remote `e9ece08`. Core run `43616028757` remains
  in Go 1.26 Race; Go 1.27 passed. Companion Core `43616028766` and Camera
  Relay `43616028829` passed on the same SHA. Wait for the Core run to finish,
  then recheck the remote branch and PR SHA before a normal push; monitor CI for
  the newly pushed exact head.
- The `stagecore-qualification` SSH alias timed out during banner exchange
  (`ConnectTimeout=8`); no remote command ran. Pi 4 engine performance remains
  pending; do not substitute Mac results or ARM64 cross-builds. Production
  services, Vault/data, ACL enforcement, physical outputs and firmware remain
  untouched.
- Next task: after the exact-head CI gate, push the seven preserved commits
  without force, monitor their CI, then continue the highest reproducible
  software gap from the completion matrix. Keep Project ACL production-off and
  PR #534 draft/unmerged.

## Resume update — 2026-11-14 16:58 UTC · Project duplicate reference safety

- Reproduced a concrete SC-023 copy bug before fixing it: Project duplication
  recursively rewrote every JSON string equal to a cloned ID, including a
  `filename` and an unrelated literal setting. Updated the traversal to remap
  only recognized identity fields (`media_asset_id`, `content_version_id`,
  `machine_role_id`, `live_source_id`); identity references still clone while
  literal values remain unchanged. The Project duplicate regression proves
  both behaviors.
- `go test ./internal/store -run 'TestDuplicateProject' -count=1`, focused
  duplicate Race tests, `go vet ./internal/store`, full
  `go test ./internal/store -count=1`, and full `go test -race
  ./internal/store -count=1` passed. The complete store Race run took 177.107 s.
  `git diff --check` passed.
- Core CI `43616028757` completed successfully on remote `e9ece08`; Go 1.26 and
  Go 1.27 checks, Race, Browser Cue Delivery and Linux ARM64 CGo-free builds all
  passed. Companion Core `43616028766` and Camera Relay `43616028829` passed on
  the same exact head. Local code HEAD is `5d227167ecbb24cc8065a13f9b3bd2a052cb2fc8`,
  nine commits ahead of remote. No push has occurred; worktree was clean before
  this documentation update.
- Before push, verify remote `integration/stagecore-rc` and PR #534 still point
  to `e9ece082debaa303f7cb5f4edf2b9491e4ef41a3`. If unchanged, push the
  accumulated commits normally to the existing branch and monitor the exact
  new-head Core/Companion/Camera checks. Never force-push.
- The Pi qualification SSH alias still fails during banner exchange; no remote
  command has run. Pi P95/P99, jitter, CPU/RAM, schedule load and sustained
  rehearsal remain pending. No production service/data, real output, firmware,
  or production ACL was touched.

## Resume update — 2026-11-14 17:00 UTC · Push to existing PR #534

- After confirming the remote branch and PR both remained at
  `e9ece082debaa303f7cb5f4edf2b9491e4ef41a3` and its Core CI completed green,
  pushed the accumulated ten commits normally. No new branch/PR or force push.
- The remote branch, PR #534 and local source HEAD now agree at
  `a90403b863009081447d1cb2ebbce88947ad1cf6`. PR remains OPEN/DRAFT. Exact-head
  runs started at 16:59:23 UTC and are active: Core `43618591866`, Companion
  Core `43618591864`, Camera Relay `43618591875`. Wait for all three before any
  further push.
- The latest code batch fixes Project duplication's overbroad JSON ID rewrite.
  Its regression first failed because a UUID-valued `filename` was changed,
  then passed after limiting remapping to recognized media/role/live-source
  identity fields. Full `internal/store` normal and Race suites, duplicate
  regressions, Vet, Show Capsule checks and Vault reopen/integrity checks passed.
- Pi qualification remains blocked by SSH banner timeout; no remote command or
  device action occurred. No production service/data/Vault, real stage output,
  firmware, or production ACL enforcement was changed. Keep #534 draft and do
  not merge/force-push.

## Resume update — 2026-11-14 17:43 UTC · Pi 4 ARM64 performance campaign

- Branch `integration/stagecore-rc`; Draft PR #534 remains open. The last
  pushed Remote/PR SHA is `a90403b863009081447d1cb2ebbce88947ad1cf6`. Its
  exact-head Core run `43618591866` passed Go 1.26 (including Race and Linux
  ARM64 builds), Go 1.27, Browser and Vet; Companion Core `43618591864` and
  Camera Relay `43618591875` passed. The local branch starts this docs batch
  from checkpoint commit `2675428`; use `git rev-parse HEAD` for the current
  local tip. No source changes are part of this performance-doc batch.
- Pi 4 Model B Rev 1.4, Debian 13.6 `linux/arm64`, Linux 6.18.39, Go 1.27.1.
  Test binaries were built from code `a90403b`; full binary hashes and sandbox
  setup are in `docs/architecture/cue-execution-performance-qualification.md`.
  Each run used unprivileged private user/network/mount/PID namespaces at nice
  +19; `/dev`, `/opt`, `/var/lib`, `/home`, and `/run` were hidden by tmpfs.
  Host CPU/memory were not cgroup-isolated. The simulator was the only output
  adapter and the guarded physical-executor count stayed zero.
- Correctness passed for the six-Cue/24-Action nested compound chain, eight-GO
  burst, linked 100-Cue/200-Action reference project, and 100-Cue/200-Action
  sustained Rehearsal. Performance runs covered 24-Action Mixed/linked cases
  (1,000 samples each with Go CPUs 1 and 4), 50/100/200-Action and
  25/50/100-Cue scale cases (100 samples), 4/8-GO bursts (100 bursts, CPUs
  1/4), STOP/Blackout against 1/8/32 delayed GO requests (100 operations),
  and 20 sustained reference rehearsals. Exact P50/P95/P99, latency/schedule
  jitter, allocations, GC, and command resource totals are in the performance
  report.
- Notable Pi samples: flat Mixed 200 Actions P50/P95/P99
  `468.0/480.4/492.5 ms`; linked 100 Cues/200 Actions
  `500.1/531.8/588.1 ms`; scheduled linked 25/50 schedule jitter
  `8.788/41.201/75.443 ms`; 8-GO burst P95 `593.5 ms` (`-cpu=1`) and
  `870.7 ms` (`-cpu=4`); STOP/32 delayed Cues P95/P99
  `998/1,134 ms`; Blackout/32 `1,028/1,059 ms`. Sustained 100-GO session
  mean was `2.998 s`; per-GO P50/P95/P99 was `30.02/39.06/41.44 ms`.
  These are low-priority samples, not SLO or hardware acceptance; P99 from
  100-sample cases is the top sample, and no numeric pass threshold was given.
- CPU profile samples show SQLite VDBE execution at 41.66%, Store event append
  at 33.83% cumulative, and Linux syscalls at 7.16% flat. The allocation
  profile included fixture creation; `readCueDraftStateTx` is fixture setup
  outside the timed GO boundary and is not labeled an execution bottleneck.
  No cache, event batching, or durability change was made; a performance
  optimization must preserve persisted event order and be measured in a
  scoped profile first.
- This report update plus matrix/handoff changes have passed `git diff
  --check`; no production service/data/Vault, ACL enforcement, real output,
  firmware, or internet-dependent test was used. Remote/PR remain at `a90403b`.
  Before normal push to #534, recheck remote/PR SHA and CI status. Do not
  force-push, merge, enable production ACL, or interrupt a new exact-head run.
- First next independent task: inspect and run Linux-tagged `internal/usbdmx`
  host tests as an ARM64 binary inside the same device-hidden Pi namespace,
  confirming no serial-device path can be opened. Then continue a reproducible
  gap from the completion matrix; SC-009 Route history and SC-015's lack of
  explicit per-output health events remain software-review candidates.

## Resume update — 2026-11-14 17:56 UTC · simulated USB-DMX refresh load

- Remote/PR/local committed HEAD remains `b1b3cb4198e109fe4807f1250f4e54a58cd0b084`.
  PR #534 is OPEN/DRAFT. Exact-head Core run `43622805955` is active: Go 1.27
  passed; Go 1.26 remains pending. Companion Core `43622805947` and Camera
  Relay `43622805957` passed. **Do not push while Go 1.26 is running.** The
  local working batch contains an uncommitted test-only benchmark and updates
  to the performance report, matrix and this handoff; `git status` is the
  authoritative list of those edits.
- SC-025's Linux tests were rebuilt as a static ARM64 test binary from current
  source. Final binary SHA-256:
  `a85673eb0c191d5e9a8192d8fa642ddae231303892860aca532d154151643fb4`.
  The entire `internal/usbdmx` suite passed in the Pi user/network/mount/PID
  namespace at nice +19, with `/dev`, production paths and Hub socket hidden;
  elapsed 1.120 s, user/system CPU 0.021/0.019 s, max RSS 10,784 KiB. Linux
  pipe/poll/write behavior, cancellation, and disconnect handling ran through
  the default Linux syscalls without a USB device.
- Added `internal/usbdmx/refresh_performance_benchmark_test.go`. It exercises
  the real continuous `ManualOutput` ticker with 100 intervals per case and
  CPU-worker contention, writing only to an in-memory `PacketPort`. Final
  ARM64 Pi run used Go 1.27.1, `-test.benchtime=1x -test.cpu=1,4`: CPU 1
  refresh P50/P95/P99 `250.3/253.7/253.7 ms`, jitter P50/P95/P99
  `0.454/4.015/4.017 ms`, 12,720 B and 34 allocs; CPU 4 refresh
  `250.3/253.7/253.8 ms`, jitter `0.581/4.015/4.207 ms`, 1,552 B and
  19 allocs. Both had zero GC. Command resource totals: 50.029 s elapsed,
  148.686 user CPU seconds, 0.181 system CPU seconds, 10,792 KiB peak RSS.
  The parallel synthetic workload explains CPU time exceeding wall time.
  No USB serial port, DMX line, fixture, or physical output was used.
- Local verification passed after the final source edit: `gofmt`,
  `git diff --check`, `go test ./internal/usbdmx -count=1`,
  `go test -race ./internal/usbdmx -count=1`, `go vet ./internal/usbdmx`,
  Race-instrumented benchmark (`-benchtime=1x -cpu=1`, 26.196 s), Linux ARM64
  cross-build, complete Pi ARM64 test binary, and the final Pi refresh
  benchmark. This is software cadence evidence; actual USB transport and
  fixture-level refresh/blackout remain Hardware Qualification Pending.
- The P95/P99 sample count is 100 intervals per Go CPU setting. Jitter is
  absolute deviation from that setting's median interval. No timing threshold
  was supplied, so do not turn these numbers into an SLO or production
  acceptance claim. SC-024 stays production-off; no production service, data,
  Vault, ACL, serial device, firmware, or physical output was touched.
- Correction to the next-task note in this historical checkpoint: SC-009 #488
  concerns Cue/Action Draft recovery; Route history is explicitly outside its
  scope and is not a justified SC-009 implementation candidate. The performance
  and USB-DMX batch was committed locally as `ecedb3e`; retain it for the next
  normal push after exact-head Core CI completes.

## Resume update — 2026-11-14 18:17 UTC · scoped Project media upload

- Branch `integration/stagecore-rc`; Draft PR #534 remains OPEN. Remote/PR HEAD
  is `b1b3cb4198e109fe4807f1250f4e54a58cd0b084`; local HEAD is
  `acd46db34414f04337f54305ecd611d270c01395`. Local commits ahead are
  `ecedb3e` (test-only USB-DMX refresh jitter benchmark and evidence) and
  `acd46db` (SC-023 Project file upload). The worktree was clean after both
  commits. No force-push, merge, production service/data/Vault, production ACL,
  real device or firmware action was used.
- Exact remote-head Core run `43622805955`: Go 1.27 passes; Go 1.26 remains
  pending. Companion run `43622805947` and Camera Relay run `43622805957` pass.
  Do not push the two local commits until all exact-head checks pass. Before
  push, verify branch/PR still point to `b1b3cb4` and query all required checks;
  push normally to the same PR, then wait on the new exact-head workflows.
- SC-023 now has a separate opt-in `POST /api/v1/projects/{project_id}/files`
  endpoint and Files UI upload form. It requires CSRF and live Project
  `media.upload`, rechecks exact browser Session and grant around each streamed
  body chunk, caps uploads at 1 GiB, validates MIME syntax and encoded names,
  streams through the existing Vault's capacity check/SHA-256 staging, and
  registers only managed Project media. Failed registration/revocation removes
  an unreferenced object. File bytes are not semantically decoded; the UI
  category remains a filename-extension label. The option is not assembled in
  production; read/upload/download stay separate opt-ins and SC-024 remains
  production-off.
- Regression tests verify opt-in/default-off behavior, CSRF, missing upload
  grants, malformed MIME/name rejection, basename normalization, exact Vault
  bytes/hash, list/download, cross-Project denial, grant revocation mid-stream,
  request-size rejection, browser asset embedding, route inventory and the
  production ACL activation guard. The route inventory digest is
  `7733f1e0c01f0affed9cffdbb0b6ebf52c725108a11560e2fb7aba6b891c8907`.
- Local checks passed: focused HTTP API tests including route/inventory guards,
  focused `go test -race` for both upload cases, `go vet ./internal/httpapi`,
  `node --check internal/operatorweb/static/project-files.js`, and
  `git diff --check`. Earlier Pi 4 USB-DMX ticker and Linux ARM64 simulations
  remain recorded above; they were not rerun for this API/UI batch.
- SC-023 remains **Partial**: content-level media-format validation, per-Project
  quotas, delete/retention/ref-count UX, explicit Capsule portability for
  draft/unreferenced files, and broader Capsule/ACL/copy qualification remain.
  First next task after CI: inspect Capsule media inclusion policy and the
  supported file-type contract, then close a reproducible software gap with
  isolated tests. Do not reopen SC-009 Route history as a candidate; do not
  repeat completed Mac/Pi performance cases without a specific reason.

## Resume update — 2026-11-14 18:23 UTC · MIME category gate and CI release

- Remote/PR HEAD remains `b1b3cb4198e109fe4807f1250f4e54a58cd0b084`;
  local code HEAD before this checkpoint is
  `3e7960bd2f1e6a9bf8e32798cabaeb4852fea49f`. Four local commits are ahead:
  `ecedb3e` (USB-DMX refresh jitter evidence), `acd46db` (opt-in scoped Project
  media upload), `6d5c519` (prior resume checkpoint), and `3e7960b` (known
  extension/MIME category compatibility). The matrix change and this checkpoint
  still need their own documentation commit. Re-read `git rev-parse HEAD` for
  the post-checkpoint tip.
- Exact remote Core run `43622805955` completed successfully on `b1b3cb4`:
  Go 1.26 and 1.27 pass. Companion run `43622805947` and Camera Relay run
  `43622805957` also pass. These runs tested only their exact remote source
  SHA; they do not qualify the local commits. Recheck PR SHA/checks immediately
  before a normal push, then follow all new exact-head CI to completion.
- Upload validation now rejects known AUDIO/VIDEO/IMAGE filename categories
  whose declared MIME top-level differs; syntactically valid
  `application/octet-stream` remains accepted for browser/format ambiguity.
  It does not decode file bytes, and downloads remain forced attachments with
  `nosniff`. Focused projectaccess and HTTP tests, upload Race tests, route
  inventory/production-activation guards, `go vet`, JS syntax, and diff checks
  passed after this change.
- Read-only companion PR survey found current heads/checks: TabletPlayer #36
  `91613da` pass; ESP32 DMX #39 `3ad7749` all six checks pass; ESP32 Camera #20
  `455b066` software hardware-probe check passes; StageLaser #56 `b14a578` both
  no-actuation/host checks pass; StageLaser #57 `c4ff10b` both checks pass
  (this PR is OPEN but not Draft). No sibling checkout existed in this workspace
  to inspect or modify; no other repository was changed.
- Pending SC-023 work: decide and test Capsule behavior for uploaded
  unreferenced/draft Project files, assess per-Project quotas and retention, and
  extend supported content verification based on demonstrated operator needs.
  Continue to treat Hardware Qualification and production approval as separate
  gates; StageCore ACL/SC-024 remains off in production.

## Resume update — 2026-11-14 18:30 UTC · Capsule file portability warning

- Remote/PR HEAD is `1418cb780b661a33fb82c012bd9a764a16782099`; this is also the
  last pushed local commit. Exact-head Core run `43626455867` completed **fail**
  on both Go versions because `TestProjectFilesProgressiveScopedUIContract`
  still required the old `payload?.read_only` and “Read-only” strings after the
  authorized upload form was added. All other Core packages passed, including
  `internal/httpapi`, `internal/showcapsule`, `internal/projectaccess` and
  `internal/usbdmx`; Companion `43626455728` and Camera Relay
  `43626455739` passed. Do not report this HEAD green.
- Updated the stale browser contract to assert scoped, capability-gated upload
  UI and the encoded filename request. The focused `internal/operatorweb`
  contract now passes. This repairs a test expectation; it does not change
  runtime authorization.
- Continued SC-023 Capsule review. A Project Files upload not referenced by
  the selected published Snapshot was previously silently omitted from a
  self-contained Capsule. Capsule preview now counts those versions in SQL and
  warns that draft, tablet-only and other unreferenced media must be transferred
  separately. Snapshot-referenced optional media still participates in the
  Capsule. An “include every workspace file” mode is intentionally not added:
  the immutable Snapshot is the Capsule boundary, avoiding an unreviewed export
  of post-publish uploads. Regressions verify warning/count/exclusion and
  optional referenced media inclusion.
- Local checks for the Capsule change passed: full `go test ./internal/showcapsule
  ./internal/store -count=1`, focused Capsule Race tests, `go vet` for both
  packages, and diff checks. The updated Operator web contract also passes.
  Current changes are not committed or pushed yet; `git status` and
  `git rev-parse HEAD` are authoritative. After committing this batch, recheck
  all exact-head state before a normal push to PR #534. Then wait for Core,
  Companion, and Camera Relay CI on the new SHA; continue other work while CI
  runs without replacing that SHA.
- Next SC-023 software gaps remain file-byte decoding/signatures, per-Project
  quotas, and delete/retention/ref-count UX. Hardware qualification and
  production release remain separate; production Hub/Relay/data/Vault and ACL
  remain untouched/off.

## Resume update — 2026-11-14 18:32 UTC · repaired Core web contract

- Remote/PR HEAD is still `1418cb780b661a33fb82c012bd9a764a16782099` with a
  completed Core failure on Go 1.26 and 1.27. The only failing assertion was
  `TestProjectFilesProgressiveScopedUIContract` expecting read-only UI text and
  not recognizing the conditional upload form. Companion and Camera Relay
  checks on this same SHA passed.
- Local commit `b92bde82cae7ab8dbac055afbb2e4c003bd53659` now contains the
  repaired web contract and the Capsule portability warning. It is one commit
  ahead of remote; the handoff update itself is still a documentation change.
- The capsule preview counts Snapshot-unreferenced Project media versions in a
  bounded SQL aggregate (without loading Vault bytes) and warns when those
  uploads are omitted. Self-contained export still includes optional media
  referenced by that Snapshot. The optional include-every-file mode is not
  offered; such files transfer separately from Project Files. Tests passed for
  both omission/warning and optional inclusion.
- Latest local validation passed: `go test ./internal/showcapsule
  ./internal/store ./internal/operatorweb -count=1`, focused Race tests for
  omitted/optional Capsule media, `go vet` for the three packages, and
  `git diff --check`. No new remote CI run is active on the failed SHA.
- Before next push: commit this resume update, verify remote branch and PR still
  point to `1418cb7`, then normally push the local fix to the same Draft PR.
  Wait for fresh Core/Companion/Camera Relay checks on the new SHA; do not infer
  green from the prior passing Companion/Relay runs. Continue remaining SC-023
  work while CI runs without changing the pushed SHA.

## Resume update — 2026-11-14 18:34 UTC · pushed Capsule/contract repair

- Normal push updated the same Draft PR #534 and branch to
  `9df291527dafd0ec8ab52d6b801d03566dcf4528`; local HEAD, origin ref, remote
  branch and PR head were verified equal. Worktree was clean immediately after
  push. No force-push or merge.
- New exact-head workflows on `9df2915` are queued/running: Core
  `43627228963`, Companion Core `43627229094`, Camera Relay `43627228900`.
  Their initial checks are pending. The preceding Core run on `1418cb7` remains
  failed for the now-updated static UI contract; its Companion and Camera Relay
  checks passed but do not replace the new exact-head runs. Do not push another
  SHA until the three new runs finish.
- The pushed fix updates the stale upload UI contract and adds Capsule warnings
  for uploaded file versions not referenced by the published Runtime Snapshot.
  Optional referenced media remains included. Focused package, Race, vet, and
  `git diff --check` evidence is in the prior checkpoint section.
- Continue with read-only SC-023 quota/retention audit or another independent
  evidenced software gap while CI runs. Keep all production surfaces and ACL
  disabled/off; no device activity was used.

## Resume update — 2026-11-14 18:41 UTC · Project media usage summary

- Local HEAD is `d1d5e3af9f8663aa0ba58f949dbea03ca34aa640`; remote branch and PR
  #534 remain at `9df291527dafd0ec8ab52d6b801d03566dcf4528`. The local commits
  `4eaade8` (prior handoff checkpoint) and `d1d5e3a` (this feature/matrix update)
  are not pushed. The exact-head Core run must finish before any push.
- On `9df2915`, Core run `43627228963` has Go 1.27 pass and Go 1.26 still
  pending. Companion run `43627229094` passes both macOS replacement/media and
  Swift CompanionCore; Camera Relay `43627228900` passes isolated tests/ARM64.
  PR is OPEN/Draft and its head is verified as `9df2915`.
- Added `entry_count` and `total_bytes` to the scoped Project Files workspace.
  The aggregate counts logical entries and sums immutable file-version sizes;
  content-addressed Vault blobs may be physically shared. The bilingual Files
  page shows total entries versus the current page and labels the logical size
  clearly. This remains behind the explicit scoped-read option and is not
  assembled in the production Hub. No quota limit was invented.
- Updated the SC-023 matrix. It now distinguishes global Preflight/disk-health
  warnings from the still-open per-Project quotas and delete/retention/ref-count
  UX. Tests verify empty/populated/duplicated/renamed workspace totals,
  pagination, the HTTP JSON response, UI contract and Arabic/English labels.
- Final local checks passed: `go test ./internal/projectaccess
  ./internal/operatorweb -count=1`; focused HTTP files-route test; focused
  Project Files Race tests; `go vet ./internal/projectaccess
  ./internal/operatorweb`; `node --check` for `project-files.js`; and
  `git diff --check`. An initial test assertion shadowed the outer pagination
  result; it was corrected and the full focused set then passed.
- Rechecked related open PRs: TabletPlayer #36 `91613da` pass; ESP32 DMX #39
  `3ad7749` all six checks pass; ESP32 Camera #20 `455b066` check passes;
  StageLaser #56 `b14a578` both checks pass; StageLaser #57 `c4ff10b` both
  checks pass and remains non-Draft. No sibling checkouts are present and no
  related repository was modified.
- Pi performance caveat: the ARM64 measurements in the qualification report
  used source `a90403b`; the later `c6d4407` changed linked-Cue graph
  validation. Requalification of that code on isolated Pi 4 is justified, but
  the recorded `stagecore-qualification` SSH endpoint failures returned
  `websocket: bad handshake` before a shell opened. No command ran on the Pi;
  do not describe the post-optimization code as Pi-qualified or repeat a failed
  access attempt without a changed endpoint/credential condition.
- Next: wait for Go 1.26 on `9df2915`. If it passes, verify branch/PR head again
  and push the two local commits normally to the existing branch; then wait for
  new exact-head Core/Companion/Relay runs. If it fails, repair that failure
  first. Continue the SC-023/SC-024 software audit while keeping production ACL
  disabled and all device/output tests simulated.

## Resume update — 2026-11-14 18:48 UTC · localized usage UI regression

- Local HEAD is now `da76d9013527719ea96e79966f29776d6e7cdacd`, four commits
  ahead of remote `9df291527dafd0ec8ab52d6b801d03566dcf4528`; PR #534 remains
  OPEN/Draft at the remote SHA. The two earlier docs/code commits and two
  checkpoint/test commits are all local. No push was made while exact-head Core
  CI was in progress.
- Core run `43627228963` remains in progress: Go 1.27 passes; Go 1.26 is still
  running. Companion `43627229094` and Relay `43627228900` remain green. The
  exact PR and `git ls-remote` head were rechecked at `9df2915`.
- Added `tests/project_files.test.js` and included it in Core CI's Node test
  command. It executes the actual Project Files render function under a DOM
  shim and verifies English/Arabic total-entry labels, 3 GiB formatting, and
  the note that content-addressed Vault bytes can be shared. `node --test
  tests/*.test.js` passed all 77 tests locally. The test was checked/fixed for
  the browser globals (`dataset`, `URLSearchParams`) required by the actual UI.
- The Project Files UI and API changes are qualification-only and remain
  separately opt-in. SC-023 #510 asks for streaming/capacity quotas but gives no
  capacity value or allocation rule. Keep the measured logical version-byte
  total; do not turn it into an arbitrary admission limit. Per-Project quota
  policy and delete/retention/ref-count UX remain explicit open work.
- Next: continue waiting for Core Go 1.26 on `9df2915`; then verify the remote
  head again before normally pushing all local commits to the existing branch.
  New exact-head CI must finish before another push. Do not retry the known Pi
  SSH endpoint failure unless its endpoint/credential condition changes.

## Resume update — 2026-11-14 18:59 UTC · Chrome Project Files acceptance

- Local HEAD is `d5e1e944784942fee25c23efe5e0f214b6a3bcd2`, seven commits ahead
  of remote/PR #534 `9df291527dafd0ec8ab52d6b801d03566dcf4528`. The remote PR
  head is still OPEN/Draft at that exact SHA. Core's current exact-head Go 1.26
  job remains inside its full Race suite; no Push has been made.
- Added `TestBrowserScopedProjectFilesShowsUsageAndPageCounts` and included it
  in Core CI's Chrome gate. It starts headless Chrome at 390×844 against the
  opt-in authenticated scoped-read route and real `project-files.js`/`app.css`,
  checks Arabic RTL plus English LTR labels and 2 GiB logical summary, page
  category counts, keyboard focus, no page overflow, and absent upload/download
  controls without their separate grant/options. It uses temporary SQLite and
  synthetic Vault metadata only; it sends no media or device operation.
- The first browser attempt exposed the test harness's inline-script/CSP
  conflict. Setup was moved into a same-origin external test script allowed by
  the production policy. The actual Chrome gate then passed locally. Focused
  HTTP read/revocation tests passed, the same tests passed under Race,
  `go vet ./internal/httpapi`, Node usage UI tests, and `git diff --check` all
  passed after the fix.
- Updated the SC-023 matrix with this browser evidence. No production assembly
  or ACL activation changed. No other repository or hardware was touched.
- Next: wait for the Core Go 1.26 Race job on `9df2915` to finish, recheck every
  check and exact PR/remote head, then normally push the seven local commits to
  the existing branch if all are green. Wait for fresh checks on the new SHA
  before any further push. Pi qualification remains open at the previously
  documented SSH endpoint failure.

## Resume update — 2026-11-14 19:04 UTC · revoke fence for usage totals

- Local HEAD is `6d9fda8cd6b1a741bc8e861461747d304cca2468`, nine commits ahead
  of origin and PR #534 head `9df291527dafd0ec8ab52d6b801d03566dcf4528`.
  PR state remains OPEN/Draft. No push or remote mutation occurred.
- Added a private managed-media fixture to
  `TestScopedProjectFilesRechecksGrantAtResponseDelivery`. For both grant and
  session revocation, the route must return the denial status without leaking
  the file name, `entry_count`, or `total_bytes`. This covers the new usage
  aggregate at the exact response boundary.
- The response-boundary test passed normally and under `go test -race`; focused
  HTTP tests, `go vet ./internal/httpapi`, and `git diff --check` passed. The
  headless Chrome gate and Node UI tests also passed in the previous checkpoint.
- The currently running remote Core run `43627228963` is still in its Go 1.26
  full Race step (started 18:37 UTC); Go 1.27, Companion/Swift and Relay remain
  green. Do not push until this exact-head run completes, then verify all checks
  and branch/PR SHA before deciding to push.

## Resume update — 2026-11-14 19:11 UTC · pushed scoped media usage batch

- Core run `43627228963` completed green, including Go 1.26 full Race in
  36m46s; Go 1.27, both Companion checks and Camera Relay were also green on
  exact prior head `9df2915`.
- Rechecked clean local state, remote branch, and OPEN/Draft PR #534 at
  `9df291527dafd0ec8ab52d6b801d03566dcf4528`, then normally pushed the ten
  accumulated local commits. No force-push or merge. The push advanced the
  branch to `61fd27e05ddad6fc3189318de2a3fd2c77fa50e5`; fetch, `ls-remote`, PR
  API and local `origin` now agree exactly and the worktree is clean.
- New exact-head CI is active: Core `43630696006`, Companion `43630695925`, and
  Camera Relay `43630695973`. Initial Go 1.26/1.27, macOS/Swift and ARM64-isolated
  checks are pending. Do not push another SHA until all checks on `61fd27e`
  complete.
- Pushed code adds scoped Project Files logical entry/byte totals, bilingual
  usage and current-page labels, and Node plus headless Chrome tests. It adds a
  revoke-at-response regression proving the new aggregates do not leak after
  session/grant revocation. All Project Files routes/options remain
  qualification-only; production ACL and production assemblers were not changed.
- Local validation passed before push: full Node `tests/*.test.js` (77 tests),
  focused Project Files Store/Operator UI/API tests, focused HTTP Race tests,
  `go vet`, Node syntax checks, and a real local headless Chrome run at 390×844
  for Arabic RTL and English LTR. No output hardware or production data was used.
- Next: wait for all new exact-head checks. If green, continue the SC-023 quota
  and retention design only where acceptance policy is defined, then proceed
  with the remaining SC-024 whole-server audit. Pi ARM64 performance after the
  `c6d4407` graph-validation optimization is still pending because the recorded
  isolated SSH endpoint failed before shell access; do not substitute Mac or
  cross-build results. Keep production ACL disabled and do not activate SC-024.

## Resume update — 2026-11-14 19:16 UTC · new Core browser gate passed

- On pushed exact head `61fd27e`, Go 1.27 Core passed in 3m31s. Go 1.26's full
  test suite, the Browser Cue Delivery gate (including the new Project Files
  Chrome test), and Vet have passed; its full Race suite is running now. The
  isolated/ARM64 Relay and both Companion checks also pass. No new code push
  occurred while Race runs.
- Local HEAD is `967901afa14652192382dcc7cbaacced9f3a27ed`, a single local
  handoff commit ahead of the clean remote `61fd27e`. The next action remains to
  wait for Go 1.26 Race and ARM64 build, verify the exact branch/PR head, then
  continue the campaign with the remaining policy-defined gaps.

## Resume update — 2026-11-14 19:18 UTC · explicit logical byte field

- Renamed the new Project Files response/UI field from `total_bytes` to
  `logical_bytes`; the value is the sum of immutable file-version sizes and is
  not a measurement of deduplicated physical Vault consumption. This makes the
  additive qualification API's semantics explicit.
- Local commit `add04d2aa639c5dbc95d7646fb1d26403441b73f` is validated and
  committed. Local HEAD is three commits ahead of remote `61fd27e` (the two
  handoff updates and this API naming refinement); worktree is clean.
- After the rename, focused Store/UI/API tests, Project Files HTTP and Store
  Race tests, `go vet`, Node UI tests/syntax, and the actual local 390×844 Chrome
  gate passed. No production route assembly changed.
- The current pushed-head CI on `61fd27e` is still in the Go 1.26 Race suite;
  Go 1.27, all Companion checks and Relay are green. Keep all three local
  commits unpushed until this exact-head run and ARM64 build complete.

## Resume update — 2026-11-14 19:38 UTC · bounded Project Files search

- Implemented server-side search for Project asset names and original
  filenames. Queries are UTF-8 validated, reject Unicode controls and more
  than 128 characters, treat LIKE wildcard characters literally, and bind
  pagination cursors to the normalized query. `match_count` reflects filtered
  rows while `entry_count` and `logical_bytes` remain whole-Project totals.
  The opt-in HTTP route and bilingual Project Files page pass the query;
  refresh, pagination, upload refresh and clear preserve/reset it correctly.
- Added Store tests for literal wildcard/backslash matching, name and filename
  queries, project-wide totals, cursor continuation/query binding and invalid
  bounds; HTTP test for search and overlength rejection; Node UI assertions;
  and extended the actual 390×844 headless Chrome gate to search and clear in
  Arabic RTL and English LTR. Updated the SC-023 matrix. Quotas and deletion/
  retention policy remain open because #510 supplies no quota allocation or
  retention policy; no arbitrary capacity limit was introduced.
- Commit `725d3ff` (`feat(project-files): add bounded server search`) is local.
  Local HEAD is `725d3ff`, five commits ahead of origin/PR #534 at
  `61fd27e`; the current worktree should be clean after this checkpoint commit.
- Validation passed: full `go test ./...`, full `go vet ./...`, focused Project
  Files Race tests, full Node `tests/*.test.js` (78 tests), Node syntax and
  `git diff --check`, plus the local headless Chrome Project Files browser
  test. No device, production service or production data was used.
- At 19:38 UTC, exact-head Core run `43630696006` on `61fd27e` still had its
  Go 1.26 full Race suite in progress; the Go 1.26 tests/browser/Vet, Go 1.27,
  Companion and Relay jobs were green, with ARM64 product builds pending. Do
  not push until that full run and ARM64 build complete. Then recheck the
  remote branch and Draft PR #534 head, push normally, and wait for CI on the
  new exact head. Keep Project Files and SC-024 opt-in/off in production.
- Pi 4 ARM64 performance requalification after linked-Cue graph validation
  optimization remains blocked by the unchanged `websocket: bad handshake`
  qualification SSH endpoint, which failed before a shell or command. Do not
  claim qualification or retry unless the endpoint/credential changes.

## Resume update — 2026-11-14 19:43 UTC · search batch pushed

- Correcting the immediately preceding pre-push note: after checkpoint commit
  `569247e`, local HEAD was `569247ed8090f42a54b833d02608917b188a75ff`, not
  `725d3ff`. The search feature is `725d3ff`; `569247e` records its evidence.
- The older exact-head Core CI `43630696006` on `61fd27e` completed green,
  including Go 1.26 full Race and Linux ARM64 product builds; Companion and
  Camera Relay runs `43630695925` and `43630695973` also completed green.
- Rechecked clean local state, `git ls-remote`, `origin`, and OPEN/Draft PR #534
  at `61fd27e`, then pushed six accumulated local commits normally. No force
  push or merge. Branch, local tracking ref, remote branch and PR now agree on
  `569247ed8090f42a54b833d02608917b188a75ff`; worktree is clean.
- New exact-head runs are active: Core `43633596721`, Companion `43633596728`,
  and Camera Relay `43633596765`. Do not push another change until all three
  finish. First action after resuming is inspect those results and exact PR
  head; if green, continue the SC-024 route-policy audit and do not re-run
  already qualified Mac performance measurements.

## Resume update — 2026-11-14 19:46 UTC · match-count revoke regression

- Tightened `TestScopedProjectFilesRechecksGrantAtResponseDelivery` to assert
  that the new `match_count`, like filenames and whole-Project aggregates, is
  absent after either grant or browser-session revocation. Updated the SC-023
  evidence matrix. The focused HTTP test passed normally and under Race.
- The `569247e` CI is still active on Core `43633596721`; Companion
  `43633596728` and Camera Relay `43633596765` have completed green. Do not push
  until Core also completes successfully.
- Local HEAD before this checkpoint entry is
  `d27fcf67a2b93df1f2570a4e2f2392ffe51f2fa4`, two commits ahead of the clean
  origin/PR #534 head `569247ed8090f42a54b833d02608917b188a75ff`; the two commits
  are `fdca32d` (push-state checkpoint) and `d27fcf6` (revocation regression).
  The worktree is clean before committing this entry.
- Next: inspect Core run `43633596721`, then recheck local/remote/PR state. If
  green, normally push the local checkpoint/test commits, wait for new exact-
  head CI, and continue the SC-024 route-policy review. Production ACL remains
  disabled; Pi 4 qualification remains unavailable at the unchanged failed
  isolated SSH handshake.

## Resume update — 2026-11-14 19:49 UTC · search-response revocation case

- Extended the revocation-at-response regression to call the actual filtered
  `/files?q=private` route for both live grant removal and browser-session
  revocation. The response must suppress the matching filename, `match_count`,
  project totals and byte totals. The test passes normally and under Race.
- Committed as `f2f3f59` (`test(project-files): fence revocation during
  search`). Local HEAD before this entry is
  `f2f3f593052009b3427f077dad39ee128e6e0dd7`, four commits ahead of
  clean origin/PR `569247e`; the earlier three local commits are `fdca32d`,
  `d27fcf6`, and `42bb90c`. Worktree is clean before committing this update.
- Core run `43633596721` on `569247e` remains in Go 1.26 full Race; Go 1.27,
  Companion `43633596728`, and Relay `43633596765` are green. Do not push until
  the exact-head Core run completes. Then recheck PR/remote and push normally.

## Resume update — 2026-11-14 19:59 UTC · Cue label color browser evidence

- Extended the real-CuStudio Chrome gate to use a 390×844 viewport, create
  nested Cues with color metadata, change a label afterward, and check the
  edited label stays visible, the blue underline marker remains, the label's
  computed text color matches ordinary table text, and the mobile page has no
  horizontal overflow. Persisted `operator_cue_color` is asserted in SQLite.
  This runs under the browser acceptance gate and uses only a Draft/temporary
  database with a simulated Cue; no runtime or hardware output is invoked.
- Updated the SC-005 matrix with the evidence and remaining keyboard/focus and
  all-palette/LTR coverage. Local normal Chrome, Chrome under Go Race,
  `tests/operator_accessibility.test.js`, syntax and diff checks passed.
- Commit `c932cdc` (`test(cue-studio): qualify cue color on mobile`). Local
  HEAD before this entry is
  `c932cdc19755d90101c1ada62eb55f43361d2a31`, six commits ahead of clean
  origin/PR #534 `569247ed8090f42a54b833d02608917b188a75ff`; worktree is clean
  before committing this entry.
- Core run `43633596721` on `569247e` remains in Go 1.26 full Race; Go 1.27,
  Companion `43633596728`, and Relay `43633596765` are green. Keep these local
  commits unpushed until that exact-head Core run completes. Then verify the
  PR/remote head, push normally and wait for CI on the new SHA. Continue the
  whole-server SC-024 review after those checks, without production activation.

## Resume update — 2026-11-14 20:18 UTC · Cue color batch pushed

- Exact-head Core `43633596721` on `569247e` completed green, including Go 1.26
  full Race and ARM64 product builds; Go 1.27 was green. Companion
  `43633596728` and Relay `43633596765` also passed.
- Rechecked clean local state, `git ls-remote`, `origin`, and OPEN/Draft PR #534
  at `569247ed8090f42a54b833d02608917b188a75ff`, then normally pushed seven
  accumulated commits. No force push or merge. Local, tracking ref, remote
  branch and PR now agree on `5fb3a6889e823aecb067b519c5a8b206f84cae21`.
- Search adds bounded server-side Project Files query and query-bound paging;
  its full project totals remain distinct from filtered match counts. HTTP,
  Store, Node and Chrome RTL/LTR tests cover search, cursor replay and revoke
  boundaries. SC-005 browser evidence now covers Cue color persistence and
  readable labels at 390×844 Arabic RTL with no horizontal overflow.
- New exact-head runs are active: Core `43636803011`, Companion `43636803043`,
  and Relay `43636803216`. They are pending at this checkpoint; do not push
  another change until all finish. Current branch/worktree are clean.
- Next: inspect all three new CI results and exact PR head. Continue the SC-024
  route-policy review after checks, keeping production ACL disabled. Pi 4
  requalification remains pending because the isolated qualification endpoint
  previously failed before shell access with `websocket: bad handshake`; do not
  retry without a changed endpoint/credential condition.

## Resume update — 2026-11-14 20:21 UTC · all Cue colors in mobile browser

- Expanded the SC-005 real-Chrome case to render and verify all four palette
  markers at 390×844 in Arabic RTL, while retaining the edited blue Cue label,
  same text color as ordinary table text and zero page overflow. The purple
  sample survives SQLite creation too. Local browser test passed normally and
  under Race; `tests/operator_accessibility.test.js`, syntax and diff checks
  also pass. Keyboard/focus, measured contrast and English LTR remain open.
- Commit `9ae3226` (`test(cue-studio): verify full cue color palette`). Local
  HEAD before this entry is
  `9ae3226bfe0c2d2ef72abeddc787d015a82cf875`, two commits ahead of clean
  origin/PR #534 `5fb3a6889e823aecb067b519c5a8b206f84cae21`; worktree was clean
  before committing this checkpoint.
- Current exact-head checks on `5fb3a68`: Core `43636803011` is in progress;
  Companion `43636803043` and Relay `43636803216` are green. Do not push until
  the Core run completes, then recheck branch and PR. SC-024 production ACL
  stays off; Pi 4 optimized-code qualification remains blocked by the
  unchanged failed isolated SSH handshake.

## Resume update — 2026-11-14 20:23 UTC · palette plus RTL/LTR acceptance

- The real-Chrome SC-005 test now reloads the same Draft in English LTR after
  its 390×844 Arabic RTL pass. It verifies all four color markers, persistent
  edited label, unchanged computed text color and no horizontal page overflow
  in both directions. Temporary Chrome session storage carries only the
  browser report across reload; no show data is persisted outside the test DB.
- Normal Chrome and Race Chrome passed locally; the accessibility Node tests,
  JavaScript syntax and diff checks pass. Updated the matrix: only actual
  keyboard/focus use and measured rendered-background contrast remain.
- Commit `54a6b48` (`test(cue-studio): verify color in both locales`). Local
  HEAD before this entry is
  `54a6b485f958e10571e209099e39fb1197c58a49`, four commits ahead of clean
  origin/PR `5fb3a6889e823aecb067b519c5a8b206f84cae21`; worktree was clean
  before this checkpoint commit.
- CI for exact head `5fb3a68`: Core `43636803011` passed Go 1.26 tests and the
  Browser Cue Delivery gate and is running Vet; Companion `43636803043` and
  Relay `43636803216` are green. Race and ARM64 Core checks are still pending.
  Keep local commits unpushed until the complete exact-head CI finishes.

## Resume update — 2026-11-14 20:40 UTC · SC-005 browser qualification and SC-024 keyboard pass

- Local HEAD is `447691b560827a50967e3d9160d2ef0bd7594fe8`, seven commits
  ahead of the remote/PR head `5fb3a6889e823aecb067b519c5a8b206f84cae21`.
  The branch is clean. PR #534 is still OPEN and Draft. Local commits are
  `a947e24`, `9ae3226`, `7e43e77`, `54a6b48`, `ee447fc`, `9626c49`, and
  `447691b`. Do not push until Core run `43636803011` on exact head `5fb3a68`
  completes; Companion `43636803043` and Camera Relay `43636803216` passed.
- SC-005 browser work now exercises actual native Tab traversal to the Cue
  Label editor field, verifies the 3px keyboard outline, and computes contrast
  from rendered colors against composited backgrounds in Arabic RTL and
  English LTR at 390×844. Measured in both locales: blue `4.75:1`, green
  `5.41:1`, amber `6.71:1`, purple `4.99:1`, Cue text `14.73:1`, focus outline
  `8.05:1`. The high-specificity theme rule preserves the outline for focused
  form controls. The matrix marks SC-005 implemented in code.
- SC-024's isolated Owner grant-editor browser test now sends an actual Chrome
  Tab from Project to Account and verifies `:focus-visible` plus its 3px
  outline at the ≤560px RTL breakpoint. It still saves only through the
  optional qualification API and verifies `PREPARED_NOT_ENFORCED`; no
  production route assembly or runtime request occurs. The matrix retains
  end-to-end keyboard grant/revoke, touch, whole-server endpoint/asynchronous
  delivery reconciliation, Companion compatibility, and installed-copy
  migration/rollback qualification as open.
- Local checks passed: Mixed Cue Chrome test normally and under Race; Project
  grant Chrome test normally and under Race; measured-contrast assertions;
  `go vet ./...` before the second test-only change and
  `go vet ./internal/httpapi` after it; `tests/operator_accessibility.test.js`
  (2/2); JavaScript syntax and `git diff --check`.
- Core `43636803011` on `5fb3a68` continues in full Go 1.26 Race after
  Go 1.26 tests, Browser Cue Delivery, and Vet passed; Go 1.27 passed. It had
  run about 20 minutes at this update; ARM64 product builds are still pending.
  Recheck the job rather than starting another remote workflow or pushing.
- Pi 4 optimized-code requalification remains unavailable: the isolated
  qualification SSH endpoint previously failed before shell access with
  `websocket: bad handshake`. Mac metrics are not a substitute and the prior
  Pi run predates linked-graph optimization. No physical device, production
  service/data/Vault, production ACL, or firmware was touched.
- First next action: wait for Core `43636803011`; if it succeeds, recheck
  `git ls-remote`, OPEN/Draft PR #534 and the exact head, then push these seven
  commits normally and follow the new exact-head checks. Continue the
  remaining SC-024 route-to-policy/asynchronous review and independent
  software-only work while Project ACL stays off.

## Resume update — 2026-11-14 20:45 UTC · keyboard test boundary correction

- Current local HEAD before this note is Handoff commit
  `ac0bdd4cd3a299e120e53e25172fd19d123a9f66`, eight commits ahead of remote
  and OPEN/Draft PR #534 at `5fb3a6889e823aecb067b519c5a8b206f84cae21`;
  the worktree is clean. Code/test HEAD `447691b` remains seven commits ahead.
- The passing Project Access Chrome regression proves native Tab from Project
  to Account and a visible focus outline. An exploratory attempt to activate
  the Load/Save buttons with CDP Enter/Space did not trigger their click
  handlers and was reverted; it does not change the accepted code. Keep
  end-to-end keyboard operation and touch in the open SC-024 list.
- Exact-head Core `43636803011` remains in Go 1.26 Race after 26 minutes;
  Go 1.26 regular tests, Browser Cue Delivery and Vet passed, as did Go 1.27.
  Companion `43636803043` and Camera Relay `43636803216` are green. Do not
  push while Core is active.
- First next action remains waiting for that run. On success, recheck remote
  and Draft PR head, then normally push every commit in the local ahead range
  to the existing branch; follow the resulting CI. SC-024 production ACL,
  production services/data/Vault, real stage devices and firmware remain
  untouched. Pi ARM64 performance requalification remains pending the
  unchanged isolated SSH handshake failure.

## Resume update — 2026-11-14 20:53 UTC · SC-024 touch simulation

- Local code/test HEAD is `32a3be9aa61658f02335c4368b211fc1906c9e53`; the
  remote branch and OPEN/Draft PR #534 remain at
  `5fb3a6889e823aecb067b519c5a8b206f84cae21`. Ten local commits currently
  precede the remote before this Handoff entry; worktree is clean. The ahead
  commits are `a947e24`, `9ae3226`, `7e43e77`, `54a6b48`, `ee447fc`,
  `9626c49`, `447691b`, `ac0bdd4`, `d9f42f9`, and `32a3be9`.
- SC-024 Owner editor Chrome acceptance now also delivers a CDP touch gesture
  at the visible Security navigation control, verifies `pointerType ===
  "touch"`, and waits for the grant editor to open at 390×844 RTL. The test
  remains on temporary SQLite and the optional `PREPARED_NOT_ENFORCED` API;
  it asserts no publish, runtime or safety call. Local normal and Race runs
  passed. This is browser touch emulation, not physical Android qualification.
- The earlier native Tab pass remains; end-to-end keyboard selection/save and
  revoke interactions remain open. A trial Enter/Space activation did not
  trigger the button handlers and was reverted. The matrix records keyboard
  completion and real Android touch acceptance as still open.
- Focused validation after the touch update: Owner grant Chrome test normal
  and Race; `go vet ./internal/httpapi`; `tests/operator_accessibility.test.js`
  (2/2); JS syntax and diff checks passed. SC-005 contrast and Mixed Cue
  browser normal/Race results are in the preceding entry.
- Exact-head Core `43636803011` on `5fb3a68` is still in full Go 1.26 Race
  after about 32 minutes. Go 1.26 regular tests, Browser Cue Delivery and Vet,
  Go 1.27, Companion `43636803043`, and Relay `43636803216` are green. Do not
  interrupt it with a push.
- Resume by watching that Core run. After completion, recheck the remote and
  PR head, then push the complete local ahead range normally to the same PR
  and follow its exact-head CI. Keep production ACL off. Pi 4 performance
  requalification and isolated installed-copy rollback remain open; no real
  stage device, production data/service/Vault or firmware was touched.

## Resume update — 2026-11-14 20:53 UTC · exact-head CI passed

- Exact-head Core run `43636803011` on remote `5fb3a688` completed success at
  20:52 UTC. Go 1.26 tests, Browser Cue Delivery, Vet, full Race, and Linux
  ARM64 CGo-free builds all passed. Go 1.27 passed in the same run.
  Companion `43636803043` passed its macOS replacement/media and Swift
  CompanionCore jobs; Camera Relay `43636803216` passed isolated tests and
  ARM64. `gh pr checks 534` reports all checks green.
- Before this CI result entry, local HEAD was
  `7c59bceb8d19dbeac053b893de30155b8409f6c6`, eleven commits ahead of clean
  `origin/integration/stagecore-rc` / open Draft PR #534 at
  `5fb3a6889e823aecb067b519c5a8b206f84cae21`. Worktree is clean. All local
  commits are part of this authorized campaign and are ready for an ordinary
  fast-forward push to the existing branch.
- Rechecked PR #534 is still OPEN/Draft and its head still equals the remote
  branch. No force push, merge, or production action. After pushing, wait for
  the new exact-head CI before another upload; meanwhile continue the open
  SC-024 route/async review and other software-only gaps.

## Resume update — 2026-11-14 21:03 UTC · Linux cue-marker contrast correction

- Local and remote branch/OPEN Draft PR #534 were at
  `9db95df9b82ffbd6ac23949d8545db0b3d09aad4`; the worktree held two preserved
  local edits before this entry. The remote Core/Companion/Relay workflows for
  `9db95df` had all completed before this follow-on began, so no CI was
  canceled by a push.
- Core run `43639869579` completed failure only in Linux Browser Cue Delivery:
  the cross-platform browser measured blue 3.19:1, green 2.95:1, amber 2.38:1,
  and purple 3.47:1 against the Cue surface, below the test's 3:1 non-text
  threshold for green/amber. Its Go 1.26 regular tests and Go 1.27 passed;
  Vet/Race/ARM64 were skipped after the browser failure. Companion
  `43639869574` and Camera Relay `43639869578` passed. This was a real rendered
  contrast issue, despite the prior Mac browser palette measurement passing.
- `internal/operatorweb/static/cue-studio.css` now uses lighter marker colors.
  Chrome computed/composited measurements for the nested Mixed Cue browser
  scenario are identical in RTL and LTR: blue 8.95:1, green 10.65:1, amber
  12.76:1, purple 9.57:1; text 14.73:1 and visible focus 8.05:1. The test
  verifies the nested chain and six ordered actions using simulation/temp
  SQLite; no real device output is used.
- Passed locally after this fix: focused `TestBrowserCreatesNestedMixedCueChainInDraft`
  in normal mode and `-race`; `go vet ./internal/httpapi`;
  `node --test tests/operator_accessibility.test.js` (2/2); browser fixture
  JavaScript syntax check; and `git diff --check`. Exact focused Chrome normal
  and Race tests both report the ratios above. The same code-level and
  qualification-matrix edits are still uncommitted at this checkpoint.
- Current related-component matrix records remote heads/CI for linked PRs and
  refreshes PR #534's failed exact-head run plus the local contrast correction.
  Before commit/push, recheck `git status`, `git diff`, remote SHA and PR #534;
  commit this bounded CSS/matrix/Handoff batch and push normally to the same
  branch. Then wait for exact-head Core CI before another push.
- Continue the Mixed/Linked Cue performance qualification only with isolated
  simulation. Pi 4 ARM64 requalification remains pending because the
  previously used isolated SSH endpoint failed before shell access with
  `websocket: bad handshake`; Mac figures do not substitute. Keep Project ACL
  disabled in production. Do not touch production services/data/Vault, send
  physical-stage commands, flash hardware, force-push, or merge.

## Resume update — 2026-11-14 21:19 UTC · dual-theme Cue contrast and SC-024 keyboard controls

- Remote and OPEN/Draft PR #534 were at `7df82f198bdf1d3832539155211060bfd836c51f`
  on `integration/stagecore-rc`. Core `43640865645`, Companion
  `43640865619`, and Camera Relay `43640865680` completed before the next
  change; Companion/Relay passed, Core failed only Browser Cue Delivery. No
  active CI was interrupted.
- Core's Linux browser measured the previous light marker colors against the
  system-light background at blue 1.695:1, green 1.498:1, amber 1.251:1 and
  purple 1.810:1. Go 1.26 regular tests and Go 1.27 passed; Core Vet/Race/ARM64
  steps were skipped after the browser failure. The prior attempt over-lightened
  the palette because local Chrome defaulted to dark mode; it did not fix Linux.
- Cue markers now use balanced colors (`#3374b5`, `#2f855a`, `#a66a00`,
  `#805ad5`). `TestBrowserCreatesNestedMixedCueChainInDraft` now explicitly
  qualifies dark RTL and light LTR, rather than depending on each host's system
  theme. Local Chrome reports dark RTL blue/green/amber/purple 3.24/3.57/3.62/
  3.58:1 and light LTR 4.68/4.46/4.41/4.84:1, all above 3:1. Focused normal
  and Race tests pass; nested Mixed/Linked actions still use temporary SQLite
  and simulation only.
- SC-024 Owner grant browser qualification now sends actual CDP Tab and Enter
  events to focus and activate Load/Save. The Enter helper supplies carriage-
  return text so Chromium performs native button activation. After the
  asynchronous load, the UI restores focus to the enabled Access-level select.
  The test passes normal and Race; the fixture still sets select values in JS,
  so native keyboard selection of options and browser revoke remain open. The
  optional API remains `PREPARED_NOT_ENFORCED`; no publish, GO, STOP or
  Emergency Blackout request is made.
- Focused Mixed Cue browser normal/Race and SC-024 browser normal/Race passed;
  `go vet ./internal/httpapi`, Node syntax checks, `tests/operator_accessibility.test.js`
  (2/2), and the prior/current `git diff --check` passed. Matrix updates record
  the exact CI failure and local evidence. Re-run `git diff --check` after the
  latest documentation edits, commit the balanced palette/theme test and
  keyboard-control batch, then push normally to the same branch and wait for
  exact-head Core CI before any other push.
- Pi 4 qualification remains pending. Read-only config inspection confirms
  `stagecore-qualification` still resolves to the unchanged `ai.ali96adil.com:22`
  endpoint after the previous `websocket: bad handshake`; it was not retried.
  No Mac measurement substitutes for the missing ARM64 Pi run. Production
  services/data/Vault and production ACL stay untouched; no real device,
  firmware, force push, or merge was used.

## Resume update — 2026-11-14 21:25 UTC · SC-024 prepared grant/revoke browser path

- Remote `integration/stagecore-rc` and OPEN/Draft PR #534 are at
  `385be99ea8e1dea66fdb86a5d62d00c52e5b0eb7`. Exact-head Core run
  `43642337411` is still in progress; Companion `43642337424` and Camera Relay
  `43642337445` completed successfully. Do not push while Core is active.
- The local SC-024 browser test now uses a temporary SQLite Project to save
  `OPERATE`, verify its ProjectView/RehearsalControl/GO/STOP/Blackout grants,
  then save `NONE` and confirm `access.Check` denies those five permissions.
  Load/Save use actual CDP Tab/Enter input; the browser form restores focus to
  Access level after loading and to Save after the async write. The test confirms
  no runtime, safety, or publish request. Project/account/level values remain
  fixture-selected in JavaScript, so native keyboard choice of select options
  is not yet qualified.
- Focused `TestBrowserOwnerProjectAccessEditorStoresPreparedLevel` passes
  normally and under `-race`; `go vet ./internal/httpapi`, JS syntax checks,
  and `tests/operator_accessibility.test.js` (2/2) pass. Matrix and handoff
  edits describe this bounded follow-on; it remains local while the exact-head
  CI run completes.
- Next: wait for Core `43642337411`, verify the remote and PR heads, run
  `git diff --check`, commit the tested SC-024 follow-on, and push normally
  once that current CI is complete. Then follow exact-head Core CI. Keep
  production ACL disabled; Pi 4 runtime qualification still requires the
  previously unavailable isolated environment. Do not touch production
  services/data/Vault, real devices, or firmware.

## Resume update — 2026-11-14 21:31 UTC · ACL mock compatibility and revocation

- Local and remote branch HEAD are both
  `385be99ea8e1dea66fdb86a5d62d00c52e5b0eb7`; Draft PR #534 remains OPEN at
  that SHA. Core run `43642337411` is complete, so no CI is being canceled.
  Go 1.26 failed before Test at `Check Operator JS syntax`: the ACL Node mock
  raised `ReferenceError: document is not defined` from asynchronous activity
  after a test ended. Go 1.27 Test/Vet and Go 1.26 Vet passed; Core browser,
  Race and ARM64 steps were skipped. Companion `43642337424` and Camera Relay
  `43642337445` passed.
- The focus logic now uses the actual control's optional `ownerDocument`, so
  browser focus restoration works without a global DOM in the Node mock.
  Locally: `node --test tests/security_project_access.test.js` passes 8/8;
  `TestBrowserOwnerProjectAccessEditorStoresPreparedLevel` passes normal and
  `-race`; `go vet ./internal/httpapi`, JS syntax checks and `git diff --check`
  pass. The browser flow stores `OPERATE`, checks the required grants, revokes
  to `NONE`, and verifies permission denial in temporary SQLite. It sends no
  publish, runtime, STOP, or Blackout request. Form focus is returned after
  asynchronous Load/Save; native keyboard selection of a select option is an
  identified remaining UI qualification gap.
- The related-component matrix now records the exact failing SHA/run and local
  correction. Mixed Cue dual-theme browser normal/Race evidence remains in the
  prior entry; it was not repeated. No performance benchmark was repeated.
- Next: commit this tested local browser/focus/docs batch, recheck that Core
  `43642337411` remains complete and remote/PR still point at `385be99`, then
  push normally to the existing PR and follow every check on the new exact
  head before another push. Pi 4 ARM64 Engine qualification remains pending
  because the isolated `stagecore-qualification` SSH route previously failed
  before reaching the device. Production ACL remains off; production services,
  data, Vault, real devices and firmware remain untouched.

## Resume update — 2026-11-14 21:43 UTC · Exact-head CI and performance evidence reconciliation

- The tested SC-024 focus/mock/revoke batch was committed as
  `055e980ac668b0771e3ae799cfce6dbe78898a79` and pushed normally to
  `integration/stagecore-rc`; local, remote and OPEN/Draft PR #534 match.
  Previous Core run `43642337411` was complete before this push.
- Exact-head Core run `43643450987`: Go 1.27 passed. Go 1.26 JS syntax,
  Firmware Foundation contract, full Test, Browser Cue Delivery and Vet passed;
  Race and Linux ARM64 CGo-free product builds were still running at this
  checkpoint. Companion `43643451012` and Camera Relay `43643451028` passed.
  Do not push another head until run `43643450987` completes.
- Local verification for `055e980`: `node --test
  tests/security_project_access.test.js` 8/8; focused Chrome SC-024 test passes
  normally and under `-race`, including OPERATE grant/readback and NONE
  revocation/denial; `go vet ./internal/httpapi`, JS syntax and diff checks
  pass. Project, account and preset values are fixture-selected; native
  keyboard selection of a select option remains unqualified. A local attempt
  to test ArrowUp/ArrowDown using CDP delivered keydown events to the focused
  select but left its value unchanged, so the experiment was fully reverted.
  Do not describe this attempt as passing browser qualification.
- Performance report review found the requested isolated Pi 4 ARM64 simulator
  measurements already recorded from source `a90403b`: Mixed Cues and linked
  chains across the requested scales, scheduled starts, bursts, sustained
  100-Cue/200-Action Rehearsal and STOP/Blackout load. They include P50/P95/P99,
  jitter, schedule skew, GC, allocations and test-process CPU/RSS. A direct
  comparison shows Cue Engine, Runtime Control, simulator, domain and publish
  execution source unchanged from `a90403b` through `055e980`; do not repeat
  those measurements just to refresh the SHA. The old Pi ACL benchmark and
  `CheckRuntimeIntent` measurement also remain applicable because their
  benchmark and checked runtime authorization code are unchanged. The Pi
  report still cannot claim formal acceptance: no end-to-end Cue timing SLO was
  specified, CPU/memory controllers were not isolated, and 32-delayed-Cue
  STOP/Blackout tails reached about one second. The `stagecore-qualification`
  read-only SSH route failed only after those runs and remains unavailable for
  a future rerun after relevant source changes. The performance report and
  related-component matrix updates are local and await a clean, completed Core
  run before push.
- Follow-on M3 CPU-profile diagnosis exercised 100 simulated STOP and 100
  Emergency Blackout operations, each against 32 delayed Cue runs. The
  instrumented run reported STOP P50/P95/P99 `11.71/13.35/13.95 ms` and
  Blackout `11.16/12.71/13.15 ms`, with P95 jitter `1.648/1.557 ms` and
  `201,577/207,292 B` allocated per operation. Do not use these instrumented
  latencies as an unprofiled baseline. The process-wide profile (including
  repeated fixture migrations) sampled 95% in `syscall.rawsyscalln`; SQLite
  VDBE execution was 83.14% cumulative and `database/sql` ExecContext 72.13%.
  Because setup is included and persistence ordering is safety/audit relevant,
  this profile does not isolate STOP's cancellative operation cost or justify
  batching/durability changes. No optimization was made.
- Linked PR heads/checks reverified without modifying other repositories:
  TabletPlayer #36 `91613da` PASS; ESP32-DMX #39 `3ad7749` all checks PASS;
  ESP32 Camera #20 `455b066` PASS; StageLaser #56 `b14a578` PASS and #57
  `c4ff10b` PASS. All remain open; #36/#39/#20/#56 are drafts, #57 is not.
  No linked checkout was present in this workspace.
- Safety boundary remains unchanged: no production service/data/Vault access,
  no real device commands or firmware, production ACL disabled, no force push
  or merge. Next: wait for all exact-head Core jobs, record their final status,
  run `git diff --check` on the pending docs, and then push the small doc
  reconciliation batch normally to the same PR.

## Resume update — 2026-11-14 22:00 UTC · Exact-head checks complete

- Core run `43643450987` completed successfully at code head
  `055e980ac668b0771e3ae799cfce6dbe78898a79`: Go 1.26/1.27 tests and Vet,
  Operator JS syntax, Firmware Foundation contract, Browser Cue Delivery,
  Race and Linux ARM64 CGo-free product builds all passed. Go 1.26 Race took
  about 23 minutes. Companion Core `43643451012` and Camera Relay
  `43643451028` also passed. No run was canceled.
- `055e980` is pushed. Local docs-only commits `850fdf9` and this checkpoint
  refresh are ahead of remote; verify remote and PR #534, then push them
  normally together. The SC-024 focus/mock/revoke behavior remains covered by
  local normal and Race Chrome tests and by Browser Cue Delivery on `055e980`.
- The performance report now reconciles earlier Pi evidence with current code:
  the requested simulated Pi 4 ARM64 Mixed/linked, scheduling, burst, sustained,
  STOP and Blackout cases already ran from `a90403b`; Cue Engine/Runtime
  Control/simulator/domain/publish execution source has no intervening diff.
  We deliberately did not repeat the Pi run. No full-Cue SLO exists, the Pi
  host controllers were not isolated, and delayed-32 safety tails remain about
  one second. A later isolated SSH attempt failed before host identity and
  remains a rerun constraint if relevant performance code changes.
- The 32-delayed-Cue M3 diagnostic profile is exploratory only: benchmark
  latencies under profiling are not baseline values, its profile includes
  repeated migrations, and it does not justify altering durable event order
  or grouping writes. No performance optimization was made.
- A keyboard Arrow-key selection attempt was reverted after CDP events reached
  the focused native select but did not change its selected value. Native
  keyboard option selection remains an open test gap; the saved code and
  successful browser test use actual Tab/Enter for Load/Save, then fixture-set
  access levels. No publish, GO, STOP, or Blackout calls were sent.
- Safety: production ACL stays disabled; no production services/data/Vault,
  real devices, or firmware were touched. No force push or merge. After the
  docs push, follow the exact-head Core/Companion/Relay CI and wait for Core
  Race/ARM64 before another push.

## Resume update — 2026-11-14 22:09 UTC · 32-Cue STOP regression

- Remote and PR #534 are at `b3a4be153f4c6ee909f29196c14480cbfe26bfa3`,
  OPEN/Draft. Core run `43645947408` is exact-head: Go 1.27 passed and Go 1.26
  Race is in progress after its full Test step passed. Companion
  `43645947414` and Camera Relay `43645947412` passed on this head. Do not push
  until Core Race and ARM64 finish.
- Added `TestStopCancels32OverlappingDelayedCues` locally to match the highest
  measured STOP concurrency. It launches 32 Rehearsal GO requests against
  60-second simulator delays, invokes STOP, and asserts 32 cancelled command
  results plus 32 persisted terminal Cue executions. Focused normal and Race
  tests both pass; `go vet ./internal/runtimecontrol` and `git diff --check`
  pass. This is a regression test only; no runtime implementation changed.
- Added the test evidence to the performance report. The test uses the existing
  simulator-only registry and no real outputs. Production ACL stays disabled;
  production services/data/Vault, devices, and firmware remain untouched.
- Next: let run `43645947408` complete, recheck branch/PR/CI exact heads, commit
  the STOP regression and report/Handoff update, then push normally to #534.
  Follow all checks at the new SHA before starting another push.

## Resume update — 2026-11-14 22:16 UTC · 32-Cue STOP and Blackout regressions

- Remote and PR #534 remain at `b3a4be153f4c6ee909f29196c14480cbfe26bfa3`.
  Core run `43645947408` is still in progress: Go 1.27 and the Go 1.26 Test,
  Browser Cue Delivery, and Vet steps passed; Go 1.26 Race is still running,
  with ARM64 pending. Companion `43645947414` and Camera Relay `43645947412`
  passed on this exact head. No push has been made while Core CI is active.
- Added `TestEmergencyBlackoutCancels32OverlappingDelayedCues` beside the
  32-Cue STOP regression. The test uses delayed simulator-only Rehearsal GO
  calls, checks the durable Blackout latch is already set before the
  simulated safety callback, requires exactly one callback, and verifies all
  32 command results and persisted executions terminate as `CANCELLED`.
- Both 32-Cue regressions passed focused normal and Race runs;
  `go vet ./internal/runtimecontrol` and `git diff --check` passed. Updated
  `docs/architecture/cue-execution-performance-qualification.md` with the
  regression evidence. No runtime implementation changed and no real output
  was registered.
- Current local HEAD is `9bc5988f092f573aeb83b451971572b34b521d5e` with the
  STOP test committed; the Blackout test and its report/Handoff updates are
  still local and uncommitted at this point. The next action is inspect the
  complete diff, commit this Blackout test batch, then wait for exact-head
  Core Race/ARM64 completion before any normal push to #534.
- Safety limits remain: no production service/data/Vault changes, no real
  device commands or firmware, no production ACL activation, no force push,
  and no PR merge.

## Resume update — 2026-11-14 22:46 UTC · SC-015 transport event timeline

- Remote and OPEN/Draft PR #534 are still at
  `b3a4be153f4c6ee909f29196c14480cbfe26bfa3`. Core run `43645947408` has now
  completed successfully on that exact head; Companion `43645947414` and
  Camera Relay `43645947412` also passed on it. The CI wait gate is clear.
- Local code HEAD is `669e6cc22b650d9c2663db1f58e89ea6982e5360`, three commits
  ahead of remote: `9bc5988` (32-Cue STOP), `f6e1409` (32-Cue Emergency
  Blackout), and `669e6cc` (SC-015 explicit transport evidence). The software
  completion matrix and this Handoff update are the only remaining local
  documentation edits for this batch.
- Added Hub-originated `stage_device.transport.connected` and
  `stage_device.transport.disconnected` events. V1 authenticated sockets emit
  transitions; V2 emits only after Project-scoped runtime readiness; socket
  replacement, socket close, Project-scope change and startup offline reset
  are explicit disconnect reasons. Startup telemetry is best-effort and
  cannot prevent the authoritative offline reset. These events carry only
  device ID, transport state, Hub connection generation and a fixed reason;
  they never claim command ACK or physical output. The timeline reads only
  Hub-source events for the same Project and Session time window, caps
  transport history at 500 with an incomplete-history flag, and uses Migration
  63's partial index. UI, comparison metrics and export use allowlisted fields.
- Verification: full normal tests passed for Device Channel, Device
  Experience, Session Memory, Store and DB; focused HTTP Session Memory API
  passed; `go vet` passed across those packages and HTTP API; timeline Node
  tests passed 6/6; syntax checks and `git diff --check` passed. Full Race for
  the affected Go packages passed before the final reconnect case; the final
  socket replacement and disconnect tests also passed focused Race. The new
  Chrome gate was added, but no local Chrome executable or
  `STAGECORE_TEST_CHROME` was available, so that visual browser check remains
  unrun.
- SC-015 remains Partial for Companion and other adapter endpoint health and
  all physical output confirmation; command attempts still do not imply
  connectivity, ACK or execution. Pi performance results remain the earlier
  source-equivalent qualification; no performance-sensitive code changed.
- The final full focused tests, vet, Node checks, and diff checks passed for
  this code; the latest reconnect and Hub-restart regressions also passed
  focused Race. Next: commit the matrix/Handoff update, recheck remote/PR and
  all workflow heads, push the four local commits normally to this same branch
  if no conflicting Core run is active, then follow Core/Companion/Camera Relay
  CI at the new exact head.
- Safety limits remain unchanged: no production services/data/Vault, real
  device commands, firmware, production ACL activation, force push or merge.

## Resume update — 2026-11-14 22:49 UTC · pushed transport and safety regressions

- Local, `origin/integration/stagecore-rc`, and OPEN/Draft PR #534 are at
  `774414d6a8c2ab6c4c597f886eb0dd7e5bfea9bc`. Four commits are now pushed:
  `9bc5988` (32-Cue STOP), `f6e1409` (32-Cue Emergency Blackout),
  `669e6cc` (SC-015 Hub transport timeline), and `774414d` (matrix/Handoff).
- Exact-head CI: Core run `43649877977` is still in progress; Companion Core
  `43649877980` and Camera Relay `43649877969` passed. Do not push another
  commit until Core `43649877977` completes and the exact PR/branch heads and
  all relevant workflows are checked again.
- Verification on the pushed code: affected Go package tests, focused HTTP
  API tests, vet, six Node timeline tests, JavaScript syntax checks, and
  `git diff --check` passed. Full Race passed before the final transport
  reconnect case; focused Race passed for reconnect, Hub restart, the 32-cue
  STOP/Blackout regressions, and their related safety paths. The optional
  Chrome browser test could not run because Chrome and
  `STAGECORE_TEST_CHROME` are unavailable locally.
- The two 32-cue regressions use only simulator delays and confirm STOP and
  Emergency Blackout cancel all 32 overlapping delayed executions. SC-015
  records authenticated Hub transport transitions, scoped by Project and
  Session window, with generation/reason evidence. It deliberately does not
  claim Companion/adapter endpoint health or physical output confirmation.
- Next software gap to inspect while CI runs: Companion runtime socket
  lifecycle and project-scoped transport evidence. First map existing
  Companion health/events and session identity, then add only a regression
  or implementation if the code proves a gap. Preserve replacement fencing,
  authorization boundaries, and the distinction between Hub transport,
  Companion reachability, adapter health, and physical output.
- Pi ARM64 qualification, broader SC-001–SC-025 evidence, linked repository
  PR/CI review, production release approval, and hardware acceptance remain
  open. Earlier Pi measurements are not being repeated absent a change that
  justifies it; isolated Pi access and formal CPU/memory isolation still need
  confirmation.
- Safety limits remain unchanged: no production service/data/Vault, real
  device commands, firmware, production ACL activation, force push or merge.

## Resume update — 2026-11-15 · SC-002 expired-session rollback regression

- Branch is `integration/stagecore-rc`; OPEN/Draft PR #534 and remote are at
  `460db92eb9a8457b840aa0ddd7aef1b5ebb91c43`. Local HEAD before this batch is
  `5aa718381f60d020c99cce2145daf9704fa99bda`, with its handoff checkpoint
  already committed locally. SC-002 source/test/matrix commit
  `fcb5fbff4927e8f7b6733cc62ac48765348de17b` and the handoff checkpoint are
  local. Local HEAD is three commits ahead of remote; no push has been made.
- Core run `43654085363` is still in progress on the exact remote head
  `460db92`; Go 1.27 Test and Go 1.26 Test/Browser/Vet have passed, while Go
  1.26 Race and ARM64 were still pending at last check. Companion Core
  `43654085383` and Camera Relay `43654085434` passed. Do not push until the
  full Core run is complete and the branch, PR, and exact workflow heads are
  rechecked.
- Static review of the existing #504/#505 clock dashboard and correction
  preview found a session-safety gap: observing a browser session expired
  after a forward wall-clock correction did not persist its retirement, so a
  later clock rollback could make the original cookie valid again. `Validate`
  and `ValidatePassive` now retire an expired session once in SQLite and fail
  closed if that write fails. Renewal also checks that the validated
  credential is still unexpired after generating its replacement material.
  The red-first fake-clock regression reproduced the resurrection before the
  fix, and now proves the old cookie stays invalid after rollback while the
  Owner can sign in locally and receive a fresh valid session. No host clock
  command or production session was touched.
- Focused regression passed under normal and Race tests; full
  `internal/userauth` normal and Race tests passed; HTTP `BrowserAuthAuditAndRenewal`
  passed; `go vet ./internal/userauth ./internal/httpapi`, gofmt verification,
  and `git diff --check` passed after the final code edit. Full HTTP API tests
  with local Chrome had passed earlier in the same feature batch before the
  final relocation of the validation timestamp; re-run if a later change
  touches this path. Matrix SC-002 wording now records only the tested
  software behavior and leaves RTC, actual clock correction, WAN-off Pi cold
  boot, and tablet reauthentication as outstanding qualification.
- Next: commit this handoff checkpoint. Keep all commits local while Core
  `43654085363` is active; then inspect the completed
  run and push a normal fast-forward to the same PR if it passes. Continue
  with the next evidenced software gap while Pi hardware access remains
  unavailable. Production Project ACL remains disabled.
- Safety remains unchanged: no production service/data/Vault, original show
  data, real device commands, firmware, production ACL activation, force push,
  merge, paid action, or backup deletion.

## Resume update — 2026-11-15 · passive expiry and renewal boundary coverage

- This checkpoint builds on local code/test HEAD
  `55f339dc54a849d164e6f2febac995445ffcf2ea`; all changes remain local on
  `integration/stagecore-rc`. Since remote `460db92`, the local commits include
  checkpoint `5aa7183`, SC-002 fix `fcb5fbf`, handoff commits `0f9aac9` and
  `3a43c9d`, and test expansion `55f339d`. There has been no push. The
  checkpoint commit itself follows the recorded code/test HEAD.
- Core run `43654085363` still targets exact remote/PR head
  `460db92eb9a8457b840aa0ddd7aef1b5ebb91c43`. Go 1.27 passed; Go 1.26 Test,
  Browser Cue Delivery and Vet passed; Go 1.26 Race remains in progress and
  ARM64 remains pending. Companion Core `43654085383` and Camera Relay
  `43654085434` passed. Do not push until the complete Core run finishes and
  the exact branch, PR, and workflow heads are verified again.
- Expanded the regression to cover both `Validate` and `ValidatePassive`
  (used for passive streams) when expiry is first observed after a forward
  fake-clock jump, followed by rollback and local Owner reauthentication. A
  second deterministic reader advances the fake clock during renewal token
  generation; renewal now has a regression proving it retires the credential
  and rejects the rotation if the session expires in that gap. The old cookie
  also remains invalid after the simulated rollback. This is isolated SQLite
  and fake-clock coverage only.
- After the expansion, `go test ./internal/userauth -count=1`,
  `go test -race ./internal/userauth -count=1`, `go vet ./internal/userauth`,
  gofmt verification and `git diff --check` passed. The previously recorded
  HTTP auth renewal regression and full local Chrome HTTP API suite remain as
  documented above; no HTTP production code changed in this test expansion.
  The SC-002 matrix now names the exact request, passive stream and renewal
  boundary evidence.
- Next: wait for Core `43654085363` to finish, diagnose any failure against
  exact-head logs, then push the complete tested local batch normally to PR
  #534 if it passes. Continue with the next software-only issue; Pi clock and
  cold-boot qualification and all production ACL activation remain out of
  scope.

## Resume update — 2026-11-15 · verified batch ready for normal push

- Core CI `43654085363` completed successfully on exact remote/PR HEAD
  `460db92eb9a8457b840aa0ddd7aef1b5ebb91c43`: Go 1.26 Test, Browser Cue
  Delivery, Vet, Race, and Linux ARM64 product builds passed; Go 1.27 Test
  and Vet passed. Companion Core `43654085383` and Camera Relay
  `43654085434` passed on the same SHA. PR #534 remains OPEN/Draft.
- The reviewed branch/PR HEAD remains `460db92`; local code/test HEAD is
  `e71cd16abf82c8031e55ac873ad0468690daa198`, seven commits ahead before this
  checkpoint commit. The local batch contains the pushed-batch checkpoint
  `5aa7183`, expired-session retirement `fcb5fbf`, checkpoints `0f9aac9` and
  `3a43c9d`, passive/renewal boundary regressions `55f339d`, evidence update
  `e66308f`, and Chrome Cue history qualification `e71cd16`. No push has yet
  been made from this batch.
- SC-002 fake-clock coverage now includes initial expiry observation through
  request and passive-stream validation, renewal expiring during token
  generation, rollback rejection of the old cookie, and local Owner sign-in
  with a fresh session. Full `internal/userauth` normal and Race suites,
  `go vet ./internal/userauth`, and the focused HTTP renewal regression passed.
- SC-009 now has real Chrome coverage at 390×844 RTL. The test uses native
  DevTools Enter to Undo and emulated touch to Redo, verifies the original
  Cue/Action identity returns with no horizontal overflow, and checks that no
  runtime Session or Published Snapshot is created. The focused Chrome test
  passed normally and under Race; Cue history Node tests, `go vet
  ./internal/httpapi`, gofmt, JavaScript syntax, and `git diff --check` passed.
  Core Browser Cue Delivery now includes this new test for the next run.
- Linked PR review remains unchanged and all are OPEN with passing exact-head
  checks: TabletPlayer #36 `91613da`; ESP32 DMX #39 `3ad7749`; ESP32 Camera
  #20 `455b066`; StageLaser #56 `b14a578` and #57 `c4ff10b`. The first four
  are Draft; #57 is open and not Draft. No linked repository was modified.
- No active workflow remains on remote `460db92`. Next action: recheck remote
  and PR heads, push this completed batch as a normal fast-forward to the same
  branch, then wait for all new exact-head Core/Companion/Camera workflows
  before the next push. Do not merge or activate production Project ACL.
- Safety remains unchanged: production services/data/Vault, original show
  data, real device commands, firmware, production ACL, force push, merge,
  paid actions and backup deletion remain untouched.

## Resume update — 2026-11-15 · batch pushed; exact-head CI running

- Normal fast-forward push completed from `460db92` to
  `6529a8bbca33e9bdc974f517ce8cdbb81eed75b5` on the existing
  `integration/stagecore-rc` branch and Draft PR #534. Remote and PR both
  report exactly `6529a8b`; the worktree was clean immediately after push.
- Exact-head workflows are now running: Core `43656497568`, Companion Core
  `43656497653`, and Camera Relay `43656497519`, all on `6529a8b`. Do not push
  another batch until Core has completed; wait for all three relevant checks
  and verify their exact HEADs before relying on the results.
- The pushed batch contains SC-002 session retirement and request/passive/
  renewal fake-clock regressions plus SC-009 real-Chrome keyboard Undo/touch
  Redo acceptance. Locally passed: full userauth normal/Race, HTTP renewal
  regression, the Chrome history test normal/Race, Cue history Node tests,
  Go vet for affected packages, JavaScript syntax and diff checks. The next
  Core Browser Cue Delivery gate now includes the new Chrome test.
- Linked component checks were freshly re-read before push and remain on
  existing heads: TabletPlayer #36 `91613da` (pass), ESP32 DMX #39 `3ad7749`
  (all build/host-contract checks pass), ESP32 Camera #20 `455b066` (pass),
  StageLaser #56 `b14a578` (firmware build/host contract pass), and #57
  `c4ff10b` (firmware build/host contract pass). No linked repo was modified.
- Next highest independent browser acceptance gap in the matrix is SC-007:
  validate the actual lighting preview-to-Draft flow and actionable error
  recovery in a local Chrome/SQLite fixture, without dispatching DMX. Do not
  change Project ACL production wiring or any real device path.
- Safety remains unchanged: production services/data/Vault, original show
  data, real device commands, firmware, production ACL, force push, merge,
  paid actions and backup deletion remain untouched.

## Resume update — 2026-11-14 23:35 UTC · Race failure and SC-024 keyboard gate

- Branch: `integration/stagecore-rc`; PR #534 remains OPEN/Draft. Before this
  checkpoint edit, local HEAD was
  `2e7997e5c03ddeda933fc9b34ee545151dbb6a46`; remote and PR head remain
  `c52a453c2d838935e1f7601fa748cec2e43ec725`. Seven commits are ahead of
  remote: `0b57512`, `dca2ef6`, `6e170f5`, `7d1f03e`, `398f804`, `85fc149`,
  and `2e7997e`. This checkpoint-only commit will be the next local commit.
- Exact remote Core run `43650575318` completed with Go 1.26 Race failure;
  Go 1.26 Test/Browser/Vet, Go 1.27 Test/Vet, Companion Core and Camera Relay
  checks passed. ARM64 was skipped because Race failed. The race log reports
  `TestStopCancels32OverlappingDelayedCues` returning `STOP_UNCONFIRMED` and
  `TestEmergencyBlackoutCancels32OverlappingDelayedCues` failing during the
  same cancellation fan-out. No race-detector report appeared. Both operations
  shared a two-second terminal wait while 32 simulated delayed Cues persisted
  cancellation.
- `internal/runtimecontrol/service.go` now allows five seconds for the bounded
  cancellation/persistence fan-out. Cancellation is still issued immediately;
  Emergency Blackout still latches and dispatches its simulated safety callback
  before waiting for Cue termination. Full Runtime Control tests pass normally
  and under Race; `go vet ./internal/runtimecontrol ./internal/httpapi` passes.
- SC-024's Owner permission editor now has real-Chrome coverage for keyboard
  type-to-select (`o` → OPERATE, `n` → NONE) using CDP input, in addition to
  native Tab/Enter traversal, save/readback/revoke, RTL and touch. The test
  stores only `PREPARED_NOT_ENFORCED` qualification grants and asserts there
  are no GO, publish or Blackout requests. Full `STAGECORE_TEST_CHROME=... go
  test ./internal/httpapi -count=1` passed (86.4 s); the focused browser test
  passed under Race; JS syntax, Vet and diff checks passed. The SC-024 matrix
  now records this completed keyboard gate while whole-server policy cutover
  and production activation remain open/off.
- Companion transport timeline changes remain software-local and tested as
  recorded above. Local functional HEAD before this checkpoint is the seven
  commits listed above; run `git rev-parse HEAD` to read the final local tip.
  No new CI was launched for unpushed commits. The failed exact-head run is
  complete, so recheck remote/PR and all exact-head workflows, then push the
  validated batch by normal fast-forward to PR #534. Wait for the new exact
  Core run before another push.
- Next independent gap after the push: continue SC-002 offline boot/session
  recovery software review without writing the host clock, then resume the
  SC-001–SC-025 matrix and isolated Raspberry Pi gate. The prior qualification
  Pi endpoint did not pass identity verification; do not use that ambiguous
  alias. Preserve existing performance measurements unless code or lab access
  changes justify a new run.
- Safety remains unchanged: no production services/data/Vault, real-device
  commands, firmware, production Project ACL, force push, new branch/PR, or
  merge.

## Resume update — 2026-11-14 23:38 UTC · batch pushed, exact-head CI active

- Normal fast-forward push completed from `c52a453c2d838935e1f7601fa748cec2e43ec725`
  to `460db92eb9a8457b840aa0ddd7aef1b5ebb91c43` on
  `integration/stagecore-rc`, updating existing OPEN/Draft PR #534. Local,
  remote and PR heads matched at `460db92`; the worktree was clean immediately
  after push. This note is a local checkpoint commit; do not push it or any
  further commit until the active Core run finishes.
- Exact-head workflows: Core `43654085363` is still running Go 1.26 and Go
  1.27 jobs; Companion Core `43654085383` passed both Swift checks; Camera
  Relay `43654085434` passed. Wait for the full Core result before another
  push, then verify remote, PR and workflow heads again.
- Read-only SC-002/#501 review confirms PR #504 remains a read-only clock/RTC
  dashboard and PR #505 is a local PHONE/MANUAL correction preview guarded by
  Owner, CSRF and active-session checks. Neither writes the system clock;
  neither qualifies RTC recovery or reauthentication after a real correction.
  Local browser sessions are eight-hour absolute-expiry credentials rotated
  offline by `/api/v1/auth/renew` while still valid. The clock test coverage
  exercises expiry and RTC evidence but not a real WAN-off Pi cold boot. Do
  not duplicate #504/#505 or run privileged clock commands. Next after CI is
  to inspect the session/cookie behavior under deterministic clock correction
  and identify a safe software-only regression; actual RTC and cold boot still
  need an isolated Pi.
- No service, clock, Project data, real device, or production ACL was touched.

## Resume update — 2026-11-14 22:55 UTC · browser gate repair pushed

- Local, remote branch, and OPEN/Draft PR #534 are now at
  `c52a453c2d838935e1f7601fa748cec2e43ec725`. The batch contains the prior
  local checkpoint `0ac5d50`, the browser fixture repair `66626e4`, and this
  checkpoint `c52a453`. The push was a normal fast-forward from `774414d`.
- Exact-head workflows have started: Core `43650575318` is in progress,
  Camera Relay `43650575301` is in progress, and Companion Core `43650575320`
  is queued. Do not push again until Core completes and the branch, PR, and
  exact workflow heads are rechecked.
- The failed `774414d` Core run `43649877977` was traced to the new browser
  timeline fixture omitting the application's `fmtDate` helper. With that
  fixture contract restored, the entire seven-test Browser Cue Delivery gate
  passed locally on Chrome with Go 1.27.1/macOS ARM64. The test now sends
  uncaught JavaScript errors to its failure report rather than waiting for its
  30-second timeout. Go vet and diff checks passed.
- Worktree was clean after the push. Next software gap: examine Companion's
  authenticated WebSocket connect/disconnect, project assignment and session
  timeline paths; add evidence only for scoped Hub transport, preserving
  replacement fencing and explicitly avoiding claims of app, adapter, ACK or
  physical output health. Keep any changes local while exact-head CI runs.
- Safety remains unchanged: no production service/data/Vault, real-device
  commands, firmware, production ACL activation, force push, or merge.

## Resume update — 2026-11-14 23:17 UTC · linked integration PR audit

- No worktree source changes were made in the linked repositories. Their
  existing PR heads and latest checks match:
  - TabletPlayer #36: `91613da1c8e0455a501c7be4b6df8ac1ce73a6bc`, OPEN/Draft,
    `tablet-player` pass (run `42719885664`).
  - ESP32 DMX #39: `3ad774988b2f78eb385f1046f7668a7f29049ab1`, OPEN/Draft,
    build variants and host contract pass (run `42759555665`).
  - ESP32 Camera #20: `455b066c6520e1c8a576d26dd2afd4b377652dde`, OPEN/Draft,
    hardware-probe workflow pass (run `42761096513`).
  - StageLaser #56: `b14a5789132549f2724f77b342bbe4c6b0fc880c`, OPEN/Draft,
    no-actuation firmware build and host contract pass (run `42765890415`).
  - StageLaser #57: `c4ff10b37dc2fbfd3ebe31e55f0f8d4869d3dc54`, OPEN (not Draft),
    no-actuation firmware build and host contract pass (run `38054411190`,
    exact to this current head).
- These checks are on the exact current heads, all PRs report a clean merge
  state, and no linked PR was changed or merged. Hardware acceptance for
  probe/firmware behavior remains separate from these host/build checks.
- Main PR #534 stays OPEN/Draft. Local HEAD is `7d1f03e`; remote remains
  `c52a453`. Local commits ahead are `0b57512`, `dca2ef6`, `6e170f5`, and
  `7d1f03e`. Core run `43650575318` is still running Go 1.26 Race on exact
  remote head `c52a453`; ARM64 is pending. Companion Core and Camera Relay pass
  on `c52a453`. No push until the Core run completes and exact heads are
  rechecked.
- Next: finish the `SC-001–SC-025` and Issue #511 evidence audit, including
  Mac Companion, Camera Relay, StageAudio, OSC/VDMX/Ableton/QLC+ contracts and
  the isolated Pi access gate, then push the already-tested Companion batch
  normally when Core CI clears. Do not create a new branch or PR.
- Safety remains unchanged: no production service/data/Vault, real-device
  commands, firmware, production ACL activation, force push, or merge.

## Resume update — 2026-11-14 23:15 UTC · Companion transport qualification

- Local HEAD is `6e170f5ac019e6f490a86618862f00938f9b3b37`; remote and OPEN/
  Draft PR #534 remain at `c52a453c2d838935e1f7601fa748cec2e43ec725`. Three
  local commits are ahead of remote: `0b57512` (browser-repair checkpoint),
  `dca2ef6` (project-scoped Companion Hub transport events), and `6e170f5`
  (SC-015 matrix update). Handoff refresh is being committed separately.
- Do not push yet. Exact-head Core run `43650575318` is still in progress on
  remote `c52a453`: Go 1.27 Test/Vet passed; Go 1.26 Test, Browser Cue
  Delivery, and Vet passed; Go 1.26 Race is running and ARM64 is pending.
  Companion Core `43650575320` and Camera Relay `43650575301` passed. Recheck
  exact PR/branch/workflow heads after Core finishes before normal push.
- Companion transport events are emitted only from authenticated Hub runtime
  sockets and scoped from the current Machine Role assignment. They record
  connection generation and fixed disconnect reasons for socket close, Hub
  restart, session revocation, replacement, and Project-scope change. Session
  Memory reads Stage Device and Companion transitions through separate partial
  Project/time indexes (Migration 64 for Companion), allowlists the displayed
  payload, and labels socket evidence without implying app readiness, adapter
  health, ACK, or physical output. Tests cover reconnect, stale replacement,
  Project reassignment, revocation, Hub close, wrong-source/project filtering,
  query indexes, time scoping, the 500-event cap, UI filtering/comparison, and
  export privacy.
- Final local verification: normal tests passed for Companion Channel,
  Session Memory, Store and DB; focused Race passed for all four Companion
  lifecycle scenarios and the full Companion Channel/Session Memory Race run
  passed before the final revocation assertion; Go vet passed across affected
  packages and HTTP API; the seven-test Chrome Browser Cue Delivery gate passed
  locally on Go 1.27.1/macOS ARM64; Session Memory Node tests passed 7/7;
  JavaScript syntax and `git diff --check` passed. An initial local package run
  found two schema-version assertions fixed from 63 to 64; the final package
  run passed.
- SC-015 remains Partial for Companion application state, adapter endpoint
  health and physical output confirmation. Next after CI: push this completed
  batch normally to PR #534, wait for new exact-head workflows, then continue
  the matrix audit with Pi ARM64 performance access and the highest remaining
  software-only integration gap. Earlier Mac/Pi measurements are not repeated
  without a code or environment change that justifies it.
- Safety remains unchanged: no production service/data/Vault, real-device
  commands, firmware, production ACL activation, force push, or merge.

## Resume update — 2026-11-14 22:54 UTC · Core browser gate repair

- PR #534 and `origin/integration/stagecore-rc` remain at
  `774414d6a8c2ab6c4c597f886eb0dd7e5bfea9bc`; local HEAD is `0ac5d50` with
  this browser-gate repair still uncommitted. Core run `43649877977` completed
  with failure only in Go 1.26 Browser Cue Delivery. Its Go 1.26 Test and
  Go 1.27 Test/Vet passed. Companion Core `43649877980` and Camera Relay
  `43649877969` passed. All workflows are now completed; no new commit has
  been pushed since the failure.
- Failure was reproduced locally with Chrome. The new Session Memory browser
  fixture called `sessionEventTimelineMarkup`, which uses the app's `fmtDate`
  helper, but the test page had not defined that app-provided helper. The
  fixture now provides the same date-format contract and reports uncaught
  browser exceptions directly to the Go test instead of timing out without a
  JavaScript error. The exact seven-test Browser Cue Delivery command passed
  locally on Go 1.27.1, macOS ARM64; `go vet ./internal/httpapi` and
  `git diff --check` passed.
- Next: commit this focused harness repair and checkpoint, verify PR/remote
  heads and absence of active Core CI again, then push normally to #534. Wait
  for Go 1.26 Browser Cue Delivery and the rest of exact-head Core CI before
  any subsequent push. The Companion transport timeline work is deferred until
  this CI regression is cleared.
- Safety limits remain unchanged: no production service/data/Vault, real
  device commands, firmware, production ACL activation, force push or merge.

## Resume update — 2026-11-15 · browser CI repair and SC-007 acceptance

- Repository: `ali96adil/StageCore`; branch `integration/stagecore-rc`; existing
  PR #534 remains OPEN/Draft. Before this handoff update, Local HEAD was
  `570eaec487a24b95b8a0d7907cd1c1a67c77e618`; Remote/PR HEAD remains
  `6529a8bbca33e9bdc974f517ce8cdbb81eed75b5`. New code commits are
  `3e15327` (make the Cue history browser gate re-query the toolbar after
  asynchronous DOM replacement) and `570eaec` (add simulated Chrome lighting
  preview recovery and copy-to-Draft acceptance). Existing local checkpoint
  `d3209cd` records the prior pushed batch. This handoff commit becomes the
  new local tip; inspect `git rev-parse HEAD` for its full ID. No new push has
  been made yet.
- Exact-head Core run `43656497568` on `6529a8b` completed **failed** in Go 1.26
  Browser Cue Delivery. Its failure was the new SC-009 browser assertion:
  after Undo removed the row, the test captured `sc009ProjectDraftHistory`
  once while Cue Studio was still rebuilding the toolbar, then waited forever
  on that disconnected/null reference. The browser trace showed successful
  POST Undo and the subsequent history GET. The test now resolves the toolbar
  afresh on every readiness poll. This is a browser-test synchronization fix;
  no product change was needed. Companion Core `43656497653` and Camera Relay
  `43656497519` passed on `6529a8b`. All three workflows are completed; no
  active Core run remains.
- SC-007 now has `TestBrowserLightingPreviewCopiesValuesToDraftWithoutDispatch`
  in the Core browser gate. Its local isolated Chrome/SQLite flow proves the
  simulated continuous-level/blackout preview, unmapped-channel error and
  recovery, conversion/copy into a Draft Cue, and absence of preview network
  requests, execution requests, Runtime Session and Published Snapshot. The
  matrix records this tested portion as qualified; reassignment while the
  editor remains open is still partial. No physical DMX command is sent.
- Verification passed locally: SC-009 real-Chrome test repeated three times;
  SC-007 real-Chrome test; both browser tests under Go Race; `go vet
  ./internal/httpapi`; the 71-test Operator Node suite; JS syntax checks and
  `git diff --check`. Browser tests use Chrome and temporary SQLite on macOS;
  the corrected synchronization still needs the new Linux exact-head Core CI.
- Immediate next action: recheck PR/branch HEAD and ensure no newer/active Core
  run, push the committed batch as a normal fast-forward to the existing PR,
  then wait for Core, Companion Core and Camera Relay on the exact pushed SHA.
  Fix any failure before another push. Do not claim overall Software Complete.
  After CI, prioritize isolated Pi 4 ARM64 performance qualification and the
  highest remaining software-only matrix gap; do not repeat prior Mac
  performance measurements without a measured reason.
- Hardware Qualification Pending: Pi ARM64 qualification and physical device
  acceptance. Production Release Approval Pending: release and any production
  ACL enablement. Project ACL/SC-024 remains disabled in production. Production
  Hub/Camera Relay/data/Vault, original show data and real devices remain
  untouched; no firmware flash, force push, merge, paid operation or backup
  deletion was performed.

## Resume update — 2026-11-15 · lighting Project fence follow-on

- Branch `integration/stagecore-rc`, existing PR #534 OPEN/Draft. Remote PR and
  branch remain at `0c0540574b73ab80557b31d20bfe46c6c1b6a08a`. Local HEAD before
  this Handoff edit is `34edd9adc425654735ef3309ed4eb6b167174a3b`; the two
  committed follow-ons are `949c3b7` (fence lighting Cue saves to the
  originating Project) and `34edd9a` (qualification matrix evidence). No
  follow-on push has been made; read `git rev-parse HEAD` after the handoff
  checkpoint commit for the resulting full local tip.
- Current exact-head workflows for remote `0c05405`: Core `43657410250` is
  still active. Go 1.27 Test/Vet pass; Go 1.26 Test, Browser Cue Delivery and
  Vet pass, including the Linux regression for SC-009's re-query of the Cue
  history toolbar. Go 1.26 Race is running and ARM64 product builds are
  pending. Companion Core `43657410210` and Camera Relay `43657410252` have
  passed. No push until Core finishes, then verify exact remote/PR heads again.
- SC-007 browser acceptance exposed and now closes a Project-crossing race:
  `lighting-authoring.js::saveCue` captures the source Project ID, uses it for
  both translation and Draft write, and after the asynchronous translation
  confirms the editor remains connected to that Project. Lighting model reads
  are held locally until the response still matches the selected Project, so a
  delayed response cannot replace the destination Project's model. The Chrome
  regression explicitly gates the second translation response, switches to a
  second temporary Project, releases the response, then asserts no Cue write
  reached that destination. A defensive release unblocks the isolated test
  server on failures. No physical output or runtime command is used.
- Local verification on this follow-on: real Chrome SC-007 test passed three
  consecutive runs and passed under `go test -race`; SC-009 test had passed
  three consecutive runs and both SC-007/SC-009 passed together under Race
  before the follow-on. The 71-test focused Operator Node suite, `go vet
  ./internal/httpapi`, Go formatting, JavaScript syntax and `git diff --check`
  passed. The first local browser attempts exposed and fixed test-fixture
  issues (an overly broad gate and an unnecessary page-name guard); these
  failed attempts did not touch remote CI or any production state.
- SC-007 remains Partial: the in-flight save Project fence, simulation-only
  preview/error recovery and Draft copy are qualified; broader editor behavior
  through Project reassignment and physical DMX output are not. First next
  action: wait for exact `0c05405` Core Race/ARM64 completion, handle any
  failure, then push committed `949c3b7` and `34edd9a` as a normal fast-forward
  to the same PR and wait for all exact-head checks before another push.
  Continue with isolated Pi 4 performance qualification and the next software
  matrix gap. No production ACL/SC-024 enablement, production service/data,
  original show data, real device, firmware, force push, merge, paid operation
  or backup deletion is allowed.

## Resume update — 2026-11-15 · matrix evidence reconciliation

- Before this checkpoint edit, local `integration/stagecore-rc` HEAD was
  `8c19c1ffade1f70f5659af61b544721a916d5c42`; remote and Draft PR #534 remain
  at `0c0540574b73ab80557b31d20bfe46c6c1b6a08a`. Commit `8c19c1f` reconciles
  the SC-013 matrix row with the existing real-Chrome nested Mixed Cue
  Inspector test. The handoff checkpoint commit is local; inspect
  `git rev-parse HEAD` for its resulting full tip. Four local commits are
  currently ahead of the remote: `949c3b7`, `34edd9a`, `826f3d6`, and
  `8c19c1f`, plus this checkpoint commit after it is created.
- SC-013 was not an implementation gap: `TestBrowserCreatesNestedMixedCueChainInDraft`
  already opens a three-level Draft Inspector, checks six nested Actions,
  simulates a virtual offline target for all six, and verifies zero fetch or
  GO/STOP/Blackout requests. This browser test is in the exact-head Core
  Browser Cue Delivery gate on `0c05405`, which passed. Remaining SC-013 gaps
  are fresh readiness/target metadata, missing Companion/DMX browser
  scenarios, assumed-latency meaning and fade detail beyond `fade_ms`.
- Performance qualification already contains isolated Raspberry Pi 4 ARM64
  measurements for 24–200 Action Mixed Cues, linked chains through 100 Cues /
  200 Actions, scheduled starts, simultaneous bursts, sustained 100-Cue / 200-
  Action Rehearsal, and STOP/Blackout under delayed load, with P50/P95/P99,
  jitter, GC/allocations and process CPU/RSS. It records the binary source and
  verifies no Cue Engine, Runtime Control, simulator, domain or publish runtime
  source changes since that benchmark. Do not rerun those Pi/Mac measurements
  without a runtime code/environment change or missing evidence.
- Core `43657410250` remains active on exact remote `0c05405`: Go 1.27 and Go
  1.26 Test/Browser/Vet passed; Go 1.26 Race is running and ARM64 builds are
  pending. Companion `43657410210` and Camera `43657410252` passed. No push
  while Core is active. After it completes, verify PR/remote HEAD and all
  workflow SHAs, then push the four local commits as a normal fast-forward to
  the existing PR. Continue exact-head CI before the next push.
- No production service/data/Vault, original show data, real device or firmware
  was touched; Project ACL/SC-024 remains disabled in production. No force push,
  merge, paid operation or backup deletion was performed.

## Resume update — 2026-11-15 · nested Mixed Inspector fault scenarios

- Branch `integration/stagecore-rc`, PR #534 OPEN/Draft. Remote remains
  `0c0540574b73ab80557b31d20bfe46c6c1b6a08a`; Local HEAD before this update is
  `435a214e1d0d5e53f4ade479650d090ca8a95833`. Seven commits are ahead of the
  remote (`949c3b7`, `34edd9a`, `826f3d6`, `8c19c1f`, `156eff7`, `169be4a`,
  `435a214`); this Handoff commit becomes the eighth. No push has been made.
- Extended the existing Chrome nested Mixed Cue Inspector regression: the
  virtual offline-target case blocks all six Actions; a missing-DMX case blocks
  only the Lighting Action; and test-only Machine Role metadata makes a missing
  Companion scenario mark all six blocked. Each scenario asserts no fetch or
  GO/STOP/Blackout request. The Companion case validates client-side scenario
  classification only; it does not claim real Companion connectivity. The
  updated test passed normally and under Race on local Chrome. Matrix SC-013
  now records these exact test boundaries.
- Existing exact-head workflows remain on `0c05405`. Go 1.27 Test/Vet and Go
  1.26 Test/Browser/Vet pass; Go 1.26 Race remains active and ARM64 product
  builds are pending. Companion Core `43657410210` and Camera Relay
  `43657410252` pass. Wait for Core completion before pushing any of the local
  commits; then verify exact PR/branch HEAD and workflow SHA before normal
  fast-forward to #534.
- Performance report review confirms Pi 4 ARM64 Mixed/linked, scheduled,
  burst, sustained 100-Cue/200-Action, STOP/Blackout and process CPU/RSS/GC
  evidence already exists and Cue Engine/Runtime Control/publish runtime
  sources have not changed since the benchmark binary. Keep the measured
  qualification; do not rerun without a runtime/source or environment change.
- All new lighting and Cue Inspector tests use temporary SQLite, local Chrome
  and simulation-only data. Production services, Vault/show data and hardware
  were not used; Project ACL/SC-024 remains off in production. No firmware,
  force push, merge, paid operation or backup deletion.

## Resume update — 2026-11-15 · lighting save and read response fences

- Main repository branch `integration/stagecore-rc`; PR #534 OPEN/Draft. Remote
  branch/PR remain at `0c0540574b73ab80557b31d20bfe46c6c1b6a08a`. Local HEAD
  before this Handoff edit is `ae7e7956e0241b779cac2a58fabb5646d3d0f3b6`; ten
  commits are ahead (`949c3b7`, `34edd9a`, `826f3d6`, `8c19c1f`, `156eff7`,
  `169be4a`, `435a214`, `ac997bd`, `0f8bd07`, `ae7e795`). The Handoff update
  will be the eleventh local commit. No push has been made.
- SC-007 `lighting-authoring.js` now retains the source Project ID across
  translation and Cue writes, abandons a stale save after the editor is
  detached/reassigned, and keeps fetched lighting models local until the
  selected Project still matches. Real-Chrome acceptance uses temporary
  Projects and SQLite, blocks a second translation response while switching
  Projects and proves no Cue write reaches the destination; it also blocks a
  source Project lighting-model response until after the destination renders
  and proves the stale response cannot replace that view. Preview error
  recovery and copied-value Draft persistence stay in the same test. The test
  sends no physical or runtime command and publishes no Snapshot.
- SC-007 browser test passed three consecutive normal runs and one Race run;
  `go vet ./internal/httpapi`, JS syntax, Operator Node tests, `git diff
  --check` and Go formatting passed. The separate SC-013 nested Mixed Inspector
  now checks offline target, missing DMX and synthetic Machine Role/missing
  Companion scenarios, each without fetch or GO/STOP/Blackout; normal and
  Race browser runs passed. The synthetic metadata tests scenario classification
  only, not a real Companion connection.
- Current remote Core `43657410250` remains **in progress** on `0c05405`:
  Go 1.27 Test/Vet and Go 1.26 Test/Browser/Vet passed, Go 1.26 Race is still
  running, and ARM64 builds are pending. Companion `43657410210` and Camera
  `43657410252` pass. Keep all ten local commits unpushed until Core completes;
  then verify branch/PR and workflow exact SHAs before normal fast-forward to
  the existing #534. After push, wait all exact-head workflows before another
  push.
- Pi 4 ARM64 performance evidence remains the existing qualified dataset; no
  runtime source changed since its measured binary. Do not rerun solely for a
  newer commit label. Project ACL/SC-024 remains production-off. Production
  services/data/Vault, original show data and real devices remain untouched;
  no firmware, force push, merge, paid action or backup deletion.

## Resume update — 2026-11-15 · simulated SHOW stop and authoring re-entry

- Main branch `integration/stagecore-rc`, Draft PR #534. Remote remains
  `0c0540574b73ab80557b31d20bfe46c6c1b6a08a`. Local HEAD before this
  checkpoint is `830e095eaea3a29a1b5496cbd557220d055c8819`; twelve commits are
  ahead of the remote. This checkpoint commit becomes the thirteenth. Earlier
  local lighting/Cue-inspector commits remain preserved; no push has occurred.
- SC-017 `TestBrowserShowWorkspacePinsRuntime` now drives the normal Runtime
  Stop button in real Chrome after opening a seeded active SHOW in temporary
  SQLite. The runtime uses only the `sim.test` adapter and an explicit no-op
  managed-output safety seam. The browser proves the only mutation is
  `runtime/stop-session`, the completed Session is no longer active, SHOW UI
  restrictions clear, and Cues/authoring controls return. The GO, Cue STOP and
  Emergency Blackout controls are checked before exit; no Cue executes and no
  device or live output is contacted. Matrix SC-017 now records this exact
  scope; keyboard/touch/RTL and target-width/role browser acceptance remain.
- Verification: focused Chrome browser test passed three consecutive runs and
  passed once under `go test -race`; `go vet ./internal/httpapi`, JS syntax,
  gofmt and `git diff --check` passed. The first test iteration exposed that
  this browser fixture had omitted the Cue Publish API registration; the test
  server now registers that existing route so re-entry is exercised through
  the actual UI/API path.
- Exact remote Core CI run `43657410250` remains in progress on
  `0c0540574b73ab80557b31d20bfe46c6c1b6a08a`: Go 1.27 is green; Go 1.26 is
  still running. Companion `43657410210` and Camera `43657410252` were green
  at the last check. Do not push until Core finishes; then verify branch, PR
  and exact workflow SHAs before normal fast-forward to #534. Re-run exact-head
  Core/Companion/Camera checks after that push and do not enqueue another push
  while they are active.
- Pi 4 performance qualification remains based on the already documented
  ARM64 engine/linked-chain/STOP/Blackout measurements; this UI-only test change
  does not alter the measured runtime binary. Keep Engine timing separate from
  ACL/HTTP and external outputs. SC-024/Project ACL remains off in production.
- No production service/data/Vault, original show data or real device was
  touched. No firmware, force push, merge, paid operation or backup deletion.
- Next: check exact-head Core CI. When it completes, push the preserved local
  batch to existing PR #534 using a normal fast-forward, then watch all exact-
  head checks. Continue with the next independently testable software gap;
  retain hardware/production qualification as separately pending.

## Resume update — 2026-11-15 · RTL, keyboard and touch workspace gate

- Main branch `integration/stagecore-rc`, Draft PR #534. Remote is still
  `0c0540574b73ab80557b31d20bfe46c6c1b6a08a`. Local HEAD before this
  checkpoint is `a0c08122007f1ffeb5176889e9895c5a835b563d`; fourteen commits
  are ahead of remote. This Handoff commit becomes the fifteenth. No push has
  occurred.
- Extended SC-017's actual SHOW-stop/re-entry browser gate with native CDP
  keyboard Enter and touch events, a 390×844 device-emulated viewport, Arabic
  RTL layout, visible keyboard focus and horizontal overflow checks for active
  SHOW, post-stop, Cues and inactive Runtime. The script's viewport check is
  synchronized with DevTools before page assertions; no real tablet is used.
  SC-017 matrix now records Owner/390×844 coverage and still marks other roles
  and desktop/tablet sizes as open.
- The completed Chrome test passed three consecutive normal runs and one Race
  run; `go vet ./internal/httpapi`, JS syntax, gofmt and `git diff --check`
  passed. Session termination remains the normal API/Service route against
  temporary SQLite with a no-op safety seam; only `/runtime/stop-session` is an
  API mutation. No Cue ran, no real output was contacted and no Snapshot was
  created by the browser flow.
- Remote Core CI `43657410250` is still **in progress** at exact SHA
  `0c0540574b73ab80557b31d20bfe46c6c1b6a08a`; Go 1.27 is successful and Go
  1.26 has not completed. Companion `43657410210` and Camera `43657410252` were
  successful at last check. Keep the fourteen local commits unpushed until
  Core completes. Verify same PR/branch and exact check SHAs before a normal
  fast-forward; then wait all checks before another push.
- Existing Pi 4 ARM64 Mixed Cue/linked-chain, scheduling, burst, sustained
  rehearsal and STOP/Blackout measurements remain valid because this batch
  changes only the browser test and qualification docs. Keep Engine, ACL, HTTP
  and external-output time separate. Production ACL remains disabled.
- No production services/data/Vault, original show, real devices or firmware
  touched; no force push, merge, paid action or backup deletion. First next
  action: recheck Core CI; then push only after completion and continue the
  next software-only gap while keeping hardware and production approval
  separately pending.

## Resume update — 2026-11-15 · workspace breakpoint coverage

- Branch `integration/stagecore-rc`, Draft PR #534. Remote remains
  `0c0540574b73ab80557b31d20bfe46c6c1b6a08a`. Local HEAD before this
  checkpoint is `962d2f69a2a97d3779746654ae60a2084bc58772`; sixteen commits
  are ahead. This Handoff becomes commit seventeen. No push has been made.
- SC-017 browser acceptance now checks the Owner workspace at mobile 390×844,
  tablet 768×1024 and desktop 1280×800 while in RTL. Cues and inactive Runtime
  are checked at all three sizes; active SHOW and post-stop states are checked
  at 390×844. All views assert no horizontal page overflow. Chrome DevTools
  applies each viewport before the browser continues. Native keyboard/touch
  navigation and real normal session exit remain part of the same isolated
  browser flow. SC-017 matrix records the still-open browser role matrix.
- Verification after the viewport changes: Chrome browser test passed three
  consecutive normal runs and one Race run; `go vet ./internal/httpapi`, JS
  syntax, gofmt and `git diff --check` passed. Test fixture remains temporary
  SQLite + `sim.test` plus a no-op stop-safety seam, with no Cue dispatch or
  actual device/hardware request.
- Core run `43657410250` remains in progress at exact remote SHA
  `0c0540574b73ab80557b31d20bfe46c6c1b6a08a`; Go 1.27 is green and Go 1.26
  has not completed at the last check. Companion `43657410210` and Camera
  `43657410252` remain green from their last exact-head checks. Wait before any
  push, then confirm the PR and all required check SHAs and push normally to
  #534; wait on the pushed SHA before another batch push.
- No performance runtime source changed; retain the already recorded Pi 4
  ARM64 Mixed/linked-chain, schedule/jitter, CPU/RAM/GC, burst/sustained and
  STOP/Blackout evidence, with ACL/HTTP/external output time kept separate.
  Production ACL stays disabled. No production services, show data, Vault,
  real devices, firmware or backup artifacts were touched; no force push,
  merge or paid action.

## Resume update — 2026-11-15 · role gate and Core contract regression

- Branch `integration/stagecore-rc`, Draft PR #534 OPEN. Previous push advanced
  the remote from `0c0540574b73ab80557b31d20bfe46c6c1b6a08a` to
  `12d7bbae3b9c1d52b9b36cb0a3dbaa3ca4b1f1ec`. Local HEAD before this Handoff
  is `6e7b642a2be4f3fb82c38c58592d022ef23a3c11`, two commits ahead; this
  checkpoint will be the third. No force push or branch change.
- Core CI `43660680334` completed **failure** at `12d7bba`: Go 1.26 and 1.27
  both failed only `internal/operatorweb/TestLightingCueBuilderOrdersAcrossAllRevisionCues`,
  whose static marker still required the old `Promise.all` assignment after
  the already-committed Project-response fence refactor changed the source
  shape. The Cue-order behavior test itself remains; its contract now checks
  all-revision Cue loading/order allocation and that stale Project responses
  are rejected before view assignment and Cue writes. Full local `go test ./...`
  now passes on Go 1.27.1. Re-run both CI versions after push.
- SC-017 Chrome coverage now authenticates actual temporary Owner, Technician,
  Operator and Viewer accounts. Technician and Operator exit simulated SHOWs;
  Viewer lacks Cue editing and has disabled GO/STOP/Blackout while pinned to a
  seeded SHOW, which Owner safely exits. All API session changes are
  stop-session requests; fixtures use temporary SQLite, `sim.test` and an
  explicit no-op safety seam. Owner layout remains checked at 390×844,
  768×1024 and 1280×800 RTL; other roles are checked at 390×844. Matrix row
  SC-017 describes this boundary and remaining role-at-wide-viewports gap.
- Verification: `go test ./...` passed; Chrome SC-017 passed three consecutive
  runs and a Race run after the role test additions; `go test ./internal/operatorweb`,
  `go vet ./internal/httpapi ./internal/operatorweb`, JS syntax, gofmt and
  `git diff --check` passed. Companion `43660680338` and Camera Relay
  `43660680351` passed on exact `12d7bba`; Core must be rerun after this fix.
- Next: verify no workflow remains active on `12d7bba`, confirm the existing
  PR and remote SHA, then push these two test commits plus this Handoff using a
  normal fast-forward. Wait Core, Companion and Camera checks on the new exact
  SHA before another push. No Pi performance runtime code changed; keep the
  existing ARM64 qualifications and Engine/ACL/HTTP/output timing separation.
  No production service/data/Vault, original show data, real device, firmware,
  backup, paid action, merge or force push was used.

## Resume update — 2026-11-15 · role coverage across workspace sizes

- Branch `integration/stagecore-rc`, Draft PR #534. Remote is
  `3f4f55b79b4fcdfa8da8d67e3212a628dbd835ee`. Local HEAD before this Handoff
  is `3e8625ed2c1c377c24e747c9a63dbe4f6a532e9c`, one commit ahead; this
  checkpoint becomes the second local commit. No new push while Core is active.
- Extended `TestBrowserShowWorkspacePinsRuntime` so real temporary Owner,
  Technician, Operator and Viewer sessions check Cues, inactive Runtime and
  active SHOW overflow/layout at 390×844, 768×1024 and 1280×800 in RTL.
  Existing native CDP keyboard/touch navigation remains Owner + mobile; role-
  specific GO/STOP/Blackout and authoring gating is checked at all viewports.
  The explicit test viewport channel applies device metrics before each
  assertion. Matrix SC-017 now reports four-role/three-size software evidence
  and the remaining input/device limits.
- Verification for this extension: focused Chrome test passed three
  consecutive runs and one Race run; `go vet ./internal/httpapi`, JavaScript
  syntax and `git diff --check` passed. The full local `go test ./...` passed
  immediately before this viewport-only test extension; the preceding code
  batch also passed the full suite on Go 1.27.1.
- Exact remote Core run `43661757214` is **in progress** at SHA
  `3f4f55b79b4fcdfa8da8d67e3212a628dbd835ee` (Go 1.26/1.27 pending).
  Companion `43661757151` and Camera Relay `43661757135` passed at that SHA.
  Wait for Core completion before pushing the local viewport commit, then
  verify PR #534/branch and run all checks on the new exact SHA before another
  push.
- No runtime Engine/ACL/output code changed, so existing Pi 4 ARM64 performance
  evidence remains applicable; preserve Engine vs ACL/HTTP/external-output
  timing separation. Production ACL remains off. No production services/data,
  original show, Vault, real devices, firmware, backups, paid action, merge or
  force push were touched.

## Resume update — 2026-11-15 · native navigation across operator roles

- Branch `integration/stagecore-rc`, Draft PR #534. Remote remains
  `3f4f55b79b4fcdfa8da8d67e3212a628dbd835ee`. Local HEAD before this Handoff
  is `1caec125e72a32f8f2f60ad589b351bd353a5176`, three commits ahead; this
  checkpoint will be the fourth. Do not push while its Core CI is active.
- SC-017's real-Chrome fixture now switches through authenticated temporary
  Owner, Technician, Operator and Viewer sessions and drives Cues navigation
  with native CDP Enter and Runtime navigation with emulated touch for every
  role at 390×844 RTL. Each role checks Cues, inactive Runtime and active SHOW
  layout at 390×844, 768×1024 and 1280×800. Viewer has no edit/GO/STOP/Blackout
  capability; Technician/Operator use the isolated normal STOP; Owner exits
  the final Viewer-seeded SHOW. All temp SHOWs complete through the no-op
  safety seam and only the four `runtime/stop-session` API requests occur.
  The matrix records the remaining other-route and physical tablet limits.
- Verification for this native-input extension: Chrome test passed three
  consecutive runs and a Race run; `go vet ./internal/httpapi`, JS syntax and
  `git diff --check` passed. The full `go test ./...` passed before these test-
  only viewport/input additions; local browser normal/Race checks passed after
  them.
- Remote Core `43661757214` is still in progress on exact `3f4f55b`:
  Go 1.27 is successful; Go 1.26 Race remains active. Companion
  `43661757151` and Camera Relay `43661757135` are successful at the same SHA.
  Wait for that Core completion, confirm same PR/branch/remote SHA, then push
  the preserved local commits normally and wait all checks on the new SHA.
- This batch touches only browser tests and qualification docs; no Cue Engine,
  Runtime Control, ACL or performance source changed. Keep Pi 4 ARM64 results
  and separate Engine/ACL/HTTP/output timing. Production ACL remains off; no
  production services/data/Vault, original show, real device, firmware,
  backup, paid action, merge or force push was used.

## Resume update — 2026-11-15 · Route Draft history, branching fix, restore and browser acceptance

- Repository: `ali96adil/StageCore`, branch `integration/stagecore-rc`, Draft PR #534 OPEN. Before this checkpoint commit, local HEAD is `4f572b4` (full SHA recorded by Git); remote/PR SHA remains `82a02f983ac64dd19ee9f1231f397f575722b3e5`. Three local commits are ahead: `fabf66c` (bounded persistent Route Draft history), `b4e3bc1` (ProjectEdit-scoped HTTP API and route inventory), and `4f572b4` (Operator controls and browser/restore qualification). The Handoff itself is the fourth commit. No force push, branch change, PR creation or merge.
- The exact remote `82a02f9` checks have all passed: Core run `43664797691` Go 1.26 Race and Go 1.27; Companion run `43664797685` macOS replacement/media and Swift CompanionCore; Camera Relay run `43664797745` isolated/ARM64. The new local commits have not been pushed or CI-qualified yet. Recheck branch/PR and active workflows, push normally only if no relevant workflow is still active, then wait for all exact-new-SHA checks before any further push.
- Migration 65 adds per-revision/per-editor bounded Route history. Route create/edit/delete and Assistant Route proposal mutations record snapshots in the same SQLite transaction; undo/redo preserves Route and RouteAction IDs, checks revision-bound references and state hashes, respects the SHOW configuration lock, and never publishes a Snapshot. ProjectEdit HTTP reads/writes are no-store, actor-scoped, and mutation authorization is rechecked before response. Bilingual controls are enabled only for editable Drafts.
- Red-first regressions found a sequence gap when Cue or Route authors branched after Undo. Both histories now reuse the first discarded redo sequence; earlier undo remains available and the new branch can redo. Route browser acceptance drives real Chrome at 390×844 RTL with native Enter Undo and emulated touchscreen Redo, verifies no overflow or unrelated mutating request, and uses temporary SQLite plus simulated unlocked SHOW state. State Backup/Restore now verifies Route history and stable Route/RouteAction identity survive restore and remain undoable/redoable. Separate Cue/Route stacks and missing durable history for other Project configuration entities remain known SC-009 software gaps.
- Verification on this batch: `go test ./... -count=1` PASS; focused store and HTTP API Race suites PASS, including the real Chrome Route history test; Backup/Restore Race test PASS; real Chrome Route history test passed three consecutive normal runs; `go vet ./internal/db ./internal/store ./internal/httpapi ./internal/backup` PASS; Node suite 81/81 PASS; JS syntax, `gofmt`, and `git diff --check` PASS. Full tests ran against temporary SQLite and simulated data only.
- No Cue Engine/runtime/ACL performance path changed, so the saved Pi 4 ARM64 qualification for Mixed Cues, linked compound chains through 100 Cues/200 Actions, bursts, sustained rehearsal and STOP/Blackout remains applicable; do not repeat it without a code-path reason. Engine latency stays distinct from ACL/HTTP/external output. Route history authoring work makes no performance claim.
- Safety boundary unchanged: no production Hub/Camera Relay/service or data/Vault, original show, real output/device command or firmware, paid action, force push or merge; SC-024 remains opt-in/off in production. Related component PRs were not modified.
- First follow-up: push the four-commit local batch after confirming the already-passed exact base run has no active workflow; wait for new exact-head Core/Companion/Camera CI. Then continue with the next reproducible software gap from the matrix, prioritizing SC-009 remaining authoring-history boundaries or SC-015 output-health evidence. Preserve the performance and release safety boundaries above.

## Resume update — 2026-11-15 · failed-update rollback keeps Draft history usable

- Current remote/PR #534 HEAD is `0c8b965d2d2d0f3ac9f424cbac4bcabbf3088467`; local base was the same SHA before this follow-up. The branch remains `integration/stagecore-rc`, Draft and OPEN. New follow-up edits are confined to the rollback integration regression and SC-009 matrix evidence; no extra feature changes.
- Exact-head Core run `43668251167` at `0c8b965` has Go 1.27 PASS and Go 1.26 Race still pending. Companion run `43668251174` (macOS replacement/media and Swift CompanionCore) and Camera Relay run `43668251160` pass. Keep the current branch head stable until Core Go 1.26 finishes; then push this follow-up normally and qualify the next exact SHA.
- Extended `internal/update/rollback_database_test.go::TestIsolatedUpdateRollbackRestoresDatabaseAndPublishedSnapshot`: start with a Published immutable Snapshot plus a new editable Draft containing actor-scoped Cue and Route/RouteAction histories; simulate an update candidate that changes Project grants, creates a candidate-only Project and removes the published Snapshot, then force postflight failure. Automatic rollback restores SQLite integrity, baseline grants and the exact Snapshot bytes; both Cue and Route history remain undoable/redoable and preserve identifiers. The test uses temporary install/config/data/Vault roots and fake systemd/doctor seams; it does not run a host update service.
- Focused normal and Race rollback integration tests and `go vet ./internal/update` pass. The prior complete `go test ./... -count=1` passed before this isolated test extension; the new extension passed focused normal/Race execution. No Cue Engine/runtime/performance code changed; previous Pi 4 qualification remains applicable and was not repeated.
- Safety limits remain: no production service/data/Vault, original show, physical output/device or firmware, paid action, force push, PR merge, or SC-024 production activation.
- Next: wait for exact `0c8b965` Core Race, push the rollback qualification and matrix/Handoff follow-up as a normal fast-forward, then wait all checks on the new exact SHA. Continue from the remaining matrix gap: full-form/restart acceptance and authoring history for other Project configuration entities; use fake outputs and isolated data.

## Resume update — 2026-11-15 · persistent Cue and Route keyboard shortcuts

- Current pushed remote/PR #534 SHA is `0c8b965d2d2d0f3ac9f424cbac4bcabbf3088467`; PR remains OPEN/Draft on `integration/stagecore-rc`. Local HEAD before this Handoff is `baab96a` (full SHA is in Git), two commits ahead: `4ce9a16` adds failed-update rollback qualification for Cue/Route Draft histories; `baab96a` adds persistent Cue/Route keyboard shortcuts and browser acceptance. The Handoff itself will be the third local commit.
- Exact-head CI on `0c8b965`: Core run `43668251167` Go 1.27 PASS, Go 1.26 Race PENDING; Companion run `43668251174` and Camera Relay run `43668251160` PASS. Do not push until Core Go 1.26 completes. The two local follow-up commits are not part of that CI.
- Cue and Routing Draft history buttons now advertise and handle Control/Meta+Z for Undo, Control/Meta+Y and Shift+Z for Redo. Persistent history shortcuts ignore text inputs, selects, contenteditable targets, composition, and any open dialog; Cue dialog-local undo remains local to its editor. Real Chrome tests send Meta+Z and Meta+Shift+Z, verify text-field focus does not traverse the server history, then verify touch Redo at 390×844 RTL with no overflow or runtime/publish request.
- Latest local checks: Cue/Route Chrome tests passed normal execution twice before the final keyboard-Redo extension and passed once after it; both passed under Race after the final extension. Node suite 82/82, `go vet ./internal/httpapi ./internal/operatorweb`, both JS syntax checks, browser-fixture syntax checks and `git diff --check` pass. Update rollback test also passes focused normal and Race, and `go vet ./internal/update` passes (see preceding entry). The complete Go suite passed before the isolated rollback integration test was extended; the extension and UI changes passed focused normal/Race tests.
- No Cue Engine or runtime timing code changed; existing Pi 4 ARM64 performance qualification remains applicable. All app, browser and rollback fixtures use temporary databases and simulated/unlocked state. No production service/data/Vault, original show, physical device/output or firmware, paid action, force push, merge or production ACL activation.
- Next: wait for `43668251167` Go 1.26 Race to finish; then verify PR/remote SHA and active workflow set, push the local commits plus this Handoff with a normal fast-forward, and wait for all exact-new-SHA checks. Continue from SC-009 full-form/restart acceptance and remaining Project configuration history gaps.

## Resume update — 2026-11-15 · Mixed linked-Cue Draft history acceptance

- Branch remains `integration/stagecore-rc`, Draft PR #534 OPEN. Remote/PR HEAD is still `0c8b965d2d2d0f3ac9f424cbac4bcabbf3088467`; local HEAD is `7a99e083c2abb55401de8dac0ddeb02a2b3a3f50`, four commits ahead. Commit `7a99e08` extends the persistent Cue history browser test and SC-009 matrix. Worktree is clean at the time of this checkpoint. Do not push until Core run `43668251167` completes Go 1.26 Race on the exact remote SHA.
- The real-Chrome test now seeds a Lighting child Cue and a linked parent Mixed Cue containing parallel OSC and sequential MIDI Actions with a 250 ms start delay. It verifies Undo removes only the parent, Redo keeps the child and restores the exact parent Cue/Action IDs, action modes, delay and linked-Cue policy. It still proves touch Redo, no page overflow, no publish/runtime request, and no active Session or Published Snapshot. All data and targets are temporary/simulated.
- Verification for `7a99e08`: focused Chrome test passed once normally and three consecutive runs; focused Chrome Race test passed; `go vet ./internal/httpapi ./internal/operatorweb` passed; Core Operator Node suite passed 82/82; browser fixture `node --check`, `gofmt` and `git diff --check` passed. No Engine, ACL or runtime performance source changed, so the saved Mac/Pi qualification data was not repeated.
- Remote checks at `0c8b965` remain Core Go 1.27 PASS, Go 1.26 Race IN_PROGRESS; Companion replacement/media and Swift CompanionCore PASS; Camera Relay isolated/ARM64 PASS. The local follow-up stack is not included in those checks. After Core completes, verify PR and remote SHA again, push the preserved commits normally to this same branch/PR, then wait for all exact-new-head checks before another push.
- Next software gap after the CI gate: SC-009's full-form/restart acceptance, then durable history for additional editable Project configuration entities. Do not duplicate existing offline recovery, named checkpoints, Cue history, Route history or rollback coverage. Keep Engine, ACL, HTTP and external-output timing distinct. Pi lab qualification remains limited to the previously recorded isolated evidence; no production-like host load.
- No production Hub/Camera Relay/data/Vault/original show, real stage device, firmware, backup deletion, paid action, merge, force push, or production SC-024 activation was used.

## Resume update — 2026-11-15 · persistent Cue history across page reload

- Branch `integration/stagecore-rc`, Draft PR #534 remains OPEN. Remote/PR HEAD is `0c8b965d2d2d0f3ac9f424cbac4bcabbf3088467`; local HEAD is `af3b8ed0cecf31e110b4a8e6c531f14aaa8acd4d`, six commits ahead and clean. The unpushed sequence includes rollback, persistent history shortcuts, their checkpoints, Mixed/linked Cue history acceptance, and this reload acceptance. Preserve the stack and push normally only after Core run `43668251167` completes on the exact current remote SHA.
- The real Chrome Draft-history flow now performs an actual full page reload after keyboard Undo/Redo and touch Redo, then confirms the database-backed Cue/Action rows and `CanUndo` state return. The parent is a Mixed Cue with parallel OSC plus delayed sequential MIDI Actions linked to a Lighting child; identity, mode, start-delay policy and linked Cue policy are checked after Redo. No Snapshot, Session, publish or execution request is made.
- Reload acceptance passed four consecutive normal runs across the two focused test rounds, plus one Race run. `go vet ./internal/httpapi ./internal/operatorweb`, the Core Operator Node suite (82/82), JS fixture syntax, `gofmt` and `git diff --check` passed. SC-009 matrix now records the tested reload behavior; full Cue-form-specific recovery remains open. No Cue Engine/ACL/runtime timing code changed.
- Exact remote CI remains: Core Go 1.27 PASS, Go 1.26 Race PENDING; Companion replacement/media and Swift CompanionCore PASS; Camera Relay isolated/ARM64 PASS. These checks do not cover the local six-commit stack. Verify current PR/remote and active workflows before the normal push, then wait for all exact-new-SHA checks before another push.
- Continue with full-form Cue editor recovery/restart acceptance and the next proven Project-configuration history boundary. Preserve separation of Engine, ACL, HTTP and external-output timing. No production services/data/Vault/show, real stage devices, firmware, backup deletion, paid action, force push, merge or production SC-024 activation.

## Resume update — 2026-11-15 · full Mixed-Cue editor Undo/Redo

- Current remote/PR #534 SHA remains `0c8b965d2d2d0f3ac9f424cbac4bcabbf3088467`; branch `integration/stagecore-rc` is OPEN/Draft. Local HEAD is `e7ffe5feb3430c8936ab01db11d17f97d504a605`, eight commits ahead, with a clean tree. Core run `43668251167` Go 1.26 Race is still pending on the exact remote SHA. Do not push before it completes; then verify the remote/PR and workflows again and push by normal fast-forward only.
- `TestBrowserCreatesNestedMixedCueChainInDraft` now exercises the actual Cue Studio dialog-local Undo/Redo on a three-level linked Mixed Cue: a temporary edit changes the parent name, one Action execution mode and its linked child; Undo restores all three baseline fields, Redo reapplies them, and a final Undo returns to the saved baseline before the intended metadata edit is saved. This proves the dialog history snapshot includes linked-chain selection and Action execution mode without altering persisted Cue state until Save. Matrix SC-009 records the tested fields and keeps broader payload/recovery boundaries open.
- Focused real Chrome test passed three normal runs and one Race run. `go vet ./internal/httpapi ./internal/operatorweb` passed; Core Operator Node suite passed 82/82; fixture syntax, `gofmt` and `git diff --check` passed. All run against temporary SQLite, simulated aliases and no-op browser helpers; no GO/STOP/Blackout/publish or device output is invoked.
- The earlier page-reload proof for server-persisted Draft history remains in commit `af3b8ed`; together, these checks distinguish dialog-local form Undo/Redo from durable per-editor Draft history. No runtime or performance code changed.
- Next: check the pending exact-head Core run, then continue the remaining SC-009 payload/recovery acceptance or another independently reproducible gap. Keep Engine/ACL/HTTP/output latency separate, do not repeat existing Mac/Pi performance evidence, and preserve all production and hardware safety boundaries.

## Resume update — 2026-11-15 · offline recovery of a nested Mixed Cue

- Repository remains `ali96adil/StageCore`, branch `integration/stagecore-rc`, Draft PR #534 OPEN. The last pushed Remote/PR SHA is `0c8b965d2d2d0f3ac9f424cbac4bcabbf3088467`; local HEAD is `89cb7a200dba991624a34154fc69f5869c2544f8`, ten commits ahead, with no uncommitted files. Core run `43668251167` has now completed **successfully** on the exact pushed base SHA, including Go 1.26 Race and ARM64 builds; Go 1.27 also passed. Companion replacement/media and Swift CompanionCore run `43668251174` and Camera Relay isolated/ARM64 run `43668251160` passed. These checks predate the local stack; verify the same PR/remote and no active exact-head workflow, then push normally and wait for every check on the new exact SHA.
- Real Chrome acceptance now creates a local recovery draft for the linked Mixed parent after changing its name, one Action's execution mode and removing its child Cue link. It writes via browser storage/pagehide, reloads, blocks only the `cue-editor-recovery` API in the isolated test wrapper, and manually restores the candidate. The test verifies those three values return, the recovery endpoint was offline, and no confirmation or actuation path was used. Draft remains unsaved; the database still contains the saved parent and exact Actions/links.
- Local checks: `TestBrowserCreatesNestedMixedCueChainInDraft` passed once and then twice consecutively in normal mode, plus one Race run; `go vet ./internal/httpapi ./internal/operatorweb` passed; Operator Node suite passed 82/82; JavaScript syntax, `gofmt`, and `git diff --check` passed. The first two test attempts exposed fixture issues (close-discard semantics and expected link state); both were corrected, and the final test passes repeatedly. No product implementation, Cue Engine/runtime, ACL or performance timing code changed.
- The latest local batch adds ten preserved commits above `0c8b965`, with no force push, new branch/PR or merge. Push this stack only after one final `gh pr view/checks` confirms the completed base run and unchanged remote SHA, then wait for Core, Companion and Camera Relay results on the new head before another push.
- Next software gaps: broader Action payload recovery through reload, stale-baseline/local candidate review, and authoring history for additional Project configuration entities. Keep the previous simulation-only performance evidence and separate Engine, ACL, HTTP and external-output time. Pi lab qualification remains hardware/lab dependent.
- No production service/data/Vault/show, real device/output, firmware, backup deletion, paid action, merge, force push or production SC-024 activation occurred.

## Resume update — 2026-11-15 · Mixed-Cue recovery keeps advanced Action payloads

- Draft PR #534 is OPEN on `integration/stagecore-rc`. Last pushed Remote/PR HEAD is `ed14084e73fecd7de137b8bfae838126448f74b2`; local HEAD `a490fb79eac426e7792e7ee777057fb7bcc87aec` is one commit ahead. The current Core/Companion/Camera Relay CI was triggered on `ed14084` (Core run `43670843903`, Companion `43670843853`, Camera `43670843835`) and is still active at the last check. Do not push this local follow-up until all exact-head checks finish; then verify same PR/remote and fast-forward normally.
- `TestBrowserCreatesNestedMixedCueChainInDraft` extends offline browser recovery with a valid advanced `visual.scene.activate` payload containing an opaque nested field. After simulated network loss and page reload, manual restore returns the Cue name, Action execution mode, payload, stable Action ID and removed linked-Cue selection. A DB assertion confirms the authored payload remains `opening-sim`, so local recovery is not persisted until an explicit Save. No actual output, Cue execution, Session or Snapshot is created.
- Verification after this extension: focused Chrome test passed once normally, then two consecutive normal runs, and one Race run; `go vet ./internal/httpapi ./internal/operatorweb`, 82/82 Operator Node tests, `node --check`, `gofmt` and `git diff --check` passed. It uses the temporary browser profile and SQLite. Runtime/ACL/performance code is unchanged.
- Matrix SC-009 now records offline restoration of the opaque Action payload and identity while leaving broader stale-baseline/review and process-restart form recovery open. The new local commit is only `a490fb7`; pushed history and active CI are untouched.
- Next: wait for run `43670843903` plus Companion and Camera Relay checks at `ed14084`; then push this one normal fast-forward commit and wait for checks at its exact SHA. Continue from stale-baseline recovery and additional Project-configuration history gaps; retain Engine/ACL/HTTP/output timing separation and prior performance evidence.
- No production service/data/Vault/show, real stage device/output, firmware, paid action, backup deletion, merge, force push or production ACL enablement occurred.

## Resume update — 2026-11-15 · stale Mixed-Cue recovery stays review-only

- Draft PR #534 remains OPEN at remote `ed14084e73fecd7de137b8bfae838126448f74b2` on `integration/stagecore-rc`. Local HEAD is `272fcac672c955b94c3e042e3ca62f427969ff95`, three commits ahead and clean. The exact `ed14084` Companion and Camera Relay checks pass, Core Go 1.27 passes, and Core Go 1.26 Race remains pending (`43670843903`). Do not push until every exact-head check completes; then verify PR/remote SHA and push normally.
- The real Chrome nested Mixed-Cue test now exercises both recovery cases. It first restores a pagehide/reload local draft offline, including an opaque Action payload and stable ID. A test-side concurrent Draft edit then changes the saved Cue before a second reload. The browser labels the older recovery as stale, exposes its JSON in the read-only review control, and leaves the newer saved name, Action mode, payload and child link in the editor. Go confirms no Session or Published Snapshot was created and the persisted Action payload is unchanged.
- Verification: the focused Chrome flow passed four normal runs and two Race runs across the final stale-review/snapshot iterations; `go vet ./internal/httpapi ./internal/operatorweb`, Operator Node 82/82, JavaScript syntax, `gofmt` and `git diff --check` pass. All state is temporary SQLite and a disposable Chrome profile; offline simulation blocks only this test's Cue recovery endpoint. No product runtime/performance code changed.
- SC-009 now records exact stale-baseline review evidence. Broader candidate selection/Project isolation, all Action fields, and Cue form recovery across process restart remain open. Next: await `43670843903` Go 1.26 Race at `ed14084`; push the three local commits once the exact CI set is complete, then continue with those remaining recovery boundaries or another proven Project-configuration history gap.
- Keep all recorded Engine/ACL/HTTP/external-output timing distinctions and Pi qualification limits. No production services/data/Vault/show, real stage output/device, firmware, backup deletion, paid action, force push, merge or production SC-024 activation occurred.

## Resume update — 2026-11-15 · offline Mixed-Cue recovery after browser process restart

- Draft PR #534 remains OPEN on `integration/stagecore-rc`. Remote/PR HEAD is still `ed14084e73fecd7de137b8bfae838126448f74b2`; local code HEAD is `efda978cf2ee4d63bdf4f1ddbff87e9afaf18c30` (five commits ahead before this checkpoint commit). Core run `43670843903` is still pending Go 1.26 Race at the exact pushed SHA; Go 1.27, Companion replacement/media, Swift CompanionCore, and Camera Relay isolated/ARM64 all pass. Preserve the full local stack and do not push until that Core run completes successfully; then verify PR #534 and remote SHA before a normal fast-forward.
- `TestBrowserCreatesNestedMixedCueChainInDraft` now saves its offline Mixed-Cue recovery draft, gracefully closes the isolated Chrome process, relaunches Chrome with the same temporary profile, confirms that `sessionStorage` belongs to a fresh browser session, and restores the local draft. It verifies the opaque nested Action payload, stable Action ID, execution mode and removed child link, while persisted SQLite remains unchanged; the test then continues the stale-baseline read-only review. Recovery endpoint failures are simulated in the browser wrapper only. This closes the process-restart form-recovery acceptance previously listed as open in SC-009; broader stale-baseline variants remain.
- Verification for `efda978`: the focused real-Chrome test passed normally (5.74 s) and under Race (13.28 s). The complete `go test ./internal/httpapi -count=1` passed (76.9 s); `go vet ./internal/httpapi ./internal/operatorweb` passed; Operator Node suite passed 82/82; JavaScript syntax and `git diff --check` passed. The suite used temporary SQLite, a disposable Chrome profile and simulated actions only. No application runtime code, Cue Engine, ACL, or performance path changed, so saved Engine/Pi performance measurements were not repeated.
- The matrix update records actual process-restart evidence. The five pre-existing local commits remain untouched. The only new implementation commit is `efda978`; matrix and this checkpoint still need a documentation commit. No new branches/PRs were created and no push has occurred.
- Continue with another reproducible SC-009 Project-authoring history gap or the SC-024 endpoint/async audit after preserving the stable Core CI gate. Keep runtime Engine, ACL, HTTP and external-output latency separate. Pi test execution remains restricted to a positively identified isolated environment; do not contact production services or hardware.
- Safety limits remain: no production Hub/Camera Relay/service, production data/Vault/show, real stage output/device command, firmware, paid action, backup deletion, force push, merge, or production SC-024 activation.

## Resume update — 2026-11-15 · linked component PRs and exact CI refreshed

- Main repo `ali96adil/StageCore`, branch `integration/stagecore-rc`, Draft PR #534 is OPEN at remote `ed14084e73fecd7de137b8bfae838126448f74b2`. Local code HEAD before this matrix/checkpoint update is `b555a4426f705d336192fe9653582d5a36669499`, six commits ahead and clean before the current documentation edit. Core run `43670843903` remains pending Go 1.26 Race; Go 1.27 passes. Companion Core run `43670843853` and Camera Relay run `43670843835` pass. Do not push until the exact `ed14084` Core Race completes, then recheck remote/PR and workflow status.
- Refreshed related PRs read-only: TabletPlayer #36 `91613da1c8e0455a501c7be4b6df8ac1ce73a6bc`, Android run `42719885664` PASS; ESP32 DMX #39 `3ad774988b2f78eb385f1046f7668a7f29049ab1`, Firmware CI run `42759555665`, all six checks PASS; ESP32 Camera #20 `455b066c6520e1c8a576d26dd2afd4b377652dde`, `hardware-probe` run `42761096513` PASS; StageLaser #56 `b14a5789132549f2724f77b342bbe4c6b0fc880c`, host/build run `42765890415` PASS; StageLaser #57 `c4ff10b37dc2fbfd3ebe31e55f0f8d4869d3dc54`, host/build run `38054411190` PASS. All remain open; #36, #39, #20 and #56 are Draft, #57 is not Draft. Their checks are source/build probes and do not qualify physical outputs. No linked component checkout was available, so these repositories were not modified.
- Updated `docs/architecture/related-component-qualification-matrix.md` with these exact current heads and check states. The only other local `.git` under the lab folder is a clean detached StageCore clone at `0ad3e00`; it is not a related component checkout and was left unchanged. No isolated Pi host was positively identified for a new run.
- No hardware, production service/data/Vault/show, paid action, PR merge, force push, or production ACL activation was used. Next, wait for exact-head Core Race, publish the six preserved main-repo commits plus documentation through a normal fast-forward only after it is green, then continue the SC-024 endpoint/async review or another demonstrated software gap.

## Resume update — 2026-11-15 · fail-closed Session confirmation before Cue GO

- Repository `ali96adil/StageCore`, branch `integration/stagecore-rc`, Draft PR #534 remains OPEN. Remote/PR HEAD is `ed14084e73fecd7de137b8bfae838126448f74b2`; the preserved local stack plus the new Cue Engine fix is at `96895ecdd2fb5e5b1d6f5fab1fcd6d1b67d09016`, eight commits ahead before this documentation update. Core run `43670843903` completed successfully at the exact remote SHA, including Go 1.26 tests, Browser Cue Delivery, Vet, Race and Linux ARM64 CGo-free builds; Go 1.27 passed. Companion Core `43670843853` and Camera Relay `43670843835` also passed. No active branch workflow remained at the last check.
- A red-first regression proved that a selected-Cue REHEARSAL marked `MANUAL_CONFIRMATION_REQUIRED` could previously reach the Cue Engine executor. Commit `96895ec` now reads the durable Session Foundation state and rejects Cue GO while either the manual-confirmation boolean or restoration-status marker remains set. The regression proves stable duplicate rejection, zero executor dispatch, zero runtime Events and zero Cue Executions; it also clears the redundant boolean directly in the temporary test database and proves the restoration marker independently remains fail-closed. The Engine guard does not claim to implement the missing selected-Cue preparation or deliberate rehearsal confirmation/resume flow.
- Local checks after the change: `go test ./internal/cueengine ./internal/runtimecontrol ./internal/simulationcontrol -count=1` passed; the same three packages passed Race, and the final Cue Engine package (including the inconsistent-marker case) passed normal and Race. `go vet ./internal/cueengine ./internal/runtimecontrol ./internal/simulationcontrol`, `gofmt`, and `git diff --check` passed.
- Because the guard adds durable state reading to the Cue Engine path, the representative 100-Cue/200-Action sustained Rehearsal benchmark was rerun locally against the Digital Twin: 20 sessions, 2,000 GO samples, P50/P95/P99 `2.929/4.050/4.462 ms`, P95 jitter `1.122 ms`, 14.80 GC per 100-GO session, 245 µs GC pause per session, and `39,208,563 B / 298,156 allocations` per 100-GO session. The standalone test process used 5.37 s user CPU, 3.35 s system CPU and 220,299,264 bytes (about 210 MiB) peak RSS including migrations/setup. This is an unpaired M3-class local run, not Hub service memory or Pi qualification; the small allocation difference from the earlier run is not treated as an improvement or regression. No physical executor was reached.
- The F-027 feature contract and software matrix now state the Engine gate and keep selected-Cue preparation/reconstruction and deliberate rehearsal confirmation open. The performance report records the follow-up run. No Pi lab host was positively identified, so this change has no new Pi timing claim. SC-024 remains opt-in/off in Production and its whole-server policy/async audit remains open.
- Next: verify PR #534 and branch HEAD again, push the preserved commits by normal fast-forward only (no force push), then wait for Core, Companion and Camera Relay CI at the exact new SHA. Continue the SC-024 endpoint-to-policy/asynchronous-delivery audit and independently executable software gaps; do not enable production ACL.
- No production service/data/Vault/show, real stage device/output, firmware, paid action, backup deletion, PR merge, force push or production ACL activation occurred.

## Resume update — 2026-11-15 · batch pushed; exact-head CI running

- The preserved nine-commit stack was pushed normally to the existing `integration/stagecore-rc` branch and Draft PR #534. Local HEAD, `origin/integration/stagecore-rc`, and PR HEAD are `767dde96782f68472fb39faec236514780475f83`; the working tree was clean immediately after the push. No new branch or PR was created.
- Exact-head workflows are running: Core CI `43673568936`, Companion Core CI `43673568922`, and Camera Relay CI `43673568934`, all at `767dde9`. Do not push while any of these runs is active; wait for completion and inspect every result before advancing the remote branch.
- The pushed batch contains browser-process restart Draft recovery, linked component qualification records, the Cue Engine manual-confirmation gate, its fail-closed regression, the sustained Rehearsal performance follow-up, and corresponding matrix/F-027/checkpoint updates. Previous exact-head Core, Companion, and Camera Relay runs at `ed14084` all succeeded before this push.
- The only current software change in the latest code batch is the Cue Engine session-state gate; its normal, Race and Vet checks passed locally, and a focused 100-Cue/200-Action Digital Twin Rehearsal benchmark recorded P50/P95/P99, jitter, GC, allocations, process CPU and peak RSS. This M3 Max measurement is not Pi ARM64 qualification. No isolated Pi host was available; no real output or production service/data was touched.
- Continue the whole-server SC-024 endpoint-to-policy and asynchronous delivery review while exact-head CI runs. Keep Production ACL opt-in/off. If another safe software gap is found, reproduce it locally and test it, but do not publish a new head until these three exact-head workflows finish.

## Review note — 2026-11-15 · project-list response fence confirmed

- Rechecked the staged SC-024 Project-list delivery path while CI runs. `GET /api/v1/projects` filters by the live Project grant, revalidates the browser session and fetches current accessible Project IDs again immediately before response serialization; revoked Projects are removed from the already-built list. `TestOperatorProjectListRechecksGrantsAtDelivery` exercises revocation at that boundary, and the route-inventory guard already covers registration drift. No duplicate code change was warranted.
- Also reviewed both opt-in Camera Relay streaming paths without contacting a configured/production Relay. The browser route requires the fixed Project's `project.view` + `media.read`, loopback upstream, no-store/no-sniff headers, checks idle viewers once per second, and rechecks before every delivered frame. The tablet gateway resolves the current Companion session, Project assignment, epoch, Snapshot, runtime generation and live-video capability before each frame. Existing regressions `TestProjectCameraViewerRevokesIdleStream`, `TestProjectCameraViewerRechecksBeforeFirstFrame`, `TestProjectCameraViewerDoesNotDeliverFrameAfterAccessRevocation`, and `TestTabletCameraViewerChecksProjectAndFencesLiveStream` cover revocation/fencing; no additional fix was indicated.
- Latest read-only related-PR check is unchanged: TabletPlayer #36, ESP32 DMX #39, ESP32 Camera #20, and StageLaser #56/#57 remain open at the previously recorded heads; all displayed checks are successful. Their checks are build/source probes, not physical-output acceptance.
- On exact pushed SHA `767dde96782f68472fb39faec236514780475f83`, Companion Core `43673568922` and Camera Relay `43673568934` are green. Core `43673568936` has Go 1.27, Go 1.26 tests, Browser Cue Delivery and Vet green; Go 1.26 Race is in progress and ARM64 product builds are pending. No remote update is authorized until those remaining Core steps complete.
- The only code ahead of `767dde9` is none; local commits beyond it are Handoff-only (`acf25ff` and this review note). Commit this note locally for recovery, but do not push while Core CI is active. First continue with another asynchronous device/Companion delivery path that lacks a final-authority test; if the audit finds no reproducible gap, move to the explicit SC-009 target/input/output history gap. Preserve the current exact-head CI and Production ACL-off constraint.

## Resume update — 2026-11-15 · persistent Input Draft Undo/Redo

- Main repository: `ali96adil/StageCore`, branch `integration/stagecore-rc`, Draft PR #534 OPEN. Remote/PR HEAD remains `767dde96782f68472fb39faec236514780475f83`; the latest exact-head Core run `43673568936`, Companion run `43673568922`, and Camera Relay run `43673568934` are all green. Local HEAD is `278191f` before this Handoff commit; the new implementation is three commits ahead of the pre-existing Handoff-only stack. Push normally to this same branch/PR; no new branch or PR.
- Added Migration 66 and a per-editor, per-Draft Input history with 50 retained events. Input create/edit/delete now record within the same SQLite transaction as the mutation. Undo/Redo restores stable Input IDs, rejects out-of-band changes, refuses to remove Inputs still referenced by Routes, and obeys the active SHOW configuration lock. The Project-edit GET/POST API uses `no-store`, live authorization rechecks and authenticated editor scope; the Configuration page now has localized Input Undo/Redo controls. Project ACL production wiring remains unchanged/off.
- Added Store coverage for create/edit/delete, stable identity, editor isolation, branching behavior through Undo/Redo, bounded retention, out-of-band conflict, Route-reference protection and active SHOW lock. HTTP coverage verifies authorization scope, no Snapshot publication, and stable identity through API Undo/Redo. State Backup/Restore now proves the Input history remains available and usable after restoration, including reference protection. SC-009 and SC-024 matrices and the reviewed HTTP registration inventory were updated; route review now has 293 registrations, 18 dynamic patterns, digest `ad88f5218ca8540ae65061b67db24f276274471b6573911073bbcff6542eab19`.
- Verification passed: full `go test` for `internal/store`, `internal/httpapi`, `internal/db`, `internal/operatorweb`, and `internal/backup`; focused Input history tests normally and under Race; focused Input-history HTTP test normally and under Race; `go vet` for all five packages; Operator Node suite 82/82; JavaScript syntax checks; and `git diff --check`. The full Store/HTTP tests initially caught stale schema-version expectations and the ACL registration inventory, which were updated and passed on rerun.
- No Cue Engine, ACL hot path, runtime execution, performance code, production service/data/Vault, real output, device or firmware was touched. No Pi performance run was repeated because this batch does not change timing code.
- Next: push these three commits plus this Handoff only after confirming the remote/PR SHA and the old exact-head CI remains fully green; then wait for CI at the new SHA. Continue SC-009 with Output and Target history and cross-entity undo semantics. Keep `Software Complete`, `Hardware Qualification Pending` and `Production Release Approval Pending` separate; no force push, merge or production ACL activation.

## Push update — 2026-11-15 · Input history batch published to Draft PR

- Normal fast-forward push completed to existing `integration/stagecore-rc` / Draft PR #534. Local, remote and PR HEAD are `d7c39c97d769e23e1f764ef54b80832da0086f4e`; the earlier seven-commit local stack was pushed without force. Worktree was clean immediately after push.
- New exact-head checks are active at `d7c39c9`: Core CI `43676259657`, Companion Core CI `43676259617`, Camera Relay CI `43676259598`. All five displayed PR checks are pending at checkpoint time. Do not push another head until these runs complete and every result is inspected.
- The previous exact-head checks at `767dde96782f68472fb39faec236514780475f83` are all green: Core `43673568936`, Companion `43673568922`, Camera Relay `43673568934`.
- Continue independent SC-009 Output/Target history work locally while CI runs; preserve any new local commits and only push them after the `d7c39c9` check set is complete. No runtime timing or real output work is authorized by this history feature. Production services/data/Vault, real stage devices and production ACL remain untouched/off.

## Resume update — 2026-11-15 · persistent Output Draft Undo/Redo

- Remote/PR #534 remains at `d7c39c97d769e23e1f764ef54b80832da0086f4e`. Core CI `43676259657` is still in Go 1.26; Go 1.27 passes. Companion CI `43676259617` and Camera Relay CI `43676259598` pass. Do not push while the exact `d7c39c9` Core job is running.
- Local Output batch HEAD is `e9cc83a` before this checkpoint commit. Commits `37a71b9`, `9edce67`, and `e9cc83a` add Migration 67, per-editor/per-Draft Output history, atomic create/edit/delete, safe confirmed Output deletion, Project-edit GET/POST history API, localized Configuration Undo/Redo controls, and route-action reference protection. Undo/Redo restores stable Output identity, rejects out-of-band changes, and retains at most 50 steps. Production ACL and output dispatch paths are unchanged.
- Tests passed: full Store, HTTP API, Backup, DB, and Operator Web package runs; focused Output history Store and HTTP tests; Store and HTTP focused Race tests; `go vet` on all five packages; Node Operator suite 82/82; JS syntax checks; and `git diff --check`. Backup/Restore test now verifies Cue, Route, Input and Output histories remain usable with stable IDs. HTTP route inventory is 296 registrations / 18 dynamic patterns, digest `fd6155536b0bbe47d41f62668fc7261f6a1f368ea620fa608a2e9d3dadc455b0`.
- No Cue Engine, timing code, device output, production service/data/Vault, hardware, firmware or Production ACL changed. No Pi benchmark was repeated because this batch only changes Draft storage and operator authoring.
- Next: inspect the current `d7c39c9` Core result and exact PR checks; if all finish green, push the preserved commits normally. Continue SC-009 Target history after reviewing ProjectConfig secret-retention and every alias writer. Keep production ACL off and do not merge or force-push.

## Resume update — 2026-11-15 · Configuration history keyboard scope

- Main repository `ali96adil/StageCore`, branch `integration/stagecore-rc`, Draft PR #534 remains OPEN. Remote/PR HEAD is `d7c39c97d769e23e1f764ef54b80832da0086f4e`; local feature commit is `f58b6322b7cbf1ffe5dd8ff5c8d5ab8fdffefaa2`, six commits ahead before this checkpoint. Exact-head Core `43676259657`, Companion `43676259617`, and Camera Relay `43676259598` are all completed successfully. The local Output history and keyboard-scope commits are not part of that CI. Publish only by normal fast-forward after this checkpoint is committed.
- A real-Chrome regression reproduced the SC-009 shortcut bug: with the old listener hard-coded to Routes, Meta+Z focused on Input did not remove the Input; the test timed out waiting for the Input row to disappear. The Configuration listener now selects the independent Route, Input or Output history from the active card, advertises Undo/Redo shortcuts on all three controls, preserves native text-field undo, and declines history hotkeys from the Targets and other non-history cards.
- The real-Chrome `TestBrowserRouteDraftHistoryKeyboardUndoAndTouchRedo` now exercises Route Undo/Redo and touch Redo plus Input and Output Meta+Z/Meta+Shift+Z, verifies each stack independently restores the original Input/Output identity, checks the accessible shortcut labels, keeps the 390×844 RTL page within the viewport, and observes no non-history mutations or Published Snapshot. It passed three consecutive normal runs and one Race run. `go vet ./internal/httpapi ./internal/operatorweb`, Node Operator suite 82/82, JavaScript syntax checks, `gofmt`, and `git diff --check` pass. Full `go test ./... -count=1` passed on the immediately preceding Output batch; this follow-up changes Operator JS and browser-test coverage only.
- The software matrix now records the keyboard coverage and the remaining Target-history gap. Project target config can contain inline credential-like values (the existing export test uses an `api_key` field); copying these configurations into history would extend their retention. Do not add Target history until the retained snapshot is protected by an appropriate secret-safe design. Cross-entity Cue/Route/Input/Output stacks remain separate.
- No Cue Engine, runtime, ACL hot path or output dispatch changed, so no timing benchmark was repeated. The existing isolated Raspberry Pi 4 ARM64 Digital Twin measurements still cover Mixed Cues, linked compound chains and the 100-Cue/200-Action workloads at the unchanged timing code. The isolated qualification SSH endpoint previously failed before host access; do not contact the `stagecore-pi` production-side alias. Production Hub/Camera Relay services and data/Vault, original show, physical outputs/devices, firmware and Production ACL remain untouched/off.
- First next step: confirm local/remote/PR heads, push the preserved Output history and shortcut-scope commits plus this Handoff normally to the existing PR, then wait for Core/Companion/Camera Relay CI on that exact new SHA. Continue with a safe, reproducible SC-009 gap; preserve secret-retention protections and the Production ACL-off boundary.

## Push update — 2026-11-15 · Output and scoped history shortcuts published

- A normal fast-forward push updated the existing `integration/stagecore-rc` branch and OPEN/Draft PR #534 from `d7c39c97d769e23e1f764ef54b80832da0086f4e` to `546ccf11aed5e54522bbe4d8e305d5106b5751c2`. Local, upstream and PR HEAD matched at verification; the worktree was clean before this checkpoint commit. No branch or PR was created.
- The push includes Output Draft history, its API and operator controls, the Route/Input/Output keyboard-scope fix and the SC-009 matrix update. The exact prior-head Core `43676259657`, Companion `43676259617`, and Camera Relay `43676259598` checks completed successfully before the push.
- New exact-head runs are active at `546ccf1`: Core CI `43678333511`, Companion Core CI `43678333487`, and Camera Relay CI `43678333533`. Do not push another PR head while these workflows run. The Handoff checkpoint itself will be a local-only follow-up until all three runs complete.
- The focused real-Chrome regression passed three normal runs and one Race run; `go vet`, Operator Node 82/82, syntax checks, and diff checks passed. Full `go test ./... -count=1` passed on the preceding Output code batch. No Cue Engine timing code changed.
- Next: record and inspect all exact-head workflow results before another push. Then continue from SC-009 Target history with a design that does not copy inline credential-like config into durable history; if that cannot be made secret-safe without breaking old Project compatibility, document the specific constraint and move to another executable matrix gap. Production services/data/Vault, physical outputs/devices, firmware and Production ACL remain untouched/off; no merge or force push.

## Resume update — 2026-11-15 · Input/Output branch recovery and qualification refresh

- Remote/PR #534 remains at `546ccf11aed5e54522bbe4d8e305d5106b5751c2` on `integration/stagecore-rc`, OPEN/Draft. Core CI `43678333511`: Go 1.27 PASS; Go 1.26 job still in progress. Companion `43678333487` and Camera Relay `43678333533` completed successfully. Local HEAD `ff2588d942215ee81b13d9736d14fd18b5497145` is three commits ahead of remote (`c56dd6c` Handoff, `942ee73` Store history tests, `ff2588d` qualification doc refresh). Do not push until all three exact-head workflows on `546ccf1` complete.
- Added Input and Output Store regressions for branching after Undo. They verify a new edit discards the stale Redo, earlier Create remains undoable, and Redo restores both original stable IDs. `go test ./internal/store -count=1`, focused branch tests, focused branch Race tests, `go vet ./internal/store`, and `git diff --check` pass. No production Store behavior changed in this batch.
- Rechecked linked component PRs read-only: TabletPlayer #36 `91613da1c8e0455a501c7be4b6df8ac1ce73a6bc` and run `42719885664` pass; ESP32 DMX #39 `3ad774988b2f78eb385f1046f7668a7f29049ab1` and run `42759555665` pass; ESP32 Camera #20 `455b066c6520e1c8a576d26dd2afd4b377652dde` and run `42761096513` pass; StageLaser #56 `b14a5789132549f2724f77b342bbe4c6b0fc880c` / run `42765890415` and #57 `c4ff10b37dc2fbfd3ebe31e55f0f8d4869d3dc54` / run `38054411190` pass. All remain OPEN; #36/#39/#20/#56 are Draft and #57 is not. Their checks remain firmware/source probes, not physical qualification. No linked repository checkout is available, so none was modified.
- Refreshed the related-component matrix to exact current heads/checks and corrected the SC-024 qualification text: the Owner Project Access editor and View/Edit/Operate/Manage presets exist, but are gated `PREPARED_NOT_ENFORCED`; the production ACL remains off. SC-024 endpoint/async and deployment qualification remain open.
- No Cue Engine/runtime/ACL hot-path/output code changed, so no performance benchmark was repeated. Existing Pi 4 ARM64 Mixed/linked 100-Cue/200-Action Digital Twin evidence remains for unchanged timing code; isolated host resource controllers and formal timing SLO are still unqualified. No Pi access retry, real output/device, production service/data/Vault, firmware, or Production ACL activation occurred.
- Next: inspect the final `546ccf1` Core Race/ARM64 and all associated workflows. Once green, push the three preserved local commits normally and follow exact-head CI. Then work the next safe qualification gap; Target Undo still requires a secret-safe retention design, so prioritize a separate reproducible SC-014/SC-018 or StageAudio software gap if that design is not safe without breaking legacy Project compatibility.

## Resume update — 2026-11-15 · Cue Inspector keyboard, touch, and Snapshot safety

- Main repo `ali96adil/StageCore`, branch `integration/stagecore-rc`, OPEN/Draft PR #534. Remote/PR HEAD remains `546ccf11aed5e54522bbe4d8e305d5106b5751c2`; the functional tip `e74a2622cb7bb2f3d6a62a03c4e9ca199ca94029` was five commits ahead before this checkpoint, and the handoff checkpoint adds one more local commit (six total ahead). Core run `43678333511` is still active on the exact remote head in Go 1.26 Race, with ARM64 build pending; Go 1.27 is green. Companion `43678333487` and Camera Relay `43678333533` are green. Do not push while this exact-head Core run is active.
- SC-014 now gives the focused Cue Inspector `J`/`K` Previous/Next shortcuts, displayed and exposed through `aria-keyshortcuts`. They act only inside an open Inspector, respect disabled boundaries, ignore modifier/composition events, and do not consume text entered in editable fields. The browser gate exercises native emulated touch to open the Inspector, native J/K to move both directions, and native Enter to activate Next.
- Chrome test now publishes a baseline Snapshot only in its isolated temporary SQLite fixture, forks an editable successor Draft, and compares Snapshot ID, source revision, version, hash, status and exact manifest after browser authoring/simulation. It confirms no runtime Session or output dispatch. The existing SC-017 active-SHOW browser gate pins the workspace to Runtime, so Cue Inspector is not exposed during SHOW.
- Verification: real-Chrome `TestBrowserCreatesNestedMixedCueChainInDraft` passed normally (6.50 s) and under Race (14.38 s); Operator Node suite 82/82; `go test ./internal/operatorweb -count=1` and its Race run passed; `go vet ./internal/httpapi ./internal/operatorweb`, JS syntax, `gofmt` and `git diff --check` passed. One earlier Race attempt timed out during the first J navigation; the harness now uses monotonically increasing CDP key command IDs, and the subsequent Race browser run passed. No timing/runtime/ACL code changed, so no performance measurements were repeated.
- Matrix SC-014 now records the new keyboard, touch, temporary Snapshot immutability and active-SHOW Runtime-pin evidence. Broader narrow-width and physical Android qualification remain separate. No Pi access retry or real stage output/device command occurred.
- Next: wait for all exact `546ccf1` workflows, especially Go 1.26 Race and ARM64. When complete, recheck PR/remote HEAD and push the six local commits normally to this same branch/PR; then inspect all checks at the new exact HEAD before another push. Continue the next reproducible software gap while preserving Target-history credential-retention safety.
- Production Hub/Camera Relay/services/data/Vault/show, real stage devices/outputs, firmware, backups, paid actions, force push, merge, and production SC-024 activation remain untouched/prohibited.

## Resume update — 2026-11-15 · Cue list responsive geometry at four widths

- Remote/PR #534 remains at `546ccf11aed5e54522bbe4d8e305d5106b5751c2`. Core `43678333511` has now completed successfully at that exact SHA: Go 1.26 (including Race and Linux ARM64 CGo-free builds) and Go 1.27 pass. Companion Core `43678333487` and Camera Relay `43678333533` also pass. The SC-014 batch plus the next SC-018 geometry/fix batch are local; remote/PR HEAD has not changed. The implementation tip is `d62fe25` (seven commits ahead); this Handoff checkpoint adds one more local commit. Push the full eight-commit fast-forward to the same branch/PR only after the final remote SHA check.
- SC-018 real-Chrome measurements exposed a reproducible 1024px document overflow: the five-column Cue filters forced the document to 1087px. `app.css` now changes that filter grid to two columns through 1080px. `TestBrowserCreatesNestedMixedCueChainInDraft` measures the open Cue Inspector in RTL and LTR at 390×844, 768×1024, 1024×768 and 1440×900; all eight cases have no document-level horizontal overflow and keep the navigation visible. The Cue table remains internally horizontally scrollable at 390/768px (measured 720px RTL and 914px LTR), which stays an open narrow-screen design issue.
- The same browser gate exercises emulated touch to open Inspector, physical CDP J/K and Enter navigation, text-field shortcut exclusion, a 123-Cue/three-level Mixed chain, offline simulations with no runtime request, and exact Published Snapshot immutability after editing only a successor Draft. The SC-017 active-SHOW browser gate continues to pin the workspace to Runtime.
- Verification after the responsive fix: real Chrome focused test passed normally (6.41 s) and under Race (15.05 s); Operator Node suite 82/82; `go vet ./internal/httpapi ./internal/operatorweb`; `node --check`; `gofmt`; and `git diff --check`. An initial measurement failed before the CSS fix with page width 1087px at 1024; after the two-column rule, all RTL/LTR viewport cases passed. No engine, timing, ACL or output path changed, so performance measurements were not repeated.
- `software-completion-matrix.md` records the measured responsive evidence and the remaining full-route screenshot/200%-zoom audit. This does not satisfy all of SC-018; internal Cue-table scrolling, other Operator routes, annotated screenshots and physical tablet fit remain open.
- Next: verify `git ls-remote`, PR #534 head, and exact run statuses still identify the same `546ccf1` head and no active run; then push the eight local commits normally to `integration/stagecore-rc`. Wait for and inspect Core, Companion and Camera Relay CI at that exact pushed SHA before another push. Continue the next safe software qualification gap after that gate.
- No production services/data/Vault/show, real device/output, firmware, paid action, backup deletion, merge, force push, or production SC-024 activation occurred.

## Push update — 2026-11-15 · Inspector and responsive Cue list qualification

- Normal push completed to the existing `integration/stagecore-rc` branch and OPEN/Draft PR #534, advancing `546ccf11aed5e54522bbe4d8e305d5106b5751c2` to `5d654d7c31a377b67c64f6ab65b7c713fa95229e`. Local, `origin`, `git ls-remote` and PR head matched at `5d654d7`; the pushed group contains eight preserved commits. No branch/PR was created, and no force push or merge occurred.
- Exact previous-head checks are fully green on `546ccf1`: Core run `43678333511` (Go 1.26 Race/ARM64 and Go 1.27), Companion `43678333487`, and Camera Relay `43678333533`.
- New exact-head workflows are pending at `5d654d7`: Core `43680858030`, Companion `43680858045`, Camera Relay `43680858075`. Do not push another head until every check completes and its result is inspected.
- Pushed changes add focused Cue Inspector J/K and Enter keyboard navigation, emulated touch activation, exact Published Snapshot immutability across successor-Draft edits, and a real-Chrome RTL/LTR geometry gate at 390×844, 768×1024, 1024×768 and 1440×900. The gate discovered and fixes 1024px Cue filter overflow. Internal table scrolling remains at narrow widths; SC-018 full route screenshots/zoom audit is still open.
- Local verification: real Chrome focused gate passes normally and under Race; Node Operator suite 82/82; Go Vet, syntax, gofmt and diff checks pass. Timing engine, ACL and output execution code were unchanged; no performance run was repeated.
- This push checkpoint is a local-only Handoff update after the push, ahead of the newly running CI. Next: inspect all three exact `5d654d7` workflows. After they finish, push this checkpoint together with the next safe completed batch, then continue the SC-018 route visual audit or another verified software gap. Keep the production and hardware boundaries unchanged.

## Resume update — 2026-11-15 · Preview timing review from Runtime

- Main repository `ali96adil/StageCore`, branch `integration/stagecore-rc`, OPEN/Draft PR #534. Remote/PR HEAD remains `5d654d7c31a377b67c64f6ab65b7c713fa95229e`; local HEAD is `ec205117530266608c0421ba3373f48cdc150346` (two commits ahead: the preceding Handoff checkpoint and `ec20511` implementation). No push has occurred.
- Exact `5d654d7` Companion `43680858045` and Camera Relay `43680858075` checks pass; Core Go 1.27 passes. Core Go 1.26 first failed only because `TestBrowserCopiesMixedActionsAcrossProjectsAndMapsTargets` did not finish before its 30-second Chrome timeout. Rerun of the failed Core job was started on the same SHA as run `43680858030` and remains in progress at this checkpoint. Wait for that exact-head workflow to finish; do not push while active.
- SC-019 now exposes Timing review navigation to Owner/Technician for any active Rehearsal (including Preview), and for the same user’s remembered ended Rehearsal. The click only opens Timing; reversible explicit selection remains the only approval write. The Chrome gate loads the real timing script, seeds a Preview Session in temporary SQLite, verifies Owner and Technician access while active, stops it only through a simulator safety seam, verifies the Technician can revisit it, and asserts the API mutation trace consists only of the five explicit temporary Session-stop requests. No Cue GO or output is dispatched.
- Verification after the final browser fixture: focused real-Chrome test passed normally (2.59 s) and under Race (6.83 s). `go test ./internal/operatorweb` and its Race run passed; the explicit Runtime approval contract test passed; Operator Node tests passed 82/82; `go vet ./internal/httpapi ./internal/operatorweb`, JavaScript syntax checks, and `git diff --check` passed. All test data and Sessions were isolated; no actual output or device was used.
- SC-019 matrix evidence was updated in `ec20511`. Continue with the next evidence-backed software gap while the Core run completes; avoid repeating the saved Mixed/linked performance work. Pi 4 qualification still requires a positively identified isolated target. Production service/data/Vault/show, real stage outputs/devices, firmware, paid actions, backup deletion, force push, merge and production ACL activation remain prohibited.

## Resume update — 2026-11-15 · real-browser Timing Capture Pause/Resume

- Remote/PR #534 remains at `5d654d7c31a377b67c64f6ab65b7c713fa95229e`; implementation tip `86bf824292684350fe6c1e755a667acb0d9cc725` was four commits ahead before this Handoff checkpoint. At last check the rerun on `5d654d7`, Core run `43680858030`, passed Go tests, Browser Cue Delivery and Vet; Go 1.26 Race was active and ARM64 builds pending. Go 1.27, Companion `43680858045`, and Camera Relay `43680858075` passed. Wait for the remaining jobs before any push.
- SC-020’s real-browser gap is now covered by `TestBrowserShowWorkspacePinsRuntime`: the isolated handler wires `WithOperatorTimingIntelligence`, and a temporary active Preview loads its actual Timing row, enters Pause then Resume, and observes the UI follow the persisted states. Navigation to review is checked for writes before any explicit capture action; the final API trace contains exactly two session capture PUTs and five simulated Session STOP requests. The existing Cue Engine regression separately proves GO continues while capture is paused and its timing observation stays excluded.
- Combined SC-019/SC-020 focused Chrome acceptance passed normally (2.75 s) and under Race (7.11 s). `go vet ./internal/httpapi ./internal/operatorweb`, JS syntax and `git diff --check` passed. Operator Node 82/82 and Operator Web normal/Race tests passed for the SC-019 implementation; timing lifecycle, API/RBAC and Cue Engine regressions remain in their previously passing package suites.
- Matrix rows SC-019 and SC-020 now distinguish review navigation, explicit approval, and independent capture pause/resume evidence. No production code path for timing/ACL was changed in this SC-020 batch. The test uses temporary SQLite, simulator-only adapters and no Cue GO or external output.
- Next: wait for exact pushed-head Core Race/ARM64, recheck all three runs and PR/remote SHA, then push the preserved local stack by normal fast-forward to the same Draft PR. Continue a separate evidence-backed software gap (SC-018 route/zoom acceptance or SC-024 endpoint audit); do not repeat qualified performance runs unless timing code changes. All production, hardware, firmware, merge, force-push and production ACL limits remain unchanged.

## Safety update — Pi alias is not an isolated target

- A read-only identity probe of the local SSH alias `stagecore-pi` returned Raspberry Pi 4 Model B Rev 1.4, AArch64 Linux, `systemd-detect-virt=none`, and host cgroup `/init.scope`; selected USB/DMX serial device paths were absent from `/dev`. This establishes bare-metal host access, not the isolated benchmark sandbox.
- Existing authorization qualification records identify that host's Hub and Camera Relay services as active and the RC qualification service as inactive, with helper permissions capable of service control and physical commands. Therefore no performance workload, shell deployment, service command, or data access was attempted after identity discovery. The only remote calls read kernel/model/virtualization/cgroup identity and matching device-node names.
- Previously completed Pi performance evidence in `cue-execution-performance-qualification.md` used a device-hidden namespace and remains applicable because Cue Engine/Runtime timing sources have not changed in the current UI/test batches. The host CPU/RAM controllers were not isolated; formal Pi performance acceptance and any timing SLO remain open. Do not use `stagecore-pi` for new workload testing. `stagecore-qualification` remains unverified from its prior failed handshake and was not contacted again.
- Continue software work using isolated local/CI simulations. Any future Pi run requires an independently verified isolated environment with production services and physical device paths excluded.

## Related repository state — 2026-11-15

- Read-only PR review: TabletPlayer #36 `91613da`, ESP32 DMX Lighting #39 `3ad7749`, ESP32 Camera #20 `455b066`, and StageLaser #56 `b14a578` remain OPEN/Draft with their listed checks passing. StageLaser #57 `c4ff10b` remains OPEN/non-Draft with both firmware checks passing. These checks are source/build probes, not physical qualification.
- No checkout for those linked repositories is available in this workspace, so no linked repository was edited. Exact check names and links are in [`related-component-qualification-matrix.md`](../architecture/related-component-qualification-matrix.md).
- Main PR #534 remains OPEN/Draft at remote `3955c65fbc7b10d3b36015dd4642fb8dd9ca7fdc`; Core `43684038797` remains active in Go 1.26 Race with ARM64 pending, while Companion `43684038783` and Camera Relay `43684038809` pass. Local commits `ff820cb`, `4e4dc6d`, and this documentation update are preserved; do not push until exact-head Core completes.

## Resume update — 2026-11-15 · exact-head Race retry fixed

- The rerun of Core CI `43680858030` at pushed HEAD `5d654d7c31a377b67c64f6ab65b7c713fa95229e` completed. Go 1.26 Test, Browser Cue Delivery, and Vet passed, but Race failed only at `TestCompanionTransportTimelineChangesProjectScopeWithRoleAssignment`; Go 1.27 and exact-head Companion/Camera Relay workflows passed. ARM64 did not start because the Race step failed.
- The failure was a test synchronization gap: after seeing Project A's disconnect, the test queried Project B transport events exactly once while the new Project B `CONNECTED` event is appended asynchronously by the same runtime connection loop. There was no Race detector report. Commit `862d190` polls the bounded Project-scoped event query up to five seconds for exactly the expected event before asserting its type, scope, source, and empty Snapshot reference. Product/runtime behavior is unchanged.
- Verification: the targeted regression passed ten normal repetitions and ten Race repetitions (`19.0 s`); `go vet ./internal/companionchannel` and `git diff --check` passed. The complete local branch remains clean; functional and Handoff stack is seven commits ahead of remote `5d654d7` before this checkpoint commit.
- Next: recheck remote and PR #534 still at `5d654d7`, verify no active workflow remains for that head, then push the preserved stack with one normal fast-forward. Wait for Core, Companion and Camera Relay CI at the new exact SHA and address any failure before another push. No Pi workload, production service/data/Vault, real output, firmware, merge, force push or production ACL activation occurred.

## Resume update — 2026-11-15 · native Pause/Resume touch and keyboard acceptance

- The preserved stack was pushed normally to the existing branch/PR, advancing `5d654d7c31a377b67c64f6ab65b7c713fa95229e` to `3955c65fbc7b10d3b36015dd4642fb8dd9ca7fdc`; local, `origin`, `git ls-remote`, and Draft PR #534 matched at that SHA. No branch/PR was created and no force push/merge occurred.
- At last check, exact-head Companion `43684038783` and Camera Relay `43684038809` passed. Core `43684038797` remained active in Go 1.26/1.27 checks. Do not push the following local test/documentation update until all exact `3955c65` checks finish.
- SC-020's real-Chrome test now uses native emulated touch to Pause and native CDP Enter to Resume the active Preview capture control. The report reload reflects each persisted state; opening Timing still makes no selection/approval write. The trace requires exactly the explicit two capture PUTs plus five simulator Session stops. The fixture uses temporary SQLite, and no GO or external output is dispatched.
- Updated test passed normally (3.31 s) and with Race (7.04 s). The current uncommitted changes are only the browser fixture plus SC-020 matrix evidence and this checkpoint. `node --check`, `go vet ./internal/httpapi ./internal/operatorweb`, and `git diff --check` passed for the current test implementation. Continue only after `3955c65` CI completes; then commit and push this small qualification update to the same PR and wait for its exact-head CI.

## Resume update — 2026-11-15 · Cue Inspector narrow desktop geometry

- Main repository `ali96adil/StageCore`, branch `integration/stagecore-rc`, OPEN/Draft PR #534. Remote HEAD remains `3955c65fbc7b10d3b36015dd4642fb8dd9ca7fdc`; local HEAD is `ff820cbd4ad6fa707c1b2c74aafc1bc7592cfa43` plus the current uncommitted SC-018 qualification change. Exact-head Core `43684038797` is still active in Go 1.26 Race; Go 1.27, Companion `43684038783`, and Camera Relay `43684038809` pass. Do not push until Core has completed and all exact-head checks are inspected.
- SC-018's existing real-Chrome Cue Inspector geometry check now includes a 720×900 CSS viewport in both RTL and LTR, in addition to 390×844, 768×1024, 1024×768 and 1440×900. The new desktop-sized 720 CSS-pixel case checks the responsive equivalent of a 1440px-wide page at 200% scale, but it is not evidence of actual browser zoom. All ten inspected layouts have no document overflow and keep Inspector navigation visible; Cue table content remains internally scrollable at narrow widths.
- Verification: `TestBrowserCreatesNestedMixedCueChainInDraft` passed in real Chrome normally (6.44 s) and with Race (13.22 s). The 720px measurement reports table client width 660 with scroll width 720 in RTL, and width 660 with scroll width 914 in LTR. Actual 200% browser zoom, retained annotated screenshots and the route-by-route inventory remain open. No production code, Cue execution, ACL, outputs or device integration changed.
- Next: finish focused static checks, commit this measurement and matrix update locally, and keep waiting for Core CI at `3955c65`; after all checks pass, normally fast-forward the preserved local commits to the same branch/PR and inspect its new exact-head workflows. Continue with an actual zoom/screenshot method or another verified software gap without overstating this CSS viewport result.
- Production Hub/Camera Relay/services/data/Vault/show, real stage devices/outputs, firmware, backups, paid actions, force push, merge, and production SC-024 activation remain untouched/prohibited.

## Resume update — 2026-11-15 · actual 200% Chrome zoom and screenshots

- Remote/PR #534 remains at `3955c65fbc7b10d3b36015dd4642fb8dd9ca7fdc`; local commits `ff820cb`, `4e4dc6d`, `5c6d3c9`, and `5b155e1` plus this 200%-zoom batch are preserved. Core `43684038797` is still running Go 1.26 Race on the exact remote SHA; ARM64 remains pending. Companion `43684038783` and Camera Relay `43684038809` pass. Do not push until all exact-head workflows complete.
- The mixed-Cue Chrome acceptance now launches a second isolated Chrome profile at actual 200% browser zoom using its page-zoom preference and a 1440px window. It verifies 720 CSS-pixel `innerWidth`, DPR 2, no document overflow, visible in-viewport Inspector controls, and zero Project API mutations. Both Arabic RTL and English LTR measurements passed; the Cue table is internally scrollable at 660/720px RTL and 660/794px LTR.
- Captured and visually inspected retained screenshots: [`cue-inspector-200-percent-ar.png`](../architecture/evidence/sc018-cue-inspector/cue-inspector-200-percent-ar.png) and [`cue-inspector-200-percent-en.png`](../architecture/evidence/sc018-cue-inspector/cue-inspector-200-percent-en.png), described in [`README.md`](../architecture/evidence/sc018-cue-inspector/README.md). The images show the current Cue table's known internal horizontal scrolling; they cover only Cue list/Inspector, not every Operator route.
- Verification: focused test passed real Chrome normally (8.35 s) and under Race (14.68 s), including both screenshot captures; `node --check` and diff check passed before the Race run. Repeat focused `go vet ./internal/httpapi ./internal/operatorweb` for the final zoom implementation, then commit this batch and continue waiting on `3955c65` CI. Remaining SC-018 work is the route-by-route screenshot/zoom audit and separate tablet hardware fit.
- No production services/data/Vault/show, runtime session, real device/output, firmware, paid action, backup, force push, merge, or production SC-024 activation was touched.

## CI completion — 2026-11-15 · push gate released

- Exact remote HEAD `3955c65fbc7b10d3b36015dd4642fb8dd9ca7fdc` completed successfully: Core run `43684038797` passed Go 1.27 and Go 1.26 Test, Browser Cue Delivery, Vet, Race, and Linux ARM64 CGo-free builds; Companion Core `43684038783` and Camera Relay `43684038809` passed. PR #534 remains OPEN/Draft on `integration/stagecore-rc`.
- The final real-Chrome zoom test passed normally (7.26 s) with retained screenshots written to `docs/architecture/evidence/sc018-cue-inspector/`; the same test passed Race (14.68 s) with temporary screenshots. `go vet ./internal/httpapi ./internal/operatorweb`, both relevant `node --check` commands, and `git diff --check` passed.
- Local source is unchanged by the remote CI and contains the preserved four commits `ff820cb`, `4e4dc6d`, `5c6d3c9`, `5b155e1`, plus the current actual-zoom test, screenshots, SC-018 matrix and this Handoff update. Commit the qualification batch, verify worktree/branch/PR/remote SHA and absence of active exact-head checks, then push by normal fast-forward to the existing PR. Follow all workflows on the resulting exact SHA before the next push.
- The test and screenshots remain confined to temporary SQLite and a temporary Chrome profile; no session or output was dispatched. Production services/data/Vault/show, real devices/outputs, firmware, backups, paid actions, merge, force push and production ACL activation remain untouched.

## Push update — 2026-11-15 · SC-018 actual 200% zoom evidence

- Pushed five commits by normal fast-forward to existing branch `integration/stagecore-rc` / OPEN-Draft PR #534, advancing `3955c65fbc7b10d3b36015dd4642fb8dd9ca7fdc` to `05f2cf53f0e58e8498521c638d33f0d0eea3f524`. Local HEAD, origin branch and PR head match. No force push, branch creation or merge occurred.
- Exact prior head `3955c65` passed Core `43684038797` (Go 1.26/1.27, Race, Browser Cue Delivery, Vet, Linux ARM64), Companion Core `43684038783`, and Camera Relay `43684038809`.
- New exact-head workflows on `05f2cf5` are active: Core `43686295363`, Companion Core `43686295344`, and Camera Relay `43686295364`. Do not push another head until all three complete and are inspected.
- The pushed code now qualifies Cue Inspector at actual 200% Chrome zoom in RTL/LTR, retaining two screenshots in `docs/architecture/evidence/sc018-cue-inspector/`. It also keeps the 720px responsive viewport, Timing Pause/Resume native input, refreshed linked-PR state, and the accompanying completion matrix/checkpoint. Local Chrome normal/Race, Vet, JS syntax and diff checks passed before the push.
- Next: wait for the three exact `05f2cf5` workflows, inspect their conclusions, then continue the route-by-route SC-018 visual/zoom audit or another open software qualification gap. No Pi workload, production service/data/Vault/show, real stage output/device, firmware, backup, paid operation, or production ACL activation occurred.

## Linked firmware repository inspection — 2026-11-15

- Current open PR heads/checks were re-read: TabletPlayer #36 `91613da` (Android pass), DMX Lighting #39 `3ad7749` (six CI checks pass), Camera #20 `455b066` (build/partition check pass), StageLaser #56 `b14a578` (host-contract/no-actuation firmware build pass), and #57 `c4ff10b` (host-contract/no-actuation firmware build pass). These are source/build checks; physical pair/reconnect, camera stream, DMX output, laser OFF/flash timing and installed-client qualification remain separate.
- Local copies were inspected read-only. No copy is checked out at these PR heads: TabletPlayer checkout is absent; Camera copies are at unrelated/older commits; the StageLaser clone is on `fix/stagelaser-online-tls-watchdog-restore` at `98edf74`, not #56/#57; DMX copies are detached at earlier commits. Several DMX copies contain untracked `sdkconfig` files (`StageCore-ESP32-DMX-Lighting-diagnostic`, `-ef338b1d`, `-final-7b7e0cd3`, `-flash`, `-iram-candidate`, `-recovery`, `-uart-snapshot`); all were preserved. No linked repository was changed.
- Main PR #534 remains at remote `05f2cf53f0e58e8498521c638d33f0d0eea3f524`. Core `43686295363` is active; Companion `43686295344` and Camera Relay `43686295364` pass. Do not push until Core completes and all three exact-head outcomes are inspected.

## SC-024 Owner Project Access zoom evidence — 2026-11-15

- Remote, origin and Draft PR #534 remain on `05f2cf53f0e58e8498521c638d33f0d0eea3f524`; local branch contains the preserved docs commits `84c6975` and `aa0b54a`, plus this prepared SC-024 browser qualification. Core run `43686295363` is still the only active check at the pushed SHA; Companion Core `43686295344` and Camera Relay `43686295364` pass. No push until Core completes and all three exact-head workflows are inspected.
- Extended `TestBrowserOwnerProjectAccessEditorStoresPreparedLevel` with a second isolated Chrome profile at actual 200% browser zoom. RTL Owner Project Access fits at a 720 CSS-pixel viewport: the Project grant form has no internal overflow, the page has no horizontal overflow, the enforcement warning and Save control are visible, and a prepared OPERATE selection records zero `/api/v1` mutations. Temporary SQLite is used; no grant is submitted by the zoom scenario.
- Retained and visually inspected [`project-access-200-percent-rtl.png`](../architecture/evidence/sc018-project-access/project-access-200-percent-rtl.png); setup and the header/scroll capture state are documented beside it. The screenshot confirms layout only, not production enablement or applied permission changes.
- Verification passed: focused real-Chrome test normally (4.23 s) and under Race (5.846 s package time), `go vet ./internal/httpapi ./internal/operatorweb`, `node --check internal/httpapi/testdata/project_access_zoom_browser.js`, and `git diff --check`. The earlier normal and Race browser runs both executed the 200% audit; no production or stage output was involved.
- Next: inspect exact `05f2cf5` Core/Companion/Relay results, finish and commit this qualification/docs update, then normally push the preserved commits only after the exact-head gate clears. Continue to prioritize remaining software acceptance (all-route visual audit and SC-024 whole-server/asynchronous ACL review). Do not use the bare-metal `stagecore-pi` alias for workload tests. Production services/data/Vault/show, real outputs/devices, firmware, backups, force push, merge, paid actions and production ACL enablement remain prohibited/untouched.

## SC-016 Cross-Project Mixed-Cue target-map zoom — 2026-11-15

- Extended `TestBrowserCopiesMixedActionsAcrossProjectsAndMapsTargets` to launch Chrome at actual 200% page zoom (1440px window, 720 CSS-pixel RTL viewport) and assert there is no document or dialog overflow and the target-map Apply control is visible before the mapping is submitted. It still verifies copied Lighting/OSC Action ordering, preserved payload, new Action IDs, distinct source/destination aliases, no configuration copy, and no publish/GO/runtime call.
- Focused real-Chrome test passed after the final RTL assertion (`1.315 s` package time); focused Race passed (`4.718 s`). `go vet ./internal/httpapi ./internal/operatorweb`, Node syntax validation and diff checks passed. Data is confined to a temporary SQLite Project Draft; no real targets or stage outputs are connected.
- Local code checkpoint at start of this batch was `ac50f05`; the change and matrix record remain uncommitted here. Remote and PR #534 remain on exact pushed `05f2cf53f0e58e8498521c638d33f0d0eea3f524`; Core run `43686295363` remains active in Go 1.26 Race, while Companion Core `43686295344` and Camera Relay `43686295364` pass. Do not push until that Core workflow completes and all three exact-head results are inspected.
- Next: finish and commit this batch, continue the route-level visual qualification, and check the exact-head CI gate before normal push. Preserve all earlier local commits. Production Hub/Relay/data/Vault/show, stage devices/outputs, firmware, backups, force push, merge, paid actions and production ACL remain untouched/prohibited.

## SC-016 target-map keyboard regression — 2026-11-15

- The red-first native Chrome keyboard test at actual 200% RTL reproduced a UI defect: pressing Enter while the first target selector was focused closed the `<dialog>` before mapping. Both selections remained empty and the pasted Actions retained `SOURCE-LIGHT` / `SOURCE-OSC` in the Draft. The failure was confirmed against the old form implementation before the fix.
- Cue Studio now avoids the implicit `method="dialog"` submission path and applies mappings from a real `type="button"` control. The regression presses native CDP Enter on an incomplete selector and requires the dialog to stay open; it then selects both destination targets, focuses Apply and presses native Enter, requiring the mapping to apply to the Draft. No publish, runtime, GO, or real-target API is used.
- Focused real-Chrome test passed after the fix (1.24 s); Race passed (5.141 s). `go vet ./internal/httpapi ./internal/operatorweb`, Node syntax checks for both scripts, `node --test tests/cue_studio_roundtrip.test.js tests/operator_accessibility.test.js` (4/4), and `git diff --check` passed. The test's SQLite and Projects are temporary.
- Current local base is `866f3e2`; this fix, regression, matrix and Handoff update remain uncommitted. Remote/PR #534 remain at `05f2cf53f0e58e8498521c638d33f0d0eea3f524`, Core `43686295363` is still active in Race, and Companion `43686295344` plus Camera Relay `43686295364` pass. Do not push until all exact-head workflows complete and are inspected.
- Native arrow selection and touch mapping remain unqualified; the matrix records these limits. Continue with route-level browser acceptance and the remaining SC-024 delivery audit after committing this small batch. Production ACL, Hub/Relay, original show data/Vault, physical devices/outputs, firmware, backups, force push and merge remain untouched/prohibited.

## Resume checkpoint — 2026-11-15 · current local stack

- Local HEAD: `1b45c60`; branch `integration/stagecore-rc`; local stack is five commits ahead of remote `05f2cf53f0e58e8498521c638d33f0d0eea3f524`: `84c6975`, `aa0b54a`, `ac50f05`, `866f3e2`, and `1b45c60`. These include preserved handoff/linked-checkout notes, Project Access actual-zoom evidence, Mixed-Cue target-map actual-zoom evidence, and the selector-Enter defect fix/regression.
- Existing PR #534 remains OPEN/Draft. On exact remote head `05f2cf5`, Companion Core `43686295344` and Camera Relay `43686295364` passed; Core `43686295363` remains active in Go 1.26 Race after Go 1.26/1.27 tests, Browser Cue Delivery and Vet passed. Do not push until it completes; then inspect Race, ARM64 build, the whole Core result, and all three exact-head workflows. Push the five-commit stack normally only if the gate is green and local/origin/PR ancestry still match.
- The latest focused code changes passed real Chrome at actual 200% zoom, Race, `go vet ./internal/httpapi ./internal/operatorweb`, Node syntax and four Cue Studio/operator-accessibility tests. The cross-Project regression uses temporary SQLite and verifies no publishing/runtime command; the Project Access zoom scenario records no API mutation.
- Resume after the push gate by continuing the SC-018/SC-017 route-level visual acceptance and SC-024 endpoint/delivery audit. The remaining matrices identify the LTR/keyboard/touch target-map paths, whole-server/asynchronous permission reconciliation, existing Companion deployment compatibility, disposable installed-copy update/rollback rehearsal, and isolated Raspberry Pi 4 performance qualification. Do not use the bare-metal `stagecore-pi` alias as a workload sandbox. Keep production ACL disabled and all production/hardware/firmware/merge/force-push boundaries in force.

## Workspace Profile keyboard draft-loss regression — 2026-11-15

- A red-first real-Chrome test reproduced a second implicit-form-submission defect: native Enter in the custom Workspace Profile name field closed the editor while device-local storage retained only the old name. Cue Studio and Workspace Profile had similar dialog-form patterns, but this failure was isolated to the profile editor.
- Workspace Profile now prevents implicit form submission, uses explicit `type="button"` Close controls, and retains explicit Save/Delete button handlers. The browser regression opens a temporary editable profile, sends native CDP Enter in its unsaved name field, confirms the dialog remains open and localStorage is unchanged, then clicks Save and verifies the name persisted. No Hub API call, Project data or live show is involved. An `operatorweb` contract test prevents reintroducing dialog submission.
- Focused real Chrome passed (1.307 s) and Race passed (2.066 s). `go test ./internal/operatorweb -count=1` passed; `TestBrowserShowWorkspacePinsRuntime` passed normal (3.093 s) and Race (8.598 s), confirming the SHOW workspace flow still works. `go vet ./internal/httpapi ./internal/operatorweb`, both Workspace Profile JS syntax checks and `git diff --check` passed.
- Exact previous remote/PR #534 head `05f2cf53f0e58e8498521c638d33f0d0eea3f524` has completed green: Core `43686295363` passed Go 1.26/1.27 Test, Browser Cue Delivery, Vet, Race and Linux ARM64 CGo-free builds; Companion Core `43686295344` and Camera Relay `43686295364` passed. PR remains OPEN/Draft on `integration/stagecore-rc`.
- Local branch was six commits ahead before this Workspace Profile batch (`84c6975`, `aa0b54a`, `ac50f05`, `866f3e2`, `1b45c60`, `c74fc27`). Workspace Profile code, the real-Chrome regression and matrix edit are committed as `3a74325`; this handoff refresh records the final push gate. Once verified, normally push the preserved stack to the same PR and inspect every exact-new-head workflow before another push.
- Continue route-by-route visual qualification and SC-024 permission/delivery audit. Pi performance is not rerun on the exposed bare-metal `stagecore-pi`; no production Hub/Relay/data/Vault/show, actual device/output, firmware, backup, merge, force push, paid service or production ACL activation occurred.

## SC-018 Workspace Profile 200% action-bar qualification — 2026-11-15

- Remote branch and OPEN/Draft PR #534 are on `322ad28656be476713cb2aa723870fb78dd49714`. Exact-head Companion Core `43689499988` and Camera Relay `43689500040` pass; Core `43689500088` is still active. Do not push another head until Core finishes and the three exact-head conclusions are inspected.
- Before this Handoff note, local HEAD was `20c9a9b29893983bbd4e9dc580cc8ebd79a549f6`, one commit ahead of remote; this docs-only checkpoint is the next local commit. `20c9a9b` adds an actual 200% RTL Chrome gate, retained screenshot, matrix evidence and F-017 action-bar CSS correction. The first geometry assertion reproduced that Save extended to y=370.5 CSS px while the dialog clipped at y=363.5; `.f017-dialog-actions` now stays inside the scroll frame. The passing gate confirms DPR 2, a 720px viewport, no document overflow and a fully visible Save button. Screenshot and capture conditions are documented in `docs/architecture/evidence/sc018-workspace-profile/`.
- The same real-Chrome test still presses native Enter in an unsaved profile name, confirms no dialog close/localStorage write, then explicit Save persists the name with zero Hub API calls. Focused Chrome normal and Race runs pass. `TestBrowserShowWorkspacePinsRuntime` passes normal and Race after the CSS change; `go test ./internal/operatorweb -count=1`, `go vet ./internal/httpapi ./internal/operatorweb`, JS syntax and `git diff --check` pass.
- Current linked Draft PRs remain at TabletPlayer #36 `91613da` (tablet-player check pass), ESP32 DMX #39 `3ad7749` (six CI checks pass), ESP32 Camera #20 `455b066` (hardware-probe CI pass), and StageLaser #56 `b14a578` plus #57 `c4ff10b` (firmware no-actuation and host-contract checks pass). No related checkout was altered.
- Next: wait for all exact `322ad28` checks, verify local/remote/PR ancestry and normally push the preserved single-commit `20c9a9b` update; then inspect the new exact-head CI. Continue the route-by-route UI visual inventory and the staged SC-024 endpoint/delivery audit. Raspberry Pi 4 workload qualification remains blocked on a disposable isolated image: `stagecore-pi` is a production bare-metal host. Project ACL stays disabled in production and PR #534 remains Draft; all production data/services, show files, hardware outputs and firmware remain untouched.

## SC-018 Workspace Profile LTR zoom follow-up — 2026-11-15

- Remote and Draft PR #534 are unchanged at `322ad28656be476713cb2aa723870fb78dd49714`; exact-head Core `43689500088` remains in progress, with Companion Core `43689499988` and Camera Relay `43689500040` passing. No push until Core completes and the exact-head workflow set is inspected.
- Local HEAD before this note is `25278841d6006f62b630af1e1903683c6e074a78`, three commits ahead of the remote: `20c9a9b` (Profile action bar and RTL 200% gate), `a300bfb` (handoff), and `2527884` (LTR 200% capture/gate). The LTR update retains `workspace-profile-200-percent-ltr.png`; the two actual-zoom checks both report 720 CSS px, document width 720, DPR 2 and a fully visible Save control.
- The focused Chrome test passed normally and under Race after adding the LTR path. `TestBrowserShowWorkspacePinsRuntime` passed normal and Race after the CSS fix. `go test ./internal/operatorweb -count=1`, `go vet ./internal/httpapi ./internal/operatorweb`, JS syntax and `git diff --check` passed. The screenshot README and SC-018 matrix now distinguish both directions.
- Resume: wait for run `43689500088`; if all three exact `322ad28` workflows pass and remote/PR ancestry remains unchanged, push the three preserved commits normally, then inspect every new-head workflow before another push. The broader route-by-route visual inventory and SC-024 endpoint/delivery audit remain next. No Pi workload on the exposed production host; production ACL stays disabled.

## SC-018 shared Appearance and Profile dialog action clipping — 2026-11-15

- Remote/PR #534 remain on `322ad28656be476713cb2aa723870fb78dd49714`; Companion Core `43689499988` and Camera Relay `43689500040` pass, while Core `43689500088` is still in the Go 1.26 Race step. Do not push before all exact-head jobs finish.
- Before this Handoff note, local HEAD was `b8fdc5a7340549a652438630d8fbdf9866b9f7ad`, five commits ahead of remote: `20c9a9b`, `a300bfb`, `2527884`, `ed9f252`, and `b8fdc5a`. `b8fdc5a` moves the dialog action-bar offset correction into the shared `.dialog-actions` rule so the F-016 Appearance modal is fixed along with F-017 Workspace Profile.
- Actual 200% Chrome at DPR 2 and 720 CSS px reproduced the shared appearance Close action at y=370.5 while its dialog ended at y=363.5; Profile Save showed the same clipping. After the global sticky-frame correction, the regression measures RTL and LTR for both dialogs and retains four screenshots under `docs/architecture/evidence/sc018-workspace-profile/`. Native Enter still cannot discard a Profile draft; explicit local Save is verified, and no Hub API is called.
- The focused Profile/Appearance Chrome group, `TestBrowserShowWorkspacePinsRuntime`, and `TestBrowserCopiesMixedActionsAcrossProjectsAndMapsTargets` passed normal and Race runs after the global CSS change. `go test ./internal/operatorweb -count=1`, `go vet ./internal/httpapi ./internal/operatorweb`, both JavaScript syntax checks, and `git diff --check` passed.
- Resume: wait for the exact `322ad28` Core run, inspect its Race and ARM64 results plus Companion/Relay, then normally push the five saved commits above to the same Draft PR if local ancestry and remote HEAD still match. Verify all newly triggered CI before any later push; continue the remaining route-by-route visual inventory and staged SC-024 delivery review. No production service/data/Vault/show, physical output/device, firmware, Pi workload, merge, force push, paid action, backup deletion or production ACL activation occurred.


## SC-016 target-map LTR and touch Apply checkpoint — 2026-11-15

- Remote branch and Draft PR #534 remain at `322ad28656be476713cb2aa723870fb78dd49714`. Local HEAD at this checkpoint is `e2b86c29ba572847d09a8e1ad8d62359ce07ed4c`, six local commits ahead of remote (`20c9a9b`, `a300bfb`, `2527884`, `ed9f252`, `b8fdc5a`, `e2b86c2`); the target-map test extension and this documentation update are the current uncommitted batch. Preserve this stack; normal push remains gated on exact-head Core `43689500088`.
- Core `43689500088` last showed Go 1.26 Race in progress after Go 1.26/1.27 Test, Browser Cue Delivery and Vet passed. Companion Core `43689499988` and Camera Relay `43689500040` passed. Inspect final status of all three workflows before any push.
- `TestBrowserCopiesMixedActionsAcrossProjectsAndMapsTargets` now measures the target-map dialog in RTL and LTR at actual 200% zoom / 720 CSS pixels and applies the Draft mapping by CDP emulated touch at the visible Apply control. The choices are set in the headless fixture; the test does not claim touch-based native selector choice. Focused Chrome test passed normally and under Race; `go vet ./internal/httpapi ./internal/operatorweb`, browser-script syntax and `git diff --check` passed.
- Native ArrowDown experiments in headless Chrome (including raw-key dispatch and an actual CDP click on the selector) delivered DOM key events but did not change the native select value. The attempted keyboard assertion was removed rather than weakening it into script-assigned keyboard evidence. Native arrow selection remains unqualified and needs a visible-browser path or another credible input harness; no product behavior defect is inferred from the headless limitation.
- Next: verify exact 322ad28 Core completion and all three exact-head workflow conclusions; then push the preserved local stack plus this passing checkpoint normally to the existing `integration/stagecore-rc` / PR #534 if ancestry still matches. Inspect all resulting exact-head checks before another push. Continue SC-016 visible-input qualification and the remaining route-level visual / SC-024 delivery audits. Production services/data/Vault/show, hardware outputs, firmware, exposed bare-metal Pi workloads, force push, merge, paid actions and production ACL activation remain untouched/prohibited.

## Resume checkpoint — 2026-11-15 · pushed SC-016 target-map qualification

- Local and remote `integration/stagecore-rc` plus OPEN/Draft PR #534 match at `5331d3b674e3464a4b3b088616f307b565356a11`. This is a normal push of seven saved commits above `322ad28656be476713cb2aa723870fb78dd49714`; no force push or new PR/branch was used.
- The prior exact-head gate on `322ad28` completed green: Core `43689500088`, Companion Core `43689499988`, Camera Relay `43689500040`. New exact-head workflows on `5331d3b` are running: Core `43692176474`, Camera Relay `43692176469`, Companion Core `43692176470`. Do not push again until these three finish and are inspected.
- Commit `5331d3b` extends SC-016 browser acceptance to actual 200% RTL/LTR geometry and CDP emulated touch on the visible Apply control. The headless fixture sets destination options itself, so the matrix explicitly leaves native arrow selection and touch-based selector choice unqualified. Focused Chrome normal/Race, SC-024 focused Race, `internal/projectaccess` Race, `go vet`, JavaScript syntax and diff checks passed locally.
- Headless native ArrowDown delivered DOM key events but did not change the native select value, even after raw CDP key dispatch and an actual CDP click; the attempted keyboard assertion was removed. No product defect is inferred without a visible-browser reproduction.
- Next: inspect all three new-head workflows, then continue route-level visual qualification and SC-024 endpoint/delivery review. Existing Pi 4 Mixed/Linked performance measurements are comprehensive and need no repeat absent Engine/runtime source changes. The exposed bare-metal `stagecore-pi` host remains unavailable for workload tests; the stopped local Colima profile is Linux ARM64 and is not a Raspberry Pi 4 substitute. Keep ACL production opt-in/off and all production, real-device, firmware, data/Vault/show, merge, force-push, backup and paid-action boundaries intact.

## SC-016 visible-selector follow-up — 2026-11-15

- The optional visible-Chrome ArrowDown experiment could not attach to Chrome: its 30-second test context expired while the Mac session was locked. CUA reported that automatic unlock was unavailable. The conditional test changes were removed completely; the previously passing headless LTR/touch test files are unchanged. No screenshot or keyboard-selection claim was produced.
- Remote and PR #534 remain at `5331d3b674e3464a4b3b088616f307b565356a11`. Exact-head Camera Relay `43692176469` and Companion Core `43692176470` pass; Core `43692176474` remains in progress. Do not push while this Core run is active.
- Local HEAD is the docs-only `825fc60` checkpoint atop remote. Worktree is clean. Native select keyboard acceptance remains open until a visible browser can be used; continue independent work without waiting for the locked desktop.

## SC-023 / SC-018 Project Files 200% zoom checkpoint — 2026-11-15

- `TestBrowserScopedProjectFilesShowsUsageAndPageCounts` now runs at actual Chrome 200% zoom (`devicePixelRatio=2`, 720 CSS px) in Arabic RTL and English LTR. It keeps the existing search/clear, usage totals, row-count, read-only, and keyboard-focus assertions and rejects page overflow. Temporary Projects, SQLite, media metadata and the isolated Chrome profile are used; no Vault source object is read or written.
- Focused Chrome passed normally and under Race; `go vet ./internal/httpapi ./internal/operatorweb`, JavaScript syntax and diff checks passed. The observed UI had no page-level overflow in either locale; no product CSS change was needed.
- The SC-018 and SC-023 matrix rows record the new route evidence. The remaining SC-018 route inventory and physical tablet fit remain open.
- Remote and PR #534 remain on `5331d3b674e3464a4b3b088616f307b565356a11`. Exact-head Camera Relay `43692176469` and Companion Core `43692176470` pass; Core `43692176474` was last observed in Go 1.26 Race. Local branch was at `d530598c91908232637ecf96c377ca9c28d994b8` before this batch; keep this test/matrix/checkpoint commit local until the exact-head Core run finishes and all three results are inspected.
- The optional visible native-select experiment remains unqualified because the Mac session is locked; its failed conditional test changes were fully removed. Continue with safe local work; no production service/data/Vault/show, physical output/device, firmware, merge, force push, paid service, or production ACL activation was touched.

## F-010 isolated installed-copy transaction — 2026-11-15

- Draft PR #534 remains OPEN on `integration/stagecore-rc`. The exact remote/PR
  head `5331d3b674e3464a4b3b088616f307b565356a11` completed green: Core run
  `43692176474` passed Go 1.26/1.27 Test, Browser Cue Delivery, Vet, Go 1.26
  Race and Linux ARM64 builds; Companion Core `43692176470` and Camera Relay
  `43692176469` passed. PR and `git ls-remote` still identify the same head.
- Local implementation commit `3f5b405` adds
  `TestIsolatedInstalledCopyUpdateAndRollback`: actual update Service,
  deployment Installer and verified Snapshotter, with temporary install/config/
  Data/Vault/unit/backup roots, fake systemd and Doctor, and loopback readiness.
  It checks candidate success, readiness-timeout rollback and postflight-Doctor
  rollback, including byte-for-byte restoration of all required binaries,
  config, Data Root, Vault Root and systemd unit. It proves that the isolated
  transaction uses the named qualification service and never enables it or
  names production units/backup roots. No real service or device is started.
- Linux ARM64 verification ran in the local Go 1.27.1 container on the existing
  Colima profile (not Raspberry Pi): `go test ./internal/update -count=1` and
  `go test -race ./internal/update -count=1` both passed as root and executed all
  three installed-copy scenarios. The ordinary macOS package/Race tests and
  `go vet ./internal/update ./internal/deployment` passed; this Linux-root-only
  transaction test intentionally skips on macOS/non-root runs. Core CI now
  builds and runs it, both normally and under Race, with `sudo -n` on its Linux
  runner. Core workflow YAML parsed successfully. The Colima VM was stopped
  after qualification; this is software integration evidence, not Pi or real
  systemd acceptance.
- SC-024 matrix and F-010 feature docs now distinguish this isolated file-level
  transaction from the still-open full migration, real systemd/Hub health and
  physical Pi update acceptance. SC-024 remains partial and production-off.
- Five commits are ready above remote `5331d3b`: `825fc60` (resume checkpoint),
  `d530598` (visible selector limitation), `84f1a86` (Project Files actual
  200% zoom), `3f5b405` (isolated update/rollback test), and this handoff
  checkpoint. Recheck ancestry/PR head and all three old-head workflows, then
  push normally to this same Draft PR. Wait for every exact new-head Core,
  Companion and Camera Relay workflow before any subsequent Push.
- Next independent software gap: continue the SC-024 whole-server
  endpoint-to-policy and asynchronous-delivery audit, while preserving the
  existing grant revocation, STOP/Blackout independence, and production-off
  boundary. The route review must not query or alter production services/data.

## Push checkpoint — 2026-11-15 · isolated update rehearsal

- Normal fast-forward push moved `integration/stagecore-rc` and OPEN/Draft PR
  #534 from `5331d3b674e3464a4b3b088616f307b565356a11` to
  `528bac6b912f500bdeef85e1dabd50a15e3faa71`. Local HEAD, `origin`,
  `git ls-remote`, and PR head match; the PR remains OPEN/Draft. The five
  pushed commits are `825fc60`, `d530598`, `84f1a86`, `3f5b405`, and
  `528bac6`. No force push, branch/PR creation, or merge occurred.
- The exact prior-head gate was green: Core `43692176474`, Companion Core
  `43692176470`, and Camera Relay `43692176469`. New exact-head runs at
  `528bac6` are Core `43694919781`, Companion Core `43694919744`, and Camera
  Relay `43694919806`, all in progress at this checkpoint. Do not push again
  until each has completed and its result is inspected.
- The F-010 transaction test passed in Linux ARM64 Go 1.27.1 both normally and
  under Race, including the full `internal/update` package with all three
  installed-copy scenarios. macOS package/Race/Vet checks passed; the
  Linux-root-only integration test skips on macOS and is explicitly executed
  normal/Race under sudo in Core CI. This Colima result is not Pi or real
  systemd qualification; its VM has been stopped.
- Next: while the three exact-head workflows run, continue the SC-024
  whole-server endpoint-to-policy and asynchronous-delivery review. Preserve
  the production ACL-off rule and stop/Blackout grants; do not query or alter
  production services or data.

## Resume checkpoint — 2026-11-15 · Runtime error delivery fence

- Branch: `integration/stagecore-rc`; OPEN/Draft PR #534. Remote/origin/PR
  head remains `528bac6b912f500bdeef85e1dabd50a15e3faa71`. The tested local
  code commits are `5a6476785bd2878524f1b729804a89caf9a598d6` and
  `6390701b3a1449c88d5017687a2911350699c2ad`; local HEAD is `6390701` and the
  four-commit stack also includes docs checkpoints `851c4c6` and `f11e1db`.
  Nothing from this stack is pushed. Worktree is clean after committing the
  implementation batches.
- Exact `528bac6` workflows: Companion Core `43694919744` and Camera Relay
  `43694919806` passed. Core `43694919781` passed Go 1.27; Go 1.26 Test,
  isolated installed-copy rollback, Browser Cue Delivery and Vet have passed;
  Go 1.26 Race is in progress and Linux ARM64 builds are pending. Do not push
  until Core and the complete three-run exact-head gate are green and inspected.
- The SC-024 audit found that GET Project Runtime returned a Store failure
  without the final live session/Project-view check used on success. The
  red-first regression observed `503 PROJECTS_UNAVAILABLE` after revocation
  during status construction. The route now returns the live scoped denial
  with `no-store`; `TestRuntimeStatusSuppressesStoreErrorAfterProjectGrantRevocation`
  passed normally and under Race. The existing authenticated Runtime flow also
  passed, `go vet ./internal/httpapi` passed, and `git diff --check` passed.
  Qualification assembly only; production ACL remains disabled.
- The same red-first response audit found the GET Lighting Configuration
  route returning raw `stage_devices` SQLite diagnostics after Project-view
  revocation. `TestLightingConfigurationSuppressesReadErrorAfterProjectGrantRevocation`
  reproduces this with a dropped table only in temporary SQLite and now requires
  the scoped denial. All `TestLightingConfiguration*` tests passed normally
  and under Race; `go vet ./internal/httpapi` and `git diff --check` passed.
  The focused tests invoked no physical or network device.
- Refreshed related Draft PR heads/checks: TabletPlayer #36 `91613da` passed;
  DMX Lighting #39 `3ad7749` passed all six checks; ESP32 Camera #20
  `455b066` passed; StageLaser #56 `b14a578` and #57 `c4ff10b` passed both
  firmware/no-actuation and host-contract checks. No linked checkout or PR was
  changed. The Pi 4 Mixed/Linked/Burst/Sustained report is tied to source
  `a90403b`, not this campaign's code head: Engine Session lookup/start-state
  gating and Runtime Control STOP wait have changed since. The report now
  labels the old run historical. Current-head Pi percentiles and CPU/RAM stay
  unqualified until an isolated Pi 4 lab is available; do not use the
  production `stagecore-pi` host. Mac results were not repeated.
- Next: continue the SC-024 endpoint error/delivery audit with regressions for
  any reproduced gap; then recheck the exact-head CI gate and push the local
  commits normally to this same PR. Keep Production ACL off and preserve all
  production, show/Vault/data, real-device, firmware, backup, merge, force-push,
  and paid-action restrictions.

## Qualification batch checkpoint — 2026-11-15 · HTTP read-error fences

- At the start of this checkpoint, local HEAD was `2893ec52c69a123e8c3a332db5f26a82e6696c03`,
  six commits ahead of origin `528bac6b912f500bdeef85e1dabd50a15e3faa71`:
  `851c4c6`, `5a64767`, `f11e1db`, `6390701`, `1570a97`, and `2893ec5`.
  PR #534 remains OPEN/Draft at `528bac6`; no change was pushed.
- Full `go test ./internal/httpapi -count=1` passed in 78.973 s after both
  SC-024 fixes. The Runtime grant-revocation regression and all
  `TestLightingConfiguration*` tests passed normally and under Race;
  `go vet ./internal/httpapi` and `git diff --check` passed. The two red-first
  failures and fixed behavior are recorded above and in the authorization
  qualification/matrix docs.
- Exact-head `528bac6`: Companion Core `43694919744` and Camera Relay
  `43694919806` passed. Core `43694919781`: Go 1.27 passed; Go 1.26 Test,
  installed-copy rollback, Browser Cue Delivery and Vet passed; Race remains
  in progress, Linux ARM64 builds pending. No push until Core finishes and the
  exact-head three-workflow gate is inspected.
- Pi 4 results are historical for source `a90403b`. Current-head P95/P99 and
  resource qualification remain pending an isolated Pi 4 environment; do not
  use the production `stagecore-pi` host. Mac performance results were not
  repeated. Production ACL stays disabled.
- First action after this checkpoint: inspect the final `528bac6` Core Race
  and ARM64 results. If all three workflows are green and branch/PR ancestry
  still matches, push the saved stack normally to the same PR, then inspect
  every exact-new-head workflow before another push. Continue the SC-024
  endpoint/delivery audit while CI runs.

## Qualification batch checkpoint — 2026-11-15 · StageAudio ACL error fences

- Started at local `646e926b9102a01af75985f5b3809a4647e5f4cc`, seven commits
  ahead of remote/origin/OPEN Draft PR #534 at
  `528bac6b912f500bdeef85e1dabd50a15e3faa71`. The prior Core run
  `43694919781` completed successfully: Go 1.26 and Go 1.27 tests, Race,
  installed-copy rollback, Browser Cue Delivery, Vet, and Linux ARM64 CGo-free
  product builds passed. Companion Core `43694919744` and Camera Relay
  `43694919806` also passed. No push occurred during that gate.
- Red-first SC-024 tests reproduced three StageAudio response-boundary gaps
  using temporary SQLite only: GET status returned 503 after Project-view
  revocation during a failed session-state read; POST command returned 503
  after `audio.control` revocation at the same failure boundary; and a scoped
  command with no Project grant returned `STAGEAUDIO_NOT_CONFIGURED` before
  evaluating `audio.control`. The routes now recheck the live session and
  Project permissions before these service errors. The revoked-command test
  confirms zero calls to the fake local sidecar.
- Commits: `88698f6` adds the error fences and regressions; `0ccba79` updates
  the SC-024 qualification record and completion matrix. The StageAudio and
  unified-ACL focused group passed normally and under Race. Full
  `go test ./internal/httpapi -count=1` passed in 80.070 s; `go vet
  ./internal/httpapi` and `git diff --check` passed. No sidecar process, audio
  output, stage device, production service/data, or production ACL was used.
- Next: add this checkpoint, then verify local ancestry, clean worktree,
  `git ls-remote`, and PR #534 still point to `528bac6`. With the old-head CI
  gate already green, push the saved fast-forward stack normally to the same
  branch/PR and inspect every exact-new-head workflow before another push.
  Continue the endpoint-to-policy and asynchronous-delivery audit; keep
  Production ACL disabled and keep the Pi performance report historical until
  an isolated Raspberry Pi 4 lab is available.

## Qualification batch checkpoint — 2026-11-15 · StageAudio SHOW response fence

- PR #534 and `origin/integration/stagecore-rc` are at
  `daae16d4c3806e7ac6b7deb1927181ba2abf26b5`. Two tested commits are saved
  locally on top: `75ca868` (live grant recheck before the StageAudio SHOW-lock
  response) and `149db97` (SC-024/matrix evidence). No push has been made for
  this follow-up batch.
- A red-first temporary-SQLite test revoked `audio.control` after the command
  read an active SHOW session but before its 423 response. Before the fix, the
  revoked operator still received `STAGEAUDIO_SHOW_LOCKED`. The handler now
  rechecks the session and Project grant before responses derived from the
  runtime state; the test asserts the scoped 404 and zero local-sidecar calls.
  The broader StageAudio regressions also cover GET-status errors, command
  session-read errors and the unconfigured-sidecar response without a grant.
- Focused StageAudio and unified-ACL tests pass normally and under Race. Full
  `go test ./internal/httpapi -count=1` passed in 80.185 s; `go vet
  ./internal/httpapi` and `git diff --check` pass. All fixtures use temporary
  SQLite and a fake sidecar; no audio playback, device, service or production
  ACL is involved.
- Exact `daae16d` CI is still running: Core Go 1.27 passed, Core Go 1.26 is
  in progress; Companion Core and Camera Relay passed. Do not push the two
  follow-up commits until Core Go 1.26 completes and the entire exact-head
  gate is green. Then verify remote/PR ancestry again, push normally to the
  same branch, and inspect all exact-new-head workflows. Continue the SC-024
  whole-server and asynchronous-delivery audit; the Pi 4 current-head
  performance rerun still requires an isolated Pi 4 lab.

## Qualification batch checkpoint — 2026-11-15 · F-025 Vault-status delivery

- The current PR #534 / remote head remains
  `daae16d4c3806e7ac6b7deb1927181ba2abf26b5`. Local follow-up now includes
  StageAudio commits `75ca868`, `149db97`, and `24fa3f9`, plus F-025 commits
  `578e92b` and `2c16072`; these have not been pushed.
- Red-first `TestExecutionEnvironmentVaultStatusSuppressesStoreErrorAfterGrantRevocation`
  creates a content-bound Vault asset in a temporary database, drops only that
  test database's `vault_objects` table, and revokes `media.read` at the Store
  error boundary. Before the fix the scoped GET returned HTTP 503 after
  revocation. It now rechecks the live browser session and Project grant before
  returning Vault-status Store errors.
- Focused Vault-status/Capture tests passed normally and under Race. Full
  `go test ./internal/httpapi -count=1` passed in 80.854 s; `go vet
  ./internal/httpapi` and `git diff --check` passed. No production Vault/data,
  real device, external output or ACL activation was used.
- The exact `daae16d` gate remains incomplete: Core Go 1.27, Companion Core,
  and Camera Relay passed; Core Go 1.26 Race is still in progress. Do not push
  until Go 1.26 and the complete old-head gate finish. Then verify branch/PR
  ancestry and push the saved fast-forward commits normally to the same PR;
  inspect all exact-new-head CI before any next push. Continue auditing scoped
  read-error and asynchronous-delivery paths. Current-head Pi 4 performance
  remains unqualified pending an isolated Pi 4 lab; keep Production ACL off.

## Resume checkpoint — 2026-11-15 · qualification endpoint and linked PR review

- Read-only SSH configuration confirms `stagecore-qualification` uses a
  hostname separate from the `stagecore-pi` production alias. A new
  `BatchMode=yes`, 8-second, read-only identity attempt timed out during SSH
  banner exchange (`Connection to UNKNOWN port 65535 timed out`); no remote
  shell or command ran. The current Pi 4 performance gate therefore remains
  open, and the performance report now records both failure modes. Do not infer
  Pi 4 P95/P99/CPU/RAM from M3 or cross-built ARM64 runs.
- Related Draft PRs are still OPEN at their previously verified heads and
  checks: TabletPlayer #36 `91613da` (1 check), ESP32 DMX Lighting #39
  `3ad7749` (6 checks), ESP32 Camera #20 `455b066` (hardware-probe CI),
  StageLaser #56 `b14a578` and #57 `c4ff10b` (firmware no-actuation plus host
  contract). No linked repository was edited. Do not inspect or modify
  `pi-backups`.
- Main PR #534 remains at `daae16d4c3806e7ac6b7deb1927181ba2abf26b5`.
  Its Companion Core and Camera Relay workflows passed, and Core Go 1.27
  passed; Core Go 1.26 Race is still in progress. The local StageAudio and
  execution-environment follow-up stack is documented immediately above and
  is not pushed. Next: wait for the exact `daae16d` Core gate, then verify
  branch ancestry and push the saved fast-forward stack normally if it is
  green; inspect all newly triggered checks before the next push.

## Qualification batch checkpoint — 2026-11-15 · F-025 snapshot read fences

- Local HEAD is `f2db1706620935bfaa957c90b76fb0eb60992c0c`, nine commits
  ahead of `origin/integration/stagecore-rc` and PR #534 at
  `daae16d4c3806e7ac6b7deb1927181ba2abf26b5`. The stack contains the
  StageAudio error/SHOW fences, F-025 Vault-status and snapshot-read fences,
  and architecture/checkpoint updates. No changes were pushed after `daae16d`.
- The red-first snapshot-list test revokes `media.read` at a failed snapshot
  Store read in temporary SQLite; before the fix, the operator received HTTP
  503. Snapshot list/detail/diff Store and lookup errors now share a live
  session/Project-grant recheck before the mapped error response. Focused
  snapshot/Vault tests pass normally and under Race; full
  `go test ./internal/httpapi -count=1` passed in 80.750 s; `go vet
  ./internal/httpapi` and `git diff --check` pass.
- Related open PRs remain green at: TabletPlayer #36 `91613da`, DMX Lighting
  #39 `3ad7749` (6 checks), ESP32 Camera #20 `455b066`, StageLaser #56
  `b14a578` and #57 `c4ff10b`. No linked checkout was edited.
- Exact remote `daae16d` gate: Core Go 1.27, Companion Core and Camera Relay
  passed; Core Go 1.26 Race is still in progress. Do not push the local stack
  until Go 1.26 completes and the entire exact-head gate is green. Then verify
  PR/remote ancestry and worktree, push normally to the existing branch, and
  inspect every exact-new-head check before another push. The latest
  `stagecore-qualification` SSH attempt timed out during banner exchange; no
  Pi host identity or remote command was established. Current-head Pi 4
  performance remains unqualified, and Production ACL remains disabled.

## Push checkpoint — 2026-11-15 · StageAudio and F-025 read-error fences

- A normal fast-forward push moved `integration/stagecore-rc` and OPEN/Draft
  PR #534 from `daae16d4c3806e7ac6b7deb1927181ba2abf26b5` to
  `f1e6a021c7ea47c9d4a6c219f2213b9d8fbdb77d`. Local HEAD, `origin`, `git
  ls-remote`, and PR head match. Ten saved commits were pushed; no merge,
  force-push, branch or PR creation occurred.
- The exact previous-head gate was green: Core Go 1.26 (including Race), Core
  Go 1.27, Companion Core, and Camera Relay all passed at `daae16d`.
  New exact-head checks are Core `43700059257`, Companion Core
  `43700059241`, and Camera Relay `43700059280`; they were pending when this
  checkpoint was written. Do not push another change until all three finish.
- The pushed software batch adds four StageAudio live-grant response fences
  and F-025 Vault-status/snapshot-read fences, with red-first SQLite-only
  regressions. Focused regressions pass normally and under Race; full
  `internal/httpapi` passes in 80.750 s; `go vet` and `git diff --check` pass.
  No production service/data/Vault/ACL, sidecar process, audio output, stage
  device, firmware or hardware was touched.
- Related open PR heads remain green: TabletPlayer #36 `91613da`, DMX Lighting
  #39 `3ad7749`, ESP32 Camera #20 `455b066`, StageLaser #56 `b14a578` and #57
  `c4ff10b`. The separate `stagecore-qualification` SSH endpoint timed out
  before banner completion; no Pi identity or command ran, so current-head Pi
  performance remains unqualified. Continue the endpoint-to-policy audit
  while the three exact-new-head workflows run; keep Production ACL disabled.

## Qualification batch checkpoint — 2026-11-15 · F-025 environment-read error fences

- Local HEAD is `ae286096df522fc9ee867473e9bd5dfe22d43545`, two commits ahead
  of `origin/integration/stagecore-rc`, `git ls-remote`, and OPEN/Draft PR #534
  at `f1e6a021c7ea47c9d4a6c219f2213b9d8fbdb77d`. `6300c6b` adds live grant
  rechecks before execution-environment list and rebuild-plan Store failures;
  `ae28609` records the F-025 qualification and matrix update. Working tree was
  clean at checkpoint creation.
- Red-first temporary-SQLite regressions for the environment-list and
  rebuild-plan GET routes each returned HTTP 503 after `project.view` was
  revoked. Both now return the scoped 404 without exposing Store failure
  details. Focused environment-list, snapshot-list, Vault-status and
  rebuild-plan tests passed normally and under Race. Full
  `go test ./internal/httpapi -count=1` passed in 80.720 s; `go vet
  ./internal/httpapi` and `git diff --check` passed. Fixtures used only
  disposable database tables; Production ACL remains disabled.
- Exact pushed-head checks for `f1e6a02`: Companion Core `43700059241` and
  Camera Relay `43700059280` passed. Core Go 1.27 passed; Core Go 1.26 Race
  (`43700059257`) remains in progress, with Linux ARM64 CGo-free builds pending.
  Do not push local commits until that run and all exact-head checks complete.
- First action on resume: inspect the final Go 1.26 Race/ARM64 results. If the
  full old-head gate is green and PR/remote ancestry still matches, push the
  saved fast-forward stack normally to the same branch and PR, then inspect all
  exact-new-head workflows. Continue the SC-024 asynchronous delivery and
  remaining endpoint-to-policy review while CI runs. Pi 4 current-head
  performance qualification still requires the isolated Pi lab; the
  `stagecore-qualification` SSH endpoint timed out before banner completion.
  Do not use the `stagecore-pi` production host, change production services or
  data, send real-device commands, or enable Project ACL in production.

## Qualification batch checkpoint — 2026-11-15 · F-025 operation dispatch fence

- Local HEAD before this checkpoint is `c0cb187ae3163711145c23c97c53f167a5f80958`,
  six commits ahead of OPEN/Draft PR #534 and `origin/integration/stagecore-rc`
  at `f1e6a021c7ea47c9d4a6c219f2213b9d8fbdb77d`. The saved stack includes
  execution-environment list/rebuild-plan read fences and a bounded Companion
  operation dispatch fence, with qualification and matrix updates. No push has
  been made after `f1e6a02`.
- `TestExecutionEnvironmentOperationSuppressesStoreErrorAfterGrantRevocation`
  first reproduced HTTP 503 after `RehearsalControl` and `MediaRead` were
  revoked during the Manifest lookup. The handler now checks the live session
  and both route-scoped grants before returning that Store error; the regression
  verifies the revoked operator gets a scoped 404 and the fake runtime is called
  zero times. Normal and Race focused tests pass.
- Full `go test ./internal/httpapi -count=1` passed in 80.740 s after this
  operation change; `go vet ./internal/httpapi` and `git diff --check` pass.
  No runtime output is connected; no device, production service/data or
  production ACL was used.
- Exact pushed-head checks at `f1e6a02`: Companion Core `43700059241` and
  Camera Relay `43700059280` passed; Core Go 1.27 passed; Core Go 1.26 Race
  (`43700059257`) remains in progress. Do not push until Core and the entire
  exact-head gate finish. Afterward verify local/remote/PR ancestry, push only
  by normal fast-forward to the existing PR, then inspect every new-head check.
- Next independent work: continue the SC-024 asynchronous delivery and
  endpoint-to-permission audit while Core runs. Current-head Raspberry Pi 4
  performance remains pending an isolated Pi environment; the qualification
  SSH endpoint timed out before banner completion. Production services/data,
  the production Pi host, original Vault/show, real device commands, firmware,
  force-push, merge, and Production ACL activation remain off limits.

## Qualification batch checkpoint — 2026-11-15 · Capsule review session fences

- Local HEAD before this checkpoint is
  `87762882e59a996c395f6af30c69b36e946ceaaf`, nine commits ahead of the OPEN/Draft
  PR #534 remote `f1e6a021c7ea47c9d4a6c219f2213b9d8fbdb77d`. The saved local
  work includes F-025 read error fences, Companion operation pre-dispatch
  rechecks, and Owner Capsule import-review session rechecks; all changes remain
  on the existing branch. No push occurred after `f1e6a02`.
- Red-first `TestOwnerCapsuleImportReviewSuppressesErrorsAfterSessionRevocation`
  reproduced an inventory HTTP 503 after the Owner session was logged out.
  Inventory, safe-path, verification, PlanImport and plan-mismatch errors now
  reauthenticate before returning details. The test also revokes the session
  during inventory and capsule-path failures; a PlanImport failure after
  logout is hidden as `SESSION_INVALID`. This is an opt-in, read-only review;
  no capsule materialization occurs.
- Owner Capsule review tests pass normally and under Race; `go vet
  ./internal/httpapi` and `git diff --check` pass. Full
  `go test ./internal/httpapi -count=1` passed in 81.241 s with the error fences;
  the added Owner-session regressions then passed focused normal/Race runs. All
  paths and test data are under temporary roots; no production Vault/show,
  service, device or ACL was used.
- Exact pushed-head `f1e6a02`: Companion Core `43700059241` and Camera Relay
  `43700059280` passed; Core Go 1.27 passed; Core Go 1.26 Race remains in
  progress in run `43700059257`. Do not push the nine-commit local stack until
  that exact-head workflow and all its jobs finish; then recheck ancestry and
  PR head before a normal fast-forward push to PR #534. Inspect all new-head
  workflows afterward.
- Next independent work: continue the SC-024 asynchronous/endpoint audit or
  close a concrete SC-007/SC-008 browser authoring gap while the old-head CI
  runs. Keep current-head Pi 4 performance pending the isolated Pi lab, and
  Production ACL disabled.

## Push checkpoint — 2026-11-15 · F-025 scoped read response fences

- After all exact-head checks passed at `f1e6a021c7ea47c9d4a6c219f2213b9d8fbdb77d`
  (Core Go 1.26/Race and Go 1.27, Companion Core, Camera Relay), a normal
  fast-forward push advanced the existing `integration/stagecore-rc` branch
  and OPEN/Draft PR #534 to `c11e29b5a98f69d42812b2b597619a8093043e15`. No new
  branch/PR, merge or force push occurred.
- New exact-head runs are active at `c11e29b`: Core `43702159421`, Companion
  Core `43702159473`, and Camera Relay `43702159464`. Local changes remain
  unpushed; wait for all three before another push.
- Local HEAD before this checkpoint is
  `30447695432df884b3afeb78ac5b1ec8262862b0`, nine commits ahead of the new
  remote. After the first four-commit environment-read batch, two more tested
  batches are saved: bounded Companion operation error fencing; then Owner
  Capsule import and Capsule library inventory session fences. Push those as
  small groups only after each exact-head CI gate completes.
- Capsule inventory regression `TestShowCapsuleInventorySuppressesStorageErrorAfterOwnerSessionRevocation`
  first returned HTTP 503 after the Owner was logged out during a temporary-root
  read error. The library now rechecks the live Owner session before its
  inventory error response. Owner Capsule review error responses also
  reauthenticate at inventory, safe-path, verification and planning boundaries.
- Full `go test ./internal/httpapi -count=1` passed in 81.251 s after the
  current Capsule error fences. Focused Capsule tests pass normally and under
  Race, `go vet ./internal/httpapi` and `git diff --check` pass. No production
  data/service, Vault/show, capsule materialization, physical device or ACL
  activation was used.
- Continue endpoint and asynchronous-delivery review while CI runs. Current
  Pi 4 performance qualification still requires the isolated lab; the separate
  qualification SSH endpoint timed out before its banner, and `stagecore-pi`
  remains a production host. Keep Production ACL disabled.

## Qualification batch checkpoint — 2026-11-15 · Tablet and StageLaser inventory errors

- Local HEAD before this checkpoint is
  `f42e27b926085334f53001071e4048cc1c6286c1`, twelve commits ahead of OPEN/Draft
  PR #534 and the remote `c11e29b5a98f69d42812b2b597619a8093043e15`. No push
  has occurred after `c11e29b`.
- `GET .../tablet-controller` returned HTTP 500 after the `project.view` grant
  was revoked while its repository read failed. `GET .../stagelaser-controller`
  returned HTTP 503 at the same boundary. Red-first regressions
  `TestTabletControllerListSuppressesStoreErrorAfterProjectGrantRevocation`
  and `TestStageLaserControllerSuppressesStoreErrorAfterProjectGrantRevocation`
  use temporary SQLite, drop only `stage_devices`, and revoke the grant before
  error delivery. Both routes now recheck the live session and Project grant
  and return a scoped 404. These routes only list simulated/test device rows;
  no real stage device is contacted.
- The focused two-route tests pass normally and under Race; `go vet
  ./internal/httpapi` and `git diff --check` pass. Full
  `go test ./internal/httpapi -count=1` passed in 81.994 s before a final
  identical-patch restoration; focused normal/Race tests passed afterward.
- Exact pushed-head `c11e29b` checks: Companion Core `43702159473` and Camera
  Relay `43702159464` passed. Core Go 1.27 passed; Core Go 1.26 Race
  `43702159421` remains in progress. Do not push local commits until that run
  and all exact-head checks complete. Then check local/remote/PR ancestry and
  push only a small fast-forward batch to the same PR; inspect every new-head
  workflow before another push.
- Continue the SC-024 endpoint and asynchronous-delivery review while CI runs.
  Current-head Pi 4 performance still requires the isolated lab; the separate
  qualification SSH endpoint timed out before its banner. Keep production
  services/data, the production Pi host, physical outputs and Production ACL
  untouched.

## Qualification batch checkpoint — 2026-11-15 · Capsule export scan fence

- Starting point: local `integration/stagecore-rc` HEAD `a71fefe396d77909e4b71b5d309d85313883fe18`, 13 commits ahead of remote/PR #534 at
  `c11e29b5a98f69d42812b2b597619a8093043e15`; no local edits to the scoped
  export files were present before this batch. Core run `43702159421` has Go
  1.27 passed and Go 1.26 Race still in progress; Companion `43702159473` and
  Camera Relay `43702159464` passed. Do not push until all exact-head checks
  finish and local/remote/PR ancestry is rechecked.
- Red-first `TestScopedCapsuleExportListSuppressesScanErrorAfterGrantRevocation`
  first observed HTTP 503 after the live `capsule.export` grant was revoked at
  a temporary-root scan error. The opt-in listing now reauthenticates the
  exact session and checks the current Project grant before returning either
  filesystem failure details or entries. The regression requires the scoped
  404 and confirms the unavailable response does not escape.
- The focused regression and sibling scoped Capsule export tests pass normally
  and under Race; `go vet ./internal/httpapi`, `git diff --check`, and full
  `go test ./internal/httpapi -count=1` pass (82.137 s). The private test seam
  is unavailable through the public option. No Capsule content was changed
  and no export/restore or hardware operation occurred.
- Qualification/matrix documentation and the fix are saved in local commit
  `1ae431c6f35af8812f84f4d1eae24582a2499663`; see the following device-read
  checkpoint for newer local HEAD and test/CI status. Continue endpoint and
  session-fence review while c11e29b CI completes. After that gate, push in a
  normal fast-forward batch to the same PR and inspect all exact-head workflows.
  Pi performance, production acceptance, production services/data, real
  devices and Production ACL remain gated.

## Qualification batch checkpoint — 2026-11-15 · Owner device-read error fences

- The Capsule scan fence and its qualification/matrix update are saved in
  commit `1ae431c6f35af8812f84f4d1eae24582a2499663`; it is local and unpushed.
  That change passed the focused normal/Race tests, `go vet ./internal/httpapi`,
  and full HTTP API tests (82.137 s).
- Red-first `TestStageDeviceTransferStatusSuppressesAssignmentErrorAfterOwnerRevocation`
  reproduced a 503 after Owner logout at a temporary assignment-store failure.
  `TestNetworkCockpitSuppressesStoreErrorAfterOwnerRevocation` reproduced a
  500 at a temporary Network Cockpit read failure. Owner session/role checks
  now run before returning either error. The transfer-status Epoch ACK error
  also rechecks the Owner session; a dedicated regression covers that branch.
- The four Owner read/error regressions pass normally and under Race;
  `go vet ./internal/httpapi`, `git diff --check`, and full
  `go test ./internal/httpapi -count=1` pass (82.613 s). Only temporary SQLite
  tables were dropped; no network/device probe or production data was used.
- Owner-read code, regressions and qualification/matrix updates are saved in
  local commit `670de92a31c0ab2bb19a0f820aa8d6c669b66d77`. It is unpushed;
  remote and PR #534 remain `c11e29b5a98f69d42812b2b597619a8093043e15`.
  Exact-head Core run `43702159421` still has Go 1.26 Race in progress; Go
  1.27, Companion `43702159473`, and Camera Relay `43702159464` pass. No push
  until that run completes. Then continue the scoped device and asynchronous
  boundary audit, keep pushes fast-forward-only, and inspect each new-head CI.

## Qualification batch checkpoint — 2026-11-15 · Owner inventory errors

- Starting local HEAD: `c8d77a5423f97e6d8868feb6090dc4e07c33ff36`, 16 commits
  ahead of remote/PR #534 at `c11e29b5a98f69d42812b2b597619a8093043e15`.
  Exact-head Core `43702159421` remains in Go 1.26 Race; Go 1.27, Companion
  `43702159473`, and Camera Relay `43702159464` pass. No push while Core Race
  is active.
- Red-first `TestStageDeviceGlobalInventorySuppressesListErrorAfterOwnerRevocation`
  reproduced HTTP 503 after the Owner session was revoked at the repository
  error boundary for both `inventory` and `unassigned`. Those Owner-only
  handlers now recheck session/role before returning list and per-device
  assignment errors. The regression requires `SESSION_INVALID` and uses only
  temporary SQLite.
- Focused normal/Race coverage includes both inventory routes, transfer status
  assignment/epoch-ACK failures, Network Cockpit failures, and the prior Owner
  success-delivery fence. `go vet ./internal/httpapi` and `git diff --check`
  pass. Full `go test ./internal/httpapi -count=1` passes (82.984 s).
- Owner inventory code, regressions and qualification/matrix updates are saved
  in local commit `b1b40e5108631f119ee66299cada56fd325b5610`, on top of
  `c8d77a5`. It is unpushed. Continue the route/error and asynchronous safety
  audit. Preserve the same branch/PR and wait for all exact-head CI before any
  push. Production ACL, services/data, physical devices and outputs stay off.

## Qualification batch checkpoint — 2026-11-15 · Direct device metadata errors

- Starting local HEAD: `0376f3879e83a84f3c2b250f79e871c4691cd265`, 18 commits
  ahead of remote/PR #534 at `c11e29b5a98f69d42812b2b597619a8093043e15`.
  Exact-head Core `43702159421` remains in Go 1.26 Race; Go 1.27, Companion
  `43702159473`, and Camera Relay `43702159464` pass. No push while Core Race
  is active.
- Red-first `TestStageDeviceMetadataSuppressesStoreErrorAfterOwnerSessionRevocation`
  reproduced stale 404/503 results after Owner logout for direct device and
  assignment reads. The handlers now recheck session, role and scope before
  returning those repository errors. The test uses only temporary SQLite and
  requires `SESSION_INVALID` after logout.
- Focused normal/Race tests, `go vet ./internal/httpapi`, and full
  `go test ./internal/httpapi -count=1` pass (83.553 s). Matrix and
  qualification docs are updated.
- The direct metadata fix and regressions are saved in local commit
  `915fa899698ff9619e10865238c62cb5f414f1b5`, on top of `0376f38`. It is
  unpushed; local is now 19 commits ahead of remote `c11e29b`. Continue the
  endpoint/error audit, and wait for exact-head CI before normal fast-forward
  pushes to the existing PR. Production ACL/services/data and physical device
  access remain prohibited.
- Read-only recheck of the related open PRs found the same heads and green
  software checks: TabletPlayer #36 `91613da`, DMX Lighting #39 `3ad7749`,
  Camera #20 `455b066`, StageLaser #56 `b14a578`, and StageLaser #57
  `c4ff10b`. No related checkout was modified. Companion, StageAudio, OSC,
  VDMX, Ableton and QLC+ are qualified through the existing source/tests and
  Core integration; their dedicated endpoint/physical acceptance remains
  separate where the matrix says so.

## Push checkpoint — 2026-11-15 · first F-025 operation-error batch

- All checks on pushed HEAD `c11e29b5a98f69d42812b2b597619a8093043e15` passed:
  Core Go 1.26 Race and Go 1.27, Companion Core, and Camera Relay. PR #534 was
  still OPEN/Draft on `integration/stagecore-rc`.
- A normal fast-forward push advanced only the first three commits after c11:
  `bce462c` operation read-error fence, `c0cb187` qualification, and `4ff6ed3`
  checkpoint. PR/remote HEAD is now `4ff6ed3f018c1589b5ed49a9972e423eddadddba`.
  No force push, new branch/PR, or merge occurred.
- Exact new-head workflows are active: Core `43705363041` (Go 1.26 and 1.27),
  Companion `43705363040`, and Camera Relay `43705363044`. All were pending on
  the last read. Do not push again until every exact-head job completes.
- Local code HEAD before this uncommitted Project Access batch is
  `a7cf2dbe6a64ec6cb648582dd02e8c785eae4954`, 18 commits ahead of remote
  `4ff6ed3`. The red-first
  `TestProjectAccessSuppressesPreflightErrorsAfterOwnerSessionRevocation`
  reproduced HTTP 503 after Owner logout on both grant-read and active-session
  preflight failures; handlers now check the exact Owner session and
  `user.manage` before returning these errors. Focused normal/Race tests and
  `go vet ./internal/httpapi` pass. Full `go test ./internal/httpapi -count=1`
  passes (84.061 s); matrix and qualification docs are updated.
- Project Access handler/tests and qualification/matrix changes are saved in
  local commit `3df576b138110b9da173b4f10b62a70b43b1eaca`. It remains
  unpushed; local is now 19 commits ahead of remote `4ff6ed3`. Continue the
  ACL/read-error audit while the new-head CI runs. After that gate, push only
  the next small fast-forward group to the same PR and inspect its CI. Keep
  Production ACL disabled and production services/data and physical outputs
  untouched.

## Resume update — 2026-11-15 · Project editor read-error fences

- Branch `integration/stagecore-rc`, OPEN/Draft PR #534. The last verified
  pushed Local/Remote/PR HEAD remains
  `4ff6ed3f018c1589b5ed49a9972e423eddadddba`. Code commit
  `261e071eda5da02516c350bcc8984c31bd1387fd` was 21 commits ahead; this
  handoff update is documentation-only. Read `git rev-parse HEAD` for the
  current local HEAD. The worktree was clean after the code commit. Nothing
  new has been pushed.
- Exact-head Core run `43705363041`: Go 1.27 passed; Go 1.26 remains in
  progress. Companion run `43705363040` (macOS replacement/media and Swift
  CompanionCore) passed; Camera Relay run `43705363044` passed. Wait for Go
  1.26 and recheck all exact-head runs before any push. The next normal
  fast-forward push target is the 4-commit Capsule review group ending at
  `3c53554`; do not push a later local commit in its place.
- Commit `261e071` fences project editor read errors. Red-first temporary
  SQLite tests reproduced stale `PROJECTS_UNAVAILABLE` for all four
  Draft-history endpoints and `CONFIGURATION_UNAVAILABLE` for Project
  Configuration after project grant revocation. The handlers now recheck the
  exact session and live grant before returning Store errors.
- Verification passed: focused normal tests for both regressions; their
  focused `go test -race`; `go vet ./internal/httpapi`; `git diff --check`; and
  full `go test ./internal/httpapi -count=1` (85.563 s). Changes are limited to
  the opt-in qualification API and qualification docs; production ACL remains
  disabled.
- Next local audit: private Cue editor recovery, checkpoint and action-preset
  read failures, then continue route-by-route authorization and asynchronous
  dispatch review while CI runs. Preserve the no-production/no-real-device
  constraints. Pi current-head performance still requires a verified isolated
  Raspberry Pi 4 lab; never use the production-like `stagecore-pi` host.

## Resume update — 2026-11-15 · private Cue editor read fences

- Remote/PR #534 remains at `4ff6ed3f018c1589b5ed49a9972e423eddadddba` on
  `integration/stagecore-rc`. Code commit
  `b159c2a9daf97bee5fee52c43625246ba9574a95` is the current local code tip,
  23 commits ahead; its qualification documentation is committed. The
  worktree is clean and nothing from the local stack has been pushed.
- Core run `43705363041` still has Go 1.26 Race in progress; Go 1.27 passed.
  Companion run `43705363040` and Camera Relay run `43705363044` passed. Do not
  push until the exact 4ff head's remaining Core job finishes and all runs are
  green. Afterward, verify PR/remote ancestry and push only the next normal
  fast-forward group ending at `3c53554`; then wait for its exact-head CI.
- Commit `b159c2a` fences Store errors from Cue Editor Recovery, Checkpoint
  and Action Preset reads. Red-first SQLite tests reproduced stale 503
  responses on all three after live grant revocation. Preflight Project/Cue/
  Revision errors are also fenced. Verification: focused normal tests for the
  three private editor reads and the preceding five read-fence cases; focused
  Race test covering all cases; `go vet ./internal/httpapi`;
  `git diff --check`; and full `go test ./internal/httpapi -count=1` (84.780 s).
- The campaign remains in qualification mode. Production ACL is disabled;
  no production services/data or physical device were used. Continue auditing
  authorization error and asynchronous delivery boundaries while the existing
  Core workflow runs. Pi performance qualification still requires the verified
  isolated Raspberry Pi 4 lab.

## Resume update — 2026-11-15 · Project detail and Dashboard read fences

- Pushed PR/remote HEAD remains
  `4ff6ed3f018c1589b5ed49a9972e423eddadddba`; local code commit
  `c9cb7b32f4ca270b9509d4c86e8f3c6bd9a0e669` is 25 commits ahead. This
  checkpoint is documentation-only and the worktree was clean after the code
  commit. No push has occurred.
- Exact-head Core run `43705363041` still has Go 1.26 Race in progress; Go
  1.27 passed. Companion `43705363040` and Camera Relay `43705363044` passed.
  Keep waiting; do not push while that Core job runs. Once it completes, verify
  the PR/remote HEAD and all checks, then push the next fast-forward batch only
  through `3c53554` and wait for its exact-head workflows.
- `c9cb7b3` closes stale errors from project detail and Dashboard reads.
  Red-first temporary-SQLite tests removed `project_revisions` and
  `runtime_snapshots` and reproduced `REVISION_UNAVAILABLE` and
  `DASHBOARD_UNAVAILABLE` after grant revocation. Both responses now recheck
  the exact session and current Project grant first.
- Focused normal/Race tests, `go vet ./internal/httpapi`, `git diff --check`,
  and full `go test ./internal/httpapi -count=1` passed (87.279 s). The tests
  restore SQLite foreign-key enforcement after their isolated schema fault.
  Production ACL remains off; no production service or hardware was used.
- Continue the whole-server response/error and asynchronous dispatch audit.
  After the CI gate, retain the current branch and Draft PR. The isolated Pi 4
  performance lab is still required for hardware performance qualification.

## Resume update — 2026-11-15 · Cue list and validation read fences

- The existing branch was normally fast-forward pushed through the four-commit
  Capsule review group ending at `3c535543c0242d349a94f23948a1ad18747468c0`.
  PR #534 remains OPEN/Draft. The pushed HEAD and `origin` match at `3c53554`.
  Local code tip `7fe84d63c25a650cf679c7e1d80f92289aa58818` is 23 commits
  ahead of that remote; the worktree was clean after the code commit.
- Exact-head Core run `43707820414`: Go 1.27 passed; Go 1.26 remains in
  progress. Companion `43707820363` and Camera Relay `43707820418` passed.
  **Do not push until Go 1.26 finishes and all exact-head checks are green.**
- `7fe84d6` fences Project read errors from Cue listing and publish validation.
  Red-first tests dropped `cues` in temporary SQLite and reproduced
  `CUES_UNAVAILABLE` and `VALIDATION_UNAVAILABLE` after `ProjectView` was
  revoked. Both now return the scoped Project denial.
- Focused normal/Race regressions, `go vet ./internal/httpapi`,
  `git diff --check`, and full `go test ./internal/httpapi -count=1` passed
  (87.369 s). Production ACL remains disabled; no Cue data or hardware was
  touched.
- Next review: Session Memory list/detail and Project Notes read errors, then
  continue scoped route and asynchronous delivery coverage. The Pi 4 lab and
  production release gates remain unchanged.

## Resume update — 2026-11-15 · Session Memory and Notes read fences

- Remote/PR #534 remains at `3c535543c0242d349a94f23948a1ad18747468c0`.
  Code commit `e1e77362690b873dd8bde9fbbb54e4537d614a87` is 25 commits ahead;
  the worktree was clean after commit. No later push has occurred.
- Exact-head run `43707820414`: Go 1.27 passed; Go 1.26 remains in progress.
  Companion `43707820363` and Camera Relay `43707820418` passed. Wait for the
  Go 1.26 Race job and recheck every exact-head check before pushing.
- `e1e7736` fences Project Session Memory list/detail and Project Notes GET
  errors. Red-first temporary-SQLite regressions for all three returned
  `PROJECTS_UNAVAILABLE` after grant revocation before the fix; they now return
  the scoped denial. Normal/Race regressions, `go vet ./internal/httpapi`,
  diff checks and full HTTP API tests (87.704 s) pass.
- Next inspect the four Project Note mutation response boundaries (create,
  update, resolve, reopen) for result delivery after revocation. Keep this
  local while the 3c53554 Core workflow is active; no ACL production cutover,
  hardware command or production data operation is allowed.

## Qualification batch checkpoint — 2026-11-15 · Project Note mutation delivery

- PR #534 and `origin/integration/stagecore-rc` remain at
  `3c535543c0242d349a94f23948a1ad18747468c0`; local HEAD is
  `2f3f8f5b4ed7b5fe4807888c3ed1455dc0375c65`, 27 commits ahead. The working
  tree is clean. No new push has occurred.
- Exact-head Core run `43707820414` still has Go 1.26 pending/in progress;
  Go 1.27 passed. Companion run `43707820363` and Camera Relay run
  `43707820418` passed. Do not push until all checks for `3c53554` are green.
- `2f3f8f5` fences Project Note create/update/resolve/reopen response bodies
  after a post-mutation session or ProjectEdit revocation. Red-first SQLite
  hooks reproduced four successful responses containing Note details despite
  revocation; the corrected responses report commit uncertainty, are
  non-retryable, and suppress Note details while preserving the completed
  Store operation.
- Focused normal and Race regressions passed; `go vet ./internal/httpapi`,
  `git diff --check`, and full `go test ./internal/httpapi -count=1` passed
  (88.780 s). This is opt-in authorization qualification only; production ACL,
  services, data and hardware remain untouched.
- Next: continue the project-scoped response/error audit, add a regression
  before correcting any reproducible stale response, and maintain the exact
  old-head CI gate. Isolated Raspberry Pi 4 qualification and hardware/production
  acceptance remain separate.

## Qualification batch checkpoint — 2026-11-15 · Project Configuration lock

- PR #534 and remote branch remain at
  `3c535543c0242d349a94f23948a1ad18747468c0`; local code HEAD is
  `d43ddf6ab10d74990425c209b994fd6812ad06ce`, 29 commits ahead. The tested
  implementation is committed; qualification/matrix and this handoff update
  are being committed as the next documentation-only commit. Nothing new has
  been pushed.
- Exact-head Core run `43707820414`: Go 1.27 passed; Go 1.26 is still in the
  Race step. Companion `43707820363` and Camera Relay `43707820418` passed.
  Preserve this gate before pushing any later local commit.
- `d43ddf6` fences Project Configuration lock GET and Draft preflight errors,
  lock-state responses, and draft mutation errors before returning data after
  revocation. Red-first temporary-SQLite tests reproduced stale
  `PROJECTS_UNAVAILABLE` on GET and POST after grant revocation. The lock
  response regression also revokes `ProjectEdit` immediately before returning
  SHOW state and verifies that Session/Snapshot identifiers are suppressed.
- Verification passed: focused normal regression, focused Race tests for both
  error and lock-state response paths, `go vet ./internal/httpapi`,
  `git diff --check`, and full `go test ./internal/httpapi -count=1`
  (89.540 s). Schema faults were confined to temporary test SQLite.
- Read-only related PR review is unchanged and green: TabletPlayer #36 at
  `91613da`, DMX Lighting #39 at `3ad7749`, Camera #20 at `455b066`, and
  StageLaser #56/#57 at `b14a578`/`c4ff10b`. No related repository was edited.
- Continue the whole-server project-scoped error and response-delivery audit.
  Production ACL remains disabled; no production service/data, physical output,
  firmware or paid service was used. Current-head isolated Pi 4 performance
  qualification is still pending; historical simulation is not current Pi proof.

## Qualification batch checkpoint — 2026-11-15 · Tablet Scene read fences

- PR #534 / remote remain at `3c535543c0242d349a94f23948a1ad18747468c0`;
  local code HEAD is `1c49bae5c1a9003d5833f8a2bc8e0afacf346ebb`, 31 commits
  ahead. The code commit is clean; qualification and matrix updates plus this
  checkpoint are documentation changes being saved next. No push occurred.
- Exact-head Core `43707820414` still has Go 1.26 Race in progress; Go 1.27
  passed. Companion `43707820363` and Camera Relay `43707820418` passed. Do
  not push until the exact `3c53554` workflow set is fully green.
- `1c49bae` fences the opt-in Tablet Scene projection's Project, Revision,
  Cue, Alias and device read errors with a live session/ProjectView recheck.
  The red-first `TestTabletSceneReadSuppressesStoreErrorAfterGrantRevocation`
  dropped only temporary SQLite `cues`; before the fix, the revoked response
  was followed by a second `TABLET_SCENES_UNAVAILABLE` body. The handler now
  returns immediately after the denial.
- Focused normal and Race tests, normal full `go test ./internal/httpapi
  -count=1` (92.646 s), `go vet ./internal/httpapi`, and diff check passed.
  Tablet behavior remains simulated and the opt-in ACL is not assembled in
  production.
- Next: apply the same red-first review to remaining project-scoped lighting
  controller GET errors and other read projections; continue waiting on Core
  1.26 CI. No production operations, hardware output or firmware changes.

## Push checkpoint — 2026-11-15 · Capsule inventory session fence

- The prior exact-head gate on `3c535543c0242d349a94f23948a1ad18747468c0`
  completed green: Core Go 1.26/1.27, Companion, and Camera Relay all passed.
- A normal fast-forward push advanced PR #534 by only three commits:
  `e143f6d` Capsule inventory session fence, `3044769` qualification record,
  and `9760e9d` checkpoint. PR/remote is now
  `9760e9d6dc51eca98cb5d0a67ddf1db2c727123a` on the same branch. No merge,
  branch creation, or force push occurred.
- Exact new-head checks are pending: Core run `43710243386`, Companion run
  `43710243389`, and Camera Relay run `43710243382`. No further push until
  every check for `9760e9d` is complete and green.
- Local code HEAD is `4c1fbcd059e51c5de54320b6c2a25be0ed754536`, 29 commits
  ahead of the new remote before the current uncommitted Lighting read-error
  batch. That batch is under full HTTP API testing; focused normal/Race
  regression and `go vet` passed, and the uncommitted changes must not be
  included in a push until their full validation and checkpoint are complete.
- Production ACL remains off. Production services/data, real outputs and
  firmware remain untouched. Pi 4 current-head performance qualification is
  separate and still pending an isolated ARM64 lab.

## Qualification batch checkpoint — 2026-11-15 · Lighting read projections

- PR #534 and remote remain at
  `9760e9d6dc51eca98cb5d0a67ddf1db2c727123a`. Local code HEAD is
  `68c361b441e511ee3868a1bbbdf5bba8c7cc216f`, 31 commits ahead; this
  qualification/matrix/handoff update is being committed as documentation.
  No change has been pushed beyond `9760e9d`.
- The new exact-head workflow set is still active: Core run `43710243386`
  (Go 1.26/1.27 pending), Companion run `43710243389` (Swift passed;
  macOS replacement/media pending), and Camera Relay run `43710243382`
  (isolated ARM64 job passed). Keep the push gate closed until all checks are
  green for the same PR head.
- `68c361b` fences Project, Revision, Cue, Alias, binding and device Store
  failures from both optional Lighting Controller GET projections with a
  live session/ProjectView recheck. Red-first
  `TestLightingCueReadSuppressesStoreErrorAfterGrantRevocation` reproduced a
  second `LIGHTING_CUES_UNAVAILABLE` body following the scoped denial after a
  temporary SQLite Cue-table fault; the handler now returns immediately.
- Focused normal/Race regression, `go vet ./internal/httpapi`,
  `git diff --check`, and full `go test ./internal/httpapi -count=1`
  (92.183 s) passed. No output was dispatched.
- Continue the endpoint error/delivery audit while CI runs. Production ACL is
  disabled; production services/data, device outputs and firmware are
  untouched. Isolated current-head Raspberry Pi 4 performance remains pending.

## Qualification batch checkpoint — 2026-11-15 · Lighting Diagnostic error fence

- PR #534 / remote remains at
  `9760e9d6dc51eca98cb5d0a67ddf1db2c727123a`. Local code HEAD is
  `538fe5bc1ebc253e602ef41511a4fa185e8b0d68`, 33 commits ahead; qualification,
  matrix and handoff updates are documentation changes being saved next. No
  further push has occurred.
- Exact-head workflow run `43710243386`: Go 1.27 passed; Go 1.26 remains in
  progress. Companion `43710243389` and Camera Relay `43710243382` passed.
  Keep the push gate closed until Go 1.26 and all exact-head checks are green.
- `538fe5b` fences the software-only Lighting Diagnostic's Project read error
  before any error response is serialized. Red-first
  `TestLightingDiagnosticSuppressesProjectReadErrorAfterGrantRevocation`
  faulted the Project query with a temporary SQLite column rename and revoked
  the Viewer grant. Before the correction it wrote `PROJECT_NOT_FOUND` and
  then appended `PROJECTS_UNAVAILABLE`; now only the scoped denial is sent.
- Focused normal/Race regressions, full `go test ./internal/httpapi -count=1`
  (89.684 s), `go vet ./internal/httpapi` and `git diff --check` passed. The
  diagnostic reader does not run after the Project error; no output commands
  or production services/data were touched.
- Continue reviewing the F-025 Project/Revision preflight errors and remaining
  project-scoped read projections while CI runs. Production ACL remains off;
  current-head isolated Pi 4 performance qualification is still pending.

## Qualification batch checkpoint — 2026-11-15 · F-025 Project/Revision preflight

- PR #534 / remote remains at
  `9760e9d6dc51eca98cb5d0a67ddf1db2c727123a`. Local code HEAD is
  `636b13659a67e7bd9102e8373059a1ab486039b2`, 35 commits ahead; this
  qualification/matrix/handoff update is documentation-only and is being
  committed separately. No new push.
- Exact remote-head run `43710243386`: Go 1.27 passed, Go 1.26 still pending.
  Companion run `43710243389` and Camera Relay run `43710243382` passed. Keep
  the push gate closed until all exact-head checks are green.
- `636b136` makes `loadExecutionEnvironmentRevision` recheck the live
  qualified route grant before either Project or Revision Store errors are
  returned. Red-first `TestExecutionEnvironmentRevisionReadSuppressesStoreErrorAfterGrantRevocation`
  renames a Project column only in temporary SQLite and revokes `ProjectView`
  at the error boundary. Before the fix, the handler appended
  `EXECUTION_ENVIRONMENTS_UNAVAILABLE` after the scoped 404; now it returns
  immediately. This shared preflight covers F-025 list/snapshot/capture/
  rebuild-plan/operation entry points.
- Focused normal/Race tests, `go vet ./internal/httpapi`, diff checks, and full
  `go test ./internal/httpapi -count=1` (90.693 s) passed. No Vault or
  production data, service or device was touched.
- Next inspect remaining F-025 manifest/asset error boundaries and other
  project-scoped read projections. Production ACL remains disabled; current
  isolated Pi 4 performance qualification is still pending.

## Qualification batch checkpoint — 2026-11-15 · F-025 asset manifest read

- PR #534 and the remote branch remain at
  `9760e9d6dc51eca98cb5d0a67ddf1db2c727123a`. The latest code commit is
  `d27a6b8d14740a28299fb4e43a6b332042b41bd1`; it is 37 commits ahead of that
  remote. Qualification, matrix and this handoff update are being recorded
  separately; no push has occurred.
- Exact remote-head checks remain open: Core Go 1.26 run `43710243386` is
  still in progress; Core Go 1.27 passed. Companion run `43710243389` and
  Camera Relay run `43710243382` passed. Do not push until all checks for
  exactly `9760e9d` are complete and green.
- `d27a6b8` adds a live qualified-route grant check after an F-025 asset
  manifest read error, suppressing the Store error if authorization was
  revoked. Red-first `TestExecutionEnvironmentAssetReadSuppressesStoreErrorAfterGrantRevocation`
  faulted only the temporary SQLite manifest column and reproduced a scoped
  `PROJECT_NOT_FOUND` plus an appended `EXECUTION_ENVIRONMENT_CAPTURE_FAILED`
  response. The corrected vault-status path emits only the scoped denial;
  capture uses the same loader.
- Focused normal test passed; Race coverage passed for asset, vault-status,
  revision, list, snapshot-list, rebuild-plan and operation revocation tests;
  `go vet ./internal/httpapi` passed; full
  `go test ./internal/httpapi -count=1` passed in 92.060 s; `git diff --check`
  passed. All database fault injection used disposable SQLite. No production
  data, Vault bytes, services, or output devices were accessed.
- Next: inspect remaining manifest reads, especially rebuild-plan setup, for
  error responses that can bypass the live grant recheck; continue the broader
  Project-scoped read audit while the exact-head CI runs. Current isolated
  Pi 4 performance qualification remains pending; Mac/simulation results are
  not Pi evidence. Production ACL remains disabled and PR #534 stays Draft.

## Qualification batch checkpoint — 2026-11-15 · F-025 rebuild-plan setup

- PR #534 and remote remain at
  `9760e9d6dc51eca98cb5d0a67ddf1db2c727123a`. The latest code commit is
  `7e2de95530ef47ff2971b7f766158e75a149a5ba`, 39 commits ahead before this
  documentation commit. No push occurred.
- Exact remote-head Core Go 1.26 run `43710243386` remains in progress; Go
  1.27, both Companion checks and Camera Relay passed. Keep the push gate
  closed until the same-head workflow set completes green.
- `7e2de95` extends the shared F-025 rebuild-plan loader with a read-error
  boundary hook and live route-grant recheck for the Environment manifest
  query, covering GET, seed, update and delete. Red-first
  `TestExecutionEnvironmentRebuildPlanEnvironmentSuppressesStoreErrorAfterGrantRevocation`
  renamed only `manifest_json` in temporary SQLite and revoked `ProjectView`;
  before the change, GET returned the generic 503, and now returns only the
  scoped `PROJECT_NOT_FOUND` response.
- Focused normal and Race rebuild-plan lifecycle/revocation tests passed,
  `go vet ./internal/httpapi` passed, and full
  `go test ./internal/httpapi -count=1` passed in 92.043 s. `git diff --check`
  passed. No production SQLite, service, Vault bytes, or output device was
  used.
- Next inspect rebuild-plan source-snapshot read failures and remaining
  project-scoped Store error responses; continue while exact-head CI is
  running. Isolated Pi 4 current-head timing qualification is still pending.
  Production ACL remains disabled; PR #534 remains Draft.

## Qualification batch checkpoint — 2026-11-15 · F-025 source Snapshot read

- PR #534 and remote remain at
  `9760e9d6dc51eca98cb5d0a67ddf1db2c727123a`. Latest code commit:
  `1b9ad2f1db1201816c0807812b4c5b9027aa5c95`, 41 commits ahead before this
  documentation commit. No push occurred.
- Core Go 1.26 run `43710243386` is still active in its Race tests step
  (started `2026-11-15T11:46:42Z`); Go 1.27, both Companion checks and Camera
  Relay passed. The run is unusually long, but it remains active. Do not
  cancel or push over it.
- `1b9ad2f` fences F-025 source Snapshot read errors in the rebuild-plan seed
  route. Red-first `TestExecutionEnvironmentRebuildPlanSourceReadSuppressesStoreErrorAfterGrantRevocation`
  renamed only `snapshot_json` in temporary SQLite and revoked `ProjectEdit`
  at the read boundary; before the fix, seed returned its generic 503 after
  revocation, and now it returns only the scoped denial.
- Focused normal and Race rebuild-plan tests passed, `go vet ./internal/httpapi`
  passed, and full `go test ./internal/httpapi -count=1` passed in 92.980 s.
  Diff checks passed. Test data stayed in disposable SQLite.
- Next inspect other rebuild-plan response boundaries (especially existing
  plan conflicts and mutation results) and continue the Project-scoped error
  audit. Current isolated Pi 4 performance qualification remains pending;
  production ACL stays disabled and PR #534 stays Draft.

## Qualification batch checkpoint — 2026-11-15 · Rebuild-plan conflict delivery

- Before this documentation commit, local code HEAD is
  `3167cb248d8d2ee54f044f66eddc9d248a419379`, 43 commits ahead of remote
  `9760e9d6dc51eca98cb5d0a67ddf1db2c727123a`. The exact old-head gate is
  now green: Core Go 1.26 Race and Go 1.27, Companion Core plus macOS
  replacement/media, and Camera Relay isolated ARM64 all passed on
  `9760e9d`. A normal push is now permitted after this checkpoint is saved.
- `3167cb2` rechecks the live qualified route grant after reading an existing
  rebuild plan and before returning its conflict response with the private
  plan ID. Red-first
  `TestExecutionEnvironmentRebuildPlanConflictSuppressesPlanIDAfterGrantRevocation`
  revoked ProjectEdit at that boundary; before the fix, the 409 included
  `rebuild_plan_id`, and now the handler returns only `PROJECT_NOT_FOUND`.
- Focused normal and Race rebuild-plan tests passed, `go vet ./internal/httpapi`
  passed, and full `go test ./internal/httpapi -count=1` passed in 92.820 s.
  `git diff --check` passed. Project state was disposable SQLite only.
- Next push the validated batch normally to PR #534, record the new remote
  SHA and exact-head CI identifiers, then wait for its complete green gate
  before any later push. Continue the rebuild-plan Store-error audit and keep
  production ACL off; current-head isolated Pi 4 performance acceptance is
  still pending.

## Push checkpoint — 2026-11-15 · F-025 rebuild-plan fences

- The exact previous PR head `9760e9d6dc51eca98cb5d0a67ddf1db2c727123a`
  completed all checks green: Core Go 1.26 Race and Go 1.27, Companion Core
  and macOS replacement/media, and Camera Relay isolated ARM64.
- Normal fast-forward push updated the existing branch and Draft PR #534 to
  `0b091584d07f9324683e18954f37707b3d57941b`. No branch or PR was created;
  there was no force push or merge. Local and remote matched immediately
  after the push.
- New exact-head checks on `0b09158` are queued: Core run `43713024420`
  (Go 1.26/1.27), Companion run `43713024392` (Swift Core and macOS
  replacement/media), and Camera Relay run `43713024390` (isolated ARM64).
  Do not push again until every check for this exact SHA is complete and
  green.
- The pushed batch includes the F-025 asset-manifest, rebuild-plan setup,
  source Snapshot and existing-plan conflict response fences, with red-first
  disposable-SQLite tests. Focused normal/Race, full HTTP API, vet and diff
  checks passed locally before push.
- Continue auditing rebuild-plan read/mutation response boundaries while CI
  runs. Keep production ACL disabled, production services/data and real
  outputs untouched. Current isolated Raspberry Pi 4 timing qualification
  remains pending; historical simulation is not Pi acceptance.

## Qualification batch checkpoint — 2026-11-15 · Rebuild-plan mutation delivery

- The latest code commit is `9811aefc03cadcf177d2c19a0ec451ba98c6b0a3`;
  local code HEAD is two commits ahead of remote `0b091584d07f9324683e18954f37707b3d57941b`
  before this documentation update. The other local commit is the already
  recorded push checkpoint. No further push has occurred.
- Exact-head CI on `0b09158`: Core Go 1.26 run `43713024420` remains in
  progress; Go 1.27 passed. Companion `43713024392` Swift and macOS checks
  passed; Camera Relay `43713024390` isolated ARM64 passed. Keep the push
  gate closed until Go 1.26 and the full exact-head set are green.
- `9811aef` adds committed-effect fences before responses from rebuild-plan
  seed, update and delete. Red-first
  `TestExecutionEnvironmentRebuildPlanMutationsSuppressCommittedResultsAfterGrantRevocation`
  covers each route: on the successful paths it revokes `ProjectEdit` after
  commit, expects a non-retryable commit-uncertainty response, and verifies
  the changed/deleted plan state is actually retained in temporary SQLite.
  The earlier red run showed seed details and a successful empty delete
  response after revocation; the now-valid update case also passes.
- Focused normal/Race tests, `go vet ./internal/httpapi`, diff checks and full
  `go test ./internal/httpapi -count=1` passed (93.867 s). No production
  database, Vault data or output device was used.
- Next inspect remaining rebuild-plan read-error response branches, then
  continue the broader Project-scoped response audit. Do not push until the
  exact `0b09158` CI gate completes. Production ACL stays off; isolated Pi 4
  timing qualification is pending.

## Qualification batch checkpoint — 2026-11-15 · Existing rebuild-plan read

- Latest code commit: `286f4dbbca4f2a9520320db12ebe440a66414edc`; local code
  HEAD is four commits ahead of remote
  `0b091584d07f9324683e18954f37707b3d57941b` before this documentation
  update. No further push has occurred.
- Exact-head CI on `0b09158`: Core Go 1.26 run `43713024420` remains in
  progress; Core Go 1.27 passed. Companion run `43713024392` and Camera Relay
  run `43713024390` passed all checks. Keep the push gate closed until the
  Go 1.26 job also completes successfully.
- `286f4db` fences non-NotFound Store errors from reading an existing plan
  during seed. Red-first
  `TestExecutionEnvironmentRebuildPlanExistingReadSuppressesStoreErrorAfterGrantRevocation`
  renamed only `plan_json` in temporary SQLite, revoked `ProjectEdit` at the
  error boundary, and reproduced the generic seed 503 after revocation; the
  corrected route returns only `PROJECT_NOT_FOUND`.
- Focused normal/Race rebuild-plan regressions, `go vet ./internal/httpapi`,
  diff checks, and full `go test ./internal/httpapi -count=1` passed
  (94.441 s). Temporary SQLite only; no production data, Vault bytes or
  output device was used.
- Next review remaining seed read outcomes and mutation error delivery while
  CI runs. Do not push past `0b09158` until its exact-head gate is green.
  Production ACL remains off; isolated current-head Pi 4 qualification is
  pending.

## Qualification batch checkpoint — 2026-11-15 · Rebuild-plan source validation

- Latest code commit: `6fb98c39767e760ccaff918bf97033b6dae2c75d`; local code
  HEAD is six commits ahead of remote
  `0b091584d07f9324683e18954f37707b3d57941b` before this documentation
  update. No further push has occurred.
- Exact-head check state on `0b09158`: Core Go 1.26 Race run `43713024420`
  remains in progress; Go 1.27, Companion Swift/macOS and Camera Relay
  isolated ARM64 passed. Do not push until that Go 1.26 job completes green.
- `6fb98c3` fences the source-loaded and unsupported-result paths of rebuild
  plan seed. Red-first
  `TestExecutionEnvironmentRebuildPlanValidationSuppressesResultAfterGrantRevocation`
  uses a valid partial Snapshot without rebuild steps, revokes `ProjectEdit`
  after the Snapshot read, and showed the unsupported-source 400 before the
  fix. The corrected route returns only `PROJECT_NOT_FOUND`.
- Focused normal/Race rebuild-plan tests, `go vet ./internal/httpapi`, diff
  checks and full `go test ./internal/httpapi -count=1` passed (94.281 s).
  Fault injection and project data remained in disposable SQLite.
- Next continue the F-025 response audit, especially mutation Store errors
  and route mismatch outcomes. Keep the push gate closed on `0b09158` until
  all exact-head checks pass. Production ACL remains disabled; Pi 4 current
  head timing qualification is pending.

## Qualification batch checkpoint — 2026-11-15 · Rebuild-plan write errors

- Latest code commit: `a6a6cfc89a6b345523850e14e32fa3d541330c01`; local code
  HEAD is eight commits ahead of remote
  `0b091584d07f9324683e18954f37707b3d57941b` before this documentation
  update. No further push has occurred.
- Exact-head CI on `0b09158`: Core Go 1.26 Race run `43713024420` remains
  in progress. Go 1.27, Companion Swift/macOS and Camera Relay isolated ARM64
  passed. Keep the gate closed until Go 1.26 completes green.
- `a6a6cfc` adds live route-grant checks to Store write-error responses from
  rebuild-plan seed, update and delete. Red-first
  `TestExecutionEnvironmentRebuildPlanWriteErrorSuppressesStoreErrorAfterGrantRevocation`
  injects a temporary SQLite trigger that fails seed insertion, revokes
  `ProjectEdit`, and reproduced the generic seed 503 before the fix; it now
  returns only `PROJECT_NOT_FOUND`.
- Focused normal/Race tests, `go vet ./internal/httpapi`, diff checks and full
  `go test ./internal/httpapi -count=1` passed (94.220 s). Temporary database
  trigger only; production data, Vault and devices untouched.
- Next continue auditing F-025 mutation Store errors and project/revision
  mismatch responses. Do not push until `0b09158` Core Go 1.26 and the full
  exact-head CI set are green. Production ACL remains off; Pi 4 timing
  qualification is pending.

## Qualification checkpoint — 2026-11-15 · Rebuild-plan scope mismatch

- `64c9535d70d4703264b9a5e42796bb64aeee7d90` fixes the rebuild-plan response
  when an Environment manifest belongs to another Revision. The handler now
  rechecks the live qualified route grant before returning the mismatch 404.
- Red-first `TestExecutionEnvironmentRebuildPlanScopeMismatchSuppressesResultAfterGrantRevocation`
  used two Projects in temporary SQLite and revoked `ProjectView` at the
  mismatch response boundary. The old response disclosed
  `EXECUTION_ENVIRONMENT_NOT_FOUND`; the fixed response returns only
  `PROJECT_NOT_FOUND`.
- Focused normal/Race rebuild-plan regressions passed, as did
  `go vet ./internal/httpapi`, `git diff --check`, and full
  `go test ./internal/httpapi -count=1` (94.626 s).
- Local code HEAD before this documentation commit is
  `64c9535d70d4703264b9a5e42796bb64aeee7d90`, 10 commits ahead of remote
  `0b091584d07f9324683e18954f37707b3d57941b`. The exact-head Core CI run
  `43713024420` was still running Go 1.26 Race at the last check; Go 1.27,
  Companion Swift/macOS and Camera Relay isolated ARM64 checks had passed.
  Do not push until that run and the complete exact-head check set finish green.
- No production services, show data, Vault originals or real outputs were
  touched. Project ACL remains disabled in production. Pi 4 current-head
  performance qualification remains open; historical simulation is not Pi
  acceptance.
- Resume by checking exact-head CI for `0b091584d07f9324683e18954f37707b3d57941b`,
  then push the accumulated tested commits to the existing branch/PR only when
  green. Continue the F-025 response-boundary review while the gate runs.

## Qualification checkpoint — 2026-11-15 · Cross-Project Revision mismatch

- `3488227dc8963ac1d6691c11cb664d8a04788bab` fences the shared
  Project/Revision loader's mismatch response with a live qualified-route
  recheck. A revoked Project grant now receives only `PROJECT_NOT_FOUND`.
- Red-first `TestExecutionEnvironmentRevisionMismatchSuppressesResultAfterGrantRevocation`
  used temporary SQLite with a Project route and a foreign Project's Revision;
  revoking `ProjectView` at the mismatch boundary previously exposed
  `EXECUTION_ENVIRONMENT_REVISION_NOT_FOUND`.
- Focused normal/Race authorization regressions passed, `go vet
  ./internal/httpapi` passed, and full `go test ./internal/httpapi -count=1`
  passed (94.791 s). `git diff --check` passed. No production resources or
  physical outputs were touched.
- Code HEAD before this documentation commit is
  `3488227dc8963ac1d6691c11cb664d8a04788bab`, 12 commits ahead of remote
  `0b091584d07f9324683e18954f37707b3d57941b`. The exact-head Core run
  `43713024420` remains in progress in Go 1.26 Race; Go 1.27, Companion
  Swift/macOS and Camera Relay isolated ARM64 checks passed. Do not push until
  the prior remote-head Core Race and all exact-head checks finish green.
- Resume with live Project-scoped response auditing of the remaining
  execution-environment CRUD paths. Production ACL remains disabled, and
  current-head Pi 4 performance qualification is still pending.

## Qualification checkpoint — 2026-11-15 · Environment mutation delivery

- `c7ea8e3cf06b99c61366eea1352072f63506dc1d` fences execution-environment
  create, update, Machine-Role bind and delete responses. Store failures now
  recheck the live route grant; successful commits revalidate the session and
  required Project capability before exposing a payload or returning 204.
- Red-first `TestExecutionEnvironmentMutationsSuppressCommittedResultsAfterGrantRevocation`
  revokes authority after each commit. Before the change the four routes all
  returned success details; afterward each returns a non-retryable
  committed-effect response with `Cache-Control: no-store`, and the test checks
  the create/update/bind/delete side effect remains in temporary SQLite.
- Focused normal/Race tests passed, `go vet ./internal/httpapi` passed, and
  full `go test ./internal/httpapi -count=1` passed (96.638 s). No production
  data or real outputs were used.
- Local code HEAD before this documentation commit is
  `c7ea8e3cf06b99c61366eea1352072f63506dc1d`, 14 commits ahead of remote
  `0b091584d07f9324683e18954f37707b3d57941b`. Core run `43713024420` remains
  in Go 1.26 Race on that remote HEAD; Go 1.27 and related Companion/Camera
  checks passed. Keep the push gate closed until all exact-head checks finish.
- Resume by testing failure-path revocation for Environment CRUD, then
  continue the wider F-025 response audit. Production ACL remains disabled;
  current-head isolated Pi 4 performance qualification remains pending.

## CI recovery checkpoint — 2026-11-15 · Core Race and F-025 CRUD

- Code HEAD before this documentation commit is
  `807eb2c94ed691dedf18c6ec164d3d31da7695d0`, 17 commits ahead of remote
  `0b091584d07f9324683e18954f37707b3d57941b`.
- The previously pending exact-head Core workflow `43713024420` has now
  completed. Go 1.27 passed; Go 1.26 Race failed in
  `TestCueStopCancelsInterruptibleActionAndPersistsTruthfulResult` after its
  ActionExecution row had persisted but before the test proved adapter entry.
  The log reports “Action stopped before dispatch”; this is the valid
  pre-dispatch cancellation path, while the test asserted only the in-flight
  executor error. The test now waits on an injected executor's explicit entry
  signal and then requests STOP. No runtime implementation changed for this
  CI failure. Companion Swift/macOS checks and Camera Relay ARM64 also passed.
- Regressions cover live grant revocation during Environment create/update/
  bind/delete success delivery, all four write errors, update/bind/delete
  manifest read errors, and Project/Revision or Environment scope mismatches.
  The errors are suppressed after revocation; committed mutation responses
  disclose no Environment payload and report non-retryable commit uncertainty.
- Focused HTTP API normal/Race tests pass, full
  `go test ./internal/httpapi -count=1` passes (97.926 s), and
  `go vet ./internal/httpapi` passes. The runtime STOP regression passes 25
  normal repetitions and 10 Race repetitions; full
  `go test ./internal/runtimecontrol -count=1` and its vet pass.
  `git diff --check` passes.
- Temporary SQLite/fault triggers only; no production services, data, Vault
  originals, physical devices or hardware output were touched. Production ACL
  remains disabled.
- The old Core workflow is complete (failed), so it can no longer be canceled
  by a branch push. After committing these notes, push the accumulated tested
  changes to the existing `integration/stagecore-rc` / PR #534 and watch the
  new exact-head CI. Current-head isolated Pi 4 performance qualification
  remains pending; historical mixed/linked simulation is not current-head Pi
  acceptance.

## Qualification checkpoint — 2026-11-15 · Capture failure and asset scope

- `4dd7c49775dc05a6ad0252ae1e3cf60f2a0fe4fc` fences F-025 capture error
  responses for frozen/mutable-project checks, Vault import failures and
  manifest-write failures. Asset reads now recheck Project scope before
  foreign-Revision and missing-asset 404 responses.
- Normal tests revoke ProjectEdit+MediaUpload at the two deterministic capture
  error boundaries, verify only a denial is returned, and confirm failed
  capture leaves no unreferenced object in the temporary Vault. Separate
  cases revoke ProjectView/MediaRead during foreign-Environment and missing
  asset responses; both hide the resource outcome.
- Focused Race tests pass (38.925 s); `go vet ./internal/httpapi` and
  `git diff --check` pass; full `go test ./internal/httpapi -count=1` passes
  (98.949 s). No physical output or production service/data was involved.
- Local code HEAD before this documentation commit is
  `4dd7c49775dc05a6ad0252ae1e3cf60f2a0fe4fc`, one commit ahead of remote
  `c63160e7c50088984d7016326c31a04554afff2d`. Exact-head Core workflow
  `43717094969` has Go 1.27 green and Go 1.26 Race still in progress;
  Companion and Camera Relay checks are green. Hold further pushes until that
  complete exact-head check set finishes.
- Resume with the remaining F-025 read/delivery error boundaries; then push
  all accumulated commits to the existing PR #534 once `c63160e` Core CI is
  green. Project ACL remains disabled in production; current-head isolated Pi
  4 performance qualification remains open.

## Qualification checkpoint — 2026-11-15 · Snapshot scope delivery

- `1d3b1c32c3eecc5acca11e28bacb7d44439f9f61` routes snapshot-list,
  detail and diff scope mismatch/incompatibility responses through live
  ProjectView/session checks before returning the outcome.
- Red-first `TestExecutionEnvironmentSnapshotScopeErrorsSuppressResultAfterGrantRevocation`
  covers three routes (foreign Environment list, foreign Snapshot detail,
  cross-Environment diff) using temporary SQLite. All three returned specific
  `EXECUTION_ENVIRONMENT_*` results after grant revocation before the fix;
  after the fix each returns only `PROJECT_NOT_FOUND`.
- Focused Race regressions passed (49.459 s); `go vet ./internal/httpapi` and
  `git diff --check` pass; full `go test ./internal/httpapi -count=1` passes
  (99.353 s). No production service, media or physical output was used.
- Local code HEAD before this documentation commit is
  `1d3b1c32c3eecc5acca11e28bacb7d44439f9f61`, three commits ahead of remote
  `c63160e7c50088984d7016326c31a04554afff2d`. Current Core workflow
  `43717094969` is still running Go 1.26 Race on `c63160e`; Go 1.27,
  Companion Swift/macOS and Camera Relay ARM64 passed. Keep pushes paused until
  the complete exact-head set finishes.
- Resume with the remaining F-025 read and mutation error boundaries, then
  push the accumulated commits to PR #534 after the active Core gate is green.
  Production ACL remains off; current-head Pi 4 performance qualification is
  still open.

## Qualification checkpoint — 2026-11-15 · Operation and OSCQuery scope

- `0458f3d5c74238286592c6ac814d1e95800aa6a0` fences the Companion operation
  Environment mismatch and OSCQuery Environment, Machine Role and Companion
  lookup errors with a live qualified-route/session recheck before response.
- Red-first regressions use temporary SQLite and a fake Companion runtime.
  They cover a foreign Revision, an unbound Machine Role and an unassigned
  Companion; OSCQuery previously returned the resource-specific outcome after
  grant revocation and now returns `FORBIDDEN`. Both tests verify zero operation
  or OSCQuery dispatch. Focused normal and Race tests, `go vet
  ./internal/httpapi`, `git diff --check`, and full
  `go test ./internal/httpapi -count=1` pass (97.913 s).
- No physical Companion, network endpoint, production service, show data or
  Vault was used. ACL remains disabled in production. The active Core CI run
  for remote `c63160e7c50088984d7016326c31a04554afff2d` (`43717094969`) is
  still running Go 1.26 Race; do not push until that run and all checks finish.
- Local code HEAD is `0458f3d5c74238286592c6ac814d1e95800aa6a0`, five
  commits ahead of remote `c63160e7c50088984d7016326c31a04554afff2d`; this
  note and the qualification matrix updates are being committed separately.
- First monitor the current remote-head Core CI. Push the accumulated commits
  to existing PR #534 only after its Race and remaining checks complete green.
  Continue the F-025 response audit while that gate runs. Current-head isolated
  Pi 4 performance qualification remains pending; the historical Pi result is
  not current-head acceptance.

## Push checkpoint — 2026-11-15 · F-025 response fences

- The exact pushed head `c63160e7c50088984d7016326c31a04554afff2d` completed
  Core run `43717094969` green: Go 1.26 Race passed in 31m17s and Go 1.27
  passed. Companion replacement/media, Swift CompanionCore, and isolated
  Camera Relay ARM64 checks also passed (`43717095124`, `43717094956`).
- After that gate cleared, six accumulated commits were pushed normally to
  the existing branch and Draft PR #534, with no force push:
  `4dd7c49`, `bc42320`, `1d3b1c3`, `9e99150`, `0458f3d`, and `0c4b055`.
  Remote and local code HEAD at this checkpoint: `0c4b055842f8755ffb2f144d0daae55d4c1bf99e`.
- New exact-head checks for `0c4b055` are pending: Core run `43719886374`,
  Companion run `43719886173`, and isolated ARM64 run `43719886155`. Do not
  push again until every check completes; a later push would cancel active CI.
- Batch evidence: the two red-first operation/OSCQuery scope regressions pass
  normally and under Race; `go vet ./internal/httpapi`, diff checks, and full
  `go test ./internal/httpapi -count=1` pass (97.913 s). Prior capture and
  snapshot response-fence evidence is in the preceding checkpoints. Working
  tree was clean after the push.
- No real Companion, OSC endpoint, production service/data/Vault, original
  show, or hardware output was used. Project ACL remains disabled. Current
  Pi 4 qualification cannot use the production-like `stagecore-pi`; the
  isolated SSH endpoint remains unreachable before host identity, so no Pi
  workload ran.
- Resume by watching all three exact-head CI runs. Then continue the highest
  remaining software gap in SC-024's route/delivery audit, keep the PR draft,
  and preserve the current-head isolated Pi performance qualification as
  pending until a verifiable isolated target is available.

## CI completion checkpoint — 2026-11-15 · exact head `0c4b055`

- Core CI `43719886374` completed successfully on
  `0c4b055842f8755ffb2f144d0daae55d4c1bf99e`: Go 1.26 (including Race) and
  Go 1.27 passed. Companion replacement/media and Swift CompanionCore passed
  in `43719886173`; isolated tests and Linux ARM64 product checks passed in
  `43719886155`.
- Remote branch and PR #534 still point to `0c4b055`. Local HEAD is
  `db9c61fa3d16266c176e356bc668a626aec4c0d9`, one documentation checkpoint
  ahead; this follow-up records the completed CI and will be pushed normally
  to the same branch now that all checks have completed. No source changes
  are waiting in the worktree.
- No production or physical-system boundary changed. ACL remains disabled;
  current-head Pi performance qualification remains pending because the
  isolated endpoint has not provided a verifiable host session.
- After pushing the updated handoff, monitor the new exact-head CI. Continue
  with the next software gap from the SC-024 route/delivery review without
  using the production-like Pi host or changing production data/services.

## Resume checkpoint — 2026-11-15 · SC-014 Cue list layout

- Source commit `37ac5e5ac787a1426a7721987194090b640a7b2c` changes the Operator Cue list into labeled cards through 1080 CSS px, preserving explicit table/row/cell roles and retaining the native table layout above that breakpoint. The existing Cue Inspector and linked Group content remain visible.
- The Chrome gate now establishes its initial 390×844 CSS viewport through CDP (the local macOS headless Chrome initially reported 500px despite `--window-size=390,844`). It checks card selection, seven field labels, table semantics, linked Group visibility and horizontal overflow at 390, 720, 768, 1024 and 1440 CSS px in RTL and LTR. At 200% zoom it captures both locales at a 720px CSS viewport and DPR 2.
- Normal and Race `TestBrowserCreatesNestedMixedCueChainInDraft` pass with local Chrome; `go vet ./internal/httpapi`, `node --check internal/httpapi/testdata/mixed_cue_browser.js`, and `git diff --check` pass. Measurements show no document or Cue-list overflow at all tested sizes; cards are used through 1024px and native table layout at 1440px. Temporary SQLite and simulator alias only; no runtime session or output dispatch.
- Local source HEAD is `37ac5e5ac787a1426a7721987194090b640a7b2c`, one commit ahead of remote `79423d7b6e22754f21bcd69e06810b99ecd91195`. Core CI for the exact remote head is still running Go 1.26 Race in run `43723530598`; Go 1.27 passed. Companion replacement/media + Swift CompanionCore run `43723530535` and isolated ARM64 run `43723530595` passed. Hold the push until the remaining exact-head CI finishes.
- The matrix now records the Cue-list overflow closure. SC-014 remains Partial for the all-route visual/screenshots and zoom audit; physical Android fit remains hardware qualification. Current-head Pi 4 performance qualification remains pending: prior attempts to the isolated SSH endpoint failed before a verifiable host session, and the production-like Pi is prohibited.
- No production service, show data, original Vault, ACL setting, networked Companion, or physical device was touched. Project ACL remains disabled. PR #534 stays Draft; no force push or merge.
- Next: finish monitoring the current exact-head Core CI, recheck the remote SHA, and push this source commit plus the matrix/handoff update normally to the existing branch only after all checks are green. Then monitor the new exact-head CI. Resume the isolated Pi 4 Mixed Cue/linked-chain performance qualification when a verifiable isolated target is available; otherwise continue the next software gap and retain hardware qualification as pending.

## Follow-up audit — 2026-11-15 · browser suite and SC-024 evidence

- The real Chrome `^TestBrowser` suite passed with `STAGECORE_TEST_CHROME` pointed at the local browser; it uses the suite's temporary HTTP/SQLite fixtures and mocks. The focused Mixed/Linked Cue browser case also passes normally and under Race, plus `go vet ./internal/httpapi`, JavaScript syntax, and diff checks.
- A new read-only `stagecore-qualification` SSH attempt again timed out during banner exchange (`ConnectTimeout=8`); host identity was not established and no remote command ran. Keep Pi 4 current-head P95/P99, jitter, GC/allocations, CPU/RAM, burst and sustained qualification pending. Do not use `stagecore-pi`.
- The SC-024 qualification now records the existing post-commit result fences for StageLaser assignment, experimental V2 software transfer and experimental lighting activation. Tests revoke the Owner session after the injected commit and require a non-retryable committed-effect response without returning the operation record; no device socket write occurs. This retires a stale blocker in the older narrative but does not close the whole endpoint-to-permission review.
- Local branch HEAD before this documentation update is `d384818caab935a7cefaf1cd262e205da7c62c3a`; code commit `37ac5e5ac787a1426a7721987194090b640a7b2c` is two commits ahead of remote `79423d7b6e22754f21bcd69e06810b99ecd91195`. Remote exact-head Core run `43723530598` remains in Go 1.26 Race; Go 1.27, Companion replacement/media, Swift CompanionCore and isolated ARM64 passed. No push until the outstanding check completes.
- Production services/data/Vault, real devices, networked Companion and physical output remain untouched. Production Project ACL remains disabled; PR #534 stays Draft.
- Next: recheck exact-head Core CI and remote SHA; push the accumulated local source and documentation commits normally only after CI is green. After the new exact-head CI starts, continue the still-open whole-server ACL reconciliation at an unreviewed endpoint group, and retry isolated Pi qualification only when the endpoint yields a verifiable host identity.

## Resume checkpoint — 2026-11-15 · unified Cue search and browser gates

- Source commit `50d579d308d5bddd8eecb07912a8352183d60434` removes the duplicate SC-008 Cue Studio search/filter strip; the existing per-Project SC-012 search/filter remains the single control and retains deeper nested Cue/Action/target search and Mixed/linked/enabled filters. Inspector, type badges and Draft history controls remain.
- The Cue Draft-history browser test asserts exactly one search control and uses it while checking native keyboard Undo and touch Redo. The Mixed/Linked Cue browser audit now centers the Inspector before strict 200% visibility checks; RTL/LTR screenshots still qualify a 720px CSS viewport at DPR 2 with no API mutations.
- Final local evidence: all `^TestBrowser` tests pass; focused `TestBrowserCueDraftHistoryKeyboardUndoAndTouchRedo` plus `TestBrowserCreatesNestedMixedCueChainInDraft` pass normally and under Race; `go test ./internal/operatorweb -count=1`, `go vet ./internal/httpapi ./internal/operatorweb`, JavaScript syntax checks and `git diff --check` pass.
- Local source HEAD before this documentation commit is `50d579d308d5bddd8eecb07912a8352183d60434`, three commits ahead of remote `79423d7b6e22754f21bcd69e06810b99ecd91195`: `37ac5e5` (responsive cards), `d384818` (SC-014 matrix/handoff), and `50d579d` (unified search). The current exact remote Core run `43723530598` is still in Go 1.26 Race; Go 1.27, Companion replacement/media, Swift CompanionCore and isolated ARM64 passed. No push until all exact-head checks finish.
- SC-012 remains Partial only for native keyboard/touch interaction on its search/filter controls and trustworthy error/readiness filters; the duplicate-bar conflict is closed. SC-014's tested Cue-list overflow is closed; the all-route screenshot/zoom audit and physical Android fit remain open.
- The latest isolated Pi SSH check timed out during banner exchange, so no remote workload ran. Keep ARM64 Pi current-head performance pending. No production service/data/Vault, physical output, device socket, firmware or ACL cutover was touched; PR #534 remains Draft.
- Next: recheck the exact Core run and remote SHA; push the accumulated commits normally to the existing branch after CI is green, then monitor its exact-head workflows. Continue the remaining whole-server permission/delivery audit. Retry Pi qualification only when a verified isolated host session is available; never use the production-like Pi host.

## Follow-up verification — 2026-11-15 · native Cue filter touch

- `TestBrowserCreatesNestedMixedCueChainInDraft` now uses native CDP touch to clear the combined Mixed/Linked filters. It verifies all 123 Cue rows return and the original sequence order is unchanged; the same browser case already checks filters against a three-level compound linked chain and measures the responsive card/table layout in RTL and LTR.
- The focused Mixed/Linked Cue browser test passes normally and under Race. The full `^TestBrowser` suite passes, as do `go vet ./internal/httpapi`, `go test ./internal/operatorweb -count=1`, JS syntax checks, and `git diff --check`.
- Working branch HEAD before saving this follow-up is `9d69c508cbe3046c8e51c43a1d5940137f01a7b3`; the remote remains `79423d7b6e22754f21bcd69e06810b99ecd91195`. Four completed commits are waiting locally, and this touch-gate change plus its matrix/handoff evidence is the fifth. Core run `43723530598` still has Go 1.26 Race in progress; no push until the entire exact-head check set is complete.
- SC-012 now has real-browser touch coverage for clearing filters; native keyboard interaction directly on the search/filter controls remains open. Search/filter operations do not mutate Draft or Snapshot. All tests remain fixture-backed; no output command, production service/data, or device was used.

## Resume update — 2026-11-15 15:00 UTC · SC-012 native keyboard qualification

- New source commit `23b5f22e2b7e42682f3ba5d86905901ff507209f` adds native Chrome DevTools Protocol keyboard input for the compact Cue search/filter flow. At 390 CSS px, RTL and LTR each type a late-list Cue search, clear it with repeated Backspace, select Mixed and Linked using Home/ArrowDown/Enter, traverse with Tab, and activate Clear with Enter. The test verifies all 123 rows and their original order return. The existing native-touch filter clear and 390–1440px responsive/200% zoom checks remain.
- Verification passed: `TestBrowserCreatesNestedMixedCueChainInDraft` in real Chrome normally and under Race, `go vet ./internal/httpapi`, `node --check internal/httpapi/testdata/mixed_cue_browser.js`, and `git diff --check`. The browser test remains isolated to temporary SQLite, simulator aliases and mocked HTTP; it creates no Session, Snapshot mutation, or output command.
- Matrix SC-012 now records the keyboard evidence. Trustworthy error/readiness filters remain conditional on a reliable source. Search/filter behavior continues to leave Draft, Snapshot and execution order unchanged.
- Local HEAD is `23b5f22e2b7e42682f3ba5d86905901ff507209f`, six commits ahead of remote `79423d7b6e22754f21bcd69e06810b99ecd91195`; the Matrix/Handoff update is the next documentation commit. Exact remote Core run `43723530598` still has Go 1.26 Race in progress (started 2026-11-15 14:23:14 UTC); ARM64 builds are queued. Go 1.27, Companion replacement/media, Swift CompanionCore and isolated ARM64 checks passed. Hold the normal push until the exact-head run and every PR check complete successfully, then recheck that the remote SHA is unchanged.
- The current-head Raspberry Pi 4 Mixed/linked performance gate remains open. The isolated qualification SSH attempt did not establish host identity; no command or workload ran. Do not access `stagecore-pi`, production services/data/Vault, or physical stage outputs. Production Project ACL remains disabled; PR #534 remains Draft.
- After CI clears and this batch is pushed normally to the existing PR, monitor the new exact-head workflows. Continue the unreviewed SC-024 endpoint/permission audit; retry current-head Pi measurements only when a verified isolated Pi 4 session is available. Keep Software Complete, Hardware Qualification Pending, and Production Release Approval Pending separate.

## Push checkpoint — 2026-11-15 15:08 UTC · SC-012 browser qualification

- The previous pushed head `79423d7b6e22754f21bcd69e06810b99ecd91195` completed all exact-head checks: Core `43723530598` passed Go 1.26 and 1.27, including Race (45m38s) and Linux ARM64 product builds; Companion replacement/media + Swift CompanionCore passed in `43723530535`; isolated tests/ARM64 passed in `43723530595`.
- After confirming the PR and remote still pointed at `79423d7`, the seven-commit local stack was pushed normally, with no force push, to `integration/stagecore-rc`. New remote and Draft PR #534 head: `c59f2287aa7c0fff66bce6b5c49ff77f5bcea3cc`. Its exact-head runs are Core `43727849631`, Companion `43727849672`, and isolated ARM64 `43727849626`. Companion and isolated ARM64 pass; Go 1.26/1.27 are pending. Do not push again until all exact-head checks finish successfully.
- The SC-012 keyboard/touch acceptance and responsive Cue-list changes are included in `c59f228`. Normal and Race real-Chrome browser checks and `go vet ./internal/httpapi` passed locally before push. A separate local documentation-only follow-up records Issue #535's explicit post-RC deferral.
- Current Issue #535 explicitly says its PWA, local static cache, incremental refresh and safe offline-authoring work is non-blocking and must not be implemented as part of this autonomous SC-024/F-025 campaign without a separate Owner decision. The completion matrix records that boundary; no implementation or branch was started.
- The SC-024 Issue #511 remains open, matching its still-incomplete all-endpoint qualification; no production ACL activation is implied. Related open PRs #36, #39, #20, #56 and #57 were read-only reviewed at their existing heads and all displayed CI checks pass. No related repo, firmware, production service/data/Vault, show, or physical output was changed.
- Raspberry Pi 4 current-head Mixed/linked performance qualification remains open because the isolated SSH endpoint failed before establishing host identity. Do not use `stagecore-pi`; no remote workload ran. Next, finish exact-head CI at `c59f228`, recheck ancestry and push the pending docs-only note only after it is green, then continue the next active software gap while keeping Hardware Qualification and Production Release Approval separate.

## Resume checkpoint — 2026-11-15 15:18 UTC · Linux focus regression and Project Files evidence

- Current PR/remote head before this batch is `c59f2287aa7c0fff66bce6b5c49ff77f5bcea3cc`. Exact-head Core run `43727849631` is complete but failed only in Go 1.26's `Browser Cue Delivery`: the Linux Chrome native `<select>` Enter changed focus, so the prior test's fixed two-Tab sequence skipped Linked Cues. Go 1.26 unit Test and isolated update/rollback passed; Go 1.27 Test and Vet passed. Race and Linux ARM64 product builds were skipped after the Browser Cue Delivery failure. Companion run `43727849672` and isolated tests/ARM64 `43727849626` passed. No push was made while those checks were running.
- Source commit `50a88c13c642e1f7a763a16cfec1f5b4bf5e0ff8` adjusts the native keyboard acceptance to inspect actual focus after Action type selection and traverse to Linked/Clear from the observed element. It also retains actual 200% RTL/LTR Project Files screenshots and records the synthetic, read-only browser evidence in the completion matrix. The test uses temporary SQLite metadata only and confirms no upload/download affordance; it does not access file bytes or production Vault.
- Local verification on this batch passed both `TestBrowserCreatesNestedMixedCueChainInDraft` and `TestBrowserScopedProjectFilesShowsUsageAndPageCounts` in real Chrome, normally and under `-race`; `go vet ./internal/httpapi`, both changed JS `node --check`s, and `git diff --check` passed. The retained images are 1440×757 physical pixels at 200% browser zoom, 720 CSS-pixel viewport, DPR 2; test reports no document overflow in either locale.
- The local branch includes documentation commit `06b199908d558ec765ece178c2e36fbc984c10bd` and source/evidence commit `50a88c13c642e1f7a763a16cfec1f5b4bf5e0ff8` beyond remote `c59f228`. The handoff update is the next local commit. Before push, verify `git ls-remote` and PR #534 still point to `c59f228`; the failed exact-head CI is complete, so this corrective batch may be pushed normally to the existing branch. No force push or PR creation.
- Do not push another batch while the new exact-head Core run is active. Wait for Go 1.26/1.27, Browser Cue Delivery, Race, Linux ARM64 product builds, Companion and isolated workflows. The isolated Raspberry Pi 4 endpoint has still not established host identity; no benchmark ran. Do not use `stagecore-pi` or real devices. Continue with the open SC-024 endpoint-to-permission review; production Project ACL remains disabled, production services/data/Vault untouched, and PR #534 remains Draft.

## Resume update — 2026-11-15 15:32 UTC · retry Linux native select traversal

- The new exact-head workflows for `28106af122a442f8f26229aad4b962062d129c00` completed. Core run `43729289159` Go 1.26 `Test` and isolated update/rollback passed; Go 1.27 `Test` and `Vet` passed; Companion run `43729289210` and Camera Relay run `43729289165` passed. Core Browser Cue Delivery still failed in `TestBrowserCreatesNestedMixedCueChainInDraft`: after Enter on the Action type `<select>`, Linux Chrome remained focused on that selector after one Tab. Race and ARM64 product builds were skipped. This is the exact-head failure; no newer push was made while it ran.
- Source commit `804f328a85f49c445d060ec439eeb10de84d661f` changes the browser gate to send bounded Tab events until it reaches the expected Linked Cues filter and Clear control, checking focus after each event. This accounts for Linux Chrome consuming an initial Tab while dismissing native select state and stops immediately once the expected control receives focus. Local real-Chrome normal and Race Mixed/Linked browser tests pass; `node --check`, `go vet ./internal/httpapi`, and `git diff --check` pass.
- Local branch now contains commit `804f328` beyond remote `28106af`; the handoff update is the next local commit. Before the normal corrective push, verify remote and PR #534 still point at `28106af`. Its failed Core workflow is complete, so a normal push to the same branch is safe. Wait for the new exact-head Core, Companion Core and Camera Relay CI to finish before any subsequent push.
- Retried read-only Pi identity through the configured `stagecore-qualification` alias. It resolves separately from `stagecore-pi.local`, but SSH again timed out during banner exchange; the identity command never ran and no workload or remote write occurred. Current-head Pi 4 mixed/linked performance remains unqualified. Keep production ACL disabled, production services/data/Vault untouched, PR #534 Draft, and all hardware commands off.

## Resume update — 2026-11-15 15:39 UTC · SC-024 full route-policy probe

- Local source commit `5b41f504acd43782bde7a2a496d6b64cf1e33396` adds `TestQualifiedProjectRoutePermissionMatrix`. Its isolated no-op router probes every pattern in `projectRoutePermissions`: the exact mapped grant authorizes, removing one required capability returns `PROJECT_NOT_FOUND`, and a grant on Project A cannot authorize Project B. No real business handler, Vault, Runtime, Companion or device adapter is called. The matrix and SC-024 qualification record the scope and limits.
- The new route-matrix test passed normally and under Race (`go test ./internal/httpapi -run '^TestQualifiedProjectRoutePermissionMatrix$' -count=1` and the same with `-race`); `go vet ./internal/httpapi` and `git diff --check` pass. This supplements the existing route inventory/dynamic registration guard; it does not claim response-delivery qualification of every live handler.
- Remote/PR head remains `bcb1061da60ba679fc80c383bd46f5eeff3c61fc`. Exact-head Core run `43730520977` has Go 1.27 Test/Vet passed and Go 1.26 Test, isolated installed-copy rollback, and Browser Cue Delivery all passed; Go 1.26 Vet is running and Race/Linux ARM64 builds are pending. Companion `43730521032` and Camera Relay `43730520956` completed successfully. Do not push while Core remains active. Local is one source commit ahead; after all exact-head checks finish green, recheck PR/remote and push the completed local batch normally.
- Pi 4 remains pending: `stagecore-qualification` again failed before SSH banner/host identity, no command or benchmark ran. Do not use `stagecore-pi`; keep production ACL disabled, production services/data/Vault untouched, and PR #534 Draft.
