# Physical Qualification Runner

This directory contains the reproducible local runner for cumulative StageCore physical/product qualification.

## Principles

- GitHub/CI proves software; this runner gathers local Raspberry Pi, Stage LAN and real-device evidence.
- Secrets and credentials remain local under `~/.config/stagecore/`; they are never committed.
- SSH uses a dedicated qualification key and non-interactive `BatchMode`.
- Passwordless sudo must be bounded to qualification commands; do not grant `NOPASSWD: ALL`.
- Automated checks distinguish `PASS`, `FAIL`, and `BLOCKED`.
- Physical observations that software cannot prove are recorded separately rather than fabricated.
- Runs write evidence under `qualification/runs/<UTC timestamp>/`.
- `--resume` is reserved for rerunning failed/blocked gates as later slices add the qualification manifest/state model.

## Bootstrap

```bash
tools/qualification/setup-access.sh
```

Edit the generated local config, authorize the generated public key on the Pi, and install the repository-provided bounded sudo policy once that policy is introduced.

## Run

```bash
tools/qualification/run-physical.sh --full --non-interactive
```

The foundation currently proves the runner/report/evidence lifecycle and bounded Pi preflight. Tablet, ESP32 DMX, cumulative Phase 4–7 gates, resume state and aggregated physical confirmations are added in subsequent slices against their canonical contracts.
