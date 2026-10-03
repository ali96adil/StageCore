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

The Hub installer verifies `SHA256SUMS` and the ELF architecture of the four required Hub service/install binaries (`stagecore-hub`, `stagecore-osc-plugin`, `stagecore-pairing`, `stagecore-setup`) before modifying the host. The release bundle also carries the optional `stagecore` CLI plus the independently installed `stagecore-camera-relay` component.

## Camera Relay persistent service

Release bundles also include the standalone Camera Relay binary and its one-time persistent-service installer. This component stays separate from the transactional Hub updater so Hub rollback remains complete and deterministic.

First install example on the Stage Pi:

```bash
./install-camera-relay.sh \\
  --source http://stagecam-xxxxxx.local:81/api/v0/stream \\
  --flash-control http://stagecam-xxxxxx.local/api/v0/flash
```

The installer enables `stagecore-camera-relay.service`; after that, Pi boot and process recovery require no operator shell. Re-running the helper preserves existing relay settings unless a value is explicitly supplied. The fresh listener default is `0.0.0.0:9081` for the isolated Stage LAN. Never port-forward this unauthenticated relay endpoint to WAN.

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

To migrate an existing loopback-only Pi to Stage-LAN Operator access without replacing unrelated settings, explicitly set only the listen address:

```bash
./install.sh --listen 0.0.0.0:7840
```

When `--listen` is explicitly supplied and an environment file already exists, the installer updates only `STAGECORE_LISTEN`; existing OSC/MTC and other environment entries are preserved. This is a one-time deployment migration. Afterward systemd starts the Hub automatically on every boot.

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
