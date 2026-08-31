# internal/deployment

This package owns the host-installation mechanics introduced by the F-005 foundation.

It intentionally does **not** own:

- StageCore runtime execution;
- Project/session/cue state;
- database migration semantics beyond starting the normal Hub against the configured Data Root;
- full update backup/rollback policy (F-010);
- diagnostics/repair (F-009);
- online/offline package catalog distribution (F-014/F-015).

The package validates a local Linux release bundle, renders the managed environment/systemd unit, preserves existing deployment configuration by default, installs product binaries atomically, and performs bounded service readiness verification.
