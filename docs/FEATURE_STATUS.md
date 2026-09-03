# StageCore Feature Status Ledger

**Status date:** 2026-09-03  
**Repository baseline reviewed:** `main` `50b8bb3a4fb2536a3a79fe2be4a2a91628878a9e`

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
| F-010 — Safe update + backup / rollback | Implemented | PASS | **COMPLETE** | Transactional update, rollback protection and real appliance update path qualified. |
| F-011 — Show / profile templates | Not started | Not run | **PLANNED** | Phase 3 after F-019. |
| F-012 — Show Mode configuration lock | Implemented | PASS | **COMPLETE** | Existing backlog completion remains valid. |
| F-013 — Diagnostics bundle | Implemented | PASS | **COMPLETE** | Included in cumulative Phase 2 qualification. |
| F-014 — Offline installer / package path | Implemented | PASS | **COMPLETE** | Offline media and WAN-independent supported path qualified. |
| F-015 — Plugin & Add-on Manager | Implemented | PASS | **COMPLETE** | Phase 2 extension lifecycle/trust/isolation/restore scope accepted as verified baseline. |
| F-016 — Appearance / Theme System | Foundation implemented | Foundation physically exercised | **CROSS-CUTTING** | Semantic tokens/System-Light-Dark/accent are real; advanced presets/portability/sync/native-client completion remain. |
| F-017 — Workspace Layouts / Operator Profiles | State-model foundation implemented | Foundation physically exercised | **CROSS-CUTTING** | Full multi-window/profile/platform polish remains. |
| F-018 — Universal Timecode & Show Synchronization | Not started | Not run | **PLANNED — NEXT** | First remaining Phase 3 implementation target. |
| F-019 — Portable Show Capsule | Not started | Not run | **PLANNED** | Phase 3 after F-028 completion. |
| F-020 — Self-Healing / HA | Not started | Not run | **PLANNED** | Phase 5. |
| F-021 — Device Profile Library | Implemented | PASS | **COMPLETE** | Guided Operator device-profile workflow included in Phase 2 closure. |
| F-022 — Stage Network Cockpit | Not started | Not run | **PLANNED** | Phase 4. |
| F-023 — StageCore Assistant | Not started | Not run | **PLANNED** | Phase 7. |
| F-024 — Full Show Simulation / Digital Twin | Not started | Not run | **PLANNED** | Phase 5. |
| F-025 — External Execution Environment | Implemented | PASS on real Pi + Apple Silicon Mac + VDMX | **COMPLETE** | Issue #107 closed PASS against current deployed baseline `50b8bb3a...`. |
| F-026 — StageCore Visual Engine | Not started | Not run | **PLANNED** | Phase 6. |
| F-027 — Rehearsal & Show Session Modes | Session Foundation implemented | Verified foundation | **VERIFIED FOUNDATION** | Full resume/checkpoint/range/SIMULATION/state-restore behavior remains open. |
| F-028 — Timing Intelligence | Capture foundation implemented | Verified foundation | **VERIFIED FOUNDATION** | Expected-next-cue analytics, confidence and contextual notes remain Phase 3 work. |

## Verified completion checkpoints

### Phase 2

Issue #60 records the cumulative Phase 2 completion checkpoint as PASS on final Phase 2 `main` `bb894a8bf52182260f2a4de0538c8d465133e842`, covering F-005, F-009, F-004, F-021, F-015, F-010, F-013, F-014 and F-008 as the verified appliance/discovery/extension baseline.

The Apple TLS issue discovered later during F-025 entry was then physically re-qualified on the real Raspberry Pi + Apple Silicon Mac path after the P-256 transport-key fix at `e31cca4d80ac24aca9ca11f6399ecc9b1bb2ae09`. Discovery, pinned TLS, pairing, challenge authentication, authenticated WebSocket runtime and remembered reconnect all passed.

### F-025

Issue #107 records final F-025 physical qualification PASS against current `main` / deployed Pi baseline `50b8bb3a4fb2536a3a79fe2be4a2a91628878a9e`:

- authenticated Companion and exact Runtime Snapshot readiness: PASS;
- real VDMX inspection: PASS/WARN only for truthful `REFERENCE_ONLY` portability;
- typed `OPEN`: `COMPLETED`, exact real VDMX workspace opened;
- typed `CAPTURE_SNAPSHOT`: `COMPLETED`, truthful `PARTIAL` snapshot;
- unsupported `RECONNECT`: explicit fail-closed result;
- Machine Role, trust, Runtime Snapshot and adapter authority remained enforced.

Result: **F-025 COMPLETE**. Phase 3 advances to F-018.

## Current transition

`M0–M6 COMPLETE -> Phase 1 foundations -> Phase 2 COMPLETE/QUALIFIED -> F-025 COMPLETE -> F-018 NEXT`
