# First Show Runbook

Status: first-show operational runbook for the current StageCore source line.

This runbook intentionally separates:

- **rehearsal-ready software paths** that may be prepared before the next hardware session; and
- the **ESP32 DMX Lighting ACTIVE gate**, which still requires attended physical qualification.

It is not a replacement for formal qualification evidence. A software zero, CI PASS, browser stream, or self-reported health value is never physical DMX/LED proof.

---

## 1. First-show architecture

Use one StageCore Project and one Published Runtime Snapshot as the authority for the rehearsal.

```text
StageCore Hub on Pi
  |
  +-- macOS Companion
  |     +-- VDMX Execution Environment / OSCQuery
  |     +-- local OSC GO receiver: 127.0.0.1:9010
  |     +-- Companion OSC/MIDI capabilities where explicitly configured
  |
  +-- v2 Tablet Players
  |     +-- Hub-owned physical identity
  |     +-- Project assignment from global v2 inventory
  |
  +-- ESP32 DMX Lighting Node
  |     +-- Hub-owned physical identity
  |     +-- project-independent BLOCKED/blackout foundation on firmware main
  |     +-- ACTIVE image remains a separate physical qualification gate
  |
  +-- standalone Camera Media Relay on Pi
        +-- one ESP32-CAM upstream
        +-- maximum four downstream viewers by default
```

The physical device identity belongs to the Hub, not to one Project. Project authority is the Hub-owned assignment sidecar and exact Published Runtime Snapshot.

---

## 2. Rehearsal-ready scope before Lighting ACTIVE qualification

The following path can be prepared independently of the Lighting ACTIVE firmware gate:

1. Hub and Operator UI.
2. macOS Companion and Machine Role.
3. VDMX Demo / unsaved Execution Environment.
4. v2 Tablet assignment/reassignment.
5. standalone Camera Media Relay.
6. StageCore Cue/Session execution.
7. local OSC `/stagecore/go` from VDMX, Ableton, or another localhost OSC sender.

For the first integrated rehearsal, start in **REHEARSAL**, not SHOW.

Lighting may be omitted from the first software rehearsal or kept in deterministic blackout until the physical ACTIVE gate in section 9 passes.

---

## 3. Start and verify the Hub

Use the normal installed StageCore Hub service on the Pi.

Before touching show outputs, verify:

- the Operator UI opens;
- the intended Project opens;
- no unintended SHOW session is already active;
- the latest intended revision is Published;
- a current Runtime Snapshot exists for that Published revision;
- the Stage Device global inventory loads.

Do not create a second Project merely to make a device appear. v2 devices are globally visible and reusable; assignment determines current Project authority.

---

## 4. Start the macOS Companion

From the StageCore checkout on the Mac:

```bash
cd companion/macos
swift run stagecore-companion
```

Normal first-run behavior:

1. discover the Hub through Bonjour;
2. verify Hub identity;
3. create/recover the Companion identity from Keychain;
4. request pairing if needed;
5. wait for local Hub approval;
6. authenticate and connect the runtime channel;
7. report `RUNNING`.

The default local show-control OSC receiver is:

```text
UDP 127.0.0.1:9010
/stagecore/go
```

No `--osc-control-port` argument is needed for the default.

If port 9010 is unavailable, the authenticated Companion runtime must remain connected; only the local OSC GO surface is unavailable for that process.

### Companion PASS

The Operator UI must show the intended Companion online and the assigned Machine Role ready for the current Runtime Snapshot.

Do not use a stale Machine Role assignment from another Mac or another Runtime Snapshot.

---

## 5. VDMX Demo / unsaved workflow

The first-show VDMX path does **not** require a savable VDMX project file.

In the guided Execution Environment setup:

- adapter: VDMX;
- workspace path: leave blank when using VDMX Demo / an unsaved workspace;
- OSCQuery URL: default `http://127.0.0.1:8080/` when VDMX publishes OSCQuery locally.

Then:

1. OPEN VDMX through the Execution Environment operation.
2. Build or restore the live VDMX state manually as required.
3. Enable/publish the intended VDMX OSCQuery namespace.
4. Run CAPTURE_SNAPSHOT.
5. Keep the returned snapshot explicitly classified as **PARTIAL / reconstruction metadata**.

The snapshot may record published controls and observable values. It is not a substitute for a complete VDMX project file and must not claim unpublished layer, plug-in, FX, media-bin, or hidden application state.

### VDMX -> StageCore GO

A VDMX control, Ableton control, Stream Deck bridge, or other local OSC sender may trigger the next StageCore Cue by sending:

```text
/stagecore/go
```

to:

```text
127.0.0.1:9010/UDP
```

Use no argument or integer `1`.

The OSC sender does **not** supply Project IDs, Session IDs, Runtime Snapshot IDs, or Cue IDs. The Hub resolves the current authoritative session and next Cue.

For the first rehearsal, validate GO in this order:

1. Operator UI GO.
2. one local OSC GO.
3. repeated local OSC GO only after the current/next Cue display advances correctly.

---

## 6. Reusable v2 Tablet Players

Open the Stage Devices workspace for the intended Project.

The **global reusable v2 inventory** must continue to show a tablet even when it is currently assigned to another Project.

### UNASSIGNED tablet

When the tablet is online and the current Project has a Published Runtime Snapshot:

1. choose **Assign tablet to this Project**;
2. StageCore requests the authenticated safe-media state;
3. the Hub commits the assignment with a new epoch;
4. the tablet reconnects;
5. the tablet acknowledges the exact Project + Runtime Snapshot;
6. only then may commands become enabled.

### ACTIVE tablet assigned to another Project

Choose **Move tablet to this Project**.

StageCore must send the exact old Hub-owned:

- Project ID;
- Runtime Snapshot ID;
- assignment epoch;

as the expected source scope.

The transition is:

```text
ACTIVE Project A / Snapshot A
  -> authenticated safe-media ACK
  -> atomic assignment epoch change
  -> reconnect
  -> ACTIVE Project B / Snapshot B
```

The old Project must lose command authority after the move.

### Tablet PASS

For each show tablet:

- online;
- assignment state ACTIVE;
- assignment Project is the opened Project;
- assignment Runtime Snapshot is the intended Published Runtime Snapshot;
- readiness READY;
- required media capability present;
- Prepare/Play/Blackout works on the intended device before broad/group sends are used.

Do not solve an offline tablet by creating another StageCore Project.

---

## 7. Camera Media Relay for the first show

For the first show, keep Camera Media Relay **standalone** rather than coupling its lifecycle to the Hub.

The current isolated relay source remains Draft and should be built from its exact branch/PR checkout.

Build on the Pi:

```bash
go test ./internal/camerarelay ./cmd/stagecore-camera-relay
go build -o /tmp/stagecore-camera-relay ./cmd/stagecore-camera-relay
```

Run on the trusted show LAN:

```bash
/tmp/stagecore-camera-relay \
  -source http://<camera-ip>:81/api/v0/stream \
  -listen <pi-show-lan-ip>:9081 \
  -allow-lan
```

Health:

```bash
curl --max-time 5 http://127.0.0.1:9081/api/v0/health
```

Expected healthy fields include:

```text
state=ready
upstream_connected=true
last_frame_age_ms reasonably fresh
max_clients=4
```

Downstream stream:

```text
http://<pi-show-lan-ip>:9081/api/v0/stream
```

Do not point four tablets directly at the ESP32-CAM. The relay owns the one upstream connection and fans out to bounded downstream viewers.

The tested path has already demonstrated:

- four simultaneous downstream viewers;
- fifth viewer rejected with HTTP 503;
- source frames continuing during the four-viewer test;
- browser playback without meaningful stage-use lag/freezing.

This does **not** replace one-tablet-then-four-tablet Android playback qualification.

### Camera rehearsal gate

Before relying on the camera in a Cue:

1. relay health READY;
2. one real tablet displays the relay stream;
3. then test all intended tablets;
4. disconnect/reconnect the ESP32-CAM once and confirm the relay recovers;
5. never expose the unauthenticated relay to untrusted Wi-Fi or the internet.

---

## 8. Ableton Live and show GO

For the first rehearsal, keep the control topology simple.

Recommended initial direction:

```text
operator / VDMX / Ableton control surface
        -> local OSC /stagecore/go
        -> Companion
        -> Hub
        -> next StageCore Cue
```

StageCore Companion also contains outbound OSC and MIDI capability executors, but use them only where the Project has an explicitly authored and tested action.

Do not create a bidirectional feedback loop where Ableton advances StageCore while the same StageCore Cue also causes Ableton to send another GO.

Validate one audio Cue at a time before combining it with Tablet/VDMX/Lighting actions.

---

## 9. ESP32 DMX Lighting ACTIVE physical gate

Firmware `main` contains the safe source foundations:

- bounded DMX diagnostics;
- GPTimer/IRAM-safe esp_dmx path;
- project-independent v2 assignment;
- deterministic blackout/BLOCKED epoch ACK;
- optional read-only software state probe.

The ACTIVE runtime remains in the separate Draft firmware PR and must not be promoted merely from CI.

### Required attended sequence

With the real decoder/LED path under observation:

1. pin the exact Hub and firmware build SHAs;
2. start with physical output expected dark;
3. flash only the intended ACTIVE qualification candidate;
4. verify boot remains physically dark;
5. verify Hub restart remains dark;
6. verify Wi-Fi loss remains dark;
7. verify reconnect remains dark until current scope is acknowledged;
8. verify power cycle remains dark;
9. assign/transfer the reusable node to the intended Project;
10. activate only the exact Published Runtime Snapshot configuration;
11. verify config hash matches;
12. verify one bounded nonzero test Cue;
13. verify immediate/faded return to zero as authored;
14. verify disconnect during/after output returns to failsafe blackout;
15. transfer ACTIVE Project A through blackout to Project B;
16. verify Project A loses authority;
17. apply Project B's exact Published configuration;
18. verify one bounded Project B Cue and return to blackout.

Independent physical decoder/LED observation is required. ESP software zero, DMX health, UART logs, CI, or StageCore UI status alone cannot mark this gate PASS.

Until this gate passes, do not enable Lighting ACTIVE merely to complete a rehearsal checklist.

---

## 10. Create the first integrated REHEARSAL session

Before starting:

- current Project is correct;
- intended revision is Published;
- Runtime Snapshot is current;
- Companion is RUNNING and Machine Role is ready;
- VDMX Execution Environment is open;
- Tablet Players are ACTIVE/READY on this Project/Snapshot;
- Camera Relay is healthy if the rehearsal uses it;
- Lighting is either physically qualified ACTIVE or intentionally kept out/blackout;
- Ableton is loaded and its intended control path has been tested once.

Start a **REHEARSAL** session.

Confirm the Operator surface shows the intended:

- current Cue;
- next Cue;
- session mode REHEARSAL;
- Runtime Snapshot.

### First GO sequence

Use a deliberately harmless first Cue.

1. GO from Operator UI.
2. Confirm command/result completion.
3. Confirm current/next Cue advanced once.
4. Send one local OSC `/stagecore/go`.
5. Confirm exactly one additional Cue advance.
6. Only then continue with normal rehearsal GO.

Do not begin by firing a mixed Cue that simultaneously changes every subsystem.

---

## 11. Suggested subsystem integration order

Bring the show up in this order:

1. Hub + Project + Published Runtime Snapshot.
2. Companion + Machine Role.
3. one Tablet Player.
4. all Tablet Players.
5. VDMX.
6. local OSC GO.
7. Ableton action/control path.
8. Camera Relay on one tablet.
9. Camera Relay on all intended tablets.
10. Lighting ACTIVE only after section 9 passes.
11. mixed Cues.
12. full rehearsal sequence.

This order keeps failures attributable to one subsystem instead of producing a mixed failure with no clear owner.

---

## 12. Stop / closeout

At the end of a rehearsal:

1. stop or complete the REHEARSAL session deliberately;
2. verify Tablets enter the intended safe/end state;
3. verify VDMX/Ableton are left in a known operator state;
4. verify the Camera Relay can be stopped without affecting the Hub;
5. verify Lighting is blackout if it was active;
6. preserve any useful VDMX partial snapshot and rehearsal notes;
7. record blockers before changing configuration.

Session closeout must never depend on replaying old commands on the next startup.

---

## 13. First-show stop conditions

Stop the affected subsystem rather than forcing through the rehearsal if any of these occur:

- Companion is not authenticated/current for the intended Machine Role;
- a Tablet assignment Project/Snapshot does not match the opened Project;
- local OSC GO advances more than one Cue;
- Camera Relay upstream is stale/disconnected and does not recover;
- a fifth relay viewer is being relied upon despite the configured four-viewer limit;
- VDMX reconstruction metadata is being mistaken for a complete saved project;
- Lighting reports nonzero output while expected BLOCKED/blackout;
- Lighting physical state contradicts the software report;
- an old Project retains command authority after a device transfer;
- a SHOW session is active while configuration/assignment changes are being attempted.

---

## 14. First rehearsal success definition

The first rehearsal is successful when the operator can:

1. open the intended Project;
2. start one authenticated Companion;
3. recover/open VDMX without requiring a saved Demo project;
4. see and assign/move reusable Tablets without reprovisioning them;
5. run the standalone Camera Relay and show its stream on the intended tablets;
6. start REHEARSAL;
7. advance Cues from the Operator UI;
8. advance exactly one Cue from local OSC `/stagecore/go`;
9. observe truthful command/results and device readiness;
10. end the rehearsal cleanly.

Lighting becomes part of this success definition only after the separate physical ACTIVE gate passes.
