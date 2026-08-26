# SPK-06 — Hub Deployment Prototype

Validates the deployable StageCore Hub service shape before M0 implementation.

## Proves

- zero-CGo Go Hub cross-builds as Linux `amd64` and `arm64` ELF binaries;
- Hub data root and Vault root are configurable independently so Vault can live on SSD/NVMe;
- stable Hub identity survives process restart;
- `/health/live`, `/health/ready`, and `/runtime/ping` work locally without Internet dependency;
- graceful SIGTERM shutdown works;
- a systemd unit can start after local filesystems/network without waiting for WAN/network-online;
- service sandbox grants write access only to the StageCore data path in the reference layout.

## Run

```bash
go test ./...
bash scripts/build-release.sh
bash scripts/smoke-local.sh
systemd-analyze verify deployment/systemd/stagecore-hub.service
```

## Limitation

The validation environment is Linux `amd64`, not Raspberry Pi/ARM64 hardware. The ARM64 binary is cross-built and inspected but cannot be executed here because no ARM64/QEMU runtime is available. Real Pi 5 / ARM64 SSD/NVMe, thermal, power-loss, network, SQLite/WAL, and soak qualification remains a hardware gate before claiming that exact device as production-ready.
