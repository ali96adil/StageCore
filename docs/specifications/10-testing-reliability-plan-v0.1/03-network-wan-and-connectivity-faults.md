# 03 — Network, WAN & Connectivity Faults

## Core Network Principle

StageCore local runtime must not depend on Internet/WAN. The stage LAN is the operational network; WAN is optional external access for updates/services and should normally enter through the stage router/firewall rather than through ad-hoc per-Mac Internet paths.

## Required WAN Tests

### WAN Loss, LAN Healthy

1. Start Rehearsal with Hub, Companion and client connected on LAN.
2. Disconnect the router's Internet/WAN uplink.
3. Execute Cues and Routes.
4. Add Note and inspect Session log.

Expected:

- local Hub discovery/known connection remains usable;
- authentication/runtime remains usable;
- no required Companion becomes OFFLINE solely because WAN disappeared;
- Cue/Route results remain normal;
- only explicitly Internet-dependent optional integrations become degraded.

### WAN Restore

Restoring Internet must not restart the Hub, duplicate runtime commands or change Hub/Companion identity.

### Dual-WAN / Failover Deployment

Where a router supports two Internet sources, switch/fail from WAN1 to WAN2 while the stage LAN remains powered.

Expected:

- StageCore local sessions remain connected where the router preserves LAN;
- no Cue is replayed;
- WAN-dependent optional services may reconnect separately;
- failover is not considered a StageCore runtime dependency.

## Required LAN Fault Tests

### Companion Link Loss

Disconnect only the Companion network path. Its role becomes OFFLINE/DEGRADED within the configured heartbeat policy. Other local runtime components remain available.

### Client Link Loss

Disconnect an operator client while Hub/Companion remain connected. Runtime authority remains at Hub; reconnecting client reloads authoritative current state without replaying commands.

### Router/AP Failure

Power off or isolate the stage router/AP in the fault lab.

Expected:

- affected endpoints become disconnected visibly;
- StageCore does not claim successful remote execution it cannot verify;
- no automatic non-idempotent replay after network return;
- reconnect performs identity/Snapshot/config reconciliation before READY.

## Multiple Mac Interfaces

A Mac may technically have stage LAN on one interface and separate Internet on another, but this is not the preferred reference topology. If supported in a field setup, test that:

- Hub traffic uses the intended stage path;
- mDNS/discovery does not bind authority to the wrong interface;
- default-route changes do not change Companion identity;
- reconnect does not create duplicate logical Companion sessions.

## Measurements

Record disconnect-detection time, reconnect/reconciliation time and any dropped/unknown in-flight execution. No hidden retry is permitted for non-idempotent Actions.