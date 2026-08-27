# 03 — Content Identity, Manifests & Integrity

## Content Identity

StageCore v0.1 uses SHA-256 as the default file checksum algorithm for managed content and package verification. The data model retains the algorithm field so future algorithms can coexist.

Canonical identity example:

```text
sha256:7f83b1657ff1fc53...
```

Filename, path and modification time are metadata, not content identity.

## Minimum Object Metadata

A verified Vault object records at least:

- content identity/checksum algorithm + checksum;
- size in bytes;
- created/imported timestamp;
- media/package type when known;
- logical asset/package references;
- availability state;
- last integrity verification timestamp when applicable.

## Manifests

Runtime/role/media/package manifests contain stable IDs and expected content identity. A Required Media entry should include at least:

- `media_asset_id`;
- `content_version_id`;
- checksum algorithm/checksum;
- size bytes;
- required/optional flag;
- role/target requirement where applicable.

Software/plugin manifests additionally include version/platform/API compatibility metadata.

## Atomic Commit Rule

Large incoming content is written to a temporary/staging file on the target filesystem, verified, then promoted to its committed location. Partial temporary files must not use the final committed object name/state.

Database metadata should be committed only when the object commit can be represented consistently. Recovery after crash must be able to distinguish committed objects from orphan staging files.

## Integrity Checks

- checksum is verified after import/upload;
- checksum is verified after Companion sync before READY;
- software/plugin package checksum is verified before offering/installing as trusted package;
- backup verification checks both manifest and copied object integrity;
- integrity failure changes availability to an explicit non-ready/error state.

## No Silent Repair

If a stored object checksum does not match its manifest, StageCore reports corruption/mismatch and requires restore/re-sync/re-import. It does not silently rewrite expected checksum metadata to match damaged bytes.