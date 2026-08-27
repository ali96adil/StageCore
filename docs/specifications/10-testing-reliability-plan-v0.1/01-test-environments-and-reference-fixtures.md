# 01 — Test Environments & Reference Fixtures

## Environment E0 — Deterministic Local

One developer machine runs Hub, test database, Web/API client, simulated Plugin/Companion and deterministic fake clock/network adapters where useful.

Purpose:

- fast automated tests;
- contract validation;
- migration tests;
- replay/deduplication tests;
- deterministic fault injection.

## Environment E1 — Reference MVP Bench

Minimum physical setup:

```text
Dedicated Stage LAN Router/AP
├── StageCore Hub candidate + SSD/NVMe-class data storage
├── macOS Companion
├── Operator Browser
└── Reproducible OSC Receiver / target
```

Internet is optional and must be removable without breaking the local runtime path.

## Environment E2 — Fault Lab

Adds controllable failure points:

- WAN/Internet link that can be disconnected;
- optional second WAN/failover source;
- Companion network link that can be interrupted;
- Plugin process that can be killed/hung;
- external SSD/NAS backup target that can disappear;
- storage capacity/corruption fixtures;
- test Hub where hard power removal is safe.

## Reference Project

The standard reliability fixture contains at least:

- 1 Project;
- 100 Cues;
- 200 Actions;
- 50 Routes;
- 10 logical targets/aliases;
- 1 required Machine Role `VIDEO-MAIN`;
- one real OSC target;
- simulated success/failure/timeout capabilities;
- notes and a representative published Runtime Snapshot.

## Test Data Rules

- test Projects must be reproducible from fixtures/seeds;
- expected Snapshot/content IDs are recorded where deterministic;
- destructive recovery tests never run against production/irreplaceable data;
- secrets used in tests are synthetic and isolated;
- every physical test records hardware, OS/app versions, network topology and StageCore commit/build.

## Clock / Time

Wall-clock timestamps are used for operator history, while monotonic timing is used for latency measurements. Tests must not assert fragile exact wall-clock timing when event ordering/correlation is the actual requirement.