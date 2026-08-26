# 02 — Native UI Integration

## Principle

Plugins integrate into StageCore through approved UI contribution points. The user should not have to open a separate plugin application for normal operation.

StageCore owns the shell, navigation, typography, interaction patterns, validation presentation, error states and design tokens. Plugins contribute data/schema/actions that the Core renders using native StageCore components.

## Required Native Surfaces

When a plugin declares relevant contributions, StageCore updates these surfaces automatically:

| Surface | Example from OSC plugin |
|---|---|
| Cue Editor | `Send OSC` becomes available in Add Action |
| Routing | `OSC Output` appears as an Output type |
| Devices | `OSC Endpoint` can be created/configured |
| Inspector | selected OSC Action shows address/arguments fields |
| Settings | OSC plugin settings/status section appears |
| Preflight | invalid/missing OSC endpoint produces checks |
| Status / Health | plugin running/degraded/unavailable state appears |
| Logs / Trace | OSC dispatch/result details appear in execution trace |

## Schema-Driven UI First

For v0.1, plugins do not ship arbitrary frontend pages for common configuration. They declare fields and actions, and StageCore renders them.

Example Action form declared by OSC:

```yaml
fields:
  - key: address
    type: string
    required: true
    label: OSC Address
  - key: arguments
    type: osc_arguments
    required: false
    label: Arguments
```

This keeps Cue Editor, Routing and Settings visually consistent.

## Custom UI Later

A future plugin may need a specialized editor or visualization. That will use a restricted `custom_panel` extension point with lifecycle, permissions, design tokens and isolation rules. It is not required for the MVP OSC plugin.

## Install / Disable Behavior

After activation, contributions appear without restarting the entire project model where technically possible. After disable/uninstall, StageCore removes the active contribution from creation menus but preserves existing project references as `Plugin Missing/Disabled`; it must never silently delete cues, routes or configuration that depended on the plugin.
