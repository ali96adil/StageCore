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
- These local commits were made after the recorded remote head. The worktree
  was clean after both commits; the normal fast-forward push to the same PR
  remains the next operation. Verify branch and remote again before pushing.
- PR #534 was open/draft/mergeable at the verified base. No branch or PR was
  created; no force push or merge was performed.

## CI and linked repositories

At remote HEAD `701cd7b`, Core CI `43518665334`, Companion Core
`43518665379`, and Camera Relay `43518665360` passed. These runs do not cover
the local commits above. After the push, wait for Core CI and companion checks
on the exact new PR head before pushing another batch.

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
  hardware requirements. It identifies SC-003 as the next code fix.
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

1. Push these commits normally to `integration/stagecore-rc` after confirming
   remote still equals the recorded base. Then monitor exact-head CI.
2. Implement SC-003 Issue #503: preserve V1 ProjectID authorization, authorize
   projectless V2 configuration only for enabled matching assignments in
   BLOCKED/ACTIVE state, and reject wrong-project/preparing/unassigned cases.
   Add PUT/DELETE/channel/alias/epoch regression tests before the fix.
3. Continue SC-001 delivery diagnostics and SC-002 offline clock workflow from
   the completion matrix; do not repeat the closed Mixed Cue baseline work.
4. Raspberry Pi 4 performance qualification remains open. The explicit
   read-only SSH attempt to `stagecore-qualification` failed during handshake
   before host identity was established. Do not run commands via the ambiguous
   Pi alias. Use a verified isolated Pi lab when access is restored.
5. Do not access production Hub/Camera Relay/data/Vault, send to real stage
   devices, flash firmware, merge to `main`, force-push, or enable production
   Project ACL/SC-024. Simulation and temporary databases only.

## Software/hardware/release state

This work adds software performance evidence and a bounded Cue Engine
optimization. It does **not** establish Software Complete. Pi ARM64 execution,
physical device integration/ACK/Blackout, and supervised acceptance remain
`Hardware Qualification Pending`; production rollout and SC-024 enablement
remain `Production Release Approval Pending`.
