# 10 — Errors, Degraded States & Safety

## 1. No False Success

StageCore must never present an Action as completed when it only knows that a packet was sent. The UI/result model must expose the highest acknowledgement level actually available.

Examples:

- OSC/UDP send with no device feedback: `SENT / no verified device state`.
- HTTP 2xx response: transport/application acknowledgement as defined by adapter.
- Companion action failure: explicit failed/timed-out result.

## 2. Operator Error Model

Every runtime error shown to the operator should contain:

- short human-readable summary;
- affected Cue/Action/target;
- status/category;
- whether retry is allowed/appropriate;
- suggested operator action when known;
- correlation/trace access for diagnostics.

Do not expose secrets/tokens in errors.

## 3. Degraded States

MVP supports explicit degraded state for:

- Companion offline;
- target unavailable/unknown;
- plugin/adapter unavailable;
- Snapshot mismatch;
- storage pressure that threatens runtime logging;
- incomplete Preflight.

A degraded component does not automatically stop every Cue. Each required target/action policy decides whether the condition is warning or blocker.

## 4. Criticality

MVP supports `NORMAL` and `CRITICAL` product-level classifications.

`SAFETY_CRITICAL` may exist in schema for future compatibility but StageCore MVP must not claim to provide certified safety control. Physical emergency-stop/interlock functions remain external to StageCore.

## 5. STOP Behavior

STOP requests cancellation/stop only for Actions whose adapters define a real stop semantic. It does not guess inverse commands or restore prior physical states.

## 6. Retry

Retry is allowed only when:

- the original result is known failed/timed out;
- Action idempotency/policy permits retry; and
- operator or defined policy authorizes it.

Non-idempotent actions must not auto-retry.

## 7. Failure Tests Required

MVP must demonstrate:

- unreachable OSC/Companion target;
- Companion disconnect during idle and during an Action;
- invalid Action parameters;
- Snapshot mismatch;
- adapter failure;
- application restart with preserved completed execution history.

## 8. Acceptance

- Every tested failure above creates an inspectable non-success result.
- No reconnect automatically replays the last Cue/Action.
- Safety-Critical action cannot be casually enabled through the normal MVP editor as though certified.
