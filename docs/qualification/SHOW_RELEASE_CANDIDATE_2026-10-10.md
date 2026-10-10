# StageCore show release candidate — 2026-10-10

**Status: SOFTWARE RELEASE CANDIDATE ONLY. NOT SHOW-QUALIFIED.**

This is a pinned, reviewable release plan; it is **not** a production
deployment authorization, an installed binary, or a claim of physical laser
safety.

## Pinned source candidates

| Component | Source | Exact SHA | Evidence |
| --- | --- | --- | --- |
| StageCore Hub (overlapping Cues) | `integration/cue-overlap-main-20261009`, Draft PR #475 | `bc06146a58e4096827116d110ce9c37f0bdddd67` | Prior code CI PASS: [run 38029139773](https://github.com/ali96adil/StageCore/actions/runs/38029139773); new code adds short Hub deadlines for enabling laser commands: [run 38031186058](https://github.com/ali96adil/StageCore/actions/runs/38031186058) pending at pin time |
| StageLaser ESP32-C3 | `fix/laser-safe-off-queue-fence-20261010`, Draft PR #56 | `dfb97b3c9c17707494d9f02c2667cf0b4b0ccedf` | Earlier no-actuation CI PASS [run 38030585778](https://github.com/ali96adil/StageCore-ESP32-StageLaser/actions/runs/38030585778); new firmware rejects unbounded/future-dated enabling frames, native C++ tests added: [run 38031014333](https://github.com/ali96adil/StageCore-ESP32-StageLaser/actions/runs/38031014333) pending at pin time |

Both branches are review candidates. Neither is merged or installed by this
document. A green CI is not proof of an installed device's readiness.

## Show-day operator scope

1. Keep the currently verified deployed Hub and device images untouched until
   a full, attended offline rehearsal of the candidate is completed.
2. Back up the current StageCore SQLite database, configuration, show project,
   media manifests and device assignments; record exact installed versions.
3. Validate snapshot publication and ACTIVE assignment on all four tablets,
   lighting node, camera relay and Companion. Record unavailable outputs in
   the UI; do not silently substitute an unverified device.
4. Rehearse mixed Cue dispatch, per-Cue delay, overlapping Cue 2 while Cue 1
   is running, duplicate GO idempotency, STOP LATEST, STOP SESSION and
   Emergency Blackout. Verify observable physical outcomes on every relevant
   non-laser output, not only Hub ACKs.
5. Exercise missing Mac Companion, missing lighting node and reconnect cases.
   WARN-only unavailable outputs may be bypassed by the operator as designed;
   an output actually needed for a Cue must be checked before GO.
6. Confirm the operator-only Show Mode lock and a complete offline rollback
   path. Stop the rehearsal on any inconsistent state or unexpected output.
7. Capture logs, observed device states, exact revision SHAs and rehearsal
   evidence before authorizing a deployment.

## Laser is explicitly excluded from show-use actuation

The laser is controlled by a **momentary toggle** relay. Releasing GPIO3
only releases the relay contact: it does **not** guarantee the beam is OFF.
A previously observed failure mode is loss of ESP32 power while an
independently powered laser remains in its prior emission state.
The operator has now confirmed a main switch that isolates both laser and
ESP32, plus a separate **momentary toggle** laser pushbutton. The main switch
is a manual all-power isolation device, not an automatic independent inhibit
on ESP32-only failure.

Until an independent, automatically effective beam inhibit/interlock is
demonstrated on the installed hardware, **do not** enable laser actuation,
flash an actuation image or treat software SAFE_OFF as a physical interlock.

Permitted candidate firmware environments remain:
`esp32c3-ci-no-actuation` and
`esp32c3-ota-candidate-no-actuation`. The GPIO3 no-load qualification
environment is bench-only with relay and laser physically disconnected.

Required hardware evidence: actual beam OFF during ESP32 power loss,
crash/watchdog, network/Hub loss and emergency stop; verified GPIO3 reset
polarity and qualified transistor interface; exact wiring, interlock,
cold-start and rollback procedure. See StageLaser
`docs/PRODUCTION_ACTUATION_ACCEPTANCE.md`.

## Additional release blockers

- Hub and StageLaser now enforce short deadlines on output-enabling laser
  commands, and StageLaser rejects implausibly future-dated enabling frames.
  This closes some stale-command paths, **not** the full ordering problem.
  Hub-to-device authenticated, persistent **per-output monotonic command
  generation** remains incomplete. Timestamp watermarks alone cannot reject
  every late or reordered command.
- StageLaser TLS trust/pin bootstrap is still under separate review.
- Hardware qualification and a complete end-to-end attended rehearsal have
  not been recorded for these exact revisions.

**Decision:** StageCore software is a CI-green review candidate; the
combined StageCore + laser system is **NOT APPROVED FOR LIVE SHOW**.
Do not promote either Draft PR or deploy a new image solely on this manifest.
