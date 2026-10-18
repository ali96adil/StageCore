# SC-024 Phase 5D — Offline loss of control and stop / blackout qualification

**2026-10-18 · SC-024 / integration PR #530 · draft, simulation only**
**NOT an emergency-stop certification and NOT cleared for theater deployment.**

## What the current Hub can and cannot do

`/runtime/stop` stops a **Cue execution** whose adapter supports a
real stop semantic. It does NOT mean Ableton Live stopped playing every
previous Scene, that an external VDMX source stopped, or that all tablets
have stopped. A network OSC send with no acknowledgment cannot prove
the remote state. `/runtime/stop-session` has a separate output safety
hook; its successful UI state must not be used as evidence of a physical
interlock, and a force exit with `FORCE_EXIT_WITHOUT_BLACKOUT` bypasses
normal shutdown safety.

During an active Session, `EmergencyBlackout` durably latches
managed-output GO blocking **before** cancelling Cues and attempting
the configured output-safety callback. If a Lighting node is offline,
times out, or reports a partial failure, the operation reports failure;
its Hub-side latch keeps GO blocked across a process restart. This
is a SOFTWARE admission fence, **not proof of zero DMX voltage,
decoded light output, LED darkness, mains interruption or safe movement
conditions**. If there is no emergency safety adapter, the existing
runtime reports `EMERGENCY_BLACKOUT_UNAVAILABLE`; do not display
`Safe` or `Stopped`.

The current StageCore MVP explicitly specifies that a certified
physical emergency-stop/interlock remains **external** to StageCore.
Neither a browser button, a Raspberry Pi process, an ESP watchdog nor
an OSC packet can be the only personnel-safety system.

## Loss-of-control decision matrix

| Scenario | Truthful application status | On-site response to qualify | Reconnect behavior |
|---|---|---|---|
| Operator browser or Wi-Fi lost, Hub still running | **Control link lost; output state unknown.** In-flight Cue may continue | Use another authenticated local Operator position if available; for urgent safety use independent venue stop/power isolation according to equipment instructions | Re-authenticate, inspect persisted Session/GO/blackout, no replay |
| Hub or Pi process lost | **No Hub GO/STOP transport.** Last software latch state might be persisted but cannot command offline outputs | Dedicated venue safety stops for powered lighting/motion; manual Ableton/VDMX audio/video stop on Mac; local tablet control if available | Recover Pi, verify RTC/time, snapshot and devices, NEVER resume Cues automatically |
| Companion/Mac offline, Hub online | **Ableton/VDMX outputs unknown.** MIDI/OSC STOP cannot be assumed delivered | On-site Mac / audio interface / mixer mute or stop, plus venue procedures | Require fresh authenticated Companion readiness, no queued cue replay |
| ESP32 Lighting / DMX decoder offline | **Software blackout unconfirmed; physical lamp state unknown** | Separate physically accessible power isolator or appropriate dedicated safety device; never assume DMX loss equals blackout | Require fresh node heartbeat, current project/epoch, DMX channel-zero acknowledgment and physical inspection |
| One or more Android tablets offline | **Tablet media state unknown** | Local actor/operator access and safe device controls / independent power if authorized | Confirm each tablet identity, active media and position; no auto playback |
| Hub clock drifted offline | **Clock not authoritative; timing/leases need requalification** | Synchronize from known-good Mac/RTC with approved procedure during non-SHOW window; inspect Session/Node/Companion state | Recheck heartbeats and timestamps; do not infer device readiness from stale time |
| Emergency blackout was requested but timed out | **Requested / unconfirmed; GO still Hub-latched if it reached the durable latch stage** | Treat all unconfirmed outputs as still potentially active; use independent stop | Do not auto-clear or reissue GO; review errors and verified output state |
| All software links are green | **Communications ready, not physical safety verified** | Qualified operator still checks physical state; human-controlled GO/STOP | No authority to self-resume a prior Cue |

**Avoid the term `EMERGENCY_STOP_CONFIRMED` in the UI** unless a separately
certified external device reports an end-to-end verified safe state
through an independently reviewed interface. A DMX zero *software*
receipt, an OSC packet send, and a Companion `READY` flag are insufficient.

## Restore/clear is more privileged than emergency engagement

A user may retain separately granted `show.stop` and
`show.blackout.engage` after `show.go` is revoked, but these are
currently **unwired policy candidates**. A future `blackout clear` or
`force stop without blackout` must remain privileged, confirmed and
physically observed. Never clear a persisted blackout automatically
on process restart, account relogin, or device reconnect.

A true venue emergency isolation control must not depend on a StageCore
cookie, Wi-Fi AP, a Raspberry Pi 4 being powered, SQLite availability,
the macOS Companion, or a browser tab. It must be engineered for the
actual installed equipment and applicable local safety standards,
installed and inspected by a qualified technician.

## Source-only regression tests

`internal/runtimecontrol/offline_blackout_regression_test.go` exercises
the existing Hub code with simulated/in-memory components and a real
test SQLite store:

1. Lighting callback returns an offline/unconfirmed response:
   result is non-success; a durable Blackout latch blocks GO.
2. A new RuntimeControl service sharing the database (simulated Hub
   restart) still refuses GO until deliberate verified clear.
3. The browser request is cancelled after the Hub wrote the latch but
   before the output acknowledged: cancellation must not erase the latch,
   and restarted GO remains blocked.
4. With no configured blackout safety adapter, the runtime cannot claim
   successful emergency intervention.

**These tests do not simulate a powered-down Pi's ability to reach an
offline DMX node or verify a physical blackout.** That requires tests
on actual hardware with safe loads and explicit consent.

## Required attended offline rehearsal, after the release is separately approved

- Back up the known-good installation and verify a restoration path.
- Confirm offline Pi clock and network topology; no WAN required.
- Use a dark/safe lighting load and mixer monitoring at low volume,
  with a qualified operator and an actual accessible independent safety
  control. Do not perform deliberate power cuts with actors on-stage.
- Test browser loss, Hub loss, Companion loss, Lighting loss and tablet
  loss **separately**, never all at once; record outputs' observed
  states, timestamps, what software admitted, independent stop result,
  and whether any automatic Cue replay was attempted.
- Verify GO never replays on reconnect; confirm persisted emergency
  latch and require an authorized manual clear. Mark unknown physical
  results as UNKNOWN, not PASS.
- Review the incident log and sign off before attempting a real SHOW.
  Do not transfer this qualification into the current theater Pi
  without an explicit attended deployment plan.

**Integration state:** no current `main` merge, no Pi deployment, no
device firmware flash, no physical DMX command and no production route
change. This is a risk analysis and limited Hub code regression suite.
