# F-010 — Safe Update + Automatic Backup/Rollback — Foundation

**Status:** Phase 2 foundation slice  
**Scope:** supported local release-bundle update transaction for managed Linux StageCore installations

## Goal

Make a StageCore software update a guarded transaction rather than a blind reinstall.

The operator supplies a validated unpacked release bundle. StageCore must prove the current installation is healthy enough to update, protect the current managed state, install the candidate, prove the candidate is healthy, and automatically return to the prior managed state when candidate installation or validation fails.

## Command

```text
stagecore-setup update [options]
```

The default bundle is the directory containing `stagecore-setup`, matching the supported F-005 release-bundle workflow.

Useful options include:

- `--bundle`
- `--backup-root`
- managed deployment path/identity overrides shared with F-005
- `--readiness-timeout`
- `--dry-run`

`--dry-run` validates the candidate bundle, runs Doctor against the current installation, evaluates the SHOW gate, and prints the update plan without stopping the service, creating a rollback snapshot, or changing files.

## Transaction order

1. Run the F-005 installer in dry-run mode against the candidate bundle.
   - Verify release checksums and executable architecture.
   - Resolve/preserve the current managed deployment configuration.
   - Do not permit update-time config replacement.
2. Run `stagecore doctor` against the current installation.
   - A `BLOCKED` Doctor result prevents update.
   - Warnings remain visible but do not silently become blockers.
3. Read the authoritative session database and refuse update while an active `SHOW` session exists.
4. For a real update, require root privileges.
5. Stop `stagecore-hub` before capturing rollback state.
6. Create a protected cold rollback snapshot of every path the current F-005 installer may mutate:
   - managed binary directory;
   - deployment configuration root;
   - authoritative Data Root;
   - installed systemd unit.
7. Verify the snapshot payload tree hashes before it is accepted.
8. Install the candidate through the existing F-005 installer and wait for `/health/ready`.
9. Run `stagecore doctor` again.
10. If installation fails or post-update Doctor is `BLOCKED`, automatically:
    - stop the failed candidate;
    - verify the rollback snapshot again;
    - restore the prior managed paths;
    - reload systemd;
    - enable/start the restored Hub;
    - run Doctor against the restored installation.

## Vault rule in this slice

The StageCore Vault is deliberately retained in place rather than copied into every update snapshot because the current F-005 update/install path does not mutate Vault payloads. This avoids copying potentially very large immutable media payloads for every software update.

This is a safety contract, not a permanent exemption: **before any future migration/update path is allowed to rewrite Vault content or metadata outside the authoritative Data Root, F-010 must expand its rollback transaction to cover that mutation first.**

## Failure semantics

- Candidate validation failure: no host mutation.
- Doctor preflight `BLOCKED`: no host mutation.
- Active SHOW: no host mutation.
- Snapshot failure: candidate is not installed; the unchanged Hub is restarted.
- Candidate install failure: automatic rollback is attempted.
- Candidate postflight `BLOCKED`: automatic rollback is attempted.
- Rollback health `BLOCKED` or restore/start failure: return an explicit `automatic rollback failed` error while preserving the original candidate failure in the error chain.

An update must never report success merely because files were copied. The candidate must pass the post-update health gate.

## Security and integrity

- Candidate release artifacts continue to use F-005 checksum + ELF architecture validation.
- Update rollback storage defaults to `/var/backups/stagecore/updates` and is root-protected.
- Snapshot copy rejects symlinks and non-regular filesystem objects in the managed mutation set rather than following them unexpectedly.
- Snapshot payloads are tree-hashed and reverified before rollback mutation begins.
- Existing deployment configuration is preserved; F-010 does not expose F-005 `--replace-config` during update.
- SHOW state is read from the local authoritative database using a read-only SQLite connection.

## Acceptance for this foundation

Automated tests must prove at minimum:

- dry-run performs validation without mutation;
- Doctor blocker prevents mutation;
- active SHOW prevents mutation;
- success creates one rollback snapshot before candidate install and performs postflight validation;
- install failure automatically restores the snapshot;
- postflight blocker automatically restores the snapshot;
- rollback failure is explicit and distinguishable;
- snapshot create/restore round-trips managed state;
- tampered snapshot payload is rejected before target mutation;
- symlinked managed state is rejected;
- the SHOW guard reads an active SHOW from SQLite.

Physical Phase 2 acceptance later must exercise this on the supported Raspberry Pi release path with a known-good upgrade and a deliberately failing candidate/health gate before F-010 is considered fully verified.

## Explicitly deferred

This foundation does not yet provide:

- online release discovery/download;
- release channels;
- unattended scheduled updates;
- remote fleet update orchestration;
- pruning/retention policy for old rollback snapshots;
- downgrade across arbitrary unsupported schema compatibility boundaries;
- UI update controls.

Those may build on this transaction only after the local update/rollback semantics are proven.
