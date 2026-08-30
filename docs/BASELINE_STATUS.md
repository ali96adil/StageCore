# StageCore Document Baseline Status

The ordered StageCore product/architecture/reliability baseline is `00–10`:

1. 00 — StageCore Master Plan v0.2
2. 01 — Architectural Decisions — Addendum 001
3. 02 — StageCore System Architecture v0.1
4. 03 — StageCore Data Model v0.1
5. 04 — StageCore Event & Command Contracts v0.1
6. 05 — StageCore MVP Product Specification v0.1
7. 06 — StageCore Plugin Contract v0.1
8. 07 — StageCore Companion Specification v0.1
9. 08 — StageCore Storage & Vault Specification v0.1
10. 09 — StageCore Security Model v0.1
11. 10 — StageCore Testing & Reliability Plan v0.1

Implementation technology was validated through `SPK-01`–`SPK-06`, and final pre-M0 consistency/entry decisions are recorded in **Architectural Decisions — Addendum 002: Implementation Baseline Finalization**.

## Current Implementation Status

**M0 — CORE PERSISTENCE: COMPLETE**

**M1 — CUE ENGINE + SIMULATOR: COMPLETE**

**M2 — REAL OSC: COMPLETE**

**M3 — ROUTING: COMPLETE**

**M4 — COMPANION + MACHINE ROLE: COMPLETE**

**M5 — STORAGE / VAULT / MEDIA READINESS: COMPLETE**

**M6 — MVP OPERATOR + SECURITY CLOSURE: COMPLETE**

**PHYSICAL RASPBERRY PI ARM64 M0–M6 QUALIFICATION: PASS — ISSUE #21 CLOSED**

**CURRENT ENGINEERING STATE: FEATURE EXPANSION READY**

Completion evidence:

- `docs/checkpoints/2026-08-26-m0-core-persistence-complete.md`
- `docs/checkpoints/2026-08-26-m1-cue-engine-simulator-complete.md`
- `docs/checkpoints/2026-08-26-m2-real-osc-complete.md`
- `docs/checkpoints/2026-08-27-m3-routing-complete.md`
- `docs/checkpoints/2026-08-27-m4-companion-machine-role-complete.md`
- `docs/checkpoints/2026-08-27-m5-storage-vault-complete.md`
- `docs/checkpoints/2026-08-27-m6-software-mvp-complete.md`
- `docs/checkpoints/2026-08-30-m0-m6-physical-qualification-complete.md`

Merged product commits:

```text
3b300ccf2549f417b3f86c4de841a4530902f9ca
m0: establish core persistence foundation

a5af7c269d516055831720fb4055276457757001
m1: implement cue engine and deterministic simulator

56feab35b7ec65fed4047bc106c12c30899adf0c
m2: implement real OSC capability path

7f573c32d8e8bbad151105025900869cf8eee5
m3: implement deterministic routing runtime

d2dab103fff7979953ac3c1af096d9bb4245d1de
m4: implement Companion and Machine Role runtime

99552d6d58512836ea325393812d52dbbded6f1d
M5 Storage / Vault / Media Readiness (#23)

268b499856aa45ee7650ff66ab28d46f2f195c7b
M6 MVP Operator + Security Closure (#28)
```

Latest merged-main software verification:

- M6 final tested branch commit: `86bb56f835265f982bfd7f9929d499dd2100cd19`;
- final tested M6 tree: `45a32de4e8989b8be0699fb45641d424fed73c05`;
- M6 merge commit: `268b499856aa45ee7650ff66ab28d46f2f195c7b`;
- merged M6 tree: `45a32de4e8989b8be0699fb45641d424fed73c05` — byte-identical to the final tested branch tree;
- pre-merge Core CI #311 — PASS;
- pre-merge Companion Core CI #146 — PASS;
- post-merge Core CI #312 — PASS;
- post-merge Companion Core CI #147 — PASS;
- Linux ARM64 CGo-free product builds — PASS;
- real macOS Companion replacement acceptance — PASS;
- `>=2 GiB` interrupted/resumed media transfer acceptance — PASS;
- integrated fresh-Hub → OWNER → Project/configuration → Publish → REHEARSAL → GO → Note → restart/history software-MVP acceptance — PASS.

Latest physical qualification verification:

- Issue #21 — `PASS — Raspberry Pi ARM64 M0–M6 Qualification Gate` — CLOSED on 2026-08-30;
- tested host: Raspberry Pi 4 Model B Rev 1.4, 8 GB class RAM, Debian GNU/Linux 13.5, kernel `6.18.39+rpt-rpi-v8`, native `aarch64`;
- authoritative storage: Kingston SA400S37480G 480 GB-class SATA SSD over USB, ext4 root, with independent StageCore Data/Vault roots;
- native ARM64 systemd lifecycle, SQLite/WAL persistence, authenticated Operator Web/RBAC, immutable Runtime Snapshots, OSC input/output, authenticated macOS Companion/Machine Role, HTTP/Script/macOS MIDI, Vault/media readiness, `>=2 GiB` interrupted/resumed transfer, SHOW bulk protection, backup/restore, WAN-disconnected local-first operation, clean reboot recovery and representative CPU/RAM/disk/thermal pressure — PASS;
- controlled power-loss/recovery — PASS on disposable/reference microSD storage only; the authoritative Kingston SSD was not subjected to destructive power-loss testing;
- no unresolved Raspberry Pi-specific blocker remains from the qualification gate.

### Accepted Technology Direction

- **SPK-01 — Core Technology Stack** — Go Hub; SQLite/WAL; HTTP+JSON; authenticated local browser/API surface; embedded offline Operator UI.
- **SPK-02 — Real OSC** — OSC 1.0 UDP `osc.send`; logical endpoint resolution; truthful `TRANSPORT_ONLY` acknowledgement.
- **SPK-03 — macOS Companion** — Swift CompanionCore; versioned authenticated WebSocket runtime channel; Machine Role/Snapshot reconciliation; duplicate/stale execution protection; Keychain-backed device identity.
- **SPK-04 — Plugin Process / IPC** — external Plugin process; JSON Lines stdio IPC; capability handshake; crash/hang containment; no automatic replay.
- **SPK-05 — Vault & Large File Transfer** — filesystem Vault objects; SHA-256 identity; staging/atomic promotion; HTTP Range/resume; verified Companion cache; SHOW transfer gate.
- **SPK-06 — Hub Deployment on ARM64 / Mini-PC** — 64-bit Linux; native `amd64`/`arm64`; systemd; local-first boot; independent Data/Vault roots for SSD/NVMe. The selected Raspberry Pi qualification configuration has passed; other hardware/storage/power/network combinations still require their own qualification evidence before equivalent claims are made.

## Delivered Product Foundation

### M0 delivered

- real Go Hub product source under `cmd/` + `internal/`;
- SQLite/WAL persistence with required safety/durability settings;
- embedded migrations and persisted UUIDv7 identities;
- Project/Revision/Cue/Action and foundational target/input/output/route persistence;
- frozen-revision guards, FK/transaction evidence and restart/reopen persistence;
- Go CI, race evidence and Linux ARM64 CGo-free build evidence.

### M1 delivered

- production Command/Event envelopes and persisted Event journal;
- immutable Runtime Snapshot identity using canonical JSON + SHA-256;
- Session/CueExecution/ActionExecution history;
- deterministic COMPLETE/FAIL/TIMEOUT simulator;
- sequential, parallel and barrier Cue execution;
- duplicate/idempotency protection and restart-safe no-replay;
- runtime uses Snapshot-captured definitions rather than mutable live state.

### M2 delivered

- generic capability executor/registry;
- immutable Snapshot logical-target resolution;
- real `osc.send` OSC 1.0 UDP typed-argument capability;
- truthful `TRANSPORT_ONLY` acknowledgement;
- external `stagecore.osc` Plugin with versioned JSON Lines IPC;
- capability/permission checks, crash/hang containment and no replay after Plugin failure;
- ARM64 CGo-free Hub + OSC Plugin builds.

### M3 delivered

- deterministic Routing from typed Test/OSC inputs;
- Snapshot-only Route lookup;
- bounded conditions/transforms/debounce;
- persistent Route Trace and explicit failures;
- Route → Cue through validated command path;
- Route → Output through generic capability registry;
- real Route → OSC path;
- typed input authority and external OSC receive isolation;
- duplicate/restart/no-replay preservation and routing latency evidence.

### M4 delivered

- stable Companion identity independent of hostname/IP;
- MachineRole + one-active assignment;
- truthful readiness and heartbeat → OFFLINE;
- RuntimeSnapshotID propagated through Cue/Route execution;
- P-256 Keychain-backed macOS identity;
- explicit pairing, challenge/response authentication and bounded runtime sessions;
- authenticated Hub WebSocket runtime;
- stale Snapshot/wrong-role/unsupported/duplicate rejection;
- revocation removes runtime authority;
- headless real `stagecore-companion` executable;
- real macOS `osc.send` with truthful `TRANSPORT_ONLY`;
- real macOS replacement/no-replay acceptance with distinct Keychain identities.

### M5 delivered

- independent configurable Hub Data Root and Vault Root;
- SQLite metadata for immutable Vault objects, logical MediaAssets and content versions without storing large blobs in DB;
- staged streaming imports with SHA-256 content identity and atomic no-overwrite promotion;
- content-addressed immutable Vault object storage;
- authenticated disk-backed HTTP Range serving;
- macOS `.part` cache with bounded chunks and restart-resume behavior;
- exact size + SHA-256 verification before final media promotion;
- Required Media captured by immutable Runtime Snapshot identity;
- truthful READY / BLOCKED / MISMATCH media readiness;
- local Software Repository with platform/arch/API/checksum/signing/notarization/channel metadata;
- local Downloads/Setup path that does not depend on WAN/Internet access;
- bulk-job policy isolated from P0/P1 runtime;
- SHOW blocks/pauses nonessential transfer/software/backup/archive work while P1 Cue execution remains functional;
- free-space health with reference warning threshold;
- configurable runtime reserve with 2 GiB default and write admission enforcement;
- verified consistent state backup, tamper evidence and non-destructive restore;
- `>=2 GiB` forced-interruption transfer acceptance with HTTP Range resume, exact SHA-256 and atomic promote.

### M6 delivered

- persistent Hub identity, fingerprint and first OWNER bootstrap;
- mature password hashing and one-time setup-code behavior;
- local login/logout, bounded sessions, RBAC and CSRF/session protections;
- secure non-loopback browser/API transport policy and authenticated realtime/browser channel;
- embedded WAN-independent Operator Web;
- Project create/open and operator-supported Target/Input/Output/Route/Cue editing;
- validation + immutable Publish workflow;
- Dashboard, Runtime, Preflight, Notes and Session Memory;
- REHEARSAL/SHOW runtime controls through normal command/idempotency paths;
- authoritative Companion/media/storage/security readiness and SHOW gating;
- structured execution history and truthful restart interruption reconciliation;
- basic HTTP Action, macOS MIDI Action and isolated Script Action with explicit bounded outcomes;
- encrypted Secret Store and `secret_ref` policy;
- explicit first-party Plugin permissions;
- security audit and denial-path coverage;
- SHOW security administration gates with bounded emergency revocation;
- local user/session/Companion revocation and renewal without Internet;
- integrated software-MVP acceptance from fresh Hub through restart/history without manual DB/file editing.

## Completed Physical Raspberry Pi Qualification Gate — Issue #21

Issue #21 physically exercised the complete M0–M6 baseline on the selected Raspberry Pi/Kingston SSD/Stage LAN configuration and closed as **PASS** on 2026-08-30.

The proven physical boundary included:

```text
Pi 64-bit Linux / arm64
→ native StageCore ARM64 product binaries
→ systemd/local-first startup
→ Data Root + independent Vault Root on intended storage
→ SQLite WAL restart/reopen
→ first OWNER bootstrap + authenticated Operator Web/RBAC
→ Project/configuration/Publish workflow
→ real OSC input/output bench paths
→ basic HTTP/Script safe bench paths
→ real macOS Companion pair/auth/connect
→ Machine Role + Runtime Snapshot readiness
→ macOS MIDI safe bench path
→ Cue + Route runtime execution
→ disconnect/reconnect and Hub restart with no replay
→ managed Vault import + SHA-256
→ real LAN >=2 GiB media transfer + interruption/resume + checksum
→ Companion required-media READY/BLOCKED/MISMATCH transitions
→ SHOW bulk/admin gates while P0/P1 runtime remains functional
→ runtime reserve / low-space behavior
→ Notes/session history across restart
→ backup/restore drill
→ WAN-disconnected local operation
→ clean reboot/service recovery
→ controlled power-loss/recovery on disposable/reference storage
→ CPU/RAM/disk/thermal observation under representative pressure
```

Known non-blocking qualification observations remain recorded rather than hidden:

- an initial historical undervoltage event occurred during native build load; after the power lead was corrected, fresh boots and later pressure tests remained at `vcgencmd get_throttled => 0x0` with no new undervoltage evidence;
- representative full-core thermal pressure reached about `77.4°C` without throttling, so adequate cooling/airflow remains an operational requirement;
- `eth0` and `wlan0` were simultaneously active on the same Stage LAN during qualification, creating route/interface ambiguity as an operational nuance rather than a product blocker; show deployments should deliberately choose the intended control interface/route.

This PASS is specific to the tested configuration. It does not automatically qualify every Raspberry Pi, SSD/NVMe, power supply, thermal enclosure, router, Stage LAN, or future StageCore build. Equivalent deployment claims still require evidence appropriate to the changed configuration and the Testing & Reliability baseline.

## Feature Expansion Entry State

The pre-expansion review of `docs/FEATURE_BACKLOG.md` together with `docs/FEATURE_IMPLEMENTATION_ORDER.md` found no new physical-qualification dependency that requires changing the dependency-first order.

The canonical next implementation sequence therefore remains:

1. **F-027 — Rehearsal & Show Session Modes** — establish the shared session/state/checkpoint semantics first.
2. **F-028 — Rehearsal Timing Intelligence** — begin the timing-capture foundation as soon as F-027 sessions are real; predictive UI remains a later slice.

No future feature is marked complete by this transition checkpoint.

## Remaining Work Is Owned — Not Unbounded TBD

- feature expansion now proceeds through the canonical dependency-first order in `docs/FEATURE_IMPLEMENTATION_ORDER.md`;
- production macOS signing/notarization/background packaging remains a later product gate;
- final appliance SKU and any additional power/thermal/storage/network combinations remain later hardware/deployment qualification work and must not inherit the Issue #21 PASS automatically;
- Hardware Nodes, full DMX/lighting automation, AI/Vision, HA/cloud and distributed offline authority remain explicitly later/post-MVP work.

Every known deferred item remains assigned in `docs/adr/addendum-002/04-deferred-register-and-ownership-gates.md`.

Changes to an established decision require an explicit superseding ADR/decision with evidence; implementation must not silently drift the baseline.
