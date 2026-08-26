# 03 — Companion Pairing & Device Trust

## Device Identity

Each Companion creates a stable `companion_id` and an asymmetric device key pair on first launch. The private key never leaves the machine and should use platform secure storage; on macOS this means Keychain where practical.

## Pairing Flow — Reference MVP

```text
Companion discovers/selects Hub
 -> Companion sends identity + public key + nonce
 -> Hub creates short-lived pairing request/code
 -> authenticated TECHNICIAN/OWNER verifies machine details and approves
 -> Hub records Companion public key as trusted
 -> Companion receives trust credential/session bootstrap material
 -> Companion becomes PAIRED
```

Pairing authority is tied to the device key, not IP address or hostname.

## Reconnect Authentication

A paired Companion proves possession of its private key through challenge/response or an equivalent cryptographically authenticated handshake, then receives a bounded authenticated runtime session.

The exact transport library may evolve, but reconnect must not rely only on a reusable plaintext bearer secret stored in normal config.

## Revocation

Hub records Companion trust state:

- `UNPAIRED`
- `PAIRED`
- `REVOKED`

Revocation immediately prevents creation of new authenticated runtime sessions. Existing connections are closed/invalidated as soon as practical.

A revoked machine that reconnects with the old identity/key remains rejected. Re-pairing requires an explicit new trust action.

## Reinstall / Replacement

If a reinstall loses the private device identity, it is treated as a new Companion and paired again. Machine Role replacement remains separate from device trust.

## Runtime Replay Defense

Authenticated transport does not make stale work valid. Runtime requests still include execution IDs, Runtime Snapshot identity, deadlines/idempotency information and are rejected if stale or already resolved.

## Audit

Pair request, approval, rejection, revocation, identity mismatch and reconnect authentication failures create security audit records without recording private keys or pairing secrets.