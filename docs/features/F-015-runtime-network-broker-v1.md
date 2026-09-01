# F-015 Runtime Network Broker v1

## Purpose

Installable Plugin processes remain inside the Bubblewrap runtime boundary with `--unshare-all`. They never receive the Hub host network namespace.

`network.udp.send` is mediated by a StageCore-owned broker. The Hub creates one ephemeral broker session for each bounded runtime probe and one for each supervised runtime generation.

## Sandbox contract

When an installed Plugin has an approved `network.udp.send` permission, StageCore mounts only that runtime generation's broker directory at:

`/stagecore/network`

The Plugin receives:

- `STAGECORE_NETWORK_BROKER=/stagecore/network/n.sock`
- `STAGECORE_NETWORK_BROKER_TOKEN=<ephemeral 256-bit token>`

The broker directory is mounted read-only. The Plugin still has no direct host/LAN network namespace.

## Protocol

One JSON object per line over the Unix-domain socket.

Request:

```json
{
  "type": "network.request",
  "schema_version": 1,
  "request_id": "opaque-request-id",
  "operation": "udp.send",
  "token": "ephemeral-session-token",
  "host": "192.168.10.50",
  "port": 9000,
  "payload_base64": "..."
}
```

Success:

```json
{
  "type": "network.result",
  "schema_version": 1,
  "request_id": "opaque-request-id",
  "status": "COMPLETED",
  "bytes_sent": 24
}
```

Failure returns `status=FAILED` and a bounded `error_code`.

## v1 restrictions

- Only `network.udp.send` is brokered.
- Targets must be explicit unicast IP addresses. DNS names are rejected.
- Unspecified, multicast, and limited-broadcast targets are rejected.
- UDP payload size is capped at 65,507 bytes.
- `network.udp.listen` remains fail-closed behind `RUNTIME_NETWORK_BROKER_REQUIRED`.
- Broker sessions and credentials are ephemeral and are removed after probe completion, Disable, crash, Hub shutdown, or the next startup cleanup pass.

## Security invariant

A Plugin permission approval authorizes a broker operation; it does not authorize direct network access. Bubblewrap continues to isolate the Plugin network namespace, and the Hub performs the actual UDP send after broker authentication, permission, target, and payload validation.
