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

Keep Camera Media Relay as a **standalone systemd service** on the Pi. Its source is merged into the canonical StageCore main tracked by #307; do not build from an old Draft branch or PR.

Build the relay binary from the same checked-out StageCore revision used for the Hub:

```bash
go build -trimpath -o /tmp/stagecore-camera-relay ./cmd/stagecore-camera-relay
sudo install -m 0755 /tmp/stagecore-camera-relay /opt/stagecore/bin/stagecore-camera-relay
```

Install the packaged standalone service and environment template:

```bash
sudo install -m 0644 deploy/systemd/stagecore-camera-relay.service /etc/systemd/system/stagecore-camera-relay.service
sudo install -m 0600 deploy/systemd/camera-relay.env.example /etc/stagecore/camera-relay.env
sudoedit /etc/stagecore/camera-relay.env
sudo systemctl daemon-reload
sudo systemctl enable --now stagecore-camera-relay.service
```

Set the show-LAN values in `/etc/stagecore/camera-relay.env`:

```text
STAGECORE_CAMERA_RELAY_SOURCE=http://stagecam-d44a4c.local:81/api/v0/stream
STAGECORE_CAMERA_RELAY_LISTEN=<pi-show-lan-ip>:9081
STAGECORE_CAMERA_FLASH_CONTROL=http://stagecam-d44a4c.local/api/v0/flash
```

Health:

```bash
systemctl --no-pager --full status stagecore-camera-relay.service
curl --max-time 5 http://127.0.0.1:9081/api/v0/health
```

Expected healthy fields include:

```text
service=stagecore-camera-relay
state=ready
upstream_connected=true
last_frame_age_ms reasonably fresh
max_clients=4
```

The Operator Live Video workspace can also read the bounded relay-health surface natively. That read is observational; it does not replace the direct health check during qualification.

Downstream stream:

```text
http://<pi-show-lan-ip>:9081/api/v0/stream
```

Do not point four tablets directly at the ESP32-CAM. The relay owns the one upstream connection and fans out to bounded downstream viewers.

The tested path has already demonstrated four simultaneous downstream viewers, fifth-viewer refusal, slot release/reacquire, relay restart recovery and selective flash behavior. Final qualification must still exercise one real Tablet first, then all intended Tablets.

### Integrated Tablet Player path

Use the exact integrated Tablet Player main/APK artifact pinned by #307. PR #27 is historical evidence, not the deployment source.

For each Tablet:

1. install the pinned integrated APK;
2. trust/pair until the Tablet is ONLINE/READY;
3. assign the exact current Published Runtime Snapshot;
4. create/publish the Direct Live Cue with the relay URL;
5. confirm the Live command reaches terminal COMPLETED only after the first rendered frame;
6. Hide must release its relay viewer slot;
7. only then expand to all four Tablets.

Before relying on the camera in a Cue:

1. relay service is active;
2. relay health is READY;
3. one real Tablet displays the relay stream;
4. then test all intended Tablets;
5. disconnect/reconnect the ESP32-CAM once and confirm relay recovery;
6. confirm flash AUTO returns OFF when no requesting viewer remains;
7. never expose the unauthenticated relay to untrusted Wi-Fi or the internet.

## 8. Ableton Live and show GO

For new Ableton/IAC Cue Actions, use the guided Companion `midi.send` authoring path with a **stable CoreMIDI destination name**.

Recommended flow:

1. assign the macOS Companion to the intended Audio/Ableton Machine Role;
2. ensure the Companion reports `midi.send` and its current MIDI destination inventory;
3. in the Cue Action builder choose **Send MIDI message**;
4. choose **Stable destination name** and select the exact IAC/CoreMIDI destination reported by that Companion;
5. choose Note On, Note Off, Control Change or Program Change;
6. set MIDI channel 1–16 and the Note/Controller/Program plus Velocity/Value;
7. save the Cue;
8. run Preflight before SHOW.

Example canonical parameters:

```json
{
  "destination_name": "IAC Driver Bus 1",
  "bytes": [144, 60, 127]
}
```

Preflight behavior:

- exact named destination with one current match -> PASS;
- missing named destination -> BLOCK;
- duplicate exact destination name -> BLOCK as ambiguous;
- legacy `destination_index` -> WARN because CoreMIDI ordering can change.

Existing numeric-index Cues remain backward compatible, but do not author new first-show Cues that depend on an index when a stable destination name is available.

In Ableton Live, map the chosen Note or Control Change to the scene, clip, transport or control needed for the rehearsal.

Recommended initial direction:

```text
operator / VDMX / Ableton control surface
        -> local OSC /stagecore/go
        -> Companion
        -> Hub
        -> next StageCore Cue
```

Do not create a bidirectional feedback loop where Ableton advances StageCore while the same StageCore Cue also causes Ableton to send another GO.

Validate one audio Cue at a time before combining it with Tablet/VDMX/Lighting actions.

## 9. ESP32 DMX Lighting ACTIVE physical gate

Use the exact integrated ESP32 DMX Lighting `main` / firmware artifact pinned by #307. The ACTIVE source is no longer selected from a Draft PR; **physical ACTIVE qualification is still mandatory**.

The integrated firmware includes:

- v2 exact-Snapshot authority;
- project-independent assignment/reuse;
- blackout-first boot and BLOCKED state;
- Stage-LAN Wi-Fi self-recovery;
- bounded runtime-loss hold/fade -> blackout;
- protected local diagnostics/emergency blackout;
- physical Hub trust reset/re-pair recovery.

### Required attended sequence

With the real decoder/LED path under observation:

1. pin the exact Hub and firmware build SHAs from #307;
2. preserve rollback before flashing;
3. start with physical output expected dark;
4. flash only the pinned integrated ACTIVE firmware artifact;
5. verify boot remains physically dark;
6. verify Hub restart remains dark;
7. verify Wi-Fi/AP loss enters the documented hold/fade path and reaches physical blackout;
8. verify reconnect remains dark until current Project/Snapshot scope is acknowledged;
9. verify power cycle remains dark;
10. assign/transfer the reusable node to the intended Project;
11. activate only the exact Published Runtime Snapshot configuration;
12. verify config hash matches;
13. verify one bounded nonzero test Cue;
14. verify immediate/faded return to zero as authored;
15. transfer ACTIVE Project A through blackout to Project B;
16. verify Project A loses authority;
17. apply Project B's exact Published configuration;
18. verify one bounded Project B Cue and return to blackout;
19. verify local emergency diagnostics and trust-reset/re-pair recovery.

Independent physical decoder/LED observation is required. ESP software zero, DMX health, UART logs, CI, or StageCore UI status alone cannot mark this gate PASS.

## 9A. Build mixed Cues without JSON

For the first show, keep subsystem authoring visual and reuse the canonical translators already built into StageCore.

Recommended composition workflow:

1. Build a **Tablet Scene** in the Tablet Scenes workspace.
2. Build a **Lighting Cue** in the Lighting Cues workspace.
3. Open the general **Cues** workspace and create the final show Cue.
4. Under **Compose from existing Cues**, choose the Tablet Scene and select **Import actions**.
5. Import the Lighting Cue the same way.
6. Add one or more visual **Send OSC message** or **Send MIDI message** Actions for VDMX and/or Ableton.
7. Review action execution modes/order.
8. Save the mixed Cue.
9. Validate and Publish a new Runtime Snapshot before rehearsal.

Import copies the source Actions into the mixed Cue. It does not link them dynamically and it does not reuse their Action IDs. Later changes to the source Tablet/Lighting Cue do not silently change an already-composed mixed Cue; re-import or edit deliberately.

This keeps the final Cue auditable while avoiding manual capability names, target references, and JSON for the common first-show path.

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
4. verify the Camera Relay can be stopped/restarted without affecting the Hub;
5. verify Lighting is physically blackout if it was active;
6. preserve any useful VDMX partial snapshot and rehearsal notes;
7. record blockers before changing configuration.

### STOP CUE vs Emergency Blackout

`STOP CUE` only interrupts the current Cue / interruptible Actions. It is **not** a global blackout.

`EMERGENCY BLACKOUT` is a separate P0 managed-output safety operation:

- the Hub persists a session blackout latch before touching outputs;
- GO and Jump remain blocked while that latch is active, including across Hub/browser restart;
- managed Lighting, active Tablets and Native Visual are driven toward their blackout state;
- partial activation failure leaves the latch active;
- audio is **UNCHANGED_BY_DESIGN**;
- external VDMX/OSC is **UNCHANGED_BY_DESIGN**;
- clearing managed blackout is explicit; Lighting intentionally remains dark until an explicit Lighting Cue/operator action restores it.

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
