# F-006 — Optional Stage Display / Actor Callboard

## Status

Phase 4 implementation specification.

## Goal

Provide an optional StageCore-controlled display mode for actors and crew using the same Stage Device identity, secure runtime channel, grouping and health model introduced for F-003. F-006 must not create a second tablet registry or a second command protocol.

## Display roles

A device may advertise `STAGE_DISPLAY` in addition to another role such as `TABLET_PLAYER`. The capability is optional and may be omitted without affecting StageCore Core operation.

Initial capability keys:

- `display.message.show`
- `display.countdown.show`
- `display.alert.show`
- `display.clear`
- `display.blackout`
- `display.chime.play` (optional)

## Presentation states

Version 1 defines these logical modes:

- `IDLE`
- `MESSAGE`
- `COUNTDOWN`
- `ALERT`
- `BLACKOUT`

The Hub stores logical state and presentation intent. A client renders that state using its native UI and the semantic appearance contract. Core does not encode Android view classes, fonts or pixel layouts.

## Messages

Messages may include:

- localized text;
- semantic category such as `INFO`, `STANDBY`, `PLACES`, `SHOW_START`, `WARNING`;
- optional title;
- optional expiry;
- optional source cue/session/correlation identity.

Examples such as audience entry, standby, places and show-start notices are presets over this generic model.

## Countdown

A countdown is defined by an absolute target instant plus display options, not by repeated per-second Hub commands. The client derives the visible countdown from its local monotonic/wall-clock mapping and StageCore time-health information where available.

Countdown targets may represent:

- show start;
- a selected cue/timeline target;
- an operator-defined instant.

A stale or expired countdown must not restart after reconnect.

## Alerts

An alert contains semantic presentation roles rather than arbitrary hard-coded colors where possible:

- severity/category;
- optional intensity;
- pulse/flash mode;
- optional bounded duration;
- optional chime identifier.

A client may map the semantic role to its platform/theme while preserving accessibility and status meaning. `display.chime.play` is used only when the client advertises it.

## Targeting

Commands may target one device or an operator-defined group/location. A group send expands to individual command identities/results under one correlation ID.

## Cue/timeline integration

F-006 actions may be manually invoked or attached to cue/timeline actions through the existing command/routing architecture. They never gain authority to trigger GO. A display action is an output consequence of operator/runtime authority, not a source of autonomous show progression.

## Reconnect

- durable current `MESSAGE`/`COUNTDOWN` state may be re-synchronized if still valid;
- expired alerts/chimes are not replayed;
- unknown completion is reported truthfully;
- device-local state is observed separately from desired state.

## SHOW policy

Sending approved display messages/countdowns/alerts is runtime control and may remain available in SHOW to authorized operators. Structural editing of groups, presets or protected configuration follows F-012 and fails closed when locked.

## Operator UX

The guided Callboard workspace includes:

- device/group target picker;
- message presets plus editable text;
- countdown target/time controls;
- alert style/severity and optional chime controls limited by capabilities;
- preview/target summary before broad sends;
- per-device result state;
- Arabic/RTL and English presentation.

## Acceptance

Deterministic software coverage must prove:

- shared identity with F-003;
- message/countdown/alert validation;
- absolute countdown semantics;
- expiry and reconnect behavior;
- optional chime capability handling;
- group expansion/per-device results;
- SHOW authorization boundaries;
- no autonomous GO authority.

Real display rendering, countdown timing and optional chime are qualified during the one cumulative Phase 4 physical gate in Issue #138.