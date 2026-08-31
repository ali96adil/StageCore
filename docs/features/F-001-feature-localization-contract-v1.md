# F-001 Feature Localization Contract v1

This checkpoint makes localization a merge-time product contract for future StageCore Operator features.

## Rule

A new user-facing feature is not Operator-ready until the same change provides:

- a stable Feature ID;
- owned Operator assets;
- an English name and short definition;
- an Arabic (`ar-IQ`) name and short definition;
- English and Arabic labels for the user actions it exposes;
- stable UI localization keys;
- Arabic values for every declared key.

## Enforcement

The contract is represented by `internal/operatorweb/feature_localization_manifest.json` and enforced by `TestFeatureLocalizationContract` in normal Core CI.

Existing pre-contract surfaces are explicitly grandfathered. That grandfather list lives in test code rather than the manifest, so a future feature cannot opt itself into legacy source-text localization by changing data only.

All new user-facing Feature IDs must use keyed localization.

The test also verifies that every local JavaScript or CSS asset loaded by the Operator shell has a declared localization owner. Adding a new dedicated Operator asset without updating the feature manifest fails CI.

## Pull request workflow

`.github/pull_request_template.md` includes the same requirement as a visible author checklist. The checklist does not replace CI; it makes the requirement explicit before review.

## Future use

The localized feature definitions and action metadata are intentionally structured so StageCore can later use them for contextual help, onboarding, feature catalogs, plugins/modules, compatibility explanations and operator-profile surfaces without creating a second source of product copy.
