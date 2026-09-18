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

## Active-fade supersession checkpoint

Q-DMX-09 uses a dedicated bounded helper rather than overlapping generic runner processes. It applies a known starting level, starts a long fade, waits until the node observation reports that exact command as the active fade, then issues a newer SET. Evidence passes only when the old fade terminalizes `CANCELLED`, the replacement SET terminalizes `COMPLETED`, and the final observation names the replacement as both last accepted and last applied with the old fade no longer active.

Configure `STAGECORE_LIGHTING_QUALIFICATION_SUPERSESSION_FADE_MS`, `STAGECORE_LIGHTING_QUALIFICATION_SUPERSESSION_REPLACEMENT_LEVEL`, and optionally the bounded activation wait. The automation records only `Q-DMX-09::supersession.sequence`; the parent gate still needs explicit physical observation that the newer SET visibly took control and the old fade did not resume.

## Duplicate-ID and expired-envelope qualification socket

Q-DMX-12 and Q-DMX-13 need firmware behavior that the normal production Operator API intentionally prevents: production idempotency never resends the same command ID, and Core rejects an already-expired deadline before dispatch. Qualification therefore uses a dormant, root-only Unix-socket path inside the Hub.

The path is disabled unless `STAGECORE_QUALIFICATION_SOCKET` is set. The one-time Pi qualification bootstrap installs a systemd drop-in pointing it at `/var/lib/stagecore/qualification-envelope.sock`, restarts Hub once, and creates the socket with mode 0600. It is not a TCP/LAN endpoint.

The socket accepts only qualification-prefixed envelopes from issuer `qualification:physical-runner` and only the bounded command set needed here: SET, FADE, STATE_READ, and read-only CONFIG_READ. CONFIG_APPLY remains forbidden. These test envelopes bypass production command persistence by design so exact duplicate IDs and already-expired deadlines can reach the real firmware parser/dedupe/expiry logic.

- Q-DMX-12: establish a known level, run a real fade, resend the exact same terminal command ID, and require the second lifecycle to be cached-terminal-only (no second ACCEPTED/fade) with the same result payload.
- Q-DMX-13: establish a known level, inject an already-expired SET to a different level, require TIMED_OUT / DEVICE_COMMAND_EXPIRED, then read state and prove the rejected level was not applied.

Both gates are AUTO evidence on the real node, but the runner still requires `STAGECORE_QUALIFICATION_ENABLE_PHYSICAL_ACTIONS=1` because the preconditions deliberately change lighting output.

## Invalid-value / configured-bound checkpoint

Q-DMX-14 deliberately avoids automatic configuration mutation. The real-node sequence first reads the installed configuration and records its hash, establishes a known level, then proves two fail-closed cases: an out-of-range logical value (>100) must return `REJECTED / DEVICE_COMMAND_INVALID`, and a syntactically valid but unknown channel must return `REJECTED / CHANNEL_LEVEL_INVALID`. A state read after each rejection must show the original level unchanged.

When the selected channel has a configured bound narrower than 0..100, the same sequence also sends an in-schema value beyond that configured bound and requires the firmware to clamp to the exact configured minimum/maximum. It then restores the original level and requires the configuration hash to remain unchanged. If the selected channel is full-range 0..100, the clamp subcheck is recorded as not applicable inside the evidence; the two fail-closed checks still run.

Automation records `Q-DMX-14::invalid_value.sequence`. Because the manifest classifies Q-DMX-14 as `AUTO_PHYSICAL`, the parent gate remains pending until the batched physical confirmation verifies that no unsafe visible output change occurred and any clamp stayed within the configured limit.

## Q-DMX-15 boot / power-cycle / brownout checkpoint

Q-DMX-15 is interruption-safe and deliberately requires two explicit hardware actions. The runner never power-cycles or browns out the ESP32 automatically.

When physical actions are armed, the runner first stores a durable pre-event snapshot under the campaign state directory and records `Q-DMX-15::power_cycle.pre`. It then stops at a BLOCKED manual-action milestone. After the real power-cycle is complete, acknowledge only the action itself:

```bash
tools/qualification/campaign.sh q15-ack power-cycle "ESP32 supply was fully removed and restored"
tools/qualification/run-physical.sh --resume --non-interactive
```

The resumed runner captures post-reboot evidence before sending any further output-changing lighting command. PASS requires the same device/project/firmware/config hash, a reboot after the prepared baseline, `reset_reason=POWERON`, fresh ONLINE/READY runtime, healthy DMX, no active fade, and every logical current level at blackout zero.

Only after that post evidence passes does the runner prepare the brownout baseline. Perform the brownout only with a controlled low-voltage test method suitable for the ESP32 power path; do not short the 24 V supply or mains wiring. Then acknowledge:

```bash
tools/qualification/campaign.sh q15-ack brownout "controlled ESP32 brownout was induced and supply recovered"
tools/qualification/run-physical.sh --resume --non-interactive
```

Brownout PASS requires `reset_reason=BROWNOUT`, `brownout_warning=true`, fresh ONLINE/WARNING runtime, the same configuration hash, healthy DMX, no active fade, and logical blackout. Any automated safe-output failure marks Q-DMX-15 FAIL immediately and suppresses later lighting physical actions until the defect is investigated.

Inspect the two-stage checkpoint at any time:

```bash
tools/qualification/campaign.sh q15-status
```

Even after both automated post-event checks pass, Q-DMX-15 remains `AUTO_PHYSICAL`: the final parent PASS is recorded only when the operator confirms that both reboot events visibly returned the real lighting output to the safe blackout state.

## Q-DMX-16 Wi-Fi-loss / failsafe checkpoint

Q-DMX-16 is a resumable real-network workflow. It runs only after the automated Q-DMX-15 power-cycle and brownout post checks have passed. The runner creates a deliberately nonzero one-channel baseline, captures a fresh observation, then stops before the network fault.

The hardware/network operator performs two bounded actions against the ESP32 only:

```bash
tools/qualification/campaign.sh q16-status
tools/qualification/campaign.sh q16-ack disconnect "ESP32 Wi-Fi was isolated without removing power"
# observe the output while the ESP32 remains powered
tools/qualification/campaign.sh q16-ack reconnect "ESP32 Wi-Fi was restored after the failsafe observation"
tools/qualification/run-physical.sh --resume --non-interactive
```

Do not power-cycle the ESP32 for this gate and do not disconnect the Raspberry Pi/Hub from the Stage LAN. Use a network-control method that isolates only the lighting node.

Automated post evidence proves the same device/project/firmware/configuration returned, no reboot occurred, the Stage Device runtime is fresh and ONLINE, DMX is healthy, authority returned to STAGECORE, no fade remains active, and all logical lighting levels are blackout zero.

The frozen StageCore contract for this gate is **brief hold, then fade to blackout**. That behavior occurs while the node is offline, so StageCore cannot truthfully prove its visible timing from its own reconnect observation. Therefore Q-DMX-16 remains `AUTO_PHYSICAL`: `wifi_loss.post` is automation evidence only, and the final physical confirmation must explicitly confirm the visible hold/fade behavior and that stale brightness did not return after reconnect.

Static firmware review of FLASH CANDIDATE `a393c74ea16176df362db5c5482f916328856a81` currently shows an immediate failsafe blackout request on runtime/network loss. Do not mark Q-DMX-16 PASS merely because reconnect returns black; the physical gate must expose this contract mismatch unless the firmware policy is reconciled first.

## Q-DMX-17 Hub restart / no stale replay checkpoint

Q-DMX-17 is an independent fault gate for the Hub rather than the ESP32 network. It runs after Q-DMX-15 automated power-event evidence and before Q-DMX-16 prepares its Wi-Fi-loss baseline.

The restart is deliberately double-armed. Physical lighting actions must be enabled and the local config must also set:

```text
STAGECORE_QUALIFICATION_ENABLE_HUB_RESTART=1
STAGECORE_LIGHTING_QUALIFICATION_HUB_RESTART_FADE_MS=<20000..120000>
```

The bounded Pi helper establishes a known level, starts a real long fade through the canonical Operator/Stage Device command path, waits until the real node observation names that exact command as the active fade, and then restarts only `stagecore-hub.service`.

Automated PASS for `Q-DMX-17::restart.sequence` requires:

- the interrupted fade was one persisted `ACCEPTED` command before restart;
- Hub returned READY;
- the same ESP32/project/firmware/configuration reconnected without an ESP reboot;
- the interrupted command became a non-success terminal result rather than remaining ambiguous or becoming COMPLETED;
- no reconnect observation names the interrupted command as active, last accepted, or last applied;
- DMX is healthy and STAGECORE authority returns;
- all logical levels are blackout;
- the safe blackout remains stable for an additional bounded hold after reconnect.

A replay/authority failure marks Q-DMX-17 FAIL and suppresses the later Wi-Fi-loss fault gate. A missing/reconnect-timeout evidence path is BLOCKED rather than fabricated.

Q-DMX-17 remains `AUTO_PHYSICAL`: final PASS still requires the operator to confirm that the active fade was visibly interrupted by the Hub restart and did not resume or replay after reconnect. This gate is independent of the Q-DMX-16 hold/fade policy mismatch tracked in the firmware repository.

## Q-DMX-18 DMX stability under network + local-web activity

Q-DMX-18 separates what StageCore can prove automatically from what must be physically observed. Configure an explicit stress duration with `STAGECORE_LIGHTING_QUALIFICATION_STABILITY_SECONDS` (10..300); `STAGECORE_LIGHTING_QUALIFICATION_STABILITY_INTERVAL_MS` defaults to 500 ms.

The runner prepares a fixed nonzero output and stops at `Q-DMX-18::local_web.action`. Open the **protected read-only diagnostics/local web UI** on the ESP32 and keep navigating/refreshing that UI; do not trigger rehearsal fallback, configuration mutation, or emergency blackout during this gate. Then acknowledge and immediately resume:

```bash
tools/qualification/campaign.sh q18-status
tools/qualification/campaign.sh q18-ack "protected diagnostics UI is open and will stay active during the stress window"
tools/qualification/run-physical.sh --resume --non-interactive
```

On resume the runner re-arms the same fixed level immediately before the stress window, then drives bounded alternating `LIGHTING_STATE_READ` / `LIGHTING_CONFIG_READ` traffic over the real authenticated Stage Device connection. Automation requires every network command to complete, DMX health to remain true, authority to remain STAGECORE, firmware/config/reset identity to stay unchanged, no reboot/fade to appear, and the selected logical level to remain fixed throughout.

The parent Q-DMX-18 gate remains `AUTO_PHYSICAL`. Final PASS additionally requires observing the real decoder/24 V output for the whole window and confirming no visible flicker, jitter, dropout, or level jump while the protected local web UI is active.

The current FLASH CANDIDATE does not expose the protected local web/diagnostic UI required by the frozen firmware handoff. Therefore Q-DMX-18 must remain BLOCKED at the local-web milestone until firmware support exists; network-only evidence is not sufficient.

## Q-DMX-19 local emergency blackout with StageCore unavailable

Q-DMX-19 is deliberately the last bounded lighting fault action in the runner. It is double-armed by the ordinary physical-action switch plus:

```text
STAGECORE_QUALIFICATION_ENABLE_HUB_UNAVAILABLE=1
```

Do **not** enable this arm until the protected local firmware UI required by `StageCore-ESP32-DMX-Lighting#4` is installed on the exact pinned firmware candidate.

The runner first establishes and captures a fresh nonzero real-lighting baseline. It then uses the root-owned bounded helper to stop only `stagecore-hub.service` and proves the service is inactive. It never invokes the ESP32 local emergency control automatically.

While Hub is intentionally unavailable, use the protected ESP32 local emergency-blackout control and physically verify that the real lighting reaches blackout. Then, without manually restarting Hub, record the action and resume:

```bash
tools/qualification/campaign.sh q19-status
tools/qualification/campaign.sh q19-ack "protected local emergency blackout visibly forced the real output to black while Hub was unavailable"
tools/qualification/run-physical.sh --resume --non-interactive
```

On resume, before the normal Hub preflight, the runner sees the durable Q-DMX-19 outage + manual-blackout milestones and uses the bounded helper to restart only `stagecore-hub.service`. It then performs ordinary readiness/device discovery and Q-DMX-19 post verification.

Automated post PASS requires the same device/project/firmware/configuration, no ESP32 reboot/reset-reason change, fresh ONLINE READY/WARNING state, healthy DMX, STAGECORE authority after recovery, blackout levels, no active fade, no replay of the pre-outage StageCore command ID, and a second stable-blackout observation after a hold.

The parent gate remains `AUTO_PHYSICAL`; final PASS additionally requires the operator confirmation that the protected local control itself visibly caused blackout while Hub was actually unavailable.

If the operator interrupts the process after Hub stop, do not run an ordinary qualification resume until the local emergency action has been performed and recorded with `q19-ack`. The recovery hook intentionally will not restart Hub before that acknowledgement.

## Q-DMX-20 authoritative observation / configuration truth

Q-DMX-20 no longer treats a non-empty device `configuration_hash` as sufficient evidence.

The gate is now staged as:

```text
observation.readiness
state_read.command
config_read.command
published_config.truth
→ Q-DMX-20 PASS
```

The runner requires the exact pinned `STAGECORE_RUNTIME_SNAPSHOT_ID`. A root-owned read-only helper opens the StageCore SQLite database in read-only mode and reads only that exact `runtime_snapshots` row. It requires the snapshot to be `PUBLISHED`, belong to the pinned Project, use a lighting-capable manifest schema, and contain exactly one binding for the selected official lighting device.

The final validator intentionally uses StageCore's production `lightingnode.CanonicalConfiguration` / SHA-256 rules rather than duplicating the hash algorithm in shell. PASS requires all four identities to agree:

1. the immutable Published Runtime Snapshot's authoritative lighting configuration;
2. the real node's `LIGHTING_CONFIG_READ` payload;
3. the real node's `LIGHTING_STATE_READ.configuration_hash`;
4. the latest canonical Stage Device observation `configuration_hash`.

It also requires ONLINE/READY, healthy DMX, no brownout warning, and STAGECORE authority. A missing/not-current Published Snapshot is BLOCKED; a configuration/hash/identity mismatch is FAIL and the runner suppresses downstream physical lighting actions because the qualification baseline is not trustworthy.

No `LIGHTING_CONFIG_APPLY` is issued by this gate.
