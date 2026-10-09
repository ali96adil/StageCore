# StageCore theatre move: show-ready freeze and offline rollout

This runbook is an **operator checklist**, not evidence of a physical installation. Do not report a device as updated until its observed firmware/software revision and live function are verified.

## Freeze rules
- Freeze new features while moving the show from home to the theatre.
- Require a green Core CI (including Go race tests and ARM64 build) for the exact Hub release revision.
- Do not treat Draft PRs or unqualified StageLaser relay actuation as show-ready.
- Preserve the existing Hub identity, DB, pairing, snapshots, operator permissions, and device credentials.
- Keep an offline copy of the previously working release and a verified backup of /var/lib/stagecore before applying an update.
- Run first in REHEARSAL, then perform explicit operator authorization for SHOW.

## Prepare the Linux ARM64 Hub bundle on a machine with Go
```bash
git clone https://github.com/ali96adil/StageCore.git stagecore-release-src
cd stagecore-release-src
git checkout <GREEN_MAIN_RELEASE_SHA>
bash scripts/build-release.sh "$PWD/dist"
cat dist/stagecore-linux-arm64/RELEASE_REVISION
(cd dist/stagecore-linux-arm64 && sha256sum -c SHA256SUMS)
```
Copy the **entire** `dist/stagecore-linux-arm64` directory or `dist/stagecore-offline-media.tar.gz` to the Pi over the trusted local network or a removable drive. Do not build from an unreviewed PR or use an unverified archive.

## On the Pi (from the copied bundle directory)
```bash
BUNDLE="$PWD"
cat "$BUNDLE/RELEASE_REVISION"
(cd "$BUNDLE" && sha256sum -c SHA256SUMS)
sudo "$BUNDLE/stagecore-setup" update --bundle "$BUNDLE" --listen 0.0.0.0:7840 --dry-run
# Review dry-run / active SHOW gate before proceeding:
sudo "$BUNDLE/stagecore-setup" update --bundle "$BUNDLE" --listen 0.0.0.0:7840
sudo systemctl is-active stagecore-hub.service
curl -fsS http://127.0.0.1:7840/health/ready
```
The transactional updater owns backup/rollback; preserve its printed rollback snapshot location. Never force an update while SHOW is active.

## Verify at theatre, in order
1. Pi Hub: installed RELEASE_REVISION matches approved commit; READY, persistent data, project, Cue list, and device trust preserved.
2. Mac Companion: installed version/build verified, same pairing identity, AUDIO-ABLETON and VIDEO-VDMX roles and OSC/MIDI endpoints confirmed.
3. ESP32 DMX: exact firmware revision, pairing/ACTIVE, channel mappings, blackout/failsafe, then actual 30-second fade.
4. Two consecutive GO actions: lighting 30s fade at t=0; audio GO at t=10; observe light continuing until t=30 without reset, with audio starting on time.
5. Tablets: all four pair/ACTIVE; media manifest present; Main, Overlay, Live, brightness/orientation and expected Cue actions checked.
6. Camera relay/ESP32-CAM: MJPEG, multiple viewers, tablet Live display and reconnect checked.
7. STOP CUE / STOP SESSION / Emergency Blackout: exercise with qualified safe loads and verify output state and terminal records. Do not assume a cancelled network request means a physical command was reversed.
8. StageLaser: **REQUIRED for the user's laser scene**. Verify the exact previously qualified firmware image/build and installed hardware revision before replacing either. Confirm the assigned ACTIVE device, physical ON/OFF, bounded local FLASH (frequency 0.1–1 Hz, duration up to 60 s), FLASH STOP, and SAFE OFF with the operator's qualified safe setup. Record results. The repo's historical NO-ACTUATION builds and the separate Draft TLS bootstrap PR are **not** substitutes for the user's physically qualified build; never overwrite it with an unqualified candidate. Missing required StageLaser is a show blocker for the laser scene, not a warning to dismiss.
9. Disconnect Mac/one optional tablet: operator should see a warning, not lose control of healthy required outputs; verify REHEARSAL GO behaviour.
10. Make an offline backup of the final show project and media before leaving home; repeat readiness at the venue after reconnecting the network.

## Evidence required to mark rollout complete
Record: Hub SHA, Companion build, ESP32 DMX firmware, StageLaser firmware/disabled status, camera relay revision, tablet app builds, runtime snapshot ID, last successful rehearsal timestamp, and each device's PASS/WARN/FAIL. A green GitHub CI is **not** evidence that devices were physically updated.
