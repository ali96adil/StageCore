# StageCore show release candidate — 2026-10-10

**Status: SOFTWARE RELEASE CANDIDATE ONLY. NOT SHOW-QUALIFIED.**

This is a pinned, reviewable release plan; it is **not** a production
deployment authorization, an installed binary, or a claim of physical laser
safety.

## Current verification — 2026-10-10

The table below is an **audit of software candidates**, not a list of approved installable show releases. Earlier pinned revisions in this document were superseded.

| Component | Review-only candidate | Verification | Remaining gate |
| --- | --- | --- | --- |
| StageCore Hub / Operator | `integration/cue-overlap-main-20261009` at `60c4ad93e6aca5c98a0ec7f21aca3b725fd4cac9`, parent [Draft #475](https://github.com/ali96adil/StageCore/pull/475) | Earlier merged head `5b5ff470` **PASS** full Go 1.26/Race + Go 1.27 + ARM64 [CI 38067617763](https://github.com/ali96adil/StageCore/actions/runs/38067617763); [PR #478](https://github.com/ali96adil/StageCore/pull/478) test-head `ce21a9a` **PASS** [CI 38068750870](https://github.com/ali96adil/StageCore/actions/runs/38068750870) | **Exact merged head** `60c4ad93` must pass independent [CI 38069924010](https://github.com/ali96adil/StageCore/actions/runs/38069924010), then paired protocol/device and attended physical tests |
| StageLaser ESP32-C3 | [Draft firmware #56](https://github.com/ali96adil/StageCore-ESP32-StageLaser/pull/56) at `c5057005b0760c0fa8d0862b5373043d22330120` | Review-only source, no verified show-device install or beam-off failure tests | Must match Hub `control_generation` protocol; **no actuation build/no laser-connected flash** until power-loss, reset, network-loss and independent fail-off are physically proven |
| Stage devices + Companion | Previously deployed, locally proven versions only (exact live image SHAs still to be gathered) | Prior operator-reported readiness is not the same as current end-to-end acceptance | Enumerate and qualify four tablets, DMX, camera relay, VDMX/Ableton Companion and STOP/Blackout under actual show load |
| Current Raspberry Pi Hub | Earlier operator-reported installed candidate `e7fe98347` (confirm with `RELEASE_REVISION` and running binary before assumptions) | Historical `/health/ready` observed; **current running SHA not remotely verified** | Record actual deployed revision, save DB/media/config/rollback bundle; do not replace it for this draft |

**No code in this review document installs firmware or changes the live Pi.** In particular, CI success is not authorisation to merge parent #475 into `main` or to load the coupled Hub/StageLaser protocol onto the live show devices.

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

- **Software vs installed protocol:** the Hub integration branch implements persistent `control_generation` and Draft StageLaser firmware #56 implements matching validation. This closes a known software ordering gap *only for coordinated matching versions*. Those revisions are not yet shown to be installed or physically qualified together. Existing Stage Devices should not be assumed to enforce the new fencing.
- StageLaser momentary toggle is not fail-off on ESP32-only power loss; an all-power manual disconnect alone does not provide independent automatic inhibition. StageLaser remains **excluded from show actuation** until a demonstrated hardware interlock/fail-off design passes the listed physical tests.
- StageLaser TLS trust/pin bootstrap and any other firmware-specific blockers require exact-device verification.
- No attended integrated rehearsal, verified backups or rollback procedure has been documented for the exact current Hub/peripheral candidate.
- The latest CI run URL is a verification pointer, **not** a release artifact or install instruction.

**Decision: SOFTWARE REVIEW CANDIDATE ONLY — NO LIVE SHOW AUTHORISATION.** Do not deploy the Draft integration head or change the show project on the basis of this document.
