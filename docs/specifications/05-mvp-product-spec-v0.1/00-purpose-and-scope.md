# 00 — Purpose & Scope

## 1. Purpose

StageCore MVP is a local-first show-control application for one operator controlling one project through a Hub and, where needed, one replaceable Mac/PC Companion.

The MVP is not a demo mockup. It must be able to create a project, configure logical targets, publish a runtime, execute cues, send a real OSC action, report success/failure honestly, and preserve a rehearsal session.

## 2. Reference Deployment

The MVP must work in this practical topology:

```text
Operator Browser/Desktop UI
        |
        v
StageCore Hub
        |
        +---- Generic OSC device on LAN
        |
        `---- Mac/PC Companion -> local app via OSC/MIDI/local integration
```

Development may run Hub + UI + simulated target on one computer. The architecture must not require that deployment in production.

## 3. Required MVP Domains

- Project create/load/save.
- Draft project editing.
- Logical device aliases and capabilities.
- Cue list and Action editor.
- Manual GO/STOP and explicit cue jump.
- Simple Input -> Route -> Action/Cue path.
- Generic OSC output as the first real integration.
- Basic HTTP and MIDI after OSC is proven.
- Runtime validation and immutable published snapshot identity.
- Rehearsal Session with CueExecution/ActionExecution trace.
- Notes attached to cue or session.
- Companion registration/readiness sufficient for one Machine Role.
- Clear errors, degraded state, and no false success.

## 4. Deliberate Simplifications

To keep v0.1 implementable:

- one active Project per Hub runtime;
- one active Published Runtime Snapshot at a time;
- one primary operator session;
- one active Companion assignment per Machine Role;
- no distributed Hardware Nodes;
- no automatic device discovery requirement: manual endpoint configuration is acceptable;
- Venue Profile may exist in the data model but the MVP UI can use one default/simple venue mapping;
- no media-server replacement and no large-media ingest pipeline;
- no certified Safety-Critical control.

## 5. Definition of a Real Feature

A feature is considered implemented only when it persists/reloads correctly, produces expected commands/events when relevant, survives normal user mistakes, exposes a failure result, and has at least one automated or repeatable acceptance test.
