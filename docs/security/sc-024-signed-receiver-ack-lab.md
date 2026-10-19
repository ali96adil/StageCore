# SC-024 Phase 5J — Signed Ableton/VDMX receiver acknowledgment lab

**2026-10-19 · StageCore Draft integration PR #530 · source-only lab.**
**NO live receiver socket, Max for Live device installation, VDMX patch change, Pi deployment, or GO/STOP change.**

## What is different from Phase 5I

The already deployed Companion only reports whether CoreMIDI or the local
UDP socket accepted its outgoing bytes (`TRANSPORT_ONLY`). Phase 5I
built a read-only in-memory matching state machine which cannot authenticate
a receiver. This phase adds an OSC **wire format with authenticated
integrity** for eventual receiver-side app acknowledgments. A token echoed
by a Max patch or VDMX alone is **not** authentication.

`internal/outputwatch/receiver_ack_wire.go` offers:
- `EncodeSignedReceiverAck` for lab/receiver-side generation.
- `DecodeVerifiedReceiverAck` for verifying an exact per-receiver key.
- `ObserveSignedReceiverAck` which verifies the signature first, then
  delegates to the existing Cue+Execution+Snapshot+Role state machine.
- No listening HTTP or UDP endpoint, no performance-affecting Hub
  instrumentation, no output dispatch, no automatic Cue replay.

## Proposed OSC address and fields

`/stagecore/receiver/ack/v1` followed by **14 OSC string arguments**
(13 signed fields plus MAC), in this exact order:

| # | OSC string | Example / meaning |
|---|---|---|
| 1 | schema version | `1` |
| 2 | project_id | Project identity |
| 3 | runtime_snapshot_id | Published immutable Snapshot |
| 4 | machine_role_id | AUDIO-ABLETON or VIDEO-VDMX role ID |
| 5 | companion_id | Trusted local Companion identity |
| 6 | receiver_instance_id | One Max for Live or VDMX-side receiver instance |
| 7 | receiver_kind | `ABLETON` or `VDMX` |
| 8 | connection_generation | Positive decimal string |
| 9 | execution_id | Exact dispatched StageCore execution |
| 10 | cue_id | Exact cue that the receiving app observed |
| 11 | challenge | Unpredictable per-execution one-time challenge |
| 12 | receiver_sequence | Positive monotonically increasing decimal string |
| 13 | reported_status | `APP_REPORTED_RECEIVED` or `APP_REPORTED_APPLIED` |
| 14 | signature | Lowercase hex HMAC-SHA256 over all 13 preceding fields |

The signature is `HMAC-SHA256(receiverKey, "stagecore/receiver-ack/v1\\0"
+ repeat(uint32BE(utf8 byte length) + UTF8 bytes of each field))`.
All OSC strings must be canonical, no trailing bytes/alternative type tags,
and packets are capped at 4 KiB. This binds every identity, status and
sequence: changing *any* field invalidates the signature. Replay checking,
expiry and two-stage receipt progression still happen in Phase 5I. The
new protocol does not claim audible Ableton output or displayed VDMX video.

**Keys:** each receiver requires its own securely provisioned, random
32-byte-or-longer signing key. The key is NEVER included in the OSC
message or the repository. The sender must not select the validation key
solely from an unauthenticated packet field; a trusted channel must
identify the receiver instance first, then resolve its key. Protect
against compromised receivers, theft, leaks, receiver restart sequence
rollback, and cross-project re-use. Any production transport MUST be
designed and reviewed separately. An HMAC-authenticated application
report is *self-reported app evidence*, not independent verification.

## Node for Max / VDMX lab-only reference

`tools/receiver-ack-lab/receiver_ack.js` uses built-in Node
`crypto` (HMAC) and OSC string encoding and can serve as a reference
for a **future** Node for Max receiver or VDMX helper.

To run its deterministic non-network self-test from a development
checkout (NOT on the theater Pi):

```sh
node --check tools/receiver-ack-lab/receiver_ack.js
node tools/receiver-ack-lab/receiver_ack.js --self-test
```

The lab example validates against a shared Go/Node test signature:
`8fc1a6d833f708cdb3e7d622cf8e91f973b49e5d9d11563033d4f56b93ea556f`.

The optional CLI accepts one JSON receipt from stdin and a
`STAGECORE_LAB_RECEIVER_KEY_HEX` environment variable with 32+
random bytes. It prints **base64 OSC packet bytes** to stdout for offline
inspection. It does not send network traffic. Do not use the deterministic
test key for a real show, check secrets into Git or put credentials in a
Max patch file; no real key provisioning exists in this phase.

## Needed to make Max for Live or VDMX genuinely acknowledge

1. **Capture the actual application event**: Max for Live must observe
   and correlate the requested Live Scene or operation; VDMX must observe
   the requested receiving action. A mere UDP receiver `bang` means
   RECEIVED at best. Claiming APPLIED needs an application-side state
   observation, and still doesn't prove sound/video output physically.
2. **Bound delivery metadata**: the future sender must pass precise
   CueID, ExecutionID, Snapshot, Role, receiver generation and
   challenge alongside the dispatched action. Current MIDI notes and
   existing VDMX OSC routes don't carry all these values. Do not
   reinterpret unrelated MIDI traffic or reuse old cues' metadata.
3. **Provision receiver keys and auth**: independently authenticated
   loopback-side bridge or mutually authenticated private channel
   binding receiving application identity and Project/role to a
   narrowly scoped key. UDP source IP/port is insufficient.
4. **Test exactly-once report correlation, not exactly-once
   performance**: idempotence, deadline expiry, disconnect, restart
   generation fencing, revocation, stolen-key recovery and reconciliation
   with the Hub's authenticated event journal.
5. **Only then** consider an opt-in read-only Operator panel. Never
   label APP_REPORTED_APPLIED as verified sound, video or personnel
   safety and never allow it to clear an emergency Blackout.

## Qualification / deployment gate

Go tests cover the signed codec roundtrip, shared Node golden vector,
signature tampering, wrong keys, changed Cue/status, invalid OSC
types/addresses, truncated and oversized packets, expired but correctly
signed reports, replay and no command dispatch. Core CI runs the
Node syntax and deterministic self-test plus Go/race validation.

**This is not a Max for Live `.amxd` device, not a VDMX plugin, and not
production application acknowledgment.** It is a self-contained
verified protocol and reference encoder. All capabilities remain Draft
until receiving-app-specific integrations and physical qualification
exist. No `main` merge, no Pi/Companion/Stage Devices update.
