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

## Resume update — 2026-11-14 12:40 UTC · SC-019 reversible approval evidence

- Commit `e457922955124e86f8a3e6455f6246b05bc8951e` extends the existing timing approval regression: it records raw timing observations before approval, repeats INCLUDE while the Rehearsal is active, verifies the active candidate remains pending, then verifies learning after stop and reversibility through AUTO/EXCLUDE. It asserts the complete raw observation records are identical before and after all selection changes. No production implementation change was necessary; the completion matrix now records the verified test evidence.
- Checks passed: focused test normal and Race; full `go test ./internal/timingintelligence -count=1` and `go test -race ./internal/timingintelligence -count=1`; `go vet ./internal/timingintelligence`; and `git diff --check`. Local branch is seven commits ahead of remote `2cad37b`; worktree was clean after commit.
- The active remote CI is still for exact SHA `2cad37b5d27bf9987a1d61e5f960dbfa34b36fcd`, which predates all seven local commits: Go 1.27 and Go 1.26 tests, Browser Cue Delivery and Vet passed; Go 1.26 Race remains active and Linux ARM64 builds have not started. Companion `43594362838` and Camera Relay `43594362840` passed. No push until the complete Core gate finishes successfully.
- After the CI gate, recheck the remote branch and PR #534, push the seven local commits normally to the existing branch, update PR description with the new exact-head checks, and wait for those checks before another source push. Continue with another partial software gap; keep production ACL disabled and all production/device/Vault boundaries intact.
