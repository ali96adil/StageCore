# 2026-08-30 — M0–M6 Physical Qualification Completion Checkpoint

## Status

**M0–M6 SOFTWARE MVP: COMPLETE**

**RASPBERRY PI ARM64 PHYSICAL QUALIFICATION: PASS**

**PROJECT STATE: FEATURE EXPANSION READY**

This checkpoint closes the verification gate required by `docs/FEATURE_IMPLEMENTATION_ORDER.md` before future-feature implementation begins.

It records the transition from:

```text
M0–M6 complete → physical qualification active
```

to:

```text
M0–M6 complete → physical qualification PASS → feature expansion ready
```

No future feature is implemented or marked complete by this checkpoint.

## Source state reviewed

At checkpoint preparation, GitHub `main` was verified at:

- `81fe42e99608d8cde1fc55b4fca2ac7b0d9a0f35` — `docs: add verification gate and timing foundations`.
- No open pull request existed.
- No open issue existed.
- Issue #21 was closed as `PASS — Raspberry Pi ARM64 M0–M6 Qualification Gate` on 2026-08-30.
- `main` had no configured required status-check contexts in branch protection; however, the repository `Core CI` workflow runs on every pull request and remains the applicable CI gate for this documentation transition. `Companion Core CI` is path-filtered and is not expected for docs-only changes.

The physical qualification began from the M6 checkpoint baseline and incorporated bounded product fixes discovered by the real hardware gate. The last runtime/product fix before the later documentation-only commits was `70e1562f077056ef77a36fd877447b32155881bc` (`fix: keep macOS Companion heartbeat current (#38)`). The later commits through the pre-expansion `main` above are documentation-only and do not change the physically qualified runtime behavior.

## M0–M6 software completion

The software MVP completion remains established by the dated M0–M6 checkpoints and their merged CI evidence. In particular, the M6 completion checkpoint records the authenticated Operator, Security, runtime, Companion, Vault/media, restart/no-replay and local-first software-MVP acceptance boundary.

This checkpoint does not rewrite those historical records.

## Physical qualification result

GitHub Issue #21 is the authoritative detailed evidence record. Its final state is:

**PASS — PHYSICAL QUALIFICATION COMPLETE.**

The selected physical reference configuration successfully exercised the complete M0–M6 path on real hardware rather than relying only on CI, emulation or cross-build evidence.

### Tested hardware and deployment boundary

- Raspberry Pi 4 Model B Rev 1.4.
- 8 GB class RAM (`7.6 GiB` usable reported during qualification).
- Debian GNU/Linux 13.5 (trixie).
- Kernel `6.18.39+rpt-rpi-v8`.
- Native `aarch64` / ARM64 execution.
- Kingston SA400S37480G 480 GB-class SATA SSD over USB as the authoritative system/StageCore storage.
- ext4 root on `/dev/sda2`; boot firmware on `/dev/sda1`.
- StageCore Data Root `/var/lib/stagecore/data` and Vault Root `/var/lib/stagecore/vault` as independent sibling roots on the intended SSD.
- Dedicated `stagecore` service account and systemd deployment.
- Bounded local Stage LAN with real Raspberry Pi ↔ macOS Companion traffic.
- Controlled power-loss/recovery performed on disposable/reference microSD storage, not on the authoritative Kingston SSD.

This is the qualification boundary that passed. The PASS must not be generalized automatically to every Raspberry Pi model, storage device, power supply, thermal enclosure, router, network topology or future StageCore build.

## Major categories physically proven

Issue #21 physically proved the representative supported M0–M6 paths, including:

- native ARM64 build/deployment and systemd lifecycle;
- SQLite/WAL persistence and restart/reopen behavior;
- first OWNER bootstrap, authenticated Operator Web, login/logout and RBAC denial paths;
- Project/configuration validation and immutable Runtime Snapshot publish behavior;
- Notes/session history persistence;
- real OSC output with truthful transport acknowledgement;
- supported OSC input → Routing → Cue execution;
- authenticated real macOS Companion and Machine Role readiness;
- Runtime Snapshot/config matching and mismatch protection;
- Cue/Route → Machine Role → Companion execution with explicit results;
- bounded HTTP, isolated Script and macOS MIDI action paths;
- Companion disconnect/reconnect and Hub restart without duplicate/replay;
- managed Vault import and SHA-256 content identity;
- real LAN `>=2 GiB` transfer, forced interruption, `.part` preservation, resume, checksum verification and atomic promotion;
- required-media `READY` / `BLOCKED` / `MISMATCH` behavior;
- runtime reserve / storage-health behavior;
- SHOW-mode bulk protection while P0/P1 runtime remained functional;
- backup/restore against the intended filesystem;
- WAN-disconnected local-first operation;
- clean reboot/service recovery;
- representative CPU/RAM/disk/thermal pressure;
- Pi-specific deployment/permission/filesystem/network review;
- controlled power-loss and recovery on disposable/reference storage with filesystem and SQLite integrity recovery.

## Non-blocking qualification observations

The following observations remain part of the qualification record and are not hidden by the PASS:

1. **Historical undervoltage event** — one undervoltage/throttling event occurred during the initial native build. The power lead was corrected; subsequent fresh boots and pressure tests remained at `vcgencmd get_throttled => 0x0` with no new undervoltage evidence.
2. **Thermal headroom** — representative full-core pressure reached about `77.4°C` without throttling. This is not a product blocker, but adequate cooling and airflow remain part of a sound deployment.
3. **Dual same-LAN interfaces** — both `eth0` and `wlan0` were active on the same Stage LAN during qualification, with the default route via Wi-Fi. The resulting route/interface ambiguity is an operational nuance, not a StageCore blocker; show deployments should deliberately select the intended control interface/route.

No unresolved Raspberry Pi-specific StageCore blocker remains from Issue #21.

## Qualification scope statement

Passing Issue #21 proves the selected Raspberry Pi / Kingston SSD / power-corrected / Stage LAN configuration for the tested M0–M6 baseline.

It does **not** constitute universal qualification of:

- every Raspberry Pi generation or RAM size;
- every SSD/NVMe/USB bridge/filesystem;
- every power supply or enclosure/cooling design;
- every router/AP or multi-interface topology;
- every rehearsal/show Project configuration;
- future builds after product behavior changes.

Changed deployment boundaries remain subject to the applicable Testing & Reliability evidence and ownership gates.

## Pre-expansion dependency review

`docs/FEATURE_BACKLOG.md` and `docs/FEATURE_IMPLEMENTATION_ORDER.md` were reviewed together after physical qualification.

Qualification exposed real bounded defects during the M0–M6 gate, and those defects were fixed and physically re-qualified rather than bypassed. None of the final qualification findings introduces a new unresolved architectural dependency that requires reordering the confirmed future features.

Therefore:

- the existing dependency-first implementation order remains valid;
- stable `F-xxx` IDs remain unchanged;
- the enabling session/state/checkpoint, command identity, live-state, persisted-schema, health/readiness, observability/Flight Recorder, clock/time-health and performance-budget foundations remain the intended small shared primitives described by `docs/FEATURE_IMPLEMENTATION_ORDER.md`;
- no separate Pi-specific feature must be inserted ahead of Phase 1 as a blocker.

## Exact next implementation order

The first implementation target remains:

1. **F-027 — Rehearsal & Show Session Modes** — first establish session identity, mode, start position, stop/resume state and truthful state-reconstruction/checkpoint semantics.
2. **F-028 — Rehearsal Timing Intelligence & Expected Next Cue — capture foundation** — once F-027 sessions are real, begin trustworthy timing/event capture through the canonical observability path; predictive UI remains a later slice.

This checkpoint intentionally does **not** implement F-027 or F-028.

## Transition statement

The StageCore pre-expansion verification gate is closed.

The engineering state after this checkpoint is:

**FEATURE EXPANSION READY — begin with F-027 under the existing architecture/contracts and dependency-first order.**

Historical M0–M6 and qualification checkpoints remain immutable historical evidence; they are not rewritten merely to make them look current.
