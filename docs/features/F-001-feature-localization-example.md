# Feature Localization Example

When a future user-facing feature is added, its manifest entry should follow this pattern:

```json
{
  "feature_id": "F-999",
  "localization_mode": "keyed",
  "owner_assets": ["static/example-feature.js"],
  "name": {
    "en": "Example Feature",
    "ar-IQ": "ميزة مثال"
  },
  "summary": {
    "en": "Short operator-facing explanation of what the feature provides.",
    "ar-IQ": "شرح مختصر للمستخدم يوضح ما الذي توفره الميزة."
  },
  "actions": [
    {
      "action_id": "example-action",
      "label": {
        "en": "Run Example Action",
        "ar-IQ": "تنفيذ إجراء المثال"
      }
    }
  ],
  "ui_keys": [
    "example.title",
    "example.action.run"
  ]
}
```

The owning UI asset must reference the declared keys and the Arabic localization dictionary must provide Arabic values for them. Core CI rejects missing definitions, missing Arabic action labels, missing assets, unowned Operator assets, missing keys and attempts by new features to use the legacy source-text compatibility mode.
