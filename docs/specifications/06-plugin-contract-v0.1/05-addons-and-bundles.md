# 05 — Add-ons & Bundles

## Definition

A Plugin is one extension package implementing capabilities/contributions. An Add-on is a higher-level installable bundle that can group multiple plugins, configuration templates and optional UI contributions under one product feature.

Example future `Projection Toolkit` Add-on could bundle:

- PJLink plugin;
- projection device profiles;
- preflight checks;
- mapping-assistant panel;
- templates for projector actions.

The Add-on still uses normal Plugin Contracts; it does not bypass them.

## Bundle Manifest

Conceptual fields:

```yaml
addon_id: stagecore.projection-toolkit
version: 1.0.0
requires:
  - plugin_id: stagecore.pjlink
    version: ">=1.0 <2.0"
optional:
  - plugin_id: stagecore.vision
```

## User Experience

Installing an Add-on should feel like enabling a StageCore feature, not assembling unrelated software manually. After activation, its contributions appear in the relevant StageCore workspaces through the same Extension Registry.

## Dependency Rules

- dependencies are explicit and versioned;
- missing mandatory dependencies prevent activation;
- optional dependencies degrade only the related feature;
- Add-on disable does not silently delete project data;
- dependency loops are invalid;
- automatic dependency installation requires explicit product policy/permission later.

## MVP Boundary

The MVP implements the Plugin Contract and OSC reference plugin. Full Add-on packaging/installer UI may be deferred, but the contract must not prevent bundles later.
