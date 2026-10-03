# StageCore Linux Deployment

F-005 provides the production foundation for installing an unpacked StageCore Linux release bundle on a supported `arm64` or `amd64` systemd host.

## Ordinary install

After extracting the bundle:

```bash
cd stagecore-linux-arm64   # or stagecore-linux-amd64
./install.sh
```

The wrapper uses `sudo` when required and delegates to `stagecore-setup install`.

Fresh production installs listen for the authenticated Operator UI on `0.0.0.0:7840`, so the Pi remains reachable from the Stage LAN even when the router/subnet changes. The installer still performs readiness checks through loopback on the Pi itself. Do not expose port 7840 to the public Internet or forward it from the Stage router.

Default fresh-host layout:

```text
/opt/stagecore/bin/
/etc/stagecore/stagecore.env
/etc/systemd/system/stagecore-hub.service
/var/lib/stagecore/data/
/var/lib/stagecore/vault/
```

The installer verifies `SHA256SUMS` and the ELF architecture of the four required service/install binaries (`stagecore-hub`, `stagecore-osc-plugin`, `stagecore-pairing`, `stagecore-setup`) before modifying the host. The release bundle also carries the optional `stagecore` CLI binary.

## Preview without changing the host

```bash
./install.sh --dry-run
```

Dry run does not require root inside `stagecore-setup`, though the shell wrapper may elevate before delegation. To guarantee no elevation prompt, call the bundle binary directly:

```bash
./stagecore-setup install --bundle . --dry-run
```

## Install without starting the Hub

```bash
./install.sh --no-start
```

This installs the binaries/config/unit, reloads systemd and enables `stagecore-hub.service`, but does not restart/start it or poll readiness.

## Existing deployments

A repeated installation preserves an existing `/etc/stagecore/stagecore.env` by default. StageCore adopts the Data Root, Vault Root, listen address and OSC plugin path from that existing configuration for service sandbox/readiness behavior.

It does **not** delete Project data, the SQLite database, security state, history, Notes, Vault objects or other authoritative contents.

To deliberately replace the environment file with values from installer flags:

```bash
./install.sh --replace-config
```

Review the existing configuration first. `--replace-config` is explicit because configuration replacement can change which authoritative data a Hub opens.

## Expert path overrides

```bash
./install.sh \
  --install-root /opt/stagecore \
  --config-root /etc/stagecore \
  --data-root /var/lib/stagecore/data \
  --vault-root /var/lib/stagecore/vault \
  --listen 0.0.0.0:7840 \
  --service-user stagecore \
  --service-group stagecore
```

The common Raspberry Pi/reference Linux path should not need these flags.

## Release build

From the StageCore repository:

```bash
bash scripts/build-release.sh
```

This creates both:

```text
dist/stagecore-linux-amd64.tar.gz
dist/stagecore-linux-arm64.tar.gz
```

Each unpacked directory contains the four required service/install binaries plus the `stagecore` CLI, `SHA256SUMS`, `RELEASE_REVISION`, and the one-command `install.sh` wrapper.

## Boundaries

- Linux/systemd only in this foundation.
- No WAN/cloud account is required after the bundle is local.
- SHA-256 verifies the bundle contents relative to the supplied checksum manifest; it is not publisher authentication/signing.
- Full update backup/automatic rollback belongs to F-010.
- Offline media/catalog orchestration belongs to F-014.
- First-run product setup belongs to F-008.
- Diagnostics/repair belongs to F-009.
