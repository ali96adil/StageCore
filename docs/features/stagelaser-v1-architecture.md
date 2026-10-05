# StageLaser V1 architecture

Status: implementation baseline  
Branch baseline: `main@89c5c801e31cc57d2dd763dbc837ed0441e0352f`

## 1. Goal

StageLaser is an official StageCore-controlled show device for a laser whose existing physical button is a toggle input. The ESP32-C3 never exposes a raw toggle command to StageCore. StageCore and the firmware exchange idempotent desired-state commands and the firmware decides whether a relay pulse is required.

The first hardware target is an ESP32-C3 Super Mini driving a 1-channel 5 V mechanical relay as a dry contact. The control contract deliberately describes a logical switch rather than a relay so a future PhotoMOS/electronic driver can replace the mechanical relay without changing Cue semantics.

## 2. Architecture decision

### Chosen: Hub-managed native adapter + official Controller ADDON

StageLaser V1 uses a native Hub-side adapter for discovery, pairing, health, command dispatch and Cue capability execution. The Operator experience is packaged like the existing Lighting Controller: an official ADDON may own the graphical workspace, while transport, safety, command identity and event history remain StageCore Core responsibilities.

The Stage Devices workspace will surface StageLaser as show hardware using its stable device identity. The everyday operator does not enter an IP address, URL, token or JSON.

### Not chosen: direct `stagecore.device/2` client

The current secure Stage Device v2 gateway is an authenticated Hub-authority protocol with connection generations, assignment epochs, Runtime Snapshot fencing and profile-specific activation for Tablet Player and ESP32 DMX Lighting. StageLaser V1 must not weaken or bypass those guarantees merely to accommodate a smaller ESP firmware.

The secure device gateway remains unchanged.

### Not chosen: standalone F-015 runtime plugin as the device transport

The extension runtime is suitable for supervised executable plugins, but its current brokered network contract is intentionally narrow and its runtime protocol is not a first-class device discovery/pairing/health model. Expanding that subsystem only for StageLaser would create more architecture than the device requires.

The official Controller ADDON remains useful for product/UI ownership without moving command authority out of Core.

## 3. Stable identity and discovery

The firmware owns a persistent random `device_id` generated once and retained in NVS.

The discovery service is:

`_stagecore-laser._tcp`

Discovery is a hint only. An mDNS record never grants command authority.

The final TXT record set will be bounded and versioned. It is expected to carry only non-secret identification and compatibility metadata such as discovery version, device ID, display name, firmware version, API protocol version and capabilities. Secrets are never advertised.

StageCore resolves the current address from the discovered stable identity, so a DHCP address change does not break the logical device binding. IP address remains diagnostic data.

## 4. Pairing and command security

Actuation endpoints must not be anonymously writable on the Stage LAN.

V1 will use a one-time commissioning/pairing flow to establish a per-device credential retained by the Hub and firmware. Discovery itself remains unauthenticated; commands and state-changing configuration require the paired credential.

The exact pairing UX is implemented only after the real ESP32-C3 board controls are verified. A physical/local pairing window is preferred over a permanently open claim endpoint.

No token is exposed in the normal Operator UI.

## 5. Control contract

Canonical profile:

`stagecore.esp32-stagelaser`

Protocol:

`stagecore.laser/1`

Capabilities:

- `laser.arm`
- `laser.disarm`
- `laser.state.set`
- `laser.flash.start`
- `laser.flash.stop`
- `laser.safe_off`
- `laser.state.read`
- `laser.state.resync`

Normal command vocabulary:

- `LASER_ARM`
- `LASER_DISARM`
- `LASER_SET_ON`
- `LASER_SET_OFF`
- `LASER_FLASH_START`
- `LASER_FLASH_STOP`
- `LASER_SAFE_OFF`

Diagnostic/commissioning commands:

- `LASER_STATE_READ`
- `LASER_STATE_RESYNC`

There is intentionally no `TOGGLE` command.

State resync is never a normal Cue action.

## 6. State machine

Safety state and laser state are separate, coupled state axes rather than unrelated booleans.

Arm state:

- `DISARMED`
- `ARMED`

Logical laser state:

- `OFF`
- `TURNING_ON`
- `ON`
- `TURNING_OFF`
- `FLASH_ON`
- `FLASH_OFF`
- `UNKNOWN`
- `ERROR`

This permits truthful states such as `DISARMED + ON + TRACKED` after a software restart. Boot never has to lie by converting that state to OFF.

State quality:

- `TRACKED`: derived from successfully completed software-controlled transitions.
- `CONFIRMED`: reserved for future physical feedback.
- `UNKNOWN`: the physical state cannot be inferred safely.

## 7. Toggle-to-idempotent conversion

The relay is a button simulator, not laser power control.

For `SET ON`:

- tracked OFF -> perform exactly one complete pulse;
- tracked ON -> complete with no pulse;
- UNKNOWN/ERROR -> reject; do not guess.

For `SET OFF`:

- tracked ON -> perform exactly one complete pulse;
- tracked OFF -> complete with no pulse;
- UNKNOWN/ERROR -> reject; do not guess.

A state transition is committed only after the relay has been released successfully. The logical state never flips at relay-pick time.

## 8. Relay timing

The initial mechanical profile is:

- pulse: 180 ms;
- minimum rest between pulses: 250 ms;
- flash range: 0.1..1.0 Hz;
- maximum single flash request: 60 seconds.

These are conservative software defaults, not a substitute for checking the exact relay module. The real hardware may reduce these limits after inspection.

The limits are represented in device health so a future electronic switch can advertise a wider range without changing StageCore Cue/API semantics.

All runtime timing is non-blocking and watchdog friendly.

## 9. Flash safety

`FLASH_START` carries both frequency and a bounded duration. A Cue therefore never relies on continuous network ON/OFF traffic and never creates an unbounded flash if the Hub disappears.

The ESP executes the flash locally.

`FLASH_STOP` stops the local flasher early and settles to OFF when the state is known.

If communication is lost while flash is active, the local duration still expires and the firmware settles to OFF. Loss of the Hub does not extend the requested flash.

## 10. ARM / DISARM

The firmware always boots DISARMED.

ARM authorizes ON/Flash commands but does not pulse the relay. ARM must fail while state is UNKNOWN/ERROR and must not silently correct state.

DISARM:

1. prevents new ON/Flash work;
2. stops any active flasher;
3. if physical state is tracked ON, performs the one required transition to OFF;
4. if state is already tracked OFF, does not pulse;
5. if state is UNKNOWN, remains DISARMED + UNKNOWN and requires resync instead of issuing a blind toggle.

## 11. Restart and persistence

Firmware persists the last stable logical state and command-deduplication information in NVS.

Before beginning a relay pulse it persists an interrupted-transition marker. That marker is cleared only after relay release and stable-state commit.

On restart:

- interrupted-transition marker -> `UNKNOWN`, resync required;
- clean software/watchdog restart with an intact stable record -> retain the tracked stable state but boot DISARMED;
- power-on/brownout is not automatically interpreted as physical laser OFF unless the installation has a separately qualified shared-power guarantee.

The statement “full laser power-up starts OFF” is useful physical behavior but, without a feedback/power sensor, the ESP cannot always prove that the laser was power-cycled together with it.

## 12. Resync

When state is UNKNOWN there is no safe universal pulse that guarantees OFF because the same pulse can turn an already-OFF laser ON.

Recovery therefore uses an explicit commissioning action:

- operator verifies physical laser state;
- while DISARMED, `LASER_STATE_RESYNC` asserts only stable OFF or ON;
- the command updates tracked software state without pulsing the relay;
- normal operation can resume only after a safe known state is restored.

Resync is Advanced/Diagnostics only and never a Cue action.

## 13. Command identity and retries

Every state-changing request carries a StageCore command/execution ID.

The firmware retains a bounded recent-command journal and the last accepted/applied IDs. Receiving the same command ID again returns the prior result and never repeats a relay pulse.

The Hub does not create a new command ID as an automatic retry for an ambiguous state-changing operation. If transport reconciliation is added, it may query status or retransmit the same ID only, relying on firmware deduplication.

## 14. Hub adapter responsibilities

The native adapter will:

- browse StageLaser mDNS announcements;
- maintain stable identity -> current endpoint resolution;
- pair and retain per-device credentials;
- poll/read bounded health;
- expose Online/Offline, arm state, logical state and quality;
- surface RSSI, IP, firmware, uptime, last command and relay pulse count;
- translate Cue capability requests into StageLaser commands;
- preserve command IDs across uncertain transport outcomes;
- refuse anonymous or incompatible endpoints;
- never infer trust from mDNS alone.

## 15. Operator integration

The Stage Devices workspace will receive a StageLaser section/card using existing StageCore UI conventions.

Normal UI:

- name;
- online/offline;
- armed/disarmed;
- ON/OFF/UNKNOWN;
- state quality;
- Wi-Fi RSSI;
- IP;
- firmware;
- uptime;
- last command;
- diagnostics/pulse count.

Advanced/commissioning UI owns pairing, firmware compatibility, resync and detailed diagnostics.

Normal operation does not expose raw endpoint, credential or JSON fields.

## 16. Cue integration

The visual builder follows the Tablet Controller pattern: the Hub facade converts a friendly request into canonical Cue actions so the browser does not author target refs or capability strings directly.

Target logical type will be StageLaser-specific and resolve by stable `device_id`.

Mixed Cue consumes the same canonical actions; there is no separate execution path for Mixed Cue.

## 17. Emergency safe state

StageLaser will become an explicit managed-output Emergency Blackout domain.

Emergency safety requests `laser.safe_off`, not raw toggle.

Known tracked ON may safely transition OFF. Known OFF is a no-op. UNKNOWN must be reported truthfully as a partial safe-state failure/resync requirement; StageCore must never claim physical OFF when it cannot know it.

Releasing Emergency Blackout never auto-arms or auto-restores the laser.

## 18. Firmware boundary

The production ESP32-C3 firmware will include:

- safe GPIO initialization before networking;
- relay-driver abstraction;
- non-blocking state machine;
- NVS identity/config/state;
- command deduplication journal;
- Wi-Fi provisioning and fallback AP;
- authenticated local control API;
- mDNS advertisement;
- browser OTA with safe output handling;
- health/diagnostics API;
- watchdog-friendly scheduling.

The relay GPIO is intentionally not selected in this architecture slice. It will be fixed only after the exact ESP32-C3 Super Mini board and relay module are verified.

## 19. Failure policy

- Wi-Fi loss: local bounded flash completes then OFF; no new commands; tracked stable state retained.
- Hub loss: same as Wi-Fi loss; no autonomous ON.
- ESP restart: boot DISARMED; restore only provable stable state.
- timeout: no new-ID automatic retry; reconcile by command ID/status.
- reset during pulse: UNKNOWN + resync required.
- duplicate command: prior result returned, no duplicate pulse.
- flash + connectivity loss: bounded local timer settles OFF.
- OTA/reboot: output initialized inactive before network/OTA services and device returns DISARMED.

## 20. Hardware gate still pending

Before firmware GPIO/driver code is finalized, verify:

- exact ESP32-C3 Super Mini board revision/pinout;
- safe non-strapping GPIO;
- whether the relay input is active-high or active-low;
- whether 3.3 V logic reliably drives the 5 V module;
- isolation/opto topology;
- dry-contact voltage/current at the laser button;
- NO/COM behavior;
- whether ESP and laser share a power domain.

No source code may assume those details before the physical module is identified.
