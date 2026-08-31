# F-002 — Operator Experience Foundation v1

**Branch:** `f002/operator-experience-foundation`  
**Starting main:** `0ce9b9affee78a10b3b387fe9b7889e5d0cc0444`

## Implemented in this slice

- Goal-oriented Project Home replaces the technical dashboard as the default project landing experience.
- Home shows mode, readiness, SHOW configuration lock, guided lifecycle and one recommended next action.
- Navigation labels become goal-oriented: Home, Setup, Cues, Run, Check, History and Notes.
- Common configuration keeps raw schemas/conditions/parameters under explicit Advanced disclosure.
- OSC target setup has a Basic builder for hostname/address and port which generates the existing `{ "osc": { "host", "port" } }` target configuration.
- Cue Actions have a Basic OSC message builder with:
  - target suggestions from configured logical targets;
  - OSC address;
  - ordered arguments;
  - Text, Whole number (`int32`), Decimal number (`float32`) and On/Off (`bool`) values;
  - automatic generation of the existing `osc.send` parameter contract.
- New OSC Actions default to the visual builder.
- Existing advanced/non-OSC Actions remain Advanced and are preserved rather than silently rewritten.
- New layout CSS uses logical properties such as `margin-block` and `text-align: start` so F-001 can add RTL without redesigning these components.
- Navigation and initial Home text expose localization keys/hooks for the F-001 foundation.

## Preserved invariants

- No runtime or Cue execution semantics change.
- No API/domain/storage contract changes.
- Published Runtime Snapshots remain immutable.
- SHOW configuration lock remains server-side authority; this UX only reflects it.
- RBAC/CSRF/session authentication paths are unchanged.
- Normal operation remains fully offline/local-first.
- Expert JSON is still available and round-trips through the existing APIs.

## Deliberately incomplete

F-002 remains open as a cross-cutting product rule. Later slices still need visual editors and guided flows for conditions/transforms, richer capability-specific Actions, Preflight remediation, Session start/resume, discovery/device profiles, plugins, updates, diagnostics, timecode, capsules, simulation/recovery and future engines.

F-001 Arabic/RTL foundation is the immediate next UI dependency. It should localize this same experience rather than fork a separate Arabic UI.
