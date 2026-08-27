# 02 — Hub Discovery & Connection

## User Experience

Preferred local flow:

```text
Open StageCore
 -> discover Hub on LAN
 -> select `StageCore — Main Stage`
 -> connect
```

Fallbacks:

- `stagecore.local` where local name resolution works;
- manual IP/hostname entry;
- previously trusted Hub entry.

## Identity Rule

Hub identity is not its IP address. The Companion stores a stable Hub identity/fingerprint after pairing. IP/hostname is only a route to reach that identity.

## MVP Discovery

mDNS/Bonjour-style local discovery is preferred but must not block the first implementation. Manual host entry is a valid fallback from day one.

Discovery result should expose:

- Hub display name;
- stable Hub ID;
- address/port;
- StageCore version/API version;
- pairing-required state.

## Connection Channels

For the first implementation, management/configuration may use local HTTP(S), with a persistent realtime channel such as WebSocket for runtime events/commands. The application code must keep these behind StageCore contracts so transport can evolve without changing Cue/Project models.

## Failure Behavior

- wrong Hub identity after pairing -> reject and show identity mismatch;
- unreachable Hub -> Companion becomes disconnected/offline, not silently reassigned;
- network change -> rediscover/reconnect using trusted identity;
- Internet loss -> no effect on local discovery/runtime requirement.