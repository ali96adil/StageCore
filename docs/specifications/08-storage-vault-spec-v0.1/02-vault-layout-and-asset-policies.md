# 02 — Vault Layout & Asset Policies

## Managed Vault Model

The Vault stores immutable content objects plus mutable metadata/manifests that point to them. Projects should reference `media_asset_id` / content identity instead of relying on a user-visible filename as identity.

A practical content-addressed layout may use:

```text
vault/objects/sha256/ab/cd/<full-hash>
```

The exact layout is implementation detail; the invariant is that one verified content identity maps to immutable bytes.

## Asset Policies

StageCore supports the policies already defined in the Data Model:

### `REFERENCE_ONLY`

The file stays outside the managed Vault. StageCore stores reference metadata and can report availability. It cannot promise the referenced file will exist on another machine.

### `MANAGED`

StageCore imports the bytes into the Vault and becomes responsible for content verification and distribution.

### `ARCHIVE_REQUIRED`

Same as MANAGED for active use, plus archive/backup policy can require a verified protected copy before a Project reaches archival completion.

## Import Flow

```text
Select/import file
 -> create transfer/import job
 -> write into staging area
 -> calculate size + checksum
 -> validate metadata
 -> check whether identical object already exists
 -> atomically commit object
 -> create/update MediaContentVersion
 -> create MediaLocation(HUB)
 -> expose asset as AVAILABLE
```

A failed/interrupted import remains a failed/staged job and must never appear as a valid managed asset.

## Deduplication

If two logical MediaAssets use identical verified bytes, the implementation may reuse the same Vault object while keeping separate logical asset identities. Deleting one logical asset must not remove bytes still referenced by another asset/version.

## Deletion

Deletion is reference-aware:

- remove/unlink logical references first;
- published Snapshots and retained Sessions/archives may pin content versions;
- physical garbage collection is a separate safe operation;
- no automatic destructive garbage collection during SHOW mode.