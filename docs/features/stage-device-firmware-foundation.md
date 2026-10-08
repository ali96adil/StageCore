# Stage Device Firmware Foundation v1

Status: required baseline for new StageCore-managed embedded devices.

This contract exists so a new firmware does not accidentally omit commissioning,
recovery, maintenance or security behavior that already exists on another device.

## Required baseline

Every new StageCore-managed ESP-class device MUST provide:

1. Persistent device identity.
2. First-run Setup AP with a device-specific SSID.
3. Shared compiled fallback Setup/Recovery AP password `12345678`.
4. WPA2 protection for Setup/Recovery AP.
5. Stage LAN credential portal.
6. Saved-credential reconnect with bounded backoff.
7. Recovery AP after a bounded Stage LAN outage.
8. Recovery setup that changes Stage LAN credentials only; it MUST preserve
   device identity, trusted Hub binding and Project/Runtime assignment.
9. Authenticated `stagecore.device/2` Hub pairing/runtime before accepting
   Hub-managed maintenance.
10. Capability-driven Setup AP credential management:
    `device.maintenance.setup-ap-password`.
11. Persistent local Setup AP password override. If no override exists, use
    `12345678`.
12. Authenticated maintenance operations:
    - `SET`: persist a new 8-63 byte password.
    - `RESET_DEFAULT`: remove the override and restore `12345678`.
13. The Setup AP password MUST never be returned in device observations, API
    responses, logs intended for normal operation, or operator read APIs.
14. Setup AP maintenance MUST remain outside Cue, Session, Runtime Snapshot and
    show execution authority.
15. Device-specific fail-safe behavior while Setup/Recovery AP is active.

## Wire contract

A capable v2 device advertises:

```
device.maintenance.setup-ap-password
```

Hub -> device:

```json
{
  "type": "maintenance.setup_ap_password",
  "schema_version": 2,
  "device_id": "<device UUID>",
  "connection_generation": 42,
  "request_id": "<UUID>",
  "operation": "SET",
  "password": "<8-63 bytes>"
}
```

Reset uses `"operation":"RESET_DEFAULT"` and omits `password`.

Device -> Hub:

```json
{
  "type": "maintenance.setup_ap_password.result",
  "schema_version": 2,
  "device_id": "<device UUID>",
  "connection_generation": 42,
  "request_id": "<same UUID>",
  "maintenance_state": "APPLIED",
  "detail": "Setup AP credential updated"
}
```

The result never contains the password.

## Operator behavior

Stage Devices renders the Setup/Recovery Wi-Fi section only when the authenticated
device advertises the capability. OWNER and TECHNICIAN may set or reset the
password. The UI uses a masked field and never reads the existing password back.

## Current device audit — 2026-10-08

All current StageCore ESP device families satisfy this Foundation contract in
software. Physical qualification remains a separate device-specific gate.

- ESP32 DMX Lighting: Foundation complete. First-run + router-loss Recovery AP,
  fallback `12345678`, persisted local override, authenticated
  `SET` / `RESET_DEFAULT`, and output-authority isolation are on main.
- ESP32 StageLaser: Foundation complete in software. First-run + router-loss
  Recovery AP, fallback `12345678`, persisted local override, authenticated
  `SET` / `RESET_DEFAULT`, APSTA recovery and bounded reconnect are on main.
  Laser physical actuation qualification remains separate.
- ESP32 Camera: Foundation complete in software. Persistent identity/trust,
  authenticated `stagecore.device/2`, managed Setup/Recovery AP password,
  protected first-run + router-loss Recovery AP and the native C3 camera
  coexistence image are on Camera main. The proven AI Thinker/OV2640 MJPEG,
  relay and flash contracts are preserved while maintenance remains isolated
  from camera/show authority. Camera C3 physical qualification remains separate.

StageCore Core renders the generic Setup/Recovery Wi-Fi maintenance UI from the
authenticated capability, including capable reusable/UNASSIGNED inventory.
No per-device StageCore settings page is required.

## New-device acceptance checklist

A new firmware is not StageCore-Ready until its CI proves the applicable
Foundation requirements. Device-specific output/actuation safety remains an
additional contract; Foundation compliance never substitutes for physical
qualification.
