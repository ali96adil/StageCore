# 09 — Security Model — v0.1

**Document Type:** Executable Security, Identity & Trust Specification  
**Status:** Initial implementation baseline  
**Based on:** 02 System Architecture + 04 Event & Command Contracts + 06 Plugin Contract + 07 Companion Specification + 08 Storage & Vault Specification

## Core Principle

StageCore is local-first, but the local network is **not** treated as automatically trusted. Users, Clients, Companions and Plugins receive authority only through explicit identity, authentication and permission checks enforced by the Hub.

Security must not depend on Internet or cloud identity. A stage network with no Internet must still support login, Companion pairing, Cue execution, Plugin permissions, secret access and audit.

The design favors practical controls that can be implemented and tested now: stable Hub identity, explicit first-owner bootstrap, authenticated user sessions, per-Companion keys, revocation, server-side authorization, encrypted/scoped secret storage, Plugin permissions and security audit records.

## Files

- [00 — Goals, Threats & Boundaries](00-goals-threats-and-boundaries.md)
- [01 — Hub Identity & First-Run Bootstrap](01-hub-identity-and-bootstrap.md)
- [02 — Users, Authentication & Authorization](02-users-authentication-and-authorization.md)
- [03 — Companion Pairing & Device Trust](03-companion-pairing-and-device-trust.md)
- [04 — Clients, Web/API & Transport Security](04-clients-web-api-and-transport-security.md)
- [05 — Secrets & Credential Store](05-secrets-and-credential-store.md)
- [06 — Plugin/Add-on Security & Permissions](06-plugin-addon-security-and-permissions.md)
- [07 — SHOW Mode Security Operations](07-show-mode-security-operations.md)
- [08 — Audit, Incident Response & Recovery](08-audit-incident-response-and-recovery.md)
- [09 — MVP Implementation Boundary](09-mvp-implementation-boundary.md)
- [10 — Acceptance Criteria](10-acceptance-criteria.md)

## Reference Trust Shape

```text
User / Web / Native Client
          | authenticated session
          v
      StageCore Hub
      /     |      \
 Plugin  Secret   Companion
 grants   Store   device key
                  + pairing
```

The Hub is the authorization authority. UI visibility alone is never considered a security control.