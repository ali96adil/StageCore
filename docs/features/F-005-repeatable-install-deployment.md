# F-005 — Simple Repeatable Installation & Deployment

**Status:** Foundation slice in implementation  
**Feature ID:** F-005  
**Phase:** 2 — Installation, diagnostics, discovery, update, and extension operations

## Goal

A fresh supported Linux Hub should move from an unpacked StageCore release bundle to a running, enabled and readiness-checked StageCore service with one ordinary operator command and no manual systemd/file-permission choreography.

The same installation path must also be safe to repeat on an existing StageCore host. Reinstallation must preserve authoritative Project/security/history/media data and must never silently replace an existing deployment configuration.

## Supported platform contract

This foundation implements the already accepted SPK-06 deployment boundary:

- 64-bit Linux;
- `arm64` and `amd64`;
- native CGo-free StageCore product binaries;
- systemd service lifecycle;
- dedicated `stagecore` service identity by default;
- `/opt/stagecore`, `/etc/stagecore`, and `/var/lib/stagecore` reference layout;
- independent authoritative Data Root and Vault Root;
- local-first startup with no WAN or `network-online.target` dependency.

The installer must not claim support for an OS/architecture it has not validated.

## Foundation user path

An unpacked release bundle contains:

```text
stagecore-hub
stagecore-osc-plugin
stagecore-pairing
stagecore-setup
SHA256SUMS
install.sh
```

The ordinary fresh-host path is:

```bash
./install.sh
```

`install.sh` is intentionally thin. It elevates with `sudo` when necessary and delegates all deployment behavior to the tested Go command:

```bash
stagecore-setup install --bundle <bundle-directory>
```

Expert flags exist for non-default roots, listen address, service identity, dry-run, configuration replacement and install-without-starting. They are not required for the common Raspberry Pi / Linux Hub path.

## Release bundle integrity

Before changing the host, the installer must:

1. require all four product binaries;
2. require a `SHA256SUMS` entry for every required product binary;
3. verify the bytes of every product binary against those checksums;
4. inspect the ELF machine type and reject a bundle that does not match the executing Linux host architecture;
5. reject symlinked required artifacts so the bundle boundary is explicit.

SHA-256 detects bundle corruption/substitution relative to the supplied checksum manifest. It is **not** publisher authentication. Release signing/trust may be added separately and must not be falsely implied by this slice.

## Fresh install behavior

For a fresh deployment, `stagecore-setup install`:

1. validates platform, options and bundle integrity;
2. requires root only after non-mutating validation (unless `--dry-run`);
3. creates the configured system group/user when missing;
4. creates installation, configuration, Data Root and Vault Root directories with bounded permissions;
5. installs the four product binaries atomically into `/opt/stagecore/bin` by default;
6. writes a managed `/etc/stagecore/stagecore.env` with the production environment keys currently consumed by StageCore;
7. installs a generated `stagecore-hub.service` under systemd;
8. runs `systemctl daemon-reload` and enables the service;
9. starts/restarts the service unless `--no-start` is supplied;
10. polls the local `/health/ready` endpoint before reporting success.

The generated service orders after `local-fs.target` and `network.target`, not `network-online.target`, and uses the accepted StageCore sandboxing/service-account boundary.

## Repeat-install behavior

A repeated install is deliberately conservative:

- Data Root and Vault Root are never deleted, recreated destructively, emptied or migrated by the installer.
- Existing Project/database/security/history/Vault contents are outside the installer write set.
- Existing StageCore environment configuration is preserved by default.
- When an existing environment file is preserved, its Data Root, Vault Root, listen address and OSC plugin path become the effective deployment paths for generated service policy/readiness checks.
- An existing environment file that cannot provide the critical deployment paths is a blocker rather than a reason to guess.
- Replacing an existing environment file requires explicit `--replace-config`.
- Product binaries and the generated systemd unit are managed installation artifacts and may be replaced by a valid repeated install.
- File replacement uses same-directory temporary files plus rename so a partially copied executable/config/unit is not exposed as the final path.

F-010 owns full update backup/automatic rollback/version-transition policy. F-005 does not pretend that an idempotent reinstall is equivalent to a transactional updater.

## Configuration defaults

Fresh-install defaults:

```text
Install root:  /opt/stagecore
Config root:   /etc/stagecore
Data root:     /var/lib/stagecore/data
Vault root:    /var/lib/stagecore/vault
Service user:  stagecore
Service group: stagecore
Listen:        127.0.0.1:7840
```

Managed environment keys:

```text
STAGECORE_DATA_ROOT
STAGECORE_VAULT_ROOT
STAGECORE_LISTEN
STAGECORE_OSC_PLUGIN_PATH
```

OSC input project/listen configuration is deliberately not invented by installation because it is Project/runtime-specific rather than a universal host-install requirement.

## Dry run

`--dry-run` performs platform/options/bundle/checksum/ELF validation and prints the intended deployment actions without requiring root and without modifying files, users or services.

This is a first-class acceptance and support path, not a hidden test-only switch.

## Failure behavior

The installer must fail with actionable errors when:

- host OS/architecture is unsupported;
- a required artifact/checksum is missing or corrupt;
- the bundle architecture is wrong;
- required filesystem paths are invalid;
- an existing unmanaged/preserved config lacks required deployment values;
- service user/group state conflicts with requested ownership;
- systemd operations fail;
- service readiness is not reached after startup.

It must never delete authoritative data as a cleanup reaction to an installation failure.

## Safety / SHOW boundary

F-005 is a host deployment operation, not a live operator action. It must not be exposed as a normal SHOW control.

This foundation does not yet implement a live SHOW-state guard for upgrades/reinstalls. F-010 will own safe update orchestration, backup and rollback. Operational documentation must therefore treat replacement of a running production build as maintenance work outside an active show unless a later explicitly safe policy exists.

## Security

- Service runs as a dedicated unprivileged account.
- Installation requires root only for host mutations.
- Installed binaries are root-owned and not writable by the service account.
- Authoritative data/Vault roots are writable by the service identity.
- systemd uses `NoNewPrivileges`, `ProtectSystem`, `ProtectHome`, `PrivateTmp`, restrictive `UMask`, and explicit writable roots.
- No Internet connection, cloud account or remote installer service is required after a bundle is available locally.

## Acceptance criteria for this foundation

Software acceptance requires:

- unit tests for option/path validation;
- checksum verification rejects modified artifacts;
- ELF architecture validation rejects wrong architecture;
- environment parsing/rendering is deterministic;
- existing configuration preservation is tested;
- systemd unit rendering contains the accepted local-first/sandbox/write-path rules;
- dry-run produces a complete validated plan without root/system mutation;
- `stagecore-setup` still supports existing `status` and `setup-code` commands;
- Core CI Test/Vet/Race and Linux ARM64 CGo-free product-build gates pass.

Physical acceptance is intentionally deferred until access to the qualified Raspberry Pi is available. The later Pi gate should use this installer/reinstall path where appropriate instead of manually reproducing deployment steps.

## Deliberately deferred

F-005 remains broader than this foundation. Deferred slices include:

- remote/download bootstrap from a release URL;
- distro package-manager integration;
- interactive first-run product wizard (F-008);
- complete diagnostics/repair workflow (F-009);
- signed release trust chain if adopted;
- transactional data backup/automatic rollback/update policy (F-010);
- offline release-media/catalog orchestration beyond a local unpacked bundle (F-014);
- Plugin/Add-on installation (F-015);
- zero-configuration discovery/pairing (F-004).

The foundation must remain compatible with those later features rather than duplicating them.