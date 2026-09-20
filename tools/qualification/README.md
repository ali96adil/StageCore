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

The path is disabled unless `STAGECORE_QUALIFICATION_SOCKET` is set. The one-time Pi qualification bootstrap installs a systemd drop-in pointing it at `/run/stagecore-qualification/qualification-envelope.sock`, provisions that parent through `RuntimeDirectory=stagecore-qualification` with mode 0700, restarts Hub once, and creates the socket with mode 0600. It is not a TCP/LAN endpoint.

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


## Q-DMX-21 MAX485 / DMX pre-energization interlock

Q-DMX-21 is a MANUAL gate from Issue #221. The runner cannot prove electrical wiring from software, so it will never synthesize PASS from a probe, simulator, Stage Device observation, or command result.

Before any qualification action that can drive real DMX/light output, record all six checks:

```bash
tools/qualification/campaign.sh q21-status

tools/qualification/campaign.sh q21-ack documented PASS "document actual ESP32 -> MAX485 -> decoder pin/terminal path"
tools/qualification/campaign.sh q21-ack logic-voltage PASS "record module supply/logic compatibility and confirm no unverified 5 V reaches an ESP32 input"
tools/qualification/campaign.sh q21-ack de-re PASS "record DE and /RE direction-control wiring"
tools/qualification/campaign.sh q21-ack polarity PASS "trace the differential pair using actual module and decoder D+/D- or terminal labels"
tools/qualification/campaign.sh q21-ack common PASS "record/verify the intentional DMX signal-common/reference path"
tools/qualification/campaign.sh q21-ack termination PASS "record/verify end-of-line termination for the actual topology"
```

Each acknowledgement requires an explicit PASS/FAIL plus a physical observation note and is stored as a durable milestone with history. A FAIL makes Q-DMX-21 FAIL and means **do not energize**. All six PASS acknowledgements make the parent MANUAL gate PASS. Once accepted, replacing Q-DMX-21 evidence requires deliberate gate invalidation/repin rather than silently overwriting a qualified baseline.

Do not infer RS-485 polarity from the letters A/B alone: vendor labeling conventions vary. The evidence must trace the actual MAX485/module terminals to the decoder's documented D+/D-/COM (or equivalent) terminals.

The physical runner now treats Q-DMX-21 as a hard pre-energization interlock. Read-only StageCore/device checks can still run, but all lighting physical actions—including set/fade/blackout and fault-injection qualification—are suppressed until Q-DMX-21 is PASS.


## Tablet media-layer batch — Q-TAB-08 / Q-TAB-09 / Q-TAB-10

The physical runner now keeps the prepared main media playing while it exercises three independent RC3 layer/control gates in one armed tablet sequence:

1. Q-TAB-08: overlay play + clear using `STAGECORE_TABLET_QUALIFICATION_OVERLAY_MEDIA_NUMBER`;
2. Q-TAB-09: live show + hide using `STAGECORE_TABLET_QUALIFICATION_LIVE_MEDIA_KEY`;
3. Q-TAB-10: blackout + clear.

Pause/stop (Q-TAB-07) runs only after those layer checks, so Q-TAB-08 can truthfully verify that overlay use does not stop the main layer. A failure in one non-safety tablet gate does not suppress the independent later layer gates; command evidence remains isolated by stable milestone IDs.

Command completion alone does not make these AUTO_PHYSICAL gates PASS. The aggregated physical confirmation still requires seeing overlay continuity, live show/hide, and blackout/clear on the real Android tablet.

## Q-DMX-22 full-chain aggregate

Q-DMX-22 is the final ESP32/DMX full-chain gate. The runner evaluates it only from durable campaign truth; it does not invent a synthetic hardware PASS.

`Q-DMX-22::regression.prereqs` becomes PASS only when Q-DMX-01..Q-DMX-21 are all terminal PASS/N/A on a campaign that pins the exact StageCore SHA, exact lighting firmware SHA, and exact hardware baseline ID.

Any failed prerequisite makes the aggregate FAIL. Missing pins or incomplete prerequisites make it BLOCKED. After the prerequisite aggregate passes, Q-DMX-22 still remains AUTO_PHYSICAL until the operator confirms the representative real Raspberry Pi + Stage LAN + ESP32 + MAX485/DMX decoder + 24 V lighting chain behaved correctly.


## Remaining Tablet qualification batch — Q-TAB-11 through Q-TAB-15

This batch deliberately separates UI/human truth from read-only/negative-command evidence.

### Q-TAB-11 — graphical Cue Builder

Configure `STAGECORE_TABLET_QUALIFICATION_CUE_NAME`, then create that uniquely named Tablet Scene through the graphical Tablet Scenes/Cue Builder UI. The runner reads the SQLite database read-only and requires an enabled `TABLET_SCENE` whose actions are canonical `tablet.media.*` actions bound through `stage_device` aliases to the selected real tablet.

After the canonical milestone passes, confirm that the graphical editor itself exposed normal form controls and did **not** require raw JSON, capability keys, or target refs:

```bash
tools/qualification/campaign.sh q11-status
tools/qualification/campaign.sh q11-ack "created the qualification Tablet Scene entirely through graphical controls; no raw JSON or target_ref was exposed"
```

That acknowledgement is the physical/UI evidence for Q-TAB-11.

### Q-TAB-12 — Published Cue to real Tablet

Publish the revision containing the qualification Tablet Scene, start/use the pinned REHEARSAL Runtime Snapshot, and execute that Cue through normal StageCore runtime controls. The runner then proves read-only that the exact Published Snapshot contains the Scene, the Cue and all canonical actions completed, and at least one matching `TABLET_*` Stage Device command completed for the same Cue correlation on the selected tablet. Final PASS still requires seeing the expected result on the real tablet.

### Q-TAB-13 — missing media

Configure `STAGECORE_TABLET_QUALIFICATION_MISSING_MEDIA_NUMBER` to a number intentionally absent from the tablet manifest. The bounded helper sends only `TABLET_PREPARE` and treats the test as successful automation evidence only when the real tablet terminalizes `FAILED` or `REJECTED` with `MEDIA_NOT_FOUND`. Final AUTO_PHYSICAL PASS still requires confirming that StageCore showed the failure clearly to the operator.

### Q-TAB-14 — disconnect/reconnect no replay

After Q-TAB-12, Q-TAB-13 and Q-TAB-15 evidence are ready, the runner records a durable no-replay baseline and stops before the network action:

```bash
tools/qualification/campaign.sh q14-status
tools/qualification/campaign.sh q14-ack disconnect "tablet network was disconnected while the app remained powered"
tools/qualification/campaign.sh q14-ack reconnect "tablet network was restored and the authenticated device channel returned"
tools/qualification/run-physical.sh --resume --non-interactive
```

Post evidence requires an actual Stage Device disconnect and reconnect observation, the same project/snapshot scope, ONLINE/READY recovery, and **zero new production Tablet commands** after the prepared baseline. The physical confirmation separately verifies that old playback did not visibly replay/restart.

### Q-TAB-15 — mismatched scope rejection

The root-only qualification Unix socket now permits exactly one additional non-playing Tablet command: `TABLET_PREPARE`. Q-TAB-15 uses it only with deliberately wrong project and Runtime Snapshot scope. The real tablet must reject with `PROJECT_MISMATCH` and `SNAPSHOT_MISMATCH`. These qualification envelopes are not inserted into production command persistence and cannot become a second Tablet authority.


### Tablet batch resume hardening

Q-TAB-14 stores its reconnect baseline directly under the durable campaign state directory before any manual disconnect acknowledgement. If a run is interrupted after the milestone is recorded, resume first recovers the exact milestone evidence path; it will only recapture a baseline while no disconnect has been acknowledged. Once disconnect is acknowledged, a lost baseline fails closed and requires deliberate gate invalidation instead of inventing no-replay evidence.

Reconnect network evidence uses the durable pre-disconnect capture time as the start of the observation window, so a deliberate operator pause before acknowledging the disconnect cannot erase a real `WEBSOCKET_DISCONNECTED` observation.

Q-TAB-12 additionally correlates every completed canonical Tablet action execution with the corresponding Stage Device command `causation_id`; a Cue-level correlation alone is not sufficient.


## Batch: Q-TAB-16 + Q-DRAFT-01..07

### Q-TAB-16 two-tablet group targeting / justified N/A

The runner reads the live Tablet registry and only considers enabled RC3-profile Tablet Players that are ONLINE/READY, advertise `tablet.media.play`, share the pinned project/snapshot scope, and have a non-empty `group_name`. `STAGECORE_TABLET_QUALIFICATION_GROUP` may pin one group.

If at least two qualified members exist, the physical runner sends **one** Tablet Controller request using `group_name` (not two independent device commands) and verifies that the returned target set is exactly the qualified same-group set and every real `TABLET_PLAY` terminalizes COMPLETED. Final PASS still requires a physical observation that at least two tablets visibly started the expected media.

If the pinned campaign genuinely has fewer than two eligible physical tablets total, the runner records durable availability evidence and does not silently waive the gate. Two available tablets placed in different/empty groups are a configuration problem, **not** an N/A condition. Use:

```bash
tools/qualification/campaign.sh q16-status
tools/qualification/campaign.sh q16-na "only one physical Android tablet is available on this campaign hardware baseline"
```

`q16-na` refuses unless the current durable evidence says `INSUFFICIENT_HARDWARE`, `eligible_device_count < 2`, and `na_allowed=true`. It refuses N/A when two or more eligible tablets exist but group configuration is wrong.

### Q-DRAFT-01..07 recovery workflow

Set `STAGECORE_DRAFT_QUALIFICATION_PROJECT_ID` to the preserved/dedicated project whose current revision is the real child Draft. Configure a separate non-OWNER local credential at `STAGECORE_QUALIFICATION_NONOWNER_CREDENTIAL_FILE` (default `~/.config/stagecore/qualification-technician.json`) with `setup-operator-credential.py --output ...`; the runner never creates users or stores credentials in evidence.

The runner first captures a durable read-only baseline containing the exact Draft, validated parent, immutable PUBLISHED Runtime Snapshot bytes/hash/identity, and audit boundary. It then performs two bounded **negative-only** HTTP probes:

- authenticated non-OWNER DELETE must return `OWNER_REQUIRED` and leave Draft/snapshot unchanged;
- while that exact project has an ACTIVE SHOW, OWNER DELETE must return `SHOW_CONFIGURATION_LOCKED` and leave Draft/snapshot unchanged.

The helper has no successful discard mode. The actual successful discard must be performed through the real graphical Operator UI. After the two automatic negative gates pass, exit SHOW and observe the recovery UI:

```bash
tools/qualification/campaign.sh draft-status
tools/qualification/campaign.sh draft-ack visible "Discard Draft control is visible for the real Draft"
tools/qualification/campaign.sh draft-ack confirmation "confirmation explicitly names the Draft; cancelled once before final approval"
```

Then perform the confirmed Discard Draft through the UI and resume the runner. Post evidence requires all of the following simultaneously: project current revision restored to the exact validated parent, abandoned Draft is `SUPERSEDED`, the exact published Runtime Snapshot row including `manifest_json` and `content_hash` is unchanged, and a successful `project.draft.discard` security-audit record identifies the exact restored revision.

Q-DRAFT-01 and Q-DRAFT-04 remain human UI observations; Q-DRAFT-02/03/05/06/07 are evidence-backed automatic gates. No raw DB mutation or hidden successful discard API path is used by qualification.


Q-TAB-16 command evidence is also pinned to the same Runtime Snapshot used by group availability. Each returned command ID is checked in `stage_device_commands` for the exact device ID, project ID, `TABLET_PLAY` type and Runtime Snapshot before a group command milestone can pass. A logout failure is never allowed to replace an already captured qualification result.


## Phase 4 Callboard / Live Video / Network inventory foundation

The root-owned read-only `phase4-inventory` helper captures actual Stage Display capabilities and ONLINE/READY freshness, configured Live Video source classes/readiness, and latest Network Cockpit observation values for the pinned project. Sensitive endpoint URLs, network addresses, source configs and observed-state blobs are excluded. Network metrics remain `UNVERIFIED` when numeric and `NOT_MEASURED` when absent, never invented.

The runner records this inventory as a stable `inventory.baseline` milestone under `Q-CALL-01`, `Q-LIVE-01` and `Q-NET-01`. **Inventory milestone PASS is not physical/product gate PASS.** Q-CALL-01..06, Q-LIVE-01..04 and Q-NET-01..04 remain open until real display commands/rendering, camera/Companion execution, disconnect/reconnect, telemetry provenance and corresponding observations are proven. Q-CALL-04 N/A cannot be inferred from absence of a real display; Q-LIVE-01 requires at least one real source, and Q-LIVE-04 records unavailable classes explicitly.


### Bounded Callboard command batch (Q-CALL-01..04)

When `STAGECORE_QUALIFICATION_ENABLE_PHYSICAL_ACTIONS=1`, the runner selects exactly one freshly ONLINE/READY, enabled, canonical `STAGE_DISPLAY` matching `STAGECORE_PROJECT_ID`, requiring message/countdown/alert/clear capabilities. Set `STAGECORE_CALLBOARD_DEVICE_ID` when multiple eligible displays exist; missing/ambiguous/stale devices BLOCK without sending commands. The Pi helper independently revalidates device identity, project, protocol, capabilities and observation freshness before each command. Exact commands: one bounded `DISPLAY_MESSAGE` (4-second observation), one canonicalized `DISPLAY_COUNTDOWN` (60-second target; 8-second observation, not 8 repeated Hub ticks), a 3-second 40%-intensity `DISPLAY_ALERT`, optional one `DISPLAY_CHIME` only when advertised, and `DISPLAY_CLEAR` cleanup. No GO command or arbitrary user-provided alerts. The runner suppresses later commands if message/countdown/alert fails. Terminal Stage Device command milestones require real physical observation to mark Q-CALL-01..04 PASS; `Q-CALL-03` also requires a completed CLEAR. A non-chime display does not automatically create a Q-CALL-04 N/A; operator must explicitly confirm the physical capability case.


Optional `Q-CALL-04` chime N/A must not be synthesized by the runner. After a real Q-CALL-01 message physical PASS, run `tools/qualification/campaign.sh qcall04-na "selected real display and all qualified available displays lack display.chime.play"`. The command requires stored real inventory, the exact completed DISPLAY_MESSAGE device, zero eligible chime-capable displays, and a physical hardware note. A missing display or available chime-capable display cannot produce N/A.


### Q-LIVE-04 — explicit class-by-class qualification

After **Q-LIVE-01 physically PASSES** on the intended client/Companion path, the runner retains redacted source-class coverage under the durable campaign state directory. The operator must explicitly classify each of the three source classes, rather than treating absent classes as a simulated PASS:

```bash
tools/qualification/campaign.sh qlive04-status
tools/qualification/campaign.sh qlive04-ack NETWORK_STREAM PASS "Observed source rendered on the intended Companion"
tools/qualification/campaign.sh qlive04-ack LOCAL_CAMERA N/A "No local camera on pinned campaign hardware"
tools/qualification/campaign.sh qlive04-ack USB_CAPTURE N/A "No USB capture on pinned campaign hardware"
```

Each PASS requires at least one configured, desired-enabled, READY source with a named execution device plus a human physical observation note. Each N/A requires that the source class is not configured **and** a human hardware-availability explanation. The parent Q-LIVE-04 becomes PASS only after all three classes are individually PASS or N/A and at least one class is PASS. A missing Q-LIVE-01 physical PASS, wrongly grouped configured source, or missing durable coverage fails closed. This does not mark Q-LIVE-02/03 or Q-NET-03 PASS.

### Q-CALL-05 and Q-NET-03 evidence-only gates

The runner captures Q-CALL-05 exact Cue/action/command causation and Q-NET-03 null-vs-numeric persistence in one read-only batch. Neither automatic evidence milestone grants a physical gate PASS by itself: Callboard Cue rendering requires explicit physical confirmation; non-null network metrics require verified measurement provenance and actual Operator Cockpit classification. Q-CALL-06, Q-LIVE-01..03 and Q-NET-01/02/04 remain open pending live fault/reconnect and client/Companion evidence.


### Q-NET-02 / Q-NET-04 real network fault batch

Set `STAGECORE_NETWORK_QUALIFICATION_DEVICE_ID` to one actual enabled Stage Device in the pinned project (defaults to `STAGECORE_TABLET_DEVICE_ID`). The runner first captures a durable root-owned, read-only connected/ONLINE/READY baseline. It **never** disconnects a device. During a bounded controlled rehearsal, isolate **only that device's Wi-Fi/network**, leaving the Pi/Hub, other devices and power untouched. Run `campaign.sh qnet-status`, `campaign.sh qnet-ack disconnect "device-only network disconnected"`, restore the device network, then `campaign.sh qnet-ack reconnect "device network restored"`, and resume the runner.

The Pi verifier requires a real ordered `UNREACHABLE`/`WEBSOCKET_DISCONNECTED` observation after the baseline, a later `REACHABLE`/`WEBSOCKET_CONNECTED` observation, and a fresh ONLINE/READY Stage Device runtime. It records the genuine warning reason. Q-NET-02 and Q-NET-04 remain AUTO_PHYSICAL until their **separate** physical confirmations verify Operator Cockpit classification and actionable warning. No synthetic network observation or latency/jitter metric is accepted. A lost durable pre-disconnect baseline fails closed; deliberate gate invalidation is required rather than recapturing after disconnect. Other gates remain independent.


### Q-NET-01 / Q-NET-03 authenticated Cockpit truth

The runner authenticates to the real read-only `/api/v1/network/cockpit` surface and cross-checks it against latest SQLite observations for the pinned project. Q-NET-01 requires at least one enabled Stage Device, one desired-enabled Live Video source, and a real Companion observation; each must be present in the API with matching reachability/transport/latency/jitter. Final Q-NET-01 PASS still needs physical Operator UI confirmation. Q-NET-03 AUTO passes only when every expected target latency/jitter is actually null and remains null through the API; pinned `phase4.js` renders null as `—`. Any numeric metric produces `PROVENANCE_REQUIRED` and leaves Q-NET-03 BLOCKED until actual measurement provenance is qualified. Sensitive addresses, endpoints, configs, raw details and credentials are never written to evidence.


### Q-CALL-06 — real expired alert and display reconnect

The root-owned read-only `callboard-reconnect` helper requires a completed real bounded DISPLAY_ALERT (expired) followed by DISPLAY_CLEAR to durable IDLE, and a fresh connected/ONLINE/READY Stage Display. The baseline is durable across runs. No network isolation is automatic.

Isolate only selected display network (not Hub, power or other clients), run `campaign.sh qcall06-ack disconnect "display-only network isolated"`, restore network, then `campaign.sh qcall06-ack reconnect "display network restored"`, and resume. Post evidence requires actual ordered disconnect/reconnect observations, fresh ONLINE/READY, unchanged production command count and durable IDLE command identity. Missing baseline after disconnection fails closed. Physical PASS still requires real visual observation of no expired alert/chime replay. No credentials or source config is collected.


## One-command campaign and percentage dashboard

After the documented **one-time** key/credential setup and supported installation of the
**exact matching qualification candidate** on the Pi, use the single entrypoint:

```bash
bash tools/qualification/qualify.sh run
```

The first run starts the manifest-backed campaign. Every later `run` automatically selects
resume, preserving already-PASS / N/A gates and their evidence. Alternatively use the
explicit second command after defect fixes, changed device readiness, or manual actions:

```bash
bash tools/qualification/qualify.sh retry
bash tools/qualification/qualify.sh status
bash tools/qualification/qualify.sh issues
```

The runner continues independent checks, but stops unsafe/dependent actions where a
physical or trust prerequisite fails. It does **not** auto-restart the Pi, deploy a new
candidate, authorize dangerous test actions, power-cycle hardware, or turn missing physical
observation into PASS. Physical output commands remain behind
`STAGECORE_QUALIFICATION_ENABLE_PHYSICAL_ACTIONS=1` and the documented separate
fault-arms/safety gates. Re-running `retry` cannot substitute for the required operator
observation. A baseline change requires a deliberate `campaign.sh repin` with explicit
affected-gate invalidation; it cannot silently reuse earlier evidence.

The dashboard writes private local artifacts under
`~/.local/state/stagecore/qualification-reports/` (overridable with
`STAGECORE_QUALIFICATION_REPORT_DIR`):

- `summary.md`: completed/total, verified and assessed percentages, per-area counts.
- `summary.json`: machine-readable progress and counts.
- `defects.csv`: FAIL parent gates and any failed command/evidence milestones.
- `blocked.csv`: BLOCKED gates and blocked underlying milestones, including those
  whose parent remains PENDING.
- `manual.csv`: uncompleted AUTO_PHYSICAL/MANUAL gates to confirm physically.

Progress uses each unique manifest gate **once**: `(PASS + N/A) / total`; the
separate applicable verification rate is `PASS / (total - N/A)`. FAIL/BLOCKED/PENDING
are not completed. A completed command milestone does not inflate the overall percentage;
an AUTO_PHYSICAL gate needs the actual physical observation. If acceptance is not listed
in the manifest, the dashboard cannot claim that it was verified.

The failure register is local and derived from the durable state; it does not auto-create
GitHub issues or share local addresses, logs or evidence. For a discovered product defect,
record the corresponding GitHub issue separately using redacted evidence. When an actual
component build changes, repin only after choosing the affected gates and required
regressions. Do not assume the new SHA inherits earlier physical PASS.

The dashboard is not a substitute for exact-head GitHub Actions status, Companion/
Android evidence, or deferred Phase 5–7 checks that are not individually inventoried.
`qualify.sh` reports them as pending until the responsible evidence or manual gate exists.

Self-test without connecting devices:

```bash
bash tools/qualification/test-qualification-report.sh
```
