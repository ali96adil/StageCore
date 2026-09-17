# Tablet Controller

## Status

Implementation branch: `feature/tablet-controller`

Tablet Controller is the StageCore operator experience for authenticated Android `TABLET_PLAYER` devices. It is intentionally **not** a second transport plugin.

## Architecture boundary

```text
StageCore Hub
  ├─ Companion pairing/session authority
  ├─ Stage Device runtime (`stagecore.device/1`)
  ├─ Stage Device repository / health / groups
  ├─ Official `stagecore.tablet-player` device profile
  └─ Tablet Controller ADDON metadata / graphical actions
          ↕ authenticated WSS
Android StageCore Player
```

The Android endpoint owns its device identity and connects directly to the Stage Device runtime. Tablet Controller contributes profile metadata, cue actions and operator UX. Legacy OSC remains rehearsal/debug compatibility only and is never authoritative when the official channel is active.

## Security contract

Stage Devices reuse the hardened Companion authority for identity and short-lived runtime sessions while using a separate Stage Device runtime protocol.

- Key algorithm contract: `P256_X963_SHA256`.
- Public key: uncompressed P-256 X9.63 point, standard Base64.
- Authentication proof: ASN.1 ECDSA signature over SHA-256 of the canonical `StageCore Companion Authentication v1` challenge message.
- Pairing request/status and auth challenge/session endpoints require the existing secure-device transport policy.
- Runtime endpoint: `GET /api/v1/stage-devices/runtime` upgraded to WebSocket.
- Authorization: `StageCoreSession <session_token>`.
- Runtime session TTL is currently 15 minutes; reconnect re-authenticates.
- Revocation is enforced while a runtime WebSocket is connected.

## Runtime sequence

1. Tablet discovers/configures the Hub endpoint.
2. Tablet authenticates an already trusted identity or creates a pairing request.
3. Operator approves the displayed pairing code through StageCore security controls.
4. Tablet obtains a short-lived runtime session.
5. Tablet opens the authenticated Stage Device WebSocket.
6. Tablet sends `device.hello` with `device_kind=TABLET_PLAYER`, profile `stagecore.tablet-player`, project scope and capabilities.
7. Hub replies `runtime.ready`.
8. Hub sends fresh `command.execute` envelopes only.
9. Tablet returns `command.result` and sends `device.observation` state.
10. Reconnect restores only safe display state; commands are not replayed.

## Tablet command vocabulary

The first official media surface includes prepare, play, pause, stop, blackout/clear, overlay play/clear and live show/hide. Each command maps to a declared tablet capability. The Hub rejects dispatch when the endpoint did not advertise the required capability.

## Graphical UX

The `stagecore.tablet-player` device profile supplies localized action names and JSON parameter schemas so Operator surfaces can render forms instead of requiring raw OSC or JSON. Existing Stage Device APIs already provide project device lists, group/location broad sends, runtime readiness and network cockpit observations.

## Extension packaging

`extensions/stagecore.tablet-controller/manifest.json` is an official `ADDON` manifest. It deliberately requests no runtime network permission because network transport belongs to StageCore Core. This avoids duplicating authentication, reconnect logic or command authority in an executable plugin.

## Qualification gate

Software CI must pass in both repositories before physical deployment. Physical qualification must then verify one real Pi/Hub and one Android tablet through pairing, authenticated connection, prepare/play, overlay, live, blackout, disconnect/reconnect, missing-media readiness, revocation and legacy OSC coexistence. A software CI pass alone is not a physical qualification pass.
