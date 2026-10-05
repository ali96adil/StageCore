# StageLaser V1 architecture

Status: implementation baseline  
StageCore baseline: `main@89c5c801e31cc57d2dd763dbc837ed0441e0352f`  
ESP32 precedent reviewed: `ali96adil/StageCore-ESP32-DMX-Lighting@ef338b1d1533b25782c81deb9f0638d23c786e2b`

## 1. Goal

StageLaser is an official StageCore Stage Device for an ESP32-C3 that simulates the original laser's momentary toggle button through a dry-contact output.

The physical button is toggle-only, but the StageCore contract is desired-state and idempotent. There is intentionally no raw TOGGLE command.

The initial driver is a 1-channel 5 V mechanical relay. The public StageCore contract describes logical laser state rather than relay mechanics so a future PhotoMOS/electronic switch can replace the relay without changing Cue semantics.

## 2. Architecture decision

### Chosen: native authenticated Stage Device v2

StageLaser V1 uses the existing secure StageCore device architecture:

```text
StageCore Hub
  _stagecore-hub._tcp discovery
  secure pairing / remembered device identity
  TLS Stage Device gateway
  stagecore.device/2 assignment + Runtime Snapshot authority
  canonical Command Envelope + command results
  Cue Engine / event history / Emergency Blackout
        |
        v
official StageLaser Controller ADDON
  graphical device / diagnostics / Cue Builder UX
  no independent network authority
        |
        v
ESP32-C3 StageLaser
  persistent P-256 identity
  Hub certificate pinning
  authenticated TLS WebSocket Stage Device runtime
  local state machine / dedupe / flash timing
        |
        v
output-driver abstraction
  mechanical relay V1
  PhotoMOS/electronic switch later
        |
        v
laser momentary button dry contact
```

### Why this supersedes the temporary Hub-adapter idea

The production `StageCore-ESP32-DMX-Lighting` firmware already proves that ESP32 firmware in this project can implement:

- ESP-IDF;
- Hub mDNS discovery;
- persistent P-256 device identity;
- Hub certificate pinning;
- secure pairing/authentication;
- TLS WebSocket Stage Device runtime;
- Stage Device command envelopes;
- command-ID deduplication;
- v2 assignment/scope handling.

Therefore creating a second HTTP/adapter command path for StageLaser would duplicate working trust, pairing, reconnect, command history and Cue authority.

StageLaser adds a new authorised product profile to the v2 machinery; it does not weaken the secure gateway and does not create an anonymous LAN actuation route.

## 3. Discovery and DHCP

StageLaser follows the existing Stage Device convention: the device discovers the Hub, not the other way around.

The ESP32-C3 browses the existing:

`_stagecore-hub._tcp`

service and verifies the Hub identity/certificate exactly as the ESP32 DMX node does.

A separate `_stagecore-laser._tcp` service is not required for V1.

The device identity is persistent and independent of IP address. The ESP reconnects outbound after DHCP changes, so normal StageCore operation never stores or asks the operator for a fixed device IP.

IP may still appear in diagnostics.

mDNS discovery never grants trust by itself.

## 4. Profile and protocol

Official profile ID:

`stagecore.esp32-stagelaser`

Stage Device protocol:

`stagecore.device/2`

StageLaser control contract:

`stagecore.stagelaser/1`

The Stage Device coarse kind remains `GENERIC`, matching the existing ESP32 DMX precedent. Product semantics come from the official profile ID.

The canonical Cue target remains the existing Stage Device logical target with stable `device_id`; no new parallel target transport is introduced.

## 5. Capabilities and commands

Capabilities:

- `laser.arm`
- `laser.disarm`
- `laser.state.set`
- `laser.flash.start`
- `laser.flash.stop`
- `laser.safe_off`
- `laser.state.read`
- `laser.state.resync`

Cue-safe commands:

- `LASER_ARM`
- `LASER_DISARM`
- `LASER_SET_ON`
- `LASER_SET_OFF`
- `LASER_FLASH_START`
- `LASER_FLASH_STOP`
- `LASER_SAFE_OFF`

Commissioning/diagnostic commands:

- `LASER_STATE_READ`
- `LASER_STATE_RESYNC`

There is deliberately no `LASER_TOGGLE`.

State resync is never a normal Cue action.

## 6. State model

Safety state and laser state are separate, coupled state axes.

Arm state:

- `DISARMED`
- `ARMED`

Logical state:

- `OFF`
- `TURNING_ON`
- `ON`
- `TURNING_OFF`
- `FLASH_ON`
- `FLASH_OFF`
- `UNKNOWN`
- `ERROR`

State quality:

- `TRACKED`: software-tracked from completed local transitions;
- `CONFIRMED`: reserved for future physical feedback;
- `UNKNOWN`: physical state cannot be inferred safely.

This allows truthful combinations such as `DISARMED + ON + TRACKED` after a clean ESP software restart. Boot never has to lie by rewriting logical state to OFF.

## 7. Toggle-to-idempotent conversion

The relay is a button simulator, not laser power control.

`SET ON`:

- tracked OFF -> exactly one complete output pulse;
- tracked ON -> no pulse;
- UNKNOWN/ERROR -> reject, no pulse.

`SET OFF`:

- tracked ON -> exactly one complete output pulse;
- tracked OFF -> no pulse;
- UNKNOWN/ERROR -> reject, no pulse.

A stable logical-state transition commits only after output release completes.

## 8. Output timing

Initial mechanical defaults:

- pulse: 180 ms;
- minimum rest: 250 ms;
- flash frequency: 0.1..1.0 Hz;
- maximum single flash request: 60 seconds.

These are conservative software defaults and remain subject to the exact relay module qualification.

Limits are observable device data. A future electronic output driver may advertise wider limits without changing the StageCore command vocabulary.

Runtime timing is non-blocking and watchdog-friendly.

## 9. Flash

`LASER_FLASH_START` always carries both frequency and bounded duration.

The device performs flashing locally. StageCore never streams ON/OFF toggles over the network.

`LASER_FLASH_STOP` ends a running flash early.

When flash ends normally or because of STOP, the target stable state is OFF when logical state is known.

If the Hub/Wi-Fi disappears during flash, the already accepted bounded local request still expires locally and attempts its deterministic tracked OFF settlement. No network loss extends flash duration.

## 10. ARM / DISARM

The firmware boots DISARMED.

ARM authorizes ON/Flash but does not itself pulse the output.

ARM must fail while state is UNKNOWN/ERROR.

DISARM:

1. blocks new ON/Flash;
2. stops a running flash;
3. tracked ON -> performs the one required OFF transition;
4. tracked OFF -> no pulse;
5. UNKNOWN -> remains DISARMED + UNKNOWN and requires attended resync.

No boot path issues an automatic blind toggle.

## 11. Restart persistence and interrupted transitions

NVS stores:

- persistent device identity and trust state;
- last stable logical state;
- state quality;
- relay/output limits;
- bounded recent command IDs/results;
- an interrupted-transition marker.

Before asserting the output driver, firmware durably marks the transition in progress. It clears that marker only after release and stable-state commit.

On restart:

- interrupted transition -> UNKNOWN + resync required;
- clean software/watchdog restart with intact stable record -> preserve tracked stable state, boot DISARMED;
- power-on/brownout does not automatically prove laser OFF unless the physical installation has separately qualified shared-power behavior.

The laser's documented full-power-up OFF behavior is useful, but without a feedback or laser-power sensor the ESP cannot always distinguish “laser power-cycled too” from “ESP restarted alone”.

## 12. Resync

UNKNOWN cannot be safely forced OFF by a blind pulse because the same pulse can turn an already-OFF laser ON.

Recovery is therefore explicit and attended:

- keep the device DISARMED;
- operator verifies the physical laser state;
- `LASER_STATE_RESYNC` asserts stable OFF or ON without pulsing;
- normal arming remains blocked until state becomes known.

Resync belongs in Advanced/Diagnostics only.

## 13. Command identity and duplicate delivery

Every command uses the existing Stage Device Command Envelope `command_id`.

Firmware keeps a bounded recent-command journal. A duplicate command ID returns the known result and never repeats the physical pulse.

Reconnect never replays prior commands automatically.

An ambiguous post-dispatch timeout must not be converted into a new command ID. Existing Stage Device connection-generation fencing remains authoritative, while firmware-side dedupe protects physical actuation from duplicate delivery of the same ID.

## 14. Stage Device v2 authority

StageLaser must receive the same v2 principles already used by current Stage Devices:

- authenticated device identity;
- new identity starts UNASSIGNED;
- client never supplies Project authority in v2 hello;
- Hub owns assignment epoch and Runtime Snapshot;
- ACTIVE command authority only after exact Hub scope handshake;
- stale/replaced connection generations cannot complete newer authority;
- command capabilities must be explicitly advertised;
- reconnect never silently restores show authority.

The implementation will extend the v2 profile allowlists/policies specifically for `stagecore.esp32-stagelaser`; it will not make arbitrary GENERIC v2 devices ACTIVE.

## 15. Observations and Device Card

The StageLaser observation contract includes:

- firmware version;
- control-contract version;
- boot ID;
- uptime;
- reset reason;
- Wi-Fi RSSI;
- diagnostic IP;
- arm state;
- logical state;
- state quality;
- resync required;
- output pulse in progress;
- pulse count;
- driver kind;
- timing/flash limits;
- active flash;
- last accepted/applied command IDs;
- last command type/result.

Canonical Online/Offline, readiness, last seen and authenticated network state remain owned by the Stage Device runtime.

The Operator Device Card will present these values using existing Stage Devices patterns.

## 16. Cue Builder and Mixed Cue

The graphical authoring flow follows the existing Tablet/Lighting pattern:

`Add Action -> StageLaser -> Device -> Action`

The Hub facade converts visual choices to canonical Cue actions. The browser never asks ordinary users for target refs, capability keys, JSON, IPs or URLs.

Mixed Cue reuses the same canonical actions; there is no second execution path.

Example:

```text
Device: Laser Left
Action: Flash
Frequency: 1 Hz
Duration: 8 sec
```

## 17. Emergency Blackout / Safe Off

StageLaser becomes an explicit managed-output safety domain.

Emergency Blackout uses `LASER_SAFE_OFF`.

- tracked ON -> one deterministic OFF transition;
- tracked OFF -> no pulse;
- UNKNOWN -> no blind pulse; report partial safe-state failure / resync required.

Releasing Emergency Blackout never automatically arms or restores the laser.

## 18. Official Controller ADDON

Like Lighting Controller, StageLaser will have an official non-executable Controller ADDON for product/UI ownership.

The ADDON may own:

- workspace/device presentation;
- graphical Cue actions;
- readiness/diagnostics presentation;
- commissioning/resync UI.

It does not own:

- network sockets;
- pairing;
- command authority;
- device command persistence;
- show history.

Those remain native StageCore Core responsibilities.

## 19. Firmware repository boundary

Production firmware will live in a dedicated repository rather than mixing ESP-IDF sources into StageCore Core.

Planned repository:

`ali96adil/StageCore-ESP32-StageLaser`

Its foundation should reuse/adapt the proven non-DMX infrastructure from `StageCore-ESP32-DMX-Lighting`:

- ESP-IDF / PlatformIO build + CI conventions;
- persistent P-256 device identity;
- Stage LAN provisioning and recovery;
- Hub discovery;
- Hub trust/certificate pinning;
- pairing/authentication;
- Stage Device v2 runtime;
- command envelope validation;
- command dedupe;
- observations;
- OTA/recovery conventions where applicable.

DMX/output-specific code is not copied.

## 20. Hardware gate

GPIO and electrical driver code remain deliberately unfixed until the actual hardware is identified.

Before selecting the ESP32-C3 output pin, verify:

- exact ESP32-C3 Super Mini revision/pinout;
- strapping/boot/USB/JTAG constraints;
- relay IN active level;
- whether 3.3 V logic reliably drives the 5 V relay module;
- optocoupler/transistor topology;
- relay default state during MCU reset;
- dry-contact voltage/current at the laser button;
- correct NO/COM terminals;
- whether ESP and laser share a power domain.

Cold boot acceptance requires zero unintended contact closure.

## 21. Failure policy

- Wi-Fi loss: no new commands; bounded local flash completes; stable tracked state retained.
- Hub loss: same; device cannot autonomously arm or turn ON.
- ESP restart: boot DISARMED; restore only provable tracked state.
- command timeout: no new-ID blind retry.
- reset during pulse: UNKNOWN + resync required.
- duplicate command: return known result, no duplicate pulse.
- flash + network loss: bounded local flash expires locally.
- OTA/reboot: output driver is initialized inactive before networking/runtime.

## 22. Implementation order

1. contract + tests;
2. official StageLaser Device Profile;
3. Stage Device v2 enrollment/assignment/command authority for the StageLaser profile;
4. deterministic software StageLaser simulator/state machine tests;
5. Cue Engine forwarding + visual Cue Builder/Mixed Cue;
6. Operator Device Card / commissioning / diagnostics;
7. managed-output Emergency Blackout integration;
8. official StageLaser Controller ADDON;
9. dedicated ESP32-C3 firmware repository;
10. software CI freeze;
11. attended hardware qualification.
