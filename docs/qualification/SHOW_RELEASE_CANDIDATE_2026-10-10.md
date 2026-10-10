# StageCore show release candidate — 2026-10-10

**Status: SOFTWARE RELEASE CANDIDATE ONLY. NOT SHOW-QUALIFIED.**

This is a pinned, reviewable release plan; it is **not** a production
deployment authorization, an installed binary, or a claim of physical laser
safety.

## Pinned source candidates

| Component | Source | Exact SHA | Evidence |
| --- | --- | --- | --- |
| StageCore Hub (overlapping Cues) | `integration/cue-overlap-main-20261009`, Draft PR #475 | `3b756639763400327dd7c9c5451affa51a441956` | Go 1.26/1.27, race and ARM64 CI PASS: [run 38025745113](https://github.com/ali96adil/StageCore/actions/runs/38025745113) |
| StageLaser ESP32-C3 | `fix/laser-safe-off-queue-fence-20261010`, Draft PR #56 | `7bef6e01cd166b4f96c23ce279136bb28fe60ea2` | Host tests PASS; no-actuation firmware CI [run 38028654541](https://github.com/ali96adil/StageCore-ESP32-StageLaser/actions/runs/38028654541) pending at manifest creation |

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

- Hub-to-device authenticated, persistent **per-output monotonic command
  generation** remains incomplete. Timestamp watermarks alone cannot reject
  every late or reordered command.
- StageLaser TLS trust/pin bootstrap is still under separate review.
- Hardware qualification and a complete end-to-end attended rehearsal have
  not been recorded for these exact revisions.

**Decision:** StageCore software is a CI-green review candidate; the
combined StageCore + laser system is **NOT APPROVED FOR LIVE SHOW**.
Do not promote either Draft PR or deploy a new image solely on this manifest.
