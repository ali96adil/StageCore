# 06 — Isolation, Permissions & Safety

## Default Isolation

Plugins that perform network I/O, vendor SDK work, parsing or other failure-prone operations run outside the Critical Show-Control Process by default.

The Core communicates with them through bounded, versioned IPC/contracts. Plugin crash, hang or memory growth must not directly crash Cue Engine or Routing runtime.

## Permissions

Manifest permissions are explicit and reviewed before activation. Examples:

- `network.udp.send`
- `network.udp.listen`
- `network.tcp.connect`
- `filesystem.project.read`
- `filesystem.plugin-data.write`
- `local.midi.send`
- `local.application.control`

Send and listen authority are separate grants. A Plugin allowed to send UDP does not automatically receive permission to bind a UDP listener.

A plugin receives only approved permissions. Project credentials/secrets are accessed through scoped Core/Vault APIs when needed rather than copied into arbitrary plugin configuration.

## Safety Rules

- Plugins cannot bypass Core mode, permission, safety or Runtime Snapshot checks.
- Capability declarations include supported criticality/acknowledgement semantics.
- Untrusted scripts/plugins do not execute P0/P1 work inside the critical process.
- A plugin failure produces an explicit degraded/unavailable state.
- Retry is allowed only according to capability idempotency/error policy.
- Plugins cannot claim stronger acknowledgement than they can verify.
- External input Plugins normalize transport data but do not become authoritative for Project/Runtime state or Route decisions.
- Before SEC0–SEC2, OSC receive is loopback-only; a Plugin permission grant must not silently expose unauthenticated Stage LAN control.

## Resource Controls

External plugin hosts should support:

- execution deadlines;
- bounded request queues;
- health heartbeat;
- restart supervision;
- memory/CPU telemetry where available;
- log rate limits;
- graceful shutdown.

Exact sandbox/container technology remains a later implementation choice.
