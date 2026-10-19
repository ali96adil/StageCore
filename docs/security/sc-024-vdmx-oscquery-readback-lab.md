# SC-024 Phase 5L — Offline VDMX 6 OSCQuery published-control readback lab

**2026-10-19 · PR #530 Draft · no VDMX connection, no WebSocket listener, no output commands.**

## Evidence and scope

VDMX 6 supports the OSCQuery protocol. Its **Control Surface plugin**
can publish explicitly enabled local UI controls as OSCQuery nodes.
The OSCQuery proposal describes a JSON address tree with FULL_PATH,
CONTENTS, TYPE, ACCESS and VALUE. Published parameters may be used by
external clients and, when available, may provide bidirectional value
updates. These reports describe the state of the published *controls*,
NOT the actual video signal coming out of a display.

Official documentation:
- https://docs.vidvox.net/vdmx/vdmx_oscquery
- https://vdmx.vidvox.net/tutorials/oscquery-intro
- https://github.com/Vidvox/OSCQueryProposal

The existing StageCore VDMX OSC **input** port used in the show (3555)
does not magically expose a published Control Surface state. A VDMX
OSCQuery URL/port, a *specific* published Control Surface item, and a
proof that this item actually tracks the requested VDMX state must
first be verified on a **separate VDMX test project**. Do not infer
that a legacy OSC destination is a readback property.

## Added no-network JSON decoder

\`tools/receiver-ack-lab/vdmx_oscquery_readback.js\` exports
\`readVDMXPublishedControl(snapshot, targetPath)\`. It accepts a
previously captured OSCQuery-compatible JSON tree in memory:
- Requires the explicit exact OSCQuery FULL_PATH through the nested
  CONTENTS tree. Does not fuzzy-match names or scan other projects.
- Requires a readable ACCESS bit, single VALUE, supported scalar TYPE,
  and finite number / bounded string / appropriate boolean value.
- Caps the serialized fixture to 64 KiB, nesting to 12 levels and
  traversal to 256 nodes. Disallows malformed or ambiguous paths.
- Returns \`evidence:"PUBLISHED_CONTROL_VALUE_ONLY"\`,
  \`authenticatedReceiver:false\`,
  \`actualVideoOutputVerified:false\`,
  \`commandsEnabled:false\` **unconditionally**.
- Contains no fetch, HTTP/WebSocket, OSC listener, MIDI or packet send.
  It never calls the signed receiver acknowledgment codec.

\`vdmx_oscquery_readback.test.js\` checks valid read-only and
read-write published controls, TYPE/VALUE pairs, foreign/malformed
FULL_PATH, missing/unpublished paths, write-only controls, oversized
input and, importantly, that a self-contained readback **cannot**
create any StageCore APP_REPORTED_RECEIVED or APP_REPORTED_APPLIED
receipt on its own. Node CI tests run without VDMX on a Mac.

## What must happen before real VDMX verification

1. In an isolated VDMX 6 project, add a **Control Surface** plugin with
   one test control and enable OSCQuery publishing for the plugin.
   Don't attach it to lighting, live outputs or the current show.
2. In VDMX Preferences → OSC / OSCQuery, inspect the application's
   advertised OSCQuery address and locally observe its published node
   using VDMX's OSCQuery Browser or browser controller. Record just
   the **sanitized FULL_PATH, TYPE, ACCESS and VALUE**, never API
   secrets, machine addresses or project media.
3. Verify whether the control changes when an actual target output
   property changes **within VDMX**. A published UI slider that
   doesn't follow the target output is only a controller, not a
   receiver-side acknowledgment.
4. Before adding any automatic polling or WebSocket subscription,
   bind a local, authenticated readback adapter to an explicit
   receiver instance, Project, Session, Snapshot, role and Cue
   Execution. Deny replay/old generations and timestamps. Do not
   elevate a direct OSC sender echo into trusted state feedback.
5. Only then consider a qualified APP_REPORTED_APPLIED receipt,
   still without making claims about actual projected pixels,
   visible stage video or audience safety.

The active StageCore Companion/Hub, current VDMX 3555 input,
Ableton Live, Raspberry Pi, Tablets and physical outputs are all
untouched. No main merge or deployment. Green CI proves the **offline
parser**, not the VDMX 6 program or OSCQuery source.

## Safe developer test

\`\`\`bash
node --check tools/receiver-ack-lab/vdmx_oscquery_readback.js
node --test tools/receiver-ack-lab/vdmx_oscquery_readback.test.js
\`\`\`
