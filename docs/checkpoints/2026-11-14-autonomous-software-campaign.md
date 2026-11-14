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
