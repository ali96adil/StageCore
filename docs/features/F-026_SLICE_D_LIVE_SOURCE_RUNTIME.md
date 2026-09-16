# F-026 Slice D2a — Native LiveSource Runtime

Status: implementation/CI qualification in progress. Software only.

Physical capture/GPU/display qualification remains deferred under GitHub Issue #148.

## Scope

D1 established F-007 LiveSource execution placement on either a Stage Device or Companion Machine Role, plus deterministic Companion control state and Machine Role Preflight authority.

D2a makes the Companion control contract truthful in production when the native Visual Engine is enabled:

- `LOCAL_CAMERA` may use the default local AVFoundation video device only when no endpoint is supplied; an explicit endpoint must match exactly or fail closed;
- `USB_CAPTURE` requires an explicit AVFoundation video device endpoint and never falls back silently to a different/default camera;
- native `NETWORK_STREAM` opens HTTP/HTTPS media supported by AVPlayer, including HLS-style streams;
- unsupported/local-file schemes such as `rtsp://` and `file://` fail closed rather than advertising unsupported native playback;
- frames remain local to the Companion and never traverse the Hub;
- `video.source.open/close/select/route/inspect` are registered only when native Visual Engine support is explicitly enabled;
- LiveSource state is committed only after the native runtime operation succeeds;
- shutdown clears native source authority and local routing state;
- a fresh Companion/LiveSourceEngine starts empty and never replays prior open/select/route commands after reconnect or restart.

## Safety invariants

- Reconnect is connectivity, not command acknowledgement or replay authority.
- A source must be explicitly opened in the current runtime before select/route can succeed.
- Runtime open/route failures fail closed and do not create false source/route state.
- Explicit capture-device selection never falls back to another device.
- Unsupported network protocols fail before creating source authority.
- The Hub carries descriptors and routing intent only, never frame payloads.
- Native Visual Engine disabled means native LiveSource capabilities are not advertised.

## Deliberate D2a boundary

D2a opens and owns real native capture/network resources and retains deterministic route intent locally, but does not yet attach the capture/player layers to the existing named-output/layer renderer surface.

D2b must complete:

- native frame-layer attachment into the Visual Engine named-output/layer renderer;
- production wiring of the D1 Machine Role LiveSource Preflight authority in the Hub product;
- renderer/source availability diagnostics required to close Slice D.

Do not mark Slice D complete from D2a alone.

## Acceptance

- existing F-007 LiveSource contract tests remain green;
- native runtime failures do not commit false state;
- explicit/missing capture-device endpoint cases fail closed without silent fallback;
- unsupported/local-file network schemes fail closed without source state;
- fresh runtime after reconnect/restart has no implicit source or route state;
- Companion bootstrap advertises LiveSource capabilities only with native Visual Engine enabled;
- Companion Core CI passes on exact head;
- Core CI remains green;
- no physical qualification is claimed.
