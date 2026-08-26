# 03 — Pairing & Trust

## Goal

Pairing creates an explicit trusted relationship between one Companion identity and one Hub.

## MVP Pairing Flow

```text
Companion discovers/selects Hub
 -> Hub shows short-lived pairing code/request
 -> operator approves
 -> Companion receives trusted identity/credential
 -> Hub records Companion as PAIRED
```

The exact certificate/key mechanism is finalized in 09 — Security Model, but the product behavior is fixed here.

## Required Rules

- unpaired Companion cannot execute Project runtime commands;
- pairing requires explicit operator approval;
- pairing requests expire;
- trust is tied to stable Companion identity, not current IP;
- Hub can revoke a Companion;
- revoked credentials cannot regain authority by reconnecting;
- secrets/credentials are stored using platform secure storage where available;
- no pairing secret appears in normal logs.

## Reinstall / New Machine

A reinstall that loses trusted machine identity is treated as a new pairing unless a deliberate secure migration mechanism exists later.

## SHOW Mode

New pairing, revocation and trust migration should be blocked or strongly restricted during SHOW mode in v0.1.

## Audit

Hub records at minimum:

- pairing requested;
- approved/rejected;
- Companion ID/name/version;
- paired timestamp;
- revoked timestamp/reason if applicable.