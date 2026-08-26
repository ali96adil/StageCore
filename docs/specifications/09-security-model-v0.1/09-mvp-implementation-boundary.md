# 09 — MVP Implementation Boundary

## MUST Implement

The first real Security slice must prove:

1. persistent Hub ID + asymmetric identity key;
2. restricted first-run OWNER bootstrap with one-time setup code;
3. local user/password login with secure password hashing;
4. `OWNER / TECHNICIAN / OPERATOR / VIEWER` server-side authorization;
5. authenticated Web/API session and authenticated realtime channel;
6. Companion asymmetric identity, explicit pairing and revocation;
7. trusted Hub fingerprint mismatch detection for native/Companion connections;
8. dedicated encrypted Secret Store with redaction;
9. Plugin manifest permission grant/enforcement sufficient for the OSC reference Plugin;
10. security audit records for login/trust/permission/secret administration;
11. SHOW-mode blocks for dangerous administration plus explicit emergency revocation;
12. security acceptance/failure tests with Internet disconnected.

## Reference Implementation Order

```text
SEC0 Hub identity + first OWNER
 -> SEC1 User auth + authorization middleware
 -> SEC2 Web/realtime authenticated sessions
 -> SEC3 Companion keys + pairing + revocation
 -> SEC4 Secret Store + redaction
 -> SEC5 Plugin permission enforcement
 -> SEC6 Security audit + SHOW gates
```

Each slice must include automated/repeatable denial-path tests, not only successful login/pairing tests.

## Practical Technology Direction

Use mature platform/framework primitives rather than writing cryptography from scratch:

- maintained TLS implementation;
- maintained password hashing library, Argon2id preferred;
- standard asymmetric signing/key libraries;
- OS secure storage where available;
- established session/CSRF middleware where suitable.

Exact backend language/framework is still open; the security behavior must survive that choice.

## Explicitly Deferred

- cloud SSO/OAuth provider dependency;
- LDAP/SAML/enterprise identity federation;
- remote Internet access broker;
- hardware security module requirement;
- TPM-backed Hub identity as a hard dependency;
- public third-party Plugin signing PKI/marketplace;
- biometric/passkey-only account model;
- production certificate automation for every possible venue network.

These may be added later without weakening the v0.1 local trust model.

## Guardrail

Do not postpone the MVP to build an enterprise IAM platform. Conversely, do not ship the stage-control loop with a shared admin password in plaintext or unauthenticated LAN APIs just because the network is local.