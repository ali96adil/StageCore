# F-016 — Cross-platform Appearance & Theme System — Foundation

## Status

Foundation slice for the confirmed F-016 feature.

## User goal

StageCore operators should be able to choose a comfortable, readable appearance without changing show behavior. The same semantic visual roles should be reusable by later Web, macOS, Android and display clients even when each platform renders them natively.

## Foundation contract

This slice establishes a semantic appearance vocabulary before additional interfaces hard-code colors.

### Semantic tokens

Operator presentation uses semantic roles rather than feature-specific color literals for:

- canvas, surface, raised surface and control backgrounds;
- primary, secondary and control text;
- default/strong/control borders;
- accent, stronger accent, accent-soft and focus/selection state;
- success/ready, warning and danger/blocker foreground/background/border roles;
- informational messages;
- cue-focus surfaces;
- overlays/backdrops and elevated shadows.

Existing legacy CSS variables remain compatibility aliases while older surfaces migrate. New F-016-owned styling uses the semantic `--sc-*` tokens.

### Built-in modes

The Web Operator provides:

- `System` / automatic mode;
- `Light` mode;
- `Dark` mode.

System mode follows `prefers-color-scheme` and reacts if the device setting changes while the Operator is open.

### Accent presets

The foundation provides four local accent presets:

- Blue;
- Teal;
- Violet;
- Amber.

Accent presets affect identity, focus, selection and primary-action presentation only. They do **not** remap success, warning or blocker/danger meaning.

### Scope and persistence

For this foundation, appearance preference is local to the current browser/device and stored in browser-local storage. It is intentionally separate from Project, Session, Runtime Snapshot and SHOW state.

No database migration is introduced.

### Accessibility and theatre behavior

- Light and Dark palettes retain distinct success/warning/danger semantics.
- Focus has an explicit semantic ring.
- Technical identifiers continue to follow F-001 directionality rules.
- Reduced-motion preference is respected by the theme layer.
- Dark mode remains suitable for low-light theatre operation, while System and Light remain available for daylight/setup work.

## Cross-platform model

The Web implementation is the first concrete client, but the token names define semantic intent rather than Web-specific widget styling. Future native clients should map the same concepts to native platform colors and controls instead of copying CSS values literally.

A later cross-platform theme manifest may persist/share user-created presets, but it must preserve these semantic roles.

## Localization contract

F-016 is a post-contract feature and therefore uses keyed bilingual presentation. Its user-facing name, description, actions and appearance-control copy are present in English and Arabic (`ar-IQ`) in the same implementation slice.

Feature-owned keyed dictionaries are allowed by the localization contract as long as the declared key is owned by the feature asset and contains an Arabic value. This lets a future feature ship its own UI definition and translations without bypassing the localization gate.

## Safety invariants

Theme changes are presentation-only. They must never:

- change Cue or Action semantics;
- modify routing or published Runtime Snapshots;
- alter permissions or security policy;
- change Preflight results;
- alter SHOW configuration locking;
- send runtime commands;
- suppress or recolor warning/blocker meaning into an ambiguous accent state.

## Deferred F-016 work

The following confirmed backlog capabilities remain intentionally deferred beyond the foundation slice:

- saved user-created theme presets;
- export/import and backup/restore of themes;
- account/site/show synchronization;
- separate synchronized Stage Display themes;
- Theme Packs distributed through the Extension Library;
- native macOS/Android implementation after those clients reach the appropriate integration phase.

These should build on the semantic token contract rather than redefining appearance state.

## Acceptance

1. Operator exposes System, Light and Dark appearance modes.
2. Operator exposes Blue, Teal, Violet and Amber accent presets.
3. Preferences survive local page reloads.
4. System mode tracks the device color-scheme preference.
5. Both palettes define semantic status tokens independent of accent selection.
6. Appearance assets are embedded and served locally by the Hub.
7. F-016 satisfies the Feature Localization Contract with English and Arabic definitions/actions/keyed UI copy.
8. No runtime, database, RBAC, Cue, Action, Snapshot, Preflight or SHOW-lock semantics change.
9. Core CI remains green, including race and ARM64 build gates.
