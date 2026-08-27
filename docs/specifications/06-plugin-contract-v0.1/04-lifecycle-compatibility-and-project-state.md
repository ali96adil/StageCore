# 04 — Lifecycle, Compatibility & Project State

## Lifecycle

Reference lifecycle:

```text
Discover Package
 -> Inspect Manifest
 -> Validate API Compatibility
 -> Review Permissions
 -> Install
 -> Activate
 -> Register Contributions
 -> Health Check
 -> Available to Projects
```

Runtime states:

- `INSTALLED`
- `ACTIVE`
- `DEGRADED`
- `DISABLED`
- `INCOMPATIBLE`
- `FAILED`

## Project Enablement

Installing a plugin makes it available to StageCore, but a Project records explicit requirements when it uses plugin capabilities. A Runtime Snapshot includes the exact plugin ID/API requirement necessary to execute that snapshot.

## Upgrade

Before upgrade StageCore checks:

- Plugin API compatibility;
- configuration schema migration requirement;
- Projects/Snapshots that depend on the old version;
- whether restart/reload is required;
- whether Show Mode blocks the operation.

An upgrade must not mutate a Published Runtime Snapshot in place.

## Missing or Disabled Plugin

If a project references a missing/disabled plugin:

- the project still opens;
- affected Actions/Routes remain visible;
- they are marked unavailable with the missing `plugin_id`/capability;
- Validate/Preflight reports a blocker when the capability is required for runtime;
- StageCore never silently substitutes a different plugin or deletes the configuration.

## Uninstall

Uninstall is rejected or requires explicit acknowledgement when Projects depend on the plugin. Stored plugin configuration may remain as inactive namespaced data so reinstall/recovery can restore functionality.

## Show Mode Rule

Plugin install, uninstall, incompatible upgrade and configuration migration are not allowed during SHOW mode in v0.1.
