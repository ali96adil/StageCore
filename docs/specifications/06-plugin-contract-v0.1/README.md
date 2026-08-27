# 06 — Plugin Contract — v0.1

**Document Type:** Executable Extension Contract  
**Status:** Initial implementation baseline  
**Based on:** 02 System Architecture + 03 Data Model + 04 Event & Command Contracts + 05 MVP Product Specification

## Core Principle

A StageCore Plugin is not a separate side application. It is an extension package that declares what it contributes to StageCore. After activation, the Core registers those contributions and the StageCore UI exposes them in the relevant native workspaces.

Example: installing the OSC plugin can add an `osc.send` Action to Cue Editor, an OSC Output type to Routing, OSC endpoint configuration to Devices, OSC checks to Preflight, plugin health to Status, and OSC execution details to Logs — without the plugin duplicating the StageCore application shell.

The Core remains responsible for project state, permissions, runtime snapshots, safety gates, UI composition, execution tracking and audit. Plugins provide capabilities and extension contributions through stable contracts.

## Files

- [00 — Goals & Boundaries](00-goals-and-boundaries.md)
- [01 — Package Manifest & Contributions](01-package-manifest-and-contributions.md)
- [02 — Native UI Integration](02-native-ui-integration.md)
- [03 — Runtime Capability Contract & OSC Reference](03-runtime-capability-and-osc-reference.md)
- [04 — Lifecycle, Compatibility & Project State](04-lifecycle-compatibility-and-project-state.md)
- [05 — Add-ons & Bundles](05-addons-and-bundles.md)
- [06 — Isolation, Permissions & Safety](06-isolation-permissions-and-safety.md)
- [07 — Acceptance Criteria](07-acceptance-criteria.md)

## v0.1 Reference Plugin

The first implementation target is `stagecore.osc` with one runtime capability: `osc.send`. OSC receive is deliberately deferred until the output path is stable end-to-end.
