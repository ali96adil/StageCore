# StageCore Engineering Decisions

Specifications define required behavior. Decision spikes choose and validate concrete implementation technology without rewriting the product baseline.

## Accepted Spikes

- [SPK-01 — Core Technology Stack](spikes/SPK-01-core-technology-stack.md) — Go Hub, SQLite/WAL, HTTP+JSON, SSE, React/TypeScript/Vite direction.
- [SPK-02 — Real OSC](spikes/SPK-02-real-osc.md) — `osc.send` over UDP, logical endpoint resolution, typed OSC arguments and truthful `TRANSPORT_ONLY` acknowledgement.

## Next

- **SPK-03 — macOS Companion** — persistent trusted execution agent, Machine Role assignment, command/result channel and reconnect behavior.

## Rule

A spike must end in one of: `ACCEPTED`, `REJECTED`, or `MORE EVIDENCE REQUIRED`, and must record the test/prototype evidence behind that result.
