# F-022 — Stage Network Cockpit

## Status

Phase 4 implementation specification.

## Goal

Provide one theatre-focused view of the network/runtime signals StageCore already owns or directly measures. The Cockpit is an aggregation and diagnosis surface, not a second discovery system, packet sniffer or enterprise NMS.

## Truth sources

The Cockpit consumes canonical signals from:

- F-004 discovery/pairing/reconnect;
- secure device and Companion runtime connection state;
- F-021 profile/device identity;
- Stage Device runtime observations from F-003/F-006;
- live-source execution/readiness from F-007;
- Doctor/Preflight health checks;
- bounded measured latency/jitter observations where StageCore actually measures them.

It must not infer a precise metric when no measurement exists.

## Target classes

Initial target classes:

- `HUB`
- `COMPANION`
- `STAGE_DEVICE`
- `LIVE_SOURCE`
- `ENDPOINT`

## Observation contract

A bounded network observation may contain:

- target kind/id;
- observation time;
- reachability state;
- transport/service state;
- optional measured latency in milliseconds;
- optional measured jitter in milliseconds;
- optional address/endpoint display value;
- error/status code;
- small structured diagnostic details.

Metrics are nullable. `null` means not measured, not zero.

## Classification

The Cockpit maps known evidence into the shared health vocabulary.

Examples:

- discovered/reachable and runtime connected -> `READY`;
- host reachable but required service/runtime unavailable -> `WARNING` or `BLOCKER` according to project requirement;
- stale last-seen -> `WARNING`;
- revoked/untrusted identity -> `BLOCKER` for any required use;
- measured latency/jitter above configured advisory thresholds -> `ADVISORY`/`WARNING`;
- address/identity conflict with deterministic evidence -> `WARNING`/`BLOCKER` depending on ambiguity;
- no latency measurement -> do not fabricate latency health.

## Critical-path budget

Telemetry collection is bounded, rate-limited and outside GO execution. UI refresh, history aggregation or trend calculations must not hold cue/runtime locks or synchronously block P0/P1 commands.

## Retention

The initial Phase 4 implementation keeps a bounded recent observation window suitable for operator diagnosis. Long-term analytics and packet capture are out of scope. Support bundles may include redacted summaries, not secrets or raw media traffic.

## Operator UX

The Network Cockpit shows:

- summary counts by readiness;
- devices/Companions/sources grouped by role/type;
- connection and last-seen state;
- latency/jitter only when measured;
- actionable diagnosis text;
- drill-down to related StageCore device/source configuration;
- Arabic/RTL and English presentation;
- semantic status tokens independent of accent theme.

## Acceptance

Software acceptance proves deterministic classification, stale-state handling, null/not-measured metric behavior, bounded telemetry retention, cross-feature aggregation, authorization/localization and no critical-path coupling.

The cumulative Phase 4 physical gate deliberately disconnects/reconnects representative real devices and verifies the Cockpit changes truthfully.