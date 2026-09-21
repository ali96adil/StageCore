# Stage Device v2 — Software-Blackout Handshake (Draft)

Status: **source-only / not qualified for physical DMX or deployment**. See [Core #239](https://github.com/ali96adil/StageCore/issues/239), [firmware #7](https://github.com/ali96adil/StageCore-ESP32-DMX-Lighting/issues/7) and [assignment design #241](https://github.com/ali96adil/StageCore/pull/241).

## Preconditions

- A trusted StageCore Hub and an approved, revocable companion credential authenticate the active `stagecore.device/2` WebSocket. DNS-SD, local web input, a Project ID from an old NVS entry, and ordinary `device.observation` are **not** transfer authorization.
- The Hub has checked Operator permissions, explicit intent, both Projects' SHOW locks, pending commands, and the exact Hub-owned sidecar Project/epoch. It has stored one short-lived `PENDING` intent with the SHA-256 of **32 random challenge bytes**, not SHA-256 of the printable hex string.
- The Hub issues a per-connection monotonically increasing generation. The reservation refers to precisely the current v2 socket. Preflight/intent reservation alone do **not** modify assignment or affect DMX.
- Existing v1 clients and the currently flashed ESP do **not** speak these messages. This contract must not be enabled in their runtime or used as a justification to remove first-run Project ID.

## Hub → authenticated node: `assignment.blackout`

Illustrative values (field names and types are part of the current Hub draft):

```json
{
  "type": "assignment.blackout",
  "schema_version": 2,
  "device_id": "stable-device-uuid",
  "transfer_id": "one-use-transfer-uuid",
  "assignment_epoch": 1,
  "connection_generation": 7,
  "challenge": "64-lowercase-hex-characters-encoding-32-random-bytes",
  "expected_channels": 12,
  "blackout_required": true
}
```

The `expected_channels` field is the full configured logical output universe (currently 12 for the StageCore ESP32 DMX Lighting Node). A sparse map, an omitted channel, or simply `dmx_healthy=true` is not a verified zero report.

The future firmware must independently authenticate the Hub/TLS identity, check the node's committed epoch and current transport generation, abandon nonzero fades and queued commands, apply **immediate** blackout through its output task, and await a *software-confirmed* all-zero state before sending an ACK. Timeout, failed output application, stale epoch, unknown transfer ID, or reconnect must remain fail-closed. An OFFLINE device cannot acknowledge by replaying an earlier health report.

## Node → Hub: `assignment.blackout_ack`

```json
{
  "type": "assignment.blackout_ack",
  "schema_version": 2,
  "device_id": "stable-device-uuid",
  "transfer_id": "one-use-transfer-uuid",
  "assignment_epoch": 1,
  "connection_generation": 7,
  "challenge": "the-identical-64-character-challenge",
  "blackout": true,
  "channel_levels": [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
}
```

`channel_levels` is an actual JSON array of *integers* in physical-channel order, not a base64 string or a list of active channels only. The Hub requires exactly `expected_channels` elements and zero for each. It compares the challenge in constant time and checks the transfer ID, device, epoch, connection generation and current authenticated socket. A mismatched or unsolicited ACK is rejected and the socket may be closed.

A matching ACK returns only an **internal software-zero confirmation**. It **does not** transition the stored assignment. A separately reviewed coordinator must then recheck permissions, live generation, SHOW and pending-command safety, verify that the exact nonexpired reservation is still PENDING, atomically CAS the sidecar to a new epoch with state BLOCKED/UNASSIGNED and write canonical audit. The node must acknowledge the *new* epoch before a new Project snapshot can be activated. v1 commands remain database-fenced throughout.

## Hub → node: committed BLOCKED epoch / software-zero receipt

After the Hub atomically commits a reserved software-zero transfer, it closes the
old socket. On a new authenticated v2 socket it sends `assignment.state` with
`state=BLOCKED`, the new `project_id`, `assignment_epoch`,
`connection_generation`, `epoch_ack_required=true`,
`blackout_required=true`, and `commands_enabled=false`.

The experimental node must first cancel old effects, write and await software
confirmation of zero for **all 12 physical DMX channels**, then atomically
persist the monotonic epoch plus a hash of the Hub Project and BLOCKED state in
NVS. A lower epoch or changed Project/state at the same epoch is rejected after
reboot. The persisted hash is **not** authorization to command a Project.

Only after those checks does the node send:

```json
{
  "type": "assignment.epoch_ack",
  "schema_version": 2,
  "device_id": "stable-device-uuid",
  "project_id": "Hub-committed-Project-uuid",
  "assignment_epoch": 2,
  "connection_generation": 8,
  "blackout": true,
  "channel_levels": [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
}
```

The Hub revalidates the runtime credential, current socket, committed audit and
reservation, Project, epoch and complete zero report; it records the epoch ACK
without touching the BLOCKED assignment or activating a snapshot. It replies
with `assignment.epoch_ack_receipt`, including matching ID/Project/epoch/
generation, `state=BLOCKED`, `persisted=true`,
`commands_enabled=false`. A missing receipt forces the experimental node
back to failsafe/reconnect, not ACTIVE. Same-epoch exact reconnects may report
again; stale epoch, Project, generation and nonzero/partial reports are denied.

**Still missing:** authorized Operator write route, device-side Project
configuration/snapshot activation, actual project-scoped v2 command execution,
and independent physical DMX decoder/LED validation.

## Qualification boundary

A software ACK is the device's **report** that all logical outputs reached zero; it cannot independently establish the voltage/current/light output of a real DMX decoder or LED strip. Physical wiring, DMX refresh/decoder behavior, loss of Wi-Fi during fades, old-command rejection and actual strip darkness remain separate physical qualification gates. Do not merge or deploy these source-only drafts based on CI alone.
