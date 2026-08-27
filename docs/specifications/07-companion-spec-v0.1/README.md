# 07 — Companion Specification — v0.1

**Document Type:** Executable Companion & Client Integration Specification  
**Status:** Initial implementation baseline  
**Based on:** 02 System Architecture + 03 Data Model + 04 Event & Command Contracts + 05 MVP Product Specification + 06 Plugin Contract

## Core Principle

StageCore separates **Clients** from **Companions**.

- A Client is a user interface: Web, macOS, iPhone/iPad, or future desktop/mobile app.
- A Companion is a trusted execution agent on a machine that performs local capabilities for StageCore.

Both connect to the same authoritative Hub. The Hub remains the source of truth for Project state, Runtime Snapshot, Machine Roles, readiness and execution history.

The normal user experience should allow a new Mac to join the stage network, open the StageCore Web interface, download the compatible macOS app/Companion package from the Hub, install it, pair, receive a Machine Role, synchronize required configuration/media, pass Preflight and become READY.

## Files

- [00 — Roles & Boundaries](00-roles-and-boundaries.md)
- [01 — Client vs Companion](01-client-vs-companion.md)
- [02 — Hub Discovery & Connection](02-hub-discovery-and-connection.md)
- [03 — Pairing & Trust](03-pairing-and-trust.md)
- [04 — Machine Identity & Roles](04-machine-identity-and-roles.md)
- [05 — Runtime Channel & Configuration Sync](05-runtime-channel-and-sync.md)
- [06 — Local Capabilities & Execution](06-local-capabilities-and-execution.md)
- [07 — Media Cache & Software Bootstrap](07-media-cache-and-software-bootstrap.md)
- [08 — Health, Readiness & Preflight](08-health-readiness-and-preflight.md)
- [09 — Offline, Reconnect & Replacement](09-offline-reconnect-and-replacement.md)
- [10 — macOS MVP Companion](10-macos-mvp-companion.md)
- [11 — Acceptance Criteria](11-acceptance-criteria.md)

## v0.1 Reference Topology

```text
Browser / macOS Client / iPad Client
                |
         StageCore Hub
          /          \
   Plugins        Mac Companion
                     |
                Local Apps
```

The MVP must work without Internet after installation/configuration.