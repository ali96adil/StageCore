# Scoped LIVE lighting reconciliation (read-only contract)

Tracking: [#255](https://github.com/ali96adil/StageCore/issues/255).

## User-visible invariant

Every successful reconnect requires a **fresh** state observation, including when
the stage is still on Cue 5 and no additional GO was issued. The cue number,
last command ACK or historical state is not evidence of current output.

Example: current desired logical DMX channels `{1:180, 2:140, 3:60}`;
fresh device report `{1:180, 2:0, 3:60}` yields `DRIFT [2]`. If all
three match, report `MATCH`; neither result authorizes an output command.

## Source-only slice

`deviceexperience.CompareLiveLighting` is a pure, read-only comparison helper.
It checks exact Hub-assigned project/session/immutable snapshot, positive
assignment epoch and authenticated connection generation, and a fresh
caller-issued observation challenge. Before use, the transport must itself
authenticate and verify the actual connection, challenge issuance and freshness.
The Hub must supply desired values from its **current** committed LIVE state,
not from a stale cue cache. All callers must recheck that scope when applying
any future correction. `DesiredRevision` is Hub-owned, nonzero and must be
checked against the latest revision by an eventual dispatch coordinator.

`BLOCKED`: authority, scope, challenge or desired channel configuration
invalid. `UNKNOWN`: fresh reported logical values absent/partial.
`DRIFT`: list of only observed differing channels. `MATCH`: all requested
logical channels equal. Even MATCH is **not** independently verified physical
DMX/decoder/LED output.

No runtime handler invokes this helper yet. No automatic retries, device
commands, output enablement, snapshot activation, Pi deployment, firmware
flashing, or physical qualification are part of this slice.

## Fresh observation gate (stacked next source-only slice)

`deviceexperience.LiveLightingObservationGate` now issues cryptographically random,
bounded-time, one-use challenges for **every** reconnect, including unchanged
Cue 5. A new Begin invalidates any previous challenge for that device. The
socket owner must pass its exact challenge when cancelling, so delayed cleanup
from an old connection cannot erase the replacement's pending verification.
An expired response, duplicate response, old socket/assignment, cue change,
same-cue desired revision change or lost command authority is BLOCKED; no
optimistic MATCH or replay. Tests exercise 32 concurrent completion attempts
and require at most one matching result.

This is still **source-only**: there is no authenticated runtime message
handler, no live desired-state derivation, no v2 ACTIVE state, no correction
dispatcher and no physical DMX/LED proof. The transport must validate the
actual authenticated connection and the originating response before consuming
its challenge. Never use a pure comparison result as a READY flag or an
automatic permission to leave blackout.

## Opt-in authenticated software report transport (next source-only slice)

`devicechannel.ProbeV2SoftwareLevels` now has a bounded, fresh diagnostic
request/response on the **current authenticated v2 socket**, carrying Hub
assignment epoch, a durable connection generation, and a 32-byte random
challenge. The receiving path accepts a report only on the exact pending
connection, checks full 12-channel logical values and ranges, revalidates
the runtime credential and Hub-owned assignment, then labels the result as
software-only. A missing/late/unsolicited/malformed response fails closed;
timeouts close the old socket. A diagnostic probe and an assignment blackout
challenge cannot overlap.

By default, the request is **not** automatically sent on reconnect.
It requires `lighting.state_probe/1` capability advertisement, and automatic
reconnect diagnostics additionally require Hub opt-in
`STAGECORE_EXPERIMENTAL_V2_AUTO_PROBE=1`. This feature remains source-only
and unapproved for show hardware. Legacy v1 and the unchanged default v2
blackout-only firmware do not advertise the probe capability; only separate
firmware Draft #11's opt-in CI image does. The response cannot mark READY, change a
Project, activate a snapshot, dispatch any command, or verify physical DMX
decoder/LED values. An unexpected nonzero or non-blackout report is surfaced
as `UnsafeWhileUnactivated`, never corrected silently.

To meet the requested same-Cue-5 self-healing behavior, the next dependency
is firmware capability plus trusted, current LIVE desired-state derivation,
followed by a session/revision-bound `LiveLightingObservationGate` comparison,
separate v2 ACTIVE command authority, and explicitly approved safe partial
correction. Do not equate this raw diagnostic with a matched Cue.

## Batched reconnect guards (opt-in, source-only)

The Hub rejects any `device.hello` or `device.observation` write from a
displaced socket, and persists an old connection's OFFLINE transition under
the same lock as socket registration. The newer connection's ONLINE state
cannot be overwritten by late old-socket updates, including under the legacy
v1 protocol.

For an opted-in v2 node, automatic probing starts once per authenticated
reconnect: after the UNASSIGNED `assignment.state`, or for BLOCKED only
after the Hub has persisted the epoch ACK and sent the non-activating receipt.
The probe is fenced to the exact initiating socket and stores its
software-only 12-channel diagnostic in a generation-bound in-memory cache.
A reconnect invalidates the cache; reads return independent copies.
No diagnostic changes the Project, epoch, snapshot, `READY` state or output.
An unexpected nonzero/non-blackout report is flagged, not corrected.

This is **not yet live Cue-5 reconciliation**. The raw observation has no
published-snapshot/current-cue authority, and the experimental v2 node is
not ACTIVE. Comparing it to cached targets or replaying old GO commands is
unsafe. A current-state derivation and independently qualified v2 activation
are prerequisites to bounded partial corrections.

The Core workflow retains Go 1.26/1.27, race and ARM64 coverage. It now
cancels superseded runs for the **same PR**. Stage changes on an unreviewed
branch before opening the PR to avoid starting unnecessary runs.

## Follow-up gates

1. Connect authenticated versioned observation challenge/response to the Hub.
2. Derive current desired state from active session/snapshot with atomic
   generation/revision checks; never replay a historical GO.
3. Implement separate device-specific, opt-in, safe partial correction for
   activated devices only. BLOCKED/blackout-only v2 lighting stays zero.
4. Test Cue 5 same-cue reconnect, missed Cue 5→7 changes, stale observation,
   concurrent GO, missing channel, timeout/fade/failsafe, and actual physical
   DMX/LED measurements. Maintain canonical Phase C qualification evidence.

Do not merge this stacked draft independently of the #254 v2 foundation,
and do not describe a source-only comparison as a complete self-healing system.
