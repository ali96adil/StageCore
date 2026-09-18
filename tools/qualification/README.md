# Physical Qualification Runner

This directory contains the reproducible local runner for cumulative StageCore physical/product qualification.

## Principles

- GitHub/CI proves software; this runner gathers Raspberry Pi, Stage LAN and real-device evidence.
- Secrets and credentials remain local under `~/.config/stagecore/`; they are never committed.
- SSH uses a dedicated qualification key and non-interactive `BatchMode`.
- Passwordless sudo is limited to a root-owned allowlisted helper. Never grant `NOPASSWD: ALL`.
- Automated checks distinguish `PASS`, `FAIL`, and `BLOCKED`.
- Physical observations that software cannot prove are recorded separately rather than fabricated.
- Runs write evidence under `qualification/runs/<UTC timestamp>-<pid>/`.
- `--resume` is reserved for rerunning failed/blocked gates as later slices add the qualification manifest/state model.

## One-time local access setup

Create the local config and dedicated key:

```bash
tools/qualification/setup-access.sh
```

Fill `STAGECORE_PI_HOST` and `STAGECORE_PI_USER` in the generated local config, then bootstrap Pi access once:

```bash
tools/qualification/bootstrap-pi-access.sh
```

The bootstrap may ask for the normal SSH password once to install the dedicated public key and the Pi sudo password once to install the root-owned helper. After that, the qualification runner uses `BatchMode` and `sudo -n`; repeated password prompts are treated as configuration failures rather than interactive prompts.

The sudo rule grants only these fixed helper operations:

- `service-status`
- `service-restart`
- `service-journal`

The helper accepts no arbitrary command or extra arguments.

## Run

```bash
tools/qualification/run-physical.sh --full --non-interactive
```

## Deterministic self-test

```bash
tools/qualification/test-runner.sh
```

The self-test requires no Pi, Tablet, ESP32 or network. It verifies report generation, PASS/BLOCKED semantics and that a sentinel secret is not copied into evidence.

The foundation currently covers local software checks plus bounded Pi/Hub preflight. Tablet, ESP32 DMX, cumulative Phase 4–7 gates, resumable state and aggregated physical confirmations are added in subsequent slices against their canonical contracts.
