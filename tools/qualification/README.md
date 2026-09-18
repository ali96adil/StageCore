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
- The device probe opens the canonical StageCore SQLite database read-only and exports only qualification-safe Tablet/Lighting identity, capability, runtime/readiness and bounded observation metadata. Command payloads/results are deliberately excluded.
- Blank Tablet/Lighting IDs auto-select only when exactly one matching canonical device exists; ambiguity is BLOCKED rather than guessed.
- `--resume` is reserved for rerunning failed/blocked gates as later slices add the qualification manifest/state model.

## One-time local access setup

Create the local config and dedicated key:

```bash
tools/qualification/setup-access.sh
```

Fill `STAGECORE_PI_HOST` and `STAGECORE_PI_USER`. `STAGECORE_PROJECT_ID`, `STAGECORE_TABLET_DEVICE_ID`, and `STAGECORE_LIGHTING_NODE_ID` may remain blank when the target is unambiguous.

Then bootstrap Pi access once:

```bash
tools/qualification/bootstrap-pi-access.sh
```

The bootstrap may ask for the normal SSH password once to install the dedicated public key and the Pi sudo password once to install the root-owned helper. After that, the qualification runner uses `BatchMode` and `sudo -n`; repeated password prompts are treated as configuration failures.

The sudo rule grants only these fixed helper operations:

- `service-status`
- `service-restart`
- `service-journal`
- `device-probe`

The helper accepts no arbitrary command or extra arguments.

## Run

```bash
tools/qualification/run-physical.sh --full --non-interactive
```

Current read-only real-device gates check:

- Tablet Player canonical profile/protocol/capabilities, ONLINE/READY and fresh observation.
- ESP32 DMX canonical profile/protocol/capabilities, ONLINE/READY/fresh observation, DMX healthy, no brownout, STAGECORE authority and applied configuration hash.

No playback, lighting level, fade, identify, config-apply or blackout command is sent by this read-only slice.

## Deterministic self-tests

```bash
tools/qualification/test-runner.sh
tools/qualification/test-device-probe.sh
```

The tests require no Pi, Tablet, ESP32 or network.

Command execution, resumable state, cumulative Phase 4–7 gates and aggregated physical confirmations are added in subsequent slices.
