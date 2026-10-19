# SC-024 Phase 5K — App-observed Live/VDMX receiver ACK lab

**2026-10-19 — Integration PR #530, Draft, NON-SHOW, NOT WIRED.**

This phase is a real **source-level laboratory prototype**, not an
installed Max for Live `.amxd` or a working VDMX output monitor.
The current StageCore Companion still sends its original OSC/MIDI
commands unchanged; this work never starts a UDP server or connects
an acknowledgment to the Hub. Nothing is installed on the theater Pi.

## Why this differs from a raw MIDI/OSC acknowledgment

An outgoing MIDI message accepted by CoreMIDI or OSC packet accepted
by the OS is **TRANSPORT_ONLY**. Even hearing an incoming OSC message
in a patch is *not* proof of playback.

Cycling '74's Live Object Model exposes a Track's observable
`playing_slot_index` (zero-based, negative values mean special
stop/arrangement conditions). The `live.observer` Max object
notifies listeners about current or changed properties. Its first
value after subscription can be an *existing* value, not a new Cue
execution. See the official Track/Live API and node.script docs.

This prototype uses an **app-side observed property transition** as
software evidence only. A change to a matching clip slot can be due to
another controller or person: it is correlation, **not conclusive
causation**, nor a guarantee the Scene is audible. Scenes involving
stop slots, Legato, fades, or multiple tracks need their own carefully
chosen observed Track/Slot(s).

For VDMX, the equivalent input must be an *actual state readback* of
a published output property, e.g. a controlled layer opacity, not a
receipt of the source OSC datagram. This repository does **not yet
provide a verified VDMX 6 published-output watcher**.

## New lab files

- `tools/receiver-ack-lab/app_observer.js`: one-command-at-a-time
  explicit observation gate. A caller arms an exact Cue/Execution/Role/
  Snapshot/Receiver metadata and a target (Live Track + playing clip
  slot, or VDMX output property + expected primitive value). Arming
  alone NEVER emits ACK. Only a matching **fresh, post-arm change**
  generates the two existing, per-instance HMAC-signed receipt
  messages: APP_REPORTED_RECEIVED, then APP_REPORTED_APPLIED.
- `tools/receiver-ack-lab/max_receiver_bridge.js`: optional
  `node.script` handlers `stagecore_arm_json`,
  `stagecore_live_slot`, `stagecore_vdmx_property`,
  `stagecore_cancel` and `stagecore_reset_observer`. A new Live API
  object ID invokes the reset handler to clear all property baselines
  and cancel any pending arm. First property report is only a baseline;
  repeated/unrelated values cannot sign anything. This adapter can
  produce a base64 OSC **lab fixture to Max's outlet** but has no
  outbound OSC transport. It fails closed on malformed data.
- `tools/receiver-ack-lab/StageCore-Live-Observer-LAB.maxpat`:
  JSON Max patch template with a `live.thisdevice` →
  `live.path live_set tracks 0` →
  `t l b` (reset on new Live object ID before binding the observer) →
  `live.observer playing_slot_index` → `prepend stagecore_live_slot 0` →
  manually started `node.script max_receiver_bridge.js @autostart 0`.
  The separate manual inlet uses `prepend stagecore_arm_json`
  to name the JSON metadata message. Cycling ’74 documents that
  `live.observer` sends a **bare property value** through its left outlet,
  so no `route playing_slot_index` belongs on that path.
  No `udpsend`, `midiout`, `dac~`, live.fire or GO route is present.
  This source patch has not been opened or validated inside Live on a
  macOS machine; it is **not** a packaged Max for Live `.amxd`.
- Offline tests: `app_observer.test.js`,
  `max_receiver_bridge.test.js` and `patch_structure.test.js`.
  The Core CI checks syntax and runs these with Node's built-in test
  runner; they require neither Live nor VDMX.

## Live API lab correctness repairs

The first Phase 5K CI exposed a false positive in the Max patch structure
test: a **comment** mentioned `udpsend` as a prohibition. The corrected
test checks executable Max object boxes instead. Additional tests protect
the observer path against a redundant property-name router and ensure
manual JSON arms reach `stagecore_arm_json`.

A Live API Track ID change can produce an old/current baseline value.
The `live.path` **middle outlet** (outlet index 1) follows the **path**
`live_set tracks 0` when Tracks are reordered. The left outlet instead
sticks with an original Track ID; using it could silently monitor the
wrong Track after reordering. The `t l b` reset message cancels a
pending lab arm and clears Node's Track baseline before binding the
replacement ID. In Max's right-to-left
trigger ordering, reset is sent first. Node tests qualify this but
the connection is still **untested in Max/Live itself**.

Track 0 is hard-coded in this prototype. Manual Scene launches,
multiple-Track Scenes, negative Stop Clip indices and asynchronous
observations must not be treated as proven StageCore Cue causation.

## Try the non-network tests first

From a developer checkout:

```sh
node --check tools/receiver-ack-lab/app_observer.js
node --check tools/receiver-ack-lab/max_receiver_bridge.js
node --test tools/receiver-ack-lab/app_observer.test.js \
  tools/receiver-ack-lab/max_receiver_bridge.test.js \
  tools/receiver-ack-lab/patch_structure.test.js
```

No Max or receiver credentials are required for these tests. The
deterministic golden HMAC key in the earlier Phase 5J example is
**ONLY** a cross-language test vector and must never be used on a
stage device or inside a real Live set.

## Optional manual Max for Live experiment — NOT A SHOW PROCEDURE

1. On a dedicated **test** Mac, open a separate disposable Ableton
   Live Set. Keep show projects, actual OSC output ports, audio levels,
   and lighting hardware disconnected from this experiment.
2. Load the provided `.maxpat` as a patch within a temporary
   Max for Live device in the Live editor, with
   `max_receiver_bridge.js`, `app_observer.js` and
   `receiver_ack.js` available as relative dependencies.
3. In Max/Live, click the `script start --lab` message manually.
   The Node for Max host must identify itself through
   `process.env.MAX_ENV`. The lab generates a **new, random, in-memory
   32-byte key** automatically; no secret needs to be typed or stored
   in the Live Set. A separately provided
   `STAGECORE_LAB_RECEIVER_KEY_HEX` is optional for controlled
   developer interoperability only. Never use a test-vector key on stage.
4. Let `live.observer` deliver its initial Track 0 value. After
   observing the baseline, click the
   `stagecore_arm_live_fixture 3` message to arm **only** fictional
   `LAB_ONLY` Cue/Execution/Snapshot/Companion IDs for Clip Slot 3
   (10-second deadline, unpredictable local challenge). Arming never
   launches a clip or sends a Cue. The old manual JSON inlet remains
   available only to developers with separately prepared lab metadata.
5. Manually launch **Clip Slot 3 on Track 0** in this disposable Set
   within the 10-second window. A matching post-arm transition may
   produce two HMAC-signed OSC **base64 fixtures to Max's outlet
   only**. They are not sent to StageCore or VDMX, and the key is
   intentionally not provisioned to the Hub, so this is not a
   production-verifiable receipt. No sound/video/physical safety is
   certified.
6. Do not connect the outlet to `udpsend`, `stagecore/go`, or a
   production Hub; do not install this patch into the theater's
   Ableton show until its security and state semantics are reviewed.

The sample's **arming metadata is currently manual and untrusted**:
the signed output proves only that the local Node process had the
key, not that a live StageCore Cue caused the transition. A future
production version requires authenticated provisioning and an
explicit receiver-side binding to the Companion's actual command
journal, Cue Execution and active Show. Do not infer a new Event from
Max's initial property read or from any generic incoming OSC route.

## Async and Track rebinding defense

Node for Max uses asynchronous message delivery. If the Operator
cancels a lab Cue, arms a replacement, or Live changes the bound Track
while the first diagnostic base64 packet is awaiting Max's outlet,
`max_receiver_bridge.js` increments a local **observer epoch** and
suppresses any remaining stale second packet. The first packet may
already have been delivered; there is no recall and no network
endpoint. Unit tests explicitly stall the first outlet, issue Cancel
or rebind, then release it and verify the stale second ACK is dropped.

The Track ID reset also clears its first-value baseline; a newly bound
Track cannot inherit the previous Track's pending lab arm. Actual
out-of-order observations in Max/Live and independent receiving-app
identity **still need attended testing**; these are not a real
authorization boundary.

## Safety/reliability limitations

- In-memory execution history and sequence number are lost across
  Node process restarts; production needs a new trusted receiver
  generation and durable replay accounting before accepting any
  restart's receipts.
- The prototype's time checks use a supplied wall-clock timestamp;
  actual offline Pi/Mac deployments need a proper monotonic clock,
  Session epoch, explicit deadline reconciliation and clock
  uncertainty handling.
- Its `change: true` marker comes from an in-process adapter
  observing differences, **not** a cryptographically verified
  Max/VDMX API transition. Other actors can change a playing slot.
- No authenticated Host-to-Max or Host-to-VDMX command metadata,
  no real receiver-to-Hub network transport or key provisioning,
  no active per-Project authorization cutover, no physically verified
  playback/stopping, and no automatic retry.
- All acknowledgments remain APPLICATION-REPORTED only.
  `physical_output_verified=false` and
  `commands_enabled=false` are mandatory. Never clear blackout,
  launch/stop a Cue, or change GO admission on a lab receipt.

**Status:** Draft prototype with offline tests; not ready for
rehearsal/show integration or deployment to a StageCore-controlled Pi.
