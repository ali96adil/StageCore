# F-008 — First-run Setup Wizard

**Status:** Foundation slice  
**Feature ID:** F-008  
**Phase:** 2 — Installation, diagnostics, discovery, update, and extension operations

## Goal

After a fresh StageCore Hub has been securely claimed by its first OWNER, the local Operator UI should guide that OWNER through the minimum product setup needed to reach the normal guided Project workspace without requiring database edits, configuration files, source code, or a second administration application.

F-008 is an onboarding layer over existing authoritative product APIs. It does not create a parallel security, Project, runtime, or deployment state machine.

## Secure entry boundary

The existing M6 first-OWNER bootstrap remains authoritative:

1. a fresh Hub is `UNCLAIMED`;
2. a one-time setup code is generated locally on the Hub with `stagecore-setup setup-code`;
3. the browser submits that code to the existing bootstrap endpoint;
4. the first OWNER is created and authenticated;
5. the Hub becomes `CLAIMED`.

F-008 deliberately does **not** expose remote/browser setup-code generation. The local setup code remains the physical/local trust proof for claiming a fresh Hub.

## Wizard eligibility

The wizard opens only when the normal authenticated Operator state proves all of the following:

- current user role is `OWNER`;
- Hub bootstrap state is `CLAIMED`;
- the authoritative Project list has been loaded;
- the Hub currently has zero Projects.

There is no durable browser `setup_complete` flag.

The existence of the first Project is the server-side product fact that naturally ends first-run onboarding. Closing the wizard only suppresses it for the current page/session; a later fresh visit with zero Projects can offer it again.

## Foundation flow

### Step 1 — Hub identity

The OWNER sees the Hub display name and fingerprint already supplied by the authentication status API, plus an explicit local-only explanation.

This step does not establish a second trust decision or change identity state. It makes the identity already used by StageCore visible during onboarding.

### Step 2 — local operator preferences

The OWNER can choose:

- Arabic or English;
- System, Light, or Dark appearance;
- the existing StageCore accent choices.

These choices reuse F-001/F-016 browser-local preference contracts. They are presentation preferences only and never change Project, SHOW, runtime, Preflight, security, or device state.

### Step 3 — first Project

The wizard creates the first Project through the existing authenticated endpoint:

```text
POST /api/v1/projects
```

with the same normal `name` and `description` shape used by the standard Projects page.

The server therefore creates the ordinary editable Draft using the existing Project rules, RBAC, CSRF/session enforcement, persistence, and audit behavior. F-008 does not duplicate Project creation logic.

After creation, the browser reloads if needed so the selected locale is applied consistently to the whole Operator shell, reopens the newly created Project, and hands off to the existing F-002 guided `Setup` page.

Device/target configuration is intentionally not reimplemented inside F-008. F-002 already owns the visual/common setup path. Likewise, Preflight remains the authoritative readiness gate rather than a wizard-owned readiness flag.

## User agency

The wizard can be closed or skipped for now.

Skipping:

- does not mutate Hub state;
- does not mark onboarding permanently complete;
- does not prevent the OWNER from using the normal Projects page;
- does not hide safety warnings or runtime state.

The standard Operator interface remains available and authoritative.

## Localization and RTL

F-008 is a keyed-localization feature under the F-001 localization contract.

Its dedicated assets own English and Arabic (`ar-IQ`) copy in the same change. The dialog switches its own `lang`/`dir` while language is being selected, and the final browser locale remains the existing F-001 preference.

CSS uses logical layout properties and a bounded mobile breakpoint so Arabic RTL, English LTR, desktop, and narrow/tablet widths are all first-class acceptance targets.

## Safety boundaries

The F-008 frontend must not call or simulate:

- setup-code generation;
- user/security administration;
- Secret Store administration;
- Publish;
- Runtime GO/STOP/Jump;
- REHEARSAL or SHOW entry;
- Preflight override;
- extension activation;
- deployment/update operations.

The only Hub mutation in the foundation wizard is normal first-Project creation through the existing authenticated Project API.

## Persistence boundaries

Browser-local persistence is limited to existing presentation preferences:

- F-001 locale;
- F-016 appearance/accent.

A short-lived `sessionStorage` Project ID may be used only to reopen the just-created Project after a locale reload. It is not authoritative setup state and is deleted once consumed.

## Software acceptance

Foundation acceptance requires automated tests proving:

- the Operator shell loads dedicated F-008 JS/CSS assets;
- F-008 is registered as a keyed feature in the localization manifest;
- declared F-008 UI keys have Arabic copy;
- eligibility requires OWNER + CLAIMED Hub + zero authoritative Projects;
- dismissal is page/session-only rather than a durable completion flag;
- Project creation uses `POST /api/v1/projects`;
- post-create handoff goes to the existing guided `configuration`/Setup page;
- the wizard does not call runtime, publish, setup-code, security, or secret-administration endpoints;
- CSS includes logical RTL-safe layout and narrow-width behavior;
- normal Core CI Test/Vet/Race and Linux ARM64 CGo-free product builds remain green.

## Physical/UI acceptance

Later cumulative Raspberry Pi qualification should verify in Safari against the exact current `main` build:

1. fresh/zero-Project OWNER sees the wizard after secure claim/login;
2. Hub name/fingerprint are readable;
3. Arabic RTL and English LTR both render without overlap;
4. appearance choice applies without changing operational state;
5. first Project creation succeeds without scripts/database editing;
6. reload returns to the created Project and opens guided Setup;
7. narrow/tablet width remains usable;
8. an existing Hub with one or more Projects does not reopen first-run onboarding.

## Deliberately deferred

- remote setup-code generation;
- automatic device enrollment inside the first-run dialog;
- automatic Cue creation/publish;
- automatic SHOW entry;
- permanent user-specific onboarding progress stored server-side;
- appliance/mobile native onboarding surfaces;
- any setup step that weakens the existing local-first security boundary.
