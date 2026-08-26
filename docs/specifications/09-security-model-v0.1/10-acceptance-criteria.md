# 10 — Acceptance Criteria

The Security Model v0.1 is implementation-ready when these behaviors can be demonstrated repeatably on the reference Hub + browser + macOS Companion setup.

## Hub Bootstrap & Users

- Fresh Hub starts `UNCLAIMED` and does not expose normal protected control as an anonymous admin.
- One-time local setup code can create the first OWNER exactly once.
- Reusing/expired setup code fails.
- OWNER can create an OPERATOR and VIEWER.
- OPERATOR can execute permitted runtime controls but cannot manage users/secrets/Plugins.
- VIEWER cannot issue `cue.go` or other state-changing runtime Commands.
- Permission denial occurs server-side even when the API is called directly without UI.

## Sessions & Transport

- Valid login creates an authenticated session.
- Invalid login is rate-limited/backed off and does not reveal password details.
- Logout/account disable/session revocation prevents subsequent protected requests.
- Realtime connection requires authentication and closes/rejects after revocation.
- Internet disconnection does not break local authentication/runtime.

## Companion Trust

- New Companion generates its own identity/key and cannot execute before pairing.
- Explicit approved pairing creates a trusted device.
- Reconnect proves the same device identity.
- Revoked Companion cannot regain authority with its old key/session.
- A known address presenting a different Hub fingerprint produces `IDENTITY MISMATCH` rather than automatic trust.
- Reconnect never replays the previous non-idempotent Action.

## Secret Store

- Create one test secret and reference it from a capability/integration without putting the value in Project configuration.
- Stored secret is not plaintext in the normal database/config/export representation.
- Unauthorized user/Plugin cannot read/use the secret.
- Authorized integration can use the specific granted secret.
- Secret value does not appear in normal logs, errors, Event traces or Project export.

## Plugin Permissions

- OSC Plugin declares `network.udp.send` before activation.
- Execution succeeds when permission is granted.
- Removing/denying the permission prevents new OSC execution with a clear unavailable/permission result.
- Plugin cannot use an undeclared protected Secret/filesystem permission.
- Unsigned development package is visibly marked as development/unverified rather than production-trusted.

## SHOW Mode

- Enter SHOW with an authorized OPERATOR and valid required trust state.
- New pairing, Plugin permission expansion, normal user administration and secret rotation are blocked by default.
- Existing authorized Cue execution continues locally.
- OWNER emergency Companion/session revocation is possible with explicit warning and creates degraded/readiness state plus audit event.

## Audit & Recovery

- Login failure, role change, Companion pairing/revocation, Plugin permission change and secret update produce inspectable audit records.
- Audit records contain actor/result/timestamp but no secret values/private keys.
- Restart preserves users, trust state and audit history.
- Restore/security recovery never silently changes Hub identity while pretending existing trust is still valid.

## Reference End-to-End Scenario

```text
Fresh Hub
 -> obtain local setup code
 -> claim Hub + create OWNER
 -> create OPERATOR
 -> install/authorize OSC Plugin
 -> store one scoped integration secret
 -> pair Mac Companion
 -> assign VIDEO-MAIN
 -> publish + Preflight
 -> OPERATOR starts Rehearsal/SHOW and executes Cue
 -> unauthorized Viewer/API action is rejected
 -> revoke Companion
 -> old Companion reconnect is rejected
 -> pair replacement Mac
 -> review audit trail
```

Passing this scenario proves StageCore is locally usable without turning the stage LAN into an implicit admin boundary.