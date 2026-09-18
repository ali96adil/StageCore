# ESP32 DMX Lighting Node Integration

Status: Slice 4 logical aliases + graphical Lighting Cue Builder

Tracker: #147

## Current architecture decision

The ESP32 DMX Lighting Node is a native **Stage Device integration** over the existing authenticated `stagecore.device/1` runtime.

The transport, trust and command authority remain in StageCore Core:

- secure device gateway and Hub discovery from F-004;
- existing pairing, remembered identity, revocation and short-lived runtime sessions;
- existing Stage Device WebSocket runtime;
- existing Command Envelope, command persistence, command result lifecycle and canonical event records;
- existing Cue Engine Stage Device forwarder;
- existing immutable Runtime Snapshot and published revision authority.

A new executable F-015 network plugin is **not** the transport boundary. Creating one would duplicate pairing, trust, reconnect, command history and Stage Device authority that already exist.

The intended product composition is therefore:

```text
StageCore Core
  secure stagecore.device/1 transport
  pairing / trust / revocation
  Command Envelope + results
  Cue Engine + Runtime Snapshot authority
        |
        v
official Lighting Controller ADDON (later slice)
  F-021 profile contribution
  graphical configuration / Cue Builder / readiness UX
  no independent network authority
        |
        v
ESP32 DMX Lighting Node
  persistent device identity
  authenticated Stage Device session
  local fade/output state machine
  DMX512 output
        |
        v
MAX485 -> DMX decoder -> 24 V LED strips
```

This mirrors the already-merged Tablet Controller split: transport stays native Core, while the product-specific profile and graphical experience can be packaged as an official ADDON without requesting a second network permission path.

## Canonical contract reuse

Issue #147 predates the production Stage Device runtime. Current repository authority supersedes several draft terms:

- use existing `contracts.CommandEnvelope.deadline_at`; do not introduce a parallel `expires_at`;
- use existing `contracts.CommandResult` statuses:
  `ACCEPTED / REJECTED / COMPLETED / FAILED / TIMED_OUT / CANCELLED`;
- use existing Stage Device `device.observation` messages and canonical `Readiness`;
- persist command lifecycle through the existing `stage_device_commands` and canonical event recorder;
- let the existing Stage Device runtime fence commands to the exact connection generation and terminalize interrupted execution;
- reconnect never replays prior commands.

The lighting contract only defines lighting-specific payloads and observations.

## Stage Device classification

The current Phase 4 `stage_devices.device_kind` database constraint predates this integration and permits only the existing coarse runtime classes. Slice 2 deliberately does **not** rebuild that trust/command table solely to add a lighting enum.

The ESP32 DMX Lighting Node therefore uses the existing coarse `GENERIC` Stage Device kind plus the authoritative official profile ID:

```text
stagecore.esp32-dmx-lighting-node
```

The profile ID is the product-specific semantic type and is required by revision lighting bindings. This avoids a risky migration of the canonical Stage Device identity/command tables while still giving discovery/profile matching an exact lighting identity.

## Device identity and discovery

The node must have a persistent device identity independent of IP address.

Normal production flow is:

1. the ESP32 discovers the StageCore Hub on the Stage LAN using the existing F-004 Hub discovery model;
2. it uses the existing secure pairing bootstrap;
3. operator approval creates the existing trusted identity;
4. the node authenticates a short-lived Stage Device runtime session;
5. it sends `device.hello` with coarse `GENERIC` device kind, official `stagecore.esp32-dmx-lighting-node` profile ID, and the lighting capabilities;
6. reconnect re-authenticates and never replays old commands.

StageCore must not treat mDNS discovery itself as trust.

## Lighting capability vocabulary

Initial capabilities:

```text
lighting.channels.set
lighting.channels.fade
lighting.blackout
lighting.state.read
lighting.identify
lighting.config.read
lighting.config.apply
```

Corresponding Stage Device command types defined by Slice 1:

```text
LIGHTING_CHANNELS_SET
LIGHTING_CHANNELS_FADE
LIGHTING_BLACKOUT
LIGHTING_STATE_READ
LIGHTING_IDENTIFY
LIGHTING_CONFIG_READ
LIGHTING_CONFIG_APPLY
```

Slice 3 wires all seven command types into the production Stage Device command path. Runtime/Cue execution is intentionally limited to set/fade/blackout; state/config/identify remain commissioning/operator commands.

## Channel configuration schema

A node owns up to 12 physical channel definitions.

Each channel has:

```text
channel_key
channel_number
display_name
kind
physical_zone
minimum_level
maximum_level
inverted
enabled
```

Initial kinds:

```text
DIMMER
WARM_WHITE
COLD_WHITE
RED
GREEN
BLUE
UNUSED
```

Rules:

- `channel_number` is unique in the node and limited to 1..12 for this hardware target;
- `channel_key` is stable and machine-safe;
- names and zones are operator-facing metadata;
- enabled `UNUSED` channels are invalid;
- ordinary levels are normalized StageCore values in the inclusive range 0..100;
- ordinary values are clamped to configured minimum/maximum before physical conversion;
- inversion is applied only at the physical DMX conversion boundary;
- blackout is a safety command and resolves all enabled channels to logical 0, independently of ordinary show levels.

Project Cue Actions use project-scoped logical aliases. Slice 2 stores their resolved node identity + stable `channel_key` in Runtime Snapshot v5. Cues must not store raw DMX channel numbers.

## Normalized level conversion

StageCore-facing values are 0..100.

For ordinary set/fade output:

```text
logical 0..100
  -> validate
  -> clamp to channel minimum/maximum
  -> scale to DMX 0..255
  -> apply physical inversion when configured
```

The conversion uses nearest-integer scaling.

Blackout is semantically separate from ordinary minimum-level clamping so a configured show minimum cannot prevent deterministic darkness. Firmware must implement its physical dark value consistently with inversion/wiring.

## Command semantics

### Set

`LIGHTING_CHANNELS_SET` applies one or more channel values immediately.

A set command supersedes an active fade.

### Fade

`LIGHTING_CHANNELS_FADE` carries:

```json
{
  "fade_ms": 1200,
  "channels": {
    "front_warm": 0,
    "front_cold_a": 20,
    "front_cold_b": 20
  }
}
```

All channels in one fade share one local monotonic start/end clock.

A newer set/fade/blackout command supersedes the active fade deterministically. The superseded fade becomes terminal `CANCELLED`; it must not silently remain accepted forever.

### Blackout

`LIGHTING_BLACKOUT` is immediate by default.

An optional explicit `fade_ms` may request a controlled fade to blackout. Without it, ordinary active fade timing is bypassed.

Blackout is expected to use P0 authority when issued as an emergency/safety action, while still passing normal StageCore authorization and runtime authority.

### State/config/identify

Read/config/identify commands remain explicit command/result operations. `LIGHTING_IDENTIFY` and `LIGHTING_CONFIG_APPLY` fail closed during active SHOW. `LIGHTING_CONFIG_APPLY` additionally requires a Published Runtime Snapshot and an exact payload match to that snapshot's authoritative configuration for the same lighting node.

## At-most-once and expiry

Every lighting command uses the existing Command Envelope identity.

Required behavior:

- a command whose `deadline_at` has expired is rejected and not executed;
- the same `command_id` is never executed twice by the node;
- duplicate delivery returns the already-known result;
- reconnect does not replay stale commands;
- a replacement Stage Device connection cannot complete an older connection generation's in-flight command;
- interrupted post-dispatch execution remains explicit/ambiguous rather than being automatically retried.

Core already provides connection-generation fencing and canonical command persistence. Firmware must also retain enough bounded recent command identity state to prevent duplicate physical application across transport retries/reconnects.

## Production command authority

Slice 3 keeps all lighting execution inside the canonical Stage Device runtime.

- every lighting command requires the connected device to advertise the official `stagecore.esp32-dmx-lighting-node` profile;
- `LIGHTING_CHANNELS_SET`, `LIGHTING_CHANNELS_FADE`, and `LIGHTING_BLACKOUT` are Cue-safe runtime commands;
- `LIGHTING_STATE_READ`, `LIGHTING_IDENTIFY`, `LIGHTING_CONFIG_READ`, and `LIGHTING_CONFIG_APPLY` are commissioning/operator commands and do not map through the Cue forwarder;
- `LIGHTING_CONFIG_APPLY` must reference a Published Runtime Snapshot for the same Project and its payload must exactly match that snapshot's authoritative configuration for the same node;
- `LIGHTING_IDENTIFY` and `LIGHTING_CONFIG_APPLY` are rejected while a SHOW session is active;
- a long-running local fade may emit an intermediate `ACCEPTED` result and later one terminal result;
- an intermediate `ACCEPTED` result keeps the original in-flight connection binding and does not create a second accepted event or replay authority;
- existing Stage Device disconnect/reconnect generation fencing still terminalizes ambiguous in-flight execution and never replays it automatically.

## Cue authoring and synchronized fades

Slice 4 keeps operator authoring logical and snapshot-safe.

- the graphical Lighting Cue Builder never exposes raw DMX channel numbers, target refs or JSON;
- operators select project logical aliases, normalized levels (0..100), an operation and optional fade duration;
- canonical Cue parameters store logical aliases, not `channel_key` or DMX channel numbers;
- at execution time the Stage Device forwarder resolves those aliases from the **Published Runtime Snapshot**, then sends node-local `channel_key` values to the ESP32;
- aliases that resolve to another node fail closed rather than crossing target authority;
- all selected aliases on the same ESP32 are grouped into one `LIGHTING_CHANNELS_SET` or `LIGHTING_CHANNELS_FADE` command;
- one fade command carries one `fade_ms` and one channel map, so the node can use one local monotonic fade clock for all channels in that command;
- selections spanning more than one node become independent Stage Device Actions using `PARALLEL_BARRIER`; they share the Cue/correlation boundary but cannot claim sample-accurate synchronization across independent ESP32 clocks;
- generated fade/blackout Actions receive a timeout policy that extends beyond the requested fade duration, preventing the previous generic 5-second runtime timeout from cutting off valid long fades;
- blackout authoring targets configured lighting nodes and defaults to P0 when no explicit priority is supplied;
- the source of truth remains ordinary revision-backed Cues/Actions and the normal Cue endpoints, matching the Tablet Scene authoring architecture.

## Startup and failsafe

Production firmware must:

- boot into deterministic blackout;
- never restore previous full brightness merely because power/network returned;
- keep DMX refresh independent of network/UI activity;
- continue an already accepted local fade without requiring further Hub traffic;
- use a bounded connection-loss policy: brief hold, then fade to blackout;
- expose protected local emergency blackout;
- never allow local fallback to become a second authoritative show-history system.

The Slice 1 simulator starts at blackout and never replays commands on reconnect.

## Observation payload

Lighting-specific observation data may include:

- firmware version;
- uptime/reset reason;
- Wi-Fi RSSI;
- current logical channel levels;
- active fade identity/timing/targets;
- last accepted command ID;
- last applied command ID;
- DMX health;
- configuration hash;
- brownout warning;
- current authority source: `STAGECORE / LOCAL_WEB / FAILSAFE`.

This payload travels inside the existing Stage Device observation channel. Canonical ONLINE/OFFLINE/readiness remains StageCore-owned.

## Deterministic simulator

`internal/lightingnode` contains a no-network fake node used for contract tests.

It proves, without physical ESP32 hardware:

- expired commands do not execute;
- duplicate command IDs execute at most once;
- reconnect does not replay prior commands;
- a newer fade cancels the older fade deterministically;
- invalid channel commands fail closed;
- immediate blackout clears all enabled logical outputs;
- normalized level-to-DMX conversion is bounded and deterministic.

Simulator evidence is software evidence only and is not physical qualification.

## Firmware repository boundary

Production ESP32 firmware should **not** be mixed casually into StageCore Core.

No firmware repository is created in Slice 1.

After the StageCore contract/profile/command path is frozen, firmware should live in a dedicated device repository with its own ESP32 toolchain, release artifacts and hardware qualification history. That repository will implement this StageCore contract rather than defining a second authority model.

The firmware boundary must include:

- provisioning without compiled-in Wi-Fi credentials/secrets;
- persistent device identity;
- authenticated Stage Device pairing/session flow;
- 12-channel DMX512 output;
- local fades and deterministic blackout;
- bounded recent-command deduplication;
- safe startup and connection-loss failsafe;
- observations/health;
- protected local provisioning/diagnostic/fallback UI.

## Slice plan after this foundation

1. **Slice 1 — implemented:** contract + deterministic fake node + safety tests.
2. **Slice 2 — implemented:** official F-021 Lighting Node profile + revision-backed node/channel configuration + Runtime Snapshot v5 mapping.
3. **Slice 3 — implemented:** production Stage Device command mapping/result path for set/fade/blackout/state/identify/config, strict payload validation, profile/snapshot authority, and intermediate ACCEPTED → terminal result support.
4. **Slice 4 — implemented:** Cue Engine logical-alias resolution from the Published Runtime Snapshot + graphical bilingual Lighting Cue Builder for set/fade/blackout. The builder groups aliases by lighting node so one node receives one multi-channel command, preserving one local fade clock per ESP32. Cross-node actions are emitted in parallel with the same Cue correlation.
5. **Slice 5:** bilingual/RTL Operator device/configuration/readiness workspace and official ADDON packaging.
6. **Slice 6:** software qualification/freeze, documentation reconciliation and handoff to real ESP32 firmware/physical qualification.

Physical ESP32/MAX485/DMX qualification remains separate and must not be claimed from simulator evidence.
