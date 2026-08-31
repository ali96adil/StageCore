# F-001 — Arabic UI and RTL Foundation

Status: In progress — Arabic-first Operator localization foundation implemented; contract enforcement added for future user-facing features.

## Product intent

StageCore uses one Operator UX and one set of runtime/domain contracts. Arabic and English are presentation locales over the same product behavior; there is no separate Arabic workflow or forked Arabic runtime.

Arabic (`ar-IQ`) is the default Operator presentation locale. English remains selectable locally and immediately. Runtime identifiers, API contracts, event names, capability keys, stored values, JSON and engineering documentation remain English and language-neutral.

## Foundation behavior

The Operator localization layer provides:

- Arabic-first presentation with `dir="rtl"` and `lang="ar-IQ"`;
- persistent local Arabic/English selection with no WAN dependency;
- RTL layout support using logical CSS properties rather than duplicated layouts;
- localization of static and dynamically rendered Operator content;
- localization of operator confirmations, prompts, state messages and known error codes;
- `ar-IQ` date/time formatting;
- explicit LTR treatment for technical identifiers, UUIDs, hashes, fingerprints and JSON;
- compatibility translation for existing Operator surfaces while newer surfaces migrate to stable localization keys.

## Feature Localization Contract

From this foundation forward, every new user-facing StageCore feature must provide localization metadata as part of the feature itself.

A feature is not considered Operator-ready when it only exposes backend capability or English UI. A new user-facing feature must declare:

1. a stable Feature ID;
2. the Operator assets it owns;
3. an English user-facing name and short definition;
4. an Arabic (`ar-IQ`) user-facing name and short definition;
5. the user-facing actions the feature provides, with English and Arabic labels;
6. stable UI localization keys used by its Operator surface;
7. Arabic translations for every declared UI key.

New features must use keyed localization. The pre-existing Operator surface may remain temporarily on the compatibility source-text layer while it is progressively migrated.

The machine-readable contract is stored at:

`internal/operatorweb/feature_localization_manifest.json`

## CI enforcement

`TestFeatureLocalizationContract` enforces the contract during normal Go CI.

The gate verifies that:

- required locales are `en` and `ar-IQ`;
- Feature IDs are unique and valid;
- every feature has an English and Arabic definition;
- every declared user action has English and Arabic labels;
- Arabic values contain Arabic text rather than an untranslated placeholder;
- every declared Operator asset exists and has one owner;
- every local JS/CSS asset loaded by the Operator shell has a localization owner;
- future features cannot opt into the legacy source-text localization mode;
- keyed features declare UI keys;
- declared keys are referenced by the owning feature assets;
- every declared key has an Arabic dictionary entry.

This means adding a new dedicated Operator feature asset without declaring its localization ownership fails CI.

## Pull request rule

Any PR that adds or expands a user-facing feature must update the Feature Localization Contract in the same PR. A feature may be merged without a new Operator surface only when it is genuinely backend/internal-only; once it becomes user-facing, the contract applies.

## What the contract enables later

The same feature metadata can become the source for future product surfaces such as:

- contextual “What does this do?” help;
- a Feature Catalog;
- setup wizards;
- capability-aware onboarding;
- localized plugin/module descriptions;
- availability and compatibility explanations;
- operator profiles that expose only relevant capabilities.

These future surfaces must consume the declared metadata rather than invent a second description source.

## Safety and runtime invariants

Localization never changes:

- RBAC;
- CSRF/session security;
- SHOW configuration lock semantics;
- Preflight authority;
- Runtime Snapshot immutability;
- Cue/Action execution behavior;
- database identifiers or event contracts.

Arabic is a presentation concern. Safety and runtime truth remain authoritative in the existing server/domain layers.
