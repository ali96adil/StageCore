# 07 — SHOW Mode Security Operations

## Goal

Security administration must not create avoidable instability during a live show, but an actual compromised identity must still be containable.

## Blocked by Default During SHOW

- new Companion pairing;
- normal user/role administration;
- Plugin/Add-on install/uninstall/permission expansion;
- incompatible software/Plugin upgrade;
- secret rotation that would alter active runtime dependencies;
- Hub identity/bootstrap reset;
- restore/replace operations;
- bulk export of protected Project/backup data.

## Runtime Operations That Continue

Authenticated OPERATOR/TECHNICIAN users retain the runtime controls permitted by the active Published Snapshot and Project policy, including GO/STOP/Jump as defined by the product spec.

Existing trusted Companions continue using their established bounded runtime sessions while valid.

## Emergency Revocation

An OWNER must be able to revoke a user session or Companion even during SHOW when compromise is suspected. This is an explicit high-impact action:

- show a clear warning about affected Machine Role/runtime;
- require strong confirmation/recent OWNER authorization where practical;
- immediately invalidate the target identity/session;
- mark affected readiness/degraded state;
- write an audit record.

Security does not keep a known-compromised device active merely to preserve a green READY indicator.

## Session Expiry

The runtime UI should avoid surprising an active operator with an avoidable mid-show logout. Before entering SHOW, Preflight verifies that the operator has a valid authorized session. Session renewal is local and must not require Internet.

Revocation, account disable or trust failure still takes effect immediately.

## Network Change

A change of IP/route alone does not destroy trust. Reconnection verifies the same Hub/Client/Companion identities before restoring authority.

## Preflight Security Checks

SHOW Preflight should include at least:

- active operator authenticated and authorized;
- Hub security state healthy/claimed;
- required Companions trusted and not revoked;
- required Plugins active with required permissions;
- required secrets present/readable by authorized Core path;
- no known identity/snapshot mismatch blocker.