# Post-show integration qualification — 2026-10-17

**Branch:** `integration/post-show-20261017` (staging ONLY). **Never deploy or merge to main without approval and attended Pi 4 qualification**. Production Show executable and snapshots remain unchanged.

## Candidate code currently staged

This candidate manually stages the **exact changed files** from three nonoverlapping Draft PRs, preserving their intended content (it does **not** merge/close the original PRs):
- [#503 SC-003](https://github.com/ali96adil/StageCore/pull/503): v2 assigned DMX node may edit its own Project Draft lighting configuration. Current published runtime lighting authority remains unchanged.
- [#512 SC-022](https://github.com/ali96adil/StageCore/pull/512): rename Project without changing stable Project, Cue, Revision, Runtime Snapshot or Vault content identity.
- [#514 SC-025](https://github.com/ali96adil/StageCore/pull/514): **pure USB-DMX Micro label 6 protocol and read-only port detection** (NO USB lighting output yet). Existing ESP32 DMX path untouched.

All three sets were based on main and touch disjoint files in this first integration pass. Re-run full Core CI on their *combined* source; an individual green PR does NOT prove combined compatibility.

## Deliberately excluded pending review

- [#500](https://github.com/ali96adil/StageCore/pull/500): Cue touchscreen reorder — touches `internal/operatorweb/static/app.js`, also modified by #512; needs manual UI merge and phone touch qualification.
- [#504](https://github.com/ali96adil/StageCore/pull/504) → [#505](https://github.com/ali96adil/StageCore/pull/505): Date & Time dashboard + time-adjustment **preview-only**; stacked PRs must be integrated in order. Touch `core-ci.yml`, Hub options, Operator static assets.
- [#506](https://github.com/ali96adil/StageCore/pull/506) → [#507](https://github.com/ali96adil/StageCore/pull/507) → [#508](https://github.com/ali96adil/StageCore/pull/508): StageAudio engine, analog+Bluetooth discovery, phone UI. Stacked. Not ready for adoption until media uses **existing Vault**, Project-scoped ACL is enforced, playback/output physically tested and SHOW safety fence re-reviewed. Shares `cmd/stagecore-hub/main.go`, `core-ci.yml`, `app.css`, Operator assets with clock UI.
- Pre-show Cue GO overlap and latency PRs #456/#475/#476/#485: separate safety review and attended published snapshot tests; never silently fold into a post-show batch.

## SC-025 future stages

Manufacturer: https://shop.dmxking.com/ultraDMX-Micro_p_47.html . USB-DMX 0084 transmits ENTTEC DMX USB Pro-compatible label-6 messages on one 512-slot universe through FTDI VCP. #514 **only** validates wire framing. Secure serial port ownership, USB hotplug fail-safe, persistent supervised sidecar, authenticated device assignment/snapshot fencing, shared lighting channel mapping and actual DMX to fixtures remain deferred under [#513](https://github.com/ali96adil/StageCore/issues/513). Never mark the adapter `READY` from serial enumeration alone.

## Qualification gates (in order)

1. Review individual PRs and this integration diff. Run combined Go 1.26/1.27 unit, vet, race, JS syntax, ARM64 builds. Do not assume green tests mean Pi verified.
2. UI rename and DMX authoring readback with snapshot IDs stable. No regression in StageCore offline GO, blackout, partial output warning, manual lighting and Stage Devices.
3. For StageAudio, finish Vault and permissions before enabling. For USB-DMX, do Pi diagnostics `lsusb` and `/dev/serial/by-id` before authorizing serial writes.
4. Explicit operator approval for merging selected reviewed PRs to main, and separate explicit approval for installer update on Pi. Back up active data/snapshots, use non-show rehearsal and rollback qualification.
5. Keep show-frozen baseline branch/tag as recovery reference, do not delete it when cleaning old branches.

**Branch is not a production release.**
