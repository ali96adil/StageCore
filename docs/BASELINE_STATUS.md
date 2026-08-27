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
**M6 — MVP OPERATOR + SECURITY CLOSURE: IN PROGRESS**  
- **S0 Hub identity + first OWNER bootstrap: COMPLETE**
- **S1 local authentication + RBAC + authenticated browser transport: COMPLETE**
- **S2 Operator Web foundation: IN PROGRESS**

**NEXT HARDWARE GATE AFTER M6: PHYSICAL RASPBERRY PI M0–M6 QUALIFICATION — ISSUE #21**

Completion evidence:

- `docs/checkpoints/2026-08-26-m0-core-persistence-complete.md`
- `docs/checkpoints/2026-08-26-m1-cue-engine-simulator-complete.md`
- `docs/checkpoints/2026-08-26-m2-real-osc-complete.md`
- `docs/checkpoints/2026-08-27-m3-routing-complete.md`
- `docs/checkpoints/2026-08-27-m4-companion-machine-role-complete.md`
- `docs/checkpoints/2026-08-27-m5-storage-vault-complete.md`
- `docs/checkpoints/2026-08-27-m6-s1-authenticated-browser-transport.md`

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
```

Latest completed M6 slice verification:

- S0 Core CI #165 — PASS;
- S1 Core CI #174 — PASS;
- S1 module lock, Go 1.26/1.27 tests, vet, race and Linux ARM64 CGo-free product builds — PASS.

### Accepted Technology Direction

- **SPK-01 — Core Technology Stack** — Go Hub; SQLite/WAL; HTTP+JSON; SSE browser events; TypeScript + React + Vite UI.
- **SPK-02 — Real OSC** — OSC 1.0 UDP `osc.send`; logical endpoint resolution; truthful `TRANSPORT_ONLY` acknowledgement.
- **SPK-03 — macOS Companion** — Swift CompanionCore; versioned authenticated WebSocket runtime channel; Machine Role/Snapshot reconciliation; duplicate/stale execution protection; Keychain-backed device identity.
- **SPK-04 — Plugin Process / IPC** — external Plugin process; JSON Lines stdio IPC; capability handshake; crash/hang containment; no automatic replay.
- **SPK-05 — Vault & Large File Transfer** — filesystem Vault objects; SHA-256 identity; staging/atomic promotion; HTTP Range/resume; verified cache; SHOW transfer gate.
- **SPK-06 — Hub Deployment on ARM64 / Mini-PC** — 64-bit Linux; native `amd64`/`arm64`; systemd; local-first boot; independent Data/Vault roots for SSD/NVMe. Physical hardware qualification remains mandatory.

## Delivered Product Foundation

M0–M5 remain as previously completed and regression-protected. M6 does not redefine those architecture boundaries.

### M6 delivered so far

#### S0

- persistent stable Hub identity independent of IP/hostname;
- Ed25519 asymmetric Hub identity key protected under the authoritative Data Root;
- stable public-key fingerprint;
- short-lived single-use OWNER setup code;
- Argon2id first OWNER password storage;
- replay/post-claim bootstrap denial;
- ARM64 CGo-free `stagecore-setup` product build.

#### S1

- local username/password login;
- opaque bounded browser sessions with digest-only persistence;
- HttpOnly/SameSite session cookie and CSRF defense;
- server-side OWNER / TECHNICIAN / OPERATOR / VIEWER permission model;
- authenticated SSE channel with revocation revalidation;
- non-loopback browser auth requires secure transport;
- Hub identity/fingerprint visible as non-secret operator verification metadata;
- denial tests for invalid credentials, rate limit, wrong role, CSRF, origin, expiry and revocation.

## Physical Raspberry Pi Qualification Gate — Issue #21

Physical qualification remains intentionally deferred until the M6 software MVP is merged. The first full Pi pass must therefore exercise the complete M0–M6 product rather than an incomplete pre-UI/pre-security build.

## Remaining Work Is Owned — Not Unbounded TBD

- M6 S2–S7 are owned by Issue #25 / Draft PR #28;
- Issue #21 owns physical ARM64/Pi qualification after M6;
- production macOS signing/notarization/background packaging remains a later product gate unless an MVP acceptance test proves a smaller prerequisite unavoidable;
- Hardware Nodes, full DMX/lighting automation, AI/Vision, HA/cloud and distributed offline authority remain explicitly later/post-MVP work.

Every known deferred item remains assigned in `docs/adr/addendum-002/04-deferred-register-and-ownership-gates.md`.

Changes to an established decision require an explicit superseding ADR/decision with evidence; implementation must not silently drift the baseline.
