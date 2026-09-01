# F-015 — Offline Bundle Import and Trusted Official Catalog v1

## Scope

This slice gives the Extension Manager a one-step offline package intake path without adding any WAN dependency and without weakening the existing `OFFICIAL` provenance boundary.

Two paths are intentionally different:

1. **Operator offline import** accepts a `.scext` file uploaded from the local browser and may register only `LOCAL` or `COMMUNITY` extensions.
2. **Trusted official catalog sync** reads `.scext` files only from the StageCore-owned Hub path `/opt/stagecore/extensions/catalog` and may register only manifests whose source is `OFFICIAL`.

The browser never supplies or overrides the trusted catalog filesystem path.

## Bundle format

Contract identifier:

`stagecore-extension-bundle-v1`

A `.scext` file is an uncompressed tar stream with exactly three regular-file entries in this order:

1. `bundle.json`
2. `manifest.json`
3. `payload.bin`

Directories, symlinks, hardlinks, alternate names, reordered entries and extra entries are rejected.

### `bundle.json`

The metadata document is strict JSON with unknown fields rejected. It binds the immutable payload to:

- product / Extension ID;
- semantic version;
- platform and architecture;
- exact Hub API compatibility range;
- original base filename;
- signing/notarization/release metadata;
- payload SHA-256;
- payload size.

The product identity, version, API range, platform and architecture must agree with `manifest.json` before any package is registered.

### Size limits

- `bundle.json`: 64 KiB maximum;
- `manifest.json`: 256 KiB maximum;
- `payload.bin`: 512 MiB maximum;
- the Operator API also applies a bounded whole-request limit above those component limits to account for tar framing.

## Staging and integrity

The importer does not stream an unverified browser upload directly into active package metadata.

It first:

1. parses the tar structure and strict metadata;
2. parses and validates the Extension Manifest;
3. writes only `payload.bin` to a private `0600` temporary regular file;
4. computes SHA-256 while staging;
5. verifies exact size and hash;
6. proves the archive contains no fourth entry;
7. rewinds the verified staging file;
8. imports the verified bytes into the immutable Vault;
9. registers Software Package metadata only when the Vault object's size/hash match the expected values;
10. registers the Extension Package through the existing Library trust/compatibility checks.

Temporary staging files are removed on both success and failure.

A content mismatch therefore cannot create a Software Package or Extension Package database row. The content-addressed Vault may retain an inert unreferenced object if a lower-level import reaches the Vault before a later metadata failure; that object has no extension/runtime authority.

## Operator-upload trust boundary

An uploaded file is untrusted input even when its `bundle.json` claims `SIGNED`, `RELEASE`, notarized, or other production metadata.

For the public Operator import path StageCore deliberately records the Software Package as:

- signing: `UNKNOWN`;
- notarization: `UNKNOWN`;
- release channel: `development`.

The Extension Manifest itself must declare `LOCAL` or `COMMUNITY`.

A browser upload declaring `OFFICIAL` is rejected. There is no UI switch, request field or API parameter that can elevate it.

This prevents a locally created bundle from displaying or inheriting official production trust simply by self-asserting metadata.

## Trusted official catalog

The trusted catalog is a fixed StageCore-owned filesystem boundary:

`/opt/stagecore/extensions/catalog`

Catalog sync:

- ignores non-`.scext` files;
- sorts bundle names deterministically;
- rejects `.scext` symlinks and other non-regular files;
- requires each imported manifest to declare `OFFICIAL`;
- retains the bundle's signing/notarization/release metadata;
- still passes the package through Software Repository compatibility checks and the existing `RegisterOfficial` production-readiness requirement.

A missing catalog directory is a valid empty catalog, so a Hub with no bundled official extensions still starts and operates normally.

The trust claim here is about the privileged StageCore-owned catalog path and the StageCore installation/release chain. This bundle format by itself is not a substitute for publisher-signature authentication. F-005/F-014 integration must provision official catalog content through the privileged installer/offline-release path rather than through Operator upload.

## Idempotence

Before creating a new package record, the importer compares already registered packages for the same Extension ID against:

- semantic version;
- source;
- canonical manifest SHA-256;
- payload SHA-256 and size;
- platform and architecture.

An exact repeat returns the existing Package ID with `already_registered: true` instead of creating another package identity. Trusted catalog sync is therefore repeatable.

## SHOW safety

Both public import and trusted catalog sync are configuration mutations.

They are rejected while an authoritative SHOW Session is active. The Library re-checks the same SHOW gate again when the final Extension Package registration occurs, so an import cannot bypass the existing F-012 boundary.

## Operator API

### Upload a local/community bundle

`POST /api/v1/extensions/import-bundle`

- body: raw `.scext` bytes;
- permission: `plugin.manage`;
- normal browser-session CSRF protection applies;
- successful response returns the registered package, verified payload digest/size, trusted flag and idempotence flag.

### Sync trusted official catalog

`POST /api/v1/extensions/catalog/sync`

- no path/body authority is accepted from the browser;
- permission: `plugin.manage`;
- normal browser-session CSRF protection applies;
- response lists the trusted bundles checked/imported.

Audit events:

- `extension.bundle.import`;
- `extension.catalog.sync`.

Audit metadata contains identifiers, source, hash/size and outcome only; bundle payload bytes are never written to the audit stream.

## Operator UI

The bilingual Extension Manager exposes a guided **Add extension packages** section for `OWNER` / `TECHNICIAN` users:

- choose a local `.scext` bundle and import it;
- sync the fixed official catalog;
- explain that browser uploads remain LOCAL/COMMUNITY;
- explain that OFFICIAL provenance comes only from the trusted StageCore catalog;
- surface SHOW lock, source, integrity, invalid-format, size and catalog-availability errors in Arabic and English;
- refresh the real Library model after a successful operation.

The browser does not parse manifests, calculate trust, choose the catalog root or register package metadata itself.

## Acceptance

This slice is accepted when Core CI proves:

- expected Software Package hash/size mismatch fails before package metadata registration;
- strict bundle order/type/size/hash rules;
- tampered payload rejection before Extension Package registration;
- public `OFFICIAL` rejection and public trust downgrade;
- idempotent public import;
- trusted OFFICIAL catalog import and idempotent re-sync;
- trusted catalog symlink rejection;
- Operator `plugin.manage` and CSRF boundaries;
- Operator UI calls the server upload/sync APIs and does not manufacture trust/path authority;
- Test, Vet, Race and Linux ARM64 CGo-free product builds remain green.
