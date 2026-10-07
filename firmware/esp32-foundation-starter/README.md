# StageCore ESP32 Firmware Foundation starter

Use this directory as the starting contract for a new ESP-IDF Stage Device.
It complements, and does not replace, the canonical
`docs/features/stage-device-firmware-foundation.md`.

A new device must keep Foundation concerns separate from device-specific output
or show authority.

## Required choices

Before writing hardware logic, define:

- a stable `profile_id`
- a human-readable default display-name prefix
- architecture / board target
- the device-specific safe state
- whether the device has any show/output capabilities
- whether OTA is supported and physically qualified

Do not add a custom StageCore Setup-Wi-Fi page. Once the authenticated device
advertises `device.maintenance.setup-ap-password`, the existing generic
Stage Devices UI exposes the masked Set / Reset-default controls automatically.

## Foundation invariants

- persistent UUID + P-256 identity
- verified/pinned Hub trust
- authenticated `stagecore.device/2`
- first-run Setup AP + router-loss Recovery AP
- device-specific AP SSID
- fallback Setup/Recovery password `12345678`
- persisted local password override
- authenticated SET / RESET_DEFAULT only
- password is never returned or logged
- maintenance is never Cue / Session / Runtime Snapshot authority
- recovery must preserve identity, Hub trust and Hub-owned assignment
- CI must prove Foundation contract before the firmware is StageCore-Ready

## Adapter boundary

The Foundation owns identity, pairing, trust, maintenance transport and Setup AP
credential policy. Device firmware owns hardware-safe behavior.

A device adapter must provide the values in
`stagecore_device_profile.h.example` and keep all output/actuation code outside
the generic Foundation.

## Suggested repository bring-up

1. Start with an inventory-only v2 image:
   - BLOCKER readiness
   - UNASSIGNED only
   - no output capabilities
2. Prove identity + Hub pairing + authenticated reconnect.
3. Add Foundation Setup/Recovery AP maintenance.
4. Add device-specific observations.
5. Add output/show capabilities one slice at a time.
6. Physically qualify any actuation separately from CI.

The Camera C1 migration is intentionally following this sequence.
