# F-001 Arabic UI and RTL Foundation v1

This slice establishes the first Arabic-first presentation layer for the embedded StageCore Operator UI.

## Included

- Arabic (`ar-IQ`) is the default presentation locale.
- The Operator shell uses RTL by default and can switch locally to English.
- The locale choice persists in browser-local storage.
- Static and dynamically rendered Operator copy is localized through one presentation layer.
- Confirmations, prompts, known operator errors and date/time formatting participate in localization.
- Technical identifiers, UUIDs, hashes, fingerprints and JSON remain LTR.
- No remote translation service or WAN dependency is introduced.
- Existing authenticated APIs, RBAC, CSRF, Preflight, SHOW lock and runtime semantics remain authoritative and unchanged.

## Feature localization policy

This slice also introduces the Feature Localization Contract v1. Future user-facing features must ship their English and Arabic definitions, action labels and stable localization keys in the same change. Core CI enforces the machine-readable manifest and prevents new features from selecting the legacy source-text compatibility mode.

## Deliberately incomplete

This is a localization foundation, not the final wording pass for every future StageCore capability. Existing pre-contract surfaces use a compatibility translation layer and can be migrated gradually to stable keys. Future features must use keyed localization from their first Operator surface.
