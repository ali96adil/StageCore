# StageCore — Dependency-First Feature Implementation Order

**Status:** Canonical planning guide for confirmed future features  
**Purpose:** Decide *when* a confirmed feature should be implemented without changing its stable `F-xxx` identity.

## Core rule

Feature IDs are permanent references, not implementation sequence numbers.

Do **not** renumber `F-xxx` items to match priority. A feature such as `F-027` may intentionally be implemented before `F-008` when its contracts are foundational.

The implementation sequence is dependency-first:

1. runtime semantics and safety contracts;
2. appliance/install/diagnostic foundations;
3. reproducible show state and external execution environment;
4. device/operator workflows;
5. simulation, recovery and advanced reliability;
6. first-party specialist engines;
7. AI and convenience layers.

## Verification gate — before feature expansion

Do **not** begin the future-feature implementation sequence merely because the backlog is defined.

Before Phase 1 starts, close the current StageCore milestone/verification work and create a clean implementation checkpoint. At minimum:

- current targeted runtime path is verified with its intended real acceptance path, not only code review;
- required CI/checks are green or any accepted exception is explicitly documented;
- `git diff --check` / repository cleanliness and migration/state integrity are verified as applicable;
- current architecture/contracts are documented well enough that future feature work does not rely on chat history;
- the Feature Backlog and this dependency order are reviewed together for newly discovered dependencies;
- a named/traceable checkpoint, tag, branch, or equivalent recovery point identifies the last verified pre-expansion state;
- no unrelated feature work is mixed into the verification fix itself.

If verification exposes a real architectural blocker, update this order before implementing dependent features. The rule is **verify → checkpoint → expand**, not feature work first and cleanup later.

## Enabling foundations — before feature expansion

These are implementation primitives, not separate product features. They should be established before broad feature work so later features share one model instead of inventing incompatible state.

- **Session/state/checkpoint primitive** — a common representation for SHOW, REHEARSAL, SIMULATION, resume points and restorable state.
- **Command Envelope / command identity contract** — every meaningful command should carry stable command/session/source/target identity, correlation, issue time, expiry/TTL where appropriate, safety class and result relationship so retries/reconnects cannot create ambiguous duplicate actions.
- **Live State Snapshot foundation** — one versioned snapshot/checkpoint framework for restorable logical state, with capability-specific adapters and explicit distinction between observed, desired, verified, restorable and manual/physical state.
- **Versioned persisted-state/schema contract** — feature additions must migrate safely and preserve published/runtime data.
- **Health/readiness contract** — common Blocker / Warning / Advisory / Ready vocabulary for Preflight, devices, plugins, network and external engines.
- **Canonical observability / Flight Recorder contract** — append-only/correlated input → decision → command → acknowledgement/result → state-change records that diagnostics, rehearsal analysis, replay, recovery and later AI can consume without inventing separate logs.
- **Clock & Time Health foundation** — trustworthy wall/monotonic timestamps plus measured clock offset/drift/latency health across Hub, Companions and future Nodes so cue timing, correlation and timecode analysis are meaningful.
- **Critical-path performance budget** — GO and other P0/P1 show-control paths have explicit responsiveness/latency budgets and remain isolated from heavy UI, thumbnails, analytics, AI, reporting and noncritical logging work.
- **Portable asset manifest** — a generic manifest model for media, external project files, plugins, presets, profiles and hashes.
- **Extension trust/version model** — source, integrity, compatibility, permissions and lifecycle metadata shared by plugins, profiles and future theme packs.
- **UI semantic foundation** — localization keys, RTL-safe layout rules and semantic design tokens rather than hard-coded text/colors.

These foundations should be deliberately small and should not become a hidden rewrite of the Core.

---

# Phase 1 — Runtime semantics and UI foundations

**Priority: P0 — do early.**

### 1. F-027 — Rehearsal & Show Session Modes

Implement the session model early because rehearsal, resume, checkpoints, simulation, timecode, recovery, timing intelligence and future flight-recorder behavior all need to agree on what a runtime session is.

First slice should establish session identity, mode, start position, stop/resume state and truthful state-reconstruction semantics before advanced UI is added.

### 2. F-028 — Rehearsal Timing Intelligence — capture foundation

Do **not** wait for the predictive UI before collecting data. As soon as F-027 sessions are real, persist trustworthy cue execution timing, cue-to-cue intervals, notes, pauses, repeats, skips, jumps, result timestamps and clock metadata through the canonical event/Flight Recorder path.

The first slice is data capture and data quality, not prediction. This allows later rehearsals to teach StageCore without losing historical observations while the operator-facing analytics are still deferred.

### 3. F-012 — Show Mode configuration lock

Establish what may and may not change during a live show before features such as plugin management, layouts, updates, recovery and external engines grow around it.

### 4. F-002 — No-code / low-code operator UX

Treat this as a product rule applied continuously rather than a one-time screen. Every later workflow should expose a simple operator path first and keep raw protocols/scripts as optional expert tools.

### 5. F-001 — Arabic UI and RTL support — foundation slice

Add localization architecture and RTL-safe component rules early. Full translation/polish can continue incrementally, but delaying the foundation risks rewriting navigation, forms and layout later.

### 6. F-016 — Cross-platform Appearance & Theme System — foundation slice

Define semantic design tokens early so new interfaces do not hard-code colors/status meanings. Advanced Theme Packs and synchronization can wait.

### 7. F-017 — Workspace Layouts & Operator Profiles — state-model slice

Define a stable workspace/layout persistence model before the UI grows many panels. Full multi-window/multi-display polish can come later.

**Phase 1 exit condition:** new UI/runtime work can depend on stable session modes, command/state/time/observability contracts, Show Mode rules, localization/tokens and workspace-state contracts without inventing parallel models. Real rehearsal sessions are already collecting timing data even if predictions are not yet shown.

---

# Phase 2 — Appliance, discovery and extension operations

**Priority: P0/P1 — makes StageCore deployable and maintainable.**

### 8. F-005 — Simple repeatable installation and deployment

Create the supported install/bootstrap path first. Everything else should assume a predictable StageCore installation rather than developer-machine setup.

### 9. F-009 — `stagecore doctor`

Diagnostics should exist very early because every subsequent hardware/network/plugin feature needs a repeatable way to prove whether its foundation is healthy.

### 10. F-004 — Zero-configuration discovery, pairing and reconnect

Stabilize discovery, identity, pairing and reconnect before building user-facing device automation around manual IP addresses.

### 11. F-021 — Device Profile Library & Guided Auto-Configuration

Build on stable discovery and capability identity so a detected device can become an understandable StageCore device without raw protocol configuration.

### 12. F-015 — Plugin & Add-on Library / Manager

Implement only after extension package, compatibility, trust and lifecycle contracts are stable enough that the UI will not encode temporary assumptions.

### 13. F-010 — Safe update + automatic backup/rollback

Build on the repeatable installer, health checks and versioned state. An updater without trustworthy install/health/rollback semantics is dangerous.

### 14. F-013 — Exportable diagnostics bundle

Build on Doctor, logs, health and plugin/device metadata so the bundle is useful instead of collecting unrelated files.

### 15. F-014 — Offline installer/package path

Add after the online/repeatable install and update package format is stable; reuse the same verified release artifacts rather than inventing a second installer.

### 16. F-008 — First-run setup wizard

**Intentionally late inside this phase.** The wizard should orchestrate already-working install/discovery/profile/plugin flows. Do not build it first and then rewrite it every time those systems change.

**Phase 2 exit condition:** a fresh supported machine can install, discover, pair, diagnose, update and manage extensions without developer intervention.

---

# Phase 3 — Reproducible show environment, synchronization and timing intelligence

**Priority: P1 — protects real productions and turns rehearsal history into useful show memory.**

### 17. F-025 — External Execution Project Assets & Reproducible Runtime Environment

Establish a generic execution-environment manifest before building full portability. VDMX, QLab, Ableton, TouchDesigner and other engines should all fit the same asset/version/hash/readiness model.

### 18. F-018 — Universal Timecode & Show Synchronization

Add timecode after session/runtime semantics are stable but before advanced simulation and recovery. Time-driven state becomes part of the normal show model rather than an afterthought.

### 19. Complete F-028 — Expected Next Cue + contextual timing notes

Build the operator-facing timing model on the rehearsal data already collected since Phase 1. Use trusted sessions to derive robust typical intervals, ranges/variation and confidence, and show the next expected cue plus relevant cue/scene notes at useful lead times.

This layer must remain advisory: it may say “Next Cue likely in ~2m 18s, high confidence” and surface the note associated with that cue, but it does not press GO. When the live run diverges materially from the learned pattern, reduce/withdraw confidence rather than presenting false precision.

### 20. F-019 — Portable Show Capsule / Complete Environment Restore

Build after install/update, plugin manifests, device profiles, workspace/theme persistence and external execution assets exist. The Capsule should package stable models, not become a custom copy mechanism that must be redesigned later.

### 21. F-011 — Show/profile templates

Templates should encode mature configuration models. Creating them too early would preserve obsolete setup patterns and schema assumptions.

**Phase 3 exit condition:** StageCore can describe what a show needs, validate that environment, move/recreate it on another supported system, and use accumulated rehearsal history to provide trustworthy timing/context assistance without taking operator authority.

---

# Phase 4 — Device and operator experiences

**Priority: P1/P2 — expand practical show coverage after foundations stabilize.**

### 22. F-003 — Android Tablet Player integration

Build after discovery, profiles, plugin/extension lifecycle and session modes are stable. The Android player should become a native StageCore capability, not a separate one-off integration.

### 23. F-006 — Optional Stage Display / Actor Callboard

Build on the tablet/device/display capability model and session modes so rehearsal/show messaging and target groups use shared StageCore concepts.

### 24. F-007 — Live Video / Camera Inputs

Introduce the generic live-source contract before the first-party Visual Engine. Keep capture/rendering on suitable Companion/render machines.

### 25. F-022 — Stage Network Cockpit

Build on Doctor, discovery, telemetry and device health. The cockpit should visualize mature signals rather than create a second network-monitoring truth source.

During this phase, complete the operator-facing polish of **F-001**, **F-016** and **F-017** as the real workspaces stabilize.

**Phase 4 exit condition:** common operator/device workflows feel native and consistent across StageCore clients without requiring manual protocol/network management.

---

# Phase 5 — Simulation, recovery and production resilience

**Priority: P1 for production readiness; implement only on stable state models.**

### 26. F-024 — Full Show Simulation / Digital Twin Mode

Implement before aggressive automatic recovery. Simulation provides a safe environment to test cue logic, device profiles, timecode behavior, partial failures, timing/reconstruction paths and checkpoint semantics.

### 27. F-020 — Self-Healing Runtime & Optional High Availability

Automatic recovery must be built on proven session semantics, command identity/expiry, health/observability, reconnect behavior and simulation/fault testing. Start with safe single-node recovery policies; optional redundant Hub/leader fencing comes later.

**Phase 5 exit condition:** failures can be rehearsed, diagnosed and recovered under explicit rules without ambiguous command replay or unsafe assumptions.

---

# Phase 6 — First-party specialist engine

**Priority: P2 — high value, high scope; deliberately late.**

### 28. F-026 — StageCore Visual Engine

Do not let the Visual Engine reshape the Core. It should plug into already-stable Cue, Media, Device, Session, Preflight, Simulation, Show Capsule and Companion contracts.

Start with deterministic theatre playback and basic projection needs. Advanced compositing/VJ/generative capabilities remain later extensions.

**Phase 6 exit condition:** ordinary theatre visual workflows can optionally run natively while external engines remain supported through the same StageCore control/state model.

---

# Phase 7 — Intelligence and assistant layer

**Priority: P3 — last, because it benefits from every earlier contract.**

### 29. F-023 — StageCore Assistant / Natural-Language Show Builder

AI should consume stable project schemas, canonical Flight Recorder events, F-028 timing intelligence, device profiles, diagnostics, rehearsal sessions, simulation and readiness results. Building it earlier would force the AI layer to compensate for unstable product semantics.

AI remains advisory/drafting/diagnostic and never becomes autonomous authority for GO, emergency or safety-critical actions.

---

# Dependency highlights

- `F-028` capture begins with `F-027` and depends on canonical observability/Flight Recorder + Clock & Time Health; its predictive UI should follow stable session/time semantics and may integrate with `F-018`.
- `F-008` depends on `F-005 + F-004 + F-021 + F-015` being real workflows first.
- `F-019` depends strongly on `F-025`, plus stable plugin/profile/theme/workspace manifests.
- `F-003` should follow `F-004 + F-021 + F-015`.
- `F-006` should follow the shared device/display model and preferably `F-003` where Android is the first display client.
- `F-026` should follow `F-007` and the stable media/session/capsule/Companion contracts.
- `F-024` depends on `F-027 + F-021 + F-018` and benefits from external-engine manifests from `F-025` plus timing/checkpoint data from `F-028`.
- `F-020` depends on `F-009 + F-004 + F-027 + command identity/expiry + observability`, and should be fault-tested through `F-024`.
- `F-023` should be one of the last major layers and can consume `F-028` rather than inventing a second prediction system.

# Features that are cross-cutting rather than one-time milestones

- **F-002 No-code UX** — enforce on every feature.
- **F-001 Arabic/RTL** — architecture early, translation completeness continuously verified.
- **F-016 Themes** — tokens early, advanced packs/sync later.
- **F-017 Workspace Layouts** — persistence model early, platform-specific polish later.
- **F-028 Timing Intelligence** — data capture early; prediction/notes UI after enough trustworthy rehearsal data and stable timing contracts exist.

# Explicit anti-rework rules

1. Do not start the feature-expansion sequence before the current verification gate is closed and checkpointed.
2. Do not build the full First-run Wizard before its underlying workflows exist.
3. Do not build the Visual Engine before generic live/media/output/session contracts are stable.
4. Do not build AI features as substitutes for missing deterministic product behavior.
5. Do not let external-engine adapters create app-specific state models inside the Core.
6. Do not add automatic recovery until reconnect, command identity and command-expiry semantics are proven.
7. Do not create separate state/checkpoint models for rehearsal, simulation, recovery and external engines; use one versioned state framework with capability-specific adapters.
8. Do not create separate analytics timing logs; collect F-028 timing through the canonical event/result/Flight Recorder path from the first real sessions.
9. Do not hard-code English strings, LTR layout, colors or window assumptions in new UI work.
10. Do not create templates until the configuration they template is stable enough to migrate safely.
11. Do not allow analytics, AI, reports, dashboards, thumbnails or verbose logging to violate the critical-path performance budget.

# Review rule

Revisit this order at the end of each major milestone and after any verification result that changes an architectural dependency. A feature may move earlier or later when implementation evidence changes its dependencies, but its stable `F-xxx` ID must not change.
