# F-028 — Rehearsal Timing Intelligence & Expected Next Cue

**Status:** Confirmed future feature  
**Backlog ID:** F-028

## Intent

StageCore should learn the practical timing of a production from rehearsals and shows so the operator receives useful timing context without turning predictions into automatic show control.

The system must record when cues are actually executed, compare repeated rehearsals, estimate typical time to the next cue, surface confidence/variation, and show the notes that matter for the upcoming cue at the right moment.

This formalizes the existing Master Plan concepts of Show Memory, cue timing, Pattern Detection, Expected Next Cue, and contextual Cue Notes into an implementation-ready feature.

## Capture timing from the beginning

Timing data collection must begin as soon as the real Session model exists, even if predictive UI is implemented later.

For every rehearsal/show session, record at minimum:

- session identity and mode;
- cue identity and scene/section;
- cue armed/selected time where meaningful;
- operator GO/request timestamp;
- dispatch timestamp;
- acknowledgement/result timestamps where available;
- elapsed time from previous relevant cue;
- elapsed time from scene/session start;
- repeats, skips, jumps, backs, pauses, and resume events;
- manual overrides and timing-affecting interruptions;
- associated notes, delays, and timers;
- trustworthy clock/source metadata needed to interpret the timestamps.

Raw observations must remain available so later algorithms can be improved without losing rehearsal history.

## Rehearsal timing model

For each cue transition or useful show segment, StageCore may derive:

- typical/median interval;
- average interval where useful;
- observed minimum/maximum or a robust expected range;
- variation/spread;
- number of trusted observations;
- recent trend versus older rehearsals;
- confidence level;
- known outliers or interrupted runs.

The model should prefer robust statistics over false precision. A single delayed rehearsal, stopped scene, repeated technical test, or operator jump must not silently distort the prediction.

Operators should be able to choose which sessions count as trusted timing references, for example Dress Rehearsals and selected Technical Rehearsals while excluding troubleshooting runs.

## Expected Next Cue

During REHEARSAL and, when enabled, SHOW mode, StageCore can display information such as:

- **Next Expected Cue**;
- expected time remaining from the current cue/state;
- an expected window rather than only one exact second;
- confidence and number of observations;
- whether the current run is early, normal, or late compared with rehearsals;
- the scene/section and upcoming cue note.

Example:

> **Next: Elevator Down**  
> Typical: 2m 18s after Cue 17  
> Expected window: 2m 05s–2m 31s  
> Confidence: High — 6 trusted rehearsals

Predictions are advisory. They must never issue GO automatically merely because the predicted time has arrived.

## Contextual notes and advance reminders

Cue notes should be able to appear before the cue rather than only after it becomes Current.

A note may be linked to:

- a cue;
- the next cue;
- scene/section;
- actor/performer context;
- time offset before the expected cue;
- a manual checkpoint or trigger.

Examples:

- “Stand by projector change” 45 seconds before the expected cue.
- “Wait for actor to reach Mark B before GO.”
- “Confirm door closed.”
- “Video operator: prepare Scene 4 layer.”

Notes retain their lifecycle such as **Keep in Show**, **Rehearsal Only**, and **Resolved**. Rehearsal-only notes must not clutter the live show unless deliberately promoted.

## Prediction during partial rehearsals

F-027 allows rehearsal to start from a scene, cue, range, or checkpoint. F-028 must therefore avoid assuming every timing prediction begins at show start.

When rehearsing from Cue 40, predictions should anchor to the actual current/reconstructed position and use cue-to-cue or section timing where enough historical data exists.

Skipped, repeated, manually jumped, paused, or resumed paths must be represented explicitly rather than treated as normal uninterrupted timing samples.

## Timecode and clock relationship

This feature is different from F-018 Universal Timecode:

- **F-018** synchronizes systems to an explicit time source such as LTC/MTC/internal timecode.
- **F-028** learns practical timing from actual rehearsal/show execution.

They should work together when both are present. A timecode-driven cue can still accumulate rehearsal statistics, while a human-driven production can use F-028 without external SMPTE/LTC.

Reliable clock health and monotonic timestamps are foundational requirements so measurements across Hub/Companion/Nodes are meaningful.

## Flight Recorder / observability relationship

Cue timing should be derived from the same canonical event/result timeline used by diagnostics and the future Flight Recorder rather than from a second ad-hoc analytics log.

The underlying event trail should make it possible to explain why a timing sample was included/excluded and reconstruct what happened in a rehearsal.

## Learning without AI dependency

The first implementation should be deterministic/statistical and local-first. It must not require cloud AI.

Future StageCore Assistant/AI features may explain trends or summarize rehearsal behavior, but the core timing model must continue to work offline and remain inspectable.

## Safety and operator authority

- A prediction is information, not a command.
- Expected timing must never auto-fire GO, emergency, critical, or safety-critical actions by default.
- Low-confidence predictions should be clearly marked or omitted.
- If the show deviates substantially from rehearsal timing, StageCore should update the estimate or show that the prediction is no longer reliable rather than pressure the operator to follow the old estimate.
- Notes and timing reminders must never hide safety warnings or required operator confirmations.

## Useful comparison views

After multiple rehearsals, StageCore should be able to show:

- cue-to-cue timing comparison across sessions;
- scenes that consistently run early/late;
- cues with high timing variation;
- repeated problem areas;
- how the latest rehearsal differs from the selected baseline;
- notes associated with timing changes.

These become inputs to Show Reports and future StageCore Assistant analysis.

## Implementation dependency direction

**Collect early, analyze later.**

The timing/event capture schema should be introduced with the early F-027 Session foundation. The full Expected Next Cue operator experience can follow after the event/clock/notes contracts are stable.

Dependencies/foundations include:

- F-027 Rehearsal & Show Session Modes;
- canonical event/result observability / Flight Recorder foundation;
- reliable Clock & Time Health foundation;
- Cue Notes / Show Memory model;
- optional F-018 Timecode integration;
- Show Mode safety/permission rules.

## Acceptance direction

A future implementation is not complete until it can demonstrate at least:

1. Record trustworthy execution timing for every cue across several rehearsal sessions.
2. Exclude or clearly distinguish repeats, skips, paused runs, manual jumps, and deliberately untrusted sessions.
3. After multiple trusted rehearsals, show a useful expected-next-cue timing and variation/confidence rather than a single unexplained number.
4. Display the relevant upcoming cue note before the cue at a configurable lead time.
5. Recalculate or withdraw the prediction when the live/rehearsal run diverges materially from the learned pattern.
6. Work when a rehearsal begins from a scene/cue/checkpoint instead of the beginning.
7. Continue to work fully offline without AI or Internet access.
8. Never auto-trigger GO or a critical action solely from the learned prediction.
