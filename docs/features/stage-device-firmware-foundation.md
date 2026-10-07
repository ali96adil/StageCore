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

## Current device audit

- ESP32 DMX Lighting: first-run + Recovery AP exist. Default password is
  `12345678`. Managed override support is the next firmware integration.
- ESP32 Camera: first-run + Recovery AP exist. Default password is
  `12345678`. It is not yet an authenticated v2 Stage Device, therefore
  remote password mutation must remain disabled until a secure maintenance
  transport exists.
- ESP32 StageLaser: first-run AP exists and the device is authenticated v2.
  Default password is `12345678`. Recovery AP on prolonged Stage LAN loss is
  currently missing and is a Foundation compliance gap.

## New-device acceptance checklist

A new firmware is not StageCore-Ready until its CI proves the applicable
Foundation requirements. Device-specific output/actuation safety remains an
additional contract; Foundation compliance never substitutes for physical
qualification.
