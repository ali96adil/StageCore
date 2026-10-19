# SC-024 Phase 5N — One-shot local VDMX OSCQuery GET qualification

**2026-10-19 — Draft PR #530; single explicit local read only, no Hub or show device integration.**

## Why this is the next step

The offline Phase 5L/5M parsers and state-machine tests can read
published JSON but cannot verify that VDMX 6 actually serves a
particular Control Surface item on the user's Mac. OSCQuery specifies
HTTP/1.1 GET requests for an exact method address; VDMX offers
OSCQuery publishing from explicitly enabled Control Surface plugins.

Official protocol / VDMX references:
- https://github.com/Vidvox/OSCQueryProposal
- https://docs.vidvox.net/vdmx/vdmx_oscquery
- https://vdmx.vidvox.net/tutorials/oscquery-in-the-control-surface-plugin

The candidate \`tools/receiver-ack-lab/vdmx_loopback_probe.js\` is a
one-shot opt-in **read-only** tool to run separately on a development
Mac. It makes exactly one HTTP GET to **127.0.0.1** on an explicitly
entered numeric TCP port, requesting an explicitly entered OSCQuery
FULL_PATH. There is:
- NO server discovery, bonjour, DNS, remote IP, URL, username,
  password, TLS downgrade, redirect, query string, HTTP write,
  WebSocket, OSC, MIDI, GO, retry, auto-replay or background polling.
- Fixed 2.5-second deadline and 64 KiB response size; HTTP 200,
  JSON content type and exact echoed FULL_PATH are mandatory.
- An offline-only diagnostic response with
  \`connection_scope_verified:false\`,
  \`application_cue_acknowledged:false\`,
  \`physical_video_output_verified:false\`,
  \`commands_enabled:false\` and \`automated_replay:false\`
  regardless of the published value.

A GET response is a point-in-time VDMX *published software control*
report only. It does **not** authenticate the VDMX process, establish
the StageCore Project/Session/Cue/Execution identity, prove OSC was
received, or certify visible stage video. A malicious or unrelated
local program could answer the same loopback port, so these diagnostics
must never be ingested as trusted runtime state.

## Manual lab procedure — ONLY on a disposable VDMX project

1. Leave the theater/production VDMX project, actual show input OSC
   port (previously 3555), Companion, StageCore Pi, QLab/Live,
   lighting, lasers and show devices untouched.
2. Open a **separate** VDMX 6 workspace on the Mac, and create a
   Control Surface with one harmless test slider (e.g. opacity).
   In that plugin's Inspector, enable **OSCQuery Enabled?**
3. In VDMX Preferences → OSC → OSCQuery, locate the address/port of
   the **HTTP OSCQuery endpoint** published by the application.
   This port is not assumed to be the show's UDP OSC input (3555).
   Prefer the documented OSCQuery Browser or built-in web controller
   to inspect the actual FULL_PATH for the slider and verify its
   TYPE/ACCESS/VALUE.
4. On the **same Mac** using a developer checkout, run the following
   with the ACTUAL numeric OSCQuery TCP port and real published
   FULL_PATH (replace both placeholders):

\`\`\`sh
node tools/receiver-ack-lab/vdmx_loopback_probe.js \
  YOUR_OSCQUERY_TCP_PORT /Your_Control_Surface/Your_Test_Slider
\`\`\`

5. Change the slider **manually** inside the disposable VDMX project,
   then run the exact command once again. Compare both returned
   software control values. Do not connect a StageCore Cue or live
   OSC command to this test. Do not upload or share raw VDMX project
   files or private network/Host Info metadata.
6. If the endpoint is not on IPv4 loopback, or if the application
   requires network, discovery or credentials, the probe deliberately
   refuses to continue. Treat this as **not qualified**, not as a
   reason to turn on a remote server or modify the stage router.
7. Close the separate test project when done. Restore no production
   state because none was altered. Do not enable a live monitor,
   receiver ACK, auto-replay or new OSC routes from this procedure.

## Protocol compatibility adjustment

The OSCQuery proposal defines the \`ACCESS\` mask as optional. If it
is omitted, a well-typed single published \`VALUE\` is considered
readable. Our Phase 5L parser now follows this rule while refusing
explicit \`ACCESS:0\`, \`ACCESS:2\` (write-only), malformed access,
missing VALUE and unsupported types. The parser NEVER uses that
compatibility rule as authorization to send a value: **GET only**.

## Coverage

CI tests start a synthetic \`127.0.0.1\` HTTP server on a random
ephemeral port and verify a single explicit GET, exact FULL_PATH,
wrong response path, HTTP redirect rejection, wrong content type,
malformed/oversized JSON, stalled timeout, unsafe URL/port rejection
and truthful diagnostics. These tests touch no actual VDMX instance.

A separate read-only authenticated application adapter, a verified
VDMX property change and Cue/Execution correlation would still be
required before any production receiving-app ACK could be claimed.

**Draft only. No main merge, Pi update, Companion/Max/VDMX show update,
production network action or physical output changes.**
