# 07 — Acceptance Criteria

The Plugin Contract v0.1 is implementation-ready when all criteria below can be tested with the reference OSC plugin.

## Package & Registration

- StageCore can inspect the OSC manifest before starting plugin code.
- Incompatible Plugin API versions are rejected with a clear reason.
- Required permissions are visible before activation.
- After activation, `osc.send` is registered in the Extension Registry.

## Seamless UI

- Cue Editor shows `Send OSC` in Add Action without hard-coding OSC into Cue Editor.
- Routing shows `OSC Output` from the same plugin contribution.
- Devices can create/configure an `OSC Endpoint` using StageCore-native fields.
- Settings/Status displays plugin health.
- Preflight detects an invalid or missing endpoint used by the Published Runtime.
- Execution Logs display the OSC action result and acknowledgement level.

## Runtime

- A Cue Action targets a logical endpoint/alias, not duplicated raw host/port data.
- Core dispatches `osc.send` with an execution ID and correlation ID.
- The plugin validates the OSC address/arguments and endpoint configuration.
- The plugin sends one UDP OSC packet to the configured endpoint.
- The result returns to the correct ActionExecution.
- UDP send success is recorded as `TRANSPORT_ONLY`, not device-confirmed success.
- Failure/timeout produces explicit Action/Cue behavior according to policy.

## Lifecycle & Resilience

- Disabling the plugin removes it from new-action menus but keeps existing project references visible as unavailable.
- A project with required OSC capability cannot become Show Ready while the plugin is missing/disabled.
- Re-enabling a compatible plugin restores the preserved configuration.
- Plugin crash does not crash the critical Core process.
- Install/uninstall/incompatible upgrade is blocked during SHOW mode.

## MVP Reference Test

```text
Create OSC Endpoint
 -> Create Cue
 -> Add Send OSC Action
 -> Publish Runtime
 -> Preflight passes
 -> Start Rehearsal
 -> GO
 -> test receiver gets OSC packet
 -> Action result recorded
 -> Cue result recorded
 -> Rehearsal Log contains full correlation trace
```

Passing this test proves that the Plugin system is not merely installable: it participates natively across configuration, UI, runtime, readiness and observability.
