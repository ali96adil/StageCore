# Stage Device Project Assignment — v2 Draft Contract

Status: **DESIGN DRAFT — NOT IMPLEMENTED / NOT DEPLOYABLE**  
Trackers: StageCore [#239](https://github.com/ali96adil/StageCore/issues/239), Lighting firmware [#7](https://github.com/ali96adil/StageCore-ESP32-DMX-Lighting/issues/7), StageCore #147, #221, #226.  
Baseline reviewed: Core `efb49e35197a82b5d334d595c794efb9602a4d4c`; ESP firmware `04005fa4780d37d266e645e74d80c6dafeb31ed3`.

This document proposes a **versioned extension** to `stagecore.device/1`. It is not a claim that a `/2` implementation, endpoint, migration, test or real hardware qualification already exists. The existing `stagecore.device/1` frozen runtime and published Runtime Snapshot semantics remain unchanged until a reviewed, paired rollout.

## Product invariant

A physical node is reusable across projects **on the same trusted StageCore Hub** without changing its Wi-Fi provisioning, persistent device identity, pairing credentials, TLS pin or firmware. The node is assigned to at most **one** active project at any instant; its project ownership is **Hub-owned**, not client-declared. Physical channel definitions and show-specific aliases/configuration are kept distinct. StageCore Operator selects the project and publishes its own snapshot. No Project ID field belongs in first-run ESP Wi-Fi provisioning after v2 activation.

An unassigned node may be connected for authenticated diagnostics, but it must be **BLOCKER / blackout** and must not receive project commands. Removing the Project ID from the web form must be the **last** compatibility step, never a cosmetic early patch.

## Existing compatibility boundary

Current firmware stores a static `project_id` in NVS, requires it for provisioning completeness, sends it in `device.hello` and rejects other project command envelopes. Current Core `device.hello` upsert has historically taken the project supplied by the client. Draft PR #240 proposes fail-closed reconnect ownership, unbound command denial, Hub-disabled persistence and blocked readiness. The draft deliberately retains first-time legacy bootstrap so it cannot establish authoritative new-device assignment on its own. Do not merge/deploy this design as if it were v2.

`stagecore.device/1` RC3 Tablet Player and the installed ESP remain pinned to their reviewed integration until client compatibility is proven. Do not flip their project silently when a user changes UI selection or a client reconnects. A Hub-only DB project change without corresponding node-side authority and safe blackout is not a transfer.

## Ownership model

- **Device-level immutable identity:** `device_id`, trusted `hub_id` and verified TLS identity, paired credential; survive project changes on the same Hub. Hub revocation remains independent of project.
- **Hub-managed mutable assignment:** nullable `project_id`, a monotonically increasing, device-scoped `assignment_epoch` and explicit `assignment_state` (`UNASSIGNED`, `PREPARING`, `ACTIVE`, `BLOCKED`).
- **Project-owned configuration:** Draft, validated and published revisions, immutable Runtime Snapshots, logical aliases, current channel bindings and configuration hash. Assigning a node does not copy the old project's bindings or mutate historical snapshots.
- **Runtime authorization:** authenticated companion/device identity **plus** exact Hub assignment, assignment epoch, project, current published configuration/snapshot context and F-012 SHOW permissions. A client-provided `project_id` or DNS-SD announcement is never assignment authority.
- **Node local safety:** independent continuous DMX refresh, deterministic physical-dark output, failsafe, and explicit confirmed zero-state acknowledgment. The acknowledgment proves only the device's reported software state; physical DMX/decoder/LED output still needs separate qualification.

Keep existing `stage_devices.device_id` history and recorded commands; introduce a small additive assignment state/epoch migration. Project deletion must not erase a reusable device identity via the existing `stage_devices.project_id ON DELETE CASCADE` relationship: migration must explicitly review foreign-key behavior and historical references before modifying it.

## Proposed v2 handshake (not implemented)

1. **Discover + pair:** existing verified-Hub TLS, operator-approved pairing, revocable short-lived runtime credentials; all before project assignment. First-use discovery metadata does not independently authenticate the Hub.
2. **Hello:** v2 node sends its identity, software version, capabilities, safe output state and last acknowledged assignment epoch, **not an authoritative Project ID**. Server checks the authenticated device identity and reads the Hub-owned assignment.
3. **Unassigned reply:** server sends authenticated `assignment.state` with `UNASSIGNED`, current epoch and `blackout_required=true`; node confirms local blackout and stays `BLOCKER`. No show or commissioning outputs may be sent.
4. **Assigned reply:** server sends `assignment.state` with the exact assigned project and epoch. Node cannot become READY from this alone. It must confirm project/epoch, reject stale pending commands, match or apply the correct project's published configuration through existing snapshot authority and report safe observable state.
5. **Commands:** the node rejects any command missing or mismatching the committed epoch/project and snapshot authority. Hub rejects commands for unbound/disabled devices and rejects commands whose issue/dispatch races a transfer. Existing `command_id`, expiry, idempotency and canonical result/event semantics continue; reconnect does not replay.

Illustrative v2 control envelope, **subject to final schema review**:

```json
{
  "type": "assignment.state",
  "schema_version": 2,
  "device_id": "stable-device-uuid",
  "assignment_epoch": 42,
  "state": "ACTIVE",
  "project_id": "target-project-uuid",
  "blackout_required": true
}
```

A separate authenticated node acknowledgment includes its epoch, `blackout_confirmed`, all resolved logical output levels `0`, and a new random transfer challenge. Mismatched/replayed acknowledgment fails closed. Do not reuse `runtime.ready` as a transfer acknowledgment or assume a healthy DMX interface guarantees that a physical LED is dark.

## Explicit Operator Assign / Transfer / Unassign

Proposed Operator endpoint shape: `PUT /api/v1/stage-devices/{device_id}/assignment` with exact `expected_project_id`, `target_project_id` (nullable to unassign), `expected_assignment_epoch`, unique idempotency key, and explicit confirmation. Endpoint and payload are **not live**.

**Preflight:** authenticate operator, apply CSRF and permission checks; require project edit permission for both existing and destination projects as applicable; reject nonexistent target, revoked/disabled identity, active SHOW or structural mutation lock for either project, active commands, concurrent transfers or untrusted Hub. No silent fallback to an arbitrary project. Display all existing bindings and indicate that the destination requires its own Draft/Publish. The node must be reachable for verified blackout; **an offline node cannot be safely transferred based solely on an old ONLINE observation**.

**Prepare:** fence new runtime commands, assign a unique transfer challenge, request bounded local blackout over the authenticated channel, cancel/supersede fades and wait for an exact zero-state acknowledgment for the current connection. Timeout/reconnect/mismatch -> stay on old assignment but blocked; do not blindly change DB state.

**Commit:** after acknowledgment, in an atomic transaction compare the expected project + epoch and re-check locks/pending commands; increment epoch and commit new assignment. Close/fence the old connection generation and old project's command queue. In a follow-up handshake the node acknowledges the committed epoch. Until that acknowledgment, project config application and readiness are blocked and the output stays dark. An uncertain commit must be retried by idempotency key / queried; never send old brightness or automatically roll back to a nonzero state.

**New project:** author/validate/publish a new Runtime Snapshot, explicitly apply its resolved node configuration with existing `LIGHTING_CONFIG_APPLY` authority and verify hash/readiness. No command from an old project, old epoch, stale snapshot or old transport generation can affect output, even when a previously published snapshot remains immutable in the old project's history.

**Unassign:** the same guarded blackout/preflight process ends in `UNASSIGNED`. Keep pairing/trust but clear old project-scoped command/config authority. Keep canonical event/audit history.

## Firmware first-run UX after v2 is qualified

```text
StageCore Lighting Node
First-run provisioning. DMX remains at blackout.

Wi-Fi SSID
Wi-Fi password
Display name

Save and restart
```

Project selection lives in StageCore Operator. Provisioning never stores `project_id` as part of Wi-Fi completeness. Preserve legacy NVS key during deliberate one-time migration only; it cannot silently become command authority. A v2 node that reaches a v1-only Hub **fails safely** instead of silently accepting an unscoped hello or wildcard project. Neither an anonymous local web action nor a changed DHCP address can assign a project.

## Verification gates before promotion

1. Unit: no project hijack from first hello/reconnect; no unassigned dispatch or READY; disabled/revoked stay disabled; exact permission/CSRF/SHOW enforcement.
2. Concurrency: duplicate assign, compare-and-swap conflict, in-flight command and connection-generation race, idempotent retry after uncertain commit, expired/replayed/mismatched challenge.
3. Integration: old/new Core/firmware and RC3 compatibility, authentication/revocation, persisted epochs, stable identity, old published snapshot unchanged and new project Draft/Publish required.
4. Simulator: failure or disconnect before/during/after blackout ACK; safe output throughout and never replay old nonzero levels.
5. Physical (later, operator-approved): safe wiring Q-DMX-21, real DMX decoder and strips, bounded old-to-new transfer/blackout, Wi-Fi reconnect and old command rejection; review historic OSC Bench Test before SHOW. These results cannot be inferred from CI.

**Deployment prohibition:** never upgrade the installed Pi `809d1f4ce7a5824c27ecf82d78cd779cd28f6e6a`, flash the intentionally offline ESP, reset pairing or mutate Phase C published snapshot v10/schema v5 solely to test this draft. Canonical 79-gate campaign remains unchanged.
