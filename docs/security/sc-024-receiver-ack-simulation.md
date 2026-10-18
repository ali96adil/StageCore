# SC-024 Phase 5I — Correlated receiving-application acknowledgment (simulation)

**2026-10-19 · Draft integration PR #530. Unwired model and tests only.**

## Verified existing semantics

The macOS Companion's `OSCExecutor` returns `TRANSPORT_ONLY` when
the local OS accepts the outgoing UDP datagram. The `MIDIExecutor`
likewise returns `TRANSPORT_ONLY` when CoreMIDI accepts a send.
The Hub's `execution.result` from Companion therefore **does not**
prove that VDMX received the OSC address or that Ableton Live launched
the requested Scene. Neither command format nor the existing reliable
Companion channel is changed here.

## Prepared correlation state machine

`internal/outputwatch/receiver_receipts.go` is an **in-memory,
unwired** diagnostics tracker for a possible future *application-side*
receipt. It never binds a UDP socket, sends commands, modifies
Snapshots/Projects, enqueues or replays Cues, or affects GO/STOP.

A future authenticated receiving-application adapter could correlate:

- Exact Project, published Runtime Snapshot, Machine Role, Companion ID,
  receiver application instance, kind (ABLETON or VDMX), and monotonically
  increasing receiver connection generation.
- Exact Cue ID **and** Execution ID, with an unpredictable one-time
  challenge supplied by the trusted dispatcher (at least 16 characters
  in this candidate model) and bounded absolute deadline.
- Strictly increasing receiver report sequence and two distinct
  application claims, `APP_REPORTED_RECEIVED` followed by
  `APP_REPORTED_APPLIED`.

The initial state is `TRANSPORT_ONLY` after the *separate* trusted sender
has confirmed local transport acceptance. Only the bound application
instance for the matching execution/nonce/snapshot can advance the
state. Wrong Cue/Project/Snapshot/Role/Companion, old generation,
missing/repeated report sequence, guessed challenge, unsolicited
APPLIED-before-RECEIVED, expired report and duplicate execution IDs are
rejected. Timeout, disconnect and clock rollback demote unconfirmed
in-flight receipts. A later receiver connection requires a strictly
newer generation even if the earlier socket already disconnected.
A 512-Execution cap fails closed rather than forgetting IDs and
allowing replay.

**This model is NOT message authentication.** A nonce alone is not
proof of sender or receiver identity. Future application transport
MUST authenticate peer identity and authorize the exact Project,
snapshot, active Cue/Session and receiver instance (e.g., a
mutually authenticated private channel); validate monotonic epochs
across Hub/receiver restarts and provide durable replay protection.
Never expose this as a generic unauthenticated UDP OSC acknowledgment
handler. The current tracker deliberately does not implement the
transport, install credentials, or enable any receiving endpoint.

`APP_REPORTED_APPLIED` means **the receiving application claimed it
applied the instruction**, not that Ableton is audibly playing,
VDMX's video is visible, speaker output is silent or the physical
lighting/laser state is safe. Both `PhysicalOutputVerified` and
`CommandsEnabled` remain **false** in every result. This historical
receipt must never be used to auto-restart a Cue or clear Blackout.

## Qualification coverage

`receiver_receipts_test.go` performs software-only simulation:
- Rejects acknowledgment without a bound qualified receiver;
- Exercises wrong Project/role/Snapshot/Companion/receiver instance,
  wrong Cue/Execution/nonce and out-of-order/replayed sequence;
- Ensures RECEIVED before APPLIED and never upgrades the evidence to
  physical verification;
- Verifies late receipt expiry, disconnect, old-generation report,
  reattachment with a new generation, and duplicate Execution IDs;
- Checks fail-closed clock rollback, concurrency of forged reports
  under `-race`, and the bounded execution history.

No `main` merge, Pi deployment, Companion firmware/application
change, Ableton or VDMX configuration change, OSC/MIDI route change,
HTTP endpoint, UI activation, physical command or automatic replay
is part of Phase 5I.

**Next attended development gate:** design a receiving-app add-on
(Max for Live / VDMX adapter) that emits authenticated, Cue-correlated
application-side acknowledgment after the app observes the action,
then test actual local software acknowledgment before calling it
production-ready.
