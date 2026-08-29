# F-027 — Rehearsal & Show Session Modes

**Status:** Confirmed future feature  
**Backlog ID:** F-027

## Intent

StageCore must treat rehearsals and live shows as explicit session types instead of assuming every run starts at the first cue and continues without interruption.

An operator must be able to start a full rehearsal, rehearse only part of a show, begin from a scene or cue, pause or end midway, and later resume with enough recorded context to continue safely and intentionally.

## Session chooser

When entering runtime operation, StageCore should offer clear choices appropriate to the project and operator permissions, such as:

- **Start Show** — normal live-show path.
- **Start Full Rehearsal** — begin the rehearsal from the configured beginning.
- **Resume Rehearsal** — continue a previous incomplete rehearsal session from its saved checkpoint.
- **Rehearse From Scene** — choose a scene/section and prepare the system for its beginning.
- **Rehearse From Cue** — choose an exact cue and prepare the system for the state expected immediately before that cue.
- **Rehearse Range** — optionally limit a rehearsal to a scene, cue range, or selected sequence.
- **Simulation** — hand off to the Digital Twin/Simulation mode when no live outputs are desired.

The UI should show the selected mode unmistakably so REHEARSAL, SHOW, and SIMULATION cannot be confused.

## Rehearsal sessions

Each rehearsal is a first-class session with its own identity and history. Record at minimum:

- start/end time;
- selected starting scene/cue and optional ending boundary;
- pauses and resumptions;
- current/last completed cue;
- repeated, skipped, backed-up, or manually jumped cues;
- operator notes and rehearsal-only notes;
- command/results timeline and relevant device health;
- important runtime-state/checkpoint references;
- reason for ending or suspending the rehearsal when supplied.

A rehearsal can be deliberately marked **Complete**, **Stopped**, or **Resume Later**.

## Resume later

When a rehearsal stops halfway, StageCore should preserve an explicit resume checkpoint rather than relying on the operator's memory.

On the next session, **Resume Rehearsal** should show where the previous rehearsal stopped, what the last completed/current/next cue was, and whether the previously observed runtime state can be reconstructed safely.

Resuming must never silently replay old commands or assume physical equipment still has the same state after a shutdown, disconnect, or day change.

## Start from scene or cue

Jumping to a cue is not merely changing the Current Cue pointer.

StageCore should calculate or retrieve the **expected pre-cue state** needed to rehearse that point meaningfully. Depending on available capabilities this can include:

- required media/preloads;
- active visual layers, clips, opacity, mappings, presets, and external-engine state that StageCore can legitimately restore;
- routing and logical device state;
- audio/playback preparation where supported;
- tablet/player preparation and scene selection;
- timers/countdowns and stage-display state;
- timecode source/offset expectations;
- automation enable/disable state;
- relevant device/capability readiness.

The operator should see a **Prepare Rehearsal Start** summary before live outputs are changed.

## State reconstruction and manual checks

Some state can be reconstructed automatically; some cannot.

StageCore must classify preparation items, for example:

- **Ready / Restorable** — StageCore can establish and verify the required state.
- **Needs Operator Check** — physical or external state cannot be proven and must be confirmed manually.
- **Unavailable / Blocker** — required state cannot currently be established.

Physical scenery, actor positions, props, manual lighting states, safety systems, motors, pyro, or other safety-critical conditions must never be assumed from software history alone.

If a cue depends on previous irreversible or ambiguous actions, StageCore should explain why the exact historical state cannot be recreated and provide an assisted setup/checklist rather than pretending the jump is exact.

## Rehearsal checkpoints

Allow deliberate checkpoints/bookmarks such as:

- Start of Scene 2
- Before blackout transition
- After interval
- Technical reset point
- Custom operator checkpoint

A checkpoint can contain the logical show position plus a verified/restorable state snapshot and manual setup notes. These become safe, fast entry points for future rehearsals.

Where appropriate this should integrate with the future Live State Snapshot capability rather than creating an unrelated state model.

## Loop and repeat workflows

Rehearsal mode should make repetition easy without polluting live-show behavior:

- repeat current cue;
- back to previous cue;
- repeat a scene/range;
- reset to a saved rehearsal checkpoint;
- optionally loop a selected technical sequence for testing.

Dangerous/critical actions remain protected by their normal safety policy even during rehearsal.

## SHOW mode

Normal **Start Show** should default to the published show start and the strictest runtime safety behavior.

If a live show is interrupted, recovery/resume is a separate deliberate flow: StageCore should reconcile the last known show position, current device health, restorable logical state, and operator-confirmed physical state before allowing continuation from a recovery point.

A live-show resume must never simply replay every command that was previously executed.

## Integration

This feature should integrate with:

- cue/show engine and scenes/sections;
- rehearsal logging and notes;
- Show Mode configuration lock;
- Preflight / READY FOR SHOW;
- Live State Snapshots and checkpoints;
- Digital Twin / Simulation;
- external execution engines and Execution Environment Snapshots;
- Timecode;
- Self-Healing / recovery;
- Stage Display / Callboard where rehearsal messaging differs from show messaging.

## Acceptance direction

A future implementation is not complete until an operator can demonstrate at least these cases:

1. Start a full rehearsal from the beginning.
2. Stop midway, close/end the session, and later resume from the recorded checkpoint with clear state reconciliation.
3. Select Scene 2 and prepare directly for that scene without running Scene 1.
4. Select an individual cue and receive a truthful pre-cue preparation plan before execution.
5. Rehearse a bounded cue range and repeat it multiple times while preserving an understandable session log.
6. Attempt to jump into a state that cannot be safely reconstructed and receive a blocker/manual checklist instead of a false Ready state.
7. Keep SHOW mode behavior and safety separate from rehearsal shortcuts.
