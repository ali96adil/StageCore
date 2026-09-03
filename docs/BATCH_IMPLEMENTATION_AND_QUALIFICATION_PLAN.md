# StageCore Batch Implementation and Qualification Plan

**Adopted:** 2026-09-03  
**Starting baseline:** `main` `50b8bb3a4fb2536a3a79fe2be4a2a91628878a9e`  
**Starting product state:** F-025 COMPLETE; F-018 is next.

## Goal

Reduce repetitive Raspberry Pi deployments without weakening StageCore verification.

The normal workflow from this point forward is:

**write several logically related software slices -> keep CI green -> freeze one cumulative batch -> deploy once -> run one cumulative physical qualification campaign -> fix only proven physical blockers -> checkpoint -> continue**.

A Raspberry Pi deployment is no longer the default after every merged code slice.

## Non-negotiable rules

1. Every implementation slice still receives its own issue/specification, tests, reviewable PR, and green CI before merge.
2. A merged software slice may be called **software complete / CI verified**, but not physically qualified until its batch gate passes.
3. The authoritative Raspberry Pi is not mutated merely to prove code that can be proven deterministically in CI, unit tests, integration tests, contract tests, simulators or local Companion tests.
4. Physical deployment happens at an explicit batch gate with one frozen exact `main` SHA and verified release hashes.
5. If the cumulative physical gate discovers a real blocker, only the narrow qualification fix is allowed before the batch is re-deployed/retested.
6. Safety, persistence, RBAC, SHOW lock, migration, command authority and fail-closed behavior remain acceptance requirements even when deployment is batched.
7. A hardware/protocol spike may occur before the batch gate only when the missing fact cannot reasonably be proven without the real device. Such a spike must be bounded and must not be relabeled as final product qualification.

---

# Part A — Implementation campaign

During an implementation campaign, work remains on GitHub/CI and supported developer test environments. Do not routinely deploy each merge to the Raspberry Pi.

## Per-slice software gate

For every slice:

1. define exact scope and acceptance in an Issue;
2. implement on a focused branch;
3. add/update deterministic tests;
4. run repository cleanliness checks and relevant local tests where available;
5. open PR;
6. require exact-head CI PASS;
7. merge only when green;
8. require post-merge CI PASS where the current workflow provides it;
9. record the merged SHA and status as **software complete / awaiting cumulative physical gate**;
10. continue to the next planned slice without installing that SHA on the authoritative Pi.

## Batch freeze gate

When all features/slices in the batch are software-complete:

1. stop feature additions to the batch;
2. select one exact `main` SHA as the qualification candidate;
3. verify full CI on that SHA;
4. verify schema/migration chain and backward/upgrade assumptions;
5. build the exact Linux ARM64 release and offline media;
6. verify revision metadata and SHA-256 manifests;
7. create a cumulative physical qualification issue/checklist tied to that exact SHA;
8. only then mutate the authoritative Raspberry Pi.

---

# Part B — Cumulative physical qualification campaign

The physical campaign happens after the software batch is complete.

## Standard qualification order

1. **Pre-deployment safety**
   - current Pi health/Doctor;
   - database integrity;
   - current installed revision;
   - cold rollback/backup checkpoint where required.

2. **Single cumulative deployment**
   - use F-010 supported update/offline path;
   - preserve authoritative environment/config/data/Vault;
   - verify exact installed hashes and revision.

3. **Core regression gate**
   - systemd lifecycle;
   - `/health/ready`;
   - SQLite quick check/migrations;
   - restart/reboot persistence;
   - no unexpected command replay;
   - representative RBAC/SHOW-lock/security path.

4. **Feature acceptance gates**
   - run the real-device acceptance sequence for every feature in the batch;
   - include positive behavior, failure behavior and truthfulness/fail-closed checks;
   - use the real Mac/Companion/tablets/cameras/timecode devices only where the feature requires them.

5. **Cumulative integration gate**
   - prove the new features coexist with the previously qualified baseline;
   - run one representative end-to-end show/rehearsal workflow rather than isolated feature demos only.

6. **Closure**
   - record exact evidence and SHA;
   - mark the batch physically PASS;
   - update `FEATURE_STATUS.md` and strict backlog completion markers only where the full feature is actually complete;
   - create a checkpoint before the next implementation campaign.

## Qualification-fix loop

If physical qualification finds a real defect:

`FAIL evidence -> narrow issue -> narrow fix PR -> CI PASS -> one replacement candidate -> redeploy -> repeat only affected gate + required regression gates`.

Do not mix unrelated improvements into a qualification fix.

---

# Remaining implementation order by cumulative batches

## Batch P3 — Complete Phase 3 before next Pi deployment

**Current starting point:** F-025 COMPLETE.

Write in this order:

### P3.1 — F-018 Universal Timecode & Show Synchronization

Recommended internal sequence:

1. Timecode domain contract: source identity, direction, frame rate, drop-frame, offsets, confidence/health, monotonic correlation.
2. Internal StageCore timecode source/generator.
3. MTC ingest/generation contract and implementation.
4. LTC ingest/generation adapter boundary; keep decode/generation off critical Hub paths where appropriate.
5. Timecode health: missing, stale, jump, discontinuity, drift and unstable-source detection.
6. Cue/timeline trigger binding with command identity, TTL/safety and duplicate-fire protection.
7. Operator UI, Preflight, logging/Flight Recorder, Show Mode behavior and simulation hooks.
8. deterministic CI/integration tests for frame-rate/drop-frame/offset/jump/reconnect cases.

Do not deploy each F-018 slice to the Pi.

### P3.2 — Complete F-028 Timing Intelligence

After F-018 contracts are stable:

1. trusted-session selection and exclusion rules;
2. cue-to-cue/section timing statistics;
3. variation/range/confidence model;
4. early/normal/late live comparison;
5. Expected Next Cue projection;
6. contextual cue/scene notes with lead time;
7. divergence handling that reduces/withdraws confidence rather than inventing precision;
8. Arabic/English operator UI and tests;
9. explicit rule: advisory only, never autonomous GO.

### P3.3 — F-019 Portable Show Capsule / Complete Environment Restore

1. capsule manifest and version contract;
2. project/runtime requirements;
3. Vault/media inclusion policy;
4. F-025 execution-environment assets/snapshots;
5. F-015 extension manifest;
6. F-021 device profiles;
7. themes/workspace/profile references where available;
8. compact manifest-only export;
9. self-contained/offline export;
10. integrity validation before export and after import;
11. restore planning, migration and failure/rollback behavior;
12. replacement-server/workstation readiness report.

### P3.4 — F-011 Show / Profile Templates

Implement last in Phase 3 so templates encode stable configuration rather than freezing temporary schemas.

1. template schema/versioning;
2. bundled official templates;
3. editable materialization into normal StageCore configuration;
4. import/export and compatibility behavior;
5. theatre-video/OSC/projection/rehearsal starter templates;
6. Arabic/English no-code selection flow.

### P3 cumulative physical gate — one Pi deployment

After P3.1–P3.4 are all merged and green:

- freeze one exact Phase 3 candidate SHA;
- deploy it once to the Pi through F-010;
- run cumulative Phase 3 qualification covering F-018 + completed F-028 + F-019 + F-011 plus regression of F-025/Companion/SHOW lock/persistence;
- use real timecode source(s), Mac/Companion and a capsule restore target as required;
- only qualification blockers justify another deployment.

**Phase 3 closes only after this cumulative gate PASS.**

---

## Batch P4 — Device and operator experiences

Write after Phase 3 PASS:

1. F-003 Android Tablet Player integration;
2. F-006 Stage Display / Actor Callboard;
3. F-007 Live Video / Camera Inputs;
4. F-022 Stage Network Cockpit;
5. complete remaining operator-facing F-001 Arabic/RTL polish;
6. complete F-016 theme portability/sync/native-client work;
7. complete F-017 workspace/profile operator polish;
8. enforce F-002 no-code UX across all new workflows.

Then perform one cumulative Phase 4 server deployment and real tablet/display/camera/network qualification campaign. Android APK/client builds may be tested independently without repeatedly replacing the Pi Hub.

---

## Batch P5 — Simulation and recovery

Write:

1. F-024 Full Show Simulation / Digital Twin;
2. F-020 Self-Healing Runtime / optional HA foundation.

Use simulation/fault injection extensively in CI first. Then deploy one cumulative Phase 5 candidate to the Pi and run failure/reconnect/recovery qualification.

---

## Batch P6 — StageCore Visual Engine

Write F-026 against already-stable media/session/capsule/device contracts. Keep rendering off the critical Hub path. Use one cumulative deployment/qualification gate for the Phase 6 server-side integration; render-machine/client testing can iterate separately.

---

## Batch P7 — StageCore Assistant

Write F-023 last. It consumes stable schemas, Flight Recorder, diagnostics, simulation and timing intelligence. The Assistant remains advisory and must never become autonomous GO/emergency/safety authority.

Perform deterministic security/authority testing in CI first, then one cumulative final integration deployment/qualification gate.

---

# Practical deployment policy

From this checkpoint forward, the default rule is:

> **No Pi install after every merged feature or code slice.**
>
> **One frozen cumulative deployment per planned batch/phase, followed by one cumulative physical qualification campaign.**

Exceptions are limited to hardware-only discovery spikes and qualification fixes backed by concrete failure evidence.
