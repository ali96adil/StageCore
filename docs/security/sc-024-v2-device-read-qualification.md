# SC-024 Phase 3C — Active V2 device Project read qualification

**Draft only. No production HTTP wiring, deployment, GO authority, or device output.**

Stack: #518 → #520 → #521 → #522 → this phase. Do not merge the
incompatible competing migration-53 alternatives #517 or #519.

## Motivation

Device identity is not Project identity. V2 Stage Devices have a deliberately
project-independent `stage_devices.project_id = NULL`, with the Hub-owned
current scope in `stage_device_assignments`. Never authorize a Project read
using only device.hello, a request URL, a legacy `stage_devices.project_id`,
an advertised device name/profile, or an old snapshot/epoch.

`ResolveActiveV2DeviceProject` is a **read-only** qualification helper for
future per-Project device metadata reads. It accepts *only*:
- enabled `stagecore.device/2` with null legacy project;
- current assignment `ACTIVE`, epoch greater than 1;
- nonempty owning Project and Runtime Snapshot matching a **PUBLISHED** Hub
  snapshot for the *same* Project;
- exact supported profile and matching persisted safety audit at **the
  current assignment epoch/Project/Snapshot**:
  - Tablet Player: `stage_device_tablet_assignment_audit` with ACTIVE
    `next_state`;
  - ESP32 DMX: `stage_device_lighting_activation_audit`;
  - StageLaser: `stage_device_stagelaser_assignment_audit`.

`CheckActiveV2DeviceRead` then verifies the caller's currently effective
`project.view` on the resolved owning Project and rejects a foreign
expected Project ID. Missing, UNASSIGNED, PREPARING, BLOCKED, disabled,
legacy, mismatched-audit, stale epoch, and mismatched Snapshot cases deny.
Unassigned/blocked device discovery belongs in a separately reviewed
**Owner-only global inventory**, not an ordinary Operator Project picker.

## Qualification tests

Test-only SQL fixtures emulate the persisted final metadata and matching
audits. They do **not** emulate a real authenticated safe-state handshake or
prove physical DMX/laser blackout. Software tests cover:
- V2 Tablet/ESP32 DMX/StageLaser positive read-only Project scope;
- active metadata without its matching safety audit denied;
- cross-Project direct device-ID probing denied;
- Project transfer without a new epoch-matched audit denied;
- later Owner visibility of correctly reaudited reassignment;
- BLOCKED, unassigned, disabled, unknown device, v1 and foreign Snapshot
  denied.

## What this deliberately does not do

- No new production endpoint or device list, and no change to existing
  global-role Project/Vault/GO routes: **real Project ACL is not yet enforced**.
- Not an output authority primitive. Commands require existing authenticated
  transport generation, safe-state evidence, Snapshot, control epoch, live
  session/GO permission and atomic dispatch-time fences.
- No universal device ownership inference for generic profiles, network
  sources, legacy v1 nodes, Companions or shared USB-DMX connections.
- No permission to revoke live SHOW/GO authority or disable emergency
  Blackout. Operational stop/blackout requires separate safety review.
- No hardware claims: completing tests/CI does not prove Pi 4, tablet,
  DMX or laser behavior.

Next gates: finish all indirect-ID/download/SSE/StageAudio/capsule scope
families; Owner grant migration impact review; atomic policy cutover with
rollback, and attended non-SHOW Raspberry Pi qualification.
