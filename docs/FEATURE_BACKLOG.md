# StageCore Feature Backlog

This document is the durable collection point for future StageCore product ideas that are intentionally **not allowed to disrupt the current milestone/verification work**.

It is a product backlog, not an implementation promise. A feature is promoted into milestone scope only through an explicit planning/architecture decision.

## Tracking rules

- `[ ]` = planned / not yet complete.
- `[x]` = implemented, verified, and documented.
- Do not check an item merely because code exists; its acceptance path must also pass.
- New user-approved ideas get a stable `F-xxx` ID and are appended here.
- Suggestions that are not yet approved stay in **Proposals** until promoted.

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

## Proposals — not yet promoted

These are useful candidates raised during design discussion. They remain proposals until explicitly approved.

- [ ] **P-001 — First-run setup wizard**
  - Friendly first-launch flow for language, StageCore name, primary devices, and common initial capabilities.

- [ ] **P-002 — `stagecore doctor` diagnostics command**
  - One command to inspect service state, connectivity, pairing, storage, versions, permissions, and major runtime dependencies.

- [ ] **P-003 — Safe update + automatic backup/rollback**
  - Pre-update validation and backup, post-update health checks, and rollback when the new version fails validation.

- [ ] **P-004 — Show/profile templates**
  - Ready-made starting points for common workflows such as theatre video tablets, OSC control, projection workflows, and rehearsal setups.

- [ ] **P-005 — Show Mode configuration lock**
  - Reduce accidental configuration changes during a live show while preserving required operator actions.

- [ ] **P-006 — Exportable diagnostics bundle**
  - Generate a support bundle with useful logs/state while excluding secrets and sensitive credentials.

- [ ] **P-007 — Offline installer/package path**
  - Support installation or recovery in venues with poor/no Internet connectivity, for example from removable media or a prepared release bundle.

## Next confirmed feature ID

`F-008`
