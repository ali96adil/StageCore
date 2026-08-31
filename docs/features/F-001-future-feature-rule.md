# Future Feature UI Rule

From F-001 onward, StageCore treats localization as part of feature completeness.

A backend capability may land without Operator localization only while it remains genuinely internal and exposes no user-facing control or explanation. The moment the capability becomes visible or operable from the StageCore Operator UI, the same change must provide its Feature Localization Manifest entry, English and Arabic definitions, English and Arabic action labels, stable UI keys and Arabic translations.

A user-facing feature that does not satisfy this rule is incomplete and must not be merged as Operator-ready.
