## Scope

Describe the bounded change and the Feature ID(s) it implements or extends.

## Verification

> These checkboxes intentionally remain unchecked in the repository template. Check each applicable item in the individual pull request only after its evidence is available.

- [ ] Tests cover the behavior changed by this PR.
- [ ] Runtime/security invariants remain unchanged unless explicitly in scope.
- [ ] No unrelated dependency or architecture changes are included.

## User-facing feature contract

Complete these items when this PR adds or expands an Operator-visible feature. Mark them N/A only for genuinely backend/internal-only work.

- [ ] Feature Localization Manifest updated for the affected Feature ID.
- [ ] English user-facing name and definition are provided.
- [ ] Arabic (`ar-IQ`) user-facing name and definition are provided.
- [ ] User-facing actions are declared in English and Arabic.
- [ ] New Operator UI uses stable localization keys, not English-only hard-coded copy.
- [ ] Every declared UI key has an Arabic translation.
- [ ] RTL behavior and LTR technical identifiers were considered.
- [ ] Basic/no-code workflow remains available for common supported tasks; expert detail stays progressively disclosed.

## Deliberately incomplete

List anything intentionally deferred so this PR does not imply a capability that is not implemented.
