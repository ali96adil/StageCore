# Localization Ownership

Each user-facing Operator asset has one declared localization owner in the Feature Localization Manifest. This keeps responsibility explicit and prevents a new UI module from appearing in the shell without a Feature ID and bilingual definition.

Shared core assets that predate the contract are explicitly grandfathered by test code. That grandfather list is intentionally not data-driven, so new features cannot mark themselves legacy through the manifest alone.
