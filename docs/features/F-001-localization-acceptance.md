# F-001 Localization Acceptance

F-001 foundation is acceptable when the following remain true:

- Arabic is the default Operator locale.
- The document uses RTL in Arabic and can switch to English without changing runtime behavior.
- The language choice persists locally.
- Operator confirmations, prompts, state messages and known error codes are localized.
- Technical identifiers remain readable LTR inside RTL layouts.
- Operator localization assets are embedded into the Hub binary and served locally.
- No WAN dependency is required for localization.
- The Feature Localization Contract test passes.
- New user-facing Feature IDs cannot choose the legacy source-text localization mode.
- Every Operator JS/CSS asset loaded by the shell has a declared feature localization owner.
- Existing runtime, security and SHOW safety semantics are unchanged.
