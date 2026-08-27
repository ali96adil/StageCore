# 07 — Security Regression & Trust Failure Tests

## Required Security Regression Areas

Every release that changes authentication, networking, Plugins, Companion or Vault behavior reruns tests for:

- first OWNER bootstrap cannot be repeated;
- invalid login/rate limiting;
- server-side role denial;
- revoked session rejection on API/realtime channel;
- Companion pairing approval and revocation;
- Hub fingerprint mismatch rejection;
- Plugin permission denial;
- Secret Store authorization and redaction;
- SHOW-mode administration blocks;
- security audit persistence.

## Network Trust Test

Join an unpaired/unauthenticated machine to the same stage LAN.

Expected:

- it may discover public Hub presence if discovery is enabled;
- it cannot read protected Project state or issue runtime Commands;
- it cannot become a Companion without explicit pairing;
- LAN membership does not grant admin authority.

## Credential / Secret Leakage Tests

Automated or repeatable scans inspect:

- normal application logs;
- Event/Action traces;
- Project export;
- backup manifest metadata;
- Plugin error output;
- browser/API error responses.

Synthetic passwords/tokens/private keys must not appear where the Security Model forbids them.

## Revocation During Runtime

In a controlled Rehearsal/SHOW test:

- revoke one client session;
- revoke one Companion identity.

Expected:

- authorization stops immediately for the revoked identity;
- affected role/readiness becomes degraded/offline;
- unrelated trusted runtime components continue where safe;
- revocation is audited;
- no previously accepted non-idempotent command is replayed to compensate.

## Security Failure Rule

A test proving unauthorized state-changing control, secret leakage, trust bypass or ineffective revocation is release-blocking until fixed or the exposed capability is removed from the milestone.