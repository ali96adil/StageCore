# F-021 — Guided Operator Device Workflow

**Status:** Phase 2 guided workflow implementation  
**Feature ID:** F-021  
**Depends on:** F-002 guided Operator shell, F-021 Device Profile Catalog v1, normal Project configuration API, F-012 SHOW configuration lock

## Goal

Close the Phase 2 no-code device-configuration gap by turning the existing Device Profile Catalog into the default guided target path in Operator Setup.

The common workflow is now:

```text
open Setup
  -> choose a Device Profile
  -> enter a Project device name
  -> fill the profile-defined connection fields
  -> Review & validate
  -> StageCore materializes the target through F-021
  -> operator explicitly adds the target to the Project
```

The operator does not need to write target JSON for this path.

## Authority boundary

The browser does not reproduce profile templates and does not become a second configuration authority.

It reads:

```text
GET /api/v1/device-profiles
```

and review uses:

```text
POST /api/v1/device-profiles/{profile_id}/materialize
```

Only the returned materialized target may be saved. The final mutation remains the existing Project API:

```text
POST /api/v1/projects/{project_id}/targets
```

Therefore Project RBAC, Draft/revision rules, F-012 SHOW configuration locking, persistence and audit behavior remain authoritative.

## Review before save

`Review & validate` is mandatory before the guided Save button is enabled.

Review validates typed profile values and asks the F-021 catalog to materialize the target. It is deliberately **not** presented as a live reachability or device-command test because executable profile health checks are not implemented yet.

Any change to profile, logical name or connection field invalidates the reviewed result and disables Save until review is repeated.

This prevents stale reviewed configuration from being saved after the operator edits fields.

## Dynamic fields

The guided form is generated from `connection_fields` rather than being hard-coded to OSC:

- `STRING` -> text field;
- `INT` -> bounded number field;
- `BOOL` -> checkbox;
- `SECRET` -> password field;
- required/default/min/max metadata is respected;
- bilingual labels/help come from the profile.

The initial official `stagecore.generic.osc-udp` profile therefore renders address and port fields today, while future validated profiles can use the same UI without adding a new hard-coded form.

## Profile knowledge shown to the operator

The selected profile exposes its localized name and summary, advertised capabilities and tested protocol/version notes before save. This keeps the profile understandable as product knowledge rather than treating a match as hidden authority.

## F-002 composition

F-021 composes after the existing F-002 guided shell. The compiled `/guided-ux.js` response intentionally contains the F-002 asset first and the F-021 asset second, so F-021 enhances the already-established Setup page rather than creating a competing page/navigation model.

Once profiles are available, the profile workflow becomes the primary target form and the older manual/raw target form moves under an explicit **Advanced / manual target setup** disclosure.

## Localization

`static/device-profiles.js` is an F-021-owned keyed Operator asset in the feature localization manifest. Workflow copy is owned in English and `ar-IQ`; profile-specific names, summaries, field labels/help and capability names come from the bilingual F-021 catalog itself.

## Failure behavior

If the Device Profile API is unavailable, Setup remains usable and shows a warning while leaving the advanced/manual target path available. F-021 does not make the whole Project configuration page fail merely because the profile catalog cannot be loaded.

If materialization rejects values, no Project mutation occurs and Save remains disabled.

## Safety boundaries

The guided workflow does not:

- pair or trust a discovered device;
- grant Machine Roles;
- execute a profile health check;
- send OSC or another live command during review;
- publish a revision;
- mutate Runtime or SHOW state;
- bypass the normal Project target API;
- persist secrets in browser-local profile state.

F-004 remains the discovery/pairing authority. Runtime execution remains downstream of normal validated/published StageCore configuration.

## Software acceptance

Automated acceptance must prove:

- the compiled Hub ships F-021 after F-002 in the guided bundle;
- the workflow reads the catalog instead of hard-coding an OSC target template;
- fields are generated from profile metadata;
- materialization is required before Save;
- changing reviewed values invalidates Save;
- final writes use the existing Project target endpoint and the materialized `logical_type` + `configuration`;
- F-021 has keyed English + `ar-IQ` ownership;
- no Runtime, Publish, setup-code, Security or Secret Store path is introduced;
- Core CI Test/Vet/Race and Linux ARM64 CGo-free builds pass.

## Physical acceptance

Physical acceptance remains part of the cumulative Phase 2 Raspberry Pi/Stage LAN qualification. The final gate must verify in a real browser that the Generic OSC profile can create a working target without raw JSON and that the resulting target behaves identically to the already-qualified OSC target contract.
