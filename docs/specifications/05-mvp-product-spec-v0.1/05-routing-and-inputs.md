# 05 — Routing & Inputs

## 1. MVP Routing Model

The MVP implements a deliberately small routing pipeline:

`Input -> Match/Condition -> Optional Transform -> RouteAction`

The first goal is not a visual node editor. The first goal is deterministic routing that can later be represented visually.

## 2. Input Types

Required for first MVP slices:

- Manual/Test Input via `input.inject_test`.
- Generic OSC input.

Allowed later in the same MVP after OSC routing is stable:

- basic MIDI input;
- basic HTTP/webhook-style local input.

Hardware sensors/StageNodes are out of scope.

## 3. Route Editor

Required fields:

- name;
- enabled;
- InputDefinition;
- optional simple condition;
- optional simple transform;
- optional delay/debounce;
- RouteAction target;
- priority;
- error policy.

MVP conditions must stay bounded and inspectable. Minimum useful operators:

- equals / not equals;
- greater/less than for numbers;
- boolean true/false;
- optional value range.

No arbitrary scripting inside Route conditions in the critical path.

## 4. Route Targets

A RouteAction may:

- dispatch an OutputDefinition/Capability action; or
- trigger an explicitly selected Cue through an internal validated Command.

Routing never bypasses runtime/safety validation.

## 5. Route Trace

For every evaluated triggered route, record:

- input event ID/value;
- route ID;
- condition result;
- transformed value if any;
- generated command/action IDs;
- final status/error.

## 6. Testing

The UI must provide a safe Test function so the operator can inject a value and inspect what the Route would do. If the output would execute a CRITICAL action, the test flow requires explicit confirmation and uses the same normal dispatch path.

## 7. Acceptance

- Injected input can trigger an OSC output through a Route.
- Non-matching condition dispatches nothing and produces an inspectable trace.
- Disabled Route dispatches nothing.
- Debounce prevents repeated accepted triggers within its configured window.
- A route failure never disappears silently from the trace.
