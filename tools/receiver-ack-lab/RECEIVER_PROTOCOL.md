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

`application_receiver.js` supplies an optional Node 18+ client for the
application integration. Construct it with the local gateway port, application
token, kind and explicit `accept(command, signal)` and `apply(command, signal)`
handlers. Call the resulting `processOne()` from the app integration's scheduler.
Use a project-specific allowlist to map parameters. Each handler must return
`status`, `execution_id`, `correlation_id` and the challenge from the actual
application callback. Honor the abort signal and `receiver_deadline_ms` at the
point of action. A plain transport completion or a boolean cannot satisfy either
handler. The client never repeats a claim on timeout. No Max patch or VDMX
project automatically starts this client.

## Installing the application integrations

Both integrations are opt-in, use a separate gateway and application token per
receiver, and require a non-show copy for first commissioning. Start the gateway
and signed-ACK-enabled Companion using the private profiles above. Use separate
Companion processes/profiles when qualifying each kind; this bridge URL selects
one gateway. Existing OSC/MIDI routes are independent.

### Ableton Live and Max for Live

1. Copy `StageCore-Live-Receiver.maxpat`, `stagecore_live_track.js`,
   `max_live_receiver.js` and `application_receiver.js` into the same Max
   project folder. Open a Max Audio Effect for editing from Live, paste the
   supplied patch objects and connections into it, and retain the device's
   normal audio passthrough. Save the device and use Max's project dependency
   tools to include the scripts. Place it at top level on a dedicated audio
   track with a unique name.
2. Launch Live/Max with `STAGECORE_LIVE_RECEIVER=1`,
   `STAGECORE_ACK_GATEWAY_PORT`, `STAGECORE_ACK_APPLICATION_TOKEN` and
   `STAGECORE_LIVE_TRACK` (exact case-sensitive track name). Use a Node for
   Max runtime with Node 18+; keep the token out of the patch. In the device,
   click **script start**, then enable the polling toggle.
3. Use the signed `osc.send` action address `/stagecore/ableton/mute` with
   exactly one `int32` argument of 0 or 1. This address is an adapter mapping.
   The Max `js` object resolves the containing Track using LiveAPI, reads the
   binding, rechecks that same object on apply, and confirms the resulting
   `mute` property via LiveAPI. The receipt covers the Track property only.
4. On the test track, toggle 0→1→0 through StageCore. Disable the patch or move
   it to a different track mid-claim and verify that the Cue does not complete
   with application ACK. Re-enable it and issue a new execution to recover.

### VDMX

1. Create a new slider in a test Control Surface, range 0–1, publish it to
   OSCQuery and inspect its exact `FULL_PATH`. Add an OSC receiver for an
   exclusive path such as `/stagecore/vdmx/opacity`. In the slider's
   **Sending** tab add an OSC sender with type **Float**, same 0–1 range and
   path `/stagecore/vdmx/opacity-applied` to a dedicated UDP port on 127.0.0.1.
   Enable sending on value changes, including changes from the OSC receiver.
   Use the VDMX control exclusively for StageCore while the adapter is running.
2. Run `node tools/receiver-ack-lab/vdmx_receiver.js` with
   `STAGECORE_VDMX_RECEIVER=1`, `STAGECORE_ACK_GATEWAY_PORT`,
   `STAGECORE_ACK_APPLICATION_TOKEN`, `STAGECORE_VDMX_OSC_PORT`,
   `STAGECORE_VDMX_QUERY_PORT`, `STAGECORE_VDMX_FEEDBACK_PORT`,
   `STAGECORE_VDMX_INPUT`, `STAGECORE_VDMX_QUERY_PATH` and
   `STAGECORE_VDMX_FEEDBACK` set to the test mapping. Use the existing input
   and query ports. Do not add this slider to a production project first.
3. Use the input path with one `float32` argument in 0–1 on a signed
   `osc.send` action. The adapter requires a differing initial readback,
   the slider's own matching outgoing OSC event, and a fresh matching
   OSCQuery readback. An already-set value or lost event fails closed. This
   is confirmation of a dedicated control change, with no evidence of
   rendering or physical output. Local OSC/OSCQuery has no native execution
   ID. The receiver assumes the configured local VDMX process exclusively
   drives these paths; other local senders or manual writes to the same
   slider invalidate that assumption.
4. In VDMX verify one 0→0.5 change, then disable its sender and request 0.5→1.
   Confirm that StageCore does not mark that action application-applied.
   Re-enable feedback and send a new command with a different target.

The automated tests execute the supplied Max script against the LiveAPI object
contract and exercise the real VDMX receiver adapter over local UDP/HTTP. The
test doubles stand in for the installed applications; repeat the above checks
with the installed versions before using either profile at a show.

Product references: [Live API](https://docs.cycling74.com/userguide/m4l/live_api/),
[Track properties](https://docs.cycling74.com/apiref/lom/track/),
[Node for Max](https://docs.cycling74.com/apiref/nodeformax/),
[VDMX OSCQuery](https://docs.vidvox.net/vdmx/vdmx_oscquery),
[VDMX data I/O](https://docs.vidvox.net/vdmx/vdmx_data_io).

### Additional ACK mappings

All Ableton addresses take one `int32` argument (0 or 1).
`/stagecore/ableton/solo` confirms the same bound Track's solo property.
`/stagecore/ableton/arm` additionally requires `STAGECORE_LIVE_ALLOW_ARM=1`
and a track that Live reports can be armed. `/stagecore/ableton/transport`
requires `STAGECORE_LIVE_ALLOW_TRANSPORT=1` and confirms Song `is_playing`
after the Live API transport call. Arm and Set-wide transport are off by default.
Immediate readback must match; a delayed application or unsupported property
produces no applied receipt. These receipts cover the property itself. Clip,
scene, media and rendering commands without independently attributable
application feedback have no ACK mapping here.

VDMX can use `STAGECORE_VDMX_MAPPINGS`, a JSON array of 1–32 objects with
`inputPath`, `feedbackPath` and `queryPath`. Each mapping needs its own
exclusive control and distinct feedback route. Shared and duplicate paths
are rejected. The previous single-mapping environment remains supported
when the array is absent. All mappings retain Float32 0–1, matching feedback
and matching readback. On installation, commission every mapping separately.

### Ableton Clip playback mapping

The Max receiver can opt into `/stagecore/ableton/clip/fire` with
`STAGECORE_LIVE_ALLOW_CLIP=1` and one Int32 zero-based Session View slot
index (0–255). Use a dedicated track whose Clip launches are exclusively
owned by this receiver. The receiver refuses armed tracks, empty slots,
already-triggered or playing Clips, and changed track, slot or Clip IDs.
It fires once. `APP_REPORTED_APPLIED` requires later Live API readback of
both the matching playing slot and the exact non-recording Clip's playing
state within the command deadline. A launch queued beyond the deadline is
unconfirmed and is not fired again. Installed Live/Max testing remains
required for quantized playback and scheduler behavior.
