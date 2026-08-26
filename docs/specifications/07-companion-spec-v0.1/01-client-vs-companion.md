# 01 — Client vs Companion

## Client

A StageCore Client presents UI and sends authorized requests to the Hub.

Examples:

- Web UI in Safari/Chrome;
- native macOS app;
- iPhone/iPad app;
- future Windows/Android client.

Clients use the same application contract. Unique show logic must not live only inside one client.

## Companion

A Companion is a machine execution agent. It can expose local capabilities such as:

- local OSC/MIDI;
- local application control;
- approved scripts/integrations;
- media/cache verification;
- machine health.

## Combined macOS Product

A future `StageCore for Mac` may package both:

```text
StageCore for Mac
├── Client UI
└── Companion Agent
```

The two remain separate logical components. Closing the operator window must not implicitly invalidate the Companion role if the agent is configured to keep running.

## Client-Agnostic Rule

The Hub exposes one stable API + realtime contract. Web, desktop and mobile clients are replaceable views of the same authoritative state.

## MVP Practical Choice

Web UI is the fastest primary management/operator client for v0.1. macOS Companion is the first execution-agent target. Native macOS/iOS UI can be added without changing Cue/Project semantics.