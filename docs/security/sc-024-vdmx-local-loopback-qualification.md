# SC-024 Phase 5N — Disposable VDMX 6 localhost OSCQuery GET qualification

**2026-10-19 · Draft integration PR #530 · developer tool only. Not installed on the macOS Companion, Hub, Raspberry Pi or active show.**

## Purpose and trust limitation

StageCore's existing OSC **input** at port 3555 is not the OSCQuery
readback endpoint. The OSCQuery proposal specifies **HTTP GET** for a
published node's FULL_PATH. A response can report the value of a
published VDMX Control Surface property. It does **not** prove the
running server is actually VDMX, that an OSC Cue caused the change,
that the video rendered, or that the input is authenticated.
No StageCore GO/STOP/blackout/receiver acknowledgment integration is
added by this lab.

**Source:** [Vidvox OSCQuery proposal](https://github.com/Vidvox/OSCQueryProposal)
and [VDMX OSCQuery documentation](https://docs.vidvox.net/vdmx/vdmx_oscquery).

\`tools/receiver-ack-lab/vdmx_loopback_read.js\` is an explicitly
opt-in, one-shot local diagnostic (no automatic polling). It:

- Requires \`STAGECORE_VDMX_DISPOSABLE_TEST_PROJECT=1\`, a manually
  specified high-numbered port (1024–65535) and the exact published
  control path. It has **no default OSCQuery port**.
- Contacts **only** \`127.0.0.1\` over HTTP, using a single GET, with
  an overall 1.8-second deadline. Does not take a user URL, hostname,
  DNS target or proxy; cannot follow an HTTP redirect.
- Requires a direct 200 JSON reply with the **exact requested
  FULL_PATH** (not a substituted root/other-tree response), refuses
  compressed/oversized responses and bounds advertised and received
  reply bytes to 64 KiB. Rejects unsupported scalar TYPE or invalid
  VALUE and an explicitly unreadable ACCESS (0 or 2).
- The OSCQuery specification makes ACCESS optional: when omitted,
  a well-typed, explicitly published single VALUE remains readable.
  This applies to diagnostic GET only, never sending or writing.
- Prints only a narrow diagnostic: property path, reported value,
  local capture timestamp and **all** trust/execution/physical-output
  and control-authority flags set to false.
- Never opens UDP, WebSockets, a Hub channel or a listener, sends a Cue,
  writes to VDMX, discovers local programs/ports, or modifies a show.

The \`captured_at_utc\` timestamp is the **developer machine's clock**
when the request completed, not a device signed report time. The tool
never claims \`monitoring_active\`, \`receiver_authenticated\`, or
\`historical_cue_execution_confirmed\`.

## Optional attended macOS test (NOT against the show)

1. Open a **disposable VDMX 6 project** on a development Mac. Do not
   open or change the live show file. Disconnect this test project
   from active stage routing, speakers and external outputs.
2. Create a simple Control Surface UI control. Publish that control
   using VDMX's OSCQuery configuration, and identify the **separate
   HTTP OSCQuery port and exact published FULL_PATH** from the app's
   documented browser/configuration UI. Do not assume port 3555.
3. Confirm the VDMX test OSCQuery HTTP endpoint is bound to localhost
   or otherwise not exposed to the theater LAN. If this cannot be
   confirmed, do **not** enable it; no test needs an external bind.
4. In a Terminal on that **same development Mac**, from an isolated
   StageCore checkout, substitute your verified port and path:

\`\`\`bash
STAGECORE_VDMX_DISPOSABLE_TEST_PROJECT=1 \
  node tools/receiver-ack-lab/vdmx_loopback_read.js \
  --port <OSCQUERY_TEST_PORT> \
  --path /StageCore_Lab/layer_opacity
\`\`\`

The path above is **only an illustrative lab name**, not a confirmed
path present in the user's VDMX. Replace it with the FULL_PATH actually
published by the local test project. Never paste tokens, live project
metadata or unrelated media into the chat/repository.

5. Change **only the local test Control Surface value** in VDMX and
   manually run the one-shot GET again. Confirm the read value changes.
   Also test disabling/unpublishing the control; the command must
   return UNKNOWN, not the previous value. Closing test VDMX must
   also return UNKNOWN.
6. After the experiment, turn off test OSCQuery publishing and close
   the disposable test project. No standing monitor should remain.

This test is deliberately manual and allows only local GETs. Even a
value that tracks the Control Surface is **not enough to claim that
VDMX received a specific Cue or rendered a physical image**. That
requires a separately authenticated receiving-app adapter, verified
runtime/snapshot/Cue/Execution correlation, replay fencing and
an output-specific state readback. A bare OSCQuery GET cannot satisfy
these requirements.

## Phase 5P — one interactive command to compare two manual values

\`tools/receiver-ack-lab/vdmx_manual_compare.js\` reuses the validated
\`getLoopbackControl\` helper, without copying another HTTP client.
It is a **manual, disposable-project-only, two-GET comparison**, not
continuous monitoring and not a receiver ACK. No new server or
authorization route was added.

Before running it, use VDMX's documented OSCQuery browser to establish
the actual **loopback HTTP TCP port** and published control FULL_PATH.
Never substitute the show's OSC input port 3555 or use a production
VDMX project. The same explicit environment opt-in applies:

\`\`\`bash
STAGECORE_VDMX_DISPOSABLE_TEST_PROJECT=1 \
  node tools/receiver-ack-lab/vdmx_manual_compare.js \
  --port YOUR_TEST_OSCQUERY_TCP_PORT \
  --path /Your_Disposable_Control_Surface/Test_Slider
\`\`\`

On an **interactive macOS terminal**, the tool performs the first GET,
asks you to change only the disposable slider in VDMX, then performs
the second GET **only when you press Enter**. It will not automatically
send MIDI, OSC, POST, SET, Cue, or GO. It never discovers a host or
port; it uses the same 127.0.0.1-only allowlist and HTTP-size/time
limits as \`vdmx_loopback_read.js\`. The manual comparison window is
capped at **two minutes**, and the terminal is required; piped or
unattended use is rejected.

The result explicitly distinguishes:
- \`PUBLISHED_CONTROL_VALUE_DIFFERED\`: the two reported software
  values differed after the operator's manual intervention;
- \`NO_VALUE_DIFFERENCE_OBSERVED\`: the two readings were identical;
- \`INCONCLUSIVE_TYPE_CHANGED\`: the published scalar type changed,
  so the observations are not comparable.

If **either** read fails, if the required human confirmation is
missing, if the local clock goes backwards, or if the two-minute
window expires, **no comparison is published**; the tool reports
UNKNOWN. All positive results still force
\`receiver_authenticated:false\`,
\`application_cue_acknowledged:false\`,
\`change_caused_by_stagecore_verified:false\`,
\`physical_video_output_verified:false\`,
\`commands_enabled:false\` and \`automatic_replay:false\`.

Why the restrictions: an unauthenticated local process can impersonate
the test OSCQuery service, a human may move the slider, and even a
correct VDMX Control Surface slider is not an independent camera/screen
verification. Two matching values do not prove that the video is
visible; two different values do not prove a StageCore Cue caused them.

Unit tests use a **synthetic loopback HTTP server** and a suspended
confirmation callback. They assert only one GET occurs before the
manual action, exactly one more occurs afterward, and cancellation,
bad source, mismatched path, changed type, unsafe port/URL and clock
problems fail closed.

Offline development verification:

\`\`\`bash
node --check tools/receiver-ack-lab/vdmx_manual_compare.js
node --test tools/receiver-ack-lab/vdmx_manual_compare.test.js
\`\`\`

**This has not been executed against VDMX 6 on the user's Mac.**
No StageCore Hub/Companion/production environment changes.

## Phase 5O compatibility and direct-call hardening

The source-only audit added two compatibility/security guards before an
attended VDMX trial:

- Both the CLI and the exported `getLoopbackControl` helper independently
  validate **an integer TCP port 1024–65535** and an explicit,
  ASCII-only published OSCQuery FULL_PATH. A developer importing the
  helper directly cannot bypass the port/path restrictions and make
  an accidental request to port 80 or to an arbitrary URL. Its HTTP
  destination remains hard-coded to `127.0.0.1`. This does not
  authenticate the local process answering on the port.
- The OSCQuery proposal permits a scalar `VALUE` when appropriate.
  This laboratory decoder now accepts **one scalar** or a
  **one-element array** for a single supported `TYPE`, while rejecting
  multi-argument arrays, unsupported types, write-only ACCESS, and
  non-finite numeric values. Optional ACCESS remains readable when
  a VALUE is actually published. Nothing enables SET/POST or OSC send.

Additional localhost fake-server tests verify that invalid internal
API arguments cause **zero network requests**, and that a single
scalar JSON VALUE remains a mere published software-control report.
No sound/video/receiver confirmation can be derived from this.

## Offline test matrix

\`vdmx_loopback_read.test.js\` uses a fake local \`127.0.0.1\` HTTP
server in the CI runner, **not a real VDMX server**. It exercises:

- Missing disposable-project opt-in; rejection of hostile
  host/URL/path/port arguments.
- One request with method GET and exact path; honest false
  receiver/execution/video/command flags.
- No redirect following, invalid path or content type, malformed
  JSON, compressed response and oversized payload.
- Connection refused returning UNKNOWN, no fallback to remote IP.
- Exact-method FULL_PATH rather than substituted root tree, absent
  optional ACCESS with typed VALUE, and declared oversized Content-Length.

Run the source-only verification without VDMX:

\`\`\`bash
node --check tools/receiver-ack-lab/vdmx_loopback_read.js
node --test tools/receiver-ack-lab/vdmx_loopback_read.test.js
\`\`\`

**No main merge, no Pi deployment, no StageCore Companion/VDMX show changes,
no Project ACL activation and no physical actuation.**
