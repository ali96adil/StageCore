# Fast-track final closure checkpoint — 2026-11-16

## Current continuation — performance refresh from the r7 source

- Repository/branch: `ali96adil/StageCore`, `integration/stagecore-rc`. Starting code/local/remote HEAD for this documentation refresh was `d5e60226591cd50a6b9c4a7dfc37f09d040283a0`; the working tree was clean before it. Draft PR #534 remains open. Remote tag `stagecore-rc-lab-2026-11-16-r7` dereferences to this exact code SHA. If this refresh is committed, that commit becomes the branch HEAD while r7 continues to identify the unchanged code candidate.
- Exact-head CI is green at d5: Core `43847441080`, Companion Core `43847440926`, Camera Relay `43847440935`. No new code changed in this continuation; this does not supersede the r7 code tag.
- Reran the existing Apple M3 simulation benchmarks because runtime code changed after the earlier 2026-11-16 performance report. The results are now in `docs/architecture/cue-execution-performance-qualification.md`: flat Mixed and compound linked 24-Action Engine P95 are 10.96 and 11.73 ms; a linked 100-Cue/200-Action group is 81.08 ms at P95; the 100-Cue/200-Action sustained Rehearsal records GO P95 3.388 ms at one Go CPU. These are full simulated engine/runtime paths, excluding HTTP, ACL, and external outputs.
- The 100-sample Mac stress confirmation measured 32 delayed GO cancellations at P95 86.39 ms for STOP and 87.51 ms for Emergency Blackout with `GOMAXPROCS=4`, versus 11.58/12.87 ms at one Go CPU. CPU profile is dominated by SQLite syscalls (`syscall.rawsyscalln` 87% flat; SQLite VM 79% cumulative). No latency SLO exists and this is not a Pi result, so no durability or batching change is justified by this evidence.
- Process RSS from benchmark commands includes fixtures and migrations (burst 225,247,232 B; 32-cue safety 233,717,760 B; sustained 221,020,160 B); it is not Hub service memory. Performance qualification remains **PENDING** until a trusted isolated Pi 4 runtime measurement; do not retry SSH without a new connectivity premise or use `stagecore-pi`.
- SC-024 whole-server policy/delivery and atomic cutover qualification remain open; production ACL remains disabled. The current source review found no new red-first authorization defect to fix in this continuation. Next work: continue only a reproducible, route-specific SC-024 delivery gap or an independently verified critical-path defect; maintain production and physical-device restrictions.

## Latest continuation — SC-024 partial-activation guard alias bypass

- Starting local and remote HEAD: `65db23c50025aba9267bcf5a4811d8c27253c8c5`; PR #534 is OPEN / DRAFT. The starting HEAD's Core, Companion, and Camera Relay checks were green. The qualified tag `stagecore-rc-lab-2026-11-16-r6` remains pinned to that starting SHA.
- A focused regression demonstrated that `TestSC024NoPartialProjectACLFeatureActivationInProduction` only searched for the literal `httpapi.Option(...)` spelling and missed the same guarded option called through an import alias such as `acl.WithProjectRouteQualification(...)`.
- The production-source guard now parses Go ASTs and identifies the guarded option by selector or direct function name, independent of package alias. No production assembly or route was changed; Project ACL remains disabled. This closes a gap in the partial-activation safety gate, not the full SC-024 audit or cutover qualification.
- Validation: the alias regression failed against the text scan and passes with AST inspection. `go test ./internal/httpapi -run '^TestSC024' -count=1`, the same command with `-race`, `go vet ./internal/httpapi`, and `git diff --check` pass locally.
- This code+handoff change is based on the exact green `65db23c...` head. On continuation, inspect the current local SHA and remote branch, push normally only if the remote still matches the base, then wait for Core, Companion, and Camera Relay CI for the resulting exact SHA before another push. Do not move or recreate tag `r6`.
- SC-024 whole-server permission/delivery and atomic cutover qualification remain open. Pi runtime performance remains **PERFORMANCE QUALIFICATION PENDING**. Keep PR #534 Draft; do not touch production, stable Pi, real devices, firmware, show data, or ACL activation.

## Latest continuation — SC-024 live-source mutation result fencing

- Starting local and remote HEAD: `61a84da0d86e62bab9545ef9dbb2a3d8516f7e26`; that exact SHA has completed Core, Companion Core, and Camera Relay CI successfully. PR #534 remains OPEN / DRAFT. No CI run is active for the current remote HEAD.
- A red-first isolated HTTP regression reproduced `PUT /api/v1/projects/{project_id}/live-video-sources/{source_id}` returning HTTP 200 and private source details after the operator's `DeviceConfigure` grant was revoked after the SQLite write committed.
- The handler now revalidates the live browser session, global ProjectEdit permission, and scoped DeviceConfigure grant before returning the source record. If authority was revoked, it emits a non-retryable committed-effect response and keeps the already-committed source stored.
- `TestProjectLiveVideoSourceUpdateSuppressesCommittedResultAfterGrantRevocation` proves the old leak, confirms the update committed, and then verifies the scoped result is suppressed. It uses temporary SQLite and a fake response-boundary hook; it performs no network or device I/O.
- Local verification passes: focused tests for the new mutation, existing live-source read revocation, and Stage Device show-requirement result; the same set under Race; `go vet ./internal/httpapi`; `git diff --check`.
- This code + matrix + checkpoint batch is local and unpushed. Before pushing, recheck remote `integration/stagecore-rc` still equals `61a84da...`; then push normally to the existing branch and wait for exact-head Core, Companion Core, and Camera Relay workflows. Do not retag the already-qualified `stagecore-rc-lab-2026-11-16-r5` at `61a84da...`; a new tag is only appropriate after the new SHA's checks pass.
- This closes one SC-024 response-delivery boundary only. Whole-server ACL delivery/cutover remains incomplete; Project ACL stays disabled. Pi ARM64 performance remains **PERFORMANCE QUALIFICATION PENDING**. Production, stable Pi, real devices, firmware, and show data remain untouched.

## Latest continuation — Emergency Blackout blocks direct Timecode GO

- Branch: `integration/stagecore-rc`; PR #534 remains open / Draft. Local and remote HEAD before this unpushed change: `438d4c2339f79063f77c2e1a55df5d5654d5c069`.
- A red-first isolated test reproduced an internal Timecode binding executing its Cue Engine GO while the persisted Emergency Blackout latch was active. This direct Timecode path bypasses Runtime Control's admission check.
- The runtime Session lookup now reads the persisted blackout latch in its existing query. Timecode marks a pending automatic GO inhibited before dispatch when the latch is active. The SessionExecutor also blocks Rehearsal/Show actions when an authoritative lookup sees the latch, covering direct execution and later actions in an already-running Cue. SIMULATION remains Digital Twin-only.
- The regression verifies no command record, CueExecution, or cursor advancement occurs when blackout precedes the timecode trigger. A separate SessionExecutor test verifies operational dispatch is denied for both REHEARSAL and SHOW.
- Validation: full `go test` for `internal/timecode`, `internal/simulator`, `internal/store`, `internal/cueengine`, and `internal/runtimecontrol`; focused Timecode and SessionExecutor regressions under Race; `go vet` for timecode/simulator/store; `git diff --check` — pass. No physical executor was used.
- Core run `43825875106` completed successfully for the previous SHA `438d4c2339f79063f77c2e1a55df5d5654d5c069`, including Go 1.26 Race and Linux ARM64 product builds. Companion Core `43825875199` and Camera Relay `43825875097` also passed on that same SHA. The Emergency Blackout fix is a new local, unpushed commit. Recheck the branch, then push normally; wait for Core, Companion Core, and Camera Relay checks on the resulting exact SHA before declaring RC LAB READY.
- Pi ARM64 performance remains **PERFORMANCE QUALIFICATION PENDING**. SC-024 remains an independent NO-GO before full Software Complete. The existing qualified tag `stagecore-rc-lab-2026-11-16-r4` still points to `62c59ad7d28dc9143bf7cdfa6432c9a75fbbc38c`; do not retag it. Production, stable Pi, real devices, firmware, and ACL activation remain untouched.

## Latest continuation — Cue output arbitration across direct callers

- Branch: `integration/stagecore-rc`; existing Draft PR: #534. Starting local and remote HEAD was `62c59ad7d28dc9143bf7cdfa6432c9a75fbbc38c`; that exact SHA's Core, Companion Core, and Camera Relay workflows were already complete and green.
- A red-first simulated regression demonstrated that two direct Cue Engine instances could dispatch different Sessions to the same configured physical output concurrently. Runtime Control's arbitration did not cover those direct Engine entry points.
- Cue Engine now resolves output identities once for the selected immutable Cue group, asks the shared Store to reserve them before creating an execution or advancing the cursor, and releases the reservation after the Cue reaches a terminal result. Runtime Control uses the same resolved identity set. SIMULATION Sessions skip physical-output reservations and continue using only their Digital Twin. Unknown physical identity remains a wildcard.
- `TestCueEngineDirectGORejectsOutputHeldByAnotherSession` proves the second direct GO is rejected before creating an execution or advancing its Session, then succeeds after the first Cue releases the output. Existing nested linked-output conflict coverage also passes.
- Local verification: `go test ./internal/runtimecontrol ./internal/cueengine ./internal/store -count=1`; focused Race regressions for direct/Runtime Control arbitration and Mixed Cue simulation; `go vet` for all three packages; `git diff --check` — all pass. The full three-package Race attempt first exposed the SIMULATION false-positive; after excluding SIMULATION physical reservations, the affected test passed under Race. The Store package's full Race suite passed.
- Apple M3 Max performance spot checks: Store reservation was about 0.57–1.09 µs/op at 10,000 iterations (7–9 allocations/op). Ten-sample simulated 24-Action flat/linked Engine runs recorded P95s around 12.7–14.3 ms; scheduled linked P95 was 24.0–24.6 ms and schedule-jitter P95 1.16–1.94 ms. These small local samples do not qualify Raspberry Pi performance; Pi remains **PERFORMANCE QUALIFICATION PENDING**.
- At the time of this checkpoint, the new batch still requires exact-HEAD Core, Companion Core, and Camera Relay CI. Do not push again while those checks are running. Keep PR #534 Draft. SC-024 remains a separate NO-GO; Project ACL, production services/data, physical outputs, and the stable Pi remain untouched.

## Repository and CI state

- Main repository: `ali96adil/StageCore`; branch: `integration/stagecore-rc`.
- Draft PR #534 remains open and in Draft state.
- Verified remote base before this batch: `b96607b1ab59757e9c6e171361bb7e9383db128b`.
- Local batch HEAD: `2835746134643cf169a4498811b25b3d32ff2cac` (one commit ahead before this checkpoint commit).
- Exact-base workflows were complete and green before the new test: Core `43771818890`, Companion Core `43771818941`, Camera Relay `43771818965`.
- No CI was active when the batch was prepared. Do not push another change until CI for the batch's exact remote HEAD finishes.

## Critical-path work and evidence

### Current-source local performance refresh

The simulator-only Apple M3 refresh is recorded in `docs/architecture/cue-execution-performance-qualification.md`. It was repeated because Cue GO began loading Session foundation state and applying the manual-restore gate, and Runtime Control's STOP wait changed from 2 to 5 seconds after the previous Mac baseline. The flat/linked 24-Action P50/P95/P99 remained close to the earlier M3 figures; larger Mixed and linked-chain cases, delayed scheduling, burst/sustained rehearsal, STOP, and Emergency Blackout are tabulated in the report. This run used temporary SQLite roots and a Digital Twin; no physical executor was called. The local process RSS includes fixtures and is not deployed Hub memory. No performance optimization is justified by these Mac results. Raspberry Pi 4 ARM64 timing, CPU/RAM, GC/allocations, and schedule behavior remain **PERFORMANCE QUALIFICATION PENDING**; do not retry SSH without a new isolated-connectivity premise.

Added `TestIsolatedInstalledCopyProjectACLAndVaultRollback` in `internal/update/installed_copy_integration_test.go`. It uses a disposable Linux install, fake systemd/Doctor, local readiness server and temporary SQLite/Vault roots. The test starts from schema 52, seeds a published Snapshot and Vault object, migrates to 53 and stores Project grants, then runs the actual update Service, Installer and Snapshotter. Dry-run must leave state unchanged. A simulated bad candidate migrates to the current schema, then changes grants, deletes the Snapshot, creates a candidate-only Project and tampers with Vault bytes. Rollback must restore the old binary/config and schema 53, SQLite integrity and foreign keys, the original Project/grants/read-only cutover preview, the published Snapshot/Cue manifest and Vault object bytes. Reopening with current code then migrates the restored database forward without losing the grant or Snapshot.

- Linux ARM64 Go 1.26 focused normal test: PASS.
- Linux ARM64 Go 1.26 focused Race test: PASS.
- `go vet ./internal/update`: PASS.
- `gofmt -d` and `git diff --check`: PASS.
- No runtime/product code changed. This Linux container is local Colima, not the Raspberry Pi and not a Pi performance qualification.

## Closure review and disposition

The SC-001–SC-025 matrix and the recent route/delivery checkpoints were reviewed. Existing exact-head evidence covers Mixed and nested Linked Cue GO/JUMP/STOP, delayed nested STOP cancellation, simulated Emergency Blackout protection, and the core recovery/update rollback paths. These unchanged Cue regressions were not rerun. Existing branch CI remains the evidence for them.

SC-024 remains a software NO-GO for whole-server qualification: policy/Owner/plugin route matrices verify the qualified gates, and focused real-handler tests cover several revocation/delivery boundaries, but the complete route-to-policy and asynchronous result-delivery reconciliation across the real server handlers is not closed. Production assembly therefore remains on legacy authorization; the read-only cutover preview is not activation evidence. Do not enable Project ACL in production.

The bounded isolated Pi identity attempt after the owner's clock/connectivity update failed before SSH identity (`websocket: bad handshake`). No remote command or benchmark ran. Current-source Pi ARM64 P50/P95/P99, jitter, CPU/RAM, GC/allocations, burst/sustained Mixed and Linked Chains, and loaded STOP/Blackout remain unqualified. Do not retry absent a new connectivity premise and do not use `stagecore-pi`.

Related PR heads last verified green:

- StageCore-TabletPlayer #36: `91613da1c8e0455a501c7be4b6df8ac1ce73a6bc`.
- StageCore-ESP32-DMX-Lighting #39: `3ad774988b2f78eb385f1046f7668a7f29049ab1`.
- StageCore-ESP32-Camera #20: `455b066c6520e1c8a576d26dd2afd4b377652dde`.
- StageCore-ESP32-StageLaser #56: `b14a5789132549f2724f77b342bbe4c6b0fc880c`; #57: `c4ff10b37dc2fbfd3ebe31e55f0f8d4869d3dc54`.
- These linked repositories were not modified. Their source/build checks do not qualify installed applications or physical output.

## Release status

- **Critical-path software scope:** tested Cue execution and rollback paths remain green on the verified branch; the new installed-copy ACL/Vault rollback regression passes in isolated Linux ARM64.
- **Overall Software Complete:** **NO-GO**. SC-024 whole-server policy/delivery qualification remains open; current-source Pi performance is also not qualified. Remaining matrix items requiring broader installed-app, visual/accessibility, or hardware acceptance are not being expanded during feature freeze; track them from `docs/architecture/software-completion-matrix.md` and the related qualification matrix.
- **Integration / Hardware / Performance Pending:** isolated Pi access and ARM64 measurements; installed Companion/Android/Live/VDMX/StageAudio integration; board, adapter, fixture, camera and physical safety acceptance.
- **Production Release Pending:** owner release decision before merge/deployment. Keep PR #534 Draft; no merge, production ACL activation, production service/data/Vault change, real-device command or firmware flash.

## Resume point

1. Recheck remote branch and PR #534 ancestry, then push the tested test+checkpoint batch normally to `integration/stagecore-rc`.
2. Wait for Core, Companion Core and Camera Relay CI at the exact new SHA; do not interrupt an active run with another push.
3. Continue only a reproducible SC-024 whole-server delivery gap. If none is demonstrated, keep the overall NO-GO status explicit and stop at feature freeze; do not add secondary improvements.

## SC-024 continuation — machine-role mutation delivery

- Starting local and remote HEAD: `c8218938f54acdc592e3d821a369adf4fcb6b160`; PR #534 is still OPEN / DRAFT. Core `43792209563`, Companion Core `43792209517`, and Camera Relay `43792209633` for that starting HEAD are all completed successfully; no run was active before preparing this batch.
- Added post-commit session and `DeviceConfigure` grant revalidation before returning results from all eight machine-role mutation operations. The deterministic regression revokes the grant after create commits, expects a non-retryable 403 with no role ID, and verifies the role remains committed.
- Focused `go test ./internal/httpapi -run '^TestOperatorMachineRole' -count=1`, focused HTTP API Race tests, `go vet ./internal/httpapi`, and `git diff --check` pass locally. The Race run for this code/test batch passed in the previous continuation. No media-upload change was retained: a probe that kept the controlling `media.upload` grant active did not demonstrate an authorization gap.
- This handoff is part of the code commit for the batch. On resume, run `git rev-parse HEAD` for its exact local SHA; remote HEAD remains `c8218938f54acdc592e3d821a369adf4fcb6b160` until the normal push succeeds. Push only after rechecking the branch has not advanced, then wait for Core, Companion Core, and Camera Relay on the new exact SHA before another push.
- SC-024 whole-server permission/delivery reconciliation and installed-copy migration/recovery qualification remain NO-GO. Project ACL is still off. Pi ARM64 performance remains **PERFORMANCE QUALIFICATION PENDING**; do not retry the failed SSH path without a new connectivity premise or use the stable Pi. No production, Vault, real-device, firmware, or show-output action was performed.
- Next critical task: after exact-head CI is green, inspect the next concrete post-commit result-delivery boundary in the project-scoped Stage Device mutation handlers; require a reproducible grant-revocation case before changing them. Keep PR #534 Draft and production ACL disabled.

## SC-024 continuation — Stage Device assignment result

- Reproduced a second concrete gap in `PUT /api/v1/projects/{project_id}/stage-devices/{device_id}/show-requirement`: after its authorized SQLite update committed, revoking `DeviceConfigure` before response delivery still produced HTTP 200 with the assignment record.
- In the same Stage Device route family, the Owner-only StageLaser visual-check endpoint returned HTTP 201 and its observation after the Owner session was logged out post-commit. A focused regression now requires a non-retryable committed-effect denial while retaining the observation; the test uses simulated state and makes no device call.
- Added a final `DeviceConfigure`/session recheck and regression coverage. Focused normal tests for machine-role and Stage Device show-requirement results pass; focused Stage Device Race coverage passes; `go vet ./internal/httpapi` and `git diff --check` pass.
- This is local work based on pushed `037fa733c10c45aa80f9ba832196abe8b8983657`. Do not push yet: Core CI run `43798769286` for that pushed SHA is in progress. Companion Core `43798769410` and Camera Relay `43798769295` passed. Push this tested fix only after Core run `43798769286` completes, then wait for all three workflows on the resulting exact SHA.
- The first red-first test confirmed the old HTTP 200/result leak; no device socket or physical target was called. No production changes, ACL activation, firmware, Vault mutation, or show output occurred. Pi performance remains **PERFORMANCE QUALIFICATION PENDING**.
- Next: after the current Core CI completes, push this fix on the same branch, verify Core/Companion/Camera Relay CI, and continue only if another concrete critical-path gap is reproduced. Keep PR #534 Draft.

## #455 — same-output arbitration for concurrent Cues

- Starting local/remote HEAD: `c38b5275a66349a5609615a77ebc12370b9530d3`; PR #534 exact-head Core, Companion Core and Camera Relay checks had passed before this batch.
- Red-first simulated regression reproduced GO of a linked compound Cue dispatching to the same `device_id` through a different alias while the prior Cue remained active.
- Runtime Control now reserves canonical output identities for every enabled action in the fully resolved linked Cue group before creating an execution or advancing the cursor. It recognizes configured device IDs, normalized OSC host/port endpoints, and explicit tablet/output group IDs; generic alias `group_name` is not treated as physical identity. Unresolved physical identity reserves a wildcard. Conflicting GO is rejected with `CUE_OUTPUT_CONFLICT`, while independent configured outputs remain concurrent. STOP/Blackout cancellation continues to persist terminal rows for admitted GO requests.
- Focused and full `go test ./internal/runtimecontrol ./internal/cueengine ./internal/snapshot -count=1`, the same package set under `-race`, `go vet` for all three packages, and `git diff --check` pass. All output tests use a simulated executor; no device or network endpoint is contacted.
- Local Apple M3 Max simulation measured output-admission P95 for a linked 6-Cue/24-Action group at `51.50 µs` on 1 CPU and `23.96 µs` on 4 CPUs after per-group target de-duplication, down from `126.3 µs` / 31,582 B to `51.50 µs` / 8,877 B in the 1-CPU sample. Generic alias `group_name` is excluded from physical identity; explicit group IDs in target configuration remain supported. The 100-Cue/200-Action Runtime Control rehearsal measured GO P50/P95/P99 `2.487/3.018/3.301 ms`; Engine Mixed and scheduled-linked 24-Action cases are recorded in `docs/architecture/cue-execution-performance-qualification.md`. These Mac results do not qualify Pi ARM64.
- Remaining #455 work: early durable schedule acknowledgement, restart recovery semantics, and arbitration ownership across distinct Cue Engine instances/direct routing and timecode callers. This batch does not close #455 or qualify all GO entry points.
- This code+matrix checkpoint is based on the existing `c38b5275a66349a5609615a77ebc12370b9530d3` candidate and will be committed/pushed only to `integration/stagecore-rc`. After the push, record the resulting exact SHA and wait for Core, Companion Core and Camera Relay CI on that SHA before starting another batch. PR #534 stays Draft.
