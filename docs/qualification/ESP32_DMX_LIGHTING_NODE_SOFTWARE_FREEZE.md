# ESP32 DMX Lighting Node — StageCore Software Freeze

Status: FREEZE CANDIDATE — exact-main CI evidence pending

## Scope

This document freezes the StageCore-side software integration for the ESP32 DMX Lighting Node.

It does not claim that production ESP32 firmware exists, that a MAX485/DMX installation has been qualified, or that the full hardware acceptance criteria in Issue #147 have passed.

Implementation candidate after Slices 1–5:

~~~
218a1bc2c8abb6debc35f5bf8e2d8065bf8035b7
~~~

This is the exact main SHA produced by merging PR #220.

At the time this closeout was created, the connected GitHub evidence surface did not yet expose a post-merge Core CI run for that exact SHA. Therefore SOFTWARE READY is not claimed yet. The remaining software-freeze gate is genuine exact-main Core CI PASS evidence.

## Promoted software slices

| Slice | Purpose | PR | Exact-head CI evidence |
|---|---|---:|---|
| 1 | Contract + deterministic simulator | #216 | Core CI #1019 — PASS |
| 2 | Official Device Profile + revision configuration + Runtime Snapshot binding | #217 | Core CI #1022 — PASS |
| 3 | Production Stage Device command/result path | #218 | Core CI #1024 — PASS |
| 4 | Logical aliases + Cue Engine/Cue Builder | #219 | Core CI #1026 — PASS |
| 5 | Lighting Setup + readiness/health + official ADDON | #220 | Core CI #1029 — PASS |

All promoted slices are merged into the implementation candidate above.

## StageCore software evidence

### Contract and simulator

- seven versioned lighting capabilities and command types;
- bounded 12-channel configuration model;
- normalized 0–100 logical levels;
- deterministic conversion, clamping and inversion rules;
- explicit ACCEPTED / REJECTED / COMPLETED / FAILED / TIMED_OUT / CANCELLED lifecycle;
- command expiry and duplicate command-ID handling;
- deterministic fade supersession;
- reconnect without stale command replay;
- immediate and timed blackout semantics;
- canonical configuration hashing.

### Identity, trust and configuration authority

- lighting nodes use the existing authenticated stagecore.device/1 Stage Device runtime;
- no parallel pairing database, transport, command queue or show-history model exists;
- official F-021 profile ID: stagecore.esp32-dmx-lighting-node;
- coarse Stage Device database kind remains GENERIC; product semantics come from the official profile;
- revision-backed structural configuration and project logical aliases;
- F-012 SHOW mutation protection at Store and database-trigger layers;
- Published Runtime Snapshot v5 captures immutable resolved lighting bindings.

### Runtime commands

- set, fade and blackout are Cue-safe Stage Device commands;
- state/config/identify commands remain commissioning/operator commands;
- strict payload validation and canonicalization;
- official lighting profile enforcement;
- intermediate ACCEPTED followed by one terminal result for local fades;
- existing Stage Device connection-generation fencing remains authoritative;
- ambiguous in-flight execution is never replayed on reconnect;
- config apply is Published-Runtime-Snapshot-authoritative;
- identify/config apply remain blocked where SHOW safety requires it.

### Cue authoring and execution

- project Cues store logical lighting aliases, not raw DMX channel numbers;
- the Published Runtime Snapshot resolves aliases immediately before dispatch;
- cross-node alias misuse fails closed;
- one ESP32 receives one multi-channel set/fade command for one local fade clock;
- multi-node operations use ordinary parallel Cue Actions without claiming sample-accurate cross-node synchronization;
- long fade/blackout Actions receive timeout budgets extending beyond the requested fade duration;
- publish validation rejects invalid lighting aliases and commissioning-only Cue capabilities;
- full Cue Engine → Stage Device command → canonical event-chain tests are present.

### Operator workflow and official ADDON

- bilingual/RTL Lighting Setup workspace;
- graphical 12-channel structural configuration;
- graphical Lighting Cue Builder;
- readiness projection from canonical Stage Device observations;
- DMX-health, brownout, authority and configuration-hash visibility;
- logical-alias Identify;
- Apply Published Config commissioning flow;
- official stagecore.lighting-controller ADDON bundled through the existing Software Repository/Vault/Extension Library path;
- ADDON payload is non-executable and owns no independent network authority.

## Acceptance matrix

The original Issue #147 intentionally spans StageCore software, ESP32 firmware and real hardware qualification.

| Acceptance area | Current evidence |
|---|---|
| Configure/rename up to 12 channels in StageCore | SOFTWARE PROVEN |
| Independent warm/cold logical outputs | SOFTWARE PROVEN |
| Logical aliases instead of raw DMX numbers in Cues | SOFTWARE PROVEN |
| Published snapshot preserves resolved mapping | SOFTWARE PROVEN |
| Set/fade/blackout command generation | SOFTWARE PROVEN |
| Multi-channel single-node fade contract | SOFTWARE PROVEN |
| Duplicate/expired command behavior | SOFTWARE PROVEN in deterministic simulator/runtime tests |
| Invalid values/config fail closed | SOFTWARE PROVEN |
| Readiness/current state/last-error projection | SOFTWARE PROVEN against canonical observation contract |
| Canonical command/result/event recording | SOFTWARE PROVEN |
| ESP32 first-run provisioning | PHYSICAL/FIRMWARE PENDING |
| Real ESP32 discovery/pairing/revocation/reconnect | PHYSICAL/FIRMWARE PENDING |
| Continuous real DMX512 output | PHYSICAL PENDING |
| Real fade timing tolerance | PHYSICAL PENDING |
| Local emergency blackout | FIRMWARE/PHYSICAL PENDING |
| Safe boot/brownout/connection-loss behavior | FIRMWARE/PHYSICAL PENDING |
| DMX stability during Wi-Fi/web load | PHYSICAL PENDING |
| MAX485/decoder/24 V electrical qualification | PHYSICAL PENDING |
| Full Pi + Stage LAN + ESP32 + DMX path | PHYSICAL PENDING |

## Firmware handoff contract

Production firmware belongs in a separate device repository and must implement the frozen StageCore contract rather than creating a second authority model.

Minimum firmware responsibilities:

- first-run network provisioning without compiled-in credentials;
- persistent node identity and StageCore pairing/session credentials;
- stagecore.device/1 authenticated runtime;
- the seven lighting capabilities from the integration document;
- bounded recent-command deduplication;
- strict deadline handling;
- 12-channel DMX512 output;
- one local monotonic clock per accepted multi-channel fade;
- deterministic supersession and blackout;
- deterministic safe boot;
- bounded connection-loss hold/fade-to-blackout behavior;
- protected local provisioning/diagnostic/rehearsal-fallback/emergency-blackout UI;
- bounded observations including DMX health, brownout, active fade, current levels and canonical configuration hash.

Firmware must not:

- accept unrestricted anonymous production commands from arbitrary LAN clients;
- invent a second Cue/show-history authority;
- replay ambiguous commands after reconnect;
- restore unexpected full brightness after reset/reconnect;
- compile Stage LAN credentials or long-lived secrets into source.

## Physical qualification handoff

Real qualification must use the intended production chain:

~~~
StageCore Hub / Raspberry Pi
    ↓ authenticated stagecore.device/1
ESP32 DMX Lighting Node
    ↓ UART / RS485
MAX485
    ↓ DMX512
12-channel DMX decoder
    ↓ 24 V outputs
warm/cold LED strips
~~~

Qualification must record at minimum:

- exact StageCore main SHA;
- exact firmware commit/build/version;
- exact ESP32 board/module;
- MAX485 wiring, DE/RE behavior and logic-voltage compatibility;
- DMX polarity, common/grounding and termination;
- decoder model and addressing;
- 24 V PSU and buck-converter measurements;
- startup and blackout state;
- pairing/revocation/reconnect;
- duplicate/expiry behavior on the real node;
- one-channel and multi-channel fades;
- measured fade tolerance;
- long fade;
- superseding fade;
- immediate and timed blackout;
- Wi-Fi loss and Hub restart;
- brownout/power-cycle recovery;
- sustained DMX stability while network and local web activity occur;
- full representative REHEARSAL/SHOW regression.

Physical qualification belongs under the dedicated firmware/physical tracker and the cumulative physical checkpoint in #148.

## Freeze rule

Until genuine post-merge exact-main Core CI PASS evidence is recorded for the relevant freeze SHA:

~~~
StageCore ESP32 DMX Lighting Node integration = FREEZE CANDIDATE
~~~

After that evidence is present:

~~~
StageCore ESP32 DMX Lighting Node integration = SOFTWARE READY
~~~

Neither state means PRODUCT COMPLETE or PHYSICALLY QUALIFIED.
