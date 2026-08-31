# F-021 — Device Profile Library & Guided Auto-Configuration

**Status:** Foundation slice in implementation  
**Feature ID:** F-021  
**Phase:** 2 — Appliance, discovery and extension operations

## Goal

Turn StageCore device setup from protocol knowledge into an understandable product workflow:

```text
device observed or chosen
  -> candidate profile(s)
  -> operator understands what the profile supports
  -> simple connection fields
  -> validate/test
  -> materialize normal StageCore target/configuration
  -> operator explicitly saves/assigns it
```

Arabic operator definition:

> **مكتبة تعريف الأجهزة:** توفّر StageCore ملفات تعريف مفهومة للأجهزة والبرامج، بحيث يختار المستخدم الجهاز أو الملف المناسب ويملي الحقول البسيطة المطلوبة بدل كتابة إعدادات البروتوكول وJSON يدوياً.

## Foundation rule

**A device profile is knowledge, not authority.**

A profile may describe discovery hints, fields, capabilities, actions, health checks and configuration templates. Merely matching a profile never pairs a device, grants trust, modifies a project, assigns a Machine Role, changes SHOW state or fires a cue.

F-004 remains responsible for secure discovery/identity/pairing. F-021 consumes observed identity/capability facts and turns them into guided configuration choices.

## Catalog schema v1

Each profile has:

- stable profile ID;
- semantic profile version;
- source trust label: `OFFICIAL`, `LOCAL`, or `COMMUNITY`;
- kind: `DEVICE`, `SOFTWARE`, or `SERVICE`;
- English and Arabic (`ar-IQ`) name and summary;
- optional discovery hints;
- typed connection fields;
- supported capabilities and user-facing actions;
- health-check definitions;
- tested protocol/version notes;
- tags;
- optional target materialization template.

The catalog itself has a versioned schema independent of profile versions.

## Localization contract

Every profile is user-facing knowledge and therefore must ship English and Arabic definitions in the same profile version. The validator rejects missing Arabic text for profile names/summaries, fields, capabilities, actions and health checks.

This foundation does not add an Operator Web panel yet, so it does not introduce a fake screen before the guided device workflow exists. The API already returns the bilingual definitions that the later UI will render.

## Discovery matching

Profiles may define weighted discovery hints using bounded matching modes:

- `EXACT`;
- `PREFIX`;
- `CONTAINS`.

No arbitrary regular-expression execution is required by the foundation.

Matching rules:

1. Discovery observations are untrusted input.
2. Attribute names are normalized.
3. Required hints must match or the profile is excluded.
4. Optional matches increase a deterministic score.
5. Equal top scores are **ambiguous**, not silently resolved.
6. A profile with no discovery hints is never automatically selected.

This is deliberately conservative. A generic/manual profile must not pretend that StageCore discovered a product it cannot identify reliably.

## Typed connection fields

Supported foundation field types:

- `STRING`;
- `INT`;
- `BOOL`;
- `SECRET`.

Supported string formats include text, host/IP, URL and path. Integer fields may define ranges.

Security rules:

- secret fields may never ship default secret values;
- unknown submitted fields are rejected;
- required fields are enforced;
- host/IP and URL formats are validated;
- materialization remains bounded JSON data, not executable scripts.

## Target materialization

Profiles may contain a typed target template. A field reference is represented structurally:

```json
{"$field":"port"}
```

rather than using text substitution such as `"{{port}}"`.

This preserves JSON types and rejects references to undeclared fields before the profile enters the catalog.

Materialization is a **preview/build step only**. It returns ordinary StageCore `logical_type + configuration` data; it does not create the Project Device Alias itself. The normal Project edit path, RBAC and F-012 SHOW configuration lock remain authoritative for the actual write.

## Initial official profile

The foundation ships one intentionally honest profile:

### `stagecore.generic.osc-udp` v1.0.0

- source: `OFFICIAL`;
- kind: `DEVICE`;
- capability: existing `osc.send`;
- fields: OSC receiver host/IP and UDP port;
- target logical type: `GENERIC`;
- output configuration matches the already-supported StageCore OSC target contract;
- discovery hints: none, therefore it is a manual fallback rather than an auto-detected product.

This gives the later F-002 UI a real no-code path for the existing OSC capability without inventing support for a named product that has not been tested.

## Operator API foundation

Authenticated routes:

```text
GET  /api/v1/device-profiles
GET  /api/v1/device-profiles/{profile_id}
POST /api/v1/device-profiles/match
POST /api/v1/device-profiles/{profile_id}/materialize
```

Policy:

- authenticated VIEWER may read the catalog/matches;
- materialization requires Project Edit permission because it prepares configuration intended for a later project write;
- these routes do not write project/runtime state;
- request bodies are bounded and unknown profile values are rejected.

## Source and trust model

The foundation defines source labels now so later F-015 Extension Library work does not need to redesign profiles:

- `OFFICIAL`: shipped/maintained as StageCore-owned profile content;
- `LOCAL`: authored/imported by the local operator/site;
- `COMMUNITY`: externally sourced and explicitly distinguishable from official content.

This slice only ships built-in official content. Package signing, updates/import/export and community profile installation remain F-015/F-019 work.

## SHOW and runtime safety

- profile listing/matching/materialization is non-runtime work;
- it never fires commands;
- it never changes a Runtime Snapshot;
- it never grants device trust;
- it never bypasses Project permissions;
- actual configuration writes remain blocked by F-012 during active SHOW as applicable;
- no profile matching/materialization work is added to the P0/P1 cue execution path.

## Software acceptance

Before merge:

- profile/catalog schema is versioned;
- duplicate/invalid IDs and versions are rejected;
- all user-facing profile text requires English + Arabic;
- secret defaults are rejected;
- target field bindings must reference declared fields;
- typed target materialization preserves JSON types;
- unknown/invalid connection values are rejected;
- profile matching is deterministic;
- required hints exclude candidates;
- equal top matches return ambiguity rather than guessing;
- catalog callers receive defensive copies;
- unauthenticated Operator API access is denied;
- VIEWER can read but cannot materialize;
- Core CI Test/Vet/Race/ARM64 gates pass.

## Physical acceptance

No separate Raspberry Pi gate is required for the in-memory catalog/model itself. When the guided device UI and real named device profiles are added, physical acceptance belongs to those concrete profiles and to the combined Stage LAN qualification issue.

For the generic OSC profile, the existing real OSC transport qualification remains the evidence for the underlying `osc.send` capability; a future guided UI acceptance should verify that profile materialization produces the same target behavior without raw JSON entry.

## Deliberately deferred

- named vendor/product profile packs until each is genuinely tested;
- automatic profile installation/update through F-015;
- locally authored profile editor/import/export;
- profile package signing/integrity distribution;
- guided Operator Web device chooser/editor;
- executing profile health-check definitions;
- default routing/preset application;
- Android/tablet-specific profiles (F-003);
- Stage Network Cockpit presentation (F-022);
- device profile portability inside the Show Capsule (F-019).
