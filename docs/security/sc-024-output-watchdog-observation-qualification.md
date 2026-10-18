# SC-024 Phase 5E — Passive output watchdog and replay qualification

**2026-10-18 · Draft integration #530 · source-only, read-only, NO runtime wiring.**

## Existing protection reused rather than replaced

- The live Hub already owns authenticated Stage Device V2 WebSocket
  generations, epoch acknowledgments, challenge/response, command journal,
  and stale-socket filtering. Their existing tests cover reconnects and
  prevent a prior socket from marking a replacement offline.
- The existing Lighting Node simulator deduplicates command IDs across
  disconnect/reconnect and rejects expired commands. The current
  Companion forwarder checks authenticated role readiness and applied
  runtime Snapshot before dispatch.
- The Hub emergency-blackout GO latch is a **software** safety fence,
  not a physical emergency-stop system or an independently measured
  light/audio state.

**Do not build a second authoritative transport or allow a watchdog
to invent READY, send GO, replay commands, clear blackout, or authorize
a Project transfer.**

## Added passive diagnostic model

\`internal/outputwatch/monitor.go\` provides a concurrency-safe
state machine which can receive information **only after** an upstream
trusted socket service validates device identity, assignment and
connection generation. There is no integration with production yet.

Its Scope binds the Device, Project, immutable Runtime Snapshot,
Assignment Epoch and durable Connection Generation. A reconnect
requires a strictly increasing generation; transferring between
Projects also requires a newer assignment epoch. A stale old socket
cannot detach a new connection.

| Observed condition | Honest UI meaning | Permission to dispatch |
|---|---|---|
| DISCONNECTED | No active known authenticated observation | No |
| AWAITING_FRESH_REPORT | New socket/heartbeat, no current applied-output observation | No |
| SOFTWARE_REPORT_CURRENT | Recent current-socket software state was reported | No |
| HEARTBEAT_STALE | Heartbeat deadline expired; report cannot be trusted | No |
| CLOCK_UNCERTAIN | Clock jumped backwards; require fresh generation | No |

Command evidence can be NONE, SENT_UNCONFIRMED,
SOFTWARE_ACKNOWLEDGED, or UNCONFIRMED. A report can upgrade to
SOFTWARE_ACKNOWLEDGED **only** for a previously tracked, unexpired
command on the current scope with strictly increasing sequences.
Connection loss, pending timeouts and clock reversal demote pending
evidence to UNCONFIRMED. Fresh heartbeat alone does not keep a stale
output observation current.

\`PhysicalOutputVerified\` and \`CommandsEnabled\` are **always false**.
Even a verified software-zero report must not become a claim about
DMX voltage, decoded lamp output, motion, mains power or speaker
silence. \`OperatorReviewRequired\` is always true. No method sends
output, provides credentials, or modifies Project/Show state.

\`TrackSent\` is informational bookkeeping for a command *already*
sent by a separate trusted router, not an authorization or send API.
Duplicate IDs are rejected for the entire monitor lifetime (including
reconnect). A bounded 512-ID history fails closed when full rather
than evicting IDs for replay; restart-persistent deduplication belongs
to the existing Hub command journal.

## Simulation coverage

\`internal/outputwatch/monitor_test.go\` tests:

- No accepting reports without a fresh authenticated-scope heartbeat.
- Source peer-reported software zero cannot imply physical darkness.
- Unsolicited, expired, duplicate and older-sequence software ACKs
  cannot confirm a command.
- An old socket's delayed heartbeat, report and cleanup cannot alter
  a newer connection; new generation alone never replays old commands.
- Changing Project at the same assignment epoch fails.
- An initial late heartbeat cannot revive a stale connection.
- Clock rollback makes the current socket CLOCK_UNCERTAIN until new
  authenticated generation, rather than trusting old leases.
- Heartbeat may remain healthy while software-output observation
  has aged out; the output then returns to UNKNOWN.
- Concurrent outdated socket callbacks leave the current scope intact
  under Go race checks.

**This phase does not send physical commands or drive the hardware.**
Actual ESP32 firmware watchdog behavior, Companion MIDI/OSC stop
acknowledgments, audio mixer fallback, and physical DMX zero validation
require separate attended testing and verified device-side contracts.

## Safe future integration gates

1. Introduce a *read-only* Hub-owned observation adapter that subscribes
   to existing authenticated DeviceChannel and Companion events,
   validates active Project/epoch/Socket/Snapshot and hands current-scope
   evidence to Monitor. It must not register a new unauthenticated
   data-entry HTTP endpoint.
2. Preserve the authoritative existing command journal, active
   assignment and Session fences. On Hub restart a new monitor begins
   UNKNOWN and must acquire a fresh authenticated generation.
3. Expose only honest diagnostics on an Owner/Operator scoped screen:
   communications, software ACK, physical state UNKNOWN. No
   automatic retry/replay or implicit media STOP.
4. Test with simulated clients and then safe disconnected loads; only
   attach a physical STOP or blackout procedure after qualified
   electrician/technician review. A physical emergency interlock must
   remain independent of browser, Wi-Fi, Pi and StageCore.
5. Never enable the new Project ACL feature or deploy this Draft to
   the show-tested Pi as a result of green CI alone.
