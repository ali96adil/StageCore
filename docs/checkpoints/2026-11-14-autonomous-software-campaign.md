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
  It is not pushed yet; remote remains at `814cf84`.
- PR #534 remains open/draft. No branch or PR was created; no force push or
  merge was performed.

## CI and linked repositories

At remote HEAD `814cf84`, Core CI run `43529309969` passed all five checks:
Go 1.26/1.27, isolated tests/ARM64, Companion replacement/media and
Swift CompanionCore. This run is complete; after the next push, wait for CI on
its exact HEAD before pushing again.

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

1. Local commit `af3e1ba` is ahead of remote `814cf84`; docs/matrix/handoff
   corrections remain in the worktree. Core CI on remote `814cf84` passed all
   five checks, so review and commit those docs, then push normally to the same
   PR #534 and wait for CI on its exact new head.
2. SC-015's session/Cue/Action evidence now makes
   `COMPLETED + TRANSPORT_ONLY` report external execution as unknown. Delay,
   timeout, duplicate-after-reconnect, offline-reconnect and signed application
   ACK simulations already exist in Cue/Companion channel tests; don't copy
   them. Remaining report work: connection/warning/snapshot/safety timeline,
   filtering, comparison and safe export. SC-001 still lacks a first-party
   Ableton execution provider/observation contract.
3. Continue SC-002 beyond the existing #504/#505 status/preview stack. Do not
   duplicate their work or add a host clock write without isolated qualification.
4. Raspberry Pi 4 performance qualification remains open. The explicit
   read-only SSH attempt to `stagecore-qualification` failed during handshake
   before host identity was established. Do not run commands via the ambiguous
   Pi alias. Use a verified isolated Pi lab when access is restored.
5. Do not access production Hub/Camera Relay/data/Vault, send to real stage
   devices, flash firmware, merge to `main`, force-push, or enable production
   Project ACL/SC-024. Simulation and temporary databases only.

## Local SC-015 follow-on

The follow-on worktree changes (not covered by the completed CI run on `814cf84`)
add delivery evidence derived only from the explicit ACK level. A Hub Action
with `COMPLETED` plus `TRANSPORT_ONLY` reports `EXTERNAL_EXECUTION_UNKNOWN`;
no downstream execution is inferred. Session Memory carries recorded dispatch
attempts and shows Cue duration, correlation and a localized evidence label. A
regression test covers completed CoreMIDI transport with unknown external
execution, and the embedded UI contract requires that distinction.

Focused checks passed before local commit `af3e1ba`:
`go test ./internal/sessionmemory -count=1`, the two
Session Memory/embedded Operator HTTP tests, their Race runs,
`go test -race ./internal/operatorweb -count=1`, `go vet` for all three
packages, `node --check internal/operatorweb/static/memory.js` and
`git diff --check`. The full `go test -race ./internal/httpapi -count=1`
exceeded the package's 10-minute timeout in the unrelated
`TestStageDeviceSetupAPSuppressesACKAfterOwnerRevocation` Argon2 login test;
the focused Session Memory/Operator HTTP tests pass under Race. Run the
follow-on branch through exact-head Core CI after its push.

## Software/hardware/release state

This work adds software performance evidence and a bounded Cue Engine
optimization. It does **not** establish Software Complete. Pi ARM64 execution,
physical device integration/ACK/Blackout, and supervised acceptance remain
`Hardware Qualification Pending`; production rollout and SC-024 enablement
remain `Production Release Approval Pending`.
