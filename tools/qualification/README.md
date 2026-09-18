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

## Authoritative qualification manifest

`tools/qualification/manifest.json` is the durable gate inventory for this campaign. It is derived from the currently authoritative open trackers:

- #148 — cumulative Phases 4–7 umbrella and final system sequence;
- #195 — Tablet Controller + Tablet Player RC3 physical acceptance;
- #221 — ESP32 DMX firmware + real hardware acceptance;
- #138 — remaining Phase 4 Callboard, Live Video and Network Cockpit gates.

Each gate has a stable `Q-...` id, a source issue/section, an acceptance statement, and one execution method:

- `AUTO` — software/network/device evidence can decide the gate;
- `AUTO_PHYSICAL` — automation drives and records the test, but real physical observation is part of PASS;
- `MANUAL` — inherently physical/documentary confirmation;
- `EVIDENCE` — release/CI/candidate evidence recorded from the authoritative build source.

The runner copies the exact manifest into each evidence directory before device operations. Later slices bind executable handlers to these stable gate IDs instead of inventing a second checklist.

Validate it without hardware:

```bash
tools/qualification/test-manifest.sh
```

## Durable campaign state and interruption-safe resume

Campaign progress is stored outside the repository by default at:

```text
~/.local/state/stagecore/qualification-campaign.json
```

The file is written atomically under an exclusive lock. Each gate keeps its current result plus history. The campaign pins the StageCore SHA and optional Tablet/firmware/hardware identities. A changed non-empty pin cannot silently reuse earlier PASS evidence.

Resume after any interruption:

```bash
tools/qualification/run-physical.sh --resume --non-interactive
```

Already-PASS and N/A gates are preserved. FAIL/BLOCKED/PENDING gates remain eligible. This makes chat/session/credit interruption irrelevant to previously captured evidence.

Inspect campaign progress:

```bash
tools/qualification/campaign.sh status
```

Manual fallback uses the same gate IDs and same state file:

```bash
tools/qualification/campaign.sh manual Q-TAB-06 PASS "operator observed expected video"
```

If a candidate component is deliberately replaced after a defect fix, repin it and explicitly invalidate only the affected gates plus required regression:

```bash
tools/qualification/campaign.sh repin lighting_firmware_sha <new-sha> "long-fade fix" Q-DMX-08 Q-DMX-09 Q-DMX-22
```

The previous result is retained in gate history. No campaign-wide wipe is performed.

### Qualification defect handling

Stop and fix immediately only when a defect is safety-critical, corrupts or invalidates evidence/state, proves the wrong candidate is installed, or makes downstream gates unsafe/untrustworthy. Otherwise record the gate FAIL with evidence, track a narrow defect, continue independent gates, then fix and rerun only affected gates plus required regression.

## Bounded non-destructive command evidence

The first executable qualification commands are deliberately allowlisted:

- `TABLET_PREPARE`
- `LIGHTING_STATE_READ`
- `LIGHTING_CONFIG_READ`

The Pi helper rejects every other command type. It logs into the Hub only through loopback, dispatches through the canonical authenticated Operator / Stage Device path, waits for the canonical persisted terminal command result, then logs out. Credentials are read from stdin and are never written to evidence.

Configure the local Operator credential once with a hidden password prompt:

```bash
tools/qualification/setup-operator-credential.py
```

The default local credential file is `~/.config/stagecore/qualification-operator.json` with mode 0600. Do not commit or paste that credential into chat.

For Tablet PREPARE, set `STAGECORE_TABLET_QUALIFICATION_MEDIA_NUMBER` in the local qualification config to a known installed media number. The command is recorded as the `prepare.command` milestone under `Q-TAB-06`; it does **not** mark the full physical PREPARE+GO gate PASS by itself.

Lighting state/config reads are recorded as `state_read.command` and `config_read.command` milestones under `Q-DMX-20`. Milestones survive interruption and are skipped on `--resume` after PASS.

## Armed physical action sequence

Visible/output-changing actions are separate from the read-only path. They run only when the local config explicitly sets:

```text
STAGECORE_QUALIFICATION_ENABLE_PHYSICAL_ACTIONS=1
```

The bounded physical allowlist currently covers Tablet PLAY/PAUSE/STOP plus one-channel Lighting SET/FADE and immediate BLACKOUT. Lighting action values must also be explicitly configured locally. Command completion is only a command-evidence milestone; it never substitutes for real visual/physical observation.

Each command evidence file is validated before PASS and rejects unredacted secret/token/cookie/password-like fields. The Pi helper also recursively redacts such result fields before emitting evidence.

After an armed run, inspect the aggregated pending block:

```bash
tools/qualification/campaign.sh pending-physical
```

When all listed observations were physically correct, record them in one operation:

```bash
tools/qualification/campaign.sh confirm-pending PASS "all listed tablet and lighting outputs were observed correctly"
```

If every listed observation is correct, `confirm-pending PASS` records the group in one operation. If one item is wrong, record that gate first with `campaign.sh confirm-one <GATE_ID> FAIL "<what was wrong>"`, then confirm the remaining pending observations. Command evidence/history remains preserved.

## Multi-channel and timed-blackout checkpoints

The armed lighting sequence now binds the next #221 gates without changing the physical-PASS rule:

- `Q-DMX-04::multi_set.command` — two configured channels in one canonical SET command;
- `Q-DMX-06::multi_fade.command` — the same two channels in one synchronized FADE command;
- `Q-DMX-11::precondition_set.command` + `timed_blackout.command` — restore visible output, then issue timed blackout.

Configure the second logical channel and test levels locally with `STAGECORE_LIGHTING_QUALIFICATION_SECOND_CHANNEL_KEY`, `...SECOND_SET_LEVEL`, `...SECOND_FADE_LEVEL`, and `STAGECORE_LIGHTING_QUALIFICATION_TIMED_BLACKOUT_MS`. The two channel keys must be different.

Command completion remains only a milestone. `Q-DMX-04`, `Q-DMX-06`, and `Q-DMX-11` become PASS only after the corresponding physical observation is explicitly confirmed.

## Measured timing and long-fade checkpoints

The #221 timing gates are prepared without inventing a product tolerance. Configure the intended acceptance tolerance locally with `STAGECORE_LIGHTING_QUALIFICATION_FADE_TOLERANCE_MS`.

After the normal one-channel fade completes, `Q-DMX-07::timing.measurement` compares the canonical persisted command lifecycle (`completed_at_us - issued_at_us`) with the requested `fade_ms`. The parent gate still requires real-device physical confirmation.

Configure `STAGECORE_LIGHTING_QUALIFICATION_LONG_FADE_MS` for Q-DMX-08. It must be greater than the normal qualification fade and no more than 120000 ms. The helper scales both command deadline and bounded result wait to the requested fade duration, so a valid long fade is not failed by the old short command timeout.

Existing local qualification configs are upgraded idempotently by `setup-access.sh`; newly introduced keys are appended without overwriting existing values.
