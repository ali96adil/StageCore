# 11 — Acceptance Criteria

The Companion Specification v0.1 is implementation-ready when the reference macOS Companion can pass the following repeatable tests.

## Discovery & Pairing

- New Mac can reach the Hub by local discovery or manual host fallback.
- Companion verifies the selected Hub identity.
- Unpaired Companion cannot execute Project runtime commands.
- Explicit pairing creates a trusted Companion identity.
- Revocation prevents later reconnect from restoring authority.

## Role & Sync

- Operator assigns `VIDEO-MAIN` to one trusted Companion.
- Companion receives only the required role/runtime configuration subset.
- Applied Runtime Snapshot ID/config hash is reported to Hub.
- Snapshot mismatch prevents READY.
- Required capabilities missing from the machine produce a clear blocker.

## Execution

- A Cue targets `VIDEO-MAIN`, not the Mac hostname.
- Hub dispatches one Action through the Companion.
- Companion returns explicit result and truthful acknowledgement level.
- Failure/timeout is recorded in the matching ActionExecution.
- Reconnect does not replay the previous Action.

## Media & Bootstrap

- StageCore Web exposes a downloadable macOS Companion/app package from the Hub repository.
- Package metadata identifies version/platform/architecture/compatibility/checksum.
- A required role media manifest detects missing/mismatched content.
- Completed media sync verifies checksum before reporting ready.
- Bulk software/media transfer can be paused/throttled during SHOW.

## Health & Replacement

- Disconnect changes the role from READY within the configured heartbeat timeout.
- Reconnect reconciles identity/snapshot/capabilities before returning READY.
- A replacement Mac can pair, receive `VIDEO-MAIN`, sync and become READY.
- No Cue or Route definition must be edited to perform that replacement.

## Reference End-to-End Scenario

```text
New Mac
 -> open stagecore.local
 -> download StageCore for macOS
 -> install/launch Companion
 -> discover/select Hub
 -> pair
 -> assign VIDEO-MAIN
 -> sync role config + required media
 -> Preflight PASS
 -> start Rehearsal
 -> GO Cue targeting VIDEO-MAIN
 -> Companion executes Action
 -> result appears in Hub Session Log
 -> disconnect/reconnect test
 -> no duplicate Action
```

Passing this scenario proves the Companion is replaceable infrastructure integrated with the Hub rather than a second independent show-control application.