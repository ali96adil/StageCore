# StageCore Feature Backlog

This document is the durable collection point for future StageCore product ideas that are intentionally **not allowed to disrupt the current milestone/verification work**.

It is a product backlog, not an implementation promise. A feature is promoted into milestone scope only through an explicit planning/architecture decision.

## Tracking rules

- `[ ]` = planned / not yet complete.
- `[x]` = implemented, verified, and documented.
- Do not check an item merely because code exists; its acceptance path must also pass.
- New user-approved ideas get a stable `F-xxx` ID and are appended here.
- Proactively surface useful product improvements during StageCore work; new suggestions stay in **Proposals** until explicitly approved and promoted.
- Stable `F-xxx` IDs are references, **not implementation-order numbers**. Priority and dependency order are tracked separately.

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

## Recommended implementation sequence

Use [Dependency-First Feature Implementation Order](FEATURE_IMPLEMENTATION_ORDER.md) as the canonical planning guide for *when* confirmed features should be implemented. The order may change as implementation evidence changes dependencies, but stable `F-xxx` IDs must not be renumbered.

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

- [ ] **F-017 — Workspace Layouts & Operator Profiles**
  - Allow operators to arrange, resize, show/hide, dock, or prioritize supported StageCore panels/workspaces and save that arrangement as a reusable layout profile.
  - Provide role-oriented presets such as Stage Manager, Video, Lighting, Sound, Rehearsal, and Monitoring while keeping every preset editable.
  - Support user-created profiles with clear names and fast switching between layouts without changing the underlying project or cue logic.
  - Preserve relevant workspace state such as panel visibility, panel sizing, selected workspace, inspector placement, and supported multi-window/multi-display placement where the client platform allows it.
  - Allow layouts to be scoped appropriately: local to one device, associated with a user/operator, or optionally shared/exported for another StageCore workstation.
  - Restore layouts safely when screen count, resolution, or platform differs; inaccessible/off-screen windows must be recovered automatically to a usable default position.
  - Integrate with Show Mode so an operator can use approved layout switching during a show while structural editing of protected layouts may be locked.
  - Keep workspace layout state separate from runtime show state: changing layout must never fire cues, modify routing, alter permissions, or change published runtime behavior.
  - Where practical, use the same semantic workspace/profile model across Web and macOS clients while allowing each platform to follow its native windowing and interaction conventions.

- [ ] **F-018 — Universal Timecode & Show Synchronization**
  - Add a first-class synchronization layer for time-driven shows, including practical support for SMPTE/LTC, MIDI Time Code (MTC), and internal StageCore timecode where appropriate.
  - Allow cues, timelines, media actions, external systems, and rehearsal markers to follow or generate timecode with configurable offsets and frame-rate/drop-frame rules.
  - Detect unstable, missing, discontinuous, or unexpectedly jumping timecode and expose clear operator warnings instead of silently firing unsafe actions.
  - Keep timecode-triggered execution compatible with Show Mode, logging, simulation, preflight, and critical-action safety policy.

- [ ] **F-019 — Portable Show Capsule / Complete Environment Restore**
  - Provide a portable, integrity-checked show package that can reproduce an operational StageCore show on another supported server or workstation.
  - Capture the project, runtime/version requirements, media manifest or selected media payloads, plugin/add-on manifest, device profiles, themes, workspace layouts, relevant presets, and migration metadata.
  - Offer validation before export and after import so the operator knows whether the destination is genuinely show-ready rather than merely copied.
  - Support both compact manifest-only transfer and a larger self-contained/offline capsule when required.

- [ ] **F-020 — Self-Healing Runtime & Optional High Availability**
  - Add automatic recovery policies for non-safety-critical failures such as crashed plugins, disconnected companions, stale device sessions, and recoverable service faults.
  - Distinguish safe automatic recovery from cases requiring operator acknowledgement; never replay expired or ambiguous live commands merely because a component reconnects.
  - Consider an optional redundant/standby Hub mode for high-value productions, with explicit leader ownership/fencing so two controllers can never issue duplicate live commands.
  - Integrate recovery events into health, logs, preflight, diagnostics, and post-show reporting.

- [ ] **F-021 — Device Profile Library & Guided Auto-Configuration**
  - Maintain a versioned library of known device/software profiles that map real products and applications to StageCore capabilities without requiring operators to understand raw protocol details.
  - Profiles may define discovery hints, connection fields, supported actions/events, default routing, health checks, tested protocol versions, and useful presets.
  - Allow official profiles to ship locally and update through the Extension Library, with clear trust/source labels for community or locally authored profiles.
  - Preserve an expert/manual path for unusual devices while making the common path close to “device found → choose profile → test → ready”.

- [ ] **F-022 — Stage Network Cockpit**
  - Provide a theatre-focused network view that shows StageCore devices, companions, nodes, relevant endpoints, connectivity state, latency/jitter trends, address conflicts, and important transport health in one operator-friendly workspace.
  - Surface actionable diagnostics such as “device reachable but OSC port not responding”, “multicast path unavailable”, or “Companion latency increased” rather than generic network errors.
  - Integrate network readiness into Preflight and diagnostics without requiring the operator to use external networking tools for common StageCore troubleshooting.
  - Keep deep packet analysis or enterprise network management out of scope unless added through a specialist extension.

- [ ] **F-023 — StageCore Assistant / Natural-Language Show Builder**
  - Provide an optional assistant that can turn operator intent into editable drafts: cue groups, routing rules, device mappings, checklists, notes, templates, and troubleshooting steps.
  - Allow questions such as “why did Cue 42 fail?”, “prepare these six tablets for Scene 3”, or “create a five-minute places countdown” using project state, logs, and available capabilities as context.
  - Use AI for explanation, drafting, rehearsal analysis, and diagnostics; never give AI autonomous authority over GO, emergency actions, safety-critical commands, or irreversible configuration changes.
  - Preserve a fully functional offline/manual show path when AI or Internet access is unavailable.

- [ ] **F-024 — Full Show Simulation / Digital Twin Mode**
  - Expand mock-device support into a complete rehearsal/simulation environment where an operator can run the show without the real stage hardware attached.
  - Simulate expected device acknowledgements, failures, timing, disconnects, cue results, and selected sensor/input events so routing and automation can be tested safely before load-in.
  - Allow switching real devices to simulated equivalents deliberately and visibly, with no possibility of accidentally mixing simulated and live outputs without explicit operator intent.
  - Produce a simulation report that highlights missing mappings, timing risks, unhandled failures, and differences between the simulated environment and the actual connected stage.

- [ ] **F-025 — External Execution Project Assets & Reproducible Runtime Environment**
  - Treat execution-program project files and runtime configuration as first-class show assets, not just media files.
  - Allow a StageCore project or Show Capsule to reference, validate, version, hash, back up, and where licensing permits copy the project/session files used by external execution software such as VDMX, QLab, Ableton Live, TouchDesigner, lighting software, or other show engines.
  - Record the required application name/version, relevant plugin/add-on requirements, presets, device mappings, expected file paths, launch target, and compatibility notes needed to recreate the execution environment on another workstation.
  - Provide preflight checks that detect missing project files, changed hashes, unavailable applications/plugins, unsupported versions, broken media references, or an environment that cannot be reproduced reliably.
  - Support adapter-specific launch/open/reconnect workflows where the external application exposes a supported API, URL scheme, command-line interface, OSC/OSCQuery, MIDI, or other legitimate integration surface.
  - Never bypass or emulate around a third-party product's licensing restrictions. If an application edition intentionally cannot save or export its project state, StageCore must report that limitation truthfully rather than claiming the show is fully portable.
  - For applications that support portable packaging or project collection, integrate that workflow into the Show Capsule so a replacement execution workstation can be prepared with the fewest manual steps possible.
  - For applications with partial export/state surfaces, support an **Execution Environment Snapshot** that captures every legitimately accessible reconstruction aid instead of giving up when a full project file is unavailable.
  - The VDMX adapter should investigate and, where supported by the installed edition, capture Workspace Templates, Plugin Templates, exported Media Bin pages, Control Surface JSON/layouts, VDMX user assets/resources, relevant preferences, plugin/FX inventories, media manifests and hashes, output/display notes, and operator-approved reference screenshots.
  - Where VDMX Control Surfaces publish parameters through OSCQuery, StageCore may discover the published namespace and record exposed control values/state as a partial live snapshot; this must be treated as partial state, never as a substitute for a real VDMX project file.
  - Generate a human-readable assisted-rebuild guide from the snapshot so a replacement Mac can reconstruct the VDMX workspace with substantially less manual work, and verify the rebuilt environment against the captured manifest/snapshot before declaring it ready.
  - Because third-party demo/edition behavior can change, StageCore must capability-test each export/template path on the installed VDMX version before relying on it and clearly mark what was or was not captured.

- [ ] **F-026 — StageCore Visual Engine**
  - Provide an optional first-party visual playback/rendering engine that can execute common theatre video and projection workflows directly under StageCore control, reducing dependence on external visual software for ordinary shows.
  - Initial scope should prioritize reliable show needs rather than attempting to clone a full VJ/compositing application: video/image playback, preload, pause/seek/loop, blackout, layers, opacity, fades/transitions, fit/fill/crop, basic transforms, masks, simple effects, camera/live inputs, multi-output, and practical projector mapping/keystone/perspective correction.
  - Expose the engine through the same StageCore Cue, Routing, Device, Preflight, Simulation, Logs, Media/Vault, Show Mode, and permission models rather than creating a separate control application.
  - Keep rendering on an appropriate Companion/render workstation/GPU rather than burdening the Hub with heavy video processing.
  - Allow StageCore to choose between its own Visual Engine and an external engine such as VDMX, TouchDesigner, QLab, or another supported renderer on a per-project or per-capability basis.
  - Use open/portable media and effect standards where practical and keep project state portable, versioned, backed up, integrity-checked, and compatible with Show Capsule restore.
  - Design for deterministic live playback and graceful failure behavior first; advanced generative visuals, deep compositing, node programming, or high-end VJ features remain optional extensions rather than requirements for the first version.
  - Never make the live show dependent on cloud services; all required rendering and control paths must work local-first.

- [ ] **F-027 — Rehearsal & Show Session Modes**
  - Treat SHOW, REHEARSAL, resumed rehearsal, partial-scene/cue rehearsal, bounded ranges, and SIMULATION as explicit runtime session modes instead of assuming every run starts at the first cue.
  - Allow rehearsal to begin from the show start, a selected scene, an exact cue, a selected cue range, or a saved rehearsal checkpoint, and allow an incomplete rehearsal to be marked for later resume.
  - Starting from a scene/cue must prepare the **expected pre-cue state**, not merely move the Current Cue pointer; StageCore should restore and verify what it legitimately can and produce explicit manual checks or blockers for state it cannot prove.
  - Preserve session history including start/end, pause/resume, current/last/next cue, repeats/skips/jumps, notes, results, relevant device health, and checkpoint/state references.
  - Support rehearsal checkpoints and repeat/loop workflows while keeping dangerous/critical actions protected by normal safety policy.
  - Keep live SHOW recovery stricter than rehearsal shortcuts: never replay historical commands blindly after interruption or reconnect.
  - Integrate with Preflight, Show Mode, Timecode, Digital Twin, external execution snapshots, Self-Healing, Stage Display, and the shared future state/checkpoint model.
  - Detailed specification: [F-027 — Rehearsal & Show Session Modes](features/F-027-rehearsal-show-session-modes.md).

- [ ] **F-028 — Rehearsal Timing Intelligence & Expected Next Cue**
  - Record trustworthy cue execution timing from the first real rehearsal/show sessions so historical data exists before predictive UI is added.
  - Compare selected trusted rehearsals to learn typical cue-to-cue/section timing, variation, recent trend, and confidence while excluding interrupted runs, repeats, skips, jumps, and other outliers appropriately.
  - During REHEARSAL and optionally SHOW, display the next expected cue, estimated time/window, confidence, and whether the current run is early/normal/late compared with rehearsals.
  - Surface relevant upcoming cue/scene notes at configurable lead times, including notes promoted from rehearsal into Show Mode.
  - Predictions are advisory only and must never auto-fire GO or critical actions merely because the expected time arrives.
  - Work with partial rehearsals that start from a scene/cue/checkpoint and integrate with F-018 Timecode when an explicit time source is present.
  - Use the canonical event/result timeline plus reliable clock metadata; do not create a second incompatible analytics log.
  - Remain local-first and statistical/deterministic without requiring AI or Internet access; AI may explain trends later but is not the timing authority.
  - Detailed specification: [F-028 — Rehearsal Timing Intelligence & Expected Next Cue](features/F-028-rehearsal-timing-intelligence.md).

## Proposals — awaiting approval

New improvement ideas may be added here during StageCore work. They remain proposals until explicitly approved and assigned a confirmed `F-xxx` ID.

_Currently empty._

## Next confirmed feature ID

`F-029`