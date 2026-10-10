# Cue overlap: device-output fencing and attended qualification

**Status:** release blocker for Draft PR #475. CI passing is not physical qualification.

## The distinction that matters

The Hub may accept GO2 while GO1 is still running. A successful STOP, STOP SESSION or P0 Blackout must not permit any earlier GO1 command to **restore** an output after the stop. There are three independent boundaries:

1. **Before Hub socket write:** cancel the Cue context and persist CANCELLED/TIMED_OUT. The device never receives the command. The Hub now checks cancellation after accepted-command persistence and again under the WebSocket writer mutex.
2. **After Hub socket write, before device execution:** Hub cancellation cannot retract bytes already delivered. The device must fence old commands, including distinct command IDs, using a monotonic per-output control generation or equivalent authenticated stop epoch. Duplicate-ID replay and assignment/connection epochs alone are insufficient.
3. **During physical output:** STOP/Blackout must preempt in-progress fades, flashes, transitions and queued commands; a delayed completion/ACK must never restore the previous state.

A Hub `command.result=COMPLETED` is evidence of a device report, **not** proof of relay position, DMX output or successful blackout.

## Device protocol and queue requirements

- Bind output generation to the authenticated device identity, project assignment and published runtime snapshot. Define explicit reset/reconnect semantics. Do not reuse generations across ownership transfers.
- STOP/Blackout advances a generation **before** subsequent ordinary output writes; device refuses commands with an older generation even if their command ID is new, and returns a terminal `SUPERSEDED`/cancelled result.
- Emergency safe-off must not wait behind ordinary FIFO output commands or be rejected only because an old ON/FLASH is busy. If the device cannot establish safe state, report a failure and retain physical interlocks.
- Reject unknown, missing or malformed generation on newly negotiated protocol versions; use explicit capability negotiation for older firmware rather than silently assuming it supports fencing.
- Keep idempotency/replay, deadlines, snapshot and project-scoping checks. A newer generation must **not** bypass arming, physical qualification, fail-safe or show authorization.
- Never use a Hub-only sequence as a substitute for a physical kill circuit when required.

## Acceptance matrix

| Scenario | Required observable behavior |
| --- | --- |
| GO1 long DMX fade, GO2 independent Ableton/VDMX cue | GO2 accepted promptly, GO1 continues without blocking; no shared output conflict |
| GO1 tablet output, GO2 same tablet | Conflict rejected or explicitly superseded, never two uncontrolled owners |
| GO1 ON queued, then P0 Blackout, then delayed GO1 ON arrives | Device stays safe/off; old ON rejected with terminal result |
| Flash/DMX fade active when STOP SESSION occurs | Output transitions to safe state; no delayed old completion re-enables it |
| Eight ordinary queued commands then SAFE_OFF | SAFE_OFF preempts backlog; stale queued commands cannot re-enable output |
| Disconnect/reconnect while old commands are queued | Old generation not replayed; device begins in qualified safe state |
| Duplicate/late ACK for old command after STOP | Hub does not change terminal STOP state or output authority |
| StageLaser relay physically qualified with GPIO polarity | Power-up OFF, loss-of-network safe behavior and emergency stop proven **on real hardware** |

## Evidence to attach before release

1. CI run and commit SHA for Hub and each device firmware.
2. Logs with Cue execution IDs, device command IDs, generations, accepted/cancelled/completed timestamps, STOP and Blackout correlation.
3. Attended hardware observations for DMX channels, tablets, OSC/MIDI, StageLaser relay and power/network failure. Do not label simulated GPIO tests as hardware qualification.
4. Explicit decision on STOP LATEST vs STOP ALL UI semantics, with operator rehearsal signoff.
5. Rollback and safe recovery plan; do not deploy the integration draft to the live Pi before acceptance.

## Current implementation status

- Hub pre-socket cancellation and late-ACK/idempotency fencing: implemented in Draft PR #475 and tested in CI.
- Device-side per-output monotonic generation, queued-command supersession and attended proof: **not implemented/qualified across all devices**.
- StageLaser Draft PR #56 now reserves emergency queue capacity, prioritizes SAFE_OFF/DISARM over ordinary FIFO, cancels queued pre-emergency work, and persists an emergency issued-at watermark across reboot. Latest passing no-actuation firmware CI at `fc75c98c` is [run 38029127029](https://github.com/ali96adil/StageCore-ESP32-StageLaser/actions/runs/38029127029); newer command-envelope hardening is under CI. These protections are **unmerged, undeployed and not equivalent** to authenticated per-output generation. Tracked at [StageLaser issue #55](https://github.com/ali96adil/StageCore-ESP32-StageLaser/issues/55).
- The live Raspberry Pi and all physical devices are unchanged by this document.
