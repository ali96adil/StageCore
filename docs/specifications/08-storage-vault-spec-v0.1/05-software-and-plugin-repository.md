# 05 — Software & Plugin Repository

## Purpose

The Hub hosts a local Software Repository so a new stage machine can obtain the correct StageCore client/Companion package from the Web interface without depending on Internet or searching GitHub.

## Software Package Metadata

Each package records at least:

- package/product ID;
- version;
- platform (`macos`, future `windows`, `linux`, etc.);
- CPU architecture;
- minimum/maximum compatible StageCore API/Hub version range;
- file size;
- SHA-256 checksum;
- signing status;
- notarization status where applicable;
- release/build channel (`development | release`);
- release notes/reference.

## Web Bootstrap Surface

Reference UX:

```text
stagecore.local
 -> Downloads / Setup
 -> StageCore for macOS
 -> compatible build for this Hub
 -> download
 -> install
 -> launch
 -> discover/pair Hub
```

The UI should prefer compatible packages and clearly mark incompatible/development builds instead of letting the user guess.

## macOS Distribution

Normal field release builds should be code-signed and notarized. Development builds may use a clearly marked local development path, but the repository must not label an unsigned dev build as production-ready.

## Plugin Package Repository

The Hub can also retain Plugin/Add-on packages plus manifest/version/API compatibility/checksum metadata. Installing a package still follows the Plugin Contract; storing it in the repository does not grant activation or permissions automatically.

## Update Rules

- packages may be downloaded explicitly outside SHOW;
- no forced Companion/client/plugin update during SHOW;
- incompatible API version creates WARN/BLOCK according to role/plugin requirement;
- existing Published Runtime Snapshot is never rewritten merely because a newer package exists;
- package bytes are verified before install/activation workflows trust them.

## Repository Scope

The MVP requires local package hosting and compatibility metadata. A remote catalog, marketplace, licensing/billing and automatic Internet package fetching are later features.