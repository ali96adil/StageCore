# 02 — Users, Authentication & Authorization

## User Roles — v0.1

Reference built-in roles:

| Role | Typical authority |
|---|---|
| `OWNER` | full local administration, users, trust, security recovery |
| `TECHNICIAN` | edit/configure, publish, pair Companions, manage Plugins outside SHOW |
| `OPERATOR` | enter Rehearsal/SHOW, GO/STOP/Jump according to policy, Notes and runtime inspection |
| `VIEWER` | read-only status/runtime/session views |

Roles are convenience bundles over permissions. The Hub enforces permissions on every protected operation; hiding a button in the UI is not sufficient.

## Minimum Permission Families

- `project.read`
- `project.edit`
- `snapshot.publish`
- `runtime.control`
- `show.enter_exit`
- `companion.pair`
- `companion.revoke`
- `plugin.manage`
- `secret.manage`
- `user.manage`
- `backup.restore`
- `audit.read`

## Password Authentication

The MVP can use local username/password accounts with:

- passwords stored only as a modern salted password hash (Argon2id preferred where supported);
- no plaintext/reversible password storage;
- basic failed-login rate limiting/backoff;
- explicit logout/revocation;
- optional password change by the user and reset by OWNER through a controlled local flow.

Cloud identity is not required.

## Browser Sessions

Reference Web sessions use opaque or signed session identifiers in cookies with `HttpOnly`, `Secure` when HTTPS is active and appropriate `SameSite` policy. State-changing browser requests require CSRF protection appropriate to the selected framework.

Sessions record user ID, issue/expiry time and revocation state. Disabling a user or revoking sessions prevents new protected operations.

## Native Clients

Native macOS/iOS clients authenticate as a user through the same Hub authorization model. Long-lived refresh credentials, if used, belong in OS secure storage such as Keychain; access tokens are short-lived and scoped to the authenticated user.

## Runtime Rule

Every protected Command reaching Core has an authenticated issuer identity. Core authorization is evaluated before the Command changes authoritative state or dispatches runtime work.