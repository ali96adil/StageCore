# F-008 — First-run Setup Wizard — Software Acceptance Checkpoint

**Date:** 2026-09-02  
**Feature:** F-008  
**Status:** SOFTWARE COMPLETE — PHYSICAL PHASE 2 QUALIFICATION PENDING

## Scope closed by this checkpoint

F-008 provides the authenticated local first-run onboarding layer for a securely claimed StageCore Hub. It reuses existing authoritative product APIs and does not introduce a parallel setup, security, Project, Runtime, or deployment state machine.

The wizard is eligible only for an authenticated `OWNER` when the Hub bootstrap state is `CLAIMED` and the authoritative Project list is empty. It exposes the existing Hub identity/fingerprint, lets the operator choose browser-local language and appearance preferences, creates the first Project through `POST /api/v1/projects`, and hands off to the existing guided `configuration` / Setup workspace.

Dismissal remains page/session-only. There is no durable browser `setup_complete` truth flag; the existence of the first Project remains the server-side fact that naturally ends first-run onboarding.

## Security and safety boundary

F-008 does not generate setup codes remotely and does not call or simulate Runtime control, Publish, SHOW entry, Preflight override, security administration, Secret Store administration, extension activation, or deployment/update operations.

The existing first-OWNER bootstrap remains the trust boundary for claiming a fresh Hub. F-008 starts only after authenticated entry and delegates its only Hub mutation to the normal Project API with the existing RBAC, session/CSRF, persistence, and audit rules.

## Localization and presentation boundary

F-008 is registered as a keyed feature under the F-001 localization contract. Its owner assets provide both English and `ar-IQ` copy, and its CSS uses logical RTL-safe layout plus a bounded narrow-width/mobile layout.

Language, appearance, and accent choices remain browser-local presentation preferences under the existing F-001/F-016 contracts. They do not mutate Project or Runtime behavior.

## Automated acceptance evidence

The current automated contracts prove:

- the Operator shell references the dedicated `first-run.js` and `first-run.css` assets;
- the compiled Hub embeds and serves both assets over their same-origin HTTP routes;
- F-008 is registered in the feature-localization manifest with English and Iraqi Arabic ownership;
- eligibility requires `OWNER`, `CLAIMED`, and zero authoritative Projects;
- dismissal is session-local rather than a durable completion flag;
- first Project creation uses `POST /api/v1/projects`;
- the created Project is retained only as a short-lived resume target and the browser hands off to `configuration` / Setup;
- the wizard contains no Runtime, Publish, setup-code, Security, or Secret Store endpoint path;
- RTL-safe logical CSS and narrow-width layout markers remain present.

The normal Core CI Test/Vet/Race and Linux ARM64 CGo-free product build gates remain the merge requirement for this checkpoint.

## Remaining physical verification

Physical/UI verification is deliberately deferred to the cumulative Raspberry Pi ARM64 Phase 2 gate. That gate must use the exact final Phase 2 build and verify in a real browser that a securely claimed zero-Project OWNER sees the wizard, Hub identity is readable, Arabic RTL and English LTR render correctly, first Project creation succeeds without scripts or database edits, Setup handoff survives reload, narrow/tablet layout remains usable, and an existing Hub does not reopen onboarding.

No physical qualification claim is made by this checkpoint.
