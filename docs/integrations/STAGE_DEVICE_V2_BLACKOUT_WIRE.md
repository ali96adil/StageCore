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

## Experimental Operator contract (source-only; disabled by default)

The Hub registers `POST /api/v1/stage-devices/{device_id}/assignment/software-transfer`
but returns `STAGE_DEVICE_TRANSFER_DISABLED` unless
`STAGECORE_EXPERIMENTAL_V2_SOFTWARE_TRANSFER=1`. **Do not enable this flag
on show hardware.** The route uses authenticated same-origin browser session
and CSRF, and requires both `project.edit` and `companion.pair` for the
current role. Its JSON body is:

```json
{
  "expected_project_id": "",
  "target_project_id": "Hub-existing-target-Project-uuid",
  "expected_assignment_epoch": 1,
  "confirm": "BLOCK_OUTPUTS_AND_CHANGE_PROJECT_SOFTWARE_ONLY"
}
```

An empty current Project identifies an unassigned v2 node; an empty target
Project means unassign, if the authoritative Preflight permits it. The browser
cannot specify a nonce, socket generation, ACK, final state or snapshot. The
Hub generates its own reservation and sends a fresh, authenticated challenge,
then revalidates Operator session/CSRF/permissions *again* after the zero
report and before database commit. The database rechecks both SHOW locks,
existing Projects, outstanding commands and current assignment/epoch.

Only a verified matching **software** ACK may advance the Hub sidecar into
`BLOCKED`/`UNASSIGNED`; the HTTP response is `202 Accepted` with the audit,
`commands_enabled=false`, `physical_blackout_verified=false` and
`epoch_ack_required=true` for BLOCKED. A failed, stale, revoked, offline or
ambiguous attempt returns no transfer success. The caller must GET the latest
Hub-owned assignment before retrying.

Read-only
`GET /api/v1/stage-devices/{device_id}/assignment/transfer-status`
requires both `project.read` and `companion.pair`. It reports a persisted
BLOCKED software-zero ACK as **current** only if it matches the Hub's live
authenticated v2 connection generation; old records remain historical. This
endpoint always reports physical verification, snapshot activation and new
command authority as false. The bilingual Stage Devices Operator page uses
this read-only view and the pairing-protected unassigned inventory; it does
**not** render a transfer or v1 command button for a v2 node.

## Hub restart: durable connection-generation fence

The Hub records every v2 socket generation in a SQLite singleton
(`stage_device_v2_connection_sequence`, schema 35) **before** registering a
socket or sending `assignment.state`. Allocation is an atomic SQL
`UPDATE ... RETURNING`, serialized even across overlapping Hub processes.
The migration initializes its high-water mark above historical generations
in transfer intents and BLOCKED epoch ACKs. Exhaustion or a storage failure
rejects the new v2 connection; no process-local fallback is permitted.

Without a durable fence, a restart could reuse generation `1`, matching a
previously persisted software-zero epoch ACK and causing the Operator
transfer-status API to mislabel a **historical** report as **current**.
The status may claim a current software report only if the stored ACK matches
the live authenticated generation. A replacement socket or a restarted Hub
must obtain a strictly newer generation and wait for a fresh zero report.
This safeguard does not independently verify physical output.

Legacy v1 command sessions do not depend on this v2 SQL allocator. No
snapshot, Project assignment or DMX output is activated by allocating a
generation.

## Qualification boundary

A software ACK is the device's **report** that all logical outputs reached zero; it cannot independently establish the voltage/current/light output of a real DMX decoder or LED strip. Physical wiring, DMX refresh/decoder behavior, loss of Wi-Fi during fades, old-command rejection and actual strip darkness remain separate physical qualification gates. Do not merge or deploy these source-only drafts based on CI alone.
