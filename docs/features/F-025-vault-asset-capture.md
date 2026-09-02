# F-025 — Verified Vault Asset Capture

**Status:** Phase 3 portability slice  
**Feature ID:** F-025  
**Depends on:** Execution Environment Manifest v1, revision persistence, F-012 SHOW configuration lock, immutable Vault/content hashes, Operator authentication

## Goal

An F-025 `CONTENT_BOUND` asset is useful for reproducibility only when StageCore can identify the exact bytes and, when the Operator chooses to capture them, prove that those bytes are present in the existing immutable Vault.

This slice adds that explicit capture path without creating a second blob store and without granting StageCore automatic access to workstation files.

## Operator workflow

The guided workflow is deliberately explicit:

1. declare the external execution environment and workstation locator as `REFERENCE_ONLY`;
2. create or enter a DRAFT revision;
3. choose the exact local file in the browser;
4. upload it through the authenticated Operator capture route;
5. StageCore streams the request through `Vault.ImportObject`;
6. Vault computes SHA-256, enforces the existing storage reserve, atomically promotes or deduplicates the immutable object, and registers authoritative object metadata;
7. Store verifies the registered Vault object and exact byte size;
8. only then does Store update the asset to `CONTENT_BOUND` and recompute the canonical execution-environment manifest identity.

No browser `FileReader`, base64 conversion, whole-file application buffer, shell command, automatic path scrape, or Companion filesystem pull is involved.

## Store authority

`Store.CaptureExecutionEnvironmentAsset` is the configuration authority for promotion.

It:

- requires the execution-environment manifest and asset key to exist;
- validates a SHA-256 object identity and non-negative exact size;
- checks the existing F-012 project SHOW lock before revision status so active SHOW returns `SHOW_CONFIGURATION_LOCKED`;
- requires a DRAFT revision when SHOW is not active;
- requires a matching `vault_objects` record and exact size;
- preserves all unrelated manifest state and the asset locator/name/kind;
- canonicalizes and re-hashes through `internal/executionenv`;
- uses the previous manifest hash as an optimistic update guard so a concurrent configuration change cannot be overwritten silently.

The operation never writes blob bytes itself.

## HTTP contract

The Operator API exposes:

```text
POST /api/v1/projects/{project_id}/revisions/{revision_id}/execution-environments/{execution_environment_id}/assets/{asset_key}/capture
GET  /api/v1/projects/{project_id}/revisions/{revision_id}/execution-environments/{execution_environment_id}/assets/{asset_key}/vault-status
```

The POST route requires `PermissionProjectEdit`, the normal authenticated browser session, same-origin policy, and CSRF validation. Its request body is the raw selected file and is streamed directly to Vault.

Known-invalid mutations are rejected before the body is imported. Store repeats SHOW/DRAFT checks after import so a state change racing the upload fails closed. If that final mutation is rejected, the immutable content-addressed object may remain safely deduplicated in Vault, but the API never claims the manifest capture succeeded.

The GET route requires `PermissionProjectRead` and reports whether the declared `CONTENT_BOUND` identity has a physically openable Vault object with matching authoritative size. This lets the Operator distinguish:

- `REFERENCE_ONLY`: bytes are not claimed;
- declared `CONTENT_BOUND` identity with no usable Vault object: reduced portability / missing Vault copy;
- verified `CONTENT_BOUND` identity with available Vault bytes.

## Storage safety

Capture reuses the existing Vault implementation:

- streaming SHA-256;
- staging directory;
- runtime-reserve admission checks;
- atomic no-overwrite promotion;
- content-addressed deduplication;
- immutable object metadata.

There is no F-025-specific storage tree, duplicate media repository, or alternate capacity policy.

## SHOW semantics

Capture is Project configuration mutation and is therefore prohibited while an authoritative SHOW Session locks that Project.

Read-only `vault-status` remains safe during SHOW.

## Boundaries

This slice does **not** add:

- automatic access to the path recorded in `locator`;
- remote Companion file transfer;
- application-specific project scraping;
- process launch/open authority;
- Show Capsule packaging or restore (F-019);
- migration of media-library objects into execution environments;
- a second health vocabulary.

Those capabilities, when needed, must continue through their existing StageCore authorities rather than bypassing F-025, F-012, Vault, or Companion trust boundaries.
