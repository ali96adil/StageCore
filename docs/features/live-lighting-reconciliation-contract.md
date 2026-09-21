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
