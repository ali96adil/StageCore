# 08 — Audit, Incident Response & Recovery

## Security Audit Record

Security-sensitive actions create append-oriented audit records with at least:

- audit event ID/type;
- timestamp;
- authenticated actor identity;
- source Client/Companion where relevant;
- affected identity/resource;
- result: success/rejected/failed;
- reason/category;
- correlation ID where tied to a Command;
- sanitized metadata.

Audit records never contain passwords, private keys, bearer tokens or raw secret values.

## Events to Audit — MVP

- login success/failure and logout/session revocation;
- user creation/disable/role change;
- Hub claim/bootstrap recovery;
- Companion pairing approval/rejection/revocation;
- Hub/Companion identity mismatch;
- Plugin permission grant/revoke and install security status;
- secret create/update/delete and authorized use metadata;
- denied privileged API/Command attempts;
- emergency security change during SHOW;
- backup/restore actions involving security state.

## Operator Visibility

OWNER/authorized TECHNICIAN can inspect security audit records from StageCore UI without reading raw server logs. Normal OPERATOR runtime history remains separate from verbose security diagnostics.

## Incident Flows

### Lost/Stolen Companion

1. revoke Companion identity;
2. affected role becomes unavailable/degraded;
3. pair replacement machine;
4. assign existing Machine Role;
5. sync/Preflight;
6. old credentials remain rejected.

### Compromised User

1. disable account/revoke sessions;
2. review audit trail;
3. rotate affected secrets if necessary;
4. create/restore authorized operator access.

### Suspected Hub Identity Loss

Do not automatically regenerate keys and pretend nothing changed. Enter explicit recovery, preserve evidence/backups, restore known identity if protected recovery material exists or establish a new identity and re-pair/re-trust endpoints.

## Retention

Exact long-term audit retention is a later policy decision, but MVP must preserve security audit records across normal restart and ordinary Project edits.

## Time

Audit uses Hub-observed timestamps. Clock drift should be visible operationally; Internet NTP cannot be a mandatory security dependency.