# F-007 — Live Video / Camera Inputs

## Status

Phase 4 implementation specification.

## Goal

Add a generic StageCore live-source layer for theatre video workflows without turning the Hub into a video-processing server. StageCore owns source identity, configuration, routing intent, readiness and observability. Capture, decode and rendering execute on an appropriate Companion/render client.

## Source classes

Initial contract classes:

- `LOCAL_CAMERA` — built-in or locally attached camera on a render client;
- `USB_CAPTURE` — USB/HDMI capture interface;
- `NETWORK_STREAM` — a generic network-source adapter boundary.

Future adapters may represent NDI, RTSP, SRT, WebRTC or platform-native bridges without changing the Core source model.

## Source descriptor

A versioned live source records:

- stable `source_id`;
- project identity;
- human-readable name;
- source class;
- execution/render device identity;
- optional device-profile reference;
- opaque adapter configuration;
- advertised/observed capabilities;
- desired enabled state;
- readiness and last-observed time.

Secrets or credentials must use the existing secret-store/reference model rather than being embedded in exported source configuration.

## Execution placement

The Hub MUST NOT carry live frames through the command/event path. It may send bounded control commands such as open, close, select or route and receive small state/result messages.

A suitable Companion/render client owns:

- hardware access;
- capture negotiation;
- decoding;
- frame buffering;
- rendering/compositing handoff;
- platform-specific source APIs.

Heavy thumbnails, analysis and frame processing are outside the P0/P1 GO path.

## Capability vocabulary

Initial logical capability keys:

- `video.source.open`
- `video.source.close`
- `video.source.select`
- `video.source.route`
- `video.source.inspect`

Adapter-specific capabilities may extend this vocabulary but Core behavior must remain generic.

## Readiness

Common health vocabulary applies:

- `READY` — the configured source is available and the execution client reports usable state;
- `WARNING` — degraded but usable state such as elevated latency or format fallback;
- `ADVISORY` — informational limitation that does not block intended operation;
- `BLOCKER` — required source/client/capability is unavailable for the configured show requirement.

An optional unrequired camera source being offline must not make the entire Hub unhealthy.

## Failure behavior

- source loss produces a state/result event rather than crashing the Hub;
- reconnect may re-establish desired source state only where the action remains valid and unambiguous;
- expired transient routing commands are not replayed;
- format/capability mismatch is reported explicitly;
- unsupported source classes are never represented as working.

## Preflight

When a project marks a source as required, Preflight evaluates:

- execution client present;
- declared source/profile present;
- required capability available;
- source observation fresh enough;
- readiness not `BLOCKER`.

## Operator UX

The guided Video Sources workspace provides:

- source list and readiness;
- add/edit flow using source class and discovered/profile-backed choices;
- execution-device selection;
- bounded adapter configuration;
- open/close/test controls;
- clear error/remediation text;
- Arabic/RTL and English presentation.

Raw protocol URLs may remain in Advanced configuration for network adapters but are not required for common discovered local/capture devices.

## Acceptance

Software acceptance covers source schema/version validation, placement rules, optional-vs-required health, command expiry, client loss/reconnect, Preflight classification, localization and critical-path isolation.

The cumulative Phase 4 physical gate qualifies at least one actually available real source class. Unavailable classes are recorded N/A and are not falsely claimed.