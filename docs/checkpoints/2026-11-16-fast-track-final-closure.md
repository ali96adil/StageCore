# Fast-track final closure checkpoint — 2026-11-16

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
