# StageCore Engineering Decisions

Specifications define required behavior. Decision spikes choose and validate concrete implementation technology without rewriting the product baseline.

## Accepted Spikes

- [SPK-01 — Core Technology Stack](spikes/SPK-01-core-technology-stack.md) — Go Hub, SQLite/WAL, HTTP+JSON, SSE, React/TypeScript/Vite direction.
- [SPK-02 — Real OSC](spikes/SPK-02-real-osc.md) — `osc.send` over UDP, logical endpoint resolution, typed OSC arguments and truthful `TRANSPORT_ONLY` acknowledgement.
- [SPK-03 — macOS Companion](spikes/SPK-03-macos-companion.md) — native Swift CompanionCore, persistent WebSocket command/result channel, Machine Role/Snapshot reconciliation and reconnect duplicate protection.
- [SPK-04 — Plugin Process / IPC](spikes/SPK-04-plugin-process-ipc.md) — external Plugin process, versioned JSON Lines over stdio, capability handshake, deadline/crash containment and no automatic replay.
- [SPK-05 — Vault & Large File Transfer](spikes/SPK-05-vault-large-file-transfer.md) — filesystem Vault objects, SHA-256 identity, HTTP range/resume, verified local cache, SHOW transfer gate and runtime storage reserve.

## Next

- **SPK-06 — Hub Deployment on ARM64 / Mini-PC** — prove the selected Hub/Vault stack on intended deployment-class hardware.

## Rule

A spike must end in one of: `ACCEPTED`, `REJECTED`, or `MORE EVIDENCE REQUIRED`, and must record the test/prototype evidence behind that result.
