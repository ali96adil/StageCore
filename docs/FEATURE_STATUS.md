# StageCore Feature Status Ledger

**Status date:** 2026-09-08  
**Qualified Phase 3 candidate:** `ced3af067ff72deedf3c942922774071374fad07`

This document is the operational status ledger for StageCore feature work. It complements `FEATURE_BACKLOG.md` and `FEATURE_IMPLEMENTATION_ORDER.md` by separating **implementation status** from **real qualification status**.

## Status rules

- **COMPLETE** — the promoted StageCore scope is implemented, CI-verified, physically/product-qualified where required, and documented.
- **VERIFIED FOUNDATION** — a deliberately bounded foundation/slice is implemented and verified, but the broader backlog feature remains open.
- **CROSS-CUTTING** — an early verified foundation exists, but completion is intentionally distributed across later feature work.
- **PLANNED** — implementation has not yet reached a verified promoted slice.
- A feature must not be called COMPLETE merely because code exists.
- Raspberry Pi deployment is not required after every software slice. Physical qualification is performed cumulatively at a planned batch/phase gate unless a hardware-only blocker must be investigated earlier.

## Core milestone baseline

| Scope | Implementation | Physical qualification | Operational status |
| --- | --- | --- | --- |
| M0 — Core Persistence | Complete | PASS | **COMPLETE** |
| M1 — Cue Engine + Simulator | Complete | PASS | **COMPLETE** |
| M2 — Real OSC | Complete | PASS | **COMPLETE** |
| M3 — Routing | Complete | PASS | **COMPLETE** |
| M4 — Companion + Machine Role | Complete | PASS | **COMPLETE** |
| M5 — Storage / Vault / Media Readiness | Complete | PASS | **COMPLETE** |
| M6 — MVP Operator + Security Closure | Complete | PASS | **COMPLETE** |

M0–M6 physical Raspberry Pi ARM64 qualification closed PASS through Issue #21.

## Feature status

| Feature | Implementation | Qualification | Status | Notes |
| --- | --- | --- | --- | --- |
| F-001 — Arabic UI / RTL | Foundation implemented | Foundation physically exercised | **CROSS-CUTTING** | Localization/RTL architecture is real; full cross-feature translation and polish continue. |
| F-002 — No-code / low-code UX | Foundation implemented | Foundation physically exercised | **CROSS-CUTTING** | Remains a product rule for every later operator workflow. |
| F-003 — Android Tablet Player | Not started | Not run | **PLANNED** | Phase 4. |
| F-004 — Discovery / pairing / reconnect | Implemented | PASS, including Apple TLS re-qualification | **COMPLETE** | Phase 2 PASS; later real Pi + Apple Silicon re-qualification cleared the SecureTransport blocker. |
| F-005 — Repeatable installation / deployment | Implemented | PASS | **COMPLETE** | Supported appliance install/update path physically qualified. |
| F-006 — Stage Display / Callboard | Not started | Not run | **PLANNED** | Phase 4. |
| F-007 — Live Video / Camera Inputs | Not started | Not run | **PLANNED** | Phase 4. |
| F-008 — First-run setup wizard | Implemented | PASS | **COMPLETE** | Included in cumulative Phase 2 qualification. |
| F-009 — `stagecore doctor` | Implemented | PASS | **COMPLETE** | Real Pi Doctor path qualified. |
| F-010 — Safe update + backup / rollback | Implemented | PASS | **COMPLETE** | Transactional update, SHOW gate, rollback protection and real appliance update path re-qualified during Phase 3 closure. |
| F-011 — Show / profile templates | Implemented | PASS | **COMPLETE** | Representative template materialization passed through Operator UI; resulting configuration remained ordinary editable StageCore state. |
| F-012 — Show Mode configuration lock | Implemented | PASS | **COMPLETE** | Regressed during Phase 3; structural mutation and update blocking during SHOW remained fail-closed. |
| F-013 — Diagnostics bundle | Implemented | PASS | **COMPLETE** | Included in cumulative Phase 2 qualification. |
| F-014 — Offline installer / package path | Implemented | PASS | **COMPLETE** | Offline media and WAN-independent supported path qualified. |
| F-015 — Plugin & Add-on Manager | Implemented | PASS | **COMPLETE** | Phase 2 extension lifecycle/trust/isolation/restore scope accepted as verified baseline. |
| F-016 — Appearance / Theme System | Foundation implemented | Foundation physically exercised | **CROSS-CUTTING** | Semantic tokens/System-Light-Dark/accent are real; advanced presets/portability/sync/native-client completion remain. |
| F-017 — Workspace Layouts / Operator Profiles | State-model foundation implemented | Foundation physically exercised | **CROSS-CUTTING** | Full multi-window/profile/platform polish remains. |
| F-018 — Universal Timecode & Show Synchronization | Implemented | PASS for internal + production raw-MIDI MTC path | **COMPLETE** | Frame rate, 29.97 DF, offset, stale/jump/discontinuity and fail-closed cue safety qualified. No LTC hardware/source was available, so no physical LTC PASS is claimed; LTC boundary remains deterministically covered. |
| F-019 — Portable Show Capsule | Implemented | PASS | **COMPLETE** | Real capsule export/integrity/zero-project restore passed; tamper detection failed closed. Missing/incompatible external-requirement behavior is deterministic/software-qualified because the real qualified capsule had no applicable external requirement. |
| F-020 — Self-Healing / HA | Not started | Not run | **PLANNED** | Phase 5. |
| F-021 — Device Profile Library | Implemented | PASS | **COMPLETE** | Guided Operator device-profile workflow included in Phase 2 closure. |
| F-022 — Stage Network Cockpit | Not started | Not run | **PLANNED** | Phase 4. |
| F-023 — StageCore Assistant | Not started | Not run | **PLANNED** | Phase 7. |
| F-024 — Full Show Simulation / Digital Twin | Not started | Not run | **PLANNED** | Phase 5. |
| F-025 — External Execution Environment | Implemented | PASS on real Pi + Apple Silicon Mac + VDMX | **COMPLETE** | Re-qualified during Phase 3 against the original canonical VDMX manifest identity; real OPEN and truthful PARTIAL CAPTURE_SNAPSHOT passed. |
| F-026 — StageCore Visual Engine | Not started | Not run | **PLANNED** | Phase 6. |
| F-027 — Rehearsal & Show Session Modes | Session Foundation implemented | Verified foundation | **VERIFIED FOUNDATION** | Full resume/checkpoint/range/SIMULATION/state-restore behavior remains open. |
| F-028 — Timing Intelligence | Implemented | PASS | **COMPLETE** | Trusted rehearsal statistics, Expected Next Cue, confidence/divergence behavior, contextual notes and advisory-only authority physically qualified. |

## Verified completion checkpoints

### Phase 2

Issue #60 records the cumulative Phase 2 completion checkpoint as PASS on final Phase 2 `main` `bb894a8bf52182260f2a4de0538c8d465133e842`, covering F-005, F-009, F-004, F-021, F-015, F-010, F-013, F-014 and F-008 as the verified appliance/discovery/extension baseline.

The Apple TLS issue discovered later during F-025 entry was then physically re-qualified on the real Raspberry Pi + Apple Silicon Mac path after the P-256 transport-key fix at `e31cca4d80ac24aca9ca11f6399ecc9b1bb2ae09`. Discovery, pinned TLS, pairing, challenge authentication, authenticated WebSocket runtime and remembered reconnect all passed.

### F-025 original qualification

Issue #107 records F-025 physical qualification PASS on a real Raspberry Pi + Apple Silicon Mac + VDMX path. Typed `OPEN` completed against the exact declared workspace; typed `CAPTURE_SNAPSHOT` completed with a truthful `PARTIAL` snapshot; unsupported internal VDMX capture remained explicit rather than fabricated.

### Phase 3

Issue #112 records cumulative Phase 3 completion and physical/product qualification PASS.

Final qualified/deployed candidate:

- `ced3af067ff72deedf3c942922774071374fad07`
- freeze: `freeze/phase3-qualification-ced3af0`
- Candidate Build #11 run `34217032677`: PASS
- artifact: `stagecore-phase3-candidate-ced3af0`
- artifact SHA-256: `f2e17101840f4a30d25a13e697cabdfebd986ccd2d3e2811e76d44cbe4325113`

Final production deployment on Raspberry Pi 5 ARM64 passed supported F-010 dry-run and real update. Preflight and postflight were `READY` with zero blockers/warnings, `stagecore-hub.service` was enabled/active, `/health/ready` returned `READY`, Doctor reported 11 READY checks with zero warnings/advisories/blockers, and all installed production binary SHA-256 values matched the candidate bundle exactly. A controlled restart returned healthy, and the read-only SQLite verification remained `quick_check=ok`, `journal_mode=wal`, with unchanged record counts across the restart.

Phase 3 product qualification also covered:

- F-018 internal timecode and production raw-MIDI MTC path, including 25 fps, 29.97 DF, offset and jump/discontinuity behavior;
- F-028 trusted rehearsal statistics, Expected Next Cue, confidence/divergence behavior, contextual notes and advisory-only no-auto-GO authority;
- F-019 real capsule export, integrity/tamper checks and zero-project restore with preserved identities;
- F-011 representative template materialization through the Operator UI;
- F-025 real VDMX regression using canonical manifest SHA `92d201d7a55bf6f1de05e967f818e01d89f680e22c379e0737693661619c5085`, with exact workspace OPEN and truthful `PARTIAL` capture;
- F-012 SHOW lock, persistence, security/history and representative REHEARSAL/SHOW workflow regression.

Qualification limits are explicit: no physical LTC hardware/source was available, no physical DIN/external MTC source was claimed, and the qualified real F-019 capsule contained no applicable external requirement for a physical missing/incompatible test. Those paths remain deterministically covered rather than being falsely represented as physical evidence.

Issue #137 tracks the separate Operator UX gap discovered during final F-025 regression: an accidental Draft currently cannot be discarded/reverted from the project UI. It is a follow-up UX defect and does not invalidate the immutable published qualification evidence or Phase 3 runtime PASS.

## Current transition

`M0–M6 COMPLETE -> Phase 1 foundations -> Phase 2 COMPLETE/QUALIFIED -> F-025 COMPLETE -> Phase 3 COMPLETE/QUALIFIED -> PHASE 4 READY`
