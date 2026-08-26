# 01 — Package Manifest & Contributions

Every plugin ships a manifest that StageCore can inspect before executing plugin code.

## Minimum Manifest

```yaml
plugin_id: stagecore.osc
name: OSC
version: 0.1.0
stagecore_api: 1
process_model: external
permissions:
  - network.udp.send
capabilities:
  - key: osc.send
    schema_version: 1
contributions:
  cue_actions:
    - capability: osc.send
      label: Send OSC
  routing_outputs:
    - capability: osc.send
      label: OSC Output
  device_types:
    - key: osc.endpoint
      label: OSC Endpoint
  settings:
    - key: osc
      label: OSC
  preflight_checks:
    - key: osc.endpoint.configuration
  status_sources:
    - key: osc.plugin.health
```

## Contribution Registry

On activation, StageCore validates the manifest and registers contributions in a central Extension Registry. Workspaces query this registry rather than hard-coding individual protocols.

Reference extension points for v0.1:

- `cue_actions`
- `routing_inputs`
- `routing_outputs`
- `device_types`
- `settings`
- `preflight_checks`
- `status_sources`
- `log_renderers`

Future extension points may be added by versioning the Plugin API; plugins cannot invent privileged extension points dynamically.

## Configuration Schemas

A contribution should provide machine-readable field schemas so StageCore can render standard controls, validate values and serialize Project/Role/Machine configuration consistently.

Examples for an OSC Endpoint:

- host: required string/IP/hostname;
- port: required integer 1–65535;
- transport: fixed `udp` in OSC v0.1;
- display_name: required string.

Plugin-specific data is namespaced by `plugin_id` and versioned.
