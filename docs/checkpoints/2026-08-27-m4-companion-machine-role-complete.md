# StageCore — M4 Companion + Machine Role Completion Checkpoint

**Date:** 2026-08-27  
**Implementation slice:** M4 — Companion + Machine Role  
**Status:** COMPLETE  
**Merged product commit:** `d2dab103fff7979953ac3c1af096d9bb4245d1de`  
**Pull request:** #19  
**Tracking issue:** #18 — closed as completed

## 1. Completion Decision

M4 is accepted as complete.

StageCore now has a real authenticated macOS Companion runtime built on the existing logical target/capability model. The Hub owns stable Companion identity/trust/assignment/readiness, dispatches runtime work to the assigned Machine Role over an authenticated WebSocket session, records explicit results, and supports safe reconnect/replacement without replay or Cue/Route edits.

**Decision:** `M4 COMPLETE — M5 STORAGE / VAULT IS NEXT`.

## 2. Delivered Runtime Path

```text
Cue / Route
→ real capability (for example osc.send)
→ logical target VIDEO-MAIN
→ Machine Role assignment
→ trust + readiness + Runtime Snapshot validation
→ authenticated Companion WebSocket
→ real macOS Companion
→ bounded local executor
→ explicit truthful result
```

Delivered M4 evidence includes:

- stable Companion identity independent of hostname/IP;
- Project MachineRole and one-active-Companion assignment semantics;
- truthful PASS/WARN/BLOCK readiness and heartbeat -> OFFLINE behavior;
- RuntimeSnapshotID propagation through Cue and Route capability execution;
- stale Snapshot and wrong-role rejection;
- explicit completed/failed/timed-out/interrupted outcomes;
- reconnect reconciliation with no automatic replay;
- secure P-256 macOS device identity with private material in Keychain;
- explicit short-lived pairing approval;
- challenge/response authentication and bounded runtime sessions;
- revocation removing active/new runtime authority;
- headless `stagecore-companion` executable;
- authenticated Hub runtime WebSocket;
- bounded `local.echo` transport proof;
- real macOS `osc.send` UDP executor with truthful `TRANSPORT_ONLY` acknowledgement;
- capability reporting based on actual local executor configuration;
- authenticated replacement preserving the same immutable Cue and Runtime Snapshot;
- real macOS process acceptance using two distinct Keychain-backed Companion identities.

## 3. Final Verification Evidence

Final reviewed M4 head:

`3c129a384e1d51ab87a4cc525154597dd1380240`

Final reviewed tree:

`11447ef7157e81dc434edc33fee3ceaee8e3ad64`

Pre-merge:

- Core CI #78 — PASS;
- Companion Core CI #24 — PASS;
- `Real macOS Companion replacement` on macOS 15 / Swift 6 — PASS.

PR #19 was squash-merged to `main` as:

`d2dab103fff7979953ac3c1af096d9bb4245d1de`

Merged tree:

`11447ef7157e81dc434edc33fee3ceaee8e3ad64`

The merged tree is byte-identical to the final reviewed/tested M4 tree.

Post-merge:

- Core CI #79 — PASS;
  - Go 1.26 module lock, tests, vet, race and Linux ARM64 CGo-free product builds — PASS;
  - Go 1.27 module lock, tests and vet — PASS;
- Companion Core CI #25 — PASS, including real macOS Companion replacement acceptance.

## 4. Raspberry Pi / ARM64 Status at This Checkpoint

The current `main` is ready for a **bounded native Raspberry Pi smoke deployment now**, before M5 begins.

Why this is an appropriate checkpoint:

- the Hub, OSC Plugin and pairing CLI have Linux ARM64 CGo-free CI build evidence;
- M0 persistence, M1 Cue runtime, M2 real OSC, M3 Routing and M4 Companion runtime are all merged and green;
- the Pi test can therefore validate the complete current control/runtime foundation on physical ARM64 hardware rather than waiting for media/Vault work.

This smoke deployment must not be confused with final hardware qualification. SPK-06 explicitly requires physical ARM64 qualification, and the First Rehearsal gate later requires SSD/NVMe, power-loss/recovery, at least 2 GiB interrupted/resumed media transfer, storage-pressure/thermal soak, Stage LAN failure/recovery and backup/restore evidence.

Recommended pre-M5 Pi smoke boundary:

1. native 64-bit Linux boot and binary execution;
2. persistent Data Root + SQLite/WAL reopen/restart;
3. Hub startup/shutdown/restart under systemd or equivalent temporary service setup;
4. real OSC send/receive path on the local Stage LAN or safe bench network;
5. macOS Companion pair/auth/connect, Machine Role assignment and runtime execution;
6. Companion disconnect/reconnect and no-replay check;
7. WAN-disconnected local runtime smoke;
8. CPU/memory/temperature observation during a short bounded run.

Passing this smoke gate means the current M0–M4 runtime foundation works natively on the selected Pi. It does **not** yet mean the Pi is rehearsal-qualified or show-ready.

## 5. Transition to M5

M5 owns the first real Storage/Vault and media-aware runtime slice.

Reference implementation order from the Storage/Vault specification:

```text
S0 Storage Root + DB persistence
→ S1 Vault object import + checksum
→ S2 File download/transfer jobs
→ S3 Companion media cache sync
→ S4 Hub Software Downloads
→ S5 SHOW traffic gates + capacity reserve
→ S6 Backup/restore proof
```

The first M5 slice must preserve the M0–M4 runtime authority and must not make media READY without content-identity/checksum evidence.

## 6. Explicitly Deferred Beyond M4

M4 does not claim completion of full media transfer/cache, Hub software-download UI, production macOS signing/notarization, background launch packaging, Windows/Linux Companion, cloud relay, distributed offline show authority, rich Operator UI, Hardware Nodes or full lighting/DMX product work.

Those remain owned by their documented later slices/gates.
