# 2026-08-27 — M6 Software MVP Completion Checkpoint

## Status

**M6 — MVP OPERATOR + SECURITY CLOSURE: COMPLETE**

This checkpoint records completion of the StageCore software MVP on merged `main`. It does **not** claim Raspberry Pi, SSD/NVMe, power, thermal, Stage LAN, rehearsal, or show qualification. Those remain owned by Issue #21.

## Product merge

- Milestone issue: #25 — `M6 — MVP Operator + Security Closure`
- Product PR: #28 — `M6 MVP Operator + Security Closure`
- Final tested branch commit: `86bb56f835265f982bfd7f9929d499dd2100cd19`
- Final tested tree: `45a32de4e8989b8be0699fb45641d424fed73c05`
- Squash merge commit on `main`: `268b499856aa45ee7650ff66ab28d46f2f195c7b`
- Merged `main` tree: `45a32de4e8989b8be0699fb45641d424fed73c05`
- The tested branch tree and merged tree are byte-identical.

## Final CI evidence

Pre-merge on the final tested tree:

- Core CI #311 — PASS
  - Go 1.26 and Go 1.27 tests
  - vet
  - Go 1.26 race tests
  - Linux ARM64 CGo-free product builds
- Companion Core CI #146 — PASS
  - Swift package build/tests
  - real macOS Companion build/replacement acceptance
  - `>=2 GiB` interrupted/resumed media transfer acceptance

Post-merge on `main` merge commit `268b499856aa45ee7650ff66ab28d46f2f195c7b`:

- Core CI #312 — PASS
- Companion Core CI #147 — PASS

## Delivered M6 software MVP

M6 closes the operator/security gap over the completed M0–M5 runtime/storage foundation:

- persistent Hub identity and stable fingerprint;
- one-time first OWNER bootstrap with explicit local setup-code generation and supported embedded first-run claim flow;
- local login/logout, bounded sessions, OWNER/TECHNICIAN/OPERATOR/VIEWER RBAC and CSRF/session protections;
- secure non-loopback browser/API transport policy and authenticated realtime/browser channel;
- embedded, WAN-independent Operator Web for Projects, Dashboard, Configuration, Cues, Runtime, Preflight, Notes, Session Memory and Security operations;
- operator-supported Target/Input/Output/Route/Cue configuration and Publish without manual DB/file editing;
- immutable Runtime Snapshot semantics preserved through runtime controls;
- REHEARSAL/SHOW controls with Current/Next, GO, STOP and confirmed Jump through normal command/idempotency paths;
- authoritative PASS/WARN/BLOCK Preflight across Snapshot, capability, Companion/Machine Role, media and storage state;
- Notes and Session Memory with structured Cue/Action execution history and truthful restart interruption reconciliation;
- basic `http.request`, macOS Companion `midi.send`, and isolated `script.run` actions with bounded, explicit results and no hidden replay;
- encrypted Secret Store and `secret_ref` policy;
- explicit first-party Plugin permissions;
- security audit for login/logout, user/role administration, Companion trust/revocation, secret/permission administration, authorization denial and SHOW-critical administration;
- dangerous security administration blocked during SHOW while bounded P0/P1 runtime remains available;
- local session renewal/revocation and emergency revocation paths without Internet dependency.

## Integrated software-MVP acceptance

`TestSoftwareMVPOperatorWorkflowSurvivesHubRestart` proves the supported authenticated local workflow end-to-end:

```text
Fresh unclaimed Hub
→ one-time OWNER bootstrap
→ OWNER login
→ create/open Project
→ create Target/Input/Output/Route
→ create Cue
→ Publish immutable Runtime Snapshot
→ start REHEARSAL
→ GO
→ explicit COMPLETED result
→ create linked Note
→ Hub restart
→ interrupted active Session reconciled truthfully as ABORTED
→ execution history preserved
→ Note history preserved
→ bootstrap replay rejected
```

Separate regression and acceptance coverage keeps the real OSC path, HTTP/Script actions, macOS MIDI/Companion execution, secure denial paths, SHOW Preflight blockers, Vault/media resume, no-replay behavior and ARM64 product builds green.

## Completion statement

The **StageCore M0–M6 software MVP is COMPLETE on `main`**.

That statement means the software can now be operated through its supported authenticated local interface without manual database/file editing, while preserving the core runtime, routing, Companion, Vault/media, security and restart/no-replay invariants proven by CI and hosted macOS acceptance.

It does not mean the physical appliance is rehearsal-ready or show-ready.

## Next gate

**Issue #21 — Raspberry Pi ARM64 M0–M6 Qualification Gate** becomes the active engineering gate.

The next evidence must come from real ARM64 hardware and intended local storage/network conditions, including native binaries, Data/Vault roots, SQLite/WAL restart, Operator Web/auth, real OSC, real macOS Companion, Vault/media transfer and resume, SHOW gates, storage pressure/reserve, backup/restore, WAN-disconnected operation, reboot/power-recovery and CPU/RAM/disk/thermal observation.

No new implementation milestone should be used to bypass this physical qualification gate unless a concrete hardware test exposes a product defect that must be fixed first.
