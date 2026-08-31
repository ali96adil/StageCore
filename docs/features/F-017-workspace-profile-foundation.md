# F-017 — Workspace Layouts & Operator Profiles — Foundation

## Status

Phase 1 state-model slice.

This document defines the first durable workspace/profile contract without claiming the full multi-window/docking feature described in the product backlog.

## User goal

An operator should be able to make StageCore feel appropriate for the job they are doing without changing the Project, Cue graph or live runtime state.

The common path should answer three simple questions:

1. Which workspace profile am I using?
2. Which workspaces should be visible and in what priority order?
3. Which workspace should StageCore open first or restore when I return to a Project?

The operator must be able to switch profiles quickly during normal work and SHOW without firing or mutating show-control state.

## Foundation principles

- Workspace state is **presentation state**, never runtime truth.
- Profile switching never fires a Cue, changes routing, changes permissions, publishes a Snapshot or writes Project configuration.
- Visibility is convenience, not authorization. Hiding a workspace does not grant or revoke access.
- Profiles use stable semantic workspace identifiers rather than DOM indexes or translated labels.
- Persisted data is versioned and sanitized on read so future clients can migrate it safely.
- Unknown future workspace identifiers are ignored rather than making the UI unusable.
- A valid safe fallback always remains available.
- Arabic/RTL and English presentation use the same profile model.
- The Web foundation is local to one browser/device. Later user/site/shared synchronization must reuse this model rather than inventing a parallel profile format.

## Workspace profile contract v1

A profile is represented conceptually as:

```json
{
  "profile_version": 1,
  "profile_id": "custom-...",
  "name": "My workspace",
  "base_preset": "stage-manager",
  "scope": "DEVICE_LOCAL",
  "visible_pages": ["dashboard", "runtime", "preflight"],
  "page_order": ["dashboard", "runtime", "preflight", "cues", "configuration", "sessions", "notes"],
  "default_page": "dashboard",
  "navigation_size": "normal",
  "updated_at": "..."
}
```

### Stable page identifiers

The v1 Web Operator model recognizes:

- `dashboard` — Project Home
- `configuration` — Setup
- `cues` — Cues
- `runtime` — Run
- `preflight` — Check
- `sessions` — History
- `notes` — Notes

`projects` and administrative `security` navigation are outside the Project workspace profile. RBAC remains authoritative for Security visibility.

### Navigation size

The v1 profile carries one presentation density hint:

- `compact`
- `normal`
- `wide`

This changes only the Project navigation presentation. It is deliberately not a generic window geometry system.

## Built-in presets

Built-in presets are immutable templates. Editing a built-in preset creates a custom profile instead of mutating the shipped definition.

The foundation ships:

- **Stage Manager** — balanced Home/Run/Check/Cues/History/Notes/Setup view.
- **Video** — prioritizes Run, Cues, Setup and Check.
- **Lighting** — prioritizes Run, Cues, Setup and Check.
- **Sound** — prioritizes Run, Cues, Setup and Check.
- **Rehearsal** — prioritizes Run, Cues, Notes and History.
- **Monitoring** — prioritizes Check, Home, Run and History.

The presets are role-oriented starting points, not permission roles and not hardware capability declarations.

## Device-local persistence

The Web foundation stores one versioned container in browser local storage.

It contains:

- active profile ID;
- custom profiles;
- last selected visible workspace per Project and profile.

Storage failure, malformed JSON, unsupported future versions or invalid profile fields must fall back to the built-in Stage Manager profile without blocking the Operator UI.

No Hub database migration is introduced by this foundation.

## Restore behavior

When a Project is opened:

1. restore the last valid visible workspace for that Project/profile if available;
2. otherwise use the profile default workspace;
3. otherwise use the first valid visible workspace;
4. otherwise fall back to Project Home.

When a profile is switched while the current workspace is hidden by the new profile, StageCore moves to the new profile's safe default workspace.

Unknown or removed workspaces are dropped during normalization. Duplicate page IDs are deduplicated. A profile cannot normalize to an empty visible workspace set.

## Profile management UX

The Web Operator provides a simple profile control in the top bar.

The operator can:

- switch instantly between built-in and custom profiles;
- create a custom profile from any built-in/current profile;
- rename custom profiles;
- choose visible workspaces;
- move visible workspaces up/down in priority order;
- choose the default workspace;
- choose compact/normal/wide navigation;
- reset to the built-in Stage Manager profile;
- delete custom profiles.

Advanced docking or coordinates are not exposed in this slice.

## SHOW behavior

Profile **switching** is presentation-only and remains allowed during SHOW.

For this device-local foundation, structural profile editing is deliberately disabled while the current Project has an active SHOW configuration lock. The lock check is read-only and uses the existing authoritative F-012 configuration-lock endpoint.

If lock state cannot be read, profile switching remains available but profile structural editing fails closed for that open management action and presents a localized explanation. This policy can be revisited when user/site-scoped profile persistence exists.

## Localization contract

F-017 is a post-contract keyed feature.

The same slice must provide:

- English feature name/definition/actions;
- Arabic (`ar-IQ`) feature name/definition/actions;
- keyed Arabic/English UI copy for profile management;
- RTL-safe layout;
- stable English technical profile/page IDs internally.

## Safety and security

F-017 must not:

- create a second RBAC model;
- hide Security as a substitute for permission checks;
- call GO/STOP or execution endpoints while applying a profile;
- update Project configuration or Runtime Snapshots;
- alter Session mode/current Cue;
- bypass F-012 configuration lock;
- load remote scripts, fonts or profile catalogs.

## Acceptance — foundation slice

- profile contract version 1 exists and is documented;
- built-in Stage Manager, Video, Lighting, Sound, Rehearsal and Monitoring presets exist;
- custom profiles can be created, renamed, reordered, filtered and deleted locally;
- active profile and last workspace restore after reload;
- invalid/corrupt/unknown workspace state normalizes to a safe fallback;
- switching a profile changes presentation only;
- profile switching remains available during SHOW;
- structural profile editing checks active SHOW lock and is blocked while locked;
- Arabic and English profile UI satisfies the Feature Localization Contract;
- workspace assets are embedded in the Hub binary and require no WAN service;
- full Core CI passes before merge.

## Deliberately deferred

The following remain part of the broader F-017 backlog item and therefore F-017 stays unchecked after this foundation slice:

- freeform docking and arbitrary panel resize;
- inspector placement/state;
- multi-window and multi-display coordinates;
- automatic off-screen native window recovery;
- user/account/site-scoped server persistence;
- shared profiles;
- export/import and Show Capsule integration;
- native macOS profile/window implementation;
- richer per-panel state once those panels exist.
