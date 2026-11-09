# Optional signed application ACK

An Action with `require_application_ack` requires a qualified receiver. The
stock Mac Companion advertises `receiver.signed_ack.v1` only when launched with
both `STAGECORE_SIGNED_ACK_BRIDGE_URL` and `STAGECORE_SIGNED_ACK_BRIDGE_TOKEN`.
The URL must be a literal `http://127.0.0.1:<port>/stagecore/execute`. Keep the
normal OSC/MIDI configuration for ordinary Actions. Signed Actions go to the
bridge exclusively. An interrupted request can have had an application effect,
so Companion never retries or falls back to the normal executor.

Run `node tools/receiver-ack-lab/receiver_gateway.js <private-config.json>`.
The regular file must have no group/other permissions. Required JSON fields are
`port`, `companionToken`, `applicationToken`, `key` (base64 of a unique random
32-byte or longer HMAC key), and `scope` containing `project_id`,
`machine_role_id`, `companion_id`, `receiver_instance_id` and `kind`
(`ABLETON` or `VDMX`). Use distinct random URL-safe tokens of at least 32
characters. Put the same scope and signing key in the private Hub profile
selected by `STAGECORE_SIGNED_ACK_CONFIG`. Companion needs the Companion token
only. The application receiver needs the separate application token only.

| Loopback path | Authentication | Contract |
|---|---|---|
| POST `/stagecore/execute` | Companion bearer | Submit the complete execution envelope and Hub challenge. Wait for two verified application reports. |
| GET `/stagecore/pending` | Application bearer | Claim the request exactly once. A 404 means there is no unclaimed request. |
| POST `/stagecore/receipt` | Application bearer | Report `APP_REPORTED_RECEIVED` when the application accepts the exact request, then `APP_REPORTED_APPLIED` only after the requested application action takes effect. |

For each report, copy all seven fields of `application_ack.scope` into the
JSON object, along with `execution_id`, `cue_id`, `challenge`,
`correlation_id` (use null when absent), and `status`. The gateway signs the
Hub-defined receipt format and returns both packets to Companion. The Hub
checks the HMAC and the pending dispatch's scope, challenge and receipt order.
Ordinary socket or MIDI send success is insufficient. The receipt represents
application-reported evidence and does not verify physical output.

A Max for Live device can use a local Node for Max client to claim a request,
apply a mapped Live API operation and report its result. A VDMX integration can
use the same contract with an instrumented receiver able to report the mapped
operation. The existing manual observer fixtures in this directory are
diagnostics. The supplied gateway and mock tests cover both receiver kinds.
Real Max and VDMX mappings and application instrumentation need qualification
before deployment.

The gateway keeps one request pending and refuses repeated execution IDs for
its lifetime. It fails closed after 8192 unique dispatches, at which point
restart it between runs. Timeout or HTTP disconnect drops the pending claim.
An application must also check the deadline before acting and avoid executing a
claimed request that no longer has authority.
