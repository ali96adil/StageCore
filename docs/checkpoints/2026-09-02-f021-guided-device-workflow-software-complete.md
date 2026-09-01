# F-021 — Guided Device Workflow — Software Acceptance Checkpoint

**Date:** 2026-09-02  
**Feature:** F-021  
**Status:** SOFTWARE COMPLETE WHEN THIS CHECKPOINT'S EXACT HEAD PASSES CORE CI — PHYSICAL PHASE 2 QUALIFICATION PENDING

## Scope closed by this checkpoint

The Phase 2 F-021 foundation already provided a versioned bilingual Device Profile Catalog, deterministic discovery matching, typed connection fields, bounded target templates, safe materialization, and authenticated read/materialize APIs.

This checkpoint closes the deferred Operator workflow: Setup now consumes that catalog as a guided no-code target path rather than requiring the operator to understand the profile API or write target JSON.

## Guided path

The Operator workflow:

1. lists target-capable Device Profiles;
2. renders localized profile knowledge and typed connection fields;
3. requires `Review & validate` through the F-021 materialize endpoint;
4. invalidates the reviewed result when any relevant value changes;
5. enables explicit Save only for the still-current reviewed result;
6. saves the exact materialized target through the existing Project target API.

The original manual/raw target editor remains available under an explicit Advanced disclosure instead of being the primary common path.

## Authority and safety

The browser does not contain an OSC target template for the F-021 path. The profile catalog remains the source of configuration knowledge and the existing Project API remains the only write authority.

Review is configuration validation/materialization only; it does not claim live device reachability and sends no device command.

The workflow introduces no pairing/trust, Runtime, Publish, setup-code, Security, Secret Store or SHOW-state authority.

## Localization and packaging

F-021 owns `static/device-profiles.js` as a keyed Operator asset with English and `ar-IQ` workflow copy. Device-specific copy continues to come from the bilingual profile catalog.

The compiled Hub serves F-021 after the F-002 guided shell in the same `/guided-ux.js` response, which preserves deterministic enhancement order without creating a parallel Setup page.

## Automated acceptance evidence

The new contract tests require:

- catalog API consumption;
- profile-driven fields and capabilities;
- materialization before save;
- disabled Save before/current-review proof;
- normal Project target write using the materialized logical type and configuration;
- absence of a hard-coded OSC template in the F-021 asset;
- absence of Runtime/Publish/setup-code/Security/Secret Store endpoint paths;
- keyed bilingual F-021 copy;
- compiled-Hub delivery with F-002 preceding F-021.

Existing F-021 catalog/API tests remain authoritative for schema validation, localization completeness, typed materialization, deterministic matching, RBAC and defensive-copy behavior.

## Remaining physical verification

Physical verification is deliberately deferred to the cumulative Raspberry Pi ARM64 Phase 2 gate. That gate must create a Generic OSC UDP target through the real browser UI without raw JSON and prove that the resulting target drives the already-qualified real OSC transport path.

No physical qualification claim is made by this checkpoint.
