# StageCore Firmware Foundation v1 — ESP-IDF component

This directory is the canonical device-independent StageCore ESP-IDF foundation.

It owns:
- persistent UUID + P-256 identity;
- verified Hub discovery and TLS SHA-256 pinning;
- pairing/authentication primitives;
- trusted clock bootstrap from the pinned Hub;
- fail-closed Hub binding persistence;
- Setup/Recovery AP password policy and persistence.

It deliberately does **not** own camera, DMX, laser, relay, GPIO, Cue or show-output authority.

## Device adapter

Each firmware supplies a `FoundationDeviceDescriptor` with its hostname prefix,
platform, architecture, firmware version and advertised capabilities. Device-specific
capabilities are data supplied by the consumer; this component defines only the
generic maintenance capability
`device.maintenance.setup-ap-password`.

For Setup/Recovery Wi-Fi, call
`FoundationStore::EffectiveSetupAPPassword()`. With no local override it returns
the Foundation fallback `12345678`. SET stores an 8–63 byte WPA2 password;
RESET_DEFAULT erases only the override. The password must never be placed in
observations, command results or logs.

## Migration compatibility

Use the existing NVS namespace when migrating a device so trust and the managed AP
password survive unchanged:
- DMX Lighting: `stagecore`
- StageLaser: `stagecore`
- Camera C2/C3: `stagecore_camv2`

`DeviceIdentity` keeps the existing `stagecore_id` namespace and keys.

The initial source extraction is based on the qualified DMX/Camera identity code and
the byte-identical DMX/StageLaser/Camera Hub discovery + trusted-clock code. Consumer
migration is intentionally separate so an extraction PR cannot change live output
behavior.
