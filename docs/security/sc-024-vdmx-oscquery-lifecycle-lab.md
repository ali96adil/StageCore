# SC-024 Phase 5M — OSCQuery readback freshness, reconnect and path invalidation

**2026-10-19 · Draft PR #530 · source-only and offline. No StageCore
Hub, Companion, VDMX 6, MIDI, lighting, tablet or Pi operation changed.**

## Why one working OSCQuery GET does not prove an ongoing VDMX state

OSCQuery's bidirectional mode is optional and uses a WebSocket. Its
published contract includes binary OSC values, plus JSON address-space
events such as PATH_CHANGED, PATH_REMOVED, PATH_ADDED and PATH_RENAMED.
Subscriptions stop on socket disconnect and require new explicit
LISTEN requests on reconnect. An in-memory value cached before a
disconnect or a changed path is **not current**.

Reference: https://github.com/Vidvox/OSCQueryProposal

The StageCore VDMX input port 3555 is not a readback channel and
an OSCQuery Control Surface's published value does **not** establish
that the output video frame was actually rendered. Never turn a readback
into an application Cue ACK unless a separately authenticated
receiving-app integration and actual state attribution are qualified.

## Offline state machine

\`tools/receiver-ack-lab/vdmx_readback_session.js\` implements a
non-network, non-actuating \`VDMXOfflineReadbackSession\`.
A prospective transport adapter would have to authenticate and
authorize the exact Project, immutable Snapshot, Machine Role,
receiver instance and generation **BEFORE** calling \`bindQualified\`.
A mere JSON report, IP address or UDP/OSC source port cannot satisfy
authentication or physical verification.

The model requires increasing per-generation observation sequences
and timestamps no older than a bounded freshness window (5 seconds
by default), neither in the future nor rolling backwards. It reads
\`FULL_PATH / CONTENTS / TYPE / ACCESS / VALUE\` only through the
previous Phase 5L parser. Any validated PATH_CHANGED / PATH_REMOVED /
PATH_ADDED / PATH_RENAMED affecting the target control or an ancestor
**invalidates** its published value until a fresh post-change snapshot
arrives. An unrelated path event does not alter the still-fresh value.

A lost connection clears the snapshot and rejects the old generation;
reconnect requires a strictly increasing generation, an exact
Project/Role/Snapshot/receiver instance, and a **new** snapshot.
Clock rollback invalidates the scope and requires a newer generation.
Both stale and malformed observations fail closed.

Every resulting \`view()\` unconditionally declares:
- \`authenticated_receiver: false\` — the offline decoder cannot prove it;
- \`application_receipt_confirmed: false\`;
- \`physical_video_output_verified: false\`;
- \`commands_enabled: false\`, \`automated_replay: false\`.

Only \`CURRENT_PUBLISHED_CONTROL_ONLY\` versus \`UNKNOWN\` is
reported; the diagnostic must not be fed into GO admission,
automatic Cue replay, blackout-clear or control commands.

The new tests \`vdmx_readback_session.test.js\` cover lifecycle,
scope mismatch, stale/future timestamps, old sockets, renamed/removed
paths, malformed JSON, unrelated path and rollback to unknown.

## Manual offline fixture inspection

A new stdin-only CLI lets a developer inspect a locally captured,
**sanitized** OSCQuery JSON tree without making any HTTP/OSC query:

\`\`\`sh
node tools/receiver-ack-lab/vdmx_readback_fixture_cli.js \
  /StageCore_Lab/layer_opacity < sanitized-oscquery.json
\`\`\`

Replace the path with the explicitly published Control Surface
property that you personally verified in a **separate test VDMX project**.
The CLI never automatically discovers a port/address, opens a socket,
fetches a URL or sends an OSC command; it refuses fixtures bigger than
64 KiB, and only prints an offline diagnostic. Because it operates
on a saved file, \`snapshot_current:false\` and
\`receiver_authenticated:false\` are always returned — even if the
value happens to match the expected video control.

The CLI's output is intentionally not accepted by
\`outputwatch.ReceiverReceiptTracker\`.

## Future VDMX/macOS gate (NOT done yet)

1. Create an isolated VDMX Control Surface and publish exactly one
   non-show test parameter using the documented OSCQuery option.
2. Use VIDVOX's browser/interface to confirm its exact FULL_PATH and
   whether changes really reflect an application output property.
   Sanitize a JSON snapshot for offline fixture inspection.
3. Require an authenticated, scope-bound Companion-side adapter
   before considering a **read-only** OSCQuery GET/LISTEN subscription.
   Observe the behavior on link failure, rename, reopen and macOS
   sleep/restart; do not silently carry the previous value forward.
4. Qualified application acknowledgment still needs a real VDMX-side
   report correlated with exact Cue/Execution and the Hub's trusted
   dispatch metadata, not just a matching Control Surface slider.

**No production connection, credentials, real VDMX plugin,
main merge, deployment or show operation was added.**
