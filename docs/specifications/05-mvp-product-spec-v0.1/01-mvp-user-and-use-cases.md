# 01 — MVP User & Use Cases

## 1. Primary User

The primary MVP user is a technical operator / stage manager who prepares the show and runs cues during rehearsal. The MVP does not need separate products for designer, programmer, operator, and administrator yet.

## 2. Primary Use Cases

### UC-01 — Create a project

The user creates a Project with name and optional description. StageCore creates the first Draft ProjectRevision and opens the Project Workspace.

**Success:** project can be closed and reopened with the same ID and data.

### UC-02 — Configure a controllable target

The user manually adds a target such as an OSC endpoint, gives it a logical alias such as `VIDEO-MAIN`, and verifies connectivity with a safe Test action.

**Success:** target is stored independently from Cue definitions and test result is visible.

### UC-03 — Create and run a cue

The user creates a Cue, adds one or more Actions, validates the draft, publishes a Runtime Snapshot, starts Rehearsal, and presses GO.

**Success:** Cue changes state, Actions execute in defined order, result is visible, and execution is recorded.

### UC-04 — Trigger an action through routing

The user defines an Input and a simple Route. A test or real input event matches the Route and dispatches the configured output/action.

**Success:** Route Trace shows input, match, action command, and final result.

### UC-05 — Recover from an unavailable target

A device/Companion becomes unavailable.

**Success:** StageCore marks it unavailable/degraded, does not show false success, identifies affected cue/action, and allows operator correction/retry when safe.

### UC-06 — Preserve rehearsal memory

The operator adds a note and runs multiple cues in a Rehearsal Session.

**Success:** reopening the session shows cue order, timestamps, results, errors, and notes.

## 3. Secondary Use Cases

These are useful in MVP but must not delay the core loop:

- duplicate/reorder a Cue;
- enable/disable a Route;
- jump to a Cue with confirmation/audit;
- inspect current and next Cue;
- filter execution log by Cue/result;
- export a simple session report as structured text/JSON later in MVP.

## 4. Explicit Non-Users for v0.1

The MVP is not yet optimized for audience-facing control, actor mobile apps, ticketing staff, touring venue technicians, remote cloud operators, or large multi-user production teams. Their future needs must not distort the first implementation.
