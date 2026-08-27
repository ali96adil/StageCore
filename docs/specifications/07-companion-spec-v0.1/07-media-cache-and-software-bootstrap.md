# 07 — Media Cache & Software Bootstrap

## Media Principle

The Hub/Vault may hold project master files and manifests, but live playback should use local storage on the assigned Companion when practical. StageCore does not stream heavy show media from the Hub as the normal playback path.

## Required Media Per Role

Each Machine Role can declare a Required Media Set. Before READY, the Companion compares:

- asset/content ID;
- checksum;
- expected size/version;
- local path/cache state.

Missing or mismatched required media blocks readiness when marked required.

## Sync

```text
Hub publishes required media manifest
 -> Companion compares local cache
 -> requests missing/mismatched content
 -> downloads to temp location
 -> verifies checksum
 -> atomically promotes local copy
 -> reports media READY
```

Exact storage layout, retention and Vault APIs are defined in 08 — Storage & Vault Specification.

## Software Bootstrap from Hub

The StageCore Web interface should expose a local Downloads/Setup surface so a new machine can obtain a compatible Companion/app package without searching GitHub or requiring Internet.

Example:

```text
stagecore.local
 -> Downloads
 -> StageCore for macOS
 -> compatible build
 -> install
 -> discover/pair Hub
```

Hub software repository metadata includes:

- product/package ID;
- version;
- platform;
- CPU architecture;
- compatible Hub/API range;
- checksum;
- signing/notarization status where applicable.

The macOS release target is code-signed and notarized. Prototype/dev builds may use a clearly marked development workflow.

## SHOW Mode Traffic Rule

Large media sync, software downloads, backup and other bulk transfers must not compete with P0/P1 show-control traffic. During SHOW mode the Hub may pause, reject, or heavily throttle nonessential bulk transfers.

Software install/update is never automatic during SHOW mode.