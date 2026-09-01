# F-010 — Safe Update + Automatic Backup/Rollback — Software Acceptance Checkpoint

**Date:** 2026-09-02  
**Feature:** F-010  
**Status:** SOFTWARE COMPLETE — PHYSICAL PHASE 2 QUALIFICATION PENDING

## Scope closed by this checkpoint

The supported local Linux update path is `stagecore-setup update`. It is a guarded release-bundle transaction built on the F-005 deployment installer and F-009 Doctor rather than a blind reinstall.

The implemented transaction:

1. validates the candidate release bundle with the deployment installer in dry-run mode;
2. preserves the existing managed deployment configuration and does not permit update-time config replacement;
3. runs Doctor preflight and blocks on `BLOCKED`;
4. reads the authoritative SQLite session state and blocks while an active `SHOW` session exists;
5. performs no mutation during `--dry-run`;
6. requires root only for the real host mutation path;
7. stops the Hub before creating a cold rollback snapshot;
8. snapshots every managed path currently mutated by F-005: managed binaries, deployment configuration, Data Root, and the systemd unit;
9. verifies snapshot integrity before accepting it and again before restore;
10. installs the candidate through F-005 and waits for readiness;
11. runs Doctor postflight;
12. automatically restores, reloads systemd, starts the restored Hub, and runs rollback Doctor if candidate installation or postflight validation fails.

The Vault remains in place because the current F-005 update path does not mutate Vault payloads. Any future update path that mutates Vault content or metadata outside the authoritative Data Root must expand the F-010 rollback set first.

## Automated acceptance evidence

The existing `internal/update` test suite covers every acceptance item in `docs/features/F-010-safe-update-rollback-foundation.md`:

- dry-run validates without host mutation;
- Doctor preflight blocker prevents mutation;
- active SHOW prevents mutation before service stop/snapshot;
- successful update creates one rollback snapshot before candidate installation and performs postflight validation;
- candidate installation failure automatically restores the snapshot;
- postflight blocker automatically restores the snapshot;
- rollback failure is explicit and distinguishable;
- snapshot create/restore round-trips managed state;
- tampered snapshot payload is rejected before target mutation;
- symlinked managed state is rejected;
- the SHOW guard detects an active SHOW from the SQLite database.

## Security boundary

F-010 does **not** give the running Hub or Operator Web UI ambient root authority. The real update transaction remains an explicit local appliance command requiring root privileges. Web UI update controls, online release discovery/download, release channels, scheduled/unattended updates, fleet orchestration, snapshot retention policy, and arbitrary unsupported-schema downgrades remain outside the F-010 foundation contract.

This is consistent with the Phase 2 appliance model: the update command is shipped as part of the supported release bundle and does not require source checkout, Go, repository access, or developer tooling.

## Remaining verification

Software acceptance is complete when CI for the exact checkpoint head passes. Physical verification is deliberately deferred to the cumulative Phase 2 Raspberry Pi ARM64 qualification gate.

That later gate must prove at minimum:

- a known-good release-bundle upgrade on the supported Raspberry Pi path;
- an automatic rollback from a deliberately failing candidate or post-update health gate;
- preserved managed configuration/data after both success and rollback;
- successful Doctor/readiness after the resulting service state.

No physical qualification claim is made by this checkpoint.
