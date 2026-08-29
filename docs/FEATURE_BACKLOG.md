# StageCore Feature Backlog

This document is the durable collection point for future StageCore product ideas that are intentionally **not allowed to disrupt the current milestone/verification work**.

It is a product backlog, not an implementation promise. A feature is promoted into milestone scope only through an explicit planning/architecture decision.

## Tracking rules

- `[ ]` = planned / not yet complete.
- `[x]` = implemented, verified, and documented.
- Do not check an item merely because code exists; its acceptance path must also pass.
- New user-approved ideas get a stable `F-xxx` ID and are appended here.
- Proactively surface useful product improvements during StageCore work; new suggestions stay in **Proposals** until explicitly approved and promoted.

## Feature documentation rule

The backlog entry must be self-contained enough that a future reader can understand the feature intent without needing the original chat or discussion.

The backlog is intentionally not the full implementation specification. Before implementation begins, each promoted feature should receive a dedicated feature/spec document that records, as applicable:

- user goal and expected operator experience;
- supported platforms and device roles;
- in-scope and explicitly out-of-scope behavior;
- configuration/state that must persist;
- security, safety, SHOW-mode, and failure behavior;
- dependencies, compatibility, migration, and upgrade expectations;
- acceptance criteria and verification path.

This keeps the backlog readable while preserving enough detail to turn each `F-xxx` item into an executable implementation plan later.

## Confirmed future features

- [ ] **F-001 — Arabic UI and RTL support**
  - Provide a complete end-user Arabic interface with correct RTL layout and theatre-friendly terminology.
  - Keep English available as an alternate UI language.

- [ ] **F-002 — No-code / low-code operator UX**
  - Common StageCore workflows must not require users to write scripts or raw code.
  - Prefer visual actions, device pickers, trigger/action builders, mappings, presets, and forms.
  - Advanced scripting may remain available as an optional expert capability.

- [ ] **F-003 — Android Tablet Player integration**
  - Revisit and audit the existing Android theatre video-player application after the StageCore core milestones are complete.
  - Preserve the Android APK as the tablet-side client while adding a StageCore plugin/module/adapter for native integration.
  - Target capabilities include automatic discovery/registration, online status, device capabilities, video/scene control, prepare/play/pause/stop/blackout, and future Stage Display mode.
  - Keep OSC as an optional compatibility/fallback layer instead of requiring the operator to manage raw OSC.

- [ ] **F-004 — Zero-configuration discovery, pairing, and reconnect**
  - The macOS Companion and supported StageCore clients/devices should discover StageCore services automatically on the local network.
  - Avoid routine manual IP entry or IP edits.
  - Support secure first pairing, remembered identity, automatic reconnect, and human-readable device names.

- [ ] **F-005 — Simple repeatable installation and deployment**
  - Target a near one-command bootstrap for a fresh Raspberry Pi or supported server.
  - Ask only for genuinely required information.
  - Install dependencies/components, configure services, start StageCore, and run post-install health verification automatically.
  - Make repeat installs safe and predictable; preserve data/configuration where appropriate.
  - Design toward safe update/reinstall/rollback behavior rather than a fragile one-shot shell script.

- [ ] **F-006 — Optional Stage Display / Actor Callboard**
  - Provide an optional tablet/dedicated-display mode controlled by StageCore for actors and crew.
  - Support countdowns before the show or specific cues.
  - Support messages such as audience entry, standby, places, and show-start notices.
  - Support configurable visual alerts such as color changes, flashing/pulsing, and optional predefined chimes/alert sounds.
  - Allow targeting by device/group/location and manual or cue/timeline-driven operation.
  - The capability must be modular: it can be enabled or omitted without affecting StageCore core operation.

- [ ] **F-007 — Live Video / Camera Inputs**
  - Add a modular live-video input layer rather than a webcam-only feature.
  - Initial source classes should include local webcams/cameras, USB/HDMI capture devices, and practical network camera sources.
  - Where supported, allow an iPhone or another device to act as a camera source through platform/native or network bridges.
  - Intended uses include projection mapping, live visuals, simulators, and future interactive/computer-vision workflows.
  - Keep heavy video processing/rendering off the Hub when practical; use the Companion/rendering machine while StageCore owns cues, state, routing, and control.
  - Leave room for adapters such as NDI, RTSP, SRT, WebRTC, or platform-specific camera bridges without changing the core cue architecture.

- [ ] **F-008 — First-run setup wizard**
  - Provide a friendly first-launch flow for language, StageCore name, primary devices, and common initial capabilities.
  - Prefer guided choices and automatic discovery over asking the operator for technical configuration.

- [ ] **F-009 — `stagecore doctor` diagnostics command**
  - Provide one command to inspect service state, connectivity, pairing, storage, versions, permissions, and major runtime dependencies.
  - Present actionable results suitable for both operators and support/debugging workflows.

- [ ] **F-010 — Safe update + automatic backup/rollback**
  - Run pre-update validation and backup before changing the installed version.
  - Run post-update health checks and automatically offer or perform rollback when the new version fails validation.
  - Protect projects, configuration, identities, and operator data across supported updates.

- [ ] **F-011 — Show/profile templates**
  - Provide ready-made starting points for common workflows such as theatre video tablets, OSC control, projection workflows, and rehearsal setups.
  - Keep templates editable and understandable rather than hiding the resulting configuration.

- [ ] **F-012 — Show Mode configuration lock**
  - Reduce accidental configuration changes during a live show while preserving required operator actions.
  - Make the active lock state obvious and provide a deliberate authorized path back to configuration mode.

- [ ] **F-013 — Exportable diagnostics bundle**
  - Generate a support bundle with useful logs, versions, health/state summaries, and relevant configuration metadata.
  - Exclude secrets, credentials, private keys, and other sensitive values by design.

- [ ] **F-014 — Offline installer/package path**
  - Support installation, update, or recovery in venues with poor or no Internet connectivity.
  - Allow a prepared release bundle or removable media path while retaining integrity checks and post-install verification.

- [ ] **F-015 — Plugin & Add-on Library / Manager**
  - Provide a dedicated StageCore page/window for discovering, installing, updating, enabling, disabling, repairing, and uninstalling Plugins and Add-ons without manual file handling.
  - Ship an official local/offline library or bundled catalog of StageCore-maintained extensions so a fresh installation can restore known add-ons without searching external locations.
  - Optionally connect to an online StageCore catalog/repository to discover newer or additional approved extensions when Internet access is available.
  - Support import from a local package/release bundle for venues without Internet access.
  - Show plugin/add-on name, description, version, compatibility, required permissions, dependencies, health/status, update availability, and whether it is official, local, or externally sourced.
  - Install explicit dependencies safely, run compatibility/integrity checks before activation, and run a post-install health check before marking an extension ready.
  - Preserve plugin configuration and project references across supported reinstall/update flows where possible.
  - Support backup/export of the installed extension manifest so a new StageCore server can reproduce the same plugin/add-on set automatically.
  - Keep installation, upgrade, removal, and incompatible migration blocked during SHOW mode unless a future explicitly safe policy is defined.

- [ ] **F-016 — Cross-platform Appearance & Theme System**
  - Provide a first-class StageCore appearance system shared conceptually across the Web UI, macOS Companion, Android tablet client, and other supported operator/display clients where practical.
  - Define semantic design tokens rather than hard-coded colors: background/surface, text, accent, status, warning, error, success, cue states, selection/focus, borders, and other UI roles.
  - Include built-in Light, Dark, and System/Automatic modes plus operator-selectable accent/theme presets.
  - Allow user-created or StageCore-provided theme presets to be saved, exported/imported, backed up, and restored on a new installation.
  - Allow optional Theme Packs to be distributed through the Extension Library, while keeping the underlying theming engine part of StageCore rather than requiring a plugin for basic appearance control.
  - A shared theme should map the same semantic intent across platforms while still allowing each native client to respect platform conventions, accessibility, contrast, typography, and control behavior.
  - Support choosing whether appearance is local to one device/user or synchronized as an account/site/show preference where appropriate; a stage-display theme may intentionally differ from the operator UI.
  - Theme changes must affect presentation only and must never change cue logic, runtime safety, permissions, or show behavior.

## Proposals — awaiting approval

New improvement ideas may be added here during StageCore work. They remain proposals until explicitly approved and assigned a confirmed `F-xxx` ID.

_Currently empty._

## Next confirmed feature ID

`F-017`
